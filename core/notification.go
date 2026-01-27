package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	notificationSubAgentSystemInstructions = "You are the agent responsible for handling notifications from different plugins. " +
		"You will receive a prompt with instructions on what to do with the notification. " +
		"Based on these instructions, you will decide whether to process the notification or ignore it. " +
		"If you decide to process it, extract relevant information and take appropriate actions as per the instructions. " +
		"If you decide to ignore it, just do nothing. " +
		"You are free to call any accessible tools to complete the task. " +
		"IMPORTANT: When the user's instructions say 'tell me', 'report', 'summarize', 'notify me', or 'let me know', " +
		"you MUST use the notification_feedback tool to communicate that information to the user."

	notificationSubAgentFeedbackToolDescription = "Send a message to the user about this notification. " +
		"Use this tool when the user's instructions ask you to tell, report, summarize, or inform them about something. " +
		"The user will see the message you provide. This is the ONLY way to communicate with the user."
)

// MonitoringStatus indicates whether a notification is being monitored for processing
type MonitoringStatus string

const (
	// MonitoringStatusNotMonitored means the notification is not being tracked
	MonitoringStatusNotMonitored MonitoringStatus = ""
	// MonitoringStatusMonitored means the notification is being tracked for processing
	MonitoringStatusMonitored MonitoringStatus = "monitored"
)

// ProcessingStatus indicates the processing state of a monitored notification
type ProcessingStatus string

const (
	// ProcessingStatusNone means no processing status (not monitored)
	ProcessingStatusNone ProcessingStatus = ""
	// ProcessingStatusPending means the notification is queued for processing
	ProcessingStatusPending ProcessingStatus = "pending"
	// ProcessingStatusProcessing means the notification is currently being processed
	ProcessingStatusProcessing ProcessingStatus = "processing"
	// ProcessingStatusProcessed means the notification has been processed
	ProcessingStatusProcessed ProcessingStatus = "processed"
	// ProcessingStatusFailed means the notification processing failed
	ProcessingStatusFailed ProcessingStatus = "failed"
)

// Notification represents a unified notification structure
// independent of the underlying protocol (MCP, A2A, etc.)
type Notification struct {
	// ServerName is the name of the server that sent the notification
	ServerName string `json:"server_name"`
	// Method is the notification method/type (e.g., "notifications/progress", "task/started")
	Method string `json:"method"`
	// Description is a human-readable short description of the notification
	Description string `json:"description"`
	// MonitoringStatus indicates whether this notification is being monitored
	MonitoringStatus MonitoringStatus `json:"monitoring_status"`
	// ProcessingStatus indicates the processing state if monitored
	ProcessingStatus ProcessingStatus `json:"processing_status"`
	// Params contains additional parameters from the notification
	Params map[string]interface{} `json:"params,omitempty"`
	// Timestamp is when the notification was received
	Timestamp time.Time `json:"timestamp"`
}

// NotificationCallback is the callback function type for receiving notifications
type NotificationCallback func(notification Notification)

// NewNotification creates a new Notification with default values
func NewNotification(serverName, method string, params map[string]interface{}) Notification {
	return Notification{
		ServerName:       serverName,
		Method:           method,
		Description:      generateDescription(method, params),
		MonitoringStatus: MonitoringStatusNotMonitored,
		ProcessingStatus: ProcessingStatusNone,
		Params:           params,
		Timestamp:        time.Now(),
	}
}

// NewNotificationFromMCP creates a Notification from an MCP JSONRPCNotification
func NewNotificationFromMCP(serverName string, mcpNotification mcp.JSONRPCNotification) Notification {
	params := make(map[string]interface{})
	if mcpNotification.Params.AdditionalFields != nil {
		params = mcpNotification.Params.AdditionalFields
	}

	return Notification{
		ServerName:       serverName,
		Method:           mcpNotification.Method,
		Description:      generateDescription(mcpNotification.Method, params),
		MonitoringStatus: MonitoringStatusNotMonitored,
		ProcessingStatus: ProcessingStatusNone,
		Params:           params,
		Timestamp:        time.Now(),
	}
}

