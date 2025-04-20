package cleverchatty

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcphost/pkg/history"
	"github.com/mark3labs/mcphost/pkg/llm"
)

func (assistant *CleverChatty) pruneMessages() {
	if len(assistant.messages) <= assistant.config.MessageWindow {
		return
	}

	// Keep only the most recent messages based on window size
	assistant.messages = assistant.messages[len(assistant.messages)-assistant.config.MessageWindow:]

	// Handle messages
	toolUseIds := make(map[string]bool)
	toolResultIds := make(map[string]bool)

	// First pass: collect all tool use and result IDs
	for _, msg := range assistant.messages {
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				toolUseIds[block.ID] = true
			} else if block.Type == "tool_result" {
				toolResultIds[block.ToolUseID] = true
			}
		}
	}

	// Second pass: filter out orphaned tool calls/results
	var prunedMessages []history.HistoryMessage
	for _, msg := range assistant.messages {
		var prunedBlocks []history.ContentBlock
		for _, block := range msg.Content {
			keep := true
			if block.Type == "tool_use" {
				keep = toolResultIds[block.ID]
			} else if block.Type == "tool_result" {
				keep = toolUseIds[block.ToolUseID]
			}
			if keep {
				prunedBlocks = append(prunedBlocks, block)
			}
		}
		// Only include messages that have content or are not assistant messages
		if (len(prunedBlocks) > 0 && msg.Role == "assistant") ||
			msg.Role != "assistant" {
			hasTextBlock := false
			for _, block := range msg.Content {
				if block.Type == "text" {
					hasTextBlock = true
					break
				}
			}
			if len(prunedBlocks) > 0 || hasTextBlock {
				msg.Content = prunedBlocks
				prunedMessages = append(prunedMessages, msg)
			}
		}
	}
	assistant.messages = prunedMessages
}

