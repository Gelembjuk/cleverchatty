package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gelembjuk/cleverchatty/notifications_shared"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	// Command line flags
	host := flag.String("host", "localhost", "Server host")
	port := flag.Int("port", 8080, "Server port")
	flag.Parse()

	serverURL := fmt.Sprintf("http://%s:%d/mcp", *host, *port)

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

	// Create MCP client that connects to HTTP streaming server
	// WithContinuousListening enables receiving server-to-client notifications
	client, err := mcpclient.NewStreamableHttpClient(
		serverURL,
		transport.WithContinuousListening(),
	)
	if err != nil {
		fmt.Printf("Failed to create MCP client: %v\n", err)
		os.Exit(1)
	}

	// Start the client
	if err := client.Start(ctx); err != nil {
		fmt.Printf("Failed to start MCP client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Initialize the MCP client
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "notifications_http_client",
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

	fmt.Printf("Connected to task notification server at %s\n", serverURL)

	// Run the main task loop (all shared logic)
	shared.RunTaskLoop(ctx, client)
}
