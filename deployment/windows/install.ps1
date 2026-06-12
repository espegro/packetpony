# PacketPony Windows Installation Script
# Run as Administrator

param(
    [string]$InstallPath = "C:\Program Files\PacketPony",
    [string]$ConfigPath = "C:\ProgramData\PacketPony",
    [string]$ServiceName = "PacketPony",
    [switch]$SkipNSSM,
    [switch]$SkipFirewall
)

$ErrorActionPreference = "Stop"

Write-Host "=== PacketPony Windows Installation ===" -ForegroundColor Cyan
Write-Host ""

# Check if running as Administrator
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "ERROR: This script must be run as Administrator" -ForegroundColor Red
    Write-Host "Right-click PowerShell and select 'Run as Administrator'" -ForegroundColor Yellow
    exit 1
}

# Create directories
Write-Host "[1/8] Creating directories..." -ForegroundColor Green
New-Item -Path $InstallPath -ItemType Directory -Force | Out-Null
New-Item -Path $ConfigPath -ItemType Directory -Force | Out-Null
New-Item -Path "$ConfigPath\logs" -ItemType Directory -Force | Out-Null
Write-Host "  Created: $InstallPath" -ForegroundColor Gray
Write-Host "  Created: $ConfigPath" -ForegroundColor Gray

# Download latest release
Write-Host "[2/8] Downloading PacketPony..." -ForegroundColor Green
$latestRelease = "v1.0.0"  # Update this or fetch from GitHub API
$downloadUrl = "https://github.com/espegro/packetpony/releases/download/$latestRelease/packetpony_${latestRelease}_Windows_x86_64.zip"
$zipFile = "$env:TEMP\packetpony.zip"

