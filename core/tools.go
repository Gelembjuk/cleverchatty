package core

import (
	"context"
	"fmt"
	"log"
	"time"

	"strings"

	"github.com/gelembjuk/cleverchatty/core/history"
	"github.com/gelembjuk/cleverchatty/core/llm"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	memoryToolRememberName = "remember"
	memoryToolRecallName   = "recall"
	ragToolName            = "knowledge_search"
)

type ToolsHost struct {
	config           map[string]ServerConfigWrapper
	context          context.Context
	clientAgentID    string
	AgentID          string
	AgentName        string
	logger           *log.Logger
	mcpClients       map[string]mcpclient.MCPClient
	a2aClients       map[string]A2AAgent
	tools            []llm.Tool
	memoryServerName string
	ragServerName    string
}

type ToolCallResult struct {
	Content []history.Content
	Error   error
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
	Endpoint  string
	Headers   []string
	Args      []string
	Env       map[string]string
	Metadata  map[string]string
	Tools     []ServerToolInfo
}

func (si ServerInfo) GetType() string {
	switch si.Transport {
	case transportStdio:
		return transportStdio
	case transportSSE:
		return transportSSE
	case transportHTTPStreaming:
		return transportHTTPStreaming
	case transportA2A:
		return transportA2A
	default:
		return "unknown"
	}
}

func (si ServerInfo) IsMCP() bool {
	return si.Transport == transportSSE ||
		si.Transport == transportHTTPStreaming ||
		si.Transport == transportStdio
}
func (si ServerInfo) IsA2A() bool {
	return si.Transport == transportA2A
}

func (si ServerInfo) IsMCPSSEServer() bool {
	return si.Transport == transportSSE
}
func (si ServerInfo) IsMCPStdioServer() bool {
	return si.Transport == transportStdio
}
func (si ServerInfo) IsMCPHTTPStreamingServer() bool {
	return si.Transport == transportHTTPStreaming
}
func (si ServerInfo) IsA2AServer() bool {
	return si.Transport == transportA2A
}

func (tc ToolCallResult) getTextContent() string {
	var textContent strings.Builder
	for _, content := range tc.Content {
		if textC, ok := content.(history.TextContent); ok {
			textContent.WriteString(textC.Text)
		}
	}
	return strings.TrimSpace(textContent.String())
}

func (tc *ToolCallResult) validateNotEmpty() {
	if tc.Content == nil {
		tc.Error = fmt.Errorf("no content from tool call")
		return
	}
	if len(tc.Content) == 0 {
		tc.Error = fmt.Errorf("no content from tool call")
		return
	}
}

func newToolsHost(
	mcpServersConfig map[string]ServerConfigWrapper,
	logger *log.Logger,
	ctx context.Context,
) (*ToolsHost, error) {
	host := &ToolsHost{
		config:  mcpServersConfig,
		context: ctx,
		logger:  logger,
	}

	return host, nil
}

func (host *ToolsHost) Init() error {
	err := host.createMCPClients()

	if err != nil {
		return fmt.Errorf("failed to create MCP clients: %w", err)
	}

	err = host.createA2AClients()
	if err != nil {
		return fmt.Errorf("failed to create A2A clients: %w", err)
	}

	host.tools = []llm.Tool{}

	err = host.loadMCPTools(host.context)
	if err != nil {
		host.Close()
		return fmt.Errorf("failed to load MCP tools: %w", err)
	}

	err = host.loadA2ATools(host.context)
	if err != nil {
		host.Close()
		return fmt.Errorf("failed to load A2A tools: %w", err)
	}

	return nil
}

func (host ToolsHost) isMCPServer(serverName string) bool {
	_, ok := host.mcpClients[serverName]
	return ok
}
func (host ToolsHost) isA2AServer(serverName string) bool {
	_, ok := host.a2aClients[serverName]
	return ok
}

