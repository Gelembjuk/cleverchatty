package cleverchatty

import (
	"context"
	"fmt"
	"log"
	"time"

	"strings"

	"github.com/gelembjuk/cleverchatty/history"
	"github.com/gelembjuk/cleverchatty/llm"
	"github.com/gelembjuk/cleverchatty/test"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	memoryToolRememberName = "remember"
	memoryToolRecallName   = "recall"
)

type MCPHost struct {
	config           map[string]ServerConfigWrapper
	logger           *log.Logger
	clients          map[string]mcpclient.MCPClient
	tools            []llm.Tool
	memoryServerName string
}

type ServerToolInfo struct {
	Name        string
	Description string
}

type ServerInfo struct {
	Name      string
	Err       error
	Transport string
	Command   string
	Url       string
	Headers   []string
	Args      []string
	Env       map[string]string
	Tools     []ServerToolInfo
}

func (si ServerInfo) GetType() string {
	switch si.Transport {
	case transportStdio:
		return transportStdio
	case transportSSE:
		return transportSSE
	default:
		return "unknown"
	}
}

func (si ServerInfo) IsSSE() bool {
	return si.Transport == transportSSE
}
func (si ServerInfo) IsStdio() bool {
	return si.Transport == transportStdio
}

func newMCPHost(
	mcpServersConfig map[string]ServerConfigWrapper,
	logger *log.Logger,
	ctx context.Context,
) (*MCPHost, error) {
	host := &MCPHost{
		config: mcpServersConfig,
		logger: logger,
	}

	err := host.createMCPClients()

	if err != nil {
		return nil, fmt.Errorf("failed to create MCP clients: %w", err)
	}

	err = host.loadMCPTools(ctx)
	if err != nil {
		host.Close()
		return nil, fmt.Errorf("failed to load MCP tools: %w", err)
	}

	return host, nil
}

func (host MCPHost) mcpToolsToAnthropicTools(
	serverName string,
	mcpTools []mcp.Tool,
) []llm.Tool {
	anthropicTools := make([]llm.Tool, len(mcpTools))

	for i, tool := range mcpTools {
		namespacedName := fmt.Sprintf("%s__%s", serverName, tool.Name)

		anthropicTools[i] = llm.Tool{
			Name:        namespacedName,
			Description: tool.Description,
			InputSchema: llm.Schema{
				Type:       tool.InputSchema.Type,
				Properties: tool.InputSchema.Properties,
				Required:   tool.InputSchema.Required,
			},
		}
	}

	return anthropicTools
}

// Set notifications callback for all servers
func (host MCPHost) SetNotificationCallback(
	callback func(server string, notification mcp.JSONRPCNotification),
) {
	for serverName, client := range host.clients {
		client.OnNotification(func(notification mcp.JSONRPCNotification) {
			callback(serverName, notification)
		})
	}
}

