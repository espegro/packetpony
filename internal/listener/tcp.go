package listener

import (
	"context"
	"errors"
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
	config        *config.ListenerConfig
	listener      net.Listener
	proxy         *proxy.TCPProxy
	logger        logging.Logger
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	rateLimiter   *ratelimit.RateLimitManager
	activeConnsMu sync.Mutex
	activeConns   []net.Conn
	stopOnce      sync.Once
	cleanupOnce   sync.Once
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
		activeConns: make([]net.Conn, 0),
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

	if err := l.Drain(); err != nil {
		return err
	}
	if err := l.Wait(); err != nil {
		return err
	}

	l.logger.LogInfo("TCP listener stopped", map[string]interface{}{
		"listener": l.config.Name,
	})

	return nil
}

// Drain stops accepting new connections while existing connections continue.
func (l *TCPListener) Drain() error {
	l.stopAccepting()
	return nil
}

// Wait blocks until the accept loop and all active connections have finished.
func (l *TCPListener) Wait() error {
	l.wg.Wait()
	l.closeRateLimiter()
	return nil
}

// ForceStop closes active connections and waits for all handlers to finish.
func (l *TCPListener) ForceStop() error {
	l.cancel()
	l.stopAccepting()
	l.closeAllConnections()
	return l.Wait()
}

func (l *TCPListener) stopAccepting() {
	l.stopOnce.Do(func() {
		if l.listener != nil {
			l.listener.Close()
		}
	})
}

func (l *TCPListener) closeRateLimiter() {
	l.cleanupOnce.Do(l.rateLimiter.Close)
}

// trackConnection adds a connection to the active connections list
func (l *TCPListener) trackConnection(conn net.Conn) {
	l.activeConnsMu.Lock()
	defer l.activeConnsMu.Unlock()
	l.activeConns = append(l.activeConns, conn)
}

// untrackConnection removes a connection from the active connections list
func (l *TCPListener) untrackConnection(conn net.Conn) {
	l.activeConnsMu.Lock()
	defer l.activeConnsMu.Unlock()
	for i, c := range l.activeConns {
		if c == conn {
			l.activeConns = append(l.activeConns[:i], l.activeConns[i+1:]...)
			break
		}
	}
}

// closeAllConnections closes all active connections
func (l *TCPListener) closeAllConnections() {
	l.activeConnsMu.Lock()
	defer l.activeConnsMu.Unlock()

	for _, conn := range l.activeConns {
		conn.Close()
	}
	l.activeConns = nil
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
			if errors.Is(err, net.ErrClosed) {
				return
			}
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

		// Track connection
		l.trackConnection(conn)

		// Handle connection in a new goroutine
		l.wg.Add(1)
		go func(c net.Conn) {
			defer l.wg.Done()
			defer l.untrackConnection(c)
			l.proxy.HandleConnection(l.ctx, c)
		}(conn)
	}
}
