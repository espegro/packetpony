// Package listener provides TCP and UDP listener management with lifecycle control.
package listener

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/espegro/packetpony/internal/config"
	"github.com/espegro/packetpony/internal/logging"
	"github.com/espegro/packetpony/internal/metrics"
)

// Listener defines the lifecycle shared by TCP and UDP listeners.
type Listener interface {
	Start() error
	Drain() error
	Wait() error
	Stop() error
	ForceStop() error
	Name() string
}

type managedListener struct {
	listener Listener
	config   config.ListenerConfig
}

// Manager manages active and draining listeners.
type Manager struct {
	mu        sync.Mutex
	reloadMu  sync.Mutex
	listeners map[string]*managedListener
	retired   map[Listener]struct{}
	shutting  bool
	logger    logging.Logger
	metrics   *metrics.ProxyMetrics
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewManager creates a new listener manager.
func NewManager(cfg *config.Config, logger logging.Logger, metricsCollector *metrics.ProxyMetrics) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		listeners: make(map[string]*managedListener),
		retired:   make(map[Listener]struct{}),
		logger:    logger,
		metrics:   metricsCollector,
		ctx:       ctx,
		cancel:    cancel,
	}

	for i := range cfg.Listeners {
		managed, err := manager.newManagedListener(cfg.Listeners[i])
		if err != nil {
			manager.closeManagedListeners(manager.listeners)
			cancel()
			return nil, err
		}
		manager.listeners[managed.config.Name] = managed
	}

	return manager, nil
}

func (m *Manager) newManagedListener(listenerConfig config.ListenerConfig) (*managedListener, error) {
	managed := &managedListener{config: listenerConfig}

	var err error
	switch strings.ToLower(managed.config.Protocol) {
	case "tcp":
		managed.listener, err = NewTCPListener(m.ctx, &managed.config, m.logger, m.metrics)
	case "udp":
		managed.listener, err = NewUDPListener(m.ctx, &managed.config, m.logger, m.metrics)
	default:
		err = fmt.Errorf("unsupported protocol %s", managed.config.Protocol)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create listener %s: %w", managed.config.Name, err)
	}
	return managed, nil
}

// Start starts all configured listeners.
func (m *Manager) Start() error {
	m.mu.Lock()
	listeners := cloneManagedMap(m.listeners)
	m.mu.Unlock()

	m.logger.LogInfo("Starting all listeners", map[string]interface{}{"count": len(listeners)})
	var started []*managedListener
	for name, managed := range listeners {
		if err := managed.listener.Start(); err != nil {
			for _, active := range started {
				_ = active.listener.ForceStop()
			}
			for _, candidate := range listeners {
				if !containsManaged(started, candidate) {
					_ = candidate.listener.ForceStop()
				}
			}
			return fmt.Errorf("failed to start listener %s: %w", name, err)
		}
		started = append(started, managed)
	}

	m.logger.LogInfo("All listeners started successfully", map[string]interface{}{"count": len(listeners)})
	return nil
}

// Reload applies listener changes while allowing replaced TCP listeners to drain.
func (m *Manager) Reload(cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid reload configuration: %w", err)
	}

	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	m.mu.Lock()
	if m.shutting {
		m.mu.Unlock()
		return fmt.Errorf("cannot reload while shutting down")
	}
	current := cloneManagedMap(m.listeners)
	m.mu.Unlock()

	desired := make(map[string]config.ListenerConfig, len(cfg.Listeners))
	for i := range cfg.Listeners {
		desired[cfg.Listeners[i].Name] = cfg.Listeners[i]
	}

	candidates := make(map[string]*managedListener)
	for name, listenerConfig := range desired {
		if existing, ok := current[name]; ok && reflect.DeepEqual(existing.config, listenerConfig) {
			continue
		}
		managed, err := m.newManagedListener(listenerConfig)
		if err != nil {
			m.closeManagedListeners(candidates)
			return err
		}
		candidates[name] = managed
	}

	conflictingOld := make(map[string]*managedListener)
	for _, candidate := range candidates {
		for oldName, existing := range current {
			if bindKey(existing.config) == bindKey(candidate.config) {
				conflictingOld[oldName] = existing
				break
			}
		}
	}

	for name, candidate := range candidates {
		if hasBindConflict(candidate.config, conflictingOld) {
			continue
		}
		if err := candidate.listener.Start(); err != nil {
			m.cleanupCandidates(candidates)
			return fmt.Errorf("failed to start reloaded listener %s: %w", name, err)
		}
	}

	for name, existing := range conflictingOld {
		if err := existing.listener.Drain(); err != nil {
			m.cleanupCandidates(candidates)
			return fmt.Errorf("failed to drain listener %s for reload: %w", name, err)
		}
	}

	for name, candidate := range candidates {
		if !hasBindConflict(candidate.config, conflictingOld) {
			continue
		}
		if err := candidate.listener.Start(); err != nil {
			m.cleanupCandidates(candidates)
			rollbackErr := m.rollbackDrained(current, conflictingOld)
			if rollbackErr != nil {
				return fmt.Errorf("failed to start reloaded listener %s: %w; rollback failed: %v", name, err, rollbackErr)
			}
			return fmt.Errorf("failed to start reloaded listener %s: %w", name, err)
		}
	}

	next := make(map[string]*managedListener, len(desired))
	for name := range desired {
		if candidate, ok := candidates[name]; ok {
			next[name] = candidate
		} else {
			next[name] = current[name]
		}
	}

	m.mu.Lock()
	m.listeners = next
	m.mu.Unlock()

	for name, existing := range current {
		if next[name] == existing {
			continue
		}
		if _, alreadyDraining := conflictingOld[name]; !alreadyDraining {
			if err := existing.listener.Drain(); err != nil {
				m.logger.LogError("Failed to drain replaced listener", map[string]interface{}{
					"listener": name,
					"error":    err.Error(),
				})
			}
		}
		m.retire(existing.listener)
	}

	m.logger.LogInfo("Configuration reload completed", map[string]interface{}{
		"active_listeners": len(next),
		"changed":          len(candidates),
	})
	return nil
}

