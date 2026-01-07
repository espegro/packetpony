# PacketPony

PacketPony is a modern network proxy/forwarder service written in Go, inspired by redir and xinetd. It provides advanced rate limiting, access control, logging, and metrics for both TCP and UDP traffic.

## Features

- **Multi-protocol support**: TCP and UDP
- **IPv4 and IPv6**: Full support for both IPv4 and IPv6
- **Rate Limiting**:
  - Max connections per IP per time window (configurable)
  - Max bandwidth per IP per time window
  - Max total connections per listener
  - Automatic connection dropping when limits exceeded
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
      max_bandwidth_per_ip: "10MB"
      bandwidth_window: "1m"
      max_total_connections: 1000
```

### Listener configuration

Each listener can be configured with:

- **name**: Unique name for the listener
- **protocol**: `tcp` or `udp`
- **listen_address**: IP:port to listen on (supports IPv4 and IPv6)
- **target_address**: IP:port to forward traffic to
- **allowlist**: List of IP addresses and/or CIDR ranges
- **rate_limits**:
  - `max_connections_per_ip`: Max connections per IP
  - `connections_window`: Time window for connection counting (e.g., "1m", "30s")
  - `max_bandwidth_per_ip`: Max bandwidth per IP (e.g., "10MB", "1GB")
  - `bandwidth_window`: Time window for bandwidth measurement
  - `max_total_connections`: Max total connections for listener

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

PacketPony uses a hybrid sliding window + token bucket approach:

- **Connection Limiting**: Sliding window log for accurate connection counting per IP
- **Bandwidth Limiting**: Sliding window tracking of bytes consumed per IP
- **Total Connection Limiting**: Atomic counter for total connections per listener

When limits are exceeded:
- Connection is dropped immediately
- Event is logged
- Metrics are updated

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

### HTTP Proxy

Proxying HTTP traffic with rate limiting:

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
      max_bandwidth_per_ip: "10MB"
      bandwidth_window: "1m"
      max_total_connections: 500
```

### DNS Proxy

UDP-based DNS proxying:

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
      max_bandwidth_per_ip: "1MB"
      bandwidth_window: "10s"
      max_total_connections: 1000
    udp:
      session_timeout: "30s"
      buffer_size: 4096
```

### SSH Proxy with strict rate limiting

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
      max_total_connections: 20
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
