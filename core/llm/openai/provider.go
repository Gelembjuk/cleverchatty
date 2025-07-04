package openai

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

func convertSchema(schema llm.Schema) map[string]interface{} {
	// Ensure required is a valid array, defaulting to empty if nil
	required := schema.Required
	if required == nil {
		required = []string{}
	}

	return map[string]interface{}{
		"type":       schema.Type,
		"properties": schema.Properties,
		"required":   required,
	}
}

func NewProvider(apiKey string, baseURL string, model string) *Provider {
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
	p.logger.Printf("creating message for OpenAI provider with prompt: %s, num_messages: %d, num_tools: %d\n",
		prompt,
		len(messages),
		len(tools))

	openaiMessages := make([]MessageParam, 0, len(messages))

	// Convert previous messages
	for _, msg := range messages {
		p.logger.Printf("converting message for OpenAI provider with role: %s, content: %s, is_tool_response: %t\n",
			msg.GetRole(),
			msg.GetContent(),
			msg.IsToolResponse())

		param := MessageParam{
			Role: msg.GetRole(),
		}

		if msg.GetContent() != "" {
			content := msg.GetContent()
			param.Content = &content
		}

		// Handle function/tool calls
		toolCalls := msg.GetToolCalls()
		if len(toolCalls) > 0 {
			param.Content = nil // Must be null for function calls

			// Convert to OpenAI tool calls format
			param.ToolCalls = make([]ToolCall, len(toolCalls))
			for i, call := range toolCalls {
				args, err := json.Marshal(call.GetArguments())
				if err != nil {
					return nil, fmt.Errorf(
						"error marshaling function arguments: %w",
						err,
					)
				}

				param.ToolCalls[i] = ToolCall{
					ID:   call.GetID(),
					Type: "function",
					Function: FunctionCall{
						Name:      call.GetName(),
						Arguments: string(args),
					},
				}
			}
		}

		// Handle function/tool responses
		if msg.IsToolResponse() {
			p.logger.Printf("processing tool response for OpenAI provider with tool_call_id: %s, raw_message: %v\n",
				msg.GetToolResponseID(),
				msg)

			// Extract content from tool response
			var contentStr string
			if content := msg.GetContent(); content != "" {
				contentStr = content
			} else {
				// Try to extract text from history message content blocks
				if historyMsg, ok := msg.(*history.HistoryMessage); ok {
					var texts []string
					for _, block := range historyMsg.Content {
						if block.Type == "tool_result" {
							if block.Text != "" {
								texts = append(texts, block.Text)
							} else if contentArray, ok := block.Content.([]interface{}); ok {
								for _, item := range contentArray {
									if contentMap, ok := item.(map[string]interface{}); ok {
										if text, ok := contentMap["text"]; ok {
											texts = append(texts, fmt.Sprint(text))
										}
									}
								}
							}
						}
					}
					contentStr = strings.Join(texts, "\n")
				}
			}

			if contentStr == "" {
				contentStr = "No content returned from function"
			}

			param.Content = &contentStr
			param.Role = "tool" // Use tool role instead of function
			param.ToolCallID = msg.GetToolResponseID()
			// Don't set name field for tool responses
		}

		openaiMessages = append(openaiMessages, param)
	}

	// Log the final message array
	p.logger.Printf("sending messages to OpenAI provider: %v , tools %d\n",
		openaiMessages,
		len(tools))

	// Add the new prompt if provided
	if prompt != "" {
		content := prompt
		openaiMessages = append(openaiMessages, MessageParam{
			Role:    "user",
			Content: &content,
		})
	}

	// Convert tools to OpenAI format
	openaiTools := make([]Tool, len(tools))
	for i, tool := range tools {
		openaiTools[i] = Tool{
			Type: "function",
			Function: FunctionDef{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  convertSchema(tool.InputSchema),
			},
		}
	}

	// Make the API call
	resp, err := p.client.CreateChatCompletion(ctx, CreateRequest{
		Model:       p.model,
		Messages:    openaiMessages,
		Tools:       openaiTools,
		MaxTokens:   4096,
		Temperature: 0.7,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &Message{Resp: resp, Choice: &resp.Choices[0]}, nil
}

func (p *Provider) SupportsTools() bool {
	return true
}

func (p *Provider) Name() string {
	return "openai"
}

func (p *Provider) SetLogger(logger *log.Logger) {
	p.logger = logger
}

func (p *Provider) CreateToolResponse(
	toolCallID string,
	content interface{},
) (llm.Message, error) {
	p.logger.Printf("creating tool response for OpenAI provider with tool_call_id: %s, content_type: %T, content: %v\n",
		toolCallID,
		fmt.Sprintf("%T", content),
		content)

	// Convert content to string representation
	var contentStr string
	switch v := content.(type) {
	case string:
		contentStr = v
	case []interface{}:
		// Handle array of content blocks
		var texts []string
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				// First try to get text directly
				if text, ok := block["text"].(string); ok {
					texts = append(texts, text)
					continue
				}

				// Then try array of text
				if textArray, ok := block["text"].([]interface{}); ok {
					for _, t := range textArray {
						if str, ok := t.(string); ok {
							texts = append(texts, str)
						}
					}
					continue
				}

				// Finally try nested content array
				if contentArray, ok := block["content"].([]interface{}); ok {
					for _, c := range contentArray {
						if cMap, ok := c.(map[string]interface{}); ok {
							if text, ok := cMap["text"].(string); ok {
								texts = append(texts, text)
							}
						}
					}
				}
			}
		}
		contentStr = strings.Join(texts, "\n")
		if contentStr == "" {
			// Fallback to JSON if no text found
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal array content: %w", err)
			}
			contentStr = string(jsonBytes)
		}
	default:
		// For other types, marshal to JSON
		jsonBytes, err := json.Marshal(content)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool response: %w", err)
		}
		contentStr = string(jsonBytes)
	}

	if contentStr == "" {
		contentStr = "No content returned from tool"
	}

	// Create a new message with the tool response
	msg := &Message{
		Choice: &Choice{
			Message: MessageParam{
				Role:       "tool",
				Content:    &contentStr,
				ToolCallID: toolCallID,
			},
		},
	}

	// Also set the response field
	msg.Resp = &APIResponse{
		Choices: []Choice{*msg.Choice},
	}

	return msg, nil
}

