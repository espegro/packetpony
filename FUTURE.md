# Future Enhancements

This document tracks potential future improvements to PacketPony. Items are categorized by priority and implementation complexity.

**Legend:**
- 🟢 **Low complexity** - Can be implemented in a few hours
- 🟡 **Medium complexity** - Requires 1-3 days of work
- 🔴 **High complexity** - Major feature requiring significant design and testing
- ⏸️ **Wait for demand** - Good idea, but waiting for user requests
- ❌ **Dismissed** - Evaluated and determined not worth implementing

---

## ✅ Configuration Hot Reload

Implemented with ordered `config.d/*.yaml` listener fragments and `SIGHUP`
reload on Unix systems. Changed TCP listeners drain established connections
while new connections use the new configuration. Invalid reloads leave the
current runtime configuration active.

Potential future extensions:

- Native Windows reload control
- Filesystem watcher for automatic reload
- Reloadable logging and metrics backends
- Standalone configuration validation command

---

## ⏸️ UDP Buffer Pool (Wait for Demand)

**Status:** Premature optimization  
**Complexity:** 🟡 Medium  
**Value:** Low (only benefits high-volume UDP traffic)

### Problem

Currently, each incoming UDP packet allocates a new buffer:

```go
// In readLoop (listener/udp.go:169-170)
data := make([]byte, n)  // Allocates new buffer per packet
copy(data, buf[:n])
```

For high packet rates (10,000+ pps), this causes:
- Increased GC pressure
- Higher CPU usage for memory management
- Potential latency spikes during GC pauses

### Proposed Solution

Use `sync.Pool` to recycle buffers:

```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, maxPacketSize)
    },
}

// In readLoop:
data := bufferPool.Get().([]byte)[:n]
copy(data, buf[:n])
// ... process packet ...
bufferPool.Put(data)  // Return to pool
```

**Estimated effort:** ~50 lines of code, 1 day with testing

### When It Matters

**High-volume scenarios:**
- DNS proxy (1,000-50,000 pps)
- VoIP/RTP relay (continuous small packets)
- IoT sensor aggregation (many small packets)
- Syslog relay (burst traffic)

**Low-volume scenarios (most PacketPony users):**
- HTTP/HTTPS proxy (few large packets)
- SSH proxy (moderate traffic)
- Gaming proxy (moderate packet rates)

### Why Not Now

- PacketPony is primarily a **TCP proxy**
- Most UDP use cases have moderate packet rates
- Current implementation works fine
- Would add complexity (buffer lifecycle management, testing)
- **No user complaints about UDP performance**

### When to Implement

Implement when:
- Users report GC-related performance issues with UDP
- Profiling shows buffer allocation as bottleneck
- PacketPony is marketed as "high-performance DNS proxy"

**Recommendation:** Classic premature optimization. Skip until proven necessary.

---

## ❌ UDP Graceful Shutdown (Dismissed)

**Status:** Conceptually flawed  
**Complexity:** 🔴 High  
**Value:** None (misunderstands UDP nature)

### Why This Was Considered

TCP has graceful shutdown with 30s timeout. Should UDP have the same?

### Why This Makes No Sense

UDP is fundamentally different from TCP:

| Feature | TCP | UDP |
|---------|-----|-----|
| Connection state | ✅ ESTABLISHED | ❌ Connectionless |
| In-flight data tracking | ✅ TCP buffers | ❌ No concept of "in-flight" |
| Acknowledgments | ✅ ACKs | ❌ Fire-and-forget |
| Ordered delivery | ✅ Guaranteed | ❌ No guarantees |
| Graceful close protocol | ✅ FIN/ACK | ❌ No close handshake |
| Client notification | ✅ Connection closed | ❌ Client unaware |

**UDP clients expect packet loss** - it's built into the protocol design:
- DNS: 2-5s timeout, automatic retry
- VoIP: Tolerates 1-5% loss, interpolates missing packets
- Gaming: Interpolates/predicts missing packets
- Streaming: Accepts occasional loss

### What "Graceful Shutdown" Would Mean

**Scenario:** PacketPony receives SIGTERM while DNS query is in flight:

```
1. Client → DNS query → PacketPony → DNS server
2. PacketPony receives SIGTERM
3. Wait for response? (How long? 1s? 10s?)
4. But new packets keep arriving...
5. When do we stop accepting new packets?
6. DNS client has its own timeout (2-5s) anyway
```

**There is no good answer** - UDP has no concept of "pending work".

### Conclusion

**Do not implement.** UDP's design assumes packet loss. Clients have retry logic. Attempting graceful shutdown for UDP misunderstands the protocol fundamentals.

---

## ❌ Protocol Detection/Validation (Dismissed)

**Status:** Over-engineering  
**Complexity:** 🔴🔴 Very High  
**Value:** Low (better tools exist)

### What This Would Be

