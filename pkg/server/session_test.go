package server

import (
	"testing"
	"time"
)

func TestSessionManager_CreateAndRefresh(t *testing.T) {
	// create session manager with 100ms timeout
	sm := NewSessionManager(100 * time.Millisecond)

	// test session refresh for nonexistent session
	if sm.RefreshHeartbeat("nonexistent") {
		t.Errorf("refreshing non-existent session should return false")
	}
}
