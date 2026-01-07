package proxy

import (
	"net"
	"time"

	"github.com/espegro/packetpony/internal/acl"
	"github.com/espegro/packetpony/internal/config"
	"github.com/espegro/packetpony/internal/logging"
	"github.com/espegro/packetpony/internal/metrics"
	"github.com/espegro/packetpony/internal/ratelimit"
	"github.com/espegro/packetpony/internal/session"
)

// UDPProxy handles UDP packet proxying with session tracking
type UDPProxy struct {
	config         *config.ListenerConfig
	logger         logging.Logger
	rateLimiter    *ratelimit.RateLimitManager
	allowlist      *acl.Allowlist
	sessionManager *session.SessionManager
	metrics        *metrics.ProxyMetrics
	bufferSize     int
}

// NewUDPProxy creates a new UDP proxy
func NewUDPProxy(
	cfg *config.ListenerConfig,
	logger logging.Logger,
	rateLimiter *ratelimit.RateLimitManager,
	allowlist *acl.Allowlist,
	sessionManager *session.SessionManager,
	metricsCollector *metrics.ProxyMetrics,
) *UDPProxy {
	bufferSize := 4096
	if cfg.UDP != nil && cfg.UDP.BufferSize > 0 {
		bufferSize = cfg.UDP.BufferSize
	}

	return &UDPProxy{
		config:         cfg,
		logger:         logger,
		rateLimiter:    rateLimiter,
		allowlist:      allowlist,
		sessionManager: sessionManager,
		metrics:        metricsCollector,
		bufferSize:     bufferSize,
	}
}

// HandlePacket handles a single UDP packet
func (p *UDPProxy) HandlePacket(data []byte, srcAddr *net.UDPAddr, listenerConn *net.UDPConn) {
	clientIP := srcAddr.IP.String()
	clientPort := srcAddr.Port

	// Check ACL
	if !p.allowlist.IsAllowed(srcAddr.IP) {
		p.metrics.ACLDrops.WithLabelValues(p.config.Name).Inc()
		p.metrics.ConnectionsTotal.WithLabelValues(p.config.Name, "udp", "acl_denied").Inc()
		return
	}

	// Get or create session
	sess, isNew, err := p.sessionManager.GetOrCreate(srcAddr, p.config.TargetAddress)
	if err != nil {
		p.logger.LogError("Failed to create UDP session", map[string]interface{}{
			"listener":  p.config.Name,
			"client_ip": clientIP,
			"error":     err.Error(),
		})
		p.metrics.Errors.WithLabelValues(p.config.Name, "session_create").Inc()
		return
	}

	// Check rate limits for new sessions
	if isNew {
		if !p.rateLimiter.AllowConnection(clientIP) {
			p.logger.LogInfo("UDP session denied by rate limit", map[string]interface{}{
				"listener":  p.config.Name,
				"client_ip": clientIP,
			})
			p.metrics.RateLimitDrops.WithLabelValues(p.config.Name, "connection_limit").Inc()
			p.metrics.ConnectionsTotal.WithLabelValues(p.config.Name, "udp", "rate_limited").Inc()
			p.sessionManager.Remove(sess.ID)
			return
		}

		// Parse target for logging
		targetHost, targetPort, _ := net.SplitHostPort(p.config.TargetAddress)

		// Log session open
		p.logger.LogConnection(logging.ConnectionEvent{
			Timestamp:    time.Now(),
			ListenerName: p.config.Name,
			Protocol:     "udp",
			SourceIP:     clientIP,
			SourcePort:   clientPort,
			TargetIP:     targetHost,
			TargetPort:   parsePort(targetPort),
			EventType:    "open",
		})

		p.metrics.ConnectionsTotal.WithLabelValues(p.config.Name, "udp", "accepted").Inc()
		p.metrics.ConnectionsActive.WithLabelValues(p.config.Name, "udp").Inc()

		// Start reading from target
		go p.startSessionReader(sess, listenerConn)
	}

	// Check bandwidth limit
	if !p.rateLimiter.AllowBandwidth(clientIP, int64(len(data))) {
		p.metrics.RateLimitDrops.WithLabelValues(p.config.Name, "bandwidth_limit").Inc()
		return
	}

	// Forward packet to target
	n, err := sess.TargetConn.Write(data)
	if err != nil {
		p.logger.LogError("Failed to write to target", map[string]interface{}{
			"listener": p.config.Name,
			"session":  sess.ID,
			"error":    err.Error(),
		})
		p.metrics.Errors.WithLabelValues(p.config.Name, "target_write").Inc()
		p.cleanupSession(sess)
		return
	}

	sess.AddBytesSent(int64(n))
	sess.AddPacketsSent(1)
	p.metrics.BytesTransferred.WithLabelValues(p.config.Name, "sent").Add(float64(n))
	p.metrics.PacketsTransferred.WithLabelValues(p.config.Name, "sent").Inc()
}

