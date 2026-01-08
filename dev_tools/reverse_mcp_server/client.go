package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/mark3labs/mcp-go/server"
)

type ClientConfig struct {
	Host       string
	ServerName string
	AuthToken  string
	UseTLS     bool
	Insecure   bool
}

func StartReverseClient(ctx context.Context, stdioServer *server.StdioServer, config ClientConfig) error {
	// Build WebSocket URL
	scheme := "ws"
	if config.UseTLS {
		scheme = "wss"
	}
	u := url.URL{Scheme: scheme, Host: config.Host, Path: "/ws"}

	// Add query parameters
	q := u.Query()
	q.Set("server_name", config.ServerName)
	if config.AuthToken != "" {
		q.Set("token", config.AuthToken)
	}
	u.RawQuery = q.Encode()

	fmt.Printf("🔌 Connecting to %s as '%s'...\n", u.String(), config.ServerName)

	// Set up headers
	header := http.Header{}
	header.Set("X-MCP-Server-Name", config.ServerName)
	if config.AuthToken != "" {
		header.Set("Authorization", "Bearer "+config.AuthToken)
	}

	// Configure WebSocket dialer
	dialer := websocket.Dialer{}
	if config.UseTLS && config.Insecure {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	// Connect to WebSocket
	conn, resp, err := dialer.Dial(u.String(), header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to connect: %v (HTTP %d)", err, resp.StatusCode)
		}
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer conn.Close()

	fmt.Println("✅ Connected successfully!")

	// Create WebSocket adapter for stdio transport
	wsAdapter := NewWebSocketAdapter(conn)

	// Handle graceful shutdown
	shutdownCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			fmt.Println("\n👋 Shutting down...")
			cancel()
			conn.Close() // Close connection to stop Listen
		case <-shutdownCtx.Done():
			// Parent context cancelled
			conn.Close()
		}
	}()

	fmt.Println("🚀 Serving MCP over WebSocket...")

	// Serve MCP over the WebSocket connection
	if err := stdioServer.Listen(shutdownCtx, wsAdapter, wsAdapter); err != nil {
		if shutdownCtx.Err() != nil {
			// Context cancelled - graceful shutdown
			fmt.Println("👋 Server stopped")
			return nil
		}
		return fmt.Errorf("server error: %v", err)
	}
	return nil
}

// WebSocketAdapter adapts a websocket.Conn to io.Reader and io.Writer interfaces
// needed by the stdio server
type WebSocketAdapter struct {
	conn     *websocket.Conn
	readBuf  []byte
	readMux  sync.Mutex
	writeMux sync.Mutex
}

// NewWebSocketAdapter creates a new WebSocket adapter
func NewWebSocketAdapter(conn *websocket.Conn) *WebSocketAdapter {
	return &WebSocketAdapter{
		conn: conn,
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
