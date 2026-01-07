package listener

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/espegro/packetpony/internal/acl"
	"github.com/espegro/packetpony/internal/config"
	"github.com/espegro/packetpony/internal/logging"
	"github.com/espegro/packetpony/internal/metrics"
	"github.com/espegro/packetpony/internal/proxy"
	"github.com/espegro/packetpony/internal/ratelimit"
	"github.com/espegro/packetpony/internal/session"
)

// UDPListener manages a UDP listening socket and handles packets
type UDPListener struct {
	config         *config.ListenerConfig
	conn           *net.UDPConn
	proxy          *proxy.UDPProxy
	logger         logging.Logger
	sessionManager *session.SessionManager
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	rateLimiter    *ratelimit.RateLimitManager
}

// NewUDPListener creates a new UDP listener
func NewUDPListener(
	ctx context.Context,
	cfg *config.ListenerConfig,
	logger logging.Logger,
	metricsCollector *metrics.ProxyMetrics,
) (*UDPListener, error) {
	// Create allowlist
	allowlist, err := acl.NewAllowlist(cfg.Allowlist)
	if err != nil {
		return nil, fmt.Errorf("failed to create allowlist: %w", err)
	}

	// Create rate limiter
	rateLimiter := ratelimit.NewRateLimitManager(cfg.RateLimits)

	// Create session manager
	sessionTimeout := 30 * time.Second
	if cfg.UDP != nil && cfg.UDP.SessionTimeout > 0 {
		sessionTimeout = cfg.UDP.SessionTimeout
	}
	sessionManager := session.NewSessionManager(sessionTimeout)

	// Create proxy
	udpProxy := proxy.NewUDPProxy(cfg, logger, rateLimiter, allowlist, sessionManager, metricsCollector)

	// Create context with cancel
	listenerCtx, cancel := context.WithCancel(ctx)

	return &UDPListener{
		config:         cfg,
		proxy:          udpProxy,
		logger:         logger,
		sessionManager: sessionManager,
		ctx:            listenerCtx,
		cancel:         cancel,
		rateLimiter:    rateLimiter,
	}, nil
}

// Start starts the UDP listener
func (l *UDPListener) Start() error {
	addr, err := net.ResolveUDPAddr("udp", l.config.ListenAddress)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address %s: %w", l.config.ListenAddress, err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", l.config.ListenAddress, err)
	}

	l.conn = conn

	l.logger.LogInfo("UDP listener started", map[string]interface{}{
		"listener": l.config.Name,
		"address":  l.config.ListenAddress,
		"target":   l.config.TargetAddress,
	})

	// Start read loop in a goroutine
	l.wg.Add(1)
	go l.readLoop()

	return nil
}

// Stop stops the UDP listener
func (l *UDPListener) Stop() error {
	l.logger.LogInfo("Stopping UDP listener", map[string]interface{}{
		"listener": l.config.Name,
	})

	// Cancel context to signal shutdown
	l.cancel()

	// Close connection to stop reading
	if l.conn != nil {
		l.conn.Close()
	}

	// Close session manager
	l.sessionManager.Close()

	// Close rate limiter cleanup goroutines
	l.rateLimiter.Close()

	// Wait for read loop to finish
	l.wg.Wait()

	l.logger.LogInfo("UDP listener stopped", map[string]interface{}{
		"listener": l.config.Name,
	})

	return nil
}

// Name returns the listener name
func (l *UDPListener) Name() string {
	return l.config.Name
}

// readLoop reads packets from the UDP socket
func (l *UDPListener) readLoop() {
	defer l.wg.Done()

	bufferSize := 4096
	if l.config.UDP != nil && l.config.UDP.BufferSize > 0 {
		bufferSize = l.config.UDP.BufferSize
	}

	buf := make([]byte, bufferSize)

	for {
		select {
		case <-l.ctx.Done():
			return
		default:
		}

		n, srcAddr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-l.ctx.Done():
				// Shutdown requested
				return
			default:
				l.logger.LogError("UDP read error", map[string]interface{}{
					"listener": l.config.Name,
					"error":    err.Error(),
				})
				continue
			}
		}

		if n > 0 {
			// Make a copy of the data for processing
			data := make([]byte, n)
			copy(data, buf[:n])

			// Handle packet inline (UDP is fast, no need for goroutine per packet)
			l.proxy.HandlePacket(data, srcAddr, l.conn)
		}
	}
}
