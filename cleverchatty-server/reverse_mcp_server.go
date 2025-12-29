package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	cleverchatty "github.com/gelembjuk/cleverchatty/core"
	"github.com/gelembjuk/cleverchatty/core/history"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	// HeaderAuthToken is the header key for authentication token
	HeaderAuthToken = "Authorization"
	// HeaderMCPServerName is the header key for the MCP server name/identifier
	HeaderMCPServerName = "X-MCP-Server-Name"
)

// ReverseMCPConnection represents a single reverse MCP connection
type ReverseMCPConnection struct {
	ServerName    string
	Tools         []mcp.Tool
	ConnectedAt   time.Time
	cancel        context.CancelFunc
	requestChan   chan []byte // Channel for sending requests to remote
	pendingCalls  map[int64]chan json.RawMessage
	pendingMux    sync.Mutex
	nextRequestID int64
}

// ReverseMCPServer handles incoming MCP connections from remote servers
type ReverseMCPServer struct {
	Config         *cleverchatty.ReverseMCPServerConfig
	Logger         *log.Logger
	httpServer     *http.Server
	connections    map[string]*ReverseMCPConnection
	connectionsMux sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewReverseMCPServer creates a new reverse MCP server
func NewReverseMCPServer(
	config *cleverchatty.ReverseMCPServerConfig,
	logger *log.Logger,
) *ReverseMCPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &ReverseMCPServer{
		Config:      config,
		Logger:      logger,
		connections: make(map[string]*ReverseMCPConnection),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start begins the reverse MCP server
func (s *ReverseMCPServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleMCPConnection)

	s.httpServer = &http.Server{
		Addr:    s.Config.ListenHost,
		Handler: mux,
	}

	go func() {
		s.Logger.Printf("Reverse MCP server starting on %s", s.Config.ListenHost)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.Logger.Printf("Reverse MCP server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the reverse MCP server
func (s *ReverseMCPServer) Stop() error {
	s.cancel()

	// Close all connections
	s.connectionsMux.Lock()
	for name, conn := range s.connections {
		if conn.cancel != nil {
			conn.cancel()
		}
		delete(s.connections, name)
	}
	s.connectionsMux.Unlock()

	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleHealth handles health check requests
func (s *ReverseMCPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleMCPConnection handles incoming MCP connection requests
func (s *ReverseMCPServer) handleMCPConnection(w http.ResponseWriter, r *http.Request) {
	// Validate authentication first
	if !s.validateAuth(r) {
		s.Logger.Printf("Authentication failed for request from %s", r.RemoteAddr)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get server name from header
	serverName := r.Header.Get(HeaderMCPServerName)
	if serverName == "" {
		serverName = fmt.Sprintf("reverse-mcp-%s-%d", r.RemoteAddr, time.Now().Unix())
	}

	switch r.Method {
	case http.MethodGet:
		// SSE stream for sending requests to remote MCP server
		s.handleSSEConnection(w, r, serverName)
	case http.MethodPost:
		// Receive responses from remote MCP server
		s.handlePostResponse(w, r, serverName)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// validateAuth validates the authentication token from the request
func (s *ReverseMCPServer) validateAuth(r *http.Request) bool {
	if len(s.Config.AuthTokens) == 0 {
		return true
	}

	authHeader := r.Header.Get(HeaderAuthToken)
	if authHeader == "" {
		return false
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	token = strings.TrimSpace(token)

	for _, validToken := range s.Config.AuthTokens {
		if token == validToken {
			return true
		}
	}

	return false
}

// handleSSEConnection handles GET requests - SSE stream for sending requests to remote
func (s *ReverseMCPServer) handleSSEConnection(w http.ResponseWriter, r *http.Request, serverName string) {
	s.Logger.Printf("SSE connection from: %s", serverName)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Create connection context
	connCtx, connCancel := context.WithCancel(s.ctx)

	// Create or update connection
	conn := &ReverseMCPConnection{
		ServerName:    serverName,
		ConnectedAt:   time.Now(),
		cancel:        connCancel,
		requestChan:   make(chan []byte, 100),
		pendingCalls:  make(map[int64]chan json.RawMessage),
		nextRequestID: 1,
	}

	s.connectionsMux.Lock()
	// Close existing connection if any
	if existing, exists := s.connections[serverName]; exists {
		if existing.cancel != nil {
			existing.cancel()
		}
	}
	s.connections[serverName] = conn
	s.connectionsMux.Unlock()

	s.Logger.Printf("Connection established with %s", serverName)

	// Immediately send initialize request
	go s.initializeConnection(serverName)

	// Send requests via SSE
	for {
		select {
		case <-connCtx.Done():
			s.Logger.Printf("SSE connection closed for %s", serverName)
			s.removeConnection(serverName)
			return
		case <-r.Context().Done():
			s.Logger.Printf("Client disconnected: %s", serverName)
			s.removeConnection(serverName)
			return
		case req := <-conn.requestChan:
			// Send as SSE event
			fmt.Fprintf(w, "data: %s\n\n", string(req))
			flusher.Flush()
			s.Logger.Printf("Sent request to %s: %s", serverName, string(req))
		case <-time.After(30 * time.Second):
			// Heartbeat
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

// handlePostResponse handles POST requests - receiving responses from remote
func (s *ReverseMCPServer) handlePostResponse(w http.ResponseWriter, r *http.Request, serverName string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.Logger.Printf("Failed to read response body: %v", err)
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	s.Logger.Printf("Received from %s: %s", serverName, string(body))

	s.connectionsMux.RLock()
	conn, exists := s.connections[serverName]
	s.connectionsMux.RUnlock()

	if !exists {
		s.Logger.Printf("No connection for %s, message ignored", serverName)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse the response to get the ID
	var response struct {
		ID     interface{}     `json:"id"`
		Result json.RawMessage `json:"result,omitempty"`
		Error  json.RawMessage `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		s.Logger.Printf("Failed to parse response: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Get request ID
	var requestID int64
	switch id := response.ID.(type) {
	case float64:
		requestID = int64(id)
	case int64:
		requestID = id
	}

	// Find and notify pending call
	conn.pendingMux.Lock()
	if ch, ok := conn.pendingCalls[requestID]; ok {
		ch <- body
		delete(conn.pendingCalls, requestID)
	}
	conn.pendingMux.Unlock()

	w.WriteHeader(http.StatusOK)
}

// initializeConnection sends initialize and tools/list requests
func (s *ReverseMCPServer) initializeConnection(serverName string) {
	// Wait a moment for SSE connection to be ready
	time.Sleep(100 * time.Millisecond)

	s.connectionsMux.RLock()
	conn, exists := s.connections[serverName]
	s.connectionsMux.RUnlock()

	if !exists {
		return
	}

	// Send initialize request
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      conn.nextRequestID,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
			"clientInfo": map[string]interface{}{
				"name":    cleverchatty.ThisAppName,
				"version": cleverchatty.ThisAppVersion,
			},
			"capabilities": map[string]interface{}{},
		},
	}

	initResp, err := s.sendRequest(serverName, initReq)
	if err != nil {
		s.Logger.Printf("Initialize failed for %s: %v", serverName, err)
		return
	}
	s.Logger.Printf("Initialize response from %s: %s", serverName, string(initResp))

	// Send tools/list request
	conn.pendingMux.Lock()
	conn.nextRequestID++
	toolsReqID := conn.nextRequestID
	conn.pendingMux.Unlock()

	toolsReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      toolsReqID,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	toolsResp, err := s.sendRequest(serverName, toolsReq)
	if err != nil {
		s.Logger.Printf("Tools list failed for %s: %v", serverName, err)
		return
	}

	// Parse tools from response
	var toolsResponse struct {
		Result struct {
			Tools []mcp.Tool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(toolsResp, &toolsResponse); err != nil {
		s.Logger.Printf("Failed to parse tools response: %v", err)
		return
	}

	// Update connection with tools
	s.connectionsMux.Lock()
	if conn, exists := s.connections[serverName]; exists {
		conn.Tools = toolsResponse.Result.Tools
	}
	s.connectionsMux.Unlock()

	s.Logger.Printf("Discovered %d tools from %s", len(toolsResponse.Result.Tools), serverName)
	for _, tool := range toolsResponse.Result.Tools {
		s.Logger.Printf("  - %s: %s", tool.Name, tool.Description)
	}
}

// sendRequest sends a request and waits for response
func (s *ReverseMCPServer) sendRequest(serverName string, request map[string]interface{}) (json.RawMessage, error) {
	s.connectionsMux.RLock()
	conn, exists := s.connections[serverName]
	s.connectionsMux.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no connection for %s", serverName)
	}

	requestID := request["id"].(int64)

	// Create response channel
	respChan := make(chan json.RawMessage, 1)
	conn.pendingMux.Lock()
	conn.pendingCalls[requestID] = respChan
	conn.pendingMux.Unlock()

	// Marshal and send request
	reqBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	select {
	case conn.requestChan <- reqBytes:
	default:
		return nil, fmt.Errorf("request channel full")
	}

	// Wait for response with timeout
	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(30 * time.Second):
		conn.pendingMux.Lock()
		delete(conn.pendingCalls, requestID)
		conn.pendingMux.Unlock()
		return nil, fmt.Errorf("request timeout")
	}
}

// removeConnection removes a connection from the map
func (s *ReverseMCPServer) removeConnection(serverName string) {
	s.connectionsMux.Lock()
	var toolCount int
	if conn, exists := s.connections[serverName]; exists {
		toolCount = len(conn.Tools)
		if conn.cancel != nil {
			conn.cancel()
		}
		delete(s.connections, serverName)
	}
	s.connectionsMux.Unlock()

	if toolCount > 0 {
		s.Logger.Printf("Removed %d tools from disconnected server %s", toolCount, serverName)
	}
}

// GetTools implements ReverseMCPClient interface - returns tools from a specific connection
func (s *ReverseMCPServer) GetTools(serverName string) []mcp.Tool {
	s.connectionsMux.RLock()
	defer s.connectionsMux.RUnlock()

	if conn, exists := s.connections[serverName]; exists {
		return conn.Tools
	}
	return nil
}

// GetAllTools implements ReverseMCPClient interface - returns all tools from all connections
func (s *ReverseMCPServer) GetAllTools() map[string][]mcp.Tool {
	s.connectionsMux.RLock()
	defer s.connectionsMux.RUnlock()

	allTools := make(map[string][]mcp.Tool)
	for name, conn := range s.connections {
		if len(conn.Tools) > 0 {
			allTools[name] = conn.Tools
		}
	}
	return allTools
}

// CallTool implements ReverseMCPClient interface - calls a tool on a reverse-connected MCP server
func (s *ReverseMCPServer) CallTool(serverName, toolName string, args map[string]interface{}, ctx context.Context) (cleverchatty.ToolCallResult, error) {
	s.connectionsMux.RLock()
	conn, exists := s.connections[serverName]
	s.connectionsMux.RUnlock()

	if !exists {
		err := fmt.Errorf("no connection found for server: %s", serverName)
		return cleverchatty.ToolCallResult{Error: err}, err
	}

	conn.pendingMux.Lock()
	conn.nextRequestID++
	requestID := conn.nextRequestID
	conn.pendingMux.Unlock()

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
	}

	respBytes, err := s.sendRequest(serverName, req)
	if err != nil {
		return cleverchatty.ToolCallResult{Error: err}, err
	}

	// Parse response
	var response struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text,omitempty"`
			} `json:"content"`
			IsError bool `json:"isError,omitempty"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &response); err != nil {
		return cleverchatty.ToolCallResult{Error: fmt.Errorf("failed to parse response: %w", err)}, err
	}

	if response.Error != nil {
		err := fmt.Errorf("tool error: %s", response.Error.Message)
		return cleverchatty.ToolCallResult{Error: err}, err
	}

	// Convert to ToolCallResult
	result := cleverchatty.ToolCallResult{}
	for _, content := range response.Result.Content {
		if content.Type == "text" {
			result.Content = append(result.Content, history.TextContent{Text: content.Text})
		}
	}

	return result, nil
}
