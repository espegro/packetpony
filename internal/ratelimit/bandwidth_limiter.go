package ratelimit

import (
	"sync"
	"time"
)

// BandwidthLimiter limits bandwidth per IP using a sliding window
type BandwidthLimiter struct {
	mu              sync.RWMutex
	maxPerIP        int64 // bytes
	throttleMinimum int64 // bytes - minimum bandwidth when throttling
	window          time.Duration
	buckets         map[string]*bandwidthBucket
	stopCleanup     chan struct{}
	action          string // drop, throttle, log_only
}

// bandwidthBucket tracks bandwidth consumption for an IP
type bandwidthBucket struct {
	mu      sync.Mutex
	entries []consumptionEntry
}

// consumptionEntry records a bandwidth consumption event
type consumptionEntry struct {
	bytes     int64
	timestamp time.Time
}

// NewBandwidthLimiter creates a new bandwidth limiter
func NewBandwidthLimiter(maxPerIP int64, window time.Duration, action string, throttleMinimum int64) *BandwidthLimiter {
	// Default to "drop" if no action specified
	if action == "" {
		action = "drop"
	}

	limiter := &BandwidthLimiter{
		maxPerIP:        maxPerIP,
		throttleMinimum: throttleMinimum,
		window:          window,
		buckets:         make(map[string]*bandwidthBucket),
		stopCleanup:     make(chan struct{}),
		action:          action,
	}

	// Start cleanup goroutine
	go limiter.cleanupLoop()

	return limiter
}

// Allow checks if the bandwidth usage is within limits
// Returns true if allowed, false if should be dropped
func (l *BandwidthLimiter) Allow(ip string, bytes int64) bool {
	allowed, _ := l.AllowWithStatus(ip, bytes)
	return allowed
}

// AllowWithStatus checks bandwidth usage in a single pass over the sliding
// window. It returns whether the bytes are allowed under the configured action,
// and whether the IP is over its limit regardless of action (so callers can
// report violations, e.g. in log_only mode, without scanning the window twice).
func (l *BandwidthLimiter) AllowWithStatus(ip string, bytes int64) (allowed, overLimit bool) {
	if bytes == 0 {
		return true, false
	}

	l.mu.Lock()
	bucket, exists := l.buckets[ip]
	if !exists {
		bucket = &bandwidthBucket{
			entries: make([]consumptionEntry, 0),
		}
		l.buckets[ip] = bucket
	}
	l.mu.Unlock()

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// Drop expired entries in place and calculate current usage. Compacting
	// into the existing backing array avoids an allocation on every call.
	validEntries := bucket.entries[:0]
	var currentUsage int64
	for _, entry := range bucket.entries {
		if entry.timestamp.After(cutoff) {
			validEntries = append(validEntries, entry)
			currentUsage += entry.bytes
		}
	}
	bucket.entries = validEntries

	overLimit = currentUsage+bytes > l.maxPerIP
	if !overLimit {
		// Within limit: record and allow.
		bucket.entries = append(bucket.entries, consumptionEntry{bytes: bytes, timestamp: now})
		return true, false
	}

	// Over limit: behaviour depends on the action mode.
	switch l.action {
	case "log_only":
		// Allow but report the violation.
		bucket.entries = append(bucket.entries, consumptionEntry{bytes: bytes, timestamp: now})
		return true, true
	case "throttle":
		// Allow only small chunks up to the configured minimum.
		if l.throttleMinimum > 0 && bytes <= l.throttleMinimum {
			bucket.entries = append(bucket.entries, consumptionEntry{bytes: bytes, timestamp: now})
			return true, true
		}
		return false, true
	default: // "drop"
		return false, true
	}
}

// IsOverLimit checks if an IP is currently over its bandwidth limit
// This is useful for logging violations in log_only mode
func (l *BandwidthLimiter) IsOverLimit(ip string, bytes int64) bool {
	if bytes == 0 {
		return false
	}

	l.mu.RLock()
	bucket, exists := l.buckets[ip]
	l.mu.RUnlock()

	if !exists {
		return false
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	var currentUsage int64
	for _, entry := range bucket.entries {
		if entry.timestamp.After(cutoff) {
			currentUsage += entry.bytes
		}
	}

	return currentUsage+bytes > l.maxPerIP
}

// cleanupLoop periodically removes expired buckets
func (l *BandwidthLimiter) cleanupLoop() {
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

// cleanup removes expired buckets
func (l *BandwidthLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window * 2) // Keep buckets for 2x window duration

	for ip, bucket := range l.buckets {
		bucket.mu.Lock()

		// If all entries are old, remove the bucket
		if len(bucket.entries) == 0 {
			delete(l.buckets, ip)
		} else if bucket.entries[len(bucket.entries)-1].timestamp.Before(cutoff) {
			delete(l.buckets, ip)
		}

		bucket.mu.Unlock()
	}
}

// Close stops the cleanup goroutine
func (l *BandwidthLimiter) Close() {
	close(l.stopCleanup)
}
