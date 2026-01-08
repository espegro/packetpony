// Package listener provides TCP and UDP listener management with lifecycle control.
// The Manager coordinates multiple listeners and handles graceful shutdown.
package listener

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/espegro/packetpony/internal/config"
	"github.com/espegro/packetpony/internal/logging"
	"github.com/espegro/packetpony/internal/metrics"
)

// Listener defines the interface for all listener types
type Listener interface {
	Start() error
	Stop() error
	Name() string
}

// Manager manages all listeners
type Manager struct {
	listeners map[string]Listener
	logger    logging.Logger
	metrics   *metrics.ProxyMetrics
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewManager creates a new listener manager
func NewManager(cfg *config.Config, logger logging.Logger, metricsCollector *metrics.ProxyMetrics) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		listeners: make(map[string]Listener),
		logger:    logger,
		metrics:   metricsCollector,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Create listeners from config
	for i := range cfg.Listeners {
		listenerCfg := &cfg.Listeners[i]

		var listener Listener
		var err error

		protocol := strings.ToLower(listenerCfg.Protocol)
		switch protocol {
		case "tcp":
			listener, err = NewTCPListener(ctx, listenerCfg, logger, metricsCollector)
		case "udp":
			listener, err = NewUDPListener(ctx, listenerCfg, logger, metricsCollector)
		default:
			return nil, fmt.Errorf("unsupported protocol %s for listener %s", listenerCfg.Protocol, listenerCfg.Name)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to create listener %s: %w", listenerCfg.Name, err)
		}

		manager.listeners[listenerCfg.Name] = listener
	}

	return manager, nil
}

// Start starts all listeners
func (m *Manager) Start() error {
	m.logger.LogInfo("Starting all listeners", map[string]interface{}{
		"count": len(m.listeners),
	})

	for name, listener := range m.listeners {
		if err := listener.Start(); err != nil {
			// Stop any listeners that were already started
			m.Stop()
			return fmt.Errorf("failed to start listener %s: %w", name, err)
		}
	}

	m.logger.LogInfo("All listeners started successfully", map[string]interface{}{
		"count": len(m.listeners),
	})

	return nil
}

// Stop stops all listeners
func (m *Manager) Stop() error {
	m.logger.LogInfo("Stopping all listeners", map[string]interface{}{
		"count": len(m.listeners),
	})

	// Cancel context to signal shutdown
	m.cancel()

	// Stop all listeners
	var lastErr error
	for name, listener := range m.listeners {
		if err := listener.Stop(); err != nil {
			m.logger.LogError("Failed to stop listener", map[string]interface{}{
				"listener": name,
				"error":    err.Error(),
			})
			lastErr = err
		}
	}

	m.logger.LogInfo("All listeners stopped", nil)

	return lastErr
}

// WaitForShutdown blocks until shutdown is requested
func (m *Manager) WaitForShutdown() {
	<-m.ctx.Done()
}

// GracefulShutdown performs a graceful shutdown with a timeout
func (m *Manager) GracefulShutdown(timeout time.Duration) error {
	m.logger.LogInfo("Starting graceful shutdown", map[string]interface{}{
		"timeout": timeout.String(),
	})

	// Stop accepting new connections
	if err := m.Stop(); err != nil {
		return fmt.Errorf("error during shutdown: %w", err)
	}

	// Wait for active connections with timeout
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.LogInfo("Graceful shutdown completed", nil)
		return nil
	case <-time.After(timeout):
		m.logger.LogWarning("Graceful shutdown timeout exceeded", map[string]interface{}{
			"timeout": timeout.String(),
		})
		return fmt.Errorf("shutdown timeout exceeded")
	}
}