```yaml
listeners:
  - name: "ssh-proxy"
    protocol: "tcp"
    protocol_validation:
      enabled: true
      expect: "ssh"           # Expect "SSH-2.0-..."
      action: "drop"          # Drop if not SSH
```

### Why It Seems Appealing

- Prevent port scanning
- Ensure SSH-only on SSH port
- Detect misconfigured clients
- Anti-tunneling (block SSH over HTTP port)

### Why It's Wrong for PacketPony

**1. Breaks PacketPony's philosophy**

PacketPony is a **transparent L4 forwarder**:
- Simple
- Low-latency (no inspection delay)
- Protocol-agnostic
- Focused on rate limiting + ACL

Protocol detection makes it an **L7 proxy** - a different tool category.

**2. High complexity, low value**

Would require:
- Buffering initial bytes (adds latency)
- Protocol signatures for 10-20 protocols
- Handling partial reads
- Signature database maintenance
- Handling encrypted protocols (TLS)
- Testing all protocol combinations

**Estimated effort:** 500-1,000 lines of code, 1-2 weeks

**3. Fails with modern traffic**

- ❌ HTTPS: Only sees TLS handshake, not HTTP
- ❌ SSH over TLS: Only sees TLS
- ❌ WebSockets: Starts as HTTP, upgrades to WS
- ❌ STARTTLS: SMTP/IMAP upgrade to TLS mid-stream

**4. Better tools exist**

For protocol validation:
- **iptables/nftables** with L7 filter modules
- **HAProxy** - L7 proxy with protocol awareness
- **nginx** - Reverse proxy with SNI/protocol detection
- **Envoy** - Modern L7 proxy

For protocol analysis:
- **tcpdump/Wireshark** - Deep packet inspection
- **Zeek (Bro)** - Network security monitoring
- **Suricata** - IDS with protocol detection

### Conclusion

**Do not implement.** Keep PacketPony as a simple, transparent L4 proxy. Use HAProxy/nginx if you need L7 features.

---

## ⏸️ Windows Event Log Integration (Wait for Demand)

**Status:** Nice-to-have for Windows users  
**Complexity:** 🟡 Medium  
**Value:** Low (few Windows users expected)

### Current Situation

On Windows:
- Syslog is disabled (`syslog_windows.go` returns error)
- Stdout logging writes to files via NSSM:
  - `C:\ProgramData\PacketPony\logs\stdout.log`
  - `C:\ProgramData\PacketPony\logs\stderr.log`
- Windows Event Log only shows NSSM service events (start/stop/crash), not PacketPony logs

### Proposed Enhancement

Create `eventlog_windows.go` using Windows Event Log API:

```go
import "golang.org/x/sys/windows/svc/eventlog"

type EventLogLogger struct {
    log *eventlog.Log
}

func NewEventLogLogger(source string) (*EventLogLogger, error) {
    // Register event source
    err := eventlog.InstallAsEventCreate(source, eventlog.Info|eventlog.Warning|eventlog.Error)
    
    log, err := eventlog.Open(source)
    // ...
}
```

Configuration:
```yaml
logging:
  eventlog:
    enabled: true
    source: "PacketPony"
```

Benefits:
- Native Windows logging
- Integration with Event Viewer
- SIEM/log forwarding via Windows mechanisms
- Standard Windows service behavior

**Estimated effort:** ~200 lines, 1 day with testing

### Why Wait

PacketPony is network proxy/forwarder inspired by **Linux tools** (redir, xinetd):
- Primary use case: **Linux servers**
- Docker/Kubernetes deployments
- SystemD-based systems

Windows support is "nice-to-have" for:
- Testing/development on Windows machines
- Edge cases with Windows Server

**Current Windows logging (NSSM file logging) is good enough** for the few Windows users.

### When to Implement

Implement when:
- Significant Windows user base emerges
- Users specifically request Event Log integration
- Windows becomes a primary deployment target

**Recommendation:** Wait for Windows users to request this feature.

---

## ⏸️ macOS launchd Service Support (Wait for Demand)

**Status:** Development/testing use case only  
**Complexity:** 🟡 Medium  
**Value:** Low (macOS is not a production deployment target)

### Current Situation

PacketPony builds for macOS (both Intel and Apple Silicon):
- ✅ Binaries available in releases
- ✅ Syslog works (uses `log/syslog` package)
- ✅ Can run directly: `sudo packetpony -config config.yaml`

**What's missing:**
- No launchd service configuration
- No macOS deployment guide
- No Homebrew formula

### Typical macOS Use Cases

**Development/Testing:** ✅ Supported
```bash
# Download binary
wget https://github.com/espegro/packetpony/releases/download/v1.0.0/packetpony_1.0.0_Darwin_arm64.tar.gz
tar xzf packetpony_1.0.0_Darwin_arm64.tar.gz
sudo mv packetpony /usr/local/bin/

# Run in foreground (perfect for testing)
sudo packetpony -config config.yaml

# Run in background
nohup sudo packetpony -config config.yaml > packetpony.log 2>&1 &
```

