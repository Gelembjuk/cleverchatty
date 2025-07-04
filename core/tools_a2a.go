package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gelembjuk/cleverchatty/core/history"
	a2aclient "trpc.group/trpc-go/trpc-a2a-go/client"
	a2aprotocol "trpc.group/trpc-go/trpc-a2a-go/protocol"
)

type A2AAgent struct {
	HostingAgentID    string
	HostingAgentTitle string // Title of the hosting agent, if any
	Endpoint          string
	Card              AgentCard
	Logger            *log.Logger
	Metadata          map[string]string
	filterFunc        func(value string) string
}

// AgentCard represents the structure of the A2A agent.json
type AgentCard struct {
	Name               string       `json:"name"`
	Description        string       `json:"description"`
	URL                string       `json:"url"`
	Version            string       `json:"version"`
	Provider           Provider     `json:"provider"`
	Capabilities       Capabilities `json:"capabilities"`
	DefaultInputModes  []string     `json:"defaultInputModes"`
	DefaultOutputModes []string     `json:"defaultOutputModes"`
	Skills             []Skill      `json:"skills"`
}

type Provider struct {
	Organization string `json:"organization,omitempty"`
}

type Capabilities struct {
	Streaming bool `json:"streaming,omitempty"`
}

type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	InputModes  []string `json:"inputModes"`
	OutputModes []string `json:"outputModes"`
}

// fetchAgentCard fetches and parses the agent.json from baseURL
func fetchA2AAgentCard(baseURL string) (*AgentCard, error) {
	url := strings.TrimRight(baseURL, "/") + "/.well-known/agent.json"

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agent card: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var card AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return nil, fmt.Errorf("failed to parse agent card JSON: %w", err)
	}
	return &card, nil
}

func NewA2AAgent(endpoint string, metadata map[string]string, logger *log.Logger) (*A2AAgent, error) {
	card, err := fetchA2AAgentCard(endpoint)
	if err != nil {
		return nil, fmt.Errorf("error fetching agent card: %v", err)
	}

	if card == nil {
		return nil, fmt.Errorf("agent card is nil")
	}

	if card.Name == "" {
		return nil, fmt.Errorf("agent card name is empty")
	}

	a2aAgent := &A2AAgent{
		Endpoint: endpoint,
		Card:     *card,
		Logger:   logger,
		Metadata: metadata,
	}

	return a2aAgent, nil
}

func (a *A2AAgent) sendMessage(skill string, toolArgs map[string]interface{}, ctx context.Context) ToolCallResult {
	a2aClient, err := a2aclient.NewA2AClient(a.Endpoint)
	if err != nil {
		return ToolCallResult{Error: fmt.Errorf("error creating A2A client: %v", err)}
	}

	parts := make([]a2aprotocol.Part, 0, len(toolArgs))
	for _, value := range toolArgs {
		// Convert value to string if it's not already
		var part a2aprotocol.Part
		switch v := value.(type) {
		case string:
			part = a2aprotocol.NewTextPart(v)
		case int, float64, bool:
			part = a2aprotocol.NewTextPart(fmt.Sprintf("%v", v))
		default:
			part = a2aprotocol.NewTextPart(fmt.Sprintf("%v", value))
		}
		parts = append(parts, part)
	}

	metadata := map[string]any{
		"skill":      skill,
		"agent_name": a.HostingAgentTitle,
	}

	if a.HostingAgentID != "" {
		metadata["agent_id"] = a.HostingAgentID
	}

	if a.Metadata != nil {
		for key, value := range a.Metadata {
			value = a.filterFunc(value)
			metadata[key] = value
		}
	}

	message := a2aprotocol.Message{
		Role:     a2aprotocol.MessageRoleUser,
		Parts:    parts,
		Metadata: metadata,
	}

	taskParams := a2aprotocol.SendMessageParams{
		Message: message,
	}

	messageResult, err := a2aClient.SendMessage(ctx, taskParams)
	if err != nil {
		return ToolCallResult{Error: fmt.Errorf("error starting task stream: %v", err)}
	}

	if messageResult == nil {
		return ToolCallResult{Error: fmt.Errorf("received nil result from A2A client")}
	}

	// Handle the result based on its type
	switch result := messageResult.Result.(type) {
	case *a2aprotocol.Message:
		return a.buildResponseFromMessage(*result)
	case *a2aprotocol.Task:
		a.Logger.Printf("Received task response - ID: %s, State: %s", result.ID, result.Status.State)
		if result.Status.State == a2aprotocol.TaskStateCompleted ||
			result.Status.State == a2aprotocol.TaskStateFailed ||
			result.Status.State == a2aprotocol.TaskStateCanceled {
			return a.buildResponseFromTask(result)
		}

		a.Logger.Printf("Task %s is %s, fetching final state...", result.ID, result.Status.State)

		// Get the task's final state.
		queryParams := a2aprotocol.TaskQueryParams{
			ID: result.ID,
		}
		var task *a2aprotocol.Task
		attemptCount := 0
		maxAttempts := 5
		for {
			// Give the server some time to process.
			time.Sleep(1 * time.Second)

			task, err := a2aClient.GetTasks(ctx, queryParams)
			if err != nil {
				a.Logger.Printf("Failed to get task status: %v", err)
			}

			a.Logger.Printf("Task %s final state: %s", task.ID, task.Status.State)

			if task.Status.State == a2aprotocol.TaskStateCompleted ||
				task.Status.State == a2aprotocol.TaskStateFailed ||
				task.Status.State == a2aprotocol.TaskStateCanceled {
				break // Exit loop if task is in a terminal state
			}
			attemptCount++
			if attemptCount >= maxAttempts {
				a.Logger.Printf("Max attempts reached (%d), exiting.", maxAttempts)
				break
			}
		}
		return a.buildResponseFromTask(task)
	default:
		a.Logger.Printf("Received unknown result type: %T", result)
	}
	return ToolCallResult{
		Error: fmt.Errorf("received unknown result type: %T", messageResult.Result),
	}
}

func (a *A2AAgent) buildResponseFromMessage(message a2aprotocol.Message) ToolCallResult {
	result := ToolCallResult{
		Content: make([]history.Content, 0),
	}
	for _, part := range message.Parts {
		switch p := part.(type) {
		case *a2aprotocol.TextPart:
			result.Content = append(result.Content, history.TextContent{
				Text: p.Text,
			})
		case *a2aprotocol.FilePart:
			// not supported yet
		case *a2aprotocol.DataPart:
			// not supported yet
		default:

		}
	}

	return result
}

// printTaskResult prints the contents of a task result.
func (a *A2AAgent) buildResponseFromTask(task *a2aprotocol.Task) ToolCallResult {
	if task.Status.Message != nil {
		return a.buildResponseFromMessage(*task.Status.Message)
	}
	result := ToolCallResult{
		Content: make([]history.Content, 0),
	}
	// Print artifacts if any
	if len(task.Artifacts) > 0 {
		for _, artifact := range task.Artifacts {
			//name := "Unnamed"
			//if artifact.Name != nil {
			//	name = *artifact.Name
			//}
			for _, part := range artifact.Parts {
				switch p := part.(type) {
				case *a2aprotocol.TextPart:
					result.Content = append(result.Content, history.TextContent{
						Text: p.Text,
					})
				case *a2aprotocol.FilePart:
					// not supported yet
				case *a2aprotocol.DataPart:
					// not supported yet
				default:
					// not supported yet
				}
			}
		}
	}
	return result
}
