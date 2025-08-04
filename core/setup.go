package core

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/gelembjuk/cleverchatty/core/history"
	"github.com/gelembjuk/cleverchatty/core/llm"
	"github.com/gelembjuk/cleverchatty/core/llm/anthropic"
	"github.com/gelembjuk/cleverchatty/core/llm/google"
	"github.com/gelembjuk/cleverchatty/core/llm/ollama"
	"github.com/gelembjuk/cleverchatty/core/llm/openai"
	"github.com/gelembjuk/cleverchatty/core/test"
)

type CleverChatty struct {
	context       context.Context
	ClientAgentID string
	config        CleverChattyConfig
	logger        *log.Logger
	provider      llm.Provider
	toolsHost     *ToolsHost
	messages      []history.HistoryMessage
	Callbacks     UICallbacks
}

func GetCleverChatty(config CleverChattyConfig, ctx context.Context) (*CleverChatty, error) {
	logger, err := InitLogger(config.LogFilePath, config.DebugMode)

	if err != nil {
		return nil, fmt.Errorf("error initializing logger: %v", err)
	}
	return GetCleverChattyWithLogger(config, ctx, logger)
}

func GetCleverChattyWithLogger(config CleverChattyConfig, ctx context.Context, logger *log.Logger) (*CleverChatty, error) {
	if config.MessageWindow <= 0 {
		config.MessageWindow = defaultMessagesWindow
	}
	assistant := &CleverChatty{
		config: config,
	}

	assistant.context = ctx

	assistant.logger = logger

	assistant.messages = make([]history.HistoryMessage, 0)

	assistant.Callbacks = UICallbacks{}

	return assistant, nil
}

func (assistant *CleverChatty) Init() error {
	var err error
	assistant.provider, err = assistant.createProvider(assistant.context, assistant.config.Model)

	if err != nil {
		return fmt.Errorf("error creating provider: %v", err)
	}

	assistant.toolsHost, err = NewToolsHost(assistant.config.ToolsServers, assistant.logger, assistant.context)

	if err != nil {
		return fmt.Errorf("error creating MCP host: %v", err)
	}

	assistant.toolsHost.clientAgentID = assistant.ClientAgentID
	assistant.toolsHost.AgentID = assistant.config.AgentID
	assistant.toolsHost.AgentName = assistant.config.A2AServerConfig.Title

	err = assistant.toolsHost.Init()

	if err != nil {
		return fmt.Errorf("error initializing MCP host: %v", err)
	}

	return nil
}

func (assistant *CleverChatty) WithLogger(logger *log.Logger) {
	assistant.logger = logger
}

func (assistant *CleverChatty) WithCallbacks(callbacks UICallbacks) {
	assistant.Callbacks = callbacks
}

func (assistant *CleverChatty) WithClientAgentID(agentID string) {
	assistant.ClientAgentID = agentID
}

func (assistant *CleverChatty) WithAgentID(agentID string) {
	assistant.config.AgentID = agentID
}

// SetReverseMCPClient sets the reverse MCP client for dynamic tool registration
func (assistant *CleverChatty) SetReverseMCPClient(client ReverseMCPClient) {
	if assistant.toolsHost != nil {
		assistant.toolsHost.SetReverseMCPClient(client)
	}
}

// Add new function to create provider
func (assistant CleverChatty) createProvider(ctx context.Context, modelString string) (llm.Provider, error) {
	parts := strings.SplitN(modelString, ":", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf(
			"invalid model format. Expected provider:model, got %s",
			modelString,
		)
	}

	provider := parts[0]
	model := parts[1]

	switch provider {
	case "anthropic":
		apiKey := assistant.config.Anthropic.APIKey

		if apiKey == "" {
			return nil, fmt.Errorf(
				"anthropic API key not provided. Use --anthropic-api-key flag or ANTHROPIC_API_KEY environment variable",
			)
		}
		return anthropic.NewProvider(apiKey, assistant.config.Anthropic.BaseURL, model), nil

	case "ollama":
		return ollama.NewProvider(model)

	case "openai":
		apiKey := assistant.config.OpenAI.APIKey

		if apiKey == "" {
			return nil, fmt.Errorf(
				"OpenAI API key not provided. Use --openai-api-key flag or OPENAI_API_KEY environment variable",
			)
		}
		return openai.NewProvider(apiKey, assistant.config.OpenAI.BaseURL, model), nil

	case "google":
		apiKey := assistant.config.Google.APIKey

		return google.NewProvider(ctx, apiKey, model)

	case "mock":
		return test.MockProvider{}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func (assistant *CleverChatty) Finish() error {
	err := assistant.toolsHost.Close()
	if err != nil {
		return fmt.Errorf(
			"error closing client %v",
			err,
		)
	} else {
		log.Printf("All clients closed")
	}
	return nil
}

func (assistant *CleverChatty) GetServersInfo() []ServerInfo {
	return assistant.toolsHost.getServersInfo()
}

func (assistant *CleverChatty) GetToolsInfo() []ServerInfo {
	return assistant.toolsHost.getToolsInfo()
}

func (assistant *CleverChatty) GetMessages() []history.HistoryMessage {
	return assistant.messages
}
