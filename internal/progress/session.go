package progress

import (
	"sync"
	"time"
)

// SessionTimer handles session durations and timing independently of progress updates.
type SessionTimer struct {
	mu                sync.Mutex
	startTime         time.Time
	savedElapsed      time.Duration
	sessionStartBytes int64
}

func (s *SessionTimer) StartTime() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startTime
}

func (s *SessionTimer) SetStartTimeForTest(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startTime = t
}

func (s *SessionTimer) GetSessionStartBytesForTest() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionStartBytes
}

func (s *SessionTimer) SetSessionStartBytesForTest(b int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionStartBytes = b
}

// SyncSessionStart synchronizes the start time and the verified bytes to start a new tracking session.
func (s *SessionTimer) SyncSessionStart(verifiedProgress int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionStartBytes = verifiedProgress
	s.startTime = time.Now()
}

// FinalizeSession closes the current session, adds its duration to savedElapsed, and starts a new session.
func (s *SessionTimer) FinalizeSession(downloaded int64) (sessionElapsed, totalElapsed time.Duration) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionElapsed = now.Sub(s.startTime)
	if sessionElapsed < 0 {
		sessionElapsed = 0
	}
	s.savedElapsed += sessionElapsed
	if s.savedElapsed < 0 {
		s.savedElapsed = 0
	}
	s.sessionStartBytes = downloaded
	s.startTime = now
	totalElapsed = s.savedElapsed

	return sessionElapsed, totalElapsed
}

// GetElapsed returns the session and total elapsed times based on pause status.
func (s *SessionTimer) GetElapsed(paused bool) (sessionElapsed, totalElapsed time.Duration, sessionStartBytes int64) {
	s.mu.Lock()
	saved := s.savedElapsed
	start := s.startTime
	sessionStartBytes = s.sessionStartBytes
	s.mu.Unlock()

	if paused {
		sessionElapsed = 0
		totalElapsed = saved
	} else {
		sessionElapsed = time.Since(start)
		if sessionElapsed < 0 {
			sessionElapsed = 0
		}
		totalElapsed = saved + sessionElapsed
	}
	if totalElapsed < 0 {
		totalElapsed = 0
	}
	return
}

func (s *SessionTimer) SetSavedElapsed(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.savedElapsed = d
}

func (s *SessionTimer) GetSavedElapsed() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.savedElapsed
}

// SessionReset completely clears the timer.
func (s *SessionTimer) SessionReset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionStartBytes = 0
	s.startTime = time.Now()
	s.savedElapsed = 0
}
