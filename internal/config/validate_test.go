package config

import (
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: Config{
				Server: ServerConfig{Name: "test"},
				Logging: LoggingConfig{
					Stdout: StdoutConfig{Enabled: true},
				},
				Metrics: MetricsConfig{
					Prometheus: PrometheusConfig{Enabled: false},
				},
				Listeners: []ListenerConfig{
					{
						Name:          "test-listener",
						Protocol:      "tcp",
						ListenAddress: "127.0.0.1:8080",
						TargetAddress: "localhost:80",
						Allowlist:     []string{"127.0.0.1"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing server name",
			config: Config{
				Server: ServerConfig{Name: ""},
				Logging: LoggingConfig{
					Stdout: StdoutConfig{Enabled: true},
				},
				Listeners: []ListenerConfig{
					{Name: "test", Protocol: "tcp", ListenAddress: "127.0.0.1:8080", TargetAddress: "localhost:80"},
				},
			},
			wantErr: true,
			errMsg:  "server.name is required",
		},
		{
			name: "no listeners",
			config: Config{
				Server:    ServerConfig{Name: "test"},
				Logging:   LoggingConfig{Stdout: StdoutConfig{Enabled: true}},
				Listeners: []ListenerConfig{},
			},
			wantErr: true,
			errMsg:  "at least one listener is required",
		},
		{
			name: "duplicate listener names",
			config: Config{
				Server:  ServerConfig{Name: "test"},
				Logging: LoggingConfig{Stdout: StdoutConfig{Enabled: true}},
				Listeners: []ListenerConfig{
					{Name: "test", Protocol: "tcp", ListenAddress: "127.0.0.1:8080", TargetAddress: "localhost:80", Allowlist: []string{"127.0.0.1"}},
					{Name: "test", Protocol: "tcp", ListenAddress: "127.0.0.1:8081", TargetAddress: "localhost:81", Allowlist: []string{"127.0.0.1"}},
				},
			},
			wantErr: true,
			errMsg:  "duplicate listener name",
		},
		{
			name: "duplicate listen addresses",
			config: Config{
				Server:  ServerConfig{Name: "test"},
				Logging: LoggingConfig{Stdout: StdoutConfig{Enabled: true}},
				Listeners: []ListenerConfig{
					{Name: "test1", Protocol: "tcp", ListenAddress: "127.0.0.1:8080", TargetAddress: "localhost:80", Allowlist: []string{"127.0.0.1"}},
					{Name: "test2", Protocol: "tcp", ListenAddress: "127.0.0.1:8080", TargetAddress: "localhost:81", Allowlist: []string{"127.0.0.1"}},
				},
			},
			wantErr: true,
			errMsg:  "duplicate listen address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !contains(err.Error(), tt.errMsg) {
					t.Errorf("Config.Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestLoggingConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  LoggingConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "stdout enabled",
			config: LoggingConfig{
				Stdout: StdoutConfig{Enabled: true},
			},
			wantErr: false,
		},
		{
			name: "syslog enabled",
			config: LoggingConfig{
				Syslog: SyslogConfig{
					Enabled: true,
					Network: "udp",
					Address: "localhost:514",
				},
			},
			wantErr: false,
		},
		{
			name: "no logging enabled",
			config: LoggingConfig{
				Stdout:  StdoutConfig{Enabled: false},
				Syslog:  SyslogConfig{Enabled: false},
				JSONLog: JSONLogConfig{Enabled: false},
			},
			wantErr: true,
			errMsg:  "at least one logging method must be enabled",
		},
		{
			name: "invalid syslog network",
			config: LoggingConfig{
				Syslog: SyslogConfig{
					Enabled: true,
					Network: "invalid",
					Address: "localhost:514",
				},
			},
			wantErr: true,
			errMsg:  "invalid network type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("LoggingConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("LoggingConfig.Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestListenerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ListenerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid TCP listener",
			config: ListenerConfig{
				Name:          "test",
				Protocol:      "tcp",
				ListenAddress: "127.0.0.1:8080",
				TargetAddress: "localhost:80",
				Allowlist:     []string{"127.0.0.1"},
			},
			wantErr: false,
		},
		{
			name: "valid UDP listener",
			config: ListenerConfig{
				Name:          "test",
				Protocol:      "udp",
				ListenAddress: "0.0.0.0:53",
				TargetAddress: "8.8.8.8:53",
				Allowlist:     []string{"0.0.0.0/0"},
				UDP: &UDPConfig{
					SessionTimeout: 30 * time.Second,
					BufferSize:     4096,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid protocol",
			config: ListenerConfig{
				Name:          "test",
				Protocol:      "sctp",
				ListenAddress: "127.0.0.1:8080",
				TargetAddress: "localhost:80",
			},
			wantErr: true,
			errMsg:  "invalid protocol",
		},
		{
			name: "missing name",
			config: ListenerConfig{
				Name:          "",
				Protocol:      "tcp",
				ListenAddress: "127.0.0.1:8080",
				TargetAddress: "localhost:80",
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "invalid listen address",
			config: ListenerConfig{
				Name:          "test",
				Protocol:      "tcp",
				ListenAddress: "invalid",
				TargetAddress: "localhost:80",
			},
			wantErr: true,
			errMsg:  "invalid listen_address",
		},
		{
			name: "invalid allowlist entry",
			config: ListenerConfig{
				Name:          "test",
				Protocol:      "tcp",
				ListenAddress: "127.0.0.1:8080",
				TargetAddress: "localhost:80",
				Allowlist:     []string{"invalid-ip"},
			},
			wantErr: true,
			errMsg:  "allowlist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ListenerConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("ListenerConfig.Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestRateLimitConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  RateLimitConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid drop mode",
			config: RateLimitConfig{
				MaxConnectionsPerIP: 10,
				ConnectionsWindow:   1 * time.Minute,
				Action:              "drop",
			},
			wantErr: false,
		},
		{
			name: "valid throttle mode",
			config: RateLimitConfig{
				MaxBandwidthPerIP:        "10MB",
				BandwidthWindow:          1 * time.Minute,
				Action:                   "throttle",
				ThrottleMinimumBandwidth: "1MB",
			},
			wantErr: false,
		},
		{
			name: "throttle without minimum",
			config: RateLimitConfig{
				Action: "throttle",
			},
			wantErr: true,
			errMsg:  "throttle_minimum is required",
		},
		{
			name: "invalid action",
			config: RateLimitConfig{
				Action: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid action",
		},
		{
			name: "negative max connections",
			config: RateLimitConfig{
				MaxConnectionsPerIP: -1,
			},
			wantErr: true,
			errMsg:  "max_connections_per_ip must be non-negative",
		},
		{
			name: "invalid bandwidth format",
			config: RateLimitConfig{
				MaxBandwidthPerIP: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid max_bandwidth_per_ip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("RateLimitConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("RateLimitConfig.Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestTCPConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TCPConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: TCPConfig{
				ReadTimeout:    30 * time.Second,
				WriteTimeout:   30 * time.Second,
				DialTimeout:    10 * time.Second,
				CopyBufferSize: 32 * 1024,
			},
			wantErr: false,
		},
		{
			name: "negative timeout",
			config: TCPConfig{
				ReadTimeout: -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "read_timeout must be non-negative",
		},
		{
			name: "buffer size too large",
			config: TCPConfig{
				CopyBufferSize: 2 * 1024 * 1024,
			},
			wantErr: true,
			errMsg:  "copy_buffer_size must not exceed 1MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("TCPConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("TCPConfig.Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestUDPConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  UDPConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: UDPConfig{
				SessionTimeout: 30 * time.Second,
				BufferSize:     4096,
			},
			wantErr: false,
		},
		{
			name: "zero session timeout",
			config: UDPConfig{
				SessionTimeout: 0,
				BufferSize:     4096,
			},
			wantErr: true,
			errMsg:  "session_timeout must be positive",
		},
		{
			name: "buffer size too large",
			config: UDPConfig{
				SessionTimeout: 30 * time.Second,
				BufferSize:     70000,
			},
			wantErr: true,
			errMsg:  "buffer_size must not exceed 65536",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("UDPConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("UDPConfig.Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidateCIDROrIP(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid IPv4", "192.168.1.1", false},
		{"valid IPv6", "2001:db8::1", false},
		{"valid CIDR IPv4", "192.168.1.0/24", false},
		{"valid CIDR IPv6", "2001:db8::/32", false},
		{"invalid IP", "999.999.999.999", true},
		{"invalid CIDR", "192.168.1.0/99", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCIDROrIP(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCIDROrIP(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid IP:port", "127.0.0.1:8080", false},
		{"valid hostname:port", "localhost:80", false},
		{"valid wildcard IPv4", "0.0.0.0:8080", false},
		{"valid wildcard IPv6", "[::]:8080", false},
		{"valid IPv6", "[2001:db8::1]:8080", false},
		{"missing port", "127.0.0.1", true},
		{"invalid format", "invalid", true},
		{"empty port", "127.0.0.1:", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAddress(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAddress(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