try {
    Invoke-WebRequest -Uri $downloadUrl -OutFile $zipFile -UseBasicParsing
    Write-Host "  Downloaded: $zipFile" -ForegroundColor Gray
} catch {
    Write-Host "  ERROR: Failed to download PacketPony" -ForegroundColor Red
    Write-Host "  $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# Extract files
Write-Host "[3/8] Extracting files..." -ForegroundColor Green
Expand-Archive -Path $zipFile -DestinationPath $InstallPath -Force
Write-Host "  Extracted to: $InstallPath" -ForegroundColor Gray

# Copy example config if config doesn't exist
if (-not (Test-Path "$ConfigPath\config.yaml")) {
    Write-Host "[4/8] Creating configuration..." -ForegroundColor Green
    if (Test-Path "$InstallPath\configs\example.yaml") {
        Copy-Item "$InstallPath\configs\example.yaml" -Destination "$ConfigPath\config.yaml"
        Write-Host "  Created: $ConfigPath\config.yaml" -ForegroundColor Gray
        Write-Host "  IMPORTANT: Edit this file before starting the service!" -ForegroundColor Yellow
    } else {
        Write-Host "  WARNING: Example config not found, you'll need to create config.yaml manually" -ForegroundColor Yellow
    }
} else {
    Write-Host "[4/8] Configuration exists, skipping..." -ForegroundColor Green
    Write-Host "  Using existing: $ConfigPath\config.yaml" -ForegroundColor Gray
}
New-Item -ItemType Directory -Force -Path "$ConfigPath\config.d" | Out-Null

# Install NSSM
if (-not $SkipNSSM) {
    Write-Host "[5/8] Installing NSSM (Service Manager)..." -ForegroundColor Green

    # Check if nssm already exists
    $nssmPath = "C:\Windows\System32\nssm.exe"
    if (Test-Path $nssmPath) {
        Write-Host "  NSSM already installed" -ForegroundColor Gray
    } else {
        $nssmUrl = "https://nssm.cc/release/nssm-2.24.zip"
        $nssmZip = "$env:TEMP\nssm.zip"
        $nssmExtract = "$env:TEMP\nssm"

        try {
            Invoke-WebRequest -Uri $nssmUrl -OutFile $nssmZip -UseBasicParsing
            Expand-Archive -Path $nssmZip -DestinationPath $nssmExtract -Force
            Copy-Item "$nssmExtract\nssm-2.24\win64\nssm.exe" -Destination $nssmPath -Force
            Write-Host "  NSSM installed to: $nssmPath" -ForegroundColor Gray
        } catch {
            Write-Host "  WARNING: Failed to install NSSM automatically" -ForegroundColor Yellow
            Write-Host "  Please download from https://nssm.cc and install manually" -ForegroundColor Yellow
        }
    }

    # Install service with NSSM
    if (Test-Path $nssmPath) {
        Write-Host "[6/8] Installing Windows service..." -ForegroundColor Green

        # Remove existing service if it exists
        $existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
        if ($existingService) {
            Write-Host "  Removing existing service..." -ForegroundColor Gray
            & nssm stop $ServiceName
            & nssm remove $ServiceName confirm
            Start-Sleep -Seconds 2
        }

        # Install service
        & nssm install $ServiceName "$InstallPath\packetpony.exe" -config "$ConfigPath\config.yaml"

        # Configure service
        & nssm set $ServiceName DisplayName "PacketPony Network Proxy"
        & nssm set $ServiceName Description "Modern network proxy with rate limiting and monitoring"
        & nssm set $ServiceName Start SERVICE_AUTO_START
        & nssm set $ServiceName AppStdout "$ConfigPath\logs\stdout.log"
        & nssm set $ServiceName AppStderr "$ConfigPath\logs\stderr.log"
        & nssm set $ServiceName AppRotateFiles 1
        & nssm set $ServiceName AppRotateOnline 1
        & nssm set $ServiceName AppRotateSeconds 86400
        & nssm set $ServiceName AppRotateBytes 10485760
        & nssm set $ServiceName AppExit Default Restart
        & nssm set $ServiceName AppRestartDelay 5000

        Write-Host "  Service installed: $ServiceName" -ForegroundColor Gray
    }
} else {
    Write-Host "[5/8] Skipping NSSM installation (--SkipNSSM specified)" -ForegroundColor Yellow
    Write-Host "[6/8] Skipping service installation" -ForegroundColor Yellow
}

# Configure firewall
if (-not $SkipFirewall) {
    Write-Host "[7/8] Configuring Windows Firewall..." -ForegroundColor Green

    # Remove old rules if they exist
    Get-NetFirewallRule -DisplayName "PacketPony*" -ErrorAction SilentlyContinue | Remove-NetFirewallRule

    # Add new rules (adjust ports based on your config)
    try {
        New-NetFirewallRule -DisplayName "PacketPony - HTTP Proxy" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow -ErrorAction SilentlyContinue | Out-Null
        New-NetFirewallRule -DisplayName "PacketPony - Metrics" -Direction Inbound -LocalPort 9090 -Protocol TCP -Action Allow -ErrorAction SilentlyContinue | Out-Null
        Write-Host "  Firewall rules created (ports 8080, 9090)" -ForegroundColor Gray
        Write-Host "  Adjust rules based on your configuration!" -ForegroundColor Yellow
    } catch {
        Write-Host "  WARNING: Failed to create firewall rules" -ForegroundColor Yellow
    }
} else {
    Write-Host "[7/8] Skipping firewall configuration (--SkipFirewall specified)" -ForegroundColor Yellow
}

# Summary
Write-Host "[8/8] Installation complete!" -ForegroundColor Green
Write-Host ""
Write-Host "=== Next Steps ===" -ForegroundColor Cyan
Write-Host "1. Edit configuration:" -ForegroundColor White
Write-Host "   notepad $ConfigPath\config.yaml" -ForegroundColor Gray
Write-Host ""
Write-Host "2. Start the service:" -ForegroundColor White
Write-Host "   Start-Service $ServiceName" -ForegroundColor Gray
Write-Host "   # or" -ForegroundColor Gray
Write-Host "   nssm start $ServiceName" -ForegroundColor Gray
Write-Host ""
Write-Host "3. Check status:" -ForegroundColor White
Write-Host "   Get-Service $ServiceName" -ForegroundColor Gray
Write-Host ""
Write-Host "4. View logs:" -ForegroundColor White
Write-Host "   Get-Content $ConfigPath\logs\stdout.log -Tail 50 -Wait" -ForegroundColor Gray
Write-Host ""
Write-Host "5. Test health endpoint:" -ForegroundColor White
Write-Host "   Invoke-WebRequest http://localhost:9090/health" -ForegroundColor Gray
Write-Host ""
Write-Host "=== Documentation ===" -ForegroundColor Cyan
Write-Host "Full guide: $InstallPath\deployment\windows\README.md" -ForegroundColor Gray
Write-Host ""

# Offer to edit config
$response = Read-Host "Do you want to edit the configuration now? (Y/N)"
if ($response -eq "Y" -or $response -eq "y") {
    notepad "$ConfigPath\config.yaml"
}

Write-Host "Installation script completed successfully!" -ForegroundColor Green
