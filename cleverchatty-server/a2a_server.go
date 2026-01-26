package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	cleverchatty "github.com/gelembjuk/cleverchatty/core"
	"github.com/google/uuid"
	a2aprotocol "trpc.group/trpc-go/trpc-a2a-go/protocol"
	a2aserver "trpc.group/trpc-go/trpc-a2a-go/server"
	a2ataskmanager "trpc.group/trpc-go/trpc-a2a-go/taskmanager"
)

const A2AServerVersion = "0.1.0"

type A2AServer struct {
	A2AServerConfig     *cleverchatty.A2AServerConfig
	SessionsManager     *cleverchatty.SessionManager
	WorkDirectory       string
	Logger              *log.Logger
	server              *a2aserver.A2AServer
	notificationSubs    map[string]a2ataskmanager.TaskSubscriber
	notificationSubsMux sync.RWMutex
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
		SessionsManager:  sessionsManager,
		A2AServerConfig:  a2aConfig,
		WorkDirectory:    WorkDirectory,
		Logger:           logger,
		notificationSubs: make(map[string]a2ataskmanager.TaskSubscriber),
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

	// Check if this is a notification subscription request
	if prompt == "__subscribe_notifications__" {
		return a.handleNotificationSubscription(ctx, message, options, handle)
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

// handleNotificationSubscription handles persistent notification subscription requests
func (a *A2AServer) handleNotificationSubscription(
	ctx context.Context,
	message a2aprotocol.Message,
	options a2ataskmanager.ProcessOptions,
	handle a2ataskmanager.TaskHandler,
) (*a2ataskmanager.MessageProcessingResult, error) {
	// Generate context ID if not provided
	if message.ContextID == nil {
		message.ContextID = stringPtr(uuid.New().String())
	}

	contextID := *message.ContextID
	a.Logger.Printf("Notification subscription requested for context: %s", contextID)

	// Must be streaming mode for persistent connection
	if !options.Streaming {
		return nil, fmt.Errorf("notification subscription requires streaming mode")
	}

	// Create a persistent task for notification subscription
	taskID, err := handle.BuildTask(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build notification subscription task: %w", err)
	}

	// Subscribe to the task for streaming events
	subscriber, err := handle.SubScribeTask(&taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to notification task: %w", err)
	}

	// Store subscriber in the notifications map
	a.notificationSubsMux.Lock()
	// Remove old subscription if exists
	if oldSub, exists := a.notificationSubs[contextID]; exists {
		oldSub.Close()
	}
	a.notificationSubs[contextID] = subscriber
	a.notificationSubsMux.Unlock()

	a.Logger.Printf("Notification subscription established for context: %s (taskID: %s)", contextID, taskID)

	// Send initial "subscribed" confirmation event
	subscribedEvent := a2aprotocol.StreamingMessageEvent{
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
						a2aprotocol.NewTextPart("notification_subscribed"),
						a2aprotocol.NewTextPart("Notification subscription active"),
					},
				},
			},
		},
	}

	err = subscriber.Send(subscribedEvent)
	if err != nil {
		a.Logger.Printf("Failed to send subscribed event: %v", err)
	}

	// Start keepalive heartbeat to prevent connection timeout
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				a.Logger.Printf("Notification subscription context cancelled for: %s", contextID)
				a.removeNotificationSubscription(contextID)
				return
			case <-ticker.C:
				// Send keepalive heartbeat
				keepaliveEvent := a2aprotocol.StreamingMessageEvent{
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
									a2aprotocol.NewTextPart("keepalive"),
									a2aprotocol.NewTextPart("Connection active"),
								},
							},
						},
					},
				}

				err := subscriber.Send(keepaliveEvent)
				if err != nil {
					a.Logger.Printf("Failed to send keepalive for context %s: %v", contextID, err)
					a.removeNotificationSubscription(contextID)
					return
				}
			}
		}
	}()

	// Return the streaming subscriber
	// Note: We do NOT send a completion event - this stream stays open indefinitely
	return &a2ataskmanager.MessageProcessingResult{
		StreamingEvents: subscriber,
	}, nil
}

// removeNotificationSubscription removes a notification subscription
func (a *A2AServer) removeNotificationSubscription(contextID string) {
	a.notificationSubsMux.Lock()
	defer a.notificationSubsMux.Unlock()

	if sub, exists := a.notificationSubs[contextID]; exists {
		sub.Close()
		delete(a.notificationSubs, contextID)
		a.Logger.Printf("Removed notification subscription for context: %s", contextID)
	}
}

// BroadcastNotification broadcasts an MCP notification to all subscribed A2A clients
func (a *A2AServer) BroadcastNotification(serverName string, method string, params map[string]interface{}) {
	a.notificationSubsMux.RLock()
	defer a.notificationSubsMux.RUnlock()

	if len(a.notificationSubs) == 0 {
		return // No subscribers
	}

	a.Logger.Printf("Broadcasting MCP notification from %s: %s to %d subscribers", serverName, method, len(a.notificationSubs))

	// Create notification event
	for contextID, subscriber := range a.notificationSubs {
		notifEvent := a2aprotocol.StreamingMessageEvent{
			Result: &a2aprotocol.TaskStatusUpdateEvent{
				TaskID:    "notification_" + uuid.New().String(),
				ContextID: contextID,
				Kind:      "status-update",
				Status: a2aprotocol.TaskStatus{
					State: a2aprotocol.TaskStateWorking,
					Message: &a2aprotocol.Message{
						MessageID: uuid.New().String(),
						Kind:      "message",
						Role:      a2aprotocol.MessageRoleAgent,
						Parts: []a2aprotocol.Part{
							a2aprotocol.NewTextPart("mcp_notification"),
							a2aprotocol.NewTextPart(serverName),
							a2aprotocol.NewTextPart(method),
							a2aprotocol.NewTextPart(fmt.Sprintf("%v", params)),
						},
					},
				},
			},
		}

		err := subscriber.Send(notifEvent)
		if err != nil {
			a.Logger.Printf("Failed to send notification to context %s: %v", contextID, err)
		}
	}
}

func (a *A2AServer) Start() error {
	// Create task manager, inject processor
	taskManager, err := a2ataskmanager.NewMemoryTaskManager(a)
	if err != nil {
		return fmt.Errorf("failed to create task manager: %w", err)
	}

	// Create the server with no timeouts for long-lived notification streams
	a.server, err = a2aserver.NewA2AServer(
		a.agentCard(),
		taskManager,
		a2aserver.WithReadTimeout(0),   // No read timeout for persistent connections
		a2aserver.WithWriteTimeout(0),  // No write timeout for streaming responses
		a2aserver.WithIdleTimeout(0),   // No idle timeout for long-lived connections
	)
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
