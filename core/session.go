package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

const DefaultSessionID = "default"

type Session struct {
	ID             string
	Name           string
	CreatedAt      int64
	LastAccessedAt int64
	IsDefault      bool
	AI             *CleverChatty
}

type SessionInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	IsDefault bool   `json:"is_default"`
}

type SessionHistoryMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type SessionManager struct {
	sessions             map[string]*Session
	mutex                sync.RWMutex
	config               *CleverChattyConfig
	context              context.Context
	logger               *log.Logger
	reverseMCPClient     ReverseMCPClient
	notificationCallback NotificationCallback
	agentMessageCallback AgentMessageCallback
}

func NewSessionManager(config *CleverChattyConfig, ctx context.Context, logger *log.Logger) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		config:   config,
		context:  ctx,
		logger:   logger,
	}
}

// SetReverseMCPClient sets the reverse MCP client for dynamic tool registration
func (sm *SessionManager) SetReverseMCPClient(client ReverseMCPClient) {
	sm.reverseMCPClient = client
}

// SetNotificationCallback sets the callback for notifications from MCP servers.
// The callback receives a unified Notification structure instead of the raw MCP notification.
func (sm *SessionManager) SetNotificationCallback(callback NotificationCallback) {
	sm.notificationCallback = callback
}

// SetAgentMessageCallback sets the callback for agent-generated messages
func (sm *SessionManager) SetAgentMessageCallback(callback AgentMessageCallback) {
	sm.agentMessageCallback = callback
}

// GetSession retrieves a session by ID. Returns nil if not found.
func (sm *SessionManager) GetSession(id string) (*Session, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	session, ok := sm.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return session, nil
}

// GetOrCreateSession retrieves an existing session or creates a new one if it doesn't exist.
func (sm *SessionManager) GetOrCreateSession(id string, clientAgentID string) (*Session, error) {
	sm.mutex.RLock()
	sm.logger.Printf("GetOrCreateSession called for ID: %s. There are %d active sessions", id, len(sm.sessions))
	session, ok := sm.sessions[id]
	sm.mutex.RUnlock()

	if ok {
		session.LastAccessedAt = time.Now().Unix()
		return session, nil
	}

	ai, err := GetCleverChattyWithLogger(*sm.config, sm.context, sm.logger)
	if err != nil {
		return nil, err
	}

	ai.WithClientAgentID(clientAgentID)

	err = ai.Init()
	if err != nil {
		return nil, err
	}

	// Set reverse MCP client if available
	if sm.reverseMCPClient != nil {
		ai.SetReverseMCPClient(sm.reverseMCPClient)
	}

	// Set agent message callback if available
	if sm.agentMessageCallback != nil {
		ai.SetAgentMessageCallback(sm.agentMessageCallback)
	}

	// Set notification callback if available
	if sm.notificationCallback != nil {
		ai.SetNotificationCallback(sm.notificationCallback)
	}

	now := time.Now().Unix()

	// Create new session
	newSession := &Session{
		ID:             id,
		CreatedAt:      now,
		LastAccessedAt: now,
		AI:             ai,
	}

	sm.mutex.Lock()
	sm.sessions[id] = newSession
	sm.mutex.Unlock()

	return newSession, nil
}

// CreateDefaultSession creates the default session that persists for the lifetime of the server.
func (sm *SessionManager) CreateDefaultSession(clientAgentID string) error {
	ai, err := GetCleverChattyWithLogger(*sm.config, sm.context, sm.logger)
	if err != nil {
		return err
	}

	ai.WithClientAgentID(clientAgentID)

	err = ai.Init()
	if err != nil {
		return err
	}

	if sm.reverseMCPClient != nil {
		ai.SetReverseMCPClient(sm.reverseMCPClient)
	}
	if sm.agentMessageCallback != nil {
		ai.SetAgentMessageCallback(sm.agentMessageCallback)
	}
	if sm.notificationCallback != nil {
		ai.SetNotificationCallback(sm.notificationCallback)
	}

	now := time.Now().Unix()
	session := &Session{
		ID:             DefaultSessionID,
		Name:           "Default",
		CreatedAt:      now,
		LastAccessedAt: now,
		IsDefault:      true,
		AI:             ai,
	}

	sm.mutex.Lock()
	sm.sessions[DefaultSessionID] = session
	sm.mutex.Unlock()

	sm.logger.Printf("Default session created")
	return nil
}

