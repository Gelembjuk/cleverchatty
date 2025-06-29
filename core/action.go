package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gelembjuk/cleverchatty/core/history"
	"github.com/gelembjuk/cleverchatty/core/llm"
	"github.com/mark3labs/mcp-go/mcp"
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
		if (len(prunedBlocks) > 0 && msg.IsAssistantResponse()) ||
			!msg.IsAssistantResponse() {
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

func (assistant *CleverChatty) addToMemory(role string, content string) {
	// TODO. Add timeouts to context
	assistant.mcpHost.Remember(role, history.ContentBlock{
		Type: "text",
		Text: content,
	}, context.Background())
}

func (assistant *CleverChatty) injectMemories(prompt string) {
	// get memories if there are any
	// TODO. Add timeouts to context
	assistant.Callbacks.CallMemoryRetrievalStarted()

	memories, _ := assistant.mcpHost.Recall(context.Background(), prompt)

	if memories == "" {
		return // no memories to inject
	}
	// if this kind of message is already in the history, remove it to add fresh one
	// from the array assistant.messages remove any records where msg.IsMemoryNote()
	var filteredMessages []history.HistoryMessage
	for _, msg := range assistant.messages {
		if !msg.IsMemoryNote() {
			filteredMessages = append(filteredMessages, msg)
		}
	}
	assistant.messages = filteredMessages

	assistant.logger.Printf("Injecting memories into the history: %s\n", memories)

	assistant.messages = append(assistant.messages, history.NewMemoryNoteMessage(memories))
}

func (assistant *CleverChatty) injectRAGContext(prompt string) {
	// get RAG context if there are any
	if !assistant.mcpHost.HasRagServer() {
		// no RAG context configured, nothing to inject
		return
	}
	// notify callbacks that we are starting RAG retrieval
	assistant.Callbacks.CallRAGRetrievalStarted()

	if assistant.config.RAGConfig.RequirePreprocessing &&
		assistant.config.RAGConfig.PreprocessingPrompt != "" {
		// if preprocessing is required, we need to preprocess the prompt first
		// Send a request to connected LLM provider to preprocess the prompt
		instructionMessage := history.NewSystemInstructionMessage(assistant.config.RAGConfig.PreprocessingPrompt)

		msg, err := assistant.provider.CreateMessage(
			assistant.context,
			prompt,
			[]llm.Message{&instructionMessage},
			assistant.mcpHost.tools,
		)
		if err == nil {
			// if we got a response, use it as the prompt for RAG context
			if msg.GetContent() != "" {
				// modify the prompt to use the response from the LLM
				prompt = msg.GetContent()
			}
		}
	}
	// TODO. Add timeouts to context
	ragDocuments, err := assistant.mcpHost.GetRAGContext(context.Background(), prompt)

	if err != nil {
		assistant.logger.Printf("Error getting RAG context: %v\n", err)
		return
	}

	if len(ragDocuments) == 0 {
		return // no RAG context to inject
	}
	prefix := assistant.config.RAGConfig.ContextPrefix
	if prefix == "" {
		prefix = "Context:"
	}
	// we do not remove the old RAG context, we just append the new one. should we remove the old one?
	// previous context injections will be removed as a part of common strategy
	for _, ragContext := range ragDocuments {
		assistant.messages = append(assistant.messages, history.NewRAGContextMessage(prefix+ragContext))
	}
}

// Method implementations for simpleMessage
func (assistant *CleverChatty) Prompt(prompt string) (string, error) {
	if prompt == "" {
		return "", nil
	}

	// append system instruction to the history
	if assistant.config.SystemInstruction != "" && len(assistant.messages) == 0 {
		assistant.messages = append(assistant.messages, history.NewSystemInstructionMessage(assistant.config.SystemInstruction))
	}

	assistant.pruneMessages()

	assistant.Callbacks.CallStartedPromptProcessing(prompt)

	// if there are memories, inject them into the history
	assistant.injectMemories(prompt)
	// if there is RAG server configured, do request to it and inject in messages
	assistant.injectRAGContext(prompt)

	assistant.messages = append(assistant.messages, history.NewUserPromptMessage(prompt))

	// time to refresh the memory
	assistant.addToMemory("user", prompt)

	response, err := assistant.processPrompt(prompt)
	if err != nil {
		return "", err
	}

	return response, nil
}

func (assistant *CleverChatty) processPrompt(prompt string) (string, error) {

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
		assistant.Callbacks.CallStartedThinking()

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
				// it is specific to Anthropic
				if retries >= maxRetries {
					return "", fmt.Errorf(
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
			return "", err
		}
		// If we got here, the request succeeded
		break
	}

	toolResults := []history.ContentBlock{}
	messageContent := []history.ContentBlock{}

	// Add text content
	if message.GetContent() != "" {
		assistant.Callbacks.CallResponseReceived(message.GetContent())

		messageContent = append(messageContent, history.ContentBlock{
			Type: "text",
			Text: message.GetContent(),
		})

		assistant.addToMemory("assistant", message.GetContent())
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

		assistant.Callbacks.CallToolCalling(toolCall.GetName())

		parts := strings.Split(toolCall.GetName(), "__")
		if len(parts) != 2 {
			continue // Invalid tool name format
		}

		serverName, toolName := parts[0], parts[1]

		toolResultPtr, err := assistant.mcpHost.callTool(
			serverName,
			toolName,
			toolCall.GetArguments(),
			assistant.context,
		)

		if err != nil {
			errMsg := fmt.Sprintf(
				"Error calling tool %s: %v",
				toolCall.GetName(),
				err,
			)
			assistant.Callbacks.CallToolCallFailed(toolCall.GetName(), err)

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

			if assistant.config.DebugMode {
				assistant.logger.Printf("created tool result block. %s, %s\n",
					resultBlock,
					toolCall.GetID())
			}

			toolResults = append(toolResults, resultBlock)
		}
	}
	assistant.messages = append(assistant.messages, history.HistoryMessage{
		Role:    message.GetRole(),
		Content: messageContent,
	})

	if len(toolResults) > 0 {
		assistant.messages = append(assistant.messages, history.HistoryMessage{
			Role:    "user",
			Content: toolResults,
		})

		// Make another call to get LLM's response to the tool results
		return assistant.processPrompt("")
	}

	return message.GetContent(), nil
}
