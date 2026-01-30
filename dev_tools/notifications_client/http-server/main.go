package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gelembjuk/cleverchatty/notifications_shared"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Command line flags
	host := flag.String("host", "localhost", "Host to bind to")
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)

	// Create task manager
	taskManager := shared.NewTaskManager()

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"Task Notification Server (HTTP Streaming)",
		"1.0.0",
	)

	// Get tool definitions
	startTaskTool, taskStatusTool := shared.CreateMCPTools()

	// Add tools with handlers
	mcpServer.AddTool(startTaskTool, shared.CreateStartTaskHandler(taskManager))
	mcpServer.AddTool(taskStatusTool, shared.CreateTaskStatusHandler(taskManager))

	// Create HTTP streaming server
	httpServer := server.NewStreamableHTTPServer(mcpServer)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Printf("HTTP Streaming MCP Server starting on %s", addr)
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
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