// SetMonitored marks the notification as monitored with pending status
func (n *Notification) SetMonitored() {
	n.MonitoringStatus = MonitoringStatusMonitored
	n.ProcessingStatus = ProcessingStatusPending
}

// SetProcessing marks the notification as currently being processed
func (n *Notification) SetProcessing() {
	if n.MonitoringStatus == MonitoringStatusMonitored {
		n.ProcessingStatus = ProcessingStatusProcessing
	}
}

// SetProcessed marks the notification as processed
func (n *Notification) SetProcessed() {
	if n.MonitoringStatus == MonitoringStatusMonitored {
		n.ProcessingStatus = ProcessingStatusProcessed
	}
}

// SetFailed marks the notification processing as failed
func (n *Notification) SetFailed() {
	if n.MonitoringStatus == MonitoringStatusMonitored {
		n.ProcessingStatus = ProcessingStatusFailed
	}
}

// IsMonitored returns true if the notification is being monitored
func (n *Notification) IsMonitored() bool {
	return n.MonitoringStatus == MonitoringStatusMonitored
}

// GetParamString returns a parameter value as a string
func (n *Notification) GetParamString(key string) string {
	if val, ok := n.Params[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
		return fmt.Sprintf("%v", val)
	}
	return ""
}

// GetParamInt returns a parameter value as an integer
func (n *Notification) GetParamInt(key string) int {
	if val, ok := n.Params[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

// GetParamFloat returns a parameter value as a float64
func (n *Notification) GetParamFloat(key string) float64 {
	if val, ok := n.Params[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0
}

// generateDescription creates a human-readable description from method and params
func generateDescription(method string, params map[string]interface{}) string {
	// Try to extract title or description from params first
	if title, ok := params["title"].(string); ok && title != "" {
		return title
	}
	if desc, ok := params["description"].(string); ok && desc != "" {
		return desc
	}
	if msg, ok := params["message"].(string); ok && msg != "" {
		return msg
	}

	// Generate description based on known notification methods
	switch {
	case strings.HasSuffix(method, "/progress"):
		// Progress notification
		if progress, ok := params["progress"].(float64); ok {
			if total, ok := params["total"].(float64); ok && total > 0 {
				return fmt.Sprintf("Progress: %.0f%%", (progress/total)*100)
			}
			return fmt.Sprintf("Progress: %.0f%%", progress)
		}
		return "Progress update"

	case strings.HasSuffix(method, "/started"):
		return "Task started"

	case strings.HasSuffix(method, "/completed"):
		return "Task completed"

	case strings.HasSuffix(method, "/cancelled"):
		return "Task cancelled"

	case strings.HasSuffix(method, "/error"):
		if errMsg, ok := params["error"].(string); ok {
			return fmt.Sprintf("Error: %s", errMsg)
		}
		return "Error occurred"

	case strings.Contains(method, "resources/"):
		return "Resource update"

	case strings.Contains(method, "tools/"):
		return "Tool update"

	case strings.Contains(method, "prompts/"):
		return "Prompt update"

	default:
		// Use the method name as description if nothing else
		parts := strings.Split(method, "/")
		if len(parts) > 0 {
			return strings.Title(strings.ReplaceAll(parts[len(parts)-1], "_", " "))
		}
		return method
	}
}

// FormatForDisplay returns a formatted string for display purposes
func (n *Notification) FormatForDisplay() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] ", n.ServerName))
	sb.WriteString(n.Method)
	if n.Description != "" && n.Description != n.Method {
		sb.WriteString(fmt.Sprintf(" - %s", n.Description))
	}
	if n.MonitoringStatus == MonitoringStatusMonitored {
		sb.WriteString(fmt.Sprintf(" [%s]", n.ProcessingStatus))
	}
	return sb.String()
}

// notificationWithInstructions bundles a notification with its processing instructions
type notificationWithInstructions struct {
	notification Notification
	instructions []string
}

// NotificationProcessor handles notifications using a single persistent agent with a queue
type NotificationProcessor struct {
	agent   *CleverChatty
	queue   chan notificationWithInstructions
	logger  *log.Logger
	wg      sync.WaitGroup
	stopped bool
	mu      sync.Mutex
}

// NewNotificationProcessor creates a new notification processor
// parentConfig is used as base config for the processing agent
func NewNotificationProcessor(parentConfig CleverChattyConfig, ctx context.Context, logger *log.Logger, clientAgentID string) (*NotificationProcessor, error) {
	// Create agent with notification-specific system instructions
	config := parentConfig
	config.SystemInstruction = notificationSubAgentSystemInstructions

	agent, err := GetCleverChattyWithLogger(config, ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create notification processor agent: %w", err)
	}

	agent.ClientAgentID = clientAgentID
	agent.processNotifications = false // Prevent notification chains

	// Initialize the agent
	if err := agent.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize notification processor agent: %w", err)
	}

	// Register the feedback tool
	err = agent.SetTool(CustomTool{
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
			logger.Printf("Notification feedback to user: %s", message)
			return "Message delivered to user", nil
		},
	})
	if err != nil {
		agent.Finish()
		return nil, fmt.Errorf("failed to register feedback tool: %w", err)
	}

	processor := &NotificationProcessor{
		agent:  agent,
		queue:  make(chan notificationWithInstructions, 100), // Buffer up to 100 notifications
		logger: logger,
	}

	return processor, nil
}

