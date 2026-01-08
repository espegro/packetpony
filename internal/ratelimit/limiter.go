// Package ratelimit provides sliding window rate limiting for connections, attempts, and bandwidth.
// Supports three action modes: drop (deny), throttle (reduce bandwidth), and log_only (monitor).
package ratelimit

import (
	"sync/atomic"

	"github.com/espegro/packetpony/internal/config"
)

// RateLimitManager manages all rate limiting for a listener
type RateLimitManager struct {
	connLimiter      *ConnectionLimiter
	attemptLimiter   *AttemptLimiter
	bandwidthLimiter *BandwidthLimiter
	totalConns       int64
	maxTotalConns    int64
	action           string
}

// NewRateLimitManager creates a new rate limit manager
func NewRateLimitManager(cfg config.RateLimitConfig) *RateLimitManager {
	var connLimiter *ConnectionLimiter
	if cfg.MaxConnectionsPerIP > 0 && cfg.ConnectionsWindow > 0 {
		connLimiter = NewConnectionLimiter(cfg.MaxConnectionsPerIP, cfg.ConnectionsWindow)
	}

	var attemptLimiter *AttemptLimiter
	if cfg.MaxConnectionAttemptsPerIP > 0 && cfg.AttemptsWindow > 0 {
		attemptLimiter = NewAttemptLimiter(cfg.MaxConnectionAttemptsPerIP, cfg.AttemptsWindow)
	}

	var bandwidthLimiter *BandwidthLimiter
	if cfg.GetMaxBandwidthBytes() > 0 && cfg.BandwidthWindow > 0 {
		action := cfg.Action
		if action == "" {
			action = "drop" // default
		}
		bandwidthLimiter = NewBandwidthLimiter(
			cfg.GetMaxBandwidthBytes(),
			cfg.BandwidthWindow,
			action,
			cfg.GetThrottleMinimumBytes(),
		)
	}

	return &RateLimitManager{
		connLimiter:      connLimiter,
		attemptLimiter:   attemptLimiter,
		bandwidthLimiter: bandwidthLimiter,
		maxTotalConns:    int64(cfg.MaxTotalConnections),
		action:           cfg.Action,
	}
}

// AllowConnection checks if a new connection from the given IP is allowed
func (m *RateLimitManager) AllowConnection(ip string) bool {
	// Check connection attempt limit first (tracks all attempts)
	if m.attemptLimiter != nil {
		if !m.attemptLimiter.RecordAttempt(ip) {
			// Too many attempts - don't even check other limits
			return false
		}
	}

	// Check total connection limit
	if !m.AllowTotalConnection() {
		return false
	}

	// Check per-IP connection limit
	if m.connLimiter != nil {
		if !m.connLimiter.Allow(ip) {
			// Rollback total connection increment
			m.ReleaseTotalConnection()
			return false
		}
	}

	return true
}

// AllowBandwidth checks if bandwidth usage for the given IP is within limits
func (m *RateLimitManager) AllowBandwidth(ip string, bytes int64) bool {
	if m.bandwidthLimiter != nil {
		return m.bandwidthLimiter.Allow(ip, bytes)
	}
	return true
}

// IsBandwidthOverLimit checks if the IP would be over the bandwidth limit
// Useful for logging violations in log_only mode
func (m *RateLimitManager) IsBandwidthOverLimit(ip string, bytes int64) bool {
	if m.bandwidthLimiter != nil {
		return m.bandwidthLimiter.IsOverLimit(ip, bytes)
	}
	return false
}

// GetAction returns the configured action mode
func (m *RateLimitManager) GetAction() string {
	if m.action == "" {
		return "drop"
	}
	return m.action
}

// AllowTotalConnection checks and increments the total connection counter
func (m *RateLimitManager) AllowTotalConnection() bool {
	if m.maxTotalConns == 0 {
		return true
	}

	current := atomic.AddInt64(&m.totalConns, 1)
	if current > m.maxTotalConns {
		atomic.AddInt64(&m.totalConns, -1)
		return false
	}

	return true
}

// ReleaseConnection releases a connection for the given IP
func (m *RateLimitManager) ReleaseConnection(ip string) {
	if m.connLimiter != nil {
		m.connLimiter.Release(ip)
	}
}

// ReleaseTotalConnection decrements the total connection counter
func (m *RateLimitManager) ReleaseTotalConnection() {
	if m.maxTotalConns > 0 {
		atomic.AddInt64(&m.totalConns, -1)
	}
}

// GetTotalConnections returns the current total connection count
func (m *RateLimitManager) GetTotalConnections() int64 {
	return atomic.LoadInt64(&m.totalConns)
}

// Close stops all cleanup goroutines
func (m *RateLimitManager) Close() {
	if m.connLimiter != nil {
		m.connLimiter.Close()
	}
	if m.attemptLimiter != nil {
		m.attemptLimiter.Close()
	}
	if m.bandwidthLimiter != nil {
		m.bandwidthLimiter.Close()
	}
}
