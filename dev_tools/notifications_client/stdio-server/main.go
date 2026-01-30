package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gelembjuk/cleverchatty/notifications_shared"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Create task manager
	taskManager := shared.NewTaskManager()

	// Create MCP server
	s := server.NewMCPServer(
		"Task Notification Server (Stdio)",
		"1.0.0",
	)

	// Get tool definitions
	startTaskTool, taskStatusTool := shared.CreateMCPTools()

	// Add tools with handlers
	s.AddTool(startTaskTool, shared.CreateStartTaskHandler(taskManager))
	s.AddTool(taskStatusTool, shared.CreateTaskStatusHandler(taskManager))

	// Start stdio server
	stdio := server.NewStdioServer(s)
	if err := stdio.Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
