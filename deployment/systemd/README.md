# PacketPony Systemd Deployment

This directory contains systemd unit files for running PacketPony as a system service.

## Installation

### 1. Create user and directories

```bash
# Create system user
sudo useradd -r -s /bin/false -d /var/lib/packetpony packetpony

# Create directories
sudo mkdir -p /etc/packetpony
sudo mkdir -p /var/lib/packetpony
sudo mkdir -p /var/log/packetpony

# Set ownership
sudo chown packetpony:packetpony /var/lib/packetpony
sudo chown packetpony:packetpony /var/log/packetpony
```

### 2. Install binary and configuration

```bash
# Build and install binary
go build -o packetpony ./cmd/packetpony
sudo install -m 755 packetpony /usr/local/bin/packetpony

# Install configuration
sudo cp configs/example.yaml /etc/packetpony/config.yaml
sudo chown root:packetpony /etc/packetpony/config.yaml
sudo chmod 640 /etc/packetpony/config.yaml

# Edit configuration as needed
sudo nano /etc/packetpony/config.yaml
```

### 3. Install systemd unit file

```bash
sudo cp deployment/systemd/packetpony.service /etc/systemd/system/
sudo systemctl daemon-reload
```

### 4. Enable and start service

```bash
# Enable service to start on boot
sudo systemctl enable packetpony

# Start service
sudo systemctl start packetpony

# Check status
sudo systemctl status packetpony
```

## Managing the Service

### View logs

```bash
# Follow logs
sudo journalctl -u packetpony -f

# View recent logs
sudo journalctl -u packetpony -n 100

# View logs since boot
sudo journalctl -u packetpony -b
```

### Control service

```bash
# Start
sudo systemctl start packetpony

# Stop
sudo systemctl stop packetpony

# Restart
sudo systemctl restart packetpony

# Reload configuration (requires app support)
sudo systemctl reload packetpony

# Check status
sudo systemctl status packetpony
```

### Check metrics

```bash
# If Prometheus metrics are enabled (default port 9090)
curl http://localhost:9090/metrics
```

## Configuration Notes

### Privileged Ports

If you need to bind to privileged ports (< 1024), the service file includes `CAP_NET_BIND_SERVICE` capability. This is more secure than running as root.

If you don't need privileged ports, you can remove these lines from the service file:
```ini
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

### Security Hardening

The systemd unit file includes various security hardening options:
- Runs as dedicated `packetpony` user
- Private /tmp
- Read-only root filesystem (except /var/log/packetpony)
- Limited system call access
- No new privileges
- Restricted address families (Unix, IPv4, IPv6)

These can be adjusted based on your security requirements.

### Resource Limits

The service is configured with:
- Max file descriptors: 65536
- Max tasks: 4096

Adjust these in the service file if needed for your workload.

### Restart Policy

The service will automatically restart on failure with:
- 5 second delay between restarts
- Maximum 3 restarts within 60 seconds
- After that, systemd will give up

Adjust `RestartSec`, `StartLimitInterval`, and `StartLimitBurst` as needed.

## Updating Configuration

After changing `/etc/packetpony/config.yaml`:

```bash
# Validate configuration (run as packetpony user)
sudo -u packetpony /usr/local/bin/packetpony -config /etc/packetpony/config.yaml &
# Press Ctrl+C immediately if it starts successfully

# Restart service
sudo systemctl restart packetpony
```

## Upgrading PacketPony

```bash
# Build new version
go build -o packetpony ./cmd/packetpony

# Stop service
sudo systemctl stop packetpony

# Replace binary
sudo install -m 755 packetpony /usr/local/bin/packetpony

# Start service
sudo systemctl start packetpony

# Check status
sudo systemctl status packetpony
```

## Troubleshooting

### Service won't start

Check logs:
```bash
sudo journalctl -u packetpony -n 50 --no-pager
```

Common issues:
- Configuration file syntax errors
- Permission issues (check file ownership)
- Port already in use
- Network not ready (wait a bit and try again)

### Permission denied errors

Ensure correct ownership:
```bash
sudo chown packetpony:packetpony /var/lib/packetpony
sudo chown packetpony:packetpony /var/log/packetpony
sudo chown root:packetpony /etc/packetpony/config.yaml
sudo chmod 640 /etc/packetpony/config.yaml
```

### High resource usage

Check current limits and adjust in service file:
```bash
# View current resource usage
systemctl status packetpony

# View detailed resource usage
systemd-cgtop -d 3
```

## Monitoring

### Systemd Status

```bash
# Quick status
systemctl is-active packetpony
systemctl is-enabled packetpony

# Detailed status
systemctl status packetpony
```

### Prometheus Metrics

If enabled in configuration:
```bash
# Scrape metrics
curl -s http://localhost:9090/metrics | grep packetpony

# Monitor specific metrics
watch -n 1 'curl -s http://localhost:9090/metrics | grep packetpony_connections_active'
```

### Logs

Configure JSON logging in `/etc/packetpony/config.yaml` for structured logs:
```yaml
logging:
  jsonlog:
    enabled: true
    path: "/var/log/packetpony/events.json"
```

Then parse with jq:
```bash
tail -f /var/log/packetpony/events.json | jq .
```
