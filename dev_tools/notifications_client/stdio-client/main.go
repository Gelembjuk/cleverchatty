package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gelembjuk/cleverchatty/notifications_shared"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		cancel()
	}()

	// Get the path to the server executable
	serverPath := getServerPath()

	// Create MCP client that will run the server
	client, err := mcpclient.NewStdioMCPClient(
		serverPath,
		nil, // env
	)
	if err != nil {
		fmt.Printf("Failed to create MCP client: %v\n", err)
		os.Exit(1)
	}

	// Start the client (this launches the server process)
	if err := client.Start(ctx); err != nil {
		fmt.Printf("Failed to start MCP client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Initialize the MCP client
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "notifications_client",
		Version: "1.0.0",
	}
	initRequest.Params.Capabilities = mcp.ClientCapabilities{}

	if _, err := client.Initialize(ctx, initRequest); err != nil {
		fmt.Printf("Failed to initialize MCP client: %v\n", err)
		os.Exit(1)
	}

	// Set up notification handler
	client.OnNotification(func(notification mcp.JSONRPCNotification) {
		shared.HandleNotification(notification)
	})

	// Run the main task loop (all shared logic)
	shared.RunTaskLoop(ctx, client)
}

func getServerPath() string {
	// Try to find the compiled server binary
	// First check if it's already built
	possiblePaths := []string{
		"../stdio-server/stdio-server",
		"./stdio-server/stdio-server",
		"../stdio-server/server",
		"./stdio-server/server",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}

	// If not found, try to build it
	fmt.Println("Server binary not found, attempting to build...")
	serverDir := "../stdio-server"
	if _, err := os.Stat("./stdio-server"); err == nil {
		serverDir = "./stdio-server"
	}

	// Build command would be handled by the user
	fmt.Printf("Please build the server first:\n")
	fmt.Printf("  cd %s && go build -o stdio-server\n", serverDir)
	os.Exit(1)
	return ""
}
