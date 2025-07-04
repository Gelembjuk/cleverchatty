package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	cleverchatty "github.com/gelembjuk/cleverchatty/core"
	"github.com/google/uuid"
	a2aprotocol "trpc.group/trpc-go/trpc-a2a-go/protocol"
	a2aserver "trpc.group/trpc-go/trpc-a2a-go/server"
	a2ataskmanager "trpc.group/trpc-go/trpc-a2a-go/taskmanager"
)

const A2AServerVersion = "0.1.0"

type A2AServer struct {
	A2AServerConfig *cleverchatty.A2AServerConfig
	SessionsManager *cleverchatty.SessionManager
	WorkDirectory   string
	Logger          *log.Logger
	server          *a2aserver.A2AServer
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}

func getA2AServer(
	sessionsManager *cleverchatty.SessionManager,
	a2aConfig *cleverchatty.A2AServerConfig,
	WorkDirectory string,
	logger *log.Logger) (*A2AServer, error) {

	a2aServer := &A2AServer{
		SessionsManager: sessionsManager,
		A2AServerConfig: a2aConfig,
		WorkDirectory:   WorkDirectory,
		Logger:          logger,
	}

	return a2aServer, nil
}
func (a *A2AServer) agentCard() a2aserver.AgentCard {
	chatSkillName := a.A2AServerConfig.ChatSkillName
	chatSkillDescription := a.A2AServerConfig.ChatSkillDescription

	if chatSkillName == "" {
		chatSkillName = "Communicate with AI"
	}
	if chatSkillDescription == "" {
		chatSkillDescription = "Accepts a prompt and returns a response from the AI."
	}
	return a2aserver.AgentCard{
		Name:        a.A2AServerConfig.Title,
		Description: a.A2AServerConfig.Description,
		URL:         a.A2AServerConfig.Url,
		Version:     A2AServerVersion,
		Provider: &a2aserver.AgentProvider{
			Organization: a.A2AServerConfig.Organization,
		},
		Capabilities: a2aserver.AgentCapabilities{
			Streaming: boolPtr(true),
		},
		DefaultInputModes:  []string{a2aprotocol.KindText},
		DefaultOutputModes: []string{a2aprotocol.KindText},
		Skills: []a2aserver.AgentSkill{
			{
				ID:          "ai_chat",
				Name:        chatSkillName,
				Description: stringPtr(chatSkillDescription),
				InputModes:  []string{a2aprotocol.KindText},
				OutputModes: []string{a2aprotocol.KindText},
			},
		},
	}
}

func (a *A2AServer) extractTextFromMessage(message a2aprotocol.Message) string {
	for _, part := range message.Parts {
		if textPart, ok := part.(*a2aprotocol.TextPart); ok {
			return textPart.Text
		}
	}
	return ""
}

