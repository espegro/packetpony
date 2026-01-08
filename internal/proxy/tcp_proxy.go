// Package proxy implements TCP and UDP proxying with rate limiting, ACLs, and metrics.
package proxy

import (
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/espegro/packetpony/internal/acl"
	"github.com/espegro/packetpony/internal/config"
	"github.com/espegro/packetpony/internal/logging"
	"github.com/espegro/packetpony/internal/metrics"
	"github.com/espegro/packetpony/internal/ratelimit"
)

// TCPProxy handles TCP connection proxying with rate limiting and access control.
type TCPProxy struct {
	config      *config.ListenerConfig
	logger      logging.Logger
	rateLimiter *ratelimit.RateLimitManager
	allowlist   *acl.Allowlist
	metrics     *metrics.ProxyMetrics
}

// connStats tracks connection statistics
type connStats struct {
	startTime     time.Time
	bytesSent     int64
	bytesReceived int64
}

// NewTCPProxy creates a new TCP proxy
func NewTCPProxy(
	cfg *config.ListenerConfig,
	logger logging.Logger,
	rateLimiter *ratelimit.RateLimitManager,
	allowlist *acl.Allowlist,
	metricsCollector *metrics.ProxyMetrics,
) *TCPProxy {
	return &TCPProxy{
		config:      cfg,
		logger:      logger,
		rateLimiter: rateLimiter,
		allowlist:   allowlist,
		metrics:     metricsCollector,
	}
}

// HandleConnection handles a single TCP connection
func (p *TCPProxy) HandleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	stats := &connStats{
		startTime: time.Now(),
	}

	// Extract client IP
	clientAddr := clientConn.RemoteAddr().(*net.TCPAddr)
	clientIP := clientAddr.IP.String()
	clientPort := clientAddr.Port

	// Parse target address
	targetHost, targetPort, err := net.SplitHostPort(p.config.TargetAddress)
	if err != nil {
		p.logger.LogError("Invalid target address", map[string]interface{}{
			"listener": p.config.Name,
			"error":    err.Error(),
		})
		p.metrics.Errors.WithLabelValues(p.config.Name, "invalid_target").Inc()
		return
	}

	// Check ACL
	if !p.allowlist.IsAllowed(clientAddr.IP) {
		p.logger.LogInfo("Connection denied by ACL", map[string]interface{}{
			"listener":  p.config.Name,
			"client_ip": clientIP,
		})
		p.metrics.ACLDrops.WithLabelValues(p.config.Name).Inc()
		p.metrics.ConnectionsTotal.WithLabelValues(p.config.Name, "tcp", "acl_denied").Inc()
		return
	}

	// Check rate limits
	if !p.rateLimiter.AllowConnection(clientIP) {
		p.logger.LogInfo("Connection denied by rate limit", map[string]interface{}{
			"listener":  p.config.Name,
			"client_ip": clientIP,
		})
		p.metrics.RateLimitDrops.WithLabelValues(p.config.Name, "connection_limit").Inc()
		p.metrics.ConnectionsTotal.WithLabelValues(p.config.Name, "tcp", "rate_limited").Inc()
		return
	}
	defer p.rateLimiter.ReleaseConnection(clientIP)
	defer p.rateLimiter.ReleaseTotalConnection()

	// Log connection open
	p.logger.LogConnection(logging.ConnectionEvent{
		Timestamp:    time.Now(),
		ListenerName: p.config.Name,
		Protocol:     "tcp",
		SourceIP:     clientIP,
		SourcePort:   clientPort,
		TargetIP:     targetHost,
		TargetPort:   parsePort(targetPort),
		EventType:    "open",
	})

	p.metrics.ConnectionsTotal.WithLabelValues(p.config.Name, "tcp", "accepted").Inc()
	p.metrics.ConnectionsActive.WithLabelValues(p.config.Name, "tcp").Inc()
	defer p.metrics.ConnectionsActive.WithLabelValues(p.config.Name, "tcp").Dec()

	// Connect to target
	targetConn, err := net.DialTimeout("tcp", p.config.TargetAddress, 10*time.Second)
	if err != nil {
		p.logger.LogError("Failed to connect to target", map[string]interface{}{
			"listener": p.config.Name,
			"target":   p.config.TargetAddress,
			"error":    err.Error(),
		})
		p.metrics.Errors.WithLabelValues(p.config.Name, "target_connect").Inc()
		p.logConnectionClose(clientIP, clientPort, targetHost, parsePort(targetPort), stats, err.Error())
		return
	}
	defer targetConn.Close()

	// Set timeouts if configured
	if p.config.TCP != nil {
		if p.config.TCP.ReadTimeout > 0 {
			clientConn.SetReadDeadline(time.Now().Add(p.config.TCP.ReadTimeout))
			targetConn.SetReadDeadline(time.Now().Add(p.config.TCP.ReadTimeout))
		}
		if p.config.TCP.WriteTimeout > 0 {
			clientConn.SetWriteDeadline(time.Now().Add(p.config.TCP.WriteTimeout))
			targetConn.SetWriteDeadline(time.Now().Add(p.config.TCP.WriteTimeout))
		}
	}

	// Bidirectional copy
	errChan := make(chan error, 2)

	// Client to target
	go func() {
		written, err := p.copyWithStats(targetConn, clientConn, &stats.bytesSent, clientIP)
		if err != nil && err != io.EOF {
			errChan <- fmt.Errorf("client->target: %w", err)
		} else {
			errChan <- nil
		}
		p.metrics.BytesTransferred.WithLabelValues(p.config.Name, "sent").Add(float64(written))
	}()

	// Target to client
	go func() {
		written, err := p.copyWithStats(clientConn, targetConn, &stats.bytesReceived, clientIP)
		if err != nil && err != io.EOF {
			errChan <- fmt.Errorf("target->client: %w", err)
		} else {
			errChan <- nil
		}
		p.metrics.BytesTransferred.WithLabelValues(p.config.Name, "received").Add(float64(written))
	}()

	// Wait for both directions to complete
	err1 := <-errChan
	err2 := <-errChan

	var errMsg string
	if err1 != nil {
		errMsg = err1.Error()
	} else if err2 != nil {
		errMsg = err2.Error()
	}

	// Log connection close
	p.logConnectionClose(clientIP, clientPort, targetHost, parsePort(targetPort), stats, errMsg)

	// Record duration
	duration := time.Since(stats.startTime)
	p.metrics.ConnectionDuration.WithLabelValues(p.config.Name, "tcp").Observe(duration.Seconds())
}