// startSessionReader reads responses from target and sends back to client
func (p *UDPProxy) startSessionReader(sess *session.Session, listenerConn *net.UDPConn) {
	defer p.cleanupSession(sess)

	buf := make([]byte, p.bufferSize)

	for {
		select {
		case <-sess.Context().Done():
			return
		default:
		}

		// Set read deadline
		if p.config.UDP != nil && p.config.UDP.SessionTimeout > 0 {
			sess.TargetConn.SetReadDeadline(time.Now().Add(p.config.UDP.SessionTimeout))
		}

		n, err := sess.TargetConn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Session timeout
				return
			}
			p.logger.LogError("Failed to read from target", map[string]interface{}{
				"listener": p.config.Name,
				"session":  sess.ID,
				"error":    err.Error(),
			})
			p.metrics.Errors.WithLabelValues(p.config.Name, "target_read").Inc()
			return
		}

		if n > 0 {
			// Send response back to client
			_, err = listenerConn.WriteToUDP(buf[:n], sess.SourceAddr)
			if err != nil {
				p.logger.LogError("Failed to write to client", map[string]interface{}{
					"listener": p.config.Name,
					"session":  sess.ID,
					"error":    err.Error(),
				})
				p.metrics.Errors.WithLabelValues(p.config.Name, "client_write").Inc()
				return
			}

			sess.AddBytesReceived(int64(n))
			sess.AddPacketsReceived(1)
			sess.UpdateActivity()
			p.metrics.BytesTransferred.WithLabelValues(p.config.Name, "received").Add(float64(n))
			p.metrics.PacketsTransferred.WithLabelValues(p.config.Name, "received").Inc()
		}
	}
}

// cleanupSession cleans up a session and logs statistics
func (p *UDPProxy) cleanupSession(sess *session.Session) {
	// Remove from session manager
	removed := p.sessionManager.Remove(sess.ID)
	if removed == nil {
		return // Already cleaned up
	}

	// Close connection
	sess.TargetConn.Close()

	// Release rate limits
	p.rateLimiter.ReleaseConnection(sess.SourceAddr.IP.String())
	p.rateLimiter.ReleaseTotalConnection()

	// Get final stats
	bytesSent, bytesReceived, packetsSent, packetsReceived := sess.GetStats()

	// Parse target for logging
	targetHost, targetPort, _ := net.SplitHostPort(p.config.TargetAddress)

	// Calculate duration
	lastActivity := sess.GetLastActivity()
	duration := time.Since(lastActivity)

	// Log session close
	p.logger.LogConnection(logging.ConnectionEvent{
		Timestamp:       time.Now(),
		ListenerName:    p.config.Name,
		Protocol:        "udp",
		SourceIP:        sess.SourceAddr.IP.String(),
		SourcePort:      sess.SourceAddr.Port,
		TargetIP:        targetHost,
		TargetPort:      parsePort(targetPort),
		EventType:       "close",
		BytesSent:       bytesSent,
		BytesReceived:   bytesReceived,
		PacketsSent:     packetsSent,
		PacketsReceived: packetsReceived,
		Duration:        duration.Milliseconds(),
	})

	p.metrics.ConnectionsActive.WithLabelValues(p.config.Name, "udp").Dec()
	p.metrics.ConnectionDuration.WithLabelValues(p.config.Name, "udp").Observe(duration.Seconds())
}
