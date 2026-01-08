// Package config provides configuration structures and parsing for PacketPony.
// It supports YAML-based configuration with validation and sensible defaults.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the top-level configuration for PacketPony.
type Config struct {
	Server    ServerConfig     `yaml:"server"`
	Logging   LoggingConfig    `yaml:"logging"`
	Metrics   MetricsConfig    `yaml:"metrics"`
	Listeners []ListenerConfig `yaml:"listeners"`
}

// ServerConfig contains server-level configuration options.
type ServerConfig struct {
	Name string `yaml:"name"`
}

// LoggingConfig defines logging backends and their configuration.
type LoggingConfig struct {
	Syslog SyslogConfig  `yaml:"syslog"`
	JSONLog JSONLogConfig `yaml:"jsonlog"`
	Stdout StdoutConfig  `yaml:"stdout"`
}

// StdoutConfig configures stdout logging (useful for systemd/journald).
type StdoutConfig struct {
	Enabled bool `yaml:"enabled"`
	UseJSON bool `yaml:"use_json"`
}

// SyslogConfig configures syslog logging backend.
type SyslogConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Network  string `yaml:"network"`
	Address  string `yaml:"address"`
	Tag      string `yaml:"tag"`
	Priority string `yaml:"priority"`
}

// JSONLogConfig configures JSON file logging.
type JSONLogConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// MetricsConfig defines metrics collection and export configuration.
type MetricsConfig struct {
	Prometheus PrometheusConfig `yaml:"prometheus"`
}

// PrometheusConfig configures the Prometheus metrics endpoint.
type PrometheusConfig struct {
	Enabled       bool   `yaml:"enabled"`
	ListenAddress string `yaml:"listen_address"`
	Path          string `yaml:"path"`
}

// ListenerConfig defines a single listener (proxy endpoint) configuration.
type ListenerConfig struct {
	Name          string          `yaml:"name"`
	Protocol      string          `yaml:"protocol"`
	ListenAddress string          `yaml:"listen_address"`
	TargetAddress string          `yaml:"target_address"`
	Allowlist     []string        `yaml:"allowlist"`
	RateLimits    RateLimitConfig `yaml:"rate_limits"`
	TCP           *TCPConfig      `yaml:"tcp,omitempty"`
	UDP           *UDPConfig      `yaml:"udp,omitempty"`
}

// RateLimitConfig defines rate limiting rules for connections and bandwidth.
// Supports three actions: drop (reject), throttle (reduce bandwidth), or log_only.
type RateLimitConfig struct {
	MaxConnectionsPerIP        int           `yaml:"max_connections_per_ip"`
	ConnectionsWindow          time.Duration `yaml:"connections_window"`
	MaxConnectionAttemptsPerIP int           `yaml:"max_connection_attempts_per_ip"`
	AttemptsWindow             time.Duration `yaml:"attempts_window"`
	MaxBandwidthPerIP          string        `yaml:"max_bandwidth_per_ip"`
	BandwidthWindow            time.Duration `yaml:"bandwidth_window"`
	MaxTotalConnections        int           `yaml:"max_total_connections"`
	Action                     string        `yaml:"action"`           // drop, throttle, log_only
	ThrottleMinimumBandwidth   string        `yaml:"throttle_minimum"` // Minimum bandwidth when throttling
	maxBandwidthBytes          int64         // parsed value
	throttleMinimumBytes       int64         // parsed value
}

