package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseBandwidth(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{
			name:  "bytes",
			input: "100B",
			want:  100,
		},
		{
			name:  "kilobytes",
			input: "10KB",
			want:  10 * 1024,
		},
		{
			name:  "megabytes",
			input: "5MB",
			want:  5 * 1024 * 1024,
		},
		{
			name:  "gigabytes",
			input: "1GB",
			want:  1024 * 1024 * 1024,
		},
		{
			name:  "terabytes",
			input: "2TB",
			want:  2 * 1024 * 1024 * 1024 * 1024,
		},
		{
			name:  "lowercase",
			input: "10mb",
			want:  10 * 1024 * 1024,
		},
		{
			name:  "with spaces",
			input: "  100 MB  ",
			want:  100 * 1024 * 1024,
		},
		{
			name:  "decimal value",
			input: "1.5MB",
			want:  int64(1.5 * 1024 * 1024),
		},
		{
			name:  "empty string",
			input: "",
			want:  0,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "no unit",
			input:   "100",
			wantErr: true,
		},
		{
			name:    "invalid unit",
			input:   "100XB",
			wantErr: true,
		},
		{
			name:    "negative value",
			input:   "-10MB",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBandwidth(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBandwidth() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseBandwidth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
server:
  name: "test-server"
  shutdown_timeout: "60s"

logging:
  stdout:
    enabled: true
    use_json: false

metrics:
  prometheus:
    enabled: true
    listen_address: ":9090"
    path: "/metrics"

listeners:
  - name: "test-listener"
    protocol: "tcp"
    listen_address: "127.0.0.1:8080"
    target_address: "localhost:80"
    allowlist:
      - "127.0.0.1"
      - "192.168.1.0/24"
    rate_limits:
      max_connections_per_ip: 10
      connections_window: "1m"
      max_bandwidth_per_ip: "10MB"
      bandwidth_window: "1m"
      action: "drop"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify server config
	if cfg.Server.Name != "test-server" {
		t.Errorf("Server.Name = %v, want %v", cfg.Server.Name, "test-server")
	}
	if cfg.Server.ShutdownTimeout != 60*time.Second {
		t.Errorf("Server.ShutdownTimeout = %v, want %v", cfg.Server.ShutdownTimeout, 60*time.Second)
	}

	// Verify listener config
	if len(cfg.Listeners) != 1 {
		t.Fatalf("Expected 1 listener, got %d", len(cfg.Listeners))
	}
	listener := cfg.Listeners[0]
	if listener.Name != "test-listener" {
		t.Errorf("Listener.Name = %v, want %v", listener.Name, "test-listener")
	}
	if listener.Protocol != "tcp" {
		t.Errorf("Listener.Protocol = %v, want %v", listener.Protocol, "tcp")
	}

	// Verify bandwidth parsing
	expectedBandwidth := int64(10 * 1024 * 1024)
	if listener.RateLimits.GetMaxBandwidthBytes() != expectedBandwidth {
		t.Errorf("RateLimits.MaxBandwidthBytes = %v, want %v",
			listener.RateLimits.GetMaxBandwidthBytes(), expectedBandwidth)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	// Minimal config without optional fields
	configContent := `
server:
  name: "test-server"

logging:
  stdout:
    enabled: true

metrics:
  prometheus:
    enabled: false

listeners:
  - name: "test-listener"
    protocol: "tcp"
    listen_address: "127.0.0.1:8080"
    target_address: "localhost:80"
    allowlist:
      - "0.0.0.0/0"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify default shutdown timeout
	if cfg.Server.ShutdownTimeout != 30*time.Second {
		t.Errorf("Server.ShutdownTimeout = %v, want default %v",
			cfg.Server.ShutdownTimeout, 30*time.Second)
	}

	// Verify TCP defaults
	listener := cfg.Listeners[0]
	if listener.TCP != nil {
		if listener.TCP.DialTimeout != 10*time.Second {
			t.Errorf("TCP.DialTimeout = %v, want default %v",
				listener.TCP.DialTimeout, 10*time.Second)
		}
		if listener.TCP.CopyBufferSize != 32*1024 {
			t.Errorf("TCP.CopyBufferSize = %v, want default %v",
				listener.TCP.CopyBufferSize, 32*1024)
		}
	}
}

func TestLoadConfig_UDPLoggingDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
server:
  name: "test-server"

logging:
  stdout:
    enabled: true

metrics:
  prometheus:
    enabled: false

listeners:
  - name: "test-udp"
    protocol: "udp"
    listen_address: "127.0.0.1:53"
    target_address: "8.8.8.8:53"
    allowlist:
      - "0.0.0.0/0"
    udp:
      session_timeout: "30s"
      buffer_size: 4096
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	listener := cfg.Listeners[0]
	if listener.UDP == nil || listener.UDP.Logging == nil {
		t.Fatal("UDP logging config should have defaults")
	}

	// Check defaults
	logging := listener.UDP.Logging
	if !logging.LogSessionStart {
		t.Error("LogSessionStart should default to true")
	}
	if !logging.LogSessionClose {
		t.Error("LogSessionClose should default to true")
	}
	if logging.PeriodicLogInterval != 5*time.Minute {
		t.Errorf("PeriodicLogInterval = %v, want %v", logging.PeriodicLogInterval, 5*time.Minute)
	}
	expectedBytes := int64(100 * 1024 * 1024)
	if logging.GetPeriodicLogBytes() != expectedBytes {
		t.Errorf("PeriodicLogBytes = %v, want %v", logging.GetPeriodicLogBytes(), expectedBytes)
	}
}

func TestLoadConfig_InvalidFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// Invalid YAML syntax
	if err := os.WriteFile(configPath, []byte("invalid: [yaml: content"), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestRateLimitConfig_GetMethods(t *testing.T) {
	cfg := RateLimitConfig{
		maxBandwidthBytes:    10 * 1024 * 1024,
		throttleMinimumBytes: 1 * 1024 * 1024,
	}

	if got := cfg.GetMaxBandwidthBytes(); got != 10*1024*1024 {
		t.Errorf("GetMaxBandwidthBytes() = %v, want %v", got, 10*1024*1024)
	}

	if got := cfg.GetThrottleMinimumBytes(); got != 1*1024*1024 {
		t.Errorf("GetThrottleMinimumBytes() = %v, want %v", got, 1*1024*1024)
	}
}

func TestUDPLoggingConfig_GetMethods(t *testing.T) {
	cfg := UDPLoggingConfig{
		periodicLogBytesValue: 100 * 1024 * 1024,
		minLogBytesValue:      1024,
	}

	if got := cfg.GetPeriodicLogBytes(); got != 100*1024*1024 {
		t.Errorf("GetPeriodicLogBytes() = %v, want %v", got, 100*1024*1024)
	}

	if got := cfg.GetMinLogBytes(); got != 1024 {
		t.Errorf("GetMinLogBytes() = %v, want %v", got, 1024)
	}
}