// copyWithStats copies data and tracks bandwidth limits
func (p *TCPProxy) copyWithStats(dst, src net.Conn, counter *int64, clientIP string) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64

	for {
		nr, err := src.Read(buf)
		if nr > 0 {
			// Check bandwidth limit
			allowed := p.rateLimiter.AllowBandwidth(clientIP, int64(nr))

			// Log if over limit (works for all modes)
			if p.rateLimiter.IsBandwidthOverLimit(clientIP, int64(nr)) {
				action := p.rateLimiter.GetAction()
				if action == "log_only" {
					p.logger.LogWarning("Bandwidth limit exceeded (log_only mode)", map[string]interface{}{
						"listener":  p.config.Name,
						"client_ip": clientIP,
						"bytes":     nr,
					})
				} else if !allowed {
					p.logger.LogInfo("Connection dropped: bandwidth limit exceeded", map[string]interface{}{
						"listener":  p.config.Name,
						"client_ip": clientIP,
						"bytes":     nr,
						"action":    action,
					})
				}
			}

			if !allowed {
				p.metrics.RateLimitDrops.WithLabelValues(p.config.Name, "bandwidth_limit").Inc()
				return written, fmt.Errorf("bandwidth limit exceeded")
			}

			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
				atomic.AddInt64(counter, int64(nw))
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}

			// Update read deadline on activity
			if p.config.TCP != nil && p.config.TCP.IdleTimeout > 0 {
				src.SetReadDeadline(time.Now().Add(p.config.TCP.IdleTimeout))
				dst.SetReadDeadline(time.Now().Add(p.config.TCP.IdleTimeout))
			}
		}
		if err != nil {
			if err != io.EOF {
				return written, err
			}
			break
		}
	}

	return written, nil
}

// logConnectionClose logs the connection close event
func (p *TCPProxy) logConnectionClose(clientIP string, clientPort int, targetIP string, targetPort int, stats *connStats, errMsg string) {
	duration := time.Since(stats.startTime)

	p.logger.LogConnection(logging.ConnectionEvent{
		Timestamp:     time.Now(),
		ListenerName:  p.config.Name,
		Protocol:      "tcp",
		SourceIP:      clientIP,
		SourcePort:    clientPort,
		TargetIP:      targetIP,
		TargetPort:    targetPort,
		EventType:     "close",
		BytesSent:     atomic.LoadInt64(&stats.bytesSent),
		BytesReceived: atomic.LoadInt64(&stats.bytesReceived),
		Duration:      duration.Milliseconds(),
		Error:         errMsg,
	})
}

// parsePort converts a port string to int
func parsePort(portStr string) int {
	_, port, err := net.SplitHostPort(":" + portStr)
	if err != nil {
		return 0
	}
	var p int
	fmt.Sscanf(port, "%d", &p)
	return p
}