// TCPConfig contains TCP-specific timeouts and options.
type TCPConfig struct {
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// UDPConfig contains UDP-specific session management and logging options.
type UDPConfig struct {
	SessionTimeout time.Duration    `yaml:"session_timeout"`
	BufferSize     int              `yaml:"buffer_size"`
	Logging        *UDPLoggingConfig `yaml:"logging,omitempty"`
}

// UDPLoggingConfig controls how UDP sessions are logged.
// Defaults: log start/close, periodic logs every 5m or 100MB, no minimum thresholds.
type UDPLoggingConfig struct {
	LogSessionStart       bool          `yaml:"log_session_start"`
	LogSessionClose       bool          `yaml:"log_session_close"`
	PeriodicLogInterval   time.Duration `yaml:"periodic_log_interval"`
	PeriodicLogBytes      string        `yaml:"periodic_log_bytes"`
	MinLogDuration        time.Duration `yaml:"min_log_duration"`
	MinLogBytes           string        `yaml:"min_log_bytes"`
	periodicLogBytesValue int64         // parsed value
	minLogBytesValue      int64         // parsed value
}

// LoadConfig reads and parses the YAML configuration file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Parse bandwidth strings and set defaults for each listener
	for i := range config.Listeners {
		if config.Listeners[i].RateLimits.MaxBandwidthPerIP != "" {
			bytes, err := ParseBandwidth(config.Listeners[i].RateLimits.MaxBandwidthPerIP)
			if err != nil {
				return nil, fmt.Errorf("listener %s: %w", config.Listeners[i].Name, err)
			}
			config.Listeners[i].RateLimits.maxBandwidthBytes = bytes
		}
		if config.Listeners[i].RateLimits.ThrottleMinimumBandwidth != "" {
			bytes, err := ParseBandwidth(config.Listeners[i].RateLimits.ThrottleMinimumBandwidth)
			if err != nil {
				return nil, fmt.Errorf("listener %s throttle_minimum: %w", config.Listeners[i].Name, err)
			}
			config.Listeners[i].RateLimits.throttleMinimumBytes = bytes
		}

		// Set UDP logging defaults and parse bandwidth values
		if config.Listeners[i].UDP != nil {
			if config.Listeners[i].UDP.Logging == nil {
				// Set defaults
				config.Listeners[i].UDP.Logging = &UDPLoggingConfig{
					LogSessionStart:     true,
					LogSessionClose:     true,
					PeriodicLogInterval: 5 * time.Minute,
					PeriodicLogBytes:    "100MB",
					MinLogDuration:      0,
					MinLogBytes:         "",
				}
			}

			// Parse periodic log bytes
			if config.Listeners[i].UDP.Logging.PeriodicLogBytes != "" {
				bytes, err := ParseBandwidth(config.Listeners[i].UDP.Logging.PeriodicLogBytes)
				if err != nil {
					return nil, fmt.Errorf("listener %s UDP logging periodic_log_bytes: %w", config.Listeners[i].Name, err)
				}
				config.Listeners[i].UDP.Logging.periodicLogBytesValue = bytes
			}

			// Parse min log bytes
			if config.Listeners[i].UDP.Logging.MinLogBytes != "" && config.Listeners[i].UDP.Logging.MinLogBytes != "0" {
				bytes, err := ParseBandwidth(config.Listeners[i].UDP.Logging.MinLogBytes)
				if err != nil {
					return nil, fmt.Errorf("listener %s UDP logging min_log_bytes: %w", config.Listeners[i].Name, err)
				}
				config.Listeners[i].UDP.Logging.minLogBytesValue = bytes
			}
		}
	}

	return &config, nil
}

// GetMaxBandwidthBytes returns the parsed bandwidth value in bytes
func (r *RateLimitConfig) GetMaxBandwidthBytes() int64 {
	return r.maxBandwidthBytes
}

// GetThrottleMinimumBytes returns the parsed throttle minimum bandwidth in bytes
func (r *RateLimitConfig) GetThrottleMinimumBytes() int64 {
	return r.throttleMinimumBytes
}

// GetPeriodicLogBytes returns the parsed periodic log bytes value
func (u *UDPLoggingConfig) GetPeriodicLogBytes() int64 {
	return u.periodicLogBytesValue
}

// GetMinLogBytes returns the parsed minimum log bytes value
func (u *UDPLoggingConfig) GetMinLogBytes() int64 {
	return u.minLogBytesValue
}

// ParseBandwidth converts a bandwidth string (e.g., "10MB", "1GB", "500KB") to bytes
func ParseBandwidth(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	// Regular expression to match number and unit
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([KMGT]?B)$`)
	matches := re.FindStringSubmatch(strings.ToUpper(s))
	if matches == nil {
		return 0, fmt.Errorf("invalid bandwidth format: %s (expected format: 10MB, 1GB, etc.)", s)
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid bandwidth value: %s", matches[1])
	}

	unit := matches[2]
	multiplier := int64(1)

	switch unit {
	case "B":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	case "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown bandwidth unit: %s", unit)
	}

	return int64(value * float64(multiplier)), nil
}
