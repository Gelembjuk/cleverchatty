package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Command line flags
	host := flag.String("host", "localhost:9090", "Host:port of the reverse MCP connector")
	serverName := flag.String("name", "reverse-mcp-server", "Server name to identify this MCP server")
	authToken := flag.String("token", "", "Authentication token")
	useTLS := flag.Bool("tls", false, "Use TLS (wss://)")
	insecure := flag.Bool("insecure", false, "Skip TLS certificate verification (for self-signed certs)")
	flag.Parse()

	// Create the stdio server (with tools registered)
	stdio := createStdioServer()

	// Configuration for the reverse client
	config := ClientConfig{
		Host:       *host,
		ServerName: *serverName,
		AuthToken:  *authToken,
		UseTLS:     *useTLS,
		Insecure:   *insecure,
	}

	// Start the reverse client
	if err := StartReverseClient(context.Background(), stdio, config); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func createStdioServer() *server.StdioServer {
	// Create MCP server
	s := server.NewMCPServer(
		"Server to manage a Linux instance",
		"1.0.0",
	)

	// Define the exec_cmd tool
	execTool := mcp.NewTool("exec_cmd",
		mcp.WithDescription("Execute a Linux command with optional working directory"),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("The full shell command to execute"),
		),
		mcp.WithString("working_dir",
			mcp.Description("Optional working directory where the command should run"),
		),
	)

	// Add the tool (no auth wrapper needed - auth is done at WebSocket connection level)
	s.AddTool(execTool, execCmdHandler)

	return server.NewStdioServer(s)
}

func execCmdHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	cmdStr, ok := args["command"].(string)
	if !ok || cmdStr == "" {
		return mcp.NewToolResultError("command is required"), nil
	}

	// Optional working_dir
	var workingDir string
	if wd, ok := args["working_dir"].(string); ok {
		workingDir = wd
	}

	// Use "sh -c" to allow full shell command with arguments and operators
	cmd := exec.Command("sh", "-c", cmdStr)

	if workingDir != "" {
		cmd.Dir = workingDir
	}

	output, err := cmd.CombinedOutput()

	if err != nil {
		// Include both the error and output for context
		return mcp.NewToolResultError(fmt.Sprintf("execution failed: %v\n%s", err, output)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}
