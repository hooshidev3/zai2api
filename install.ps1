# install.ps1 — zai2api installer for Windows
# Usage (PowerShell as Admin): iwr -useb https://raw.githubusercontent.com/hooshidev3/zai2api/main/install.ps1 | iex

$ErrorActionPreference = "Stop"
$Repo = "hooshidev3/zai2api"
$InstallDir = "C:\Program Files\zai2api"
$DataDir = "C:\ProgramData\zai2api"

Write-Host "=== zai2api Installer (Windows) ===" -ForegroundColor Cyan

# Detect architecture
$Arch = if ([System.Environment]::Is64BitOperatingSystem) { "amd64" } else { "x86" }
Write-Host "Platform: windows/$Arch"

# Get latest release
Write-Host "Fetching latest release..."
$Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
$Tag = $Release.tag_name
Write-Host "Version: $Tag"

# Download URL
$DownloadUrl = "https://github.com/$Repo/releases/download/$Tag/zai2api_windows_$Arch.zip"
$TempFile = "$env:TEMP\zai2api.zip"
$TempDir = "$env:TEMP\zai2api-extract"

Write-Host "Downloading from: $DownloadUrl"
Invoke-WebRequest -Uri $DownloadUrl -OutFile $TempFile -UseBasicParsing

# Extract
if (Test-Path $TempDir) { Remove-Item $TempDir -Recurse -Force }
Expand-Archive -Path $TempFile -DestinationPath $TempDir -Force

# Install
Write-Host "Installing to $InstallDir"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item "$TempDir\zai2api-windows-$Arch.exe" "$InstallDir\zai2api.exe" -Force

# Data directory
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null

# Config
$EnvFile = "$DataDir\.env"
if (-not (Test-Path $EnvFile)) {
    @"
GATEWAY_TOKEN=sk-merged-change-me
ACCOUNTS_DB=$DataDir\accounts.sqlite
GLM_CAPTCHA_DB=$DataDir\tokens.sqlite
ZAI_STRATEGY=round-robin
"@ | Out-File -FilePath $EnvFile -Encoding ASCII
    Write-Host "Config created at $EnvFile (edit GATEWAY_TOKEN!)"
}

# Add to PATH
$CurrentPath = [System.Environment]::GetEnvironmentVariable("Path", "Machine")
if ($CurrentPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable("Path", "$CurrentPath;$InstallDir", "Machine")
    Write-Host "Added to system PATH."
}

# Install Windows Service (optional)
$InstallService = Read-Host "Install as Windows Service? (y/N)"
if ($InstallService -eq "y" -or $InstallService -eq "Y") {
    $ServiceName = "zai2api"
    $ExistingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($ExistingService) {
        Stop-Service $ServiceName -Force -ErrorAction SilentlyContinue
        sc.exe delete $ServiceName | Out-Null
    }

    sc.exe create $ServiceName binPath= "`"$InstallDir\zai2api.exe`"" start= auto | Out-Null
    sc.exe description $ServiceName "zai2api Unified AI Gateway" | Out-Null
    Start-Service $ServiceName
    Write-Host "Service installed and started." -ForegroundColor Green
}

# Cleanup
Remove-Item $TempFile -Force -ErrorAction SilentlyContinue
Remove-Item $TempDir -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "Installation complete!" -ForegroundColor Green
Write-Host "  Binary: $InstallDir\zai2api.exe"
Write-Host "  Data:   $DataDir"
Write-Host "  Config: $DataDir\.env"
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. Edit config: notepad $DataDir\.env"
Write-Host "  2. Start:       zai2api"
Write-Host "  3. Dashboard:   http://localhost:8080"
