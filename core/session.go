package core

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

type Session struct {
	ID        string
	CreatedAt int64
	AI        *CleverChatty
}

type SessionManager struct {
	sessions             map[string]*Session
	mutex                sync.RWMutex
	config               *CleverChattyConfig
	context              context.Context
	logger               *log.Logger
	reverseMCPClient     ReverseMCPClient
	notificationCallback NotificationCallback
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

	// Set notification callback if available
	if sm.notificationCallback != nil {
		ai.SetNotificationCallback(sm.notificationCallback)
	}

	// Create new session
	newSession := &Session{
		ID:        id,
		CreatedAt: time.Now().Unix(),
		AI:        ai,
	}

	sm.mutex.Lock()
	sm.sessions[id] = newSession
	sm.mutex.Unlock()

	return newSession, nil
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
					if now-s.CreatedAt > int64(sm.config.ServerConfig.SessionTimeout) {
						sm.sessions[id].AI.Finish() // Ensure AI session is finished
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
		session.AI.Finish()
		delete(sm.sessions, id)
	}
}