// Start begins processing notifications from the queue
func (p *NotificationProcessor) Start() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.logger.Printf("Notification processor started")

		for item := range p.queue {
			p.process(item)
		}

		p.logger.Printf("Notification processor stopped")
	}()
}

// Stop gracefully shuts down the processor
func (p *NotificationProcessor) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.mu.Unlock()

	p.logger.Printf("Stopping notification processor, closing queue...")
	close(p.queue)
	p.wg.Wait()

	p.logger.Printf("Finishing notification processor agent...")
	if err := p.agent.Finish(); err != nil {
		p.logger.Printf("Error finishing notification processor agent: %v", err)
	}
}

// Enqueue adds a notification to the processing queue
// Returns false if the processor is stopped or queue is full
func (p *NotificationProcessor) Enqueue(notification Notification, instructions []string) bool {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		p.logger.Printf("Notification processor is stopped, dropping notification: %s", notification.Method)
		return false
	}
	p.mu.Unlock()

	select {
	case p.queue <- notificationWithInstructions{notification: notification, instructions: instructions}:
		p.logger.Printf("Notification enqueued: server=%s, method=%s", notification.ServerName, notification.Method)
		return true
	default:
		p.logger.Printf("Notification queue full, dropping notification: %s", notification.Method)
		return false
	}
}

// QueueLength returns the current number of notifications waiting in the queue
func (p *NotificationProcessor) QueueLength() int {
	return len(p.queue)
}

// process handles a single notification
func (p *NotificationProcessor) process(item notificationWithInstructions) {
	notification := item.notification
	instructions := item.instructions

	p.logger.Printf("Processing notification: server=%s, method=%s", notification.ServerName, notification.Method)

	// Serialize notification to JSON for the prompt
	notificationJSON, err := json.Marshal(notification)
	if err != nil {
		p.logger.Printf("Error serializing notification to JSON: %v", err)
		return
	}

	// Build the prompt with instructions and notification content
	instructionsText := strings.Join(instructions, "\n")
	prompt := fmt.Sprintf("Instructions from the user:\n%s\n\nNotification content:\n%s", instructionsText, string(notificationJSON))

	p.logger.Printf("Notification prompt: %s", prompt)

	// Prompt the agent
	response, err := p.agent.Prompt(prompt)
	if err != nil {
		p.logger.Printf("Error processing notification: %v", err)
		return
	}

	p.logger.Printf("Notification LLM response: %s", response)
	p.logger.Printf("Notification processed successfully: server=%s, method=%s", notification.ServerName, notification.Method)
}
