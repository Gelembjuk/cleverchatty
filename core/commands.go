package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gelembjuk/cleverchatty/core/history"
)

func (assistant *CleverChatty) handleSlashCommand(prompt string) (bool, string, error) {
	if !strings.HasPrefix(prompt, "/") {
		return false, "", nil
	}

	switch strings.ToLower(strings.TrimSpace(prompt)) {
	case "/tools":
		return true, assistant.handleToolsCommand(), nil
	case "/history":
		return true, assistant.handleHistoryCommand(), nil
	case "/servers":
		return true, assistant.handleServersCommand(), nil
	default:
		return true, "", errors.New("unknown command")
	}
}

func (assistant *CleverChatty) handleServersCommand() string {
	servers := assistant.GetServersInfo()
	if len(servers) == 0 {
		return "No servers configured.\n"
	}

	var result strings.Builder
	result.WriteString("Available Servers:\n\n")

	for _, server := range servers {
		result.WriteString(fmt.Sprintf("- %s\n", server.Name))
		result.WriteString(fmt.Sprintf("  Transport: %s\n", server.GetType()))

		if server.IsMCPSSEServer() || server.IsMCPHTTPStreamingServer() {
			result.WriteString(fmt.Sprintf("  URL: %s\n", server.Url))
			if server.Headers != nil && len(server.Headers) > 0 {
				result.WriteString("  Headers:\n")
				for _, header := range server.Headers {
					parts := strings.SplitN(header, ":", 2)
					if len(parts) == 2 {
						key := strings.TrimSpace(parts[0])
						result.WriteString(fmt.Sprintf("    %s: [REDACTED]\n", key))
					}
				}
			}
		} else if server.IsA2AServer() {
			result.WriteString(fmt.Sprintf("  Endpoint: %s\n", server.Endpoint))
		} else if server.IsReverseMCP() {
			result.WriteString("  (Remote MCP server connected to this instance)\n")
			if len(server.Tools) > 0 {
				result.WriteString(fmt.Sprintf("  Tools: %d\n", len(server.Tools)))
				for _, tool := range server.Tools {
					result.WriteString(fmt.Sprintf("    - %s", tool.Name))
					if tool.Description != "" {
						result.WriteString(fmt.Sprintf(": %s", tool.Description))
					}
					result.WriteString("\n")
				}
			}
		} else {
			result.WriteString(fmt.Sprintf("  Command: %s\n", server.Command))
			if len(server.Args) > 0 {
				result.WriteString(fmt.Sprintf("  Arguments: %s\n", strings.Join(server.Args, " ")))
			}
		}
		result.WriteString("\n")
	}

	return result.String()
}

func (assistant *CleverChatty) handleToolsCommand() string {
	results := assistant.GetToolsInfo()
	if len(results) == 0 {
		return "No tools available.\n"
	}

	var result strings.Builder
	result.WriteString("Available Tools:\n\n")

	for _, server := range results {
		if server.Err != nil {
			result.WriteString(fmt.Sprintf("- %s (Error: %v)\n", server.Name, server.Err))
			continue
		}

		result.WriteString(fmt.Sprintf("- %s\n", server.Name))
		if len(server.Tools) == 0 {
			result.WriteString("  No tools available\n")
		} else {
			for _, tool := range server.Tools {
				result.WriteString(fmt.Sprintf("  - %s", tool.Name))
				if tool.Description != "" {
					result.WriteString(fmt.Sprintf(": %s", tool.Description))
				}
				result.WriteString("\n")
			}
		}
		result.WriteString("\n")
	}

	return result.String()
}

func (assistant *CleverChatty) handleHistoryCommand() string {
	messages := assistant.GetMessages()
	if len(messages) == 0 {
		return "No conversation history.\n"
	}

	var result strings.Builder
	result.WriteString("Conversation History:\n\n")

	for _, msg := range messages {
		roleTitle := "User"
		switch msg.Role {
		case "assistant":
			roleTitle = "Assistant"
		case "system":
			roleTitle = "System"
		}
		result.WriteString(fmt.Sprintf("--- %s ---\n", roleTitle))

		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				result.WriteString(block.Text + "\n")
			case "tool_use":
				result.WriteString(fmt.Sprintf("[Tool Use: %s]\n", block.Name))
				if block.Input != nil {
					prettyInput, err := json.MarshalIndent(block.Input, "", "  ")
					if err == nil {
						result.WriteString(string(prettyInput) + "\n")
					}
				}
			case "tool_result":
				result.WriteString(fmt.Sprintf("[Tool Result for %s]\n", block.ToolUseID))
				switch v := block.Content.(type) {
				case string:
					result.WriteString(v + "\n")
				case []history.ContentBlock:
					for _, contentBlock := range v {
						if contentBlock.Type == "text" {
							result.WriteString(contentBlock.Text + "\n")
						}
					}
				}
			}
		}
		result.WriteString("\n")
	}

	return result.String()
}
