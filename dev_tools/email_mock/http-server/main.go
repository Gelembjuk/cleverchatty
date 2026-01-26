package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gelembjuk/cleverchatty/email_mock_shared"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Command line flags
	host := flag.String("host", "localhost", "Host to bind to")
	port := flag.Int("port", 8081, "Port to listen on")
	verbose := flag.Bool("verbose", true, "Enable verbose logging (enabled by default for HTTP)")
	flag.Parse()

	// Set verbose mode in shared package
	shared.SetVerbose(*verbose)

	addr := fmt.Sprintf("%s:%d", *host, *port)

	// Create email manager
	emailManager := shared.NewEmailManager()

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"Email Mock Server (HTTP Streaming)",
		"1.0.0",
	)

	// Get tool definitions
	getEmailsTool, markEmailReadTool := shared.CreateMCPTools()

	// Add tools with handlers
	mcpServer.AddTool(getEmailsTool, shared.CreateGetEmailsHandler(emailManager))
	mcpServer.AddTool(markEmailReadTool, shared.CreateMarkEmailReadHandler(emailManager))

	// Create HTTP streaming server
	httpServer := server.NewStreamableHTTPServer(mcpServer)

	// Setup graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start email notification sender immediately
	// It will broadcast to all connected clients
	shared.StartEmailNotificationSender(ctx, mcpServer, emailManager)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Printf("HTTP Streaming Email Mock Server starting on %s", addr)
		log.Printf("MCP endpoint: http://%s/mcp", addr)
		log.Printf("Press Ctrl+C to stop")
		if err := httpServer.Start(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt
	<-sigChan
	log.Println("\nReceived interrupt signal, shutting down...")

	// Graceful shutdown
	cancel() // Stop email sender
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
