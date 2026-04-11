# PacketPony - Windows Deployment Guide

This guide covers deploying PacketPony as a Windows service.

## Table of Contents

- [Quick Start](#quick-start)
- [Installation Methods](#installation-methods)
  - [Method 1: NSSM (Recommended)](#method-1-nssm-recommended)
  - [Method 2: Windows Task Scheduler](#method-2-windows-task-scheduler)
  - [Method 3: Manual Service](#method-3-manual-service)
- [Configuration](#configuration)
- [Firewall Rules](#firewall-rules)
- [Monitoring and Logs](#monitoring-and-logs)
- [Troubleshooting](#troubleshooting)
- [Uninstall](#uninstall)

## Quick Start

**5-minute setup for Windows service:**

```powershell
# 1. Download and extract PacketPony
Invoke-WebRequest -Uri "https://github.com/espegro/packetpony/releases/latest/download/packetpony_1.0.0_Windows_x86_64.zip" -OutFile packetpony.zip
Expand-Archive packetpony.zip -DestinationPath "C:\Program Files\PacketPony"

# 2. Download NSSM
Invoke-WebRequest -Uri "https://nssm.cc/release/nssm-2.24.zip" -OutFile nssm.zip
Expand-Archive nssm.zip -DestinationPath C:\Temp\nssm
Copy-Item "C:\Temp\nssm\nssm-2.24\win64\nssm.exe" -Destination "C:\Windows\System32\"

# 3. Create config directory
New-Item -Path "C:\ProgramData\PacketPony" -ItemType Directory -Force

# 4. Copy example config
Copy-Item "C:\Program Files\PacketPony\configs\example.yaml" -Destination "C:\ProgramData\PacketPony\config.yaml"

# 5. Edit config (open in notepad)
notepad "C:\ProgramData\PacketPony\config.yaml"

# 6. Install as Windows service
nssm install PacketPony "C:\Program Files\PacketPony\packetpony.exe" -config "C:\ProgramData\PacketPony\config.yaml"

# 7. Configure service
nssm set PacketPony DisplayName "PacketPony Network Proxy"
nssm set PacketPony Description "Modern network proxy with rate limiting and monitoring"
nssm set PacketPony Start SERVICE_AUTO_START
nssm set PacketPony AppStdout "C:\ProgramData\PacketPony\logs\stdout.log"
nssm set PacketPony AppStderr "C:\ProgramData\PacketPony\logs\stderr.log"
nssm set PacketPony AppRotateFiles 1
nssm set PacketPony AppRotateBytes 10485760

# 8. Start service
nssm start PacketPony

# 9. Verify
Get-Service PacketPony
```

## Installation Methods

### Method 1: NSSM (Recommended)

**NSSM (Non-Sucking Service Manager)** is the easiest way to run PacketPony as a Windows service.

#### Download and Install

**Option A: Manual Download**

1. Download NSSM from [nssm.cc](https://nssm.cc/download)
2. Extract the ZIP file
3. Copy `nssm.exe` (from `win64` folder) to `C:\Windows\System32`

**Option B: PowerShell**

```powershell
# Download and install NSSM
$nssmUrl = "https://nssm.cc/release/nssm-2.24.zip"
$nssmZip = "$env:TEMP\nssm.zip"
$nssmExtract = "$env:TEMP\nssm"

Invoke-WebRequest -Uri $nssmUrl -OutFile $nssmZip
Expand-Archive $nssmZip -DestinationPath $nssmExtract -Force
Copy-Item "$nssmExtract\nssm-2.24\win64\nssm.exe" -Destination "C:\Windows\System32\" -Force

Write-Host "NSSM installed successfully" -ForegroundColor Green
```

#### Install PacketPony

```powershell
# Install as service
nssm install PacketPony "C:\Program Files\PacketPony\packetpony.exe" -config "C:\ProgramData\PacketPony\config.yaml"

# Configure service properties
nssm set PacketPony DisplayName "PacketPony Network Proxy"
nssm set PacketPony Description "Modern network proxy with advanced rate limiting, ACLs, and Prometheus metrics"
nssm set PacketPony Start SERVICE_AUTO_START

# Configure logging
New-Item -Path "C:\ProgramData\PacketPony\logs" -ItemType Directory -Force
nssm set PacketPony AppStdout "C:\ProgramData\PacketPony\logs\stdout.log"
nssm set PacketPony AppStderr "C:\ProgramData\PacketPony\logs\stderr.log"
nssm set PacketPony AppRotateFiles 1
nssm set PacketPony AppRotateOnline 1
nssm set PacketPony AppRotateSeconds 86400
nssm set PacketPony AppRotateBytes 10485760

# Configure failure recovery
nssm set PacketPony AppExit Default Restart
nssm set PacketPony AppRestartDelay 5000

# Start the service
nssm start PacketPony
```

#### Verify Installation

```powershell
# Check service status
Get-Service PacketPony

# View service details
nssm status PacketPony

# View logs
Get-Content "C:\ProgramData\PacketPony\logs\stdout.log" -Tail 50
```

#### Managing the Service

```powershell
# Start service
nssm start PacketPony
# or
Start-Service PacketPony

# Stop service
nssm stop PacketPony
# or
Stop-Service PacketPony

# Restart service
nssm restart PacketPony
# or
Restart-Service PacketPony

# View status
nssm status PacketPony

# Edit service (opens GUI)
nssm edit PacketPony

# Remove service
nssm remove PacketPony confirm
```

### Method 2: Windows Task Scheduler

For simpler deployments without NSSM:

#### Create Scheduled Task

```powershell
# Define task action
$action = New-ScheduledTaskAction -Execute "C:\Program Files\PacketPony\packetpony.exe" -Argument "-config C:\ProgramData\PacketPony\config.yaml" -WorkingDirectory "C:\Program Files\PacketPony"

# Define trigger (at startup)
$trigger = New-ScheduledTaskTrigger -AtStartup

# Define settings
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)

# Define principal (run as SYSTEM)
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

# Register the task
Register-ScheduledTask -TaskName "PacketPony" -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Description "PacketPony Network Proxy"

# Start the task immediately
Start-ScheduledTask -TaskName "PacketPony"
```

#### Manage Scheduled Task

```powershell
# View task status
Get-ScheduledTask -TaskName "PacketPony" | Get-ScheduledTaskInfo

# Stop task
Stop-ScheduledTask -TaskName "PacketPony"

# Start task
Start-ScheduledTask -TaskName "PacketPony"

# Remove task
Unregister-ScheduledTask -TaskName "PacketPony" -Confirm:$false
```

### Method 3: Manual Service

Run manually in a PowerShell window (useful for testing):

```powershell
# Navigate to installation directory
cd "C:\Program Files\PacketPony"

# Run PacketPony
.\packetpony.exe -config "C:\ProgramData\PacketPony\config.yaml"
```

Press `Ctrl+C` to stop.

## Configuration

### Directory Structure

```
C:\Program Files\PacketPony\      # Installation directory
├── packetpony.exe                # Binary
├── README.md                     # Documentation
├── LICENSE                       # License file
└── configs\
    └── example.yaml              # Example config

C:\ProgramData\PacketPony\        # Configuration and data
├── config.yaml                   # Active configuration
└── logs\                         # Log files (if using NSSM)
    ├── stdout.log
    └── stderr.log
```

### Example Configuration

```yaml
server:
  name: "packetpony-windows"
  shutdown_timeout: "30s"

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
  - name: "http-proxy"
    protocol: "tcp"
    listen_address: "0.0.0.0:8080"
    target_address: "backend.example.com:80"
    allowlist:
      - "0.0.0.0/0"
    rate_limits:
      max_connections_per_ip: 100
      connections_window: "1m"
      max_bandwidth_per_ip: "10MB"
      bandwidth_window: "1m"
      action: "drop"
```

**Note:** Syslog is not available on Windows. Use `stdout` logging instead, which will be captured by NSSM.

### Edit Configuration

```powershell
# Edit config file
notepad "C:\ProgramData\PacketPony\config.yaml"

# Restart service to apply changes
Restart-Service PacketPony
# or
nssm restart PacketPony
```

## Firewall Rules

### Allow Incoming Connections

```powershell
# Allow PacketPony through Windows Firewall
New-NetFirewallRule -DisplayName "PacketPony - HTTP Proxy" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow

# Allow Prometheus metrics endpoint
New-NetFirewallRule -DisplayName "PacketPony - Metrics" -Direction Inbound -LocalPort 9090 -Protocol TCP -Action Allow

# For UDP proxies
New-NetFirewallRule -DisplayName "PacketPony - DNS Proxy" -Direction Inbound -LocalPort 53 -Protocol UDP -Action Allow
```

### Remove Firewall Rules

```powershell
Remove-NetFirewallRule -DisplayName "PacketPony - HTTP Proxy"
Remove-NetFirewallRule -DisplayName "PacketPony - Metrics"
Remove-NetFirewallRule -DisplayName "PacketPony - DNS Proxy"
```

## Monitoring and Logs

### View Logs

**With NSSM:**

```powershell
# View stdout log
Get-Content "C:\ProgramData\PacketPony\logs\stdout.log" -Tail 50 -Wait

# View stderr log (errors)
Get-Content "C:\ProgramData\PacketPony\logs\stderr.log" -Tail 50 -Wait

# View all recent logs
Get-ChildItem "C:\ProgramData\PacketPony\logs\*.log" | ForEach-Object { 
    Write-Host "`n=== $($_.Name) ===" -ForegroundColor Cyan
    Get-Content $_.FullName -Tail 20 
}
```

**Windows Event Log:**

```powershell
# View application events
Get-EventLog -LogName Application -Source "PacketPony" -Newest 50
```

### Health Check

```powershell
# Check if service is running
Get-Service PacketPony

# Test health endpoint
Invoke-WebRequest -Uri "http://localhost:9090/health" | Select-Object -ExpandProperty Content

# Test metrics endpoint
Invoke-WebRequest -Uri "http://localhost:9090/metrics" | Select-Object -ExpandProperty Content
```

### Monitor Performance

```powershell
# View process stats
Get-Process packetpony | Format-List *

# Monitor CPU and memory
Get-Counter "\Process(packetpony)\% Processor Time", "\Process(packetpony)\Working Set - Private"

# View network connections
Get-NetTCPConnection | Where-Object { $_.OwningProcess -eq (Get-Process packetpony).Id }
```

## Troubleshooting

### Service Won't Start

**Check logs:**

```powershell
# View error log
Get-Content "C:\ProgramData\PacketPony\logs\stderr.log"

# Check Windows Event Log
Get-EventLog -LogName Application -Newest 10
```

**Common issues:**

1. **Port already in use**
   ```powershell
   # Find what's using port 8080
   Get-NetTCPConnection -LocalPort 8080 | Select-Object -Property State, OwningProcess
   
   # Find process name
   Get-Process -Id <OwningProcess>
   ```
   Solution: Change port in config or stop the conflicting service.

2. **Config file not found**
   ```powershell
   # Verify config exists
   Test-Path "C:\ProgramData\PacketPony\config.yaml"
   ```
   Solution: Create config file from example.

3. **Permission denied**
   - Solution: Run NSSM installation as Administrator
   - Or grant permissions to PacketPony directory

### Service Crashes

**View crash logs:**

```powershell
Get-Content "C:\ProgramData\PacketPony\logs\stderr.log" -Tail 100
```

**Check service recovery settings:**

```powershell
# View current settings
nssm status PacketPony

# Configure automatic restart on failure
nssm set PacketPony AppExit Default Restart
nssm set PacketPony AppRestartDelay 5000
```

### High CPU/Memory Usage

**Monitor resource usage:**

```powershell
# Real-time monitoring
Get-Process packetpony | Format-Table -AutoSize -Property CPU, PM, WS, Id

# Check active connections
Get-NetTCPConnection | Where-Object { $_.OwningProcess -eq (Get-Process packetpony).Id } | Measure-Object
```

**Solutions:**

- Reduce `max_total_connections` in config
- Increase `connections_window` to reduce cleanup frequency
- Check for connection leaks in Prometheus metrics

### Can't Access from Network

**Verify firewall:**

```powershell
# List PacketPony firewall rules
Get-NetFirewallRule | Where-Object { $_.DisplayName -like "*PacketPony*" }

# Test if port is listening
Test-NetConnection -ComputerName localhost -Port 8080
```

**Check binding:**

- Ensure `listen_address` uses `0.0.0.0` not `127.0.0.1`
- Verify Windows Firewall allows the port

## Uninstall

### Remove Service (NSSM)

```powershell
# Stop service
nssm stop PacketPony

# Remove service
nssm remove PacketPony confirm
```

### Remove Service (Task Scheduler)

```powershell
# Stop and remove task
Stop-ScheduledTask -TaskName "PacketPony"
Unregister-ScheduledTask -TaskName "PacketPony" -Confirm:$false
```

### Remove Files

```powershell
# Remove installation
Remove-Item "C:\Program Files\PacketPony" -Recurse -Force

# Remove configuration and logs
Remove-Item "C:\ProgramData\PacketPony" -Recurse -Force

# Remove firewall rules
Remove-NetFirewallRule -DisplayName "PacketPony*"
```

### Complete Cleanup

```powershell
# Stop and remove service
nssm stop PacketPony
nssm remove PacketPony confirm

# Remove all files
Remove-Item "C:\Program Files\PacketPony" -Recurse -Force
Remove-Item "C:\ProgramData\PacketPony" -Recurse -Force

# Remove firewall rules
Get-NetFirewallRule | Where-Object { $_.DisplayName -like "*PacketPony*" } | Remove-NetFirewallRule

Write-Host "PacketPony completely removed" -ForegroundColor Green
```

## Advanced Configuration

### Run as Specific User

```powershell
# Run as network service account (recommended)
nssm set PacketPony ObjectName "NT AUTHORITY\NetworkService"

# Or run as specific user
nssm set PacketPony ObjectName "DOMAIN\username" "password"
```

### Service Dependencies

```powershell
# Start after network is available
nssm set PacketPony DependOnService Tcpip Dnscache
```

### Environment Variables

```powershell
# Set environment variables for the service
nssm set PacketPony AppEnvironmentExtra "LOG_LEVEL=debug" "CUSTOM_VAR=value"
```

## Integration with Prometheus

### Scrape Configuration

Add to your Prometheus `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'packetpony'
    static_configs:
      - targets: ['localhost:9090']
        labels:
          instance: 'windows-server-01'
          service: 'packetpony'
```

### View Metrics

```powershell
# Open metrics in browser
Start-Process "http://localhost:9090/metrics"

# Fetch metrics via PowerShell
Invoke-WebRequest -Uri "http://localhost:9090/metrics" | Select-Object -ExpandProperty Content
```

## Performance Tuning

### For High-Throughput TCP Proxies

```yaml
listeners:
  - name: "high-performance-proxy"
    protocol: "tcp"
    tcp:
      dial_timeout: "5s"
      copy_buffer_size: 131072  # 128KB
      idle_timeout: "10m"
    rate_limits:
      max_total_connections: 10000
```

### For Low-Latency Proxies

```yaml
listeners:
  - name: "low-latency-proxy"
    protocol: "tcp"
    tcp:
      dial_timeout: "2s"
      copy_buffer_size: 16384  # 16KB
      read_timeout: "5s"
      write_timeout: "5s"
```

## Support

- **GitHub Issues**: https://github.com/espegro/packetpony/issues
- **Documentation**: https://github.com/espegro/packetpony
- **Releases**: https://github.com/espegro/packetpony/releases

## Security Recommendations

1. **Run with minimal privileges**
   - Use `NT AUTHORITY\NetworkService` account
   - Don't run as Administrator unless binding to privileged ports

2. **Restrict firewall rules**
   - Only allow necessary ports
   - Use specific IP ranges instead of `0.0.0.0/0`

3. **Protect configuration**
   ```powershell
   # Restrict access to config file
   $acl = Get-Acl "C:\ProgramData\PacketPony\config.yaml"
   $acl.SetAccessRuleProtection($true, $false)
   $rule = New-Object System.Security.AccessControl.FileSystemAccessRule("BUILTIN\Administrators", "FullControl", "Allow")
   $acl.AddAccessRule($rule)
   Set-Acl "C:\ProgramData\PacketPony\config.yaml" $acl
   ```

4. **Enable logging**
   - Monitor for suspicious activity
   - Rotate logs regularly
   - Forward logs to SIEM if available

5. **Keep updated**
   - Subscribe to GitHub releases
   - Test updates in non-production first
   - Maintain backups of working configurations
