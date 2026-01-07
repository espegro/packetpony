package logging

import (
	"fmt"
	"log/syslog"
	"strings"

	"github.com/espegro/packetpony/internal/config"
)

// SyslogLogger implements logging to syslog
type SyslogLogger struct {
	writer   *syslog.Writer
	tag      string
	priority syslog.Priority
}

// NewSyslogLogger creates a new syslog logger
func NewSyslogLogger(cfg config.SyslogConfig) (*SyslogLogger, error) {
	priority := parseSyslogPriority(cfg.Priority)

	var writer *syslog.Writer
	var err error

	// Connect to syslog
	if cfg.Network == "" || cfg.Network == "unix" {
		// Local syslog
		writer, err = syslog.New(priority|syslog.LOG_DAEMON, cfg.Tag)
	} else {
		// Remote syslog
		writer, err = syslog.Dial(cfg.Network, cfg.Address, priority|syslog.LOG_DAEMON, cfg.Tag)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to syslog: %w", err)
	}

	return &SyslogLogger{
		writer:   writer,
		tag:      cfg.Tag,
		priority: priority,
	}, nil
}

// LogConnection logs a connection event
func (s *SyslogLogger) LogConnection(event ConnectionEvent) {
	msg := s.formatConnectionEvent(event)

	switch event.EventType {
	case "open":
		s.writer.Info(msg)
	case "close":
		if event.Error != "" {
			s.writer.Warning(msg)
		} else {
			s.writer.Info(msg)
		}
	default:
		s.writer.Info(msg)
	}
}

// LogError logs an error message
func (s *SyslogLogger) LogError(msg string, fields map[string]interface{}) {
	formatted := s.formatMessage(msg, fields)
	s.writer.Err(formatted)
}

// LogInfo logs an informational message
func (s *SyslogLogger) LogInfo(msg string, fields map[string]interface{}) {
	formatted := s.formatMessage(msg, fields)
	s.writer.Info(formatted)
}

// LogWarning logs a warning message
func (s *SyslogLogger) LogWarning(msg string, fields map[string]interface{}) {
	formatted := s.formatMessage(msg, fields)
	s.writer.Warning(formatted)
}

// Close closes the syslog connection
func (s *SyslogLogger) Close() error {
	return s.writer.Close()
}

// formatConnectionEvent formats a connection event for syslog
func (s *SyslogLogger) formatConnectionEvent(event ConnectionEvent) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("listener=%s", event.ListenerName))
	parts = append(parts, fmt.Sprintf("proto=%s", event.Protocol))
	parts = append(parts, fmt.Sprintf("event=%s", event.EventType))
	parts = append(parts, fmt.Sprintf("src=%s:%d", event.SourceIP, event.SourcePort))
	parts = append(parts, fmt.Sprintf("dst=%s:%d", event.TargetIP, event.TargetPort))

	if event.EventType == "close" {
		parts = append(parts, fmt.Sprintf("duration=%dms", event.Duration))
		parts = append(parts, fmt.Sprintf("bytes_sent=%d", event.BytesSent))
		parts = append(parts, fmt.Sprintf("bytes_recv=%d", event.BytesReceived))

		if event.Protocol == "udp" {
			parts = append(parts, fmt.Sprintf("pkts_sent=%d", event.PacketsSent))
			parts = append(parts, fmt.Sprintf("pkts_recv=%d", event.PacketsReceived))
		}

		if event.Error != "" {
			parts = append(parts, fmt.Sprintf("error=%q", event.Error))
		}
	}

	return strings.Join(parts, " ")
}

// formatMessage formats a general log message
func (s *SyslogLogger) formatMessage(msg string, fields map[string]interface{}) string {
	if len(fields) == 0 {
		return msg
	}

	var parts []string
	parts = append(parts, msg)

	for key, value := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}

	return strings.Join(parts, " ")
}

// parseSyslogPriority converts a priority string to syslog.Priority
func parseSyslogPriority(priority string) syslog.Priority {
	switch strings.ToLower(priority) {
	case "debug":
		return syslog.LOG_DEBUG
	case "info":
		return syslog.LOG_INFO
	case "warning":
		return syslog.LOG_WARNING
	case "error":
		return syslog.LOG_ERR
	default:
		return syslog.LOG_INFO
	}
}
