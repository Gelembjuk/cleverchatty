package test

import (
	"github.com/gelembjuk/cleverchatty/llm"
)

type MockMessage struct {
	role           string
	content        string
	toolCalls      []MockToolCall
	toolResponseID string
	usage          struct {
		input  int
		output int
	}
}

// GetRole returns the role of the MockMessage sender (e.g., "user", "assistant", "system")
func (m MockMessage) GetRole() string {
	return m.role
}

// GetContent returns the text content of the MockMessage
func (m MockMessage) GetContent() string {
	return m.content
}

// GetToolCalls returns any tool calls made in this MockMessage
func (m MockMessage) GetToolCalls() []llm.ToolCall {
	// Convert MockToolCall to llm.ToolCall
	toolCalls := make([]llm.ToolCall, len(m.toolCalls))
	for i, toolCall := range m.toolCalls {
		toolCalls[i] = toolCall
	}
	return toolCalls
}

// IsToolResponse returns true if this MockMessage is a response from a tool
func (m MockMessage) IsToolResponse() bool {
	return m.toolResponseID != ""
}

// GetToolResponseID returns the ID of the tool call this MockMessage is responding to
func (m MockMessage) GetToolResponseID() string {
	return m.toolResponseID
}

// GetUsage returns token usage statistics if available
func (m MockMessage) GetUsage() (input int, output int) {
	return m.usage.input, m.usage.output
}
