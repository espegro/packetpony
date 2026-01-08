<img width="500" alt="image" src="https://github.com/user-attachments/assets/71b7783e-f8e4-4053-9b1c-cc8234f606b4" />


# PacketPony

PacketPony is a modern network proxy/forwarder service written in Go, inspired by redir and xinetd. It provides advanced rate limiting, access control, logging, and metrics for both TCP and UDP traffic.

## Table of Contents

- [Features](#features)
- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [Installation](#installation)
- [Configuration](#configuration)
  - [Minimal Configuration](#minimal-configuration)
  - [Listener Configuration](#listener-configuration)
  - [TCP-Specific Settings](#tcp-specific-settings)
  - [UDP-Specific Settings](#udp-specific-settings)
- [Rate Limiting](#rate-limiting)
- [UDP Session Tracking](#udp-session-tracking)
- [Logging](#logging)
  - [UDP Session Logging Configuration](#udp-session-logging-configuration)
- [Metrics](#metrics)
  - [Health Check Endpoints](#health-check-endpoints)
- [Usage Examples](#usage-examples)
- [Troubleshooting](#troubleshooting)
- [FAQ](#faq)
- [Best Practices](#best-practices)
- [Signal Handling](#signal-handling)
- [Performance](#performance)
- [Security](#security)
- [Development Guide](#development-guide)
- [License](#license)

## Features

- **Multi-protocol support**: TCP and UDP
- **IPv4 and IPv6**: Full support for both IPv4 and IPv6
- **Rate Limiting**:
  - Max connections per IP per time window (active connections)
  - Max connection attempts per IP per time window (including rejected)
  - Max bandwidth per IP per time window (bidirectional for TCP and UDP)
  - Max total connections per listener
  - Configurable actions: drop, throttle, or log_only
  - Throttle mode: reduce to minimum bandwidth instead of dropping
- **Access Control**: IP and CIDR-based allowlist per listener
- **Logging**:
  - Syslog support (UDP/TCP/Unix)
  - JSON file logging
  - Stdout logging (text or JSON, for systemd/journald)
  - Connection lifecycle events (open/close/update)
  - Detailed traffic statistics (bytes, packets)
  - **UDP session logging** with configurable thresholds:
    - Periodic updates based on time or bandwidth
    - Minimum session duration/bytes filters
    - Reduces log volume for high-traffic services
- **UDP Session Tracking**: Intelligent session management based on source IP:port
- **Prometheus Metrics**: Built-in metrics endpoint for monitoring
- **Health Checks**: Health endpoints at `/health`, `/healthz`, and `/ready` for Kubernetes probes
- **Graceful Shutdown**: Safe shutdown with timeout for active connections

## Quick Start

Get PacketPony up and running in 2 minutes:

### 1. Install

```bash
# Clone and build
git clone https://github.com/espegro/packetpony.git
cd packetpony
make build
```

### 2. Create a simple config

Create `my-config.yaml`:

```yaml
server:
  name: "my-proxy"

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
  - name: "web-proxy"
    protocol: "tcp"
    listen_address: "127.0.0.1:8080"
    target_address: "example.com:80"
    allowlist:
      - "0.0.0.0/0"  # Allow all (change in production!)
    rate_limits:
      max_connections_per_ip: 10
      connections_window: "1m"
      action: "drop"
```

### 3. Run it

```bash
./packetpony -config my-config.yaml
```

### 4. Test it

```bash
# In another terminal
curl http://localhost:8080

# Check health
curl http://localhost:9090/health

# Check metrics
curl http://localhost:9090/metrics
```

### 5. View logs

```bash
# Logs appear in stdout
# You should see connection open/close events
```

**Next steps:**
- Read [Configuration](#configuration) for all available options
- See [Usage Examples](#usage-examples) for real-world scenarios
- Check [Best Practices](#best-practices) before deploying to production

## Architecture

```
packetpony/
├── cmd/packetpony/main.go           # Entry point
├── internal/
│   ├── config/                      # Configuration and validation
│   ├── listener/                    # TCP/UDP listeners and manager
│   ├── proxy/                       # Proxy logic for TCP and UDP
│   ├── ratelimit/                   # Rate limiting (sliding window)
│   ├── acl/                         # IP/CIDR allowlist
│   ├── logging/                     # Syslog and JSON logging
│   ├── metrics/                     # Prometheus metrics
│   └── session/                     # UDP session tracking
└── configs/example.yaml             # Example configuration
```

## Installation

### From source

```bash
git clone https://github.com/espegro/packetpony.git
cd packetpony

# Build with make (recommended)
make build

# Or build with go directly
go build -o packetpony ./cmd/packetpony
```

### Using the Makefile

The project includes a comprehensive Makefile with many useful targets:

```bash
make help              # Show all available targets
make build             # Build the binary
make test              # Run tests with race detection
make lint              # Run linters (requires golangci-lint)
make install           # Install to /usr/local/bin
make install-service   # Install as systemd service (requires root)
make clean             # Remove build artifacts
make release           # Build optimized release binary
make cross-compile     # Build for multiple platforms
```

### Running

```bash
./packetpony -config configs/example.yaml

# Or with make
make run
```

### Running as a systemd service

For production deployments, see the [systemd deployment guide](deployment/systemd/README.md) for instructions on running PacketPony as a system service with automatic startup, logging, and monitoring.

Quick setup:
```bash
sudo cp deployment/systemd/packetpony.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now packetpony
```

## Configuration

PacketPony uses YAML for configuration. See `configs/example.yaml` for a complete example.

### Minimal configuration

```yaml
server:
  name: "packetpony-01"

logging:
  syslog:
    enabled: true
    network: "udp"
    address: "localhost:514"
    tag: "packetpony"
    priority: "info"

metrics:
  prometheus:
    enabled: true
    listen_address: ":9090"
    path: "/metrics"

listeners:
  - name: "http-proxy"
    protocol: "tcp"
    listen_address: "0.0.0.0:8080"
    target_address: "192.168.1.100:80"
    allowlist:
      - "0.0.0.0/0"
    rate_limits:
      max_connections_per_ip: 100
      connections_window: "1m"
      max_connection_attempts_per_ip: 500
      attempts_window: "1m"
      max_bandwidth_per_ip: "10MB"
      bandwidth_window: "1m"
      max_total_connections: 1000
      action: "drop"
```

### Listener configuration

Each listener can be configured with:

- **name**: Unique name for the listener
- **protocol**: `tcp` or `udp`
- **listen_address**: IP:port to listen on (supports IPv4 and IPv6)
- **target_address**: IP:port to forward traffic to
- **allowlist**: List of IP addresses and/or CIDR ranges
- **rate_limits**:
  - `max_connections_per_ip`: Max active connections per IP
  - `connections_window`: Time window for connection counting (e.g., "1m", "30s")
  - `max_connection_attempts_per_ip`: Max connection attempts (including rejected)
  - `attempts_window`: Time window for attempt counting
  - `max_bandwidth_per_ip`: Max bandwidth per IP (e.g., "10MB", "1GB")
  - `bandwidth_window`: Time window for bandwidth measurement
  - `max_total_connections`: Max total connections for listener
  - `action`: Action when limit exceeded: `drop`, `throttle`, or `log_only` (default: `drop`)
  - `throttle_minimum`: Minimum bandwidth when throttling (required if action is `throttle`)

### TCP-specific settings

```yaml
tcp:
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "5m"
```

### UDP-specific settings

```yaml
udp:
  session_timeout: "30s"   # Idle timeout for UDP sessions
  buffer_size: 4096        # Buffer size for UDP packets
```

## Rate Limiting

PacketPony uses a sliding window approach for rate limiting with multiple enforcement modes:

### Limit Types

- **Connection Limiting**: Tracks active (successful) connections per IP using a sliding window
- **Attempt Limiting**: Tracks ALL connection attempts per IP, including rejected ones
  - Protects against SYN flood and connection spam attacks
  - Typically set higher than connection limit (e.g., 4-5x)
- **Bandwidth Limiting**: Tracks bytes consumed per IP in a sliding window
  - **TCP**: Bidirectional - counts both client→target and target→client
  - **UDP**: Bidirectional - counts both inbound packets and return traffic
- **Total Connection Limiting**: Atomic counter for total connections per listener

### Action Modes

Configure how rate limits are enforced using the `action` parameter:

- **`drop` (default)**: Drop connection/packet immediately when limit exceeded
  - Best for security and DoS protection
  - Recommended for internet-facing services

- **`throttle`**: Reduce bandwidth to configured minimum instead of dropping
  - Requires `throttle_minimum` parameter (e.g., "1MB")
  - Allows legitimate users to continue at reduced speed
  - Good for internal services with occasional bursts

- **`log_only`**: Allow all traffic but log violations
  - Useful for testing and monitoring
  - Helps determine appropriate limits before enforcement
  - Does not drop any connections

### Behavior

- Dropped connections/packets do NOT count against quotas
- Quotas reset via sliding window expiration
- Active connections release quota immediately on close
- Each listener has independent rate limits

## UDP Session Tracking

For UDP traffic, which is connectionless, PacketPony implements virtual sessions:

1. First packet from new source IP:port → create session with target connection
2. Subsequent packets from same source → use existing session
3. Bi-directional communication: Response from target forwarded back to client
4. Session timeout → cleanup and logging of statistics

Sessions are identified by `srcIP:srcPort` and have configurable idle timeout.

## Logging

### Connection Events

PacketPony logs connection lifecycle events:

**Open event:**
```
listener=http-proxy proto=tcp event=open src=192.168.1.50:12345 dst=192.168.1.100:80
```

**Close event:**
```
listener=http-proxy proto=tcp event=close src=192.168.1.50:12345 dst=192.168.1.100:80 duration=5230ms bytes_sent=1024 bytes_recv=4096
```

For UDP, `pkts_sent` and `pkts_recv` are also included.

### Stdout Logging (Recommended for systemd)

PacketPony supports logging to stdout, which is automatically captured by journald when running under systemd:

```yaml
logging:
  stdout:
    enabled: true
    use_json: false  # false = human-readable text, true = JSON format
```

**Text format output:**
```
[2026-01-07 10:38:44] Connection opened: listener=ssh-test protocol=tcp src=127.0.0.1:49500 dst=127.0.0.1:22
[2026-01-07 10:40:44] Connection closed: listener=ssh-test protocol=tcp src=127.0.0.1:49500 dst=127.0.0.1:22 duration=120016ms bytes_sent=0 bytes_recv=29
```

**View logs with journalctl:**
```bash
# Follow logs in real-time
sudo journalctl -u packetpony -f

# View logs since last boot
sudo journalctl -u packetpony -b

# View logs from last hour
sudo journalctl -u packetpony --since "1 hour ago"
```

### Syslog Logging

Traditional syslog is also supported:

```yaml
logging:
  syslog:
    enabled: true
    network: "udp"
    address: "localhost:514"
    tag: "packetpony"
    priority: "info"
```

### JSON Logging

With JSON logging enabled, structured events are written to file:

```json
{
  "timestamp": "2026-01-07T10:30:45Z",
  "listener_name": "http-proxy",
  "protocol": "tcp",
  "source_ip": "192.168.1.50",
  "source_port": 12345,
  "target_ip": "192.168.1.100",
  "target_port": 80,
  "event_type": "close",
  "bytes_sent": 1024,
  "bytes_received": 4096,
  "duration_ms": 5230
}
```

### UDP Session Logging Configuration

For UDP listeners, you can configure logging behavior to reduce log volume for high-traffic services:

```yaml
listeners:
  - name: "dns-proxy"
    protocol: "udp"
    listen_address: "0.0.0.0:53"
    target_address: "8.8.8.8:53"
    udp:
      session_timeout: 30s
      logging:
        log_session_start: true          # Log when session opens (default: true)
        log_session_close: true          # Log when session closes (default: true)
        periodic_log_interval: 5m        # Log every 5 minutes for active sessions (default: 5m)
        periodic_log_bytes: 100MB        # Or log after 100MB transferred (default: 100MB)
        min_log_duration: 5s             # Skip logging sessions < 5s (default: 0, log all)
        min_log_bytes: 1KB               # Skip logging sessions < 1KB (default: 0, log all)
```

**Logging behavior:**
- **Session start**: Logged when first packet arrives (if `log_session_start: true`)
- **Periodic updates**: Logged based on time OR bytes threshold for long-running sessions
- **Session close**: Logged when session times out or shuts down (if meets thresholds)

**Example for DNS (short sessions):**
```yaml
udp:
  logging:
    log_session_start: false    # Skip start events
    min_log_duration: 5s        # Only log sessions > 5s
```

**Example for gaming/streaming (long sessions):**
```yaml
udp:
  logging:
    periodic_log_interval: 1m   # Update every minute
    periodic_log_bytes: 10MB    # Or after 10MB
```

**Log events:**
- `event_type: open` - Session created
- `event_type: update` - Periodic status update
- `event_type: close` - Session terminated

## Metrics

PacketPony exposes Prometheus metrics on the `/metrics` endpoint:

- `packetpony_connections_total{listener, protocol, status}` - Total connections
- `packetpony_connections_active{listener, protocol}` - Active connections
- `packetpony_bytes_transferred_total{listener, direction}` - Bytes transferred
- `packetpony_packets_transferred_total{listener, direction}` - Packets transferred (UDP)
- `packetpony_connection_duration_seconds{listener, protocol}` - Connection duration histogram
- `packetpony_rate_limit_drops_total{listener, reason}` - Dropped due to rate limiting
- `packetpony_acl_drops_total{listener}` - Dropped due to ACL
- `packetpony_errors_total{listener, type}` - Errors encountered

### Health Check Endpoints

When Prometheus metrics are enabled, PacketPony also exposes health check endpoints for Kubernetes liveness and readiness probes:

- `GET /health` - Returns `{"status":"healthy","service":"packetpony"}`
- `GET /healthz` - Same as `/health` (Kubernetes convention)
- `GET /ready` - Same as `/health` (readiness probe)

All endpoints return HTTP 200 with JSON response.

**Kubernetes deployment example:**
```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: packetpony
    image: packetpony:latest
    livenessProbe:
      httpGet:
        path: /healthz
        port: 9090
      initialDelaySeconds: 5
      periodSeconds: 10
    readinessProbe:
      httpGet:
        path: /ready
        port: 9090
      initialDelaySeconds: 3
      periodSeconds: 5
```

## Usage Examples

### HTTP Proxy with Drop Mode

Proxying HTTP traffic with strict rate limiting:

```yaml
listeners:
  - name: "http-proxy"
    protocol: "tcp"
    listen_address: "0.0.0.0:8080"
    target_address: "backend.example.com:80"
    allowlist:
      - "10.0.0.0/8"
    rate_limits:
      max_connections_per_ip: 50
      connections_window: "1m"
      max_connection_attempts_per_ip: 200
      attempts_window: "1m"
      max_bandwidth_per_ip: "10MB"
      bandwidth_window: "1m"
      max_total_connections: 500
      action: "drop"  # Drop connections when limits exceeded
```

### HTTPS Proxy with Throttle Mode

Allow connections but throttle bandwidth when limits exceeded:

```yaml
listeners:
  - name: "https-proxy"
    protocol: "tcp"
    listen_address: "0.0.0.0:8443"
    target_address: "backend.example.com:443"
    allowlist:
      - "0.0.0.0/0"
    rate_limits:
      max_connections_per_ip: 50
      connections_window: "30s"
      max_connection_attempts_per_ip: 200
      attempts_window: "30s"
      max_bandwidth_per_ip: "50MB"
      bandwidth_window: "1m"
      max_total_connections: 500
      action: "throttle"      # Throttle instead of drop
      throttle_minimum: "5MB"  # Minimum bandwidth when throttling
```

### DNS Proxy with Log-Only Mode

UDP-based DNS proxying with monitoring (no enforcement):

```yaml
listeners:
  - name: "dns-proxy"
    protocol: "udp"
    listen_address: "0.0.0.0:5353"
    target_address: "8.8.8.8:53"
    allowlist:
      - "0.0.0.0/0"
    rate_limits:
      max_connections_per_ip: 100
      connections_window: "10s"
      max_connection_attempts_per_ip: 500
      attempts_window: "10s"
      max_bandwidth_per_ip: "1MB"
      bandwidth_window: "10s"
      max_total_connections: 1000
      action: "log_only"  # Log violations but don't enforce
    udp:
      session_timeout: "30s"
      buffer_size: 4096
```

### SSH Proxy with Strict Rate Limiting

```yaml
listeners:
  - name: "ssh-proxy"
    protocol: "tcp"
    listen_address: "0.0.0.0:2222"
    target_address: "internal-server:22"
    allowlist:
      - "192.168.1.0/24"
    rate_limits:
      max_connections_per_ip: 3
      connections_window: "5m"
      max_connection_attempts_per_ip: 10  # Allow retries for auth failures
      attempts_window: "5m"
      max_total_connections: 20
      action: "drop"
```

### Game Server Proxy (Long-lived UDP Sessions)

UDP proxy for gaming with periodic logging and extended timeouts:

```yaml
listeners:
  - name: "game-server-proxy"
    protocol: "udp"
    listen_address: "0.0.0.0:27015"
    target_address: "game.backend.local:27015"
    allowlist:
      - "0.0.0.0/0"
    rate_limits:
      max_connections_per_ip: 2  # Limit simultaneous sessions per player
      connections_window: "5m"
      max_bandwidth_per_ip: "5MB"
      bandwidth_window: "1m"
      max_total_connections: 100
      action: "drop"
    udp:
      session_timeout: "10m"  # Long timeout for gaming sessions
      buffer_size: 8192       # Larger buffer for game packets
      logging:
        log_session_start: true
        log_session_close: true
        periodic_log_interval: 2m   # Log every 2 minutes
        periodic_log_bytes: 50MB    # Or after 50MB transferred
        min_log_duration: 30s       # Skip very short sessions
```

### IPv6 Dual-Stack Proxy

Listen on both IPv4 and IPv6:

```yaml
listeners:
  # IPv4 listener
  - name: "web-ipv4"
    protocol: "tcp"
    listen_address: "0.0.0.0:8080"
    target_address: "backend.example.com:80"
    allowlist:
      - "0.0.0.0/0"
    rate_limits:
      max_connections_per_ip: 50
      connections_window: "1m"
      action: "drop"

  # IPv6 listener
  - name: "web-ipv6"
    protocol: "tcp"
    listen_address: "[::]:8080"
    target_address: "backend.example.com:80"
    allowlist:
      - "::/0"  # Allow all IPv6
    rate_limits:
      max_connections_per_ip: 50
      connections_window: "1m"
      action: "drop"
```

### Multi-Backend Logging Setup

Log to multiple destinations simultaneously:

```yaml
logging:
  # Syslog for system logs
  syslog:
    enabled: true
    network: "udp"
    address: "localhost:514"
    tag: "packetpony"
    priority: "info"

  # JSON file for detailed analysis
  jsonlog:
    enabled: true
    path: "/var/log/packetpony/connections.json"

  # Stdout for systemd/journald
  stdout:
    enabled: true
    use_json: false  # Human-readable for journalctl

metrics:
  prometheus:
    enabled: true
    listen_address: ":9090"
    path: "/metrics"
```

### VPN/Tunnel Proxy with Strict Limits

Proxy VPN traffic with aggressive rate limiting:

```yaml
listeners:
  - name: "vpn-gateway"
    protocol: "udp"
    listen_address: "0.0.0.0:1194"
    target_address: "vpn.backend:1194"
    allowlist:
      - "10.0.0.0/8"
      - "172.16.0.0/12"
      - "192.168.0.0/16"
    rate_limits:
      max_connections_per_ip: 1  # One VPN session per IP
      connections_window: "1h"
      max_connection_attempts_per_ip: 5
      attempts_window: "1h"
      max_bandwidth_per_ip: "100MB"  # 100MB per hour
      bandwidth_window: "1h"
      max_total_connections: 500
      action: "drop"
    udp:
      session_timeout: "5m"
      buffer_size: 2048
      logging:
        log_session_start: true
        log_session_close: true
        periodic_log_interval: 10m
```

### API Gateway with Throttling

HTTP API proxy with bandwidth throttling instead of hard drops:

```yaml
listeners:
  - name: "api-gateway"
    protocol: "tcp"
    listen_address: "0.0.0.0:443"
    target_address: "api.backend:8443"
    allowlist:
      - "0.0.0.0/0"
    rate_limits:
      max_connections_per_ip: 20
      connections_window: "1m"
      max_connection_attempts_per_ip: 100
      attempts_window: "1m"
      max_bandwidth_per_ip: "10MB"
      bandwidth_window: "1m"
      max_total_connections: 2000
      action: "throttle"           # Throttle instead of drop
      throttle_minimum: "512KB"    # Reduce to 512KB/min when over limit
    tcp:
      read_timeout: "30s"
      write_timeout: "30s"
      idle_timeout: "2m"
```

### Database Proxy with Connection Limits

Protect database from connection exhaustion:

```yaml
listeners:
  - name: "postgres-proxy"
    protocol: "tcp"
    listen_address: "0.0.0.0:5432"
    target_address: "postgres.internal:5432"
    allowlist:
      - "10.0.0.0/8"  # Internal network only
    rate_limits:
      max_connections_per_ip: 10      # 10 connections per app server
      connections_window: "5m"
      max_connection_attempts_per_ip: 50  # Allow connection pool reconnects
      attempts_window: "5m"
      max_total_connections: 100      # Database max_connections
      action: "drop"
    tcp:
      idle_timeout: "10m"  # Database idle timeout
```

### DNS Proxy with Minimal Logging

High-volume DNS with reduced logging:

```yaml
listeners:
  - name: "dns-proxy"
    protocol: "udp"
    listen_address: "0.0.0.0:53"
    target_address: "8.8.8.8:53"
    allowlist:
      - "0.0.0.0/0"
    rate_limits:
      max_connections_per_ip: 100
      connections_window: "10s"
      max_bandwidth_per_ip: "100KB"
      bandwidth_window: "10s"
      action: "log_only"  # Monitor without enforcement
    udp:
      session_timeout: "5s"  # DNS queries are fast
      buffer_size: 4096
      logging:
        log_session_start: false     # Don't log starts
        log_session_close: false     # Don't log normal closes
        min_log_duration: 10s        # Only log sessions > 10s (anomalies)
        min_log_bytes: 10KB          # Only log large responses
```

## Troubleshooting

### Service won't start

**Symptom:** PacketPony exits immediately after starting

**Common causes:**

1. **Port already in use**
   ```bash
   # Check what's using the port
   sudo lsof -i :8080
   sudo netstat -tlnp | grep 8080
   ```
   **Solution:** Change `listen_address` port or stop the conflicting service

2. **Configuration validation failed**
   ```bash
   # Run manually to see detailed error
   ./packetpony -config /path/to/config.yaml
   ```
   **Solution:** Fix the configuration error shown in the output

3. **Permission denied (privileged ports)**
   ```bash
   # Error: bind: permission denied
   ```
   **Solution:** Either:
   - Use ports > 1024, OR
   - Run as root (not recommended), OR
   - Set capability: `sudo setcap CAP_NET_BIND_SERVICE=+eip ./packetpony`

### Connections dropping unexpectedly

**Symptom:** Connections close without apparent reason

**Diagnostic steps:**

```bash
# Check metrics for drops
curl http://localhost:9090/metrics | grep drop

# Check rate limit drops by reason
curl http://localhost:9090/metrics | grep rate_limit_drops_total

# Check ACL drops
curl http://localhost:9090/metrics | grep acl_drops_total

# View logs for specific listener
journalctl -u packetpony | grep "listener=mylistener"
```

**Common causes:**

1. **Rate limiting triggered**
   - Check `packetpony_rate_limit_drops_total` metric
   - Look for log entries: "denied by rate limit"
   - **Solution:** Increase rate limits or change action to "throttle"

2. **ACL rejecting connections**
   - Check `packetpony_acl_drops_total` metric
   - **Solution:** Add client IP to allowlist

3. **TCP timeout expired**
   - Check TCP timeout settings in config
   - **Solution:** Increase `idle_timeout`, `read_timeout`, or `write_timeout`

4. **Backend unreachable**
   - Check `packetpony_errors_total` metric
   - Look for "failed to dial target" in logs
   - **Solution:** Verify target_address is correct and reachable

### UDP traffic not reaching backend

**Symptom:** UDP packets not being forwarded

**Diagnostic steps:**

```bash
# Capture packets on listener interface
sudo tcpdump -i any -n port 5353

# Check UDP session timeouts in logs
journalctl -u packetpony | grep "UDP session"

# Check active UDP sessions
curl http://localhost:9090/metrics | grep "connections_active.*udp"
```

**Common causes:**

1. **Session timeout too short**
   - **Solution:** Increase `udp.session_timeout` in config

2. **Buffer size too small**
   - Large packets being truncated
   - **Solution:** Increase `udp.buffer_size` (default: 4096)

3. **Backend not responding**
   - Check if backend receives packets: `tcpdump -i any dst port 53`
   - **Solution:** Verify backend is listening and firewall allows traffic

### High CPU usage

**Symptom:** PacketPony consuming excessive CPU

**Diagnostic steps:**

```bash
# Check active connections
curl http://localhost:9090/metrics | grep connections_active

# Monitor goroutines (if pprof enabled)
curl http://localhost:6060/debug/pprof/goroutine?debug=1

# Check system stats
top -p $(pidof packetpony)
```

**Common causes:**

1. **Too many concurrent connections**
   - **Solution:** Reduce `max_total_connections` or scale horizontally

2. **Aggressive logging**
   - JSON logging with high connection churn
   - **Solution:** Disable verbose logging or use syslog instead

3. **Small rate limit windows**
   - Windows < 10s cause frequent cleanup
   - **Solution:** Use larger windows (30s-60s)

4. **Metrics scraping overhead**
   - Very frequent Prometheus scrapes
   - **Solution:** Reduce scrape frequency to 15-30s

### Memory usage growing (OOMKilled)

**Symptom:** systemd kills PacketPony with OOM error

**Diagnostic steps:**

```bash
# Check memory limit in systemd
systemctl show packetpony | grep MemoryLimit

# Monitor memory usage
watch -n 1 'ps aux | grep packetpony'

# Check for goroutine leaks
curl http://localhost:6060/debug/pprof/heap
```

**Common causes:**

1. **Unbounded connections**
   - **Solution:** Set `max_total_connections` appropriately

2. **UDP buffer accumulation**
   - Too many concurrent UDP sessions
   - **Solution:** Reduce `udp.session_timeout` or `max_connections_per_ip`

3. **Memory limit too low**
   - **Solution:** Increase `MemoryLimit` in systemd unit file

### Metrics endpoint returns 404

**Symptom:** `curl http://localhost:9090/metrics` fails

**Diagnostic steps:**

```bash
# Check if metrics server started
journalctl -u packetpony | grep "metrics server started"

# Check if port is listening
sudo lsof -i :9090
sudo netstat -tlnp | grep 9090

# Test health endpoint
curl http://localhost:9090/health
```

**Common causes:**

1. **Prometheus not enabled**
   - Check config: `metrics.prometheus.enabled: true`

2. **Wrong port or interface**
   - Check `listen_address` in config
   - Try `curl http://127.0.0.1:9090/metrics`

3. **Firewall blocking port**
   - **Solution:** Allow port in firewall

### Configuration validation errors

**Common YAML mistakes:**

```yaml
# ❌ WRONG - missing quotes around time duration
connections_window: 1m

# ✓ CORRECT
connections_window: "1m"

# ❌ WRONG - invalid CIDR notation
allowlist:
  - 192.168.1.1/24

# ✓ CORRECT
allowlist:
  - "192.168.1.0/24"  # Network address, not host

# ❌ WRONG - throttle without minimum
action: "throttle"

# ✓ CORRECT
action: "throttle"
throttle_minimum: "1MB"

# ❌ WRONG - missing protocol
listeners:
  - name: "my-proxy"
    listen_address: "0.0.0.0:8080"

# ✓ CORRECT
listeners:
  - name: "my-proxy"
    protocol: "tcp"
    listen_address: "0.0.0.0:8080"
```

### Service stuck on shutdown

**Symptom:** `systemctl stop packetpony` hangs for 30+ seconds

**Cause:** Active connections not closing within graceful shutdown timeout (30s)

**Solutions:**

```bash
# Force kill if stuck
sudo systemctl kill packetpony

# For long-lived connections, increase shutdown timeout
# Edit /etc/systemd/system/packetpony.service:
TimeoutStopSec=60s

# Reload systemd
sudo systemctl daemon-reload
```

### Debug mode / Verbose logging

**Enable detailed logging for troubleshooting:**

```yaml
logging:
  stdout:
    enabled: true
    use_json: true  # Structured logging for analysis
  jsonlog:
    enabled: true
    path: "/var/log/packetpony/debug.json"
```

**Then analyze logs:**

```bash
# Follow JSON logs
tail -f /var/log/packetpony/debug.json | jq .

# Filter by event type
jq 'select(.event_type == "close")' /var/log/packetpony/debug.json

# Find high-bandwidth sessions
jq 'select(.bytes_sent + .bytes_received > 1000000)' /var/log/packetpony/debug.json
```

## FAQ

### General Questions

**Q: Does PacketPony support hot reload of configuration?**

A: No, PacketPony does not support hot reload. You must restart the service to apply configuration changes:
```bash
sudo systemctl restart packetpony
```

**Q: Can I run multiple PacketPony instances?**

A: Yes! You can run multiple instances either:
- On different ports (different listeners)
- On different servers (load balancing/HA)
- Using different config files with different systemd units

**Q: What's the maximum number of concurrent connections?**

A: PacketPony uses one goroutine per TCP connection and per UDP session. The limit depends on:
- `max_total_connections` setting (recommended: < 10,000 per instance)
- System file descriptor limits (`ulimit -n`)
- Available memory (~50KB per connection base + buffers)

**Q: Does PacketPony modify the traffic?**

A: No, PacketPony is a transparent proxy. It forwards packets unchanged between client and backend.

### Rate Limiting

**Q: How does the sliding window rate limiting work?**

A: PacketPony uses a sliding window approach:
- Each IP has a map of timestamps for connections/attempts/bandwidth
- Old entries expire when they fall outside the window (e.g., older than 1 minute)
- Periodic cleanup removes expired entries
- Quota resets naturally as time passes, not at fixed intervals

**Q: What's the difference between connection limit and attempt limit?**

A:
- **Connection limit**: Tracks only successful, active connections
- **Attempt limit**: Tracks ALL connection attempts, including rejected ones (ACL, rate limits, errors)
- Use attempt limit to prevent connection spam/SYN floods
- Recommended: `max_connection_attempts_per_ip` = 4-5x `max_connections_per_ip`

**Q: Is bandwidth limiting bidirectional?**

A: Yes:
- **TCP**: Counts both client→target and target→client bytes
- **UDP**: Counts both inbound packets and return traffic
- Example: Download 5MB + upload 1MB = 6MB consumed

**Q: When should I use `drop` vs `throttle` vs `log_only`?**

A:
- **drop**: Best for security, DoS protection, internet-facing services
- **throttle**: Good for internal services where you want degraded service instead of denial
- **log_only**: Use during testing to understand traffic patterns before enforcing limits

**Q: Do dropped connections count against quotas?**

A: No:
- Connections dropped due to rate limits do NOT consume quota
- Connections dropped due to ACL do NOT count as attempts
- Only successful/accepted traffic counts

### UDP Sessions

**Q: How are UDP sessions identified?**

A: By source IP and source port: `192.168.1.100:54321`
- Each unique source creates a separate session
- NAT can cause multiple clients to appear as one session

**Q: What happens when a UDP session times out?**

A:
1. Session is removed from active sessions
2. Statistics are logged (if meets logging thresholds)
3. Target connection is closed
4. Rate limit quota is released
5. Next packet from same source creates a NEW session

**Q: Can I see active UDP sessions?**

A: Yes, via Prometheus metrics:
```bash
curl http://localhost:9090/metrics | grep 'connections_active.*udp'
```

### Logging

**Q: Which logging backend should I use?**

A: Depends on your setup:
- **systemd/journald**: Use `stdout` logging (automatically captured)
- **Centralized logging**: Use `syslog` (send to remote syslog server)
- **Analysis/compliance**: Use `jsonlog` (structured JSON for processing)
- **All of the above**: Enable multiple backends simultaneously

**Q: Can I reduce UDP logging for high-traffic services?**

A: Yes! Use UDP logging configuration:
```yaml
udp:
  logging:
    log_session_start: false      # Skip start events
    min_log_duration: 5s          # Only log sessions > 5s
    min_log_bytes: 10KB           # Only log sessions > 10KB
```

**Q: What are the different log event types?**

A:
- **open**: Connection/session started
- **close**: Connection/session ended (with statistics)
- **update**: Periodic update for long-running UDP sessions

### Deployment

**Q: How do I run PacketPony on privileged ports (<1024) without root?**

A: Use Linux capabilities:
```bash
# Set capability on binary
sudo setcap CAP_NET_BIND_SERVICE=+eip /usr/local/bin/packetpony

# Or use systemd (already configured in provided unit file)
AmbientCapabilities=CAP_NET_BIND_SERVICE
```

**Q: How do I validate configuration without starting the service?**

A: Run manually and check for errors:
```bash
./packetpony -config /path/to/config.yaml
# If configuration is valid, service will start
# Press Ctrl+C if it starts successfully
```

**Q: Can I use PacketPony in Kubernetes?**

A: Yes! See the [Health Check Endpoints](#health-check-endpoints) section for Kubernetes probe configuration. You'll want to:
- Use a ConfigMap for configuration
- Set liveness and readiness probes
- Expose metrics for Prometheus scraping
- Consider DaemonSet for per-node proxying

**Q: How do I monitor PacketPony in production?**

A: Use the Prometheus metrics endpoint:
```bash
# Key metrics to alert on:
- packetpony_rate_limit_drops_total (rate limiting active)
- packetpony_acl_drops_total (ACL rejecting traffic)
- packetpony_errors_total (errors increasing)
- packetpony_connections_active (capacity issues)
```

### Performance

**Q: What's the performance overhead of PacketPony?**

A: Minimal overhead:
- **TCP**: Uses `io.Copy` (zero-copy kernel proxying)
- **UDP**: Inline packet handling (no goroutine per packet)
- **Rate limiting**: Per-IP locking, periodic cleanup
- Typical overhead: < 5% CPU, ~50KB memory per connection

**Q: How many listeners can I configure?**

A: No hard limit, but practical considerations:
- Each listener binds a port
- Each listener has independent rate limiting state
- Recommend: < 50 listeners per instance
- For many services, use multiple PacketPony instances

**Q: Does PacketPony support connection pooling?**

A: No, PacketPony creates a new backend connection for each client connection (1:1 mapping). For connection pooling, use a dedicated connection pooler like PgBouncer for databases.

### Security

**Q: Is PacketPony secure?**

A: PacketPony provides:
- ✅ ACL-based access control
- ✅ Rate limiting (DoS protection)
- ✅ Connection limits (resource exhaustion protection)
- ✅ Configuration validation
- ✅ Privilege dropping via systemd

PacketPony does NOT provide:
- ❌ TLS termination
- ❌ Authentication
- ❌ Packet inspection/filtering
- ❌ DDoS mitigation (use dedicated DDoS protection)

**Q: Should I expose PacketPony directly to the internet?**

A: Depends on your use case:
- ✅ Yes: If you need rate limiting/ACL at the edge
- ❌ No: If you need TLS, WAF, or advanced DDoS protection
- Recommended: Place behind a load balancer with TLS termination

**Q: How do I protect the metrics endpoint?**

A: The metrics endpoint has no built-in authentication. Options:
- Firewall rules (restrict to monitoring systems only)
- Reverse proxy with authentication
- Network isolation (metrics on internal-only interface)

### Troubleshooting

**Q: Why are my connections being dropped?**

A: Check in this order:
1. Metrics: `curl localhost:9090/metrics | grep drops`
2. ACL: Is client IP in allowlist?
3. Rate limits: Are limits being exceeded?
4. Backend: Is target reachable?
5. Timeouts: Are TCP timeouts too aggressive?

**Q: How do I debug rate limiting issues?**

A: Use `log_only` mode first:
```yaml
rate_limits:
  # ... your limits ...
  action: "log_only"  # Log violations without enforcing
```
Check logs to see which IPs would be affected, then switch to `drop` or `throttle`.

**Q: PacketPony is using too much memory, what should I do?**

A: Reduce:
- `max_total_connections` (limit concurrent connections)
- `udp.session_timeout` (clean up UDP sessions faster)
- UDP `buffer_size` (if you have many UDP sessions)
- Disable verbose logging (JSON logging uses more memory)

## Best Practices

### Production Deployment

**Pre-Deployment Checklist:**
- [ ] Configuration validated locally
- [ ] Rate limits tested with `log_only` mode
- [ ] Target addresses verified reachable
- [ ] Monitoring configured (Prometheus scraping)
- [ ] Log rotation configured (for JSON logs)
- [ ] Backup of configuration file created
- [ ] Rollback plan documented

**Security Hardening:**
- Use restrictive allowlists (don't use `0.0.0.0/0` in production)
- Set `max_total_connections` to prevent resource exhaustion
- Restrict metrics endpoint to monitoring systems only
- Use systemd security features (already in provided unit file)
- Enable rate limiting with `drop` mode for internet-facing services
- Monitor logs for unusual patterns

**Capacity Planning:**
- **Memory**: ~50KB per connection + buffer_size per UDP session
- **CPU**: Minimal for proxying; grows with rate limit checking
- **Disk**: JSON logs ~1KB per event
  - Estimate: `listeners × concurrent_sessions × events_per_minute × 1KB`
- **Network**: Set bandwidth limits to prevent saturation

### Configuration Best Practices

**Rate Limiting:**
- Set `max_connection_attempts_per_ip` = 4-5x `max_connections_per_ip`
- Use larger windows for better accuracy (30s-60s)
- Start with `log_only`, observe traffic, then enforce
- For APIs: Use `throttle` mode for better user experience
- For databases: Set `max_total_connections` = database max_connections

**UDP Sessions:**
- Match `session_timeout` to your application behavior
  - DNS: 5-10s
  - Gaming: 5-10m
  - VPN: 2-5m
- Use larger `buffer_size` for applications with large packets
- Configure logging thresholds to reduce log volume

**Logging:**
- Production: Use `stdout` (captured by systemd/journald)
- Debugging: Enable `jsonlog` temporarily
- High traffic: Reduce UDP session logging with thresholds
- Long-term storage: Forward syslog to centralized logging

**Monitoring:**
- Alert on `packetpony_rate_limit_drops_total` increasing rapidly
- Alert on `packetpony_errors_total` > 0
- Monitor `packetpony_connections_active` for capacity planning
- Track connection duration for performance baselines

### Operational Recommendations

**Graceful Updates:**
```bash
# 1. Test new configuration
./packetpony -config /etc/packetpony/config.yaml.new

# 2. If valid, backup old config
sudo cp /etc/packetpony/config.yaml /etc/packetpony/config.yaml.bak

# 3. Deploy new config
sudo cp /etc/packetpony/config.yaml.new /etc/packetpony/config.yaml

# 4. Restart service
sudo systemctl restart packetpony

# 5. Verify
sudo systemctl status packetpony
curl http://localhost:9090/health
```

**Emergency Procedures:**
```bash
# Force kill if stuck
sudo systemctl kill packetpony

# Rollback configuration
sudo cp /etc/packetpony/config.yaml.bak /etc/packetpony/config.yaml
sudo systemctl restart packetpony

# Disable service if broken
sudo systemctl stop packetpony
sudo systemctl disable packetpony
```

**Log Management:**
```bash
# JSON log rotation (add to crontab)
0 0 * * * /usr/bin/find /var/log/packetpony -name "*.json" -mtime +7 -delete

# Or use logrotate
# Create /etc/logrotate.d/packetpony:
/var/log/packetpony/*.json {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
}
```

**Multi-Instance Setup:**

For high availability or load distribution:

```bash
# Instance 1 - HTTP traffic
./packetpony -config /etc/packetpony/http.yaml

# Instance 2 - HTTPS traffic
./packetpony -config /etc/packetpony/https.yaml

# Instance 3 - UDP services
./packetpony -config /etc/packetpony/udp.yaml
```

Or use systemd templates:
```bash
# /etc/systemd/system/packetpony@.service
# Start with: systemctl start packetpony@http packetpony@https
```

## Signal Handling

PacketPony supports graceful shutdown:

- `SIGINT` (Ctrl+C): Graceful shutdown
- `SIGTERM`: Graceful shutdown

On shutdown:
1. Stop accepting new connections
2. Wait for active connections to complete (max 30s)
3. Flush logs and metrics
4. Exit

## Performance

PacketPony is designed for performance:

- **Zero-copy TCP proxying**: Uses `io.Copy` for efficient kernel-level copying
- **Goroutine per TCP connection**: Scales well for many concurrent connections
- **Inline UDP handling**: Packets are handled inline (no goroutine per packet)
- **Fine-grained locking**: Per-IP locking in rate limiters for minimal contention
- **Periodic cleanup**: Batch cleanup of rate limit maps

## Security

- **Input validation**: All configuration is validated at startup
- **Resource limits**: Rate limiting protects against DoS
- **Connection limits**: Max total connections prevents resource exhaustion
- **Buffer limits**: Fixed-size UDP buffers prevent memory exhaustion
- **Privilege dropping**: Can run as non-root after binding to ports

## Development Guide

### Running tests

```bash
go test ./...
```

### Build with race detection

```bash
go build -race -o packetpony ./cmd/packetpony
```

### Linting

```bash
golangci-lint run
```

## System Requirements

- Go 1.21 or newer
- Linux, macOS, or Windows
- For binding to privileged ports (<1024): root/administrator or CAP_NET_BIND_SERVICE

## License

MIT License

## Contributing

Contributions are welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Open a Pull Request

## Support

For bugs and feature requests, please open an issue on GitHub.
