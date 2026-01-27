package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	cleverchatty "github.com/gelembjuk/cleverchatty/core"
	a2aclient "trpc.group/trpc-go/trpc-a2a-go/client"
	a2aprotocol "trpc.group/trpc-go/trpc-a2a-go/protocol"
	a2aserver "trpc.group/trpc-go/trpc-a2a-go/server"
)

func checkServerIsCleverChatty(serverURL string) (bool, error) {
	// According to the A2A protocol, agent cards are available at protocol.AgentCardPath
	agentCardURL := serverURL
	if agentCardURL[len(agentCardURL)-1] != '/' {
		agentCardURL += "/"
	}
	// Use the constant defined in the protocol package instead of hardcoding the path
	agentCardURL += a2aprotocol.AgentCardPath[1:] // Remove leading slash as we already have one

	// Create a request with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, agentCardURL, nil)
	if err != nil {
		return false, fmt.Errorf("error creating request: %w", err)
	}

	// Make the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("error fetching agent card: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read and parse the agent card
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("error reading response body: %w", err)
	}

	var agentCard a2aserver.AgentCard
	if err := json.Unmarshal(body, &agentCard); err != nil {
		return false, fmt.Errorf("error parsing agent card: %w", err)
	}

	// Handle the new *bool type for Streaming capability
	if agentCard.Capabilities.Streaming == nil {
		return false, nil
	}
	if !(*agentCard.Capabilities.Streaming) {
		return false, nil
	}
	for _, skill := range agentCard.Skills {
		if skill.ID == "ai_chat" {
			return true, nil // This is a CleverChatty server
		}
	}
	return false, nil // Not a CleverChatty server
}
func sendHelloMessage(ctx context.Context, serverURL string, agentid string, ContextID *string) error {
	a2aClient, err := a2aclient.NewA2AClient(serverURL)
	if err != nil {
		return fmt.Errorf("error creating A2A client: %v", err)
	}
	taskParams := a2aprotocol.SendMessageParams{
		Message: a2aprotocol.Message{
			Role: a2aprotocol.MessageRoleUser,
			Parts: []a2aprotocol.Part{
				a2aprotocol.NewTextPart("/hello"),
			},
			ContextID: ContextID,
			Metadata: map[string]any{
				"agent_id": agentid,
			},
		},
	}

	_, err = a2aClient.SendMessage(ctx, taskParams)

	return err
}
func processA2AStreamEvents(ctx context.Context,
	streamChan <-chan a2aprotocol.StreamingMessageEvent,
	callbacks cleverchatty.UICallbacks) (string, error) {

	for {
		select {
		case <-ctx.Done():
			// Context timed out or was cancelled
			return "", nil
		case event, ok := <-streamChan:
			if !ok {
				// Channel closed by the client/server
				if ctx.Err() != nil {
					return "", ctx.Err() // Return context error if any
				}
				return "", nil
			}

			// Process the received event
			switch e := event.Result.(type) {
			case *a2aprotocol.TaskStatusUpdateEvent:
				if e.Status.State == a2aprotocol.TaskStateWorking {
					if e.Status.Message != nil && len(e.Status.Message.Parts) == 3 {
						statusCode := e.Status.Message.Parts[0].(*a2aprotocol.TextPart).Text
						statusMessage := e.Status.Message.Parts[1].(*a2aprotocol.TextPart).Text
						statusMessageExtra := e.Status.Message.Parts[2].(*a2aprotocol.TextPart).Text

						switch statusCode {
						case cleverchatty.CallbackCodePromptProcessing:
							callbacks.CallStartedPromptProcessing(statusMessage)
						case cleverchatty.CallbackCodeStartedThinking:
							callbacks.CallStartedThinking()
						case cleverchatty.CallbackCodeResponseReceived:
							callbacks.CallResponseReceived(statusMessage)
						case cleverchatty.CallbackCodeToolCalling:
							callbacks.CallToolCalling(statusMessage)
						case cleverchatty.CallbackCodeToolCallFailed:
							callbacks.CallToolCallFailed(statusMessageExtra, errors.New(statusMessage))
						case cleverchatty.CallbackCodeMemoryRetrieval:
							callbacks.CallMemoryRetrievalStarted()
						case cleverchatty.CallbackCodeRAGRetrieval:
							callbacks.CallRAGRetrievalStarted()
						default:
							//
						}
					}
				}
				if e.Final {
					switch e.Status.State {
					case a2aprotocol.TaskStateCompleted:
						if e.Status.Message != nil {
							response := e.Status.Message.Parts[0].(*a2aprotocol.TextPart).Text
							return response, nil
						}
					case a2aprotocol.TaskStateFailed:
						if e.Status.Message != nil {
							errorMessage := e.Status.Message.Parts[0].(*a2aprotocol.TextPart).Text
							return "", fmt.Errorf("task failed: %s", errorMessage)
						}
					case a2aprotocol.TaskStateCanceled:
						// in this architecture we do not expect to receive canceled state
					}
					return "", nil
				}
			case *a2aprotocol.TaskArtifactUpdateEvent:
				// We do not expect artifacts in the stream
				// A response is returned fully together with a2aprotocol.TaskStateCompleted
				// TODO: But it would be nice to support real streaming for LLM response
			default:
				//
			}
		}
	}
}