// Create MCP servers instances
func (host *MCPHost) createMCPClients() error {
	clients := make(map[string]mcpclient.MCPClient)

	for name, server := range host.config {

		if server.Disabled {
			continue
		}

		var client mcpclient.MCPClient
		var err error

		if server.Config.GetType() == transportSSE {
			sseConfig := server.Config.(SSEServerConfig)

			options := []transport.ClientOption{}

			if sseConfig.Headers != nil {
				// Parse headers from the config
				headers := make(map[string]string)
				for _, header := range sseConfig.Headers {
					parts := strings.SplitN(header, ":", 2)
					if len(parts) == 2 {
						key := strings.TrimSpace(parts[0])
						value := strings.TrimSpace(parts[1])
						headers[key] = value
					}
				}
				options = append(options, transport.WithHeaders(headers))
			}

			client, err = mcpclient.NewSSEMCPClient(
				sseConfig.Url,
				options...,
			)
		} else if server.Config.GetType() == transportInternal {
			internalConfig := server.Config.(InternalServerConfig)

			if internalConfig.Kind == "mock" {
				client = &test.MockMCPClient{}
				err = nil
			} else {
				err = fmt.Errorf("unknown internal server kind: %s", internalConfig.Kind)
			}
		} else {
			stdioConfig := server.Config.(STDIOServerConfig)
			var env []string
			for k, v := range stdioConfig.Env {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
			client, err = mcpclient.NewStdioMCPClient(
				stdioConfig.Command,
				env,
				stdioConfig.Args...)
		}
		if err == nil {
			err = client.(*mcpclient.Client).Start(context.Background())
		}
		if err != nil {
			for _, c := range clients {
				c.Close()
			}
			return fmt.Errorf(
				"failed to create MCP client for %s: %w",
				name,
				err,
			)
		}
		// TODO
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		host.logger.Printf("Initializing server...%s\n", name)
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    thisToolName,
			Version: thisToolVersion,
		}
		initRequest.Params.Capabilities = mcp.ClientCapabilities{}

		_, err = client.Initialize(ctx, initRequest)
		if err != nil {
			client.Close()
			for _, c := range clients {
				c.Close()
			}
			return fmt.Errorf(
				"failed to initialize MCP client for %s: %w",
				name,
				err,
			)
		}

		clients[name] = client

		if server.isMemoryServer() {
			host.memoryServerName = name
			host.logger.Printf("Memory server connected %s\n", name)
		}

		host.logger.Printf("Server connected %s\n", name)
	}

	host.clients = clients

	return nil
}
func (host *MCPHost) Close() error {
	errors := []error{}
	for _, client := range host.clients {
		err := client.Close()

		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to close MCP clients: %v", errors)
	}
	return nil
}
func (host *MCPHost) loadMCPTools(ctx context.Context) error {
	var allTools []llm.Tool
	for serverName, mcpClient := range host.clients {
		config, ok := host.config[serverName]

		if !ok {
			host.logger.Printf("Server %s not found in config\n", serverName)
			continue
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
		cancel()

		if err != nil {
			host.logger.Printf(
				"Error fetching tools from server %s: %v\n",
				serverName,
				err,
			)
			continue
		}

		filteredTools := []mcp.Tool{}

		for _, tool := range toolsResult.Tools {
			if config.isMemoryServer() {
				// Ignore memory-related tools
				if tool.Name == memoryToolRememberName ||
					tool.Name == memoryToolRecallName {
					continue
				}
			}
			filteredTools = append(filteredTools, tool)
		}

		serverTools := host.mcpToolsToAnthropicTools(serverName, filteredTools)
		allTools = append(allTools, serverTools...)

		host.logger.Printf(
			"Tools loaded from server %s: %d tools\n",
			serverName,
			len(filteredTools),
		)
	}
	host.tools = allTools
	return nil
}

func (host MCPHost) callTool(serverName string, toolName string, toolArgs map[string]interface{}, ctx context.Context) (*mcp.CallToolResult, error) {
	mcpClient, ok := host.clients[serverName]
	if !ok {
		return nil, fmt.Errorf("server not found: %s", serverName)
	}

	type result struct {
		toolResultPtr *mcp.CallToolResult
		err           error
	}

	resultCh := make(chan result, 1)

	go func() {

		req := mcp.CallToolRequest{}
		req.Params.Name = toolName
		req.Params.Arguments = toolArgs

		host.logger.Printf(
			"Tool %s called on server %s. Waiting response\n",
			toolName,
			serverName,
		)
		toolResultPtr, err := mcpClient.CallTool(
			ctx,
			req,
		)
		host.logger.Printf(
			"Response received for tool %s on server %s\n",
			toolName,
			serverName,
		)
		resultCh <- result{toolResultPtr: toolResultPtr, err: err}

	}()

	select {
	case res := <-resultCh:
		// done!
		return res.toolResultPtr, res.err
	case <-ctx.Done():
		// context cancelled or timed out
		return nil, ctx.Err()
	}
}

func (host MCPHost) getServersInfo() []ServerInfo {
	var servers []ServerInfo
	for name, server := range host.config {
		switch server.Config.(type) {
		case STDIOServerConfig:
			stdioServer := server.Config.(STDIOServerConfig)
			servers = append(servers, ServerInfo{
				Name:      name,
				Transport: transportStdio,
				Command:   stdioServer.Command,
				Args:      stdioServer.Args,
				Env:       stdioServer.Env,
			})
		case SSEServerConfig:
			sseServer := server.Config.(SSEServerConfig)
			servers = append(servers, ServerInfo{
				Name:      name,
				Transport: transportSSE,
				Url:       sseServer.Url,
				Headers:   sseServer.Headers,
			})
		default:
			host.logger.Printf("Unknown server type %T", server)
		}
	}
	return servers
}

func (host MCPHost) getToolsInfo() []ServerInfo {
	servers := host.getServersInfo()
	for i, server := range servers {

		mcpClient := host.clients[server.Name]
		ctx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
		if err != nil {
			servers[i].Err = fmt.Errorf(
				"failed to list tools: %w",
				err,
			)
			continue
		}

		tools := []ServerToolInfo{}
		if toolsResult != nil {
			for _, tool := range toolsResult.Tools {
				tools = append(tools, ServerToolInfo{
					Name:        tool.Name,
					Description: tool.Description,
				})
			}
		} else {
			servers[i].Err = fmt.Errorf("no tools found")
		}
		servers[i].Err = nil
		servers[i].Tools = tools
	}
	return servers
}

// if there is a memory MCP server, then it should be used. Send the messages to it
// this is async, so the messages are not sent immediately
func (host MCPHost) Remember(role string, content history.ContentBlock, ctx context.Context) {
	if host.memoryServerName == "" {
		return
	}
	if content.Type != "text" {
		return
	}
	host.logger.Printf(
		"Remembering message: %s %s\n",
		role,
		content.Text,
	)
	// call the memory server to remember the messages
	_, err := host.callTool(
		host.memoryServerName,
		memoryToolRememberName,
		map[string]interface{}{
			"role":     role,
			"contents": content.Text,
		},
		ctx,
	)
	if err != nil {
		host.logger.Printf(
			"Error remembering message: %v\n",
			err,
		)
		return
	}
}

// requests the memory server to recall the messages
func (host MCPHost) Recall(ctx context.Context) (string, error) {
	if host.memoryServerName == "" {
		return "", nil
	}

	// call the memory server to recall the messages
	toolResultPtr, err := host.callTool(
		host.memoryServerName,
		memoryToolRecallName,
		map[string]interface{}{},
		ctx,
	)
	if err != nil {
		host.logger.Printf(
			"Error recalling messages: %v\n",
			err,
		)
		return "", err
	}
	if toolResultPtr == nil {
		return "", fmt.Errorf("no result from memory server")
	}
	if toolResultPtr.Content == nil {
		return "", fmt.Errorf("no content from memory server")
	}
	var resultText string
	for _, item := range toolResultPtr.Content {
		if contentMap, ok := item.(mcp.TextContent); ok {
			resultText += fmt.Sprintf("%v ", contentMap.Text)
		}
	}
	resultText = strings.TrimSpace(resultText)

	if resultText == "none" {
		return "", nil
	}

	return strings.TrimSpace(resultText), nil
}

func (host MCPHost) getContentOnNotification(
	serverName string,
	notification mcp.JSONRPCNotification,
	ctx context.Context,
	requestStartCallback func(string),
) (toolName string, resultContent string, err error) {

	methodName := notification.Method
	// if the method is not like tool/something then return
	if !strings.HasPrefix(methodName, "tool/") {
		return
	}
	toolName = strings.TrimPrefix(methodName, "tool/")
	params := notification.Params.AdditionalFields

	if content, ok := params["_content"]; ok {
		// if there is content then we just use it as a tool result
		contentMap, ok := content.(map[string]interface{})
		if !ok {
			err = fmt.Errorf(
				"Error converting content to map[string]interface{}: %v",
				content,
			)
			return
		}
		contentType, ok := contentMap["type"].(string)
		if !ok {
			err = fmt.Errorf(
				"Error converting content type to string: %v",
				contentMap["type"],
			)
			return
		}
		if contentType != "text" {
			err = fmt.Errorf(
				"Error: content type is not text: %v",
				contentType,
			)
			return
		}
		text, ok := contentMap["text"].(string)
		if !ok {
			err = fmt.Errorf(
				"Error converting content text to string: %v",
				contentMap["text"],
			)
			return
		}

		resultContent = text

		return
	}
	// if there is no content then we call the tool using the provided parameters.
	// we presume the method contains the tool name

	requestStartCallback(serverName + "__" + toolName)

	var toolResultPtr *mcp.CallToolResult

	toolResultPtr, err = host.callTool(
		serverName,
		toolName,
		params,
		ctx,
	)

	if err != nil {
		err = fmt.Errorf(
			"Error calling tool after notification received %s: %v",
			toolName,
			err,
		)
		return
	}

	toolResult := *toolResultPtr

	if toolResult.Content == nil {
		// no error but no content
		return
	}

	if len(toolResult.Content) == 0 {
		// no error but no content
		return
	}
	// Extract text content
	var resultText string
	// Handle array content directly since we know it's []interface{}
	for _, item := range toolResult.Content {
		if contentMap, ok := item.(mcp.TextContent); ok {
			resultText += fmt.Sprintf("%v ", contentMap.Text)
		}
	}

	resultContent = strings.TrimSpace(resultText)

	return
}