func (host ToolsHost) mcpToolsToAnthropicTools(
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

// The method replaces some templates with internal values
// like agentid, sessionid, etc.
func (host *ToolsHost) filterConfigValue(value string) string {
	value = strings.ReplaceAll(value, "{CLIENT_AGENT_ID}", host.clientAgentID)
	value = strings.ReplaceAll(value, "{AGENT_ID}", host.AgentID)
	return value
}

// Create MCP servers instances
func (host *ToolsHost) createMCPClients() error {
	clients := make(map[string]mcpclient.MCPClient)

	for name, server := range host.config {

		if server.Disabled {
			continue
		}

		if !server.isMCPServer() {
			continue
		}

		var client mcpclient.MCPClient
		var err error

		if server.Config.GetType() == transportSSE {
			sseConfig := server.Config.(SSEMCPServerConfig)

			options := []transport.ClientOption{}

			if sseConfig.Headers != nil {
				// Parse headers from the config
				headers := make(map[string]string)
				for _, header := range sseConfig.Headers {
					parts := strings.SplitN(header, ":", 2)
					if len(parts) == 2 {
						key := strings.TrimSpace(parts[0])
						value := strings.TrimSpace(parts[1])
						// Replace placeholders in header values
						value = host.filterConfigValue(value)
						headers[key] = value
					}
				}
				options = append(options, transport.WithHeaders(headers))
			}

			client, err = mcpclient.NewSSEMCPClient(
				sseConfig.Url,
				options...,
			)
		} else if server.Config.GetType() == transportHTTPStreaming {
			httpConfig := server.Config.(HTTPStreamingMCPServerConfig)

			options := []transport.StreamableHTTPCOption{}

			if httpConfig.Headers != nil {
				// Parse headers from the config
				headers := make(map[string]string)
				for _, header := range httpConfig.Headers {
					parts := strings.SplitN(header, ":", 2)
					if len(parts) == 2 {
						key := strings.TrimSpace(parts[0])
						value := strings.TrimSpace(parts[1])
						value = host.filterConfigValue(value)
						headers[key] = value
					}
				}
				options = append(options, transport.WithHTTPHeaders(headers))
			}

			client, err = mcpclient.NewStreamableHttpClient(
				httpConfig.Url,
				options...,
			)
		} else if server.Config.GetType() == transportInternal {
			internalConfig := server.Config.(InternalServerConfig)

			err = fmt.Errorf("unknown internal server kind: %s", internalConfig.Kind)
		} else {
			stdioConfig := server.Config.(STDIOMCPServerConfig)
			var env []string
			for k, v := range stdioConfig.Env {
				// Replace placeholders in environment variables
				v = host.filterConfigValue(v)
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
			var stdioArgs []string
			for _, arg := range stdioConfig.Args {
				arg = host.filterConfigValue(arg)
				stdioArgs = append(stdioArgs, arg)
			}
			client, err = mcpclient.NewStdioMCPClient(
				stdioConfig.Command,
				env,
				stdioArgs...)
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
		if server.isRAGServer() {
			host.ragServerName = name
			host.logger.Printf("RAG server connected %s\n", name)
		}

		host.logger.Printf("Server connected %s\n", name)
	}

	host.mcpClients = clients

	return nil
}

func (host *ToolsHost) createA2AClients() error {
	clients := make(map[string]A2AAgent)

	for name, server := range host.config {

		if server.Disabled {
			continue
		}

		if !server.isA2AServer() {
			continue
		}

		config := server.Config.(A2AToolsServerConfig)

		agent, err := NewA2AAgent(config.Endpoint, config.Metadata, host.logger)
		if err != nil {
			return fmt.Errorf("failed to fetch agent card for %s: %w", name, err)
		}

		agent.filterFunc = host.filterConfigValue
		agent.HostingAgentID = host.AgentID
		agent.HostingAgentTitle = host.AgentName

		clients[name] = *agent

		if server.isMemoryServer() {
			host.memoryServerName = name
			host.logger.Printf("Memory server connected %s\n", name)
		}
		if server.isRAGServer() {
			host.ragServerName = name
			host.logger.Printf("RAG server connected %s\n", name)
		}

		host.logger.Printf("Server connected %s\n", name)
	}

	host.a2aClients = clients

	return nil
}

// Check if the host has a RAG server connected
func (host ToolsHost) HasRagServer() bool {
	return host.ragServerName != ""
}
func (host *ToolsHost) Close() error {
	errors := []error{}
	for _, client := range host.mcpClients {
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
func (host *ToolsHost) loadMCPTools(ctx context.Context) error {
	var allTools []llm.Tool
	for serverName, mcpClient := range host.mcpClients {
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
			if config.isRAGServer() {
				// Ignore RAG-related tools
				if tool.Name == ragToolName {
					continue
				}
			}
			host.logger.Printf("Tool %s loaded from server %s\n", tool.Name, serverName)
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
	host.tools = append(host.tools, allTools...)
	return nil
}

func (host *ToolsHost) loadA2ATools(ctx context.Context) error {
	var allTools []llm.Tool
	for serverName, a2aClient := range host.a2aClients {
		config, ok := host.config[serverName]

		if !ok {
			host.logger.Printf("Server %s not found in config\n", serverName)
			continue
		}

		serverTools := []llm.Tool{}

		for _, a2aSkill := range a2aClient.Card.Skills {
			if config.isMemoryServer() {
				// Ignore memory-related tools
				if a2aSkill.ID == memoryToolRememberName ||
					a2aSkill.ID == memoryToolRecallName {
					continue
				}
			}
			if config.isRAGServer() {
				// Ignore RAG-related tools
				if a2aSkill.ID == ragToolName {
					continue
				}
			}
			tool := llm.Tool{
				Name:        fmt.Sprintf("%s__%s", serverName, a2aSkill.ID),
				Description: a2aSkill.Name + "\n" + a2aSkill.Description,
				InputSchema: llm.Schema{
					Type: "object",
					Properties: map[string]any{
						"message": map[string]any{
							"description": a2aSkill.Name + ". " + a2aSkill.Description,
						},
					},
				},
			}
			serverTools = append(serverTools, tool)
		}

		allTools = append(allTools, serverTools...)

		host.logger.Printf(
			"Tools loaded from server %s: %d tools\n",
			serverName,
			len(serverTools),
		)
	}
	host.tools = append(host.tools, allTools...)
	return nil
}

func (host ToolsHost) callTool(serverName string, toolName string, toolArgs map[string]interface{}, ctx context.Context) ToolCallResult {
	if host.isMCPServer(serverName) {
		return host.callMCPTool(serverName, toolName, toolArgs, ctx)
	}
	if host.isA2AServer(serverName) {
		if agentCard, ok := host.a2aClients[serverName]; ok {
			return agentCard.sendMessage(toolName, toolArgs, ctx)
		}
		return ToolCallResult{
			Error: fmt.Errorf("A2A server %s not found", serverName),
		}
	}
	return ToolCallResult{
		Error: fmt.Errorf("server %s is not a valid MCP or A2A server", serverName),
	}
}

func (host ToolsHost) callMCPTool(serverName string, toolName string, toolArgs map[string]interface{}, ctx context.Context) ToolCallResult {
	mcpClient, ok := host.mcpClients[serverName]
	if !ok {
		return ToolCallResult{
			Error: fmt.Errorf("server %s not found", serverName),
		}
	}

	resultCh := make(chan ToolCallResult, 1)

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
		result := ToolCallResult{
			Content: []history.Content{},
			Error:   err,
		}
		if err == nil {
			toolResult := *toolResultPtr

			if toolResult.Content != nil {
				for _, content := range toolResult.Content {
					switch content := content.(type) {
					case mcp.TextContent:
						// Convert mcp.TextContent to history.TextContent
						result.Content = append(result.Content, history.TextContent{
							Type: "text",
							Text: content.Text,
						})
					case mcp.ImageContent:
						// Convert mcp.ImageContent to history.ImageContent
						// TODO: handle image content
					default:
					}
				}
			}
			result.validateNotEmpty()
		}
		resultCh <- result

	}()

	select {
	case res := <-resultCh:
		// done!
		return res
	case <-ctx.Done():
		// context cancelled or timed out
		return ToolCallResult{
			Error: ctx.Err(),
		}
	}
}

func (host ToolsHost) getServersInfo() []ServerInfo {
	var servers []ServerInfo
	for name, server := range host.config {
		switch server.Config.(type) {
		case STDIOMCPServerConfig:
			stdioServer := server.Config.(STDIOMCPServerConfig)
			servers = append(servers, ServerInfo{
				Name:      name,
				Transport: transportStdio,
				Command:   stdioServer.Command,
				Args:      stdioServer.Args,
				Env:       stdioServer.Env,
			})
		case SSEMCPServerConfig:
			sseServer := server.Config.(SSEMCPServerConfig)
			servers = append(servers, ServerInfo{
				Name:      name,
				Transport: transportSSE,
				Url:       sseServer.Url,
				Headers:   sseServer.Headers,
			})
		case HTTPStreamingMCPServerConfig:
			httpServer := server.Config.(HTTPStreamingMCPServerConfig)
			servers = append(servers, ServerInfo{
				Name:      name,
				Transport: transportHTTPStreaming,
				Url:       httpServer.Url,
				Headers:   httpServer.Headers,
			})
		case A2AToolsServerConfig:
			a2aServer := server.Config.(A2AToolsServerConfig)
			servers = append(servers, ServerInfo{
				Name:      name,
				Transport: transportA2A,
				Endpoint:  a2aServer.Endpoint,
				Metadata:  a2aServer.Metadata,
			})
		case InternalServerConfig:
			internalServer := server.Config.(InternalServerConfig)
			servers = append(servers, ServerInfo{
				Name:      name,
				Transport: transportInternal,
				Command:   internalServer.Kind,
			})
		default:
			host.logger.Printf("Unknown server type %T", server)
		}
	}
	return servers
}

func (host ToolsHost) getToolsInfo() []ServerInfo {
	servers := host.getServersInfo()
	for i, server := range servers {
		var toolsResult *mcp.ListToolsResult

		mcpClient := host.mcpClients[server.Name]
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
func (host ToolsHost) Remember(role string, content history.ContentBlock, ctx context.Context) {
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
	res := host.callTool(
		host.memoryServerName,
		memoryToolRememberName,
		map[string]interface{}{
			"role":     role,
			"contents": content.Text,
		},
		ctx,
	)
	if res.Error != nil {
		host.logger.Printf(
			"Error remembering message: %v\n",
			res.Error,
		)
		return
	}
}

// requests the memory server to recall the messages
func (host ToolsHost) Recall(ctx context.Context, prompt string) (string, error) {
	if host.memoryServerName == "" {
		return "", nil
	}

	// call the memory server to recall the messages
	res := host.callTool(
		host.memoryServerName,
		memoryToolRecallName,
		map[string]interface{}{
			"query": prompt,
		},
		ctx,
	)
	if res.Error != nil {
		host.logger.Printf(
			"Error recalling messages: %v\n",
			res.Error,
		)
		return "", res.Error
	}

	resultText := res.getTextContent()

	if resultText == "none" {
		return "", nil
	}

	return resultText, nil
}

// requests the memory server to recall the messages
func (host ToolsHost) GetRAGContext(ctx context.Context, prompt string) ([]string, error) {
	if host.ragServerName == "" {
		return []string{}, nil
	}

	// call the memory server to recall the messages
	res := host.callTool(
		host.ragServerName,
		ragToolName,
		map[string]interface{}{
			"query": prompt,
			"num":   3,
		},
		ctx,
	)
	if res.Error != nil {
		host.logger.Printf(
			"Error calling RAG server: %v\n",
			res.Error,
		)
		return []string{}, res.Error
	}
	resultText := res.getTextContent()

	if resultText == "none" {
		return []string{}, nil
	}

	// split the result for chunks, empty line is a separator
	results := []string{}
	paragraphs := strings.Split(resultText, "\n\n")
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p != "" {
			results = append(results, p)
		}
	}

	return results, nil
}
