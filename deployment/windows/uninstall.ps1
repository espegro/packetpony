# PacketPony Windows Uninstallation Script
# Run as Administrator

param(
    [string]$ServiceName = "PacketPony",
    [switch]$KeepConfig,
    [switch]$Force
)

$ErrorActionPreference = "Stop"

Write-Host "=== PacketPony Windows Uninstallation ===" -ForegroundColor Cyan
Write-Host ""

# Check if running as Administrator
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "ERROR: This script must be run as Administrator" -ForegroundColor Red
    exit 1
}

# Confirm unless -Force specified
if (-not $Force) {
    Write-Host "This will remove PacketPony and all associated files." -ForegroundColor Yellow
    if (-not $KeepConfig) {
        Write-Host "Configuration and logs will also be removed." -ForegroundColor Yellow
    }
    Write-Host ""
    $response = Read-Host "Are you sure you want to continue? (yes/no)"
    if ($response -ne "yes") {
        Write-Host "Uninstallation cancelled." -ForegroundColor Gray
        exit 0
    }
}

# Stop and remove service
Write-Host "[1/5] Stopping and removing service..." -ForegroundColor Green
$service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($service) {
    try {
        # Try NSSM first
        if (Get-Command nssm -ErrorAction SilentlyContinue) {
            & nssm stop $ServiceName
            Start-Sleep -Seconds 2
            & nssm remove $ServiceName confirm
            Write-Host "  Service removed with NSSM" -ForegroundColor Gray
        } else {
            # Fallback to sc.exe
            Stop-Service $ServiceName -Force
            Start-Sleep -Seconds 2
            & sc.exe delete $ServiceName
            Write-Host "  Service removed with sc.exe" -ForegroundColor Gray
        }
    } catch {
        Write-Host "  WARNING: Failed to remove service: $($_.Exception.Message)" -ForegroundColor Yellow
    }
} else {
    Write-Host "  Service not found, skipping..." -ForegroundColor Gray
}

# Remove scheduled task (if used instead of service)
Write-Host "[2/5] Checking for scheduled task..." -ForegroundColor Green
$task = Get-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue
if ($task) {
    try {
        Stop-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue
        Unregister-ScheduledTask -TaskName $ServiceName -Confirm:$false
        Write-Host "  Scheduled task removed" -ForegroundColor Gray
    } catch {
        Write-Host "  WARNING: Failed to remove scheduled task" -ForegroundColor Yellow
    }
} else {
    Write-Host "  No scheduled task found" -ForegroundColor Gray
}

# Remove firewall rules
Write-Host "[3/5] Removing firewall rules..." -ForegroundColor Green
try {
    $rules = Get-NetFirewallRule -DisplayName "PacketPony*" -ErrorAction SilentlyContinue
    if ($rules) {
        $rules | Remove-NetFirewallRule
        Write-Host "  Firewall rules removed" -ForegroundColor Gray
    } else {
        Write-Host "  No firewall rules found" -ForegroundColor Gray
    }
} catch {
    Write-Host "  WARNING: Failed to remove firewall rules" -ForegroundColor Yellow
}

# Remove installation files
Write-Host "[4/5] Removing installation files..." -ForegroundColor Green
$installPath = "C:\Program Files\PacketPony"
if (Test-Path $installPath) {
    try {
        Remove-Item $installPath -Recurse -Force
        Write-Host "  Removed: $installPath" -ForegroundColor Gray
    } catch {
        Write-Host "  WARNING: Failed to remove installation directory" -ForegroundColor Yellow
        Write-Host "  You may need to remove it manually: $installPath" -ForegroundColor Yellow
    }
} else {
    Write-Host "  Installation directory not found" -ForegroundColor Gray
}

# Remove configuration and logs
Write-Host "[5/5] Cleaning up configuration..." -ForegroundColor Green
$configPath = "C:\ProgramData\PacketPony"
if (Test-Path $configPath) {
    if ($KeepConfig) {
        Write-Host "  Keeping configuration (--KeepConfig specified)" -ForegroundColor Yellow
        Write-Host "  Location: $configPath" -ForegroundColor Gray
    } else {
        try {
            Remove-Item $configPath -Recurse -Force
            Write-Host "  Removed: $configPath" -ForegroundColor Gray
        } catch {
            Write-Host "  WARNING: Failed to remove configuration directory" -ForegroundColor Yellow
        }
    }
} else {
    Write-Host "  Configuration directory not found" -ForegroundColor Gray
}

Write-Host ""
Write-Host "=== Uninstallation Complete ===" -ForegroundColor Green
Write-Host ""

if ($KeepConfig) {
    Write-Host "Configuration preserved at: $configPath" -ForegroundColor Cyan
}

Write-Host "PacketPony has been removed from this system." -ForegroundColor White
