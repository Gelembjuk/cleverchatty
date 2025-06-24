package history

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gelembjuk/cleverchatty/core/llm"
)

const (
	messageRoleUser            = "user"
	messageRoleAssistant       = "assistant"
	messageRoleSystem          = "system"
	messageSubroleMemory       = "memory"
	messageSubroleRAGContext   = "rag_context"
	messageSubrolePrompt       = "prompt"
	messageSubroleInstruction  = "instruction"
	messageSubroleToolResponse = "tool_response"
)

// HistoryMessage implements the llm.Message interface for stored messages
type HistoryMessage struct {
	Role    string         `json:"role"`
	SubRole string         `json:"sub_role,omitempty"`
	Content []ContentBlock `json:"content"`
}

func NewMemoryNoteMessage(content string) HistoryMessage {
	return HistoryMessage{
		Role:    messageRoleSystem,
		SubRole: messageSubroleMemory,
		Content: []ContentBlock{
			{
				Type: "text",
				Text: content,
			},
		},
	}
}

func NewRAGContextMessage(content string) HistoryMessage {
	return HistoryMessage{
		Role:    messageRoleUser,
		SubRole: messageSubroleRAGContext,
		Content: []ContentBlock{
			{
				Type: "text",
				Text: content,
			},
		},
	}
}

func NewUserPromptMessage(prompt string) HistoryMessage {
	return HistoryMessage{
		Role:    messageRoleUser,
		SubRole: messageSubrolePrompt,
		Content: []ContentBlock{
			{
				Type: "text",
				Text: prompt,
			},
		},
	}
}

func NewSystemInstructionMessage(instruction string) HistoryMessage {
	return HistoryMessage{
		Role:    messageRoleSystem,
		SubRole: messageSubroleInstruction,
		Content: []ContentBlock{
			{
				Type: "text",
				Text: instruction,
			},
		},
	}
}

func (m *HistoryMessage) GetRole() string {
	return m.Role
}

func (m HistoryMessage) GetSubRole() string {
	return m.SubRole
}

func (m HistoryMessage) IsAssistantResponse() bool {
	return m.Role == messageRoleAssistant
}

func (m HistoryMessage) IsMemoryNote() bool {
	return m.SubRole == messageSubroleMemory && m.Role == messageRoleSystem
}

func (m HistoryMessage) IsRAGContext() bool {
	return m.SubRole == messageSubroleRAGContext && m.Role == messageRoleUser
}

func (m HistoryMessage) IsSystemInstruction() bool {
	return m.SubRole == messageSubroleInstruction && m.Role == messageRoleSystem
}

// the first block should be the text content
func (m *HistoryMessage) ReplaceContents(text string) error {
	// Check if the first block is of type "text"
	if len(m.Content) == 0 {
		return errors.New("no content blocks available")
	}
	if m.Content[0].Type != "text" {
		return errors.New("first block is not of type 'text'")
	}
	m.Content[0].Text = text

	return nil
}

func (m *HistoryMessage) GetContent() string {
	// Concatenate all text content blocks
	var content string
	for _, block := range m.Content {
		if block.Type == "text" {
			content += block.Text + " "
		}
	}
	return strings.TrimSpace(content)
}

func (m *HistoryMessage) GetToolCalls() []llm.ToolCall {
	var calls []llm.ToolCall
	for _, block := range m.Content {
		if block.Type == "tool_use" {
			calls = append(calls, &HistoryToolCall{
				id:   block.ID,
				name: block.Name,
				args: block.Input,
			})
		}
	}
	return calls
}

func (m *HistoryMessage) IsToolResponse() bool {
	for _, block := range m.Content {
		if block.Type == "tool_result" {
			return true
		}
	}
	return false
}

func (m *HistoryMessage) GetToolResponseID() string {
	for _, block := range m.Content {
		if block.Type == "tool_result" {
			return block.ToolUseID
		}
	}
	return ""
}

func (m *HistoryMessage) GetUsage() (int, int) {
	return 0, 0 // History doesn't track usage
}

// HistoryToolCall implements llm.ToolCall for stored tool calls
type HistoryToolCall struct {
	id   string
	name string
	args json.RawMessage
}

func (t *HistoryToolCall) GetID() string {
	return t.id
}

func (t *HistoryToolCall) GetName() string {
	return t.name
}

func (t *HistoryToolCall) GetArguments() map[string]interface{} {
	var args map[string]interface{}
	if err := json.Unmarshal(t.args, &args); err != nil {
		return make(map[string]interface{})
	}
	return args
}

// ContentBlock represents a block of content in a message
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   interface{}     `json:"content,omitempty"`
}
