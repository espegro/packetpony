package ratelimit

import (
	"sync"
)

// ConnectionLimiter caps the number of concurrent connections per IP.
// It is a pure concurrency limiter: Allow reserves a slot, Release frees it.
// Rate limiting over a time window is handled separately by AttemptLimiter.
type ConnectionLimiter struct {
	mu       sync.Mutex
	maxPerIP int
	active   map[string]int
}

// NewConnectionLimiter creates a new concurrent-connection limiter.
func NewConnectionLimiter(maxPerIP int) *ConnectionLimiter {
	return &ConnectionLimiter{
		maxPerIP: maxPerIP,
		active:   make(map[string]int),
	}
}

// Allow reserves a connection slot for the IP, returning false if the IP is
// already at its concurrent-connection limit.
func (l *ConnectionLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.active[ip] >= l.maxPerIP {
		return false
	}

	l.active[ip]++
	return true
}

// Release frees a previously reserved connection slot for the IP. The entry is
// removed once it reaches zero so the map does not grow unbounded.
func (l *ConnectionLimiter) Release(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if count, ok := l.active[ip]; ok && count > 0 {
		if count == 1 {
			delete(l.active, ip)
		} else {
			l.active[ip] = count - 1
		}
	}
}