// ResolveSession resolves a requested session ID to an actual session.
// Returns (session, actualID, isNew, error).
// Special values: "", "default", "main" → default session; "new" → create new session.
func (sm *SessionManager) ResolveSession(requestedID string, clientAgentID string) (*Session, string, bool, error) {
	switch requestedID {
	case "", DefaultSessionID, "main":
		session, err := sm.GetSession(DefaultSessionID)
		if err != nil {
			return nil, "", false, fmt.Errorf("default session not found: %w", err)
		}
		session.LastAccessedAt = time.Now().Unix()
		return session, DefaultSessionID, false, nil

	case "new":
		newID := uuid.New().String()
		session, err := sm.GetOrCreateSession(newID, clientAgentID)
		if err != nil {
			return nil, "", false, err
		}
		return session, newID, true, nil

	default:
		session, err := sm.GetSession(requestedID)
		if err != nil {
			return nil, "", false, fmt.Errorf("session %q not found", requestedID)
		}
		session.LastAccessedAt = time.Now().Unix()
		return session, requestedID, false, nil
	}
}

// GetSessionsList returns info about all active sessions.
func (sm *SessionManager) GetSessionsList() []SessionInfo {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	list := make([]SessionInfo, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		name := s.Name
		if name == "" {
			name = time.Unix(s.CreatedAt, 0).Format("2006-01-02 15:04:05")
		}
		list = append(list, SessionInfo{
			ID:        s.ID,
			Name:      name,
			CreatedAt: s.CreatedAt,
			IsDefault: s.IsDefault,
		})
	}
	return list
}

// GetSessionHistory returns the user/assistant text messages for a session.
func (sm *SessionManager) GetSessionHistory(id string) ([]SessionHistoryMessage, error) {
	session, err := sm.GetSession(id)
	if err != nil {
		return nil, err
	}

	msgs := session.AI.GetMessages()
	var result []SessionHistoryMessage

	for _, m := range msgs {
		sub := m.GetSubRole()
		role := m.GetRole()

		// Only include user prompts and assistant responses (including agent notifications)
		if role == "user" && sub == "prompt" {
			text := m.GetContent()
			if text != "" {
				result = append(result, SessionHistoryMessage{Role: "user", Text: text})
			}
		} else if role == "assistant" {
			text := m.GetContent()
			if text != "" {
				result = append(result, SessionHistoryMessage{Role: "assistant", Text: text})
			}
		}
	}
	return result, nil
}

func (sm *SessionManager) StartCleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-sm.context.Done():
				return
			case <-ticker.C:
				now := time.Now().Unix()
				sm.mutex.Lock()
				for id, s := range sm.sessions {
					if s.IsDefault {
						continue // Never clean up the default session
					}
					if now-s.LastAccessedAt > int64(sm.config.ServerConfig.SessionTimeout) {
						sm.sessions[id].AI.Finish()
						delete(sm.sessions, id)
					}
				}
				sm.mutex.Unlock()
			}
		}
	}()
}

func (sm *SessionManager) FinishSession(id string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if session, ok := sm.sessions[id]; ok {
		if session.IsDefault {
			sm.logger.Printf("Cannot destroy default session")
			return
		}
		session.AI.Finish()
		delete(sm.sessions, id)
	}
}

// FinishAllNonDefaultSessions destroys all sessions except the default one.
// Returns the number of sessions removed.
func (sm *SessionManager) FinishAllNonDefaultSessions() int {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	removed := 0
	for id, session := range sm.sessions {
		if session.IsDefault {
			continue
		}
		session.AI.Finish()
		delete(sm.sessions, id)
		removed++
	}
	return removed
}
