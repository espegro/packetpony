package listener

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/espegro/packetpony/internal/acl"
	"github.com/espegro/packetpony/internal/config"
	"github.com/espegro/packetpony/internal/logging"
	"github.com/espegro/packetpony/internal/metrics"
	"github.com/espegro/packetpony/internal/proxy"
	"github.com/espegro/packetpony/internal/ratelimit"
)

// TCPListener manages a TCP listening socket and handles connections
type TCPListener struct {
	config      *config.ListenerConfig
	listener    net.Listener
	proxy       *proxy.TCPProxy
	logger      logging.Logger
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	rateLimiter *ratelimit.RateLimitManager
}

// NewTCPListener creates a new TCP listener
func NewTCPListener(
	ctx context.Context,
	cfg *config.ListenerConfig,
	logger logging.Logger,
	metricsCollector *metrics.ProxyMetrics,
) (*TCPListener, error) {
	// Create allowlist
	allowlist, err := acl.NewAllowlist(cfg.Allowlist)
	if err != nil {
		return nil, fmt.Errorf("failed to create allowlist: %w", err)
	}

	// Create rate limiter
	rateLimiter := ratelimit.NewRateLimitManager(cfg.RateLimits)

	// Create proxy
	tcpProxy := proxy.NewTCPProxy(cfg, logger, rateLimiter, allowlist, metricsCollector)

	// Create context with cancel
	listenerCtx, cancel := context.WithCancel(ctx)

	return &TCPListener{
		config:      cfg,
		proxy:       tcpProxy,
		logger:      logger,
		ctx:         listenerCtx,
		cancel:      cancel,
		rateLimiter: rateLimiter,
	}, nil
}

// Start starts the TCP listener
func (l *TCPListener) Start() error {
	listener, err := net.Listen("tcp", l.config.ListenAddress)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", l.config.ListenAddress, err)
	}

	l.listener = listener

	l.logger.LogInfo("TCP listener started", map[string]interface{}{
		"listener": l.config.Name,
		"address":  l.config.ListenAddress,
		"target":   l.config.TargetAddress,
	})

	// Start accept loop in a goroutine
	l.wg.Add(1)
	go l.acceptLoop()

	return nil
}

// Stop stops the TCP listener
func (l *TCPListener) Stop() error {
	l.logger.LogInfo("Stopping TCP listener", map[string]interface{}{
		"listener": l.config.Name,
	})

	// Cancel context to signal shutdown
	l.cancel()

	// Close listener to stop accepting new connections
	if l.listener != nil {
		l.listener.Close()
	}

	// Close rate limiter cleanup goroutines
	l.rateLimiter.Close()

	// Wait for all connection handlers to finish
	l.wg.Wait()

	l.logger.LogInfo("TCP listener stopped", map[string]interface{}{
		"listener": l.config.Name,
	})

	return nil
}

// Name returns the listener name
func (l *TCPListener) Name() string {
	return l.config.Name
}

// acceptLoop accepts incoming connections
func (l *TCPListener) acceptLoop() {
	defer l.wg.Done()

	for {
		conn, err := l.listener.Accept()
		if err != nil {
			select {
			case <-l.ctx.Done():
				// Shutdown requested
				return
			default:
				l.logger.LogError("Accept error", map[string]interface{}{
					"listener": l.config.Name,
					"error":    err.Error(),
				})
				continue
			}
		}

		// Handle connection in a new goroutine
		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			l.proxy.HandleConnection(conn)
		}()
	}
}
