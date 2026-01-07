package acl

import (
	"fmt"
	"net"
	"strings"
)

// Allowlist manages IP and CIDR-based access control
type Allowlist struct {
	rules []*net.IPNet
}

// NewAllowlist creates a new allowlist from CIDR strings and IP addresses
func NewAllowlist(entries []string) (*Allowlist, error) {
	var rules []*net.IPNet

	for _, entry := range entries {
		ipNet, err := parseCIDROrIP(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid allowlist entry %q: %w", entry, err)
		}
		rules = append(rules, ipNet)
	}

	return &Allowlist{
		rules: rules,
	}, nil
}

// IsAllowed checks if an IP address is allowed
func (a *Allowlist) IsAllowed(ip net.IP) bool {
	// If no rules are defined, deny all
	if len(a.rules) == 0 {
		return false
	}

	// Check if IP matches any rule
	for _, rule := range a.rules {
		if rule.Contains(ip) {
			return true
		}
	}

	return false
}

// parseCIDROrIP parses a CIDR range or single IP address into an IPNet
func parseCIDROrIP(s string) (*net.IPNet, error) {
	s = strings.TrimSpace(s)

	// Try parsing as CIDR first
	if strings.Contains(s, "/") {
		_, ipNet, err := net.ParseCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR: %w", err)
		}
		return ipNet, nil
	}

	// Parse as single IP address
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", s)
	}

	// Convert single IP to CIDR
	var ipNet *net.IPNet
	if ip.To4() != nil {
		// IPv4: /32
		_, ipNet, _ = net.ParseCIDR(ip.String() + "/32")
	} else {
		// IPv6: /128
		_, ipNet, _ = net.ParseCIDR(ip.String() + "/128")
	}

	return ipNet, nil
}
