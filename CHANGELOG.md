# Changelog

All notable changes to PacketPony will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **`max_connections_per_ip` is now a concurrent-connection limit** instead of a sliding-window rate. A slot is reserved when a connection is admitted and released when it closes, so the count is always accurate (the old behavior popped the oldest timestamp on release, corrupting the window for long-lived connections). Use `max_connection_attempts_per_ip` for rate limiting.
- `connections_window` is **deprecated and ignored**. Existing configs still load (the field is accepted), but it no longer has any effect.

### Fixed

- Bandwidth limiting no longer double-counts the current chunk/packet, which previously produced spurious "bandwidth limit exceeded" log entries in `log_only` mode well below the configured limit. The allow/over-limit decision is now computed in a single pass over the window.
- UDP session creation no longer performs the target dial (including any DNS lookup) while holding the session-manager lock, so a slow or unresolvable target no longer stalls all other sessions on the same listener.
- Listener `protocol` is now matched case-insensitively for protocol-specific validation (e.g. `TCP`/`UDP` no longer skip TCP/UDP config checks).

## [1.1.1] - 2026-06-12

### Changed

- Systemd service now uses `-config-dir` parameter to load listener fragments from `/etc/packetpony/config.d/`
- Improved documentation for privileged port binding (ports < 1024)
- Added SELinux considerations and troubleshooting guide for systemd deployments

## [1.1.0] - 2026-06-12

### Added

- Listener fragments from `config.d/*.yaml`, loaded in filename order
- Configuration reload with `SIGHUP` and `systemctl reload packetpony`
- Connection-preserving TCP listener replacement during reload

### Fixed

- UDP session cleanup now releases sockets, rate-limit capacity, and metrics exactly once
- Graceful shutdown now drains TCP connections before enforcing its timeout
- TCP proxy half-close handling and per-connection context goroutine cleanup

## [1.0.0] - 2026-04-11

### Added

- Initial release of PacketPony
- TCP and UDP proxy support with bidirectional traffic forwarding
- Multi-protocol support (IPv4 and IPv6)
- Advanced rate limiting features:
  - Per-IP connection limits with sliding window
  - Connection attempt tracking (protects against SYN floods)
  - Bidirectional bandwidth limiting (TCP and UDP)
  - Total connection limits per listener
  - Three action modes: drop, throttle, and log_only
- IP/CIDR-based access control lists (allowlist)
- Comprehensive logging:
  - Syslog support (UDP/TCP/Unix)
  - JSON file logging
  - Stdout logging (systemd/journald compatible)
  - Connection lifecycle events (open/close/update)
  - Configurable UDP session logging with thresholds
- Prometheus metrics:
  - Connection counters and active connections
  - Bytes and packets transferred
  - Connection duration histograms
  - Rate limit and ACL drops
  - Error tracking
- Health check endpoints (`/health`, `/healthz`, `/ready`)
- Graceful shutdown with configurable timeout
- UDP session tracking:
  - Intelligent session management based on source IP:port
  - Configurable session timeouts
  - Periodic logging for long-running sessions
  - Bidirectional communication support
- Configuration features:
  - YAML-based configuration
  - Bandwidth parsing (KB, MB, GB, TB)
  - TCP-specific settings (timeouts, buffer sizes)
  - UDP-specific settings (session timeout, buffer size, logging)
  - Comprehensive validation at startup
- systemd integration:
  - Service unit file
  - Automatic restart on failure
  - Capability-based privilege management
  - Security hardening
- Comprehensive test suite:
  - 94 unit tests across critical components
  - 100% coverage for ACL matching
  - 74.2% coverage for configuration parsing
  - 55.6% coverage for rate limiting
  - Concurrent access testing
  - Performance benchmarks
- Documentation:
  - Comprehensive README with examples
  - Troubleshooting guide
  - FAQ section
  - Best practices
  - systemd deployment guide
  - Testing guide

### Security

- Non-root execution with capability-based port binding
- Input validation for all configuration
- Resource limits to prevent exhaustion
- Fixed-size buffers to prevent memory attacks
- ACL-based access control

### Performance

- Zero-copy TCP proxying using `io.Copy`
- Inline UDP packet handling (no goroutine per packet)
- Fine-grained locking for minimal contention
- Configurable buffer sizes for optimization
- Efficient sliding window rate limiting

[1.0.0]: https://github.com/espegro/packetpony/releases/tag/v1.0.0
[1.1.0]: https://github.com/espegro/packetpony/compare/v1.0.0...v1.1.0
