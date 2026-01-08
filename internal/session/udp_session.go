// Package session provides UDP session tracking and management.
// Sessions are identified by source IP:port and maintain bidirectional communication state.
package session

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// SessionManager manages UDP sessions
type SessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	timeout     time.Duration
	stopCleanup chan struct{}
}

// Session represents a UDP session
type Session struct {
	ID                   string
	SourceAddr           *net.UDPAddr
	TargetConn           *net.UDPConn
	LastActivity         time.Time
	BytesSent            int64
	BytesReceived        int64
	PacketsSent          int64
	PacketsReceived      int64
	CreatedAt            time.Time
	LastPeriodicLog      time.Time
	LastPeriodicLogBytes int64
	ctx                  context.Context
	cancel               context.CancelFunc
	mu                   sync.Mutex
}

// NewSessionManager creates a new session manager
func NewSessionManager(timeout time.Duration) *SessionManager {
	manager := &SessionManager{
		sessions:    make(map[string]*Session),
		timeout:     timeout,
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	go manager.cleanupLoop()

	return manager
}

// GetOrCreate gets an existing session or creates a new one
func (m *SessionManager) GetOrCreate(srcAddr *net.UDPAddr, targetAddr string) (*Session, bool, error) {
	key := sessionKey(srcAddr)

	// Check if session exists
	m.mu.RLock()
	session, exists := m.sessions[key]
	m.mu.RUnlock()

	if exists {
		session.UpdateActivity()
		return session, false, nil
	}

	// Create new session
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	session, exists = m.sessions[key]
	if exists {
		session.UpdateActivity()
		return session, false, nil
	}

	// Create target connection
	targetConn, err := net.DialTimeout("udp", targetAddr, 5*time.Second)
	if err != nil {
		return nil, false, fmt.Errorf("failed to dial target: %w", err)
	}

	udpConn, ok := targetConn.(*net.UDPConn)
	if !ok {
		targetConn.Close()
		return nil, false, fmt.Errorf("failed to convert to UDP connection")
	}

	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()

	session = &Session{
		ID:                   key,
		SourceAddr:           srcAddr,
		TargetConn:           udpConn,
		LastActivity:         now,
		CreatedAt:            now,
		LastPeriodicLog:      now,
		LastPeriodicLogBytes: 0,
		ctx:                  ctx,
		cancel:               cancel,
	}

	m.sessions[key] = session

	return session, true, nil
}

// Get retrieves an existing session
func (m *SessionManager) Get(srcAddr *net.UDPAddr) (*Session, bool) {
	key := sessionKey(srcAddr)

	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[key]
	if exists {
		session.UpdateActivity()
	}

	return session, exists
}

// Remove removes a session from the manager
func (m *SessionManager) Remove(sessionID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil
	}

	delete(m.sessions, sessionID)
	session.cancel()

	return session
}

// cleanupLoop periodically removes expired sessions
func (m *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(m.timeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopCleanup:
			return
		}
	}
}

// cleanup removes expired sessions
func (m *SessionManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, session := range m.sessions {
		session.mu.Lock()
		lastActivity := session.LastActivity
		session.mu.Unlock()

		if now.Sub(lastActivity) > m.timeout {
			delete(m.sessions, key)
			session.cancel()
			session.TargetConn.Close()
		}
	}
}

// Close closes all sessions and stops the cleanup goroutine
func (m *SessionManager) Close() {
	close(m.stopCleanup)

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, session := range m.sessions {
		session.cancel()
		session.TargetConn.Close()
	}
	m.sessions = make(map[string]*Session)
}

// UpdateActivity updates the last activity timestamp
func (s *Session) UpdateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActivity = time.Now()
}

// AddBytesSent atomically adds to bytes sent counter
func (s *Session) AddBytesSent(bytes int64) {
	atomic.AddInt64(&s.BytesSent, bytes)
}

// AddBytesReceived atomically adds to bytes received counter
func (s *Session) AddBytesReceived(bytes int64) {
	atomic.AddInt64(&s.BytesReceived, bytes)
}

// AddPacketsSent atomically adds to packets sent counter
func (s *Session) AddPacketsSent(count int64) {
	atomic.AddInt64(&s.PacketsSent, count)
}

// AddPacketsReceived atomically adds to packets received counter
func (s *Session) AddPacketsReceived(count int64) {
	atomic.AddInt64(&s.PacketsReceived, count)
}

// GetStats returns the session statistics
func (s *Session) GetStats() (bytesSent, bytesReceived, packetsSent, packetsReceived int64) {
	return atomic.LoadInt64(&s.BytesSent),
		atomic.LoadInt64(&s.BytesReceived),
		atomic.LoadInt64(&s.PacketsSent),
		atomic.LoadInt64(&s.PacketsReceived)
}

// Context returns the session context
func (s *Session) Context() context.Context {
	return s.ctx
}

// GetLastActivity returns the last activity time safely
func (s *Session) GetLastActivity() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.LastActivity
}

// ShouldLogPeriodic checks if we should log based on time or bytes thresholds
func (s *Session) ShouldLogPeriodic(intervalDuration time.Duration, intervalBytes int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	totalBytes := atomic.LoadInt64(&s.BytesSent) + atomic.LoadInt64(&s.BytesReceived)

	// Check time-based threshold
	if intervalDuration > 0 && now.Sub(s.LastPeriodicLog) >= intervalDuration {
		return true
	}

	// Check bytes-based threshold
	if intervalBytes > 0 && (totalBytes-s.LastPeriodicLogBytes) >= intervalBytes {
		return true
	}

	return false
}

// UpdatePeriodicLog updates the periodic logging tracking fields
func (s *Session) UpdatePeriodicLog() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastPeriodicLog = time.Now()
	s.LastPeriodicLogBytes = atomic.LoadInt64(&s.BytesSent) + atomic.LoadInt64(&s.BytesReceived)
}

// GetCreatedAt returns the session creation time safely
func (s *Session) GetCreatedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.CreatedAt
}

// sessionKey generates a unique key for a UDP session
func sessionKey(addr *net.UDPAddr) string {
	return addr.String()
}
