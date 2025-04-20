package cleverchatty

import (
	"context"
	"fmt"
	"log"
	"time"

	"strings"

	"github.com/gelembjuk/cleverchatty/test"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcphost/pkg/llm"
)

type MCPHost struct {
	config  map[string]ServerConfigWrapper
	logger  *log.Logger
	clients map[string]mcpclient.MCPClient
	tools   []llm.Tool
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

// Create MCP servers instances
func (host *MCPHost) createMCPClients() error {
	clients := make(map[string]mcpclient.MCPClient)

	for name, server := range host.config {
		var client mcpclient.MCPClient
		var err error

		if server.Config.GetType() == transportSSE {
			sseConfig := server.Config.(SSEServerConfig)

			options := []mcpclient.ClientOption{}

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
				options = append(options, mcpclient.WithHeaders(headers))
			}

			client, err = mcpclient.NewSSEMCPClient(
				sseConfig.Url,
				options...,
			)
			if err == nil {
				err = client.(*mcpclient.SSEMCPClient).Start(context.Background())
			}
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

		serverTools := host.mcpToolsToAnthropicTools(serverName, toolsResult.Tools)
		allTools = append(allTools, serverTools...)
		host.logger.Printf(
			"Tools loaded from server %s: %d tools\n",
			serverName,
			len(toolsResult.Tools),
		)
	}
	host.tools = allTools
	return nil
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
