package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/gelembjuk/cleverchatty/core/history"
	"github.com/gelembjuk/cleverchatty/core/llm"
	"github.com/gelembjuk/cleverchatty/core/llm/anthropic"
	"github.com/gelembjuk/cleverchatty/core/llm/google"
	"github.com/gelembjuk/cleverchatty/core/llm/ollama"
	"github.com/gelembjuk/cleverchatty/core/llm/openai"
	"github.com/gelembjuk/cleverchatty/core/test"
)

type CleverChatty struct {
	context              context.Context
	ClientAgentID        string
	config               CleverChattyConfig
	logger               *log.Logger
	provider             llm.Provider
	toolsHost            *ToolsHost
	messages             []history.HistoryMessage
	Callbacks            UICallbacks
	subAgents            map[string]*CleverChatty
	subAgentsMu          sync.Mutex // Protects subAgents map for concurrent access
	processNotifications bool       // When false, notifications are ignored (used for subagents)
	onFinishCallback     func()     // Called when Finish() is invoked, used to notify parent
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

	assistant.subAgents = make(map[string]*CleverChatty)

	assistant.processNotifications = true // Enable notification processing by default

	assistant.Callbacks = UICallbacks{}

	return assistant, nil
}

func (assistant *CleverChatty) Init() error {
	var err error
	assistant.provider, err = assistant.createProvider(assistant.context, assistant.config.Model)

	if err != nil {
		return fmt.Errorf("error creating provider: %v", err)
	}

	assistant.toolsHost, err = newToolsHost(assistant.config.ToolsServers, assistant.logger, assistant.context)

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

// SetNotificationCallback sets a callback for notifications from all MCP servers.
// The callback receives a unified Notification structure instead of the raw MCP notification.
// If a notification is monitored and has instructions configured, a subagent will be
// automatically spawned to process it (unless processNotifications is false).
func (assistant *CleverChatty) SetNotificationCallback(callback NotificationCallback) {
	assistant.logger.Printf("SetNotificationCallback called, processNotifications=%v", assistant.processNotifications)

	// Create a wrapper callback that handles subagent processing for monitored notifications
	wrappedCallback := func(notification Notification) {
		assistant.logger.Printf("Notification wrapper received: server=%s, method=%s, monitored=%v",
			notification.ServerName, notification.Method, notification.IsMonitored())

		// Only process notifications with subagents if enabled (disabled for subagents to prevent chains)
		if assistant.processNotifications && notification.IsMonitored() {
			// Get the server config to retrieve instructions
			if serverConfig, ok := assistant.config.ToolsServers[notification.ServerName]; ok {
				if instructions := serverConfig.GetNotificationInstructions(notification.Method); instructions != nil && len(instructions) > 0 {
					// Process the notification with a subagent
					assistant.ProcessNotificationWithSubagent(notification, instructions)
				}
			}
		}

		// Always call the original callback
		if callback != nil {
			assistant.logger.Printf("Calling original notification callback for server=%s", notification.ServerName)
			callback(notification)
		} else {
			assistant.logger.Printf("Original notification callback is nil!")
		}
	}

	assistant.toolsHost.SetNotificationCallback(wrappedCallback)
}

// Get or create subagent with given alias
func (assistant *CleverChatty) getSubagent(alias string) (*CleverChatty, error) {
	subAgent, err := GetCleverChattyWithLogger(assistant.config, assistant.context, assistant.logger)
	if err != nil {
		return nil, err
	}

	subAgent.ClientAgentID = assistant.ClientAgentID
	subAgent.processNotifications = false // Disable notification processing for subagents

	if alias == "" {
		alias = generateRandomString(16)
	}

	assistant.subAgentsMu.Lock()
	assistant.subAgents[alias] = subAgent
	assistant.subAgentsMu.Unlock()

	// Set callback to remove subagent from parent's map when it finishes itself
	subAgent.onFinishCallback = func() {
		assistant.subAgentsMu.Lock()
		delete(assistant.subAgents, alias)
		assistant.subAgentsMu.Unlock()
		assistant.logger.Printf("Subagent %s removed from parent after self-finish", alias)
	}

	return subAgent, nil
}

// Get or create subagent with given alias and custom instruction
func (assistant *CleverChatty) getSubagentWithInstructions(alias string, instruction string) (*CleverChatty, error) {
	subAgent, err := assistant.getSubagent(alias)
	if err != nil {
		return nil, err
	}

	subAgent.config.SystemInstruction = instruction

	return subAgent, nil
}

// SetTool registers a custom tool with the assistant.
// The tool will be available to the LLM alongside MCP and A2A tools.
// Returns an error if the tool definition is invalid.
func (assistant *CleverChatty) SetTool(tool CustomTool) error {
	if assistant.toolsHost == nil {
		return fmt.Errorf("toolsHost not initialized, call Init() first")
	}
	return assistant.toolsHost.AddCustomTool(tool)
}

// RemoveTool removes a custom tool by name
func (assistant *CleverChatty) RemoveTool(name string) {
	if assistant.toolsHost != nil {
		assistant.toolsHost.RemoveCustomTool(name)
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
func (assistant *CleverChatty) finishSubagent(alias string) error {
	assistant.subAgentsMu.Lock()
	subAgent, exists := assistant.subAgents[alias]
	if !exists {
		assistant.subAgentsMu.Unlock()
		return fmt.Errorf("subagent with alias %s does not exist", alias)
	}

	// Clear callback since parent is explicitly removing the subagent
	subAgent.onFinishCallback = nil
	delete(assistant.subAgents, alias)
	assistant.subAgentsMu.Unlock()

	err := subAgent.Finish()
	if err != nil {
		return err
	}

	return nil
}

func (assistant *CleverChatty) Finish() error {
	assistant.subAgentsMu.Lock()
	assistant.logger.Printf("Finishing CleverChatty assistant with %d subagents", len(assistant.subAgents))

	// Collect subagents to finish (avoid holding lock during Finish calls)
	subAgentsToFinish := make(map[string]*CleverChatty)
	for alias, subAgent := range assistant.subAgents {
		// Clear the callback to prevent it from trying to modify the map
		subAgent.onFinishCallback = nil
		subAgentsToFinish[alias] = subAgent
	}
	assistant.subAgents = make(map[string]*CleverChatty)
	assistant.subAgentsMu.Unlock()

	// Finish all subagents outside the lock
	for alias, subAgent := range subAgentsToFinish {
		assistant.logger.Printf("Finishing subagent with alias: %s", alias)
		err := subAgent.Finish()
		if err != nil {
			assistant.logger.Printf("Error finishing subagent %s: %v", alias, err)
		}
	}

	err := assistant.toolsHost.Close()
	if err != nil {
		return fmt.Errorf(
			"error closing client %v",
			err,
		)
	}

	// Notify parent (if any) that this agent has finished
	if assistant.onFinishCallback != nil {
		assistant.onFinishCallback()
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

// ProcessNotificationWithSubagent handles a monitored notification by creating a subagent
// that processes the notification based on the provided instructions.
// This method runs in a goroutine and logs the result when complete.
func (assistant *CleverChatty) ProcessNotificationWithSubagent(notification Notification, instructions []string) {
	go func() {
		// Serialize notification to JSON for the prompt
		notificationJSON, err := json.Marshal(notification)
		if err != nil {
			assistant.logger.Printf("Error serializing notification to JSON: %v", err)
			return
		}

		// Build the prompt with instructions and notification content
		instructionsText := strings.Join(instructions, "\n")
		prompt := fmt.Sprintf("Instructions from the user:\n%s\n\nNotification content:\n%s", instructionsText, string(notificationJSON))

		// Create a subagent with notification-specific system instructions
		subAgent, err := assistant.getSubagentWithInstructions("", notificationSubAgentSystemInstructions)
		if err != nil {
			assistant.logger.Printf("Error creating subagent for notification processing: %v", err)
			return
		}

		// Initialize the subagent
		err = subAgent.Init()
		if err != nil {
			assistant.logger.Printf("Error initializing subagent for notification processing: %v", err)
			return
		}

		// Register a custom tool
		err = subAgent.SetTool(CustomTool{
			Name:        "notification_feedback",
			Description: notificationSubAgentFeedbackToolDescription,
			Arguments: []ToolArgument{
				{
					Name:        "message",
					Type:        "string",
					Description: "The message to provide to the user. It can be Markdown formatted.",
					Required:    true,
				},
			},
			Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
				message := args["message"].(string)
				// Your custom logic here
				assistant.logger.Printf("Message to user: %s", message)
				return "Okay", nil
			},
		})

		// Ensure cleanup when done
		defer func() {
			if finishErr := subAgent.Finish(); finishErr != nil {
				assistant.logger.Printf("Error finishing notification subagent: %v", finishErr)
			}
		}()

		// Prompt the subagent
		_, err = subAgent.Prompt(prompt)
		if err != nil {
			assistant.logger.Printf("Error processing notification with subagent: %v", err)
			return
		}
	}()
}
