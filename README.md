# PacketPony

PacketPony is a modern network proxy/forwarder service written in Go, inspired by redir and xinetd. It provides advanced rate limiting, access control, logging, and metrics for both TCP and UDP traffic.

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
  - Connection lifecycle events (open/close)
  - Detailed traffic statistics (bytes, packets)
- **UDP Session Tracking**: Intelligent session management based on source IP:port
- **Prometheus Metrics**: Built-in metrics endpoint for monitoring
- **Graceful Shutdown**: Safe shutdown with timeout for active connections

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