**Production Deployment:** ❌ Not recommended
- Production servers run Linux (systemd)
- Docker/Kubernetes deployments are Linux-based
- macOS Server is rare (desktop OS, not server OS)

### What Could Be Added

**Option 1: Minimal Documentation** (~30 minutes)

Create `deployment/macos/README.md` with:
- Download and installation steps
- Running in foreground/background
- Basic troubleshooting
- Note: "For production, use Linux"

**Option 2: launchd Service** (~2-3 hours)

Create launchd plist file:
```xml
<!-- /Library/LaunchDaemons/com.packetpony.plist -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.packetpony</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/packetpony</string>
        <string>-config</string>
        <string>/usr/local/etc/packetpony/config.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/usr/local/var/log/packetpony/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/usr/local/var/log/packetpony/stderr.log</string>
</dict>
</plist>
```

With install/uninstall scripts and documentation.

**Option 3: Homebrew Formula** (~1 day + maintenance)

Create Homebrew tap with formula for easy installation:
```bash
brew install packetpony
brew services start packetpony
```

Requires:
- Homebrew tap repository
- Formula maintenance
- CI for Homebrew testing
- Ongoing version updates

### Why Wait

1. **macOS is primarily for development/testing**
   - Direct execution is sufficient
   - No need for service management in dev environments

2. **Production deployments are Linux-based**
   - Docker containers (Linux)
   - Kubernetes clusters (Linux)
   - Cloud VMs (Linux)
   - On-premises servers (Linux)

3. **Current solution works**
   - Binary downloads work
   - Can run directly without service wrapper
   - Background execution available via nohup/screen

4. **Low demand**
   - No requests for macOS service support
   - Network proxies are typically server-side tools
   - macOS Server market is tiny

### When to Implement

Consider implementing when:
- Multiple users request macOS service support
- Production deployments on macOS Server emerge
- macOS becomes a common development platform for contributors

**Recommendation:**
- Skip service support for now
- Current binary downloads are sufficient for testing/development
- Production users should deploy on Linux

---

## 🟢 Enhancements Worth Considering (Low Effort)

These are small improvements that could be added easily if requested:

### 1. Connection Draining on Shutdown

**Problem:** Connections drop immediately on SIGTERM  
**Solution:** Wait briefly for connections to complete naturally

```yaml
server:
  shutdown_timeout: "30s"
  drain_period: "5s"  # Stop accepting new, wait 5s for active to finish
```

**Complexity:** 🟢 Low (~50 lines)  
**Status:** Nice-to-have, not critical

### 2. Metrics Enhancements

**Add more detailed metrics:**
- Histogram for packet/message sizes
- Gauge for current bandwidth usage per listener
- Rate limit violations by IP (top offenders)

**Complexity:** 🟢 Low (~100 lines)  
**Status:** Can be added when users request specific metrics

### 3. Health Check Customization

**Allow custom health check logic:**

```yaml
health:
  custom_checks:
    - name: "backend-reachable"
      type: "tcp-dial"
      address: "backend.example.com:80"
      timeout: "2s"
```

**Complexity:** 🟡 Medium (~200 lines)  
**Status:** Only if users need advanced health checks

### 4. Configuration Validation Tool

**Standalone tool to validate config before deployment:**

```bash
packetpony validate -config /etc/packetpony/config.yaml
# ✓ Configuration is valid
# ✓ All listeners can bind to specified ports
# ✓ All target addresses are resolvable
# ⚠ Warning: listener 'ssh' has very restrictive rate limits
```

**Complexity:** 🟢 Low (uses existing validation)  
**Status:** Useful for CI/CD pipelines

---

## 📝 Contributing Future Enhancements

If you want to implement any of these features:

1. **Check the status** - Don't implement dismissed features
2. **Open an issue first** - Discuss design before coding
3. **Start small** - Implement minimal viable version
4. **Write tests** - Must include tests for new features
5. **Update docs** - README, examples, and this file

For "wait for demand" features, provide:
- Real-world use case
- Why existing alternatives don't work
- Expected usage patterns

---

## 🎯 Project Philosophy

PacketPony aims to be:
- ✅ **Simple** - Easy to understand and configure
- ✅ **Focused** - L4 transparent proxy with rate limiting
- ✅ **Reliable** - Stable, well-tested, production-ready
- ✅ **Efficient** - Low overhead, high performance

**We resist feature creep.** New features must have:
- Clear use case from real users
- Measurable value
- Reasonable complexity
- Alignment with project philosophy

**When in doubt, keep it simple.**

---

*Last updated: 2026-04-11*  
*Next review: When v2.0.0 is planned*
