package core

import (
	"context"
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
	sessions map[string]*Session
	mutex    sync.RWMutex
	config   *CleverChattyConfig
	context  context.Context
	logger   *log.Logger
}

func NewSessionManager(config *CleverChattyConfig, ctx context.Context, logger *log.Logger) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		config:   config,
		context:  ctx,
		logger:   logger,
	}
}

func (sm *SessionManager) GetOrCreateSession(id string) (*Session, error) {
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
