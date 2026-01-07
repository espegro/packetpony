package ratelimit

import (
	"sync"
	"time"
)

// ConnectionLimiter limits connections per IP using a sliding window
type ConnectionLimiter struct {
	mu          sync.RWMutex
	maxPerIP    int
	window      time.Duration
	connections map[string]*connEntry
	stopCleanup chan struct{}
}

// connEntry tracks connection timestamps for an IP
type connEntry struct {
	mu         sync.Mutex
	count      int
	timestamps []time.Time
}

// NewConnectionLimiter creates a new connection limiter
func NewConnectionLimiter(maxPerIP int, window time.Duration) *ConnectionLimiter {
	limiter := &ConnectionLimiter{
		maxPerIP:    maxPerIP,
		window:      window,
		connections: make(map[string]*connEntry),
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	go limiter.cleanupLoop()

	return limiter
}

// Allow checks if a new connection from the IP is allowed and increments the counter
func (l *ConnectionLimiter) Allow(ip string) bool {
	l.mu.Lock()
	entry, exists := l.connections[ip]
	if !exists {
		entry = &connEntry{
			timestamps: make([]time.Time, 0),
		}
		l.connections[ip] = entry
	}
	l.mu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// Remove expired timestamps
	validTimestamps := make([]time.Time, 0, len(entry.timestamps))
	for _, ts := range entry.timestamps {
		if ts.After(cutoff) {
			validTimestamps = append(validTimestamps, ts)
		}
	}
	entry.timestamps = validTimestamps
	entry.count = len(validTimestamps)

	// Check if limit is exceeded
	if entry.count >= l.maxPerIP {
		return false
	}

	// Allow the connection
	entry.timestamps = append(entry.timestamps, now)
	entry.count++

	return true
}

// Release releases a connection for the IP
func (l *ConnectionLimiter) Release(ip string) {
	l.mu.RLock()
	entry, exists := l.connections[ip]
	l.mu.RUnlock()

	if !exists {
		return
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.count > 0 {
		entry.count--
		// Remove the oldest timestamp
		if len(entry.timestamps) > 0 {
			entry.timestamps = entry.timestamps[1:]
		}
	}
}

// cleanupLoop periodically removes expired entries
func (l *ConnectionLimiter) cleanupLoop() {
	ticker := time.NewTicker(l.window)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanup()
		case <-l.stopCleanup:
			return
		}
	}
}

// cleanup removes expired entries
func (l *ConnectionLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window * 2) // Keep entries for 2x window duration

	for ip, entry := range l.connections {
		entry.mu.Lock()

		// If entry has no active connections and all timestamps are old, remove it
		if entry.count == 0 && len(entry.timestamps) > 0 {
			if entry.timestamps[len(entry.timestamps)-1].Before(cutoff) {
				delete(l.connections, ip)
			}
		} else if entry.count == 0 && len(entry.timestamps) == 0 {
			// Empty entry, remove it
			delete(l.connections, ip)
		}

		entry.mu.Unlock()
	}
}

// Close stops the cleanup goroutine
func (l *ConnectionLimiter) Close() {
	close(l.stopCleanup)
}
