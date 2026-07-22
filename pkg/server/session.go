package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"remote-container-manager/pkg/docker"
	"remote-container-manager/pkg/logger"
)

// session holds heartbeat monitoring state for an active container session
type Session struct {
	ID            string
	ContainerName string
	PublicKey     string
	LastHeartbeat time.Time
	timer         *time.Timer
}

// sessionmanager tracks active heartbeat sessions and handles timeouts
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.Mutex
	timeout  time.Duration
}

// newsessionmanager creates a new session manager instance
func NewSessionManager(timeout time.Duration) *SessionManager {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &SessionManager{
		sessions: make(map[string]*Session),
		timeout:  timeout,
	}
}

// createsession creates a new heartbeat session with a watchdog timer
func (sm *SessionManager) CreateSession(containerName, pubKey string, dockerMgr *docker.Manager) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// stop any existing session for this container
	for id, s := range sm.sessions {
		if s.ContainerName == containerName {
			if s.timer != nil {
				s.timer.Stop()
			}
			delete(sm.sessions, id)
		}
	}

	// generate random session id
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed to generate session id: %w", err)
	}
	sessionID := hex.EncodeToString(b)

	sess := &Session{
		ID:            sessionID,
		ContainerName: containerName,
		PublicKey:     pubKey,
		LastHeartbeat: time.Now(),
	}

	// setup watchdog timer that stops container if heartbeat times out
	sess.timer = time.AfterFunc(sm.timeout, func() {
		sm.handleTimeout(sessionID, containerName, dockerMgr)
	})

	sm.sessions[sessionID] = sess
	return sess, nil
}

// refreshheartbeat updates last heartbeat timestamp and resets watchdog timer
func (sm *SessionManager) RefreshHeartbeat(sessionID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sess, exists := sm.sessions[sessionID]
	if !exists {
		return false
	}

	sess.LastHeartbeat = time.Now()
	if sess.timer != nil {
		sess.timer.Reset(sm.timeout)
	}
	return true
}

// removesession deletes a session and cancels its watchdog timer
func (sm *SessionManager) RemoveSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sess, exists := sm.sessions[sessionID]; exists {
		if sess.timer != nil {
			sess.timer.Stop()
		}
		delete(sm.sessions, sessionID)
	}
}

// handletimeout executes container stop when heartbeat timer expires
func (sm *SessionManager) handleTimeout(sessionID, containerName string, dockerMgr *docker.Manager) {
	sm.mu.Lock()
	delete(sm.sessions, sessionID)
	sm.mu.Unlock()

	logger.Error("heartbeat timeout detected for session %s (container: %s)! stopping container automatically...", sessionID, containerName)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	info, err := dockerMgr.StopContainer(ctx, containerName)
	if err != nil {
		logger.Error("failed to stop container %s after heartbeat timeout: %v", containerName, err)
		return
	}

	logger.Info("container %s stopped successfully after heartbeat timeout (state: %s)", containerName, info.State)
}
