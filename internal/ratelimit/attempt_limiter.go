package ratelimit

import (
	"sync"
	"time"
)

// AttemptLimiter limits connection attempts per IP using a sliding window
// This tracks ALL connection attempts, including rejected ones
type AttemptLimiter struct {
	mu          sync.RWMutex
	maxPerIP    int
	window      time.Duration
	attempts    map[string]*attemptEntry
	stopCleanup chan struct{}
}

// attemptEntry tracks connection attempt timestamps for an IP
type attemptEntry struct {
	mu         sync.Mutex
	timestamps []time.Time
}

// NewAttemptLimiter creates a new connection attempt limiter
func NewAttemptLimiter(maxPerIP int, window time.Duration) *AttemptLimiter {
	limiter := &AttemptLimiter{
		maxPerIP:    maxPerIP,
		window:      window,
		attempts:    make(map[string]*attemptEntry),
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	go limiter.cleanupLoop()

	return limiter
}

// RecordAttempt records a connection attempt and returns true if allowed
func (l *AttemptLimiter) RecordAttempt(ip string) bool {
	l.mu.Lock()
	entry, exists := l.attempts[ip]
	if !exists {
		entry = &attemptEntry{
			timestamps: make([]time.Time, 0),
		}
		l.attempts[ip] = entry
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

	// Check if limit is exceeded
	if len(entry.timestamps) >= l.maxPerIP {
		// Still record the attempt for tracking
		entry.timestamps = append(entry.timestamps, now)
		return false
	}

	// Record the attempt
	entry.timestamps = append(entry.timestamps, now)
	return true
}

// cleanupLoop periodically removes expired entries
func (l *AttemptLimiter) cleanupLoop() {
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
func (l *AttemptLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window * 2) // Keep entries for 2x window duration

	for ip, entry := range l.attempts {
		entry.mu.Lock()

		// If all timestamps are old, remove the entry
		if len(entry.timestamps) > 0 {
			if entry.timestamps[len(entry.timestamps)-1].Before(cutoff) {
				delete(l.attempts, ip)
			}
		} else {
			// Empty entry, remove it
			delete(l.attempts, ip)
		}

		entry.mu.Unlock()
	}
}

// Close stops the cleanup goroutine
func (l *AttemptLimiter) Close() {
	close(l.stopCleanup)
}
