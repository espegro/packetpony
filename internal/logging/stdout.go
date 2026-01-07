package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// StdoutLogger implements logging to stdout/stderr
// When running under systemd, this will automatically be captured by journald
type StdoutLogger struct {
	useJSON bool
	mu      sync.Mutex
}

// NewStdoutLogger creates a new stdout logger
func NewStdoutLogger(useJSON bool) *StdoutLogger {
	return &StdoutLogger{
		useJSON: useJSON,
	}
}

// LogConnection logs a connection event
func (s *StdoutLogger) LogConnection(event ConnectionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.useJSON {
		json.NewEncoder(os.Stdout).Encode(event)
	} else {
		s.logConnectionText(event)
	}
}

// LogError logs an error message
func (s *StdoutLogger) LogError(msg string, fields map[string]interface{}) {
	s.logMessage("ERROR", msg, fields, os.Stderr)
}

// LogInfo logs an informational message
func (s *StdoutLogger) LogInfo(msg string, fields map[string]interface{}) {
	s.logMessage("INFO", msg, fields, os.Stdout)
}

// LogWarning logs a warning message
func (s *StdoutLogger) LogWarning(msg string, fields map[string]interface{}) {
	s.logMessage("WARNING", msg, fields, os.Stderr)
}

// Close is a no-op for stdout logger
func (s *StdoutLogger) Close() error {
	return nil
}

// logConnectionText logs a connection event in text format
func (s *StdoutLogger) logConnectionText(event ConnectionEvent) {
	var msg string
	if event.EventType == "open" {
		msg = fmt.Sprintf("[%s] Connection opened: listener=%s protocol=%s src=%s:%d dst=%s:%d",
			event.Timestamp.Format("2006-01-02 15:04:05"),
			event.ListenerName,
			event.Protocol,
			event.SourceIP, event.SourcePort,
			event.TargetIP, event.TargetPort)
	} else {
		msg = fmt.Sprintf("[%s] Connection closed: listener=%s protocol=%s src=%s:%d dst=%s:%d duration=%dms bytes_sent=%d bytes_recv=%d",
			event.Timestamp.Format("2006-01-02 15:04:05"),
			event.ListenerName,
			event.Protocol,
			event.SourceIP, event.SourcePort,
			event.TargetIP, event.TargetPort,
			event.Duration,
			event.BytesSent, event.BytesReceived)

		if event.Protocol == "udp" {
			msg += fmt.Sprintf(" pkts_sent=%d pkts_recv=%d", event.PacketsSent, event.PacketsReceived)
		}

		if event.Error != "" {
			msg += fmt.Sprintf(" error=%q", event.Error)
		}
	}

	fmt.Fprintln(os.Stdout, msg)
}

// logMessage logs a general message
func (s *StdoutLogger) logMessage(level, msg string, fields map[string]interface{}, output *os.File) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.useJSON {
		logEntry := map[string]interface{}{
			"level":   level,
			"message": msg,
		}
		for key, value := range fields {
			logEntry[key] = value
		}
		json.NewEncoder(output).Encode(logEntry)
	} else {
		formatted := fmt.Sprintf("[%s] %s", level, msg)
		if len(fields) > 0 {
			formatted += " "
			for key, value := range fields {
				formatted += fmt.Sprintf("%s=%v ", key, value)
			}
		}
		fmt.Fprintln(output, formatted)
	}
}
