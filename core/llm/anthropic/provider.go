package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/gelembjuk/cleverchatty/core/history"
	"github.com/gelembjuk/cleverchatty/core/llm"
)

type Provider struct {
	client *Client
	model  string
	logger *log.Logger
}

func NewProvider(apiKey string, baseURL string, model string) *Provider {
	if model == "" {
		model = "claude-3-5-sonnet-20240620" // 默认模型
	}
	return &Provider{
		client: NewClient(apiKey, baseURL),
		model:  model,
		logger: log.New(io.Discard, "", log.LstdFlags),
	}
}

func (p *Provider) CreateMessage(
	ctx context.Context,
	prompt string,
	messages []llm.Message,
	tools []llm.Tool,
) (llm.Message, error) {
	p.logger.Printf("creating message for Anthropic provider with prompt: %s, num_messages: %d, num_tools: %d\n",
		prompt,
		len(messages),
		len(tools))

	anthropicMessages := make([]MessageParam, 0, len(messages))

	for _, msg := range messages {
		p.logger.Printf("converting message for Anthropic provider with role: %s, content: %s, is_tool_response: %t\n",
			msg.GetRole(),
			msg.GetContent(),
			msg.IsToolResponse())

		content := []ContentBlock{}

		// Add regular text content if present
		if textContent := strings.TrimSpace(msg.GetContent()); textContent != "" {
			content = append(content, ContentBlock{
				Type: "text",
				Text: textContent,
			})
		}

		// Add tool calls if present
		for _, call := range msg.GetToolCalls() {
			input, _ := json.Marshal(call.GetArguments())
			content = append(content, ContentBlock{
				Type:  "tool_use",
				ID:    call.GetID(),
				Name:  call.GetName(),
				Input: input,
			})
		}

		// Handle tool responses
		if msg.IsToolResponse() {
			p.logger.Printf("processing tool response for message: %s, tool_call_id: %s\n",
				msg.GetToolResponseID(),
				msg)

			if historyMsg, ok := msg.(*history.HistoryMessage); ok {
				for _, block := range historyMsg.Content {
					if block.Type == "tool_result" {
						content = append(content, ContentBlock{
							Type:      "tool_result",
							ToolUseID: block.ToolUseID,
							Content:   block.Content,
						})
					}
				}
			} else {
				// Always include tool response content
				content = append(content, ContentBlock{
					Type:      "tool_result",
					ToolUseID: msg.GetToolResponseID(),
					Content:   msg.GetContent(),
				})
			}
		}

		// Always append the message, even if content is empty
		// This maintains conversation flow
		anthropicMessages = append(anthropicMessages, MessageParam{
			Role:    msg.GetRole(),
			Content: content,
		})
	}

	// Add the new prompt if provided
	if prompt != "" {
		anthropicMessages = append(anthropicMessages, MessageParam{
			Role: "user",
			Content: []ContentBlock{{
				Type: "text",
				Text: prompt,
			}},
		})
	}

	// Convert tools to Anthropic format
	anthropicTools := make([]Tool, len(tools))
	for i, tool := range tools {
		anthropicTools[i] = Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: InputSchema{
				Type:       tool.InputSchema.Type,
				Properties: tool.InputSchema.Properties,
				Required:   tool.InputSchema.Required,
			},
		}
	}

	p.logger.Printf("sending messages to Anthropic provider: %v, tools: %d\n",
		anthropicMessages,
		len(tools))

	// Make the API call
	resp, err := p.client.CreateMessage(ctx, CreateRequest{
		Model:     p.model,
		Messages:  anthropicMessages,
		MaxTokens: 4096,
		Tools:     anthropicTools,
	})
	if err != nil {
		return nil, err
	}

	return &Message{Msg: *resp}, nil
}

func (p *Provider) SupportsTools() bool {
	return true
}

func (p *Provider) Name() string {
	return "anthropic"
}

func (p *Provider) SetLogger(logger *log.Logger) {
	p.logger = logger
}

func (p *Provider) CreateToolResponse(
	toolCallID string,
	content interface{},
) (llm.Message, error) {
	p.logger.Printf("creating tool response for tool_call_id: %s, content_type: %T, content: %v\n",
		toolCallID,
		fmt.Sprintf("%T", content),
		content)

	var contentStr string
	var structuredContent interface{} = content

	// Convert content to string if needed
	switch v := content.(type) {
	case string:
		contentStr = v
	case []byte:
		contentStr = string(v)
	default:
		// For structured content, create JSON representation
		if jsonBytes, err := json.Marshal(content); err == nil {
			contentStr = string(jsonBytes)
		} else {
			contentStr = fmt.Sprintf("%v", content)
		}
	}

	msg := &Message{
		Msg: APIMessage{
			Role: "tool",
			Content: []ContentBlock{{
				Type:      "tool_result",
				ToolUseID: toolCallID,
				Content:   structuredContent, // Original structure
				Text:      contentStr,        // String representation
			}},
		},
	}

	return msg, nil
}
