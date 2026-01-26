package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/gelembjuk/cleverchatty/email_mock_shared"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Command line flags
	verbose := flag.Bool("verbose", false, "Enable verbose logging to stderr (disabled by default for stdio)")
	flag.Parse()

	// Set verbose mode in shared package
	shared.SetVerbose(*verbose)

	// Create email manager
	emailManager := shared.NewEmailManager()

	// Create MCP server
	s := server.NewMCPServer(
		"Email Mock Server (Stdio)",
		"1.0.0",
	)

	// Get tool definitions
	getEmailsTool, markEmailReadTool := shared.CreateMCPTools()

	// Add tools with handlers
	s.AddTool(getEmailsTool, shared.CreateGetEmailsHandler(emailManager))
	s.AddTool(markEmailReadTool, shared.CreateMarkEmailReadHandler(emailManager))

	// Create context for server and email sender
	ctx := context.Background()

	// Start email notification sender immediately
	// It will broadcast to all connected clients
	shared.StartEmailNotificationSender(ctx, s, emailManager)

	// Start stdio server
	stdio := server.NewStdioServer(s)
	if err := stdio.Listen(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