func (m *Manager) rollbackDrained(current, drained map[string]*managedListener) error {
	replacements := make(map[string]*managedListener, len(drained))
	for name, existing := range drained {
		replacement, err := m.newManagedListener(existing.config)
		if err != nil {
			m.closeManagedListeners(replacements)
			return err
		}
		if err := replacement.listener.Start(); err != nil {
			_ = replacement.listener.ForceStop()
			m.closeManagedListeners(replacements)
			return fmt.Errorf("failed to restore listener %s: %w", name, err)
		}
		replacements[name] = replacement
	}

	m.mu.Lock()
	for name, replacement := range replacements {
		m.listeners[name] = replacement
	}
	m.mu.Unlock()

	for name, existing := range drained {
		current[name] = replacements[name]
		m.retire(existing.listener)
	}
	return nil
}

func (m *Manager) cleanupCandidates(candidates map[string]*managedListener) {
	for _, candidate := range candidates {
		_ = candidate.listener.ForceStop()
	}
}

func (m *Manager) closeManagedListeners(listeners map[string]*managedListener) {
	for _, managed := range listeners {
		_ = managed.listener.ForceStop()
	}
}

func (m *Manager) retire(listener Listener) {
	m.mu.Lock()
	m.retired[listener] = struct{}{}
	m.mu.Unlock()

	go func() {
		if err := listener.Wait(); err != nil {
			m.logger.LogError("Draining listener failed", map[string]interface{}{
				"listener": listener.Name(),
				"error":    err.Error(),
			})
		}
		m.mu.Lock()
		delete(m.retired, listener)
		m.mu.Unlock()
	}()
}

// Stop drains all active listeners and waits indefinitely.
func (m *Manager) Stop() error {
	listeners := m.beginShutdown()
	if err := drainAll(listeners); err != nil {
		return err
	}
	err := waitAll(listeners)
	m.cancel()
	return err
}

// WaitForShutdown blocks until shutdown is requested.
func (m *Manager) WaitForShutdown() {
	<-m.ctx.Done()
}

// GracefulShutdown drains listeners and force-closes remaining connections after timeout.
func (m *Manager) GracefulShutdown(timeout time.Duration) error {
	m.logger.LogInfo("Starting graceful shutdown", map[string]interface{}{"timeout": timeout.String()})
	listeners := m.beginShutdown()
	if err := drainAll(listeners); err != nil {
		return fmt.Errorf("error starting shutdown: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- waitAll(listeners)
	}()

	select {
	case err := <-done:
		m.cancel()
		if err != nil {
			return fmt.Errorf("error during shutdown: %w", err)
		}
		m.logger.LogInfo("Graceful shutdown completed", nil)
		return nil
	case <-time.After(timeout):
		m.logger.LogWarning("Graceful shutdown timeout exceeded", map[string]interface{}{"timeout": timeout.String()})
		for _, listener := range listeners {
			_ = listener.ForceStop()
		}
		m.cancel()
		<-done
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

func (m *Manager) beginShutdown() []Listener {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutting = true

	listeners := make([]Listener, 0, len(m.listeners)+len(m.retired))
	seen := make(map[Listener]struct{})
	for _, managed := range m.listeners {
		listeners = append(listeners, managed.listener)
		seen[managed.listener] = struct{}{}
	}
	for listener := range m.retired {
		if _, exists := seen[listener]; !exists {
			listeners = append(listeners, listener)
		}
	}
	return listeners
}

func drainAll(listeners []Listener) error {
	var lastErr error
	for _, listener := range listeners {
		if err := listener.Drain(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func waitAll(listeners []Listener) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(listeners))
	for _, listener := range listeners {
		wg.Add(1)
		go func(listener Listener) {
			defer wg.Done()
			if err := listener.Wait(); err != nil {
				errs <- err
			}
		}(listener)
	}
	wg.Wait()
	close(errs)

	var lastErr error
	for err := range errs {
		lastErr = err
	}
	return lastErr
}

func cloneManagedMap(source map[string]*managedListener) map[string]*managedListener {
	clone := make(map[string]*managedListener, len(source))
	for name, listener := range source {
		clone[name] = listener
	}
	return clone
}

func bindKey(listenerConfig config.ListenerConfig) string {
	return strings.ToLower(listenerConfig.Protocol) + "\x00" + listenerConfig.ListenAddress
}

func hasBindConflict(listenerConfig config.ListenerConfig, conflicts map[string]*managedListener) bool {
	key := bindKey(listenerConfig)
	for _, existing := range conflicts {
		if bindKey(existing.config) == key {
			return true
		}
	}
	return false
}

func containsManaged(listeners []*managedListener, candidate *managedListener) bool {
	for _, listener := range listeners {
		if listener == candidate {
			return true
		}
	}
	return false
}
