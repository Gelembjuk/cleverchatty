package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	cleverchatty "github.com/gelembjuk/cleverchatty/core"
	"github.com/gelembjuk/cleverchatty/core/history"
	"github.com/gorilla/websocket"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	// HeaderAuthToken is the header key for authentication token
	HeaderAuthToken = "Authorization"
	// HeaderMCPServerName is the header key for the MCP server name/identifier
	HeaderMCPServerName = "X-MCP-Server-Name"
)

// ReverseMCPConnection represents a single reverse MCP connection over WebSocket
type ReverseMCPConnection struct {
	ServerName  string
	Tools       []mcp.Tool
	ConnectedAt time.Time
	cancel      context.CancelFunc
	client      *mcpclient.Client
	wsConn      *websocket.Conn
}

// ReverseMCPConnector handles incoming MCP connections from remote MCP servers via WebSocket
type ReverseMCPConnector struct {
	Config         *cleverchatty.ReverseMCPListenerConfig
	ToolsServers   map[string]cleverchatty.ServerConfigWrapper
	Logger         *log.Logger
	httpServer     *http.Server
	listener       net.Listener
	connections    map[string]*ReverseMCPConnection
	connectionsMux sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	upgrader       websocket.Upgrader
}

// NewReverseMCPConnector creates a new reverse MCP connector
func NewReverseMCPConnector(
	config *cleverchatty.ReverseMCPListenerConfig,
	toolsServers map[string]cleverchatty.ServerConfigWrapper,
	logger *log.Logger,
) *ReverseMCPConnector {
	ctx, cancel := context.WithCancel(context.Background())
	return &ReverseMCPConnector{
		Config:       config,
		ToolsServers: toolsServers,
		Logger:       logger,
		connections:  make(map[string]*ReverseMCPConnection),
		ctx:          ctx,
		cancel:       cancel,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins, authentication is done via token
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// Start begins the reverse MCP connector
func (s *ReverseMCPConnector) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/", s.handleWebSocket) // Also handle root path

	s.httpServer = &http.Server{
		Handler: mux,
	}

	var err error
	if s.Config.TLS.Enabled {
		// TLS enabled - load certificates
		cert, err := tls.LoadX509KeyPair(s.Config.TLS.CertFile, s.Config.TLS.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS certificates: %w", err)
		}

		tlsConfig := &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{cert},
		}

		s.listener, err = tls.Listen("tcp", s.Config.ListenHost, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to start TLS listener: %w", err)
		}
		s.Logger.Printf("Reverse MCP connector (WebSocket/TLS) starting on wss://%s", s.Config.ListenHost)
	} else {
		// No TLS - plain TCP
		s.listener, err = net.Listen("tcp", s.Config.ListenHost)
		if err != nil {
			return fmt.Errorf("failed to start listener: %w", err)
		}
		s.Logger.Printf("Reverse MCP connector (WebSocket) starting on ws://%s", s.Config.ListenHost)
	}

	go func() {
		if err := s.httpServer.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.Logger.Printf("Reverse MCP connector error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the reverse MCP connector
func (s *ReverseMCPConnector) Stop() error {
	s.cancel()

	// Close all connections
	s.connectionsMux.Lock()
	for name, conn := range s.connections {
		if conn.cancel != nil {
			conn.cancel()
		}
		if conn.wsConn != nil {
			conn.wsConn.Close()
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
func (s *ReverseMCPConnector) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleWebSocket handles WebSocket upgrade and MCP connection
func (s *ReverseMCPConnector) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Get server name from header or query param - required for authentication
	serverName := r.Header.Get(HeaderMCPServerName)
	if serverName == "" {
		serverName = r.URL.Query().Get("server_name")
	}
	if serverName == "" {
		s.Logger.Printf("Missing server name from %s", r.RemoteAddr)
		http.Error(w, "Server name required", http.StatusBadRequest)
		return
	}

	// Validate authentication against the server config
	if !s.validateAuth(r, serverName) {
		s.Logger.Printf("Authentication failed for server %s from %s", serverName, r.RemoteAddr)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	wsConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.Logger.Printf("WebSocket upgrade failed for %s: %v", r.RemoteAddr, err)
		return
	}

	s.Logger.Printf("WebSocket connection established with %s", serverName)

	// Create connection context
	connCtx, connCancel := context.WithCancel(s.ctx)

	// Create the WebSocket adapter for MCP transport
	wsAdapter := NewWebSocketAdapter(wsConn, func() {
		s.Logger.Printf("Connection closed/error for %s", serverName)
		s.removeConnection(serverName)
	})

	// Create MCP client using the IO transport over WebSocket
	mcpTransport := transport.NewIO(wsAdapter, wsAdapter, nil)
	client := mcpclient.NewClient(mcpTransport)

	// Create connection record
	conn := &ReverseMCPConnection{
		ServerName:  serverName,
		ConnectedAt: time.Now(),
		cancel:      connCancel,
		client:      client,
		wsConn:      wsConn,
	}

	// Store connection
	s.connectionsMux.Lock()
	// Close existing connection if any
	if existing, exists := s.connections[serverName]; exists {
		if existing.cancel != nil {
			existing.cancel()
		}
		if existing.wsConn != nil {
			existing.wsConn.Close()
		}
	}
	s.connections[serverName] = conn
	s.connectionsMux.Unlock()

	// Initialize the MCP connection
	go s.initializeConnection(serverName, connCtx)

	// Keep connection alive and handle disconnection
	go s.monitorConnection(serverName, connCtx, wsConn)
}

// validateAuth validates the authentication token from the request against the server's config
func (s *ReverseMCPConnector) validateAuth(r *http.Request, serverName string) bool {
	// Look up the server in the tools servers config
	serverConfig, exists := s.ToolsServers[serverName]
	if !exists {
		s.Logger.Printf("Server %s not found in tools servers config", serverName)
		return false
	}

	// Check if it's a reverse MCP server
	if !serverConfig.IsReverseMCPServer() {
		s.Logger.Printf("Server %s is not configured as a reverse MCP server", serverName)
		return false
	}

	// Get the expected auth token for this server
	expectedToken := serverConfig.GetReverseMCPAuthToken()
	if expectedToken == "" {
		// No auth token configured - allow connection
		return true
	}

	// Get the provided token from header
	authHeader := r.Header.Get(HeaderAuthToken)
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		token = strings.TrimSpace(token)
		if token == expectedToken {
			return true
		}
	}

	// Check query parameter as fallback (useful for WebSocket clients)
	tokenParam := r.URL.Query().Get("token")
	if tokenParam == expectedToken {
		return true
	}

	return false
}

// initializeConnection initializes the MCP client and discovers tools
func (s *ReverseMCPConnector) initializeConnection(serverName string, ctx context.Context) {
	s.connectionsMux.RLock()
	conn, exists := s.connections[serverName]
	s.connectionsMux.RUnlock()

	if !exists || conn.client == nil {
		return
	}

	// Start the MCP client
	if err := conn.client.Start(ctx); err != nil {
		s.Logger.Printf("Failed to start MCP client for %s: %v", serverName, err)
		s.removeConnection(serverName)
		return
	}

	// Initialize the connection
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    cleverchatty.ThisAppName,
		Version: cleverchatty.ThisAppVersion,
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	_, err := conn.client.Initialize(ctx, initReq)
	if err != nil {
		s.Logger.Printf("Initialize failed for %s: %v", serverName, err)
		s.removeConnection(serverName)
		return
	}

	s.Logger.Printf("MCP connection initialized with %s", serverName)

	// List available tools
	toolsReq := mcp.ListToolsRequest{}
	toolsResp, err := conn.client.ListTools(ctx, toolsReq)
	if err != nil {
		s.Logger.Printf("Tools list failed for %s: %v", serverName, err)
		return
	}

	// Update connection with tools
	s.connectionsMux.Lock()
	if conn, exists := s.connections[serverName]; exists {
		conn.Tools = toolsResp.Tools
	}
	s.connectionsMux.Unlock()

	s.Logger.Printf("Discovered %d tools from %s", len(toolsResp.Tools), serverName)
	for _, tool := range toolsResp.Tools {
		s.Logger.Printf("  - %s: %s", tool.Name, tool.Description)
	}
}

// monitorConnection monitors the WebSocket connection and handles disconnection
func (s *ReverseMCPConnector) monitorConnection(serverName string, ctx context.Context, wsConn *websocket.Conn) {
	// Set up ping/pong for connection health
	wsConn.SetPongHandler(func(string) error {
		wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.Logger.Printf("Connection context cancelled for %s", serverName)
			s.removeConnection(serverName)
			return
		case <-ticker.C:
			if err := wsConn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
				s.Logger.Printf("Ping failed for %s: %v", serverName, err)
				s.removeConnection(serverName)
				return
			}
		}
	}
}

// removeConnection removes a connection from the map
func (s *ReverseMCPConnector) removeConnection(serverName string) {
	s.connectionsMux.Lock()
	var toolCount int
	if conn, exists := s.connections[serverName]; exists {
		toolCount = len(conn.Tools)
		if conn.cancel != nil {
			conn.cancel()
		}
		if conn.wsConn != nil {
			conn.wsConn.Close()
		}
		delete(s.connections, serverName)
	}
	s.connectionsMux.Unlock()

	if toolCount > 0 {
		s.Logger.Printf("Removed %d tools from disconnected server %s", toolCount, serverName)
	}
}

// GetTools implements ReverseMCPClient interface - returns tools from a specific connection
func (s *ReverseMCPConnector) GetTools(serverName string) []mcp.Tool {
	s.connectionsMux.RLock()
	defer s.connectionsMux.RUnlock()

	if conn, exists := s.connections[serverName]; exists {
		return conn.Tools
	}
	return nil
}

// GetAllTools implements ReverseMCPClient interface - returns all tools from all connections
func (s *ReverseMCPConnector) GetAllTools() map[string][]mcp.Tool {
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
func (s *ReverseMCPConnector) CallTool(serverName, toolName string, args map[string]interface{}, ctx context.Context) (cleverchatty.ToolCallResult, error) {
	s.connectionsMux.RLock()
	conn, exists := s.connections[serverName]
	s.connectionsMux.RUnlock()

	if !exists {
		err := fmt.Errorf("no connection found for server: %s", serverName)
		return cleverchatty.ToolCallResult{Error: err}, err
	}

	if conn.client == nil {
		err := fmt.Errorf("MCP client not initialized for server: %s", serverName)
		return cleverchatty.ToolCallResult{Error: err}, err
	}

	// Create tool call request
	callReq := mcp.CallToolRequest{}
	callReq.Params.Name = toolName
	callReq.Params.Arguments = args

	// Call the tool
	resp, err := conn.client.CallTool(ctx, callReq)
	if err != nil {
		return cleverchatty.ToolCallResult{Error: err}, err
	}

	// Convert to ToolCallResult
	result := cleverchatty.ToolCallResult{}
	for _, content := range resp.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			result.Content = append(result.Content, history.TextContent{Text: textContent.Text})
		}
	}

	if resp.IsError {
		// Extract error message from content if available
		var errMsg string
		for _, content := range resp.Content {
			if textContent, ok := content.(mcp.TextContent); ok {
				errMsg = textContent.Text
				break
			}
		}
		result.Error = fmt.Errorf("tool error: %s", errMsg)
	}

	return result, nil
}

// WebSocketAdapter adapts a websocket.Conn to io.Reader and io.Writer interfaces
// needed by transport.NewIO
type WebSocketAdapter struct {
	conn     *websocket.Conn
	readBuf  []byte
	readMux  sync.Mutex
	writeMux sync.Mutex
	onClose  func()
}

// NewWebSocketAdapter creates a new WebSocket adapter
func NewWebSocketAdapter(conn *websocket.Conn, onClose func()) *WebSocketAdapter {
	return &WebSocketAdapter{
		conn:    conn,
		onClose: onClose,
	}
}

// Read implements io.Reader interface
func (w *WebSocketAdapter) Read(p []byte) (n int, err error) {
	w.readMux.Lock()
	defer w.readMux.Unlock()

	// If we have leftover data from previous read, use it first
	if len(w.readBuf) > 0 {
		n = copy(p, w.readBuf)
		w.readBuf = w.readBuf[n:]
		return n, nil
	}

	// Read new message from WebSocket
	_, message, err := w.conn.ReadMessage()
	if err != nil {
		if w.onClose != nil {
			w.onClose()
		}
		return 0, err
	}

	// Ensure message ends with newline (JSON-RPC messages should be newline-delimited)
	if len(message) > 0 && message[len(message)-1] != '\n' {
		message = append(message, '\n')
	}

	n = copy(p, message)
	if n < len(message) {
		// Store the remainder for next read
		w.readBuf = message[n:]
	}
	return n, nil
}

// Write implements io.Writer interface
func (w *WebSocketAdapter) Write(p []byte) (n int, err error) {
	w.writeMux.Lock()
	defer w.writeMux.Unlock()

	err = w.conn.WriteMessage(websocket.TextMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close implements io.Closer interface
func (w *WebSocketAdapter) Close() error {
	return w.conn.Close()
}
