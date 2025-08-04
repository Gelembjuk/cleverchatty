package core

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gelembjuk/cleverchatty/core/history"
	"github.com/gelembjuk/cleverchatty/core/llm"
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
			switch block.Type {
			case "tool_use":
				toolUseIds[block.ID] = true
			case "tool_result":
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
			switch block.Type {
			case "tool_use":
				keep = toolResultIds[block.ID]
			case "tool_result":
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
	assistant.toolsHost.Remember(role, history.ContentBlock{
		Type: "text",
		Text: content,
	}, assistant.ClientAgentID, context.Background())
}

func (assistant *CleverChatty) addToolRequestToMemory(serverName string, toolName string, toolRequest llm.ToolCall, toolResponse ToolCallResult) {
	/*
	* Remember the tool request and response in the memory server.
	* Find if this tool is in the list of notification producers.
	* If it is, then remember the request and response in the memory server.
	 */
	if !assistant.config.ToolsListenerConfig.Enabled {
		return // no tools listener configured, nothing to remember
	}
	found := false
	for _, tool := range assistant.config.ToolsListenerConfig.ToolServers {
		if tool.ServerID == serverName {
			// Check if toolName is in tool.Tools array
			if slices.Contains(tool.Tools, toolName) {
				found = true
				break
			}
		}
	}
	if !found {
		// tool is not in the list of notification producers, nothing to remember
		return
	}
	// This tool is in the list of notification producers, remember the request and response
	// Remember the request. IT is the request from the agent to a tool
	assistant.toolsHost.Remember(
		"assistant",
		history.ContentBlock{
			Type:      "text",
			Text:      fmt.Sprintf("Tool %s called with arguments: %s", toolName, toolRequest.GetArguments()),
			ToolUseID: toolRequest.GetID(),
		},
		fmt.Sprintf("%s__%s", serverName, toolName),
		context.Background(),
	)

	if toolResponse.Error == nil {
		// Remember the response if there is no error
		assistant.toolsHost.Remember(
			"user", // we consider a tool like a user. TODO: maybe we should use "tool" role?
			history.ContentBlock{
				Type:      "text",
				Text:      fmt.Sprintf("Tool %s returned: %s", toolName, toolResponse.getTextContent()),
				ToolUseID: toolRequest.GetID(),
			},
			fmt.Sprintf("%s__%s", serverName, toolName),
			context.Background(),
		)
	}
}

func (assistant *CleverChatty) injectMemories(prompt string) {
	// get memories if there are any
	// TODO. Add timeouts to context
	assistant.Callbacks.CallMemoryRetrievalStarted()

	memories, _ := assistant.toolsHost.Recall(context.Background(), prompt, assistant.ClientAgentID)

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
	if !assistant.toolsHost.HasRagServer() {
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
			assistant.toolsHost.GetAllToolsForLLM(),
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
	ragDocuments, err := assistant.toolsHost.GetRAGContext(context.Background(), prompt)

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

	// Check for slash commands first
	handled, response, err := assistant.handleSlashCommand(prompt)
	if handled {
		if err != nil {
			return "", err
		}
		// Call the response callback so streaming clients receive the response
		assistant.Callbacks.CallResponseReceived(response)
		return response, nil
	}

	if len(assistant.messages) == 0 {
		// append system instruction to the history

		instructions := ""

		if assistant.config.SystemInstruction != "" {
			instructions = assistant.config.SystemInstruction
			instructions = strings.ReplaceAll(instructions, "{AGENT_ID}", assistant.config.AgentID)
			instructions = strings.ReplaceAll(instructions, "{CLIENT_AGENT_ID}", assistant.ClientAgentID)
		} else if assistant.ClientAgentID != "" {
			instructions = fmt.Sprintf(
				"You communicate with the agent ID %s. Use this ID for future references.",
				assistant.ClientAgentID,
			)
		}

		if instructions != "" {
			assistant.messages = append(assistant.messages, history.NewSystemInstructionMessage(instructions))
		}
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

	response, err = assistant.processPrompt(prompt)
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
				assistant.toolsHost.GetAllToolsForLLM(),
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

		toolResult := assistant.toolsHost.callTool(
			serverName,
			toolName,
			toolCall.GetArguments(),
			assistant.context,
		)

		if toolResult.Error != nil {
			errMsg := fmt.Sprintf(
				"Error calling tool %s: %v",
				toolCall.GetName(),
				toolResult.Error,
			)
			assistant.Callbacks.CallToolCallFailed(toolCall.GetName(), toolResult.Error)

			// Add error message as tool result
			toolResults = append(toolResults, history.ContentBlock{
				Type:      "tool_result",
				Text:      errMsg,
				ToolUseID: toolCall.GetID(),
				Content:   history.NewTextContent(errMsg),
			})
			continue
		}
		// Remember this request in the memory server
		assistant.addToolRequestToMemory(serverName, toolName, toolCall, toolResult)

		// Create the tool result block
		resultBlock := history.ContentBlock{
			Type:      "tool_result",
			Text:      toolResult.getTextContent(),
			ToolUseID: toolCall.GetID(),
			Content:   toolResult.Content,
		}

		if assistant.config.DebugMode {
			assistant.logger.Printf("created tool result block. %s, %s\n",
				resultBlock,
				toolCall.GetID())
		}

		toolResults = append(toolResults, resultBlock)
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
