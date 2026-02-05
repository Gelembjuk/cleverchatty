package core

import "fmt"

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
