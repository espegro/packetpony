# Testing Guide

This document describes the test suite for PacketPony.

## Overview

PacketPony has comprehensive unit tests for critical components:

- **Config** - Configuration parsing, validation, and bandwidth string parsing
- **ACL** - IP/CIDR allowlist matching (100% coverage)
- **Rate Limiting** - Connection and bandwidth limiting logic
- **Benchmarks** - Performance tests for hot paths

## Running Tests

### Quick test
```bash
make test
```

### With coverage report
```bash
make coverage
# Opens coverage.html in browser
```

### Short tests (no race detection)
```bash
make test-short
```

### Benchmarks
```bash
make bench
```

## Test Coverage

Current coverage by package:

| Package | Coverage | Key Areas Tested |
|---------|----------|------------------|
| `internal/acl` | **100%** | IP/CIDR matching, IPv4/IPv6 support |
| `internal/config` | **74.2%** | Bandwidth parsing, validation, config loading |
| `internal/ratelimit` | **55.6%** | Connection limits, bandwidth limits, sliding windows |

## Test Structure

### Config Tests (`internal/config/`)

**`config_test.go`**:
- Bandwidth parsing (10MB, 1GB, etc.)
- Config file loading from YAML
- Default value handling
- UDP logging configuration
- Error cases (invalid YAML, missing files)

**`validate_test.go`**:
- Server configuration validation
- Listener validation (TCP/UDP)
- Rate limit validation
- Protocol-specific config validation
- Address and CIDR validation

### ACL Tests (`internal/acl/allowlist_test.go`)

- Single IP matching
- CIDR range matching
- IPv4 and IPv6 support
- Wildcard addresses (0.0.0.0/0, ::/0)
- Empty allowlist behavior (deny all)
- Multiple overlapping ranges
- Private network ranges

### Rate Limiting Tests (`internal/ratelimit/`)

**`connection_limiter_test.go`**:
- Per-IP connection limits
- Sliding window behavior
- Connection release
- Multiple independent IPs
- Concurrent access safety
- Automatic cleanup

**`bandwidth_limiter_test.go`**:
- Drop mode (reject when over limit)
- Throttle mode (reduce to minimum)
- Log-only mode (allow but track violations)
- Sliding window bandwidth tracking
- Bidirectional bandwidth
- Multi-IP isolation
- Concurrent access safety

## Benchmark Results

Current performance (on 32-core system):

```
ACL Matching:
  Single IP:     61 ns/op    (0 allocs)
  Large list:    1627 ns/op  (0 allocs)

Rate Limiting:
  Connection:    30 µs/op    (1 alloc)
  Bandwidth:     355 µs/op   (2 allocs)
  Multi-IP:      3.4 µs/op   (1 alloc)
```

## Testing Best Practices

### Table-Driven Tests

Most tests use table-driven approach for better coverage:

```go
tests := []struct {
    name    string
    input   string
    want    int64
    wantErr bool
}{
    {name: "megabytes", input: "10MB", want: 10 * 1024 * 1024},
    // ... more cases
}
```

### Concurrent Safety

Critical components are tested with concurrent access:

```go
func TestConnectionLimiter_ConcurrentAccess(t *testing.T) {
    // Multiple goroutines hammering the limiter
    // No panics = success
}
```

### Timing-Sensitive Tests

Tests involving sliding windows use controlled delays:

```go
// Wait for window to expire
time.Sleep(window + 50*time.Millisecond)
```

## Writing New Tests

When adding new functionality:

1. **Add unit tests** in the same package (`*_test.go`)
2. **Test edge cases**: empty inputs, zero values, negative values
3. **Test concurrent access** if component uses locks
4. **Add benchmarks** for performance-critical code
5. **Run race detector**: `go test -race ./...`

Example test structure:

```go
func TestNewFeature(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        // Test cases here
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := NewFeature(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

## CI/CD Integration

Tests can be integrated into CI pipelines:

```yaml
# GitHub Actions example
- name: Run tests
  run: make test

- name: Upload coverage
  run: make coverage
```

## Known Gaps

The following components have **no tests yet** (0% coverage):

- `cmd/packetpony` - Main entry point
- `internal/listener` - Listener management
- `internal/logging` - Log backends
- `internal/metrics` - Prometheus metrics
- `internal/proxy` - TCP/UDP proxying
- `internal/session` - UDP session tracking

These would benefit from integration tests.

## Test Data

Temporary test files are created using `t.TempDir()` which auto-cleans:

```go
tmpDir := t.TempDir()  // Cleaned up automatically
configPath := filepath.Join(tmpDir, "test.yaml")
```

## Debugging Failed Tests

### Verbose output
```bash
go test -v ./internal/config
```

### Run specific test
```bash
go test -v ./internal/acl -run TestAllowlist_IsAllowed
```

### Race detection
```bash
go test -race ./...
```

### Coverage for specific package
```bash
go test -cover ./internal/ratelimit
```

## Performance Regression

Run benchmarks regularly to catch performance regressions:

```bash
# Baseline
go test -bench=. ./... > old.txt

# After changes
go test -bench=. ./... > new.txt

# Compare
benchcmp old.txt new.txt
```

## Resources

- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Table-Driven Tests](https://github.com/golang/go/wiki/TableDrivenTests)
- [Go Testing Best Practices](https://golang.org/doc/effective_go#testing)
