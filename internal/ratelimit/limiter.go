package ratelimit

import (
	"sync/atomic"

	"github.com/espegro/packetpony/internal/config"
)

// RateLimitManager manages all rate limiting for a listener
type RateLimitManager struct {
	connLimiter      *ConnectionLimiter
	bandwidthLimiter *BandwidthLimiter
	totalConns       int64
	maxTotalConns    int64
}

// NewRateLimitManager creates a new rate limit manager
func NewRateLimitManager(cfg config.RateLimitConfig) *RateLimitManager {
	var connLimiter *ConnectionLimiter
	if cfg.MaxConnectionsPerIP > 0 && cfg.ConnectionsWindow > 0 {
		connLimiter = NewConnectionLimiter(cfg.MaxConnectionsPerIP, cfg.ConnectionsWindow)
	}

	var bandwidthLimiter *BandwidthLimiter
	if cfg.GetMaxBandwidthBytes() > 0 && cfg.BandwidthWindow > 0 {
		bandwidthLimiter = NewBandwidthLimiter(cfg.GetMaxBandwidthBytes(), cfg.BandwidthWindow)
	}

	return &RateLimitManager{
		connLimiter:      connLimiter,
		bandwidthLimiter: bandwidthLimiter,
		maxTotalConns:    int64(cfg.MaxTotalConnections),
	}
}

// AllowConnection checks if a new connection from the given IP is allowed
func (m *RateLimitManager) AllowConnection(ip string) bool {
	// Check total connection limit first
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
	if m.bandwidthLimiter != nil {
		m.bandwidthLimiter.Close()
	}
}
