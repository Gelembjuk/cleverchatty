package core

import (
	"fmt"
)

// NewCustomTool creates a new CustomTool and validates it.
// It enforces that a handler callback is provided.
func NewCustomTool(name, description string, args []ToolArgument, handler CustomToolHandler) (CustomTool, error) {
	tool := CustomTool{
		Name:        name,
		Description: description,
		Arguments:   args,
		Handler:     handler,
	}

	if err := tool.Validate(); err != nil {
		return CustomTool{}, fmt.Errorf("invalid custom tool: %w", err)
	}

	return tool, nil
}

const (
	notificationSubAgentFeedbackToolDescription = "Send a message to the user about this notification. " +
		"Use this tool when the user's instructions ask you to tell, report, summarize, or inform them about something. " +
		"The user will see the message you provide. This is the ONLY way to communicate with the user."
)

// NewNotificationFeedbackTool creates a new notification feedback tool.
func NewNotificationFeedbackTool(handler CustomToolHandler) (CustomTool, error) {
	return NewCustomTool(
		"notification_feedback",
		notificationSubAgentFeedbackToolDescription,
		[]ToolArgument{
			{
				Name:        "message",
				Type:        "string",
				Description: "The message to provide to the user. It can be Markdown formatted.",
				Required:    true,
			},
		},
		handler,
	)
}

// NewGetNotificationTypesTool creates a tool that lists all available notification types.
// Call this tool first to discover what notification types are available before subscribing.
func NewGetNotificationTypesTool(handler CustomToolHandler) (CustomTool, error) {
	return NewCustomTool(
		"get_notification_types",
		"Get the list of all available notification types from connected servers. "+
			"Call this tool first to discover what notification types are available before subscribing.",
		[]ToolArgument{},
		handler,
	)
}

// NewSubscribeNotificationsTool creates a tool that subscribes to a specific notification type.
// The LLM provides a composite key (from get_notification_types) and processing instructions.
func NewSubscribeNotificationsTool(handler CustomToolHandler) (CustomTool, error) {
	return NewCustomTool(
		"subscribe_notifications",
		"Subscribe to receive notifications of a specific type. "+
			"Use the composite key returned by get_notification_types to identify the notification type. "+
			"When a matching notification arrives, it will be processed according to the instructions you provide.",
		[]ToolArgument{
			{
				Name:        "notification_key",
				Type:        "string",
				Description: "The composite key identifying the notification type, as returned by get_notification_types (e.g., 'email-server::notifications/message').",
				Required:    true,
			},
			{
				Name:        "instructions",
				Type:        "string",
				Description: "Instructions for how to process this notification when it arrives.",
				Required:    true,
			},
		},
		handler,
	)
}
