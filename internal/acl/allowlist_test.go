package acl

import (
	"net"
	"testing"
)

func TestNewAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		wantErr bool
	}{
		{
			name:    "valid single IP",
			entries: []string{"192.168.1.1"},
			wantErr: false,
		},
		{
			name:    "valid CIDR",
			entries: []string{"192.168.1.0/24"},
			wantErr: false,
		},
		{
			name:    "valid IPv6",
			entries: []string{"2001:db8::1"},
			wantErr: false,
		},
		{
			name:    "valid IPv6 CIDR",
			entries: []string{"2001:db8::/32"},
			wantErr: false,
		},
		{
			name:    "multiple entries",
			entries: []string{"192.168.1.0/24", "10.0.0.1", "172.16.0.0/12"},
			wantErr: false,
		},
		{
			name:    "empty list",
			entries: []string{},
			wantErr: false,
		},
		{
			name:    "invalid IP",
			entries: []string{"999.999.999.999"},
			wantErr: true,
		},
		{
			name:    "invalid CIDR",
			entries: []string{"192.168.1.0/99"},
			wantErr: true,
		},
		{
			name:    "mixed valid and invalid",
			entries: []string{"192.168.1.1", "invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAllowlist(tt.entries)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAllowlist() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAllowlist_IsAllowed(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		testIP  string
		want    bool
	}{
		{
			name:    "IP in allowlist",
			entries: []string{"192.168.1.1"},
			testIP:  "192.168.1.1",
			want:    true,
		},
		{
			name:    "IP not in allowlist",
			entries: []string{"192.168.1.1"},
			testIP:  "192.168.1.2",
			want:    false,
		},
		{
			name:    "IP in CIDR range",
			entries: []string{"192.168.1.0/24"},
			testIP:  "192.168.1.100",
			want:    true,
		},
		{
			name:    "IP not in CIDR range",
			entries: []string{"192.168.1.0/24"},
			testIP:  "192.168.2.1",
			want:    false,
		},
		{
			name:    "wildcard IPv4",
			entries: []string{"0.0.0.0/0"},
			testIP:  "1.2.3.4",
			want:    true,
		},
		{
			name:    "wildcard IPv6",
			entries: []string{"::/0"},
			testIP:  "2001:db8::1",
			want:    true,
		},
		{
			name:    "empty allowlist denies all",
			entries: []string{},
			testIP:  "192.168.1.1",
			want:    false,
		},
		{
			name:    "multiple ranges - match first",
			entries: []string{"192.168.1.0/24", "10.0.0.0/8"},
			testIP:  "192.168.1.50",
			want:    true,
		},
		{
			name:    "multiple ranges - match second",
			entries: []string{"192.168.1.0/24", "10.0.0.0/8"},
			testIP:  "10.5.5.5",
			want:    true,
		},
		{
			name:    "multiple ranges - no match",
			entries: []string{"192.168.1.0/24", "10.0.0.0/8"},
			testIP:  "172.16.0.1",
			want:    false,
		},
		{
			name:    "localhost",
			entries: []string{"127.0.0.1"},
			testIP:  "127.0.0.1",
			want:    true,
		},
		{
			name:    "IPv6 address in range",
			entries: []string{"2001:db8::/32"},
			testIP:  "2001:db8::1234",
			want:    true,
		},
		{
			name:    "IPv6 address not in range",
			entries: []string{"2001:db8::/32"},
			testIP:  "2001:db9::1",
			want:    false,
		},
		{
			name:    "private network ranges",
			entries: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			testIP:  "192.168.50.100",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowlist, err := NewAllowlist(tt.entries)
			if err != nil {
				t.Fatalf("Failed to create allowlist: %v", err)
			}

			ip := net.ParseIP(tt.testIP)
			if ip == nil {
				t.Fatalf("Invalid test IP: %s", tt.testIP)
			}

			got := allowlist.IsAllowed(ip)
			if got != tt.want {
				t.Errorf("IsAllowed(%s) = %v, want %v", tt.testIP, got, tt.want)
			}
		})
	}
}

func TestParseCIDROrIP(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantNet string // Expected network in CIDR notation
	}{
		{
			name:    "IPv4 address",
			input:   "192.168.1.1",
			wantErr: false,
			wantNet: "192.168.1.1/32",
		},
		{
			name:    "IPv4 CIDR",
			input:   "192.168.1.0/24",
			wantErr: false,
			wantNet: "192.168.1.0/24",
		},
		{
			name:    "IPv6 address",
			input:   "2001:db8::1",
			wantErr: false,
			wantNet: "2001:db8::1/128",
		},
		{
			name:    "IPv6 CIDR",
			input:   "2001:db8::/32",
			wantErr: false,
			wantNet: "2001:db8::/32",
		},
		{
			name:    "whitespace trimmed",
			input:   "  192.168.1.1  ",
			wantErr: false,
			wantNet: "192.168.1.1/32",
		},
		{
			name:    "invalid IP",
			input:   "999.999.999.999",
			wantErr: true,
		},
		{
			name:    "invalid CIDR",
			input:   "192.168.1.0/99",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "not-an-ip",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ipNet, err := parseCIDROrIP(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCIDROrIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && ipNet != nil && ipNet.String() != tt.wantNet {
				t.Errorf("parseCIDROrIP() = %v, want %v", ipNet.String(), tt.wantNet)
			}
		})
	}
}

func BenchmarkAllowlist_IsAllowed(b *testing.B) {
	entries := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"2001:db8::/32",
	}

	allowlist, err := NewAllowlist(entries)
	if err != nil {
		b.Fatalf("Failed to create allowlist: %v", err)
	}

	testIP := net.ParseIP("192.168.1.100")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		allowlist.IsAllowed(testIP)
	}
}

func BenchmarkAllowlist_IsAllowed_LargeList(b *testing.B) {
	// Create allowlist with many entries
	entries := make([]string, 100)
	for i := 0; i < 100; i++ {
		entries[i] = net.ParseIP("10.0.0.0").String() + "/8"
	}

	allowlist, err := NewAllowlist(entries)
	if err != nil {
		b.Fatalf("Failed to create allowlist: %v", err)
	}

	testIP := net.ParseIP("192.168.1.100")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		allowlist.IsAllowed(testIP)
	}
}
