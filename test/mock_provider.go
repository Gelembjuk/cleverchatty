package test

import (
	"context"
	"strconv"
	"strings"

	"github.com/mark3labs/mcphost/pkg/history"
	"github.com/mark3labs/mcphost/pkg/llm"
)

type MockProvider struct {
}

func (p MockProvider) CreateMessage(ctx context.Context, prompt string, messages []llm.Message, tools []llm.Tool) (llm.Message, error) {
	// Simulate a message creation process
	// This is just a placeholder implementation
	// if prompt starts with "tool:N:..." then it is a tool call simulated. N is an index of the tool (-1. 1 goes to 0 etc)

	if strings.HasPrefix(prompt, "tool:") {
		parts := strings.SplitN(prompt, ":", 3)
		if len(parts) != 3 {
			return nil, nil
		}
		// convert to int
		toolIndex, err := strconv.Atoi(parts[1])

		if err != nil {
			return nil, err
		}
		toolIndex = toolIndex - 1 // convert to 0 based index
		if toolIndex < 0 || toolIndex >= len(tools) {
			return nil, nil
		}
		tool := tools[toolIndex]
		toolCall := MockToolCall{
			Name:      tool.Name,
			Arguments: map[string]interface{}{"argument": parts[2]},
			ID:        "tool_call_id",
		}

		return &MockMessage{
			role:           "assistant",
			content:        "",
			toolCalls:      []MockToolCall{toolCall},
			toolResponseID: "",
		}, nil
	} else if prompt == "" && len(messages) > 0 {
		hM := messages[len(messages)-1].(*history.HistoryMessage)
		return &MockMessage{
			role:           "assistant",
			content:        "FAKE_ANALYSED_RESPONSE:" + hM.Content[0].Text,
			toolCalls:      []MockToolCall{},
			toolResponseID: "",
		}, nil
	}
	responseMessage := ""

	if prompt != "" {
		responseMessage = "FAKE_RESPONSE:" + prompt
	}
	return &MockMessage{
		role:           "assistant",
		content:        responseMessage,
		toolCalls:      []MockToolCall{},
		toolResponseID: "",
	}, nil
}

// CreateToolResponse creates a message representing a tool response
func (p MockProvider) CreateToolResponse(toolCallID string, content interface{}) (llm.Message, error) {
	// Simulate creating a tool response
	// This is just a placeholder implementation
	return &MockMessage{}, nil
}

// SupportsTools returns whether this provider supports tool/function calling
func (p MockProvider) SupportsTools() bool {
	// Simulate checking for tool support
	// This is just a placeholder implementation
	return true
}

// Name returns the provider's name
func (p MockProvider) Name() string {
	return "MockProvider"
}
