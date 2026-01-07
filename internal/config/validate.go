package config

import (
	"fmt"
	"net"
	"strings"
)

// Validate validates the entire configuration
func (c *Config) Validate() error {
	// Validate server config
	if c.Server.Name == "" {
		return fmt.Errorf("server.name is required")
	}

	// Validate logging config
	if err := c.Logging.Validate(); err != nil {
		return fmt.Errorf("logging config: %w", err)
	}

	// Validate metrics config
	if err := c.Metrics.Validate(); err != nil {
		return fmt.Errorf("metrics config: %w", err)
	}

	// Validate listeners
	if len(c.Listeners) == 0 {
		return fmt.Errorf("at least one listener is required")
	}

	listenerNames := make(map[string]bool)
	listenerAddrs := make(map[string]bool)

	for i, listener := range c.Listeners {
		if err := listener.Validate(); err != nil {
			return fmt.Errorf("listener[%d] (%s): %w", i, listener.Name, err)
		}

		// Check for duplicate names
		if listenerNames[listener.Name] {
			return fmt.Errorf("duplicate listener name: %s", listener.Name)
		}
		listenerNames[listener.Name] = true

		// Check for duplicate listen addresses
		if listenerAddrs[listener.ListenAddress] {
			return fmt.Errorf("duplicate listen address: %s", listener.ListenAddress)
		}
		listenerAddrs[listener.ListenAddress] = true
	}

	return nil
}

// Validate validates the logging configuration
func (l *LoggingConfig) Validate() error {
	if l.Syslog.Enabled {
		if err := l.Syslog.Validate(); err != nil {
			return fmt.Errorf("syslog: %w", err)
		}
	}

	if l.JSONLog.Enabled {
		if err := l.JSONLog.Validate(); err != nil {
			return fmt.Errorf("jsonlog: %w", err)
		}
	}

	if !l.Syslog.Enabled && !l.JSONLog.Enabled && !l.Stdout.Enabled {
		return fmt.Errorf("at least one logging method must be enabled")
	}

	return nil
}

// Validate validates the syslog configuration
func (s *SyslogConfig) Validate() error {
	if s.Network != "" && s.Network != "udp" && s.Network != "tcp" && s.Network != "unix" {
		return fmt.Errorf("invalid network type: %s (must be udp, tcp, or unix)", s.Network)
	}

	if s.Address == "" {
		return fmt.Errorf("address is required when syslog is enabled")
	}

	validPriorities := map[string]bool{
		"debug": true, "info": true, "warning": true, "error": true,
	}
	if s.Priority != "" && !validPriorities[strings.ToLower(s.Priority)] {
		return fmt.Errorf("invalid priority: %s (must be debug, info, warning, or error)", s.Priority)
	}

	return nil
}

// Validate validates the JSON log configuration
func (j *JSONLogConfig) Validate() error {
	if j.Path == "" {
		return fmt.Errorf("path is required when JSON logging is enabled")
	}
	return nil
}

// Validate validates the metrics configuration
func (m *MetricsConfig) Validate() error {
	if m.Prometheus.Enabled {
		if err := m.Prometheus.Validate(); err != nil {
			return fmt.Errorf("prometheus: %w", err)
		}
	}
	return nil
}

// Validate validates the Prometheus configuration
func (p *PrometheusConfig) Validate() error {
	if p.ListenAddress == "" {
		return fmt.Errorf("listen_address is required when Prometheus is enabled")
	}

	if p.Path == "" {
		return fmt.Errorf("path is required when Prometheus is enabled")
	}

	if !strings.HasPrefix(p.Path, "/") {
		return fmt.Errorf("path must start with /")
	}

	return nil
}

// Validate validates the listener configuration
func (l *ListenerConfig) Validate() error {
	if l.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Validate protocol
	if err := validateProtocol(l.Protocol); err != nil {
		return err
	}

	// Validate listen address
	if l.ListenAddress == "" {
		return fmt.Errorf("listen_address is required")
	}
	if err := validateAddress(l.ListenAddress); err != nil {
		return fmt.Errorf("invalid listen_address: %w", err)
	}

	// Validate target address
	if l.TargetAddress == "" {
		return fmt.Errorf("target_address is required")
	}
	if err := validateAddress(l.TargetAddress); err != nil {
		return fmt.Errorf("invalid target_address: %w", err)
	}

	// Validate allowlist
	for i, entry := range l.Allowlist {
		if err := validateCIDROrIP(entry); err != nil {
			return fmt.Errorf("allowlist[%d]: %w", i, err)
		}
	}

	// Validate rate limits
	if err := l.RateLimits.Validate(); err != nil {
		return fmt.Errorf("rate_limits: %w", err)
	}

	// Validate protocol-specific config
	if l.Protocol == "tcp" && l.TCP != nil {
		if err := l.TCP.Validate(); err != nil {
			return fmt.Errorf("tcp config: %w", err)
		}
	}

	if l.Protocol == "udp" && l.UDP != nil {
		if err := l.UDP.Validate(); err != nil {
			return fmt.Errorf("udp config: %w", err)
		}
	}

	return nil
}

