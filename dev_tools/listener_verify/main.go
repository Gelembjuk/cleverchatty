package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
)

// JSONRPCRequest represents a JSON-RPC request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// TestMCPServer simulates an MCP server that connects to the reverse connector
type TestMCPServer struct {
	serverName string
	authToken  string
	tools      []Tool
	conn       *websocket.Conn
}

func main() {
	// Command line flags
	host := flag.String("host", "localhost:9090", "Host:port of the reverse MCP connector")
	serverName := flag.String("name", "test-mcp-server", "Server name to identify this MCP server")
	authToken := flag.String("token", "", "Authentication token")
	useTLS := flag.Bool("tls", false, "Use TLS (wss://)")
	flag.Parse()

	// Create test server with sample tools
	server := &TestMCPServer{
		serverName: *serverName,
		authToken:  *authToken,
		tools: []Tool{
			{
				Name:        "echo",
				Description: "Echoes back the input message",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"message": map[string]interface{}{
							"type":        "string",
							"description": "Message to echo",
						},
					},
					"required": []string{"message"},
				},
			},
			{
				Name:        "add",
				Description: "Adds two numbers together",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"a": map[string]interface{}{
							"type":        "number",
							"description": "First number",
						},
						"b": map[string]interface{}{
							"type":        "number",
							"description": "Second number",
						},
					},
					"required": []string{"a", "b"},
				},
			},
			{
				Name:        "get_time",
				Description: "Returns the current server time",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}

	// Build WebSocket URL
	scheme := "ws"
	if *useTLS {
		scheme = "wss"
	}
	u := url.URL{Scheme: scheme, Host: *host, Path: "/ws"}

	// Add query parameters
	q := u.Query()
	q.Set("server_name", server.serverName)
	if server.authToken != "" {
		q.Set("token", server.authToken)
	}
	u.RawQuery = q.Encode()

	log.Printf("Connecting to %s as '%s'...", u.String(), server.serverName)

	// Set up headers
	header := http.Header{}
	header.Set("X-MCP-Server-Name", server.serverName)
	if server.authToken != "" {
		header.Set("Authorization", "Bearer "+server.authToken)
	}

	// Connect to WebSocket
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		if resp != nil {
			log.Fatalf("Failed to connect: %v (HTTP %d)", err, resp.StatusCode)
		}
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	server.conn = conn

	log.Println("Connected successfully!")
	log.Printf("Registered %d tools: ", len(server.tools))
	for _, tool := range server.tools {
		log.Printf("  - %s: %s", tool.Name, tool.Description)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start message handler
	done := make(chan struct{})
	go server.handleMessages(done)

	// Wait for shutdown signal
	select {
	case <-sigChan:
		log.Println("Shutting down...")
	case <-done:
		log.Println("Connection closed")
	}
}

func (s *TestMCPServer) handleMessages(done chan struct{}) {
	defer close(done)

	for {
		_, message, err := s.conn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
			return
		}

		log.Printf("Received: %s", string(message))

		// Parse JSON-RPC request
		var req JSONRPCRequest
		if err := json.Unmarshal(message, &req); err != nil {
			log.Printf("Failed to parse request: %v", err)
			continue
		}

		// Handle the request
		response := s.handleRequest(&req)
		if response != nil {
			respBytes, err := json.Marshal(response)
			if err != nil {
				log.Printf("Failed to marshal response: %v", err)
				continue
			}

			log.Printf("Sending: %s", string(respBytes))
			if err := s.conn.WriteMessage(websocket.TextMessage, respBytes); err != nil {
				log.Printf("Write error: %v", err)
				return
			}
		}
	}
}

func (s *TestMCPServer) handleRequest(req *JSONRPCRequest) *JSONRPCResponse {
	// Notifications have no ID - don't respond to them
	if req.ID == nil {
		log.Printf("Received notification: %s (no response needed)", req.Method)
		return nil
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	default:
		log.Printf("Unknown method: %s", req.Method)
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s *TestMCPServer) handleInitialize(req *JSONRPCRequest) *JSONRPCResponse {
	log.Println("Handling initialize request")
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]interface{}{
				"name":    s.serverName,
				"version": "1.0.0",
			},
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
		},
	}
}

func (s *TestMCPServer) handleToolsList(req *JSONRPCRequest) *JSONRPCResponse {
	log.Println("Handling tools/list request")
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"tools": s.tools,
		},
	}
}

func (s *TestMCPServer) handleToolsCall(req *JSONRPCRequest) *JSONRPCResponse {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Invalid params: %v", err),
			},
		}
	}

	log.Printf("Handling tools/call: %s with args: %v", params.Name, params.Arguments)

	var result string
	var isError bool

	switch params.Name {
	case "echo":
		if msg, ok := params.Arguments["message"].(string); ok {
			result = fmt.Sprintf("Echo: %s", msg)
		} else {
			result = "Error: message argument required"
			isError = true
		}

	case "add":
		a, aOk := params.Arguments["a"].(float64)
		b, bOk := params.Arguments["b"].(float64)
		if aOk && bOk {
			result = fmt.Sprintf("Result: %v + %v = %v", a, b, a+b)
		} else {
			result = "Error: a and b arguments required (numbers)"
			isError = true
		}

	case "get_time":
		result = fmt.Sprintf("Current time: %s", "2026-01-07T12:00:00Z")

	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Unknown tool: %s", params.Name),
			},
		}
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": result,
				},
			},
			"isError": isError,
		},
	}
}

// Interactive mode for manual testing
func runInteractive(conn *websocket.Conn) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Interactive mode. Type messages to send (Ctrl+C to exit):")

	for scanner.Scan() {
		text := scanner.Text()
		if text == "" {
			continue
		}

		if err := conn.WriteMessage(websocket.TextMessage, []byte(text)); err != nil {
			log.Printf("Write error: %v", err)
			return
		}
	}
}