func (a *A2AServer) ProcessMessage(
	ctx context.Context,
	message a2aprotocol.Message,
	options a2ataskmanager.ProcessOptions,
	handle a2ataskmanager.TaskHandler,
) (*a2ataskmanager.MessageProcessingResult, error) {
	// Extract text from the incoming message
	prompt := a.extractTextFromMessage(message)

	if prompt == "" {
		return nil, fmt.Errorf("no text part found in the message")
	}

	agentid := ""

	if val, ok := message.Metadata["agent_id"]; ok {
		if str, ok := val.(string); ok {
			agentid = str
		}
	}

	if agentid == "" && a.A2AServerConfig.AgentIDRequired {
		return nil, fmt.Errorf("agent ID is required")
	}

	// TODO. Authentication and authorization here

	if message.ContextID == nil {
		message.ContextID = stringPtr(uuid.New().String()) // Use an empty string if no context ID is provided
	}

	a.Logger.Printf("Text message: %s", prompt)

	session, err := a.SessionsManager.GetOrCreateSession(*message.ContextID, agentid) // Ensure session exists

	if err != nil {
		return nil, fmt.Errorf("failed to get or create session: %w", err)
	}

	if strings.HasPrefix(prompt, "/") {
		if prompt == "/hello" {
			// in fact this is a command to test the server and agentid (and auth in the future)
			return a.buildTextMessageResponse("hello!"), nil
		}
		if prompt == "/quit" || prompt == "/exit" || prompt == "/bye" {
			a.Logger.Printf("Received exit command, stopping server, removing session ID: %s", session.ID)
			a.SessionsManager.FinishSession(session.ID) // Finish the session
			return a.buildTextMessageResponse("Bye!"), nil
		}
	}

	if !options.Streaming {
		// Process the text This is not streaming response
		response, err := session.AI.Prompt(prompt)

		if err != nil {
			return nil, fmt.Errorf("failed to process prompt: %w", err)
		}
		a.Logger.Printf("Response from AI: %s. ", response)
		// Return a simple response message
		return a.buildTextMessageResponse(response), nil
	}

	a.Logger.Println("Using streaming mode")

	// Create a task for streaming
	taskID, err := handle.BuildTask(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build task: %w", err)
	}

	// Subscribe to the task for streaming events
	subscriber, err := handle.SubScribeTask(&taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to task: %w", err)
	}

	// Start streaming processing in a goroutine
	go func() {
		defer func() {
			if subscriber != nil {
				subscriber.Close()
			}
		}()

		contextID := handle.GetContextID()

		session.AI.Callbacks.SetStartedPromptProcessing(func(prompt string) error {
			a.statusUpdate(cleverchatty.CallbackCodePromptProcessing, prompt, "", taskID, contextID, subscriber)
			return nil
		})
		session.AI.Callbacks.SetStartedThinking(func() error {
			a.statusUpdate(cleverchatty.CallbackCodeStartedThinking, "Thinking...", "", taskID, contextID, subscriber)
			return nil
		})
		session.AI.Callbacks.SetMemoryRetrievalStarted(func() error {
			a.statusUpdate(cleverchatty.CallbackCodeMemoryRetrieval, "Recalling...", "", taskID, contextID, subscriber)
			return nil
		})
		session.AI.Callbacks.SetRAGRetrievalStarted(func() error {
			a.statusUpdate(cleverchatty.CallbackCodeRAGRetrieval, "Searching knowledge database ...", "", taskID, contextID, subscriber)
			return nil
		})
		session.AI.Callbacks.SetToolCalling(func(toolName string) error {
			a.statusUpdate(cleverchatty.CallbackCodeToolCalling, "Using tool: "+toolName, toolName, taskID, contextID, subscriber)
			return nil
		})
		session.AI.Callbacks.SetToolCallFailed(func(toolName string, err error) error {
			a.statusUpdate(cleverchatty.CallbackCodeToolCallFailed, err.Error(), toolName, taskID, contextID, subscriber)
			return nil
		})
		session.AI.Callbacks.SetResponseReceived(func(response string) error {
			a.statusUpdate(cleverchatty.CallbackCodeResponseReceived, response, "", taskID, contextID, subscriber)
			return nil
		})

		response, err := session.AI.Prompt(prompt)

		if err != nil {
			a.statusFailed(err, taskID, contextID, subscriber)
			return
		}

		// Final completion status update
		completeEvent := a2aprotocol.StreamingMessageEvent{
			Result: &a2aprotocol.TaskStatusUpdateEvent{
				TaskID:    taskID,
				ContextID: contextID,
				Kind:      "status-update",
				Status: a2aprotocol.TaskStatus{
					State: a2aprotocol.TaskStateCompleted,
					Message: &a2aprotocol.Message{
						MessageID: uuid.New().String(),
						Kind:      "message",
						Role:      a2aprotocol.MessageRoleAgent,
						Parts:     []a2aprotocol.Part{a2aprotocol.NewTextPart(response)},
					},
				},
				Final: true,
			},
		}
		err = subscriber.Send(completeEvent)
		if err != nil {
			a.Logger.Fatalf("Failed to send complete event: %v", err)
		}

		a.Logger.Printf("Task %s streaming completed successfully.", taskID)
	}()

	return &a2ataskmanager.MessageProcessingResult{
		StreamingEvents: subscriber,
	}, nil

}
func (a *A2AServer) buildTextMessageResponse(text string) *a2ataskmanager.MessageProcessingResult {
	responseMessage := a2aprotocol.NewMessage(
		a2aprotocol.MessageRoleAgent,
		[]a2aprotocol.Part{a2aprotocol.NewTextPart(text)},
	)
	return &a2ataskmanager.MessageProcessingResult{
		Result: &responseMessage,
	}
}
func (a *A2AServer) statusUpdate(statusCode string, statusMessage string, statusMessageExtra string, taskID string, contextID string, subscriber a2ataskmanager.TaskSubscriber) {
	workingEvent := a2aprotocol.StreamingMessageEvent{
		Result: &a2aprotocol.TaskStatusUpdateEvent{
			TaskID:    taskID,
			ContextID: contextID,
			Kind:      "status-update",
			Status: a2aprotocol.TaskStatus{
				State: a2aprotocol.TaskStateWorking,
				Message: &a2aprotocol.Message{
					MessageID: uuid.New().String(),
					Kind:      "message",
					Role:      a2aprotocol.MessageRoleAgent,
					Parts: []a2aprotocol.Part{
						a2aprotocol.NewTextPart(statusCode),
						a2aprotocol.NewTextPart(statusMessage),
						a2aprotocol.NewTextPart(statusMessageExtra),
					},
				},
			},
		},
	}
	err := subscriber.Send(workingEvent)
	if err != nil {
		a.Logger.Fatalf("Failed to send status update event: %v", err)
	}
}

func (a *A2AServer) statusFailed(err error, taskID string, contextID string, subscriber a2ataskmanager.TaskSubscriber) {
	cancelEvent := a2aprotocol.StreamingMessageEvent{
		Result: &a2aprotocol.TaskStatusUpdateEvent{
			TaskID:    taskID,
			ContextID: contextID,
			Kind:      "status-update",
			Status: a2aprotocol.TaskStatus{
				State: a2aprotocol.TaskStateFailed,
				Message: &a2aprotocol.Message{
					MessageID: uuid.New().String(),
					Kind:      "message",
					Role:      a2aprotocol.MessageRoleAgent,
					Parts:     []a2aprotocol.Part{a2aprotocol.NewTextPart(err.Error())},
				},
			},
			Final: true,
		},
	}
	err = subscriber.Send(cancelEvent)
	if err != nil {
		a.Logger.Fatalf("Failed to report failed event: %v", err)
	}
}

func (a *A2AServer) Start() error {
	// Create task manager, inject processor
	taskManager, err := a2ataskmanager.NewMemoryTaskManager(a)
	if err != nil {
		return fmt.Errorf("failed to create task manager: %w", err)
	}

	// Create the server
	a.server, err = a2aserver.NewA2AServer(a.agentCard(), taskManager)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	go func() {
		// Start the server
		a.Logger.Printf("Agent server started on %s", a.A2AServerConfig.ListenHost)
		if err := a.server.Start(a.A2AServerConfig.ListenHost); err != nil {
			a.Logger.Fatalf("Server start failed: %v", err)
		}
	}()
	return nil
}
func (a *A2AServer) Stop() error {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := a.server.Stop(shutdownCtx); err != nil {
		return err
	}
	return nil
}