// Message implements the llm.Message interface
type Message struct {
	Resp   *APIResponse
	Choice *Choice
}

func (m *Message) GetRole() string {
	return m.Choice.Message.Role
}

func (m *Message) GetContent() string {
	if m.Choice.Message.Content == nil {
		return ""
	}
	return *m.Choice.Message.Content
}

func (m *Message) GetToolCalls() []llm.ToolCall {
	var calls []llm.ToolCall
	for _, call := range m.Choice.Message.ToolCalls {
		calls = append(calls, &ToolCallWrapper{call})
	}
	return calls
}

func (m *Message) IsToolResponse() bool {
	return m.Choice.Message.ToolCallID != ""
}

func (m *Message) GetToolResponseID() string {
	return m.Choice.Message.ToolCallID
}

func (m *Message) GetUsage() (int, int) {
	return m.Resp.Usage.PromptTokens, m.Resp.Usage.CompletionTokens
}

// ToolCallWrapper implements llm.ToolCall
type ToolCallWrapper struct {
	Call ToolCall
}

func (t *ToolCallWrapper) GetID() string {
	return t.Call.ID
}

func (t *ToolCallWrapper) GetName() string {
	return t.Call.Function.Name
}

func (t *ToolCallWrapper) GetArguments() map[string]interface{} {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(t.Call.Function.Arguments), &args); err != nil {
		return make(map[string]interface{})
	}
	return args
}
