package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// JSONLogger implements logging to a JSON file
type JSONLogger struct {
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
}

// NewJSONLogger creates a new JSON file logger
func NewJSONLogger(path string) (*JSONLogger, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	encoder := json.NewEncoder(file)

	return &JSONLogger{
		file:    file,
		encoder: encoder,
	}, nil
}

// LogConnection logs a connection event as JSON
func (j *JSONLogger) LogConnection(event ConnectionEvent) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if err := j.encoder.Encode(event); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write connection event to JSON log: %v\n", err)
	}
}

// LogError logs an error message as JSON
func (j *JSONLogger) LogError(msg string, fields map[string]interface{}) {
	j.logMessage("error", msg, fields)
}

// LogInfo logs an informational message as JSON
func (j *JSONLogger) LogInfo(msg string, fields map[string]interface{}) {
	j.logMessage("info", msg, fields)
}

// LogWarning logs a warning message as JSON
func (j *JSONLogger) LogWarning(msg string, fields map[string]interface{}) {
	j.logMessage("warning", msg, fields)
}

// Close closes the log file
func (j *JSONLogger) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.file.Close()
}

// logMessage logs a general message as JSON
func (j *JSONLogger) logMessage(level, msg string, fields map[string]interface{}) {
	j.mu.Lock()
	defer j.mu.Unlock()

	logEntry := map[string]interface{}{
		"level":   level,
		"message": msg,
	}

	// Merge fields into log entry
	for key, value := range fields {
		logEntry[key] = value
	}

	if err := j.encoder.Encode(logEntry); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write log message to JSON log: %v\n", err)
	}
}
