package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	notificationSubAgentSystemInstructions = "You are the agent responsible for handling notifications from different plugins." +
		"You will receive a prompt which is the set of instructions what to do or not to do with the notification." +
		"The prompt is a request from a user on how to handle the notification." +
		"Based on these instructions, you will decide whether to process the notification or ignore it." +
		" If you decide to process it, you will extract relevant information and take appropriate actions as per the instructions." +
		" If you decide to ignore it, just do nothing." +
		" You are free to call any accessible tools to make a final decision." +
		" If you deside to provide some feedback on this notification, use the notification_feedback tool. " +
		" Only use the notification_feedback tool to communicate important information to the user, if. auser asked to report or there is important event."

	notificationSubAgentFeedbackToolDescription = "Provide a message to the user in response to a notification. " +
		"Use this tool to communicate important information to the user only if you think the user should be informed about something." +
		"Call this to say or to tell or to show something to the user. A user will see the message you provide."
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
