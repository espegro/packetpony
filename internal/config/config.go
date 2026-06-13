// Package config provides configuration structures and parsing for PacketPony.
// It supports YAML-based configuration with validation and sensible defaults.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
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
	Name            string        `yaml:"name"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"` // Graceful shutdown timeout (default: 30s)
}

// LoggingConfig defines logging backends and their configuration.
type LoggingConfig struct {
	Syslog  SyslogConfig  `yaml:"syslog"`
	JSONLog JSONLogConfig `yaml:"jsonlog"`
	Stdout  StdoutConfig  `yaml:"stdout"`
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
	ConnectionsWindow          time.Duration `yaml:"connections_window"` // Deprecated: max_connections_per_ip is now a concurrent-connection limit; this field is ignored.
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
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	IdleTimeout    time.Duration `yaml:"idle_timeout"`
	DialTimeout    time.Duration `yaml:"dial_timeout"`     // Target connection timeout (default: 10s)
	CopyBufferSize int           `yaml:"copy_buffer_size"` // Buffer size for io.Copy (default: 32KB)
}

// UDPConfig contains UDP-specific session management and logging options.
type UDPConfig struct {
	SessionTimeout time.Duration     `yaml:"session_timeout"`
	BufferSize     int               `yaml:"buffer_size"`
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

// LoadConfig reads the main YAML configuration and an optional sibling config.d directory.
func LoadConfig(path string) (*Config, error) {
	return LoadConfigWithDir(path, filepath.Join(filepath.Dir(path), "config.d"))
}

// LoadConfigWithDir reads the main YAML configuration and listener fragments from configDir.
// Missing config directories are ignored. Fragment files are applied in lexical filename order.
func LoadConfigWithDir(path, configDir string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	if err := loadListenerFragments(&config, configDir); err != nil {
		return nil, err
	}

	if err := applyDefaults(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func loadListenerFragments(config *Config, configDir string) error {
	if configDir == "" {
		return nil
	}

	entries, err := os.ReadDir(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read config directory %s: %w", configDir, err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension == ".yaml" || extension == ".yml" {
			paths = append(paths, filepath.Join(configDir, entry.Name()))
		}
	}
	sort.Strings(paths)

	listenerIndexes := make(map[string]int, len(config.Listeners))
	for i := range config.Listeners {
		if config.Listeners[i].Name != "" {
			listenerIndexes[config.Listeners[i].Name] = i
		}
	}

	for _, fragmentPath := range paths {
		listeners, err := readListenerFragment(fragmentPath)
		if err != nil {
			return err
		}
		for _, listener := range listeners {
			if listener.Name == "" {
				return fmt.Errorf("config fragment %s: listener name is required", fragmentPath)
			}
			if index, exists := listenerIndexes[listener.Name]; exists {
				config.Listeners[index] = listener
				continue
			}
			listenerIndexes[listener.Name] = len(config.Listeners)
			config.Listeners = append(config.Listeners, listener)
		}
	}

	return nil
}

func readListenerFragment(path string) ([]ListenerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config fragment %s: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse config fragment %s: %w", path, err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config fragment %s must contain a YAML mapping", path)
	}

	mapping := root.Content[0]
	if mappingHasKey(mapping, "listeners") {
		var fragment struct {
			Listeners []ListenerConfig `yaml:"listeners"`
		}
		if err := decodeKnownFields(data, &fragment); err != nil {
			return nil, fmt.Errorf("failed to parse config fragment %s: %w", path, err)
		}
		if len(fragment.Listeners) == 0 {
			return nil, fmt.Errorf("config fragment %s contains no listeners", path)
		}
		return fragment.Listeners, nil
	}

	var listener ListenerConfig
	if err := decodeKnownFields(data, &listener); err != nil {
		return nil, fmt.Errorf("failed to parse config fragment %s: %w", path, err)
	}
	return []ListenerConfig{listener}, nil
}

func mappingHasKey(mapping *yaml.Node, key string) bool {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return true
		}
	}
	return false
}

func decodeKnownFields(data []byte, value interface{}) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	return decoder.Decode(value)
}

func applyDefaults(config *Config) error {
	// Set server defaults
	if config.Server.ShutdownTimeout == 0 {
		config.Server.ShutdownTimeout = 30 * time.Second
	}

	// Parse bandwidth strings and set defaults for each listener
	for i := range config.Listeners {
		listener := &config.Listeners[i]

		// Normalize the protocol so validation and dispatch are case-insensitive.
		listener.Protocol = strings.ToLower(strings.TrimSpace(listener.Protocol))

		if config.Listeners[i].RateLimits.MaxBandwidthPerIP != "" {
			bytes, err := ParseBandwidth(config.Listeners[i].RateLimits.MaxBandwidthPerIP)
			if err != nil {
				return fmt.Errorf("listener %s: %w", config.Listeners[i].Name, err)
			}
			config.Listeners[i].RateLimits.maxBandwidthBytes = bytes
		}
		if config.Listeners[i].RateLimits.ThrottleMinimumBandwidth != "" {
			bytes, err := ParseBandwidth(config.Listeners[i].RateLimits.ThrottleMinimumBandwidth)
			if err != nil {
				return fmt.Errorf("listener %s throttle_minimum: %w", config.Listeners[i].Name, err)
			}
			config.Listeners[i].RateLimits.throttleMinimumBytes = bytes
		}

		// Set TCP defaults
		if config.Listeners[i].TCP != nil {
			if config.Listeners[i].TCP.DialTimeout == 0 {
				config.Listeners[i].TCP.DialTimeout = 10 * time.Second
			}
			if config.Listeners[i].TCP.CopyBufferSize == 0 {
				config.Listeners[i].TCP.CopyBufferSize = 32 * 1024 // 32KB
			}
		}

		// Set UDP defaults even when the optional udp block is omitted.
		if strings.EqualFold(listener.Protocol, "udp") {
			if listener.UDP == nil {
				listener.UDP = &UDPConfig{}
			}
			if listener.UDP.SessionTimeout == 0 {
				listener.UDP.SessionTimeout = 30 * time.Second
			}
			if listener.UDP.BufferSize == 0 {
				listener.UDP.BufferSize = 4096
			}
		}

		// Set UDP logging defaults and parse bandwidth values
		if listener.UDP != nil {
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
					return fmt.Errorf("listener %s UDP logging periodic_log_bytes: %w", config.Listeners[i].Name, err)
				}
				config.Listeners[i].UDP.Logging.periodicLogBytesValue = bytes
			}

			// Parse min log bytes
			if config.Listeners[i].UDP.Logging.MinLogBytes != "" && config.Listeners[i].UDP.Logging.MinLogBytes != "0" {
				bytes, err := ParseBandwidth(config.Listeners[i].UDP.Logging.MinLogBytes)
				if err != nil {
					return fmt.Errorf("listener %s UDP logging min_log_bytes: %w", config.Listeners[i].Name, err)
				}
				config.Listeners[i].UDP.Logging.minLogBytesValue = bytes
			}
		}
	}

	return nil
}

// ValidateReload rejects process-wide changes that require a restart.
func ValidateReload(current, next *Config) error {
	if current.Server.Name != next.Server.Name {
		return fmt.Errorf("server.name cannot be changed during reload")
	}
	if !reflect.DeepEqual(current.Logging, next.Logging) {
		return fmt.Errorf("logging configuration cannot be changed during reload")
	}
	if !reflect.DeepEqual(current.Metrics, next.Metrics) {
		return fmt.Errorf("metrics configuration cannot be changed during reload")
	}
	return nil
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
	var multiplier int64

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