// Validate validates the rate limit configuration
func (r *RateLimitConfig) Validate() error {
	if r.MaxConnectionsPerIP < 0 {
		return fmt.Errorf("max_connections_per_ip must be non-negative")
	}

	if r.ConnectionsWindow < 0 {
		return fmt.Errorf("connections_window must be non-negative")
	}

	if r.MaxConnectionAttemptsPerIP < 0 {
		return fmt.Errorf("max_connection_attempts_per_ip must be non-negative")
	}

	if r.AttemptsWindow < 0 {
		return fmt.Errorf("attempts_window must be non-negative")
	}

	if r.MaxBandwidthPerIP != "" {
		if _, err := ParseBandwidth(r.MaxBandwidthPerIP); err != nil {
			return fmt.Errorf("invalid max_bandwidth_per_ip: %w", err)
		}
	}

	if r.BandwidthWindow < 0 {
		return fmt.Errorf("bandwidth_window must be non-negative")
	}

	if r.MaxTotalConnections < 0 {
		return fmt.Errorf("max_total_connections must be non-negative")
	}

	// Validate action mode
	if r.Action != "" {
		validActions := map[string]bool{
			"drop": true, "throttle": true, "log_only": true,
		}
		if !validActions[strings.ToLower(r.Action)] {
			return fmt.Errorf("invalid action: %s (must be drop, throttle, or log_only)", r.Action)
		}
	}

	// Validate throttle_minimum if action is throttle
	if strings.ToLower(r.Action) == "throttle" {
		if r.ThrottleMinimumBandwidth == "" {
			return fmt.Errorf("throttle_minimum is required when action is 'throttle'")
		}
		if _, err := ParseBandwidth(r.ThrottleMinimumBandwidth); err != nil {
			return fmt.Errorf("invalid throttle_minimum: %w", err)
		}
	}

	return nil
}

// Validate validates the TCP configuration
func (t *TCPConfig) Validate() error {
	if t.ReadTimeout < 0 {
		return fmt.Errorf("read_timeout must be non-negative")
	}
	if t.WriteTimeout < 0 {
		return fmt.Errorf("write_timeout must be non-negative")
	}
	if t.IdleTimeout < 0 {
		return fmt.Errorf("idle_timeout must be non-negative")
	}
	return nil
}

// Validate validates the UDP configuration
func (u *UDPConfig) Validate() error {
	if u.SessionTimeout <= 0 {
		return fmt.Errorf("session_timeout must be positive")
	}
	if u.BufferSize <= 0 {
		return fmt.Errorf("buffer_size must be positive")
	}
	if u.BufferSize > 65536 {
		return fmt.Errorf("buffer_size must not exceed 65536 bytes")
	}
	return nil
}

// validateProtocol validates the protocol string
func validateProtocol(protocol string) error {
	protocol = strings.ToLower(protocol)
	if protocol != "tcp" && protocol != "udp" {
		return fmt.Errorf("invalid protocol: %s (must be tcp or udp)", protocol)
	}
	return nil
}

// validateAddress validates an IP:port address
func validateAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address format: %w", err)
	}

	// Validate host (can be IP or hostname)
	if host != "" && host != "0.0.0.0" && host != "::" {
		ip := net.ParseIP(host)
		if ip == nil {
			// Not an IP, check if it's a valid hostname
			// For simplicity, we'll allow any non-empty string as hostname
			if host == "" {
				return fmt.Errorf("empty hostname")
			}
		}
	}

	// Validate port
	if port == "" {
		return fmt.Errorf("port is required")
	}

	return nil
}

// validateCIDROrIP validates a CIDR range or single IP address
func validateCIDROrIP(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("empty CIDR or IP")
	}

	// Try parsing as CIDR
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		if err != nil {
			return fmt.Errorf("invalid CIDR: %w", err)
		}
		return nil
	}

	// Try parsing as IP
	ip := net.ParseIP(s)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", s)
	}

	return nil
}
