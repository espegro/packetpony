package logging

import (
	"fmt"
	"time"

	"github.com/espegro/packetpony/internal/config"
)

// Logger defines the interface for logging connection events and messages
type Logger interface {
	LogConnection(event ConnectionEvent)
	LogError(msg string, fields map[string]interface{})
	LogInfo(msg string, fields map[string]interface{})
	LogWarning(msg string, fields map[string]interface{})
	Close() error
}

// ConnectionEvent represents a connection lifecycle event
type ConnectionEvent struct {
	Timestamp       time.Time `json:"timestamp"`
	ListenerName    string    `json:"listener_name"`
	Protocol        string    `json:"protocol"`
	SourceIP        string    `json:"source_ip"`
	SourcePort      int       `json:"source_port"`
	TargetIP        string    `json:"target_ip"`
	TargetPort      int       `json:"target_port"`
	EventType       string    `json:"event_type"` // "open", "close"
	BytesSent       int64     `json:"bytes_sent"`
	BytesReceived   int64     `json:"bytes_received"`
	PacketsSent     int64     `json:"packets_sent,omitempty"`     // UDP only
	PacketsReceived int64     `json:"packets_received,omitempty"` // UDP only
	Duration        int64     `json:"duration_ms"`                // milliseconds
	Error           string    `json:"error,omitempty"`
}

// MultiLogger supports multiple logging backends simultaneously
type MultiLogger struct {
	loggers []Logger
}

// NewMultiLogger creates a logger that writes to multiple backends
func NewMultiLogger(cfg config.LoggingConfig) (*MultiLogger, error) {
	var loggers []Logger

	// Setup syslog if enabled
	if cfg.Syslog.Enabled {
		syslogger, err := NewSyslogLogger(cfg.Syslog)
		if err != nil {
			return nil, fmt.Errorf("failed to create syslog logger: %w", err)
		}
		loggers = append(loggers, syslogger)
	}

	// Setup JSON file logging if enabled
	if cfg.JSONLog.Enabled {
		jsonLogger, err := NewJSONLogger(cfg.JSONLog.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to create JSON logger: %w", err)
		}
		loggers = append(loggers, jsonLogger)
	}

	if len(loggers) == 0 {
		return nil, fmt.Errorf("no logging backends enabled")
	}

	return &MultiLogger{
		loggers: loggers,
	}, nil
}

// LogConnection logs a connection event to all backends
func (m *MultiLogger) LogConnection(event ConnectionEvent) {
	for _, logger := range m.loggers {
		logger.LogConnection(event)
	}
}

// LogError logs an error message to all backends
func (m *MultiLogger) LogError(msg string, fields map[string]interface{}) {
	for _, logger := range m.loggers {
		logger.LogError(msg, fields)
	}
}

// LogInfo logs an informational message to all backends
func (m *MultiLogger) LogInfo(msg string, fields map[string]interface{}) {
	for _, logger := range m.loggers {
		logger.LogInfo(msg, fields)
	}
}

// LogWarning logs a warning message to all backends
func (m *MultiLogger) LogWarning(msg string, fields map[string]interface{}) {
	for _, logger := range m.loggers {
		logger.LogWarning(msg, fields)
	}
}

// Close closes all logging backends
func (m *MultiLogger) Close() error {
	var lastErr error
	for _, logger := range m.loggers {
		if err := logger.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
