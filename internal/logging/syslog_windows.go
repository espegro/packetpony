//go:build windows
// +build windows

package logging

import (
	"fmt"

	"github.com/espegro/packetpony/internal/config"
)

// SyslogLogger is a no-op on Windows (syslog not available)
type SyslogLogger struct{}

// NewSyslogLogger returns an error on Windows as syslog is not supported
func NewSyslogLogger(cfg config.SyslogConfig) (*SyslogLogger, error) {
	return nil, fmt.Errorf("syslog is not supported on Windows")
}

// LogConnection is a no-op on Windows
func (s *SyslogLogger) LogConnection(event ConnectionEvent) {}

// LogError is a no-op on Windows
func (s *SyslogLogger) LogError(msg string, fields map[string]interface{}) {}

// LogInfo is a no-op on Windows
func (s *SyslogLogger) LogInfo(msg string, fields map[string]interface{}) {}

// LogWarning is a no-op on Windows
func (s *SyslogLogger) LogWarning(msg string, fields map[string]interface{}) {}

// Close is a no-op on Windows
func (s *SyslogLogger) Close() error {
	return nil
}