// Method implementations for simpleMessage
func (assistant *CleverChatty) Prompt(prompt string) error {

	assistant.pruneMessages()

	// Display the user's prompt if it's not empty (i.e., not a tool response)
	if prompt != "" {
		if err := assistant.Callbacks.callStartedPromptProcessing(prompt); err != nil {
			return fmt.Errorf("error in callback: %v", err)
		}

		assistant.messages = append(
			assistant.messages,
			history.HistoryMessage{
				Role: "user",
				Content: []history.ContentBlock{{
					Type: "text",
					Text: prompt,
				}},
			},
		)
	}

	var message llm.Message
	var err error
	backoff := initialBackoff
	retries := 0

	// Convert MessageParam to llm.Message for provider
	// Messages already implement llm.Message interface
	llmMessages := make([]llm.Message, len(assistant.messages))

	for i := range assistant.messages {
		llmMessages[i] = &(assistant.messages)[i]
	}

	for {
		if err := assistant.Callbacks.callStartedThinking(); err != nil {
			return fmt.Errorf("error in callback: %v", err)
		}

		type result struct {
			message llm.Message
			err     error
		}

		resultCh := make(chan result, 1)

		go func() {
			msg, err := assistant.provider.CreateMessage(
				assistant.context,
				prompt,
				llmMessages,
				assistant.mcpHost.tools,
			)
			resultCh <- result{message: msg, err: err}
		}()

		select {
		case res := <-resultCh:
			// done!
			message = res.message
			err = res.err
		case <-assistant.context.Done():
			// context cancelled or timed out
			err = assistant.context.Err()
		}

		if err != nil {
			// Check if it's an overloaded error
			if strings.Contains(err.Error(), "overloaded_error") {
				if retries >= maxRetries {
					return fmt.Errorf(
						"claude is currently overloaded. please wait a few minutes and try again",
					)
				}

				assistant.logger.Printf("Claude is overloaded, retrying... (attempt %d, %s)\n", retries+1, backoff.String())

				time.Sleep(backoff)
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				retries++
				continue
			}
			// If it's not an overloaded error, return the error immediately
			return err
		}
		// If we got here, the request succeeded
		break
	}

	var messageContent []history.ContentBlock

	toolResults := []history.ContentBlock{}
	messageContent = []history.ContentBlock{}

	// Add text content
	if message.GetContent() != "" {
		if err := assistant.Callbacks.callResponseReceived(message.GetContent()); err != nil {
			return fmt.Errorf("error in callback: %v", err)
		}
		messageContent = append(messageContent, history.ContentBlock{
			Type: "text",
			Text: message.GetContent(),
		})
	}

	// Handle tool calls
	for _, toolCall := range message.GetToolCalls() {

		input, _ := json.Marshal(toolCall.GetArguments())
		messageContent = append(messageContent, history.ContentBlock{
			Type:  "tool_use",
			ID:    toolCall.GetID(),
			Name:  toolCall.GetName(),
			Input: input,
		})

		// Log usage statistics if available
		inputTokens, outputTokens := message.GetUsage()
		if inputTokens > 0 || outputTokens > 0 {
			assistant.logger.Printf("Usage statistics: input_tokens=%d, output_tokens=%d, total_tokens=%d\n",
				inputTokens, outputTokens, inputTokens+outputTokens)
		}

		parts := strings.Split(toolCall.GetName(), "__")
		if len(parts) != 2 {
			assistant.logger.Printf(
				"Error: Invalid tool name format: %s\n",
				toolCall.GetName(),
			)
			continue
		}

		serverName, toolName := parts[0], parts[1]
		mcpClient, ok := assistant.mcpHost.clients[serverName]
		if !ok {
			assistant.logger.Printf("Error: Server not found: %s\n", serverName)
			continue
		}

		var toolArgs map[string]interface{}
		if err := json.Unmarshal(input, &toolArgs); err != nil {
			assistant.logger.Printf("Error parsing tool arguments: %v\n", err)
			continue
		}

		var toolResultPtr *mcp.CallToolResult
		type result struct {
			toolResultPtr *mcp.CallToolResult
			err           error
		}

		resultCh := make(chan result, 1)

		go func() {

			req := mcp.CallToolRequest{}
			req.Params.Name = toolName
			req.Params.Arguments = toolArgs

			toolResultPtr, err := mcpClient.CallTool(
				context.Background(),
				req,
			)

			resultCh <- result{toolResultPtr: toolResultPtr, err: err}

		}()

		if err := assistant.Callbacks.callToolCalling(toolCall.GetName()); err != nil {
			return fmt.Errorf("error in callback: %v", err)
		}

		select {
		case res := <-resultCh:
			// done!
			toolResultPtr = res.toolResultPtr
			err = res.err
		case <-assistant.context.Done():
			// context cancelled or timed out
			err = assistant.context.Err()
		}

		if err != nil {
			errMsg := fmt.Sprintf(
				"Error calling tool %s: %v",
				toolName,
				err,
			)
			if err := assistant.Callbacks.callToolCallFailed(toolCall.GetName(), err); err != nil {
				return fmt.Errorf("error in callback: %v", err)
			}

			// Add error message as tool result
			toolResults = append(toolResults, history.ContentBlock{
				Type:      "tool_result",
				ToolUseID: toolCall.GetID(),
				Content: []history.ContentBlock{{
					Type: "text",
					Text: errMsg,
				}},
			})
			continue
		}

		toolResult := *toolResultPtr

		if toolResult.Content != nil {
			// Create the tool result block
			resultBlock := history.ContentBlock{
				Type:      "tool_result",
				ToolUseID: toolCall.GetID(),
				Content:   toolResult.Content,
			}

			// Extract text content
			var resultText string
			// Handle array content directly since we know it's []interface{}
			for _, item := range toolResult.Content {
				if contentMap, ok := item.(mcp.TextContent); ok {
					resultText += fmt.Sprintf("%v ", contentMap.Text)
				}
			}

			resultBlock.Text = strings.TrimSpace(resultText)
			assistant.logger.Printf("created tool result block. %s, %s\n",
				resultBlock,
				toolCall.GetID())

			toolResults = append(toolResults, resultBlock)
		}
	}

	assistant.messages = append(assistant.messages, history.HistoryMessage{
		Role:    message.GetRole(),
		Content: messageContent,
	})

	if len(toolResults) > 0 {
		assistant.messages = append(assistant.messages, history.HistoryMessage{
			Role:    "user", // why user? TODO. To confirm against MCP specs if this shouldbe a user or soething else
			Content: toolResults,
		})

		// Make another call to get LLM's response to the tool results
		return assistant.Prompt("")
	}

	return nil
}
