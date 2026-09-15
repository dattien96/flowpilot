# FlowPilot Install Script for Windows PowerShell
# Usage: irm https://raw.githubusercontent.com/dattien96/flowpilot/main/scripts/install.ps1 | iex

$ErrorActionPreference = 'Stop'

$Repo = "dattien96/flowpilot"
$InstallDir = Join-Path $HOME ".flowpilot\bin"

Write-Host "==> Detecting architecture..." -ForegroundColor Cyan
$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
if ($Arch -ne "amd64") {
    Write-Error "Error: 32-bit Windows is not supported. FlowPilot requires Windows 64-bit."
    exit 1
}

Write-Host "==> Fetching latest release info from GitHub..." -ForegroundColor Cyan
try {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    $LatestTag = $Release.tag_name
} catch {
    $LatestTag = "v1.0.0"
}

$Version = $LatestTag.TrimStart("v")
$ArchiveName = "flowpilot_${Version}_windows_${Arch}.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$LatestTag/$ArchiveName"

Write-Host "==> Downloading FlowPilot $LatestTag for windows/amd64..." -ForegroundColor Cyan
$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $TempDir -Force | Out-Null
$ZipPath = Join-Path $TempDir $ArchiveName

try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $ZipPath -UseBasicParsing
} catch {
    Write-Error "Failed to download $DownloadUrl. Check releases: https://github.com/$Repo/releases"
    exit 1
}

Write-Host "==> Installing binary to $InstallDir..." -ForegroundColor Cyan
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
Expand-Archive -Path $ZipPath -DestinationPath $TempDir -Force
Copy-Item -Path (Join-Path $TempDir "flowpilot.exe") -Destination (Join-Path $InstallDir "flowpilot.exe") -Force
Remove-Item -Path $TempDir -Recurse -Force

Write-Host "`n✅ FlowPilot installed successfully to $InstallDir\flowpilot.exe" -ForegroundColor Green

# Add to User PATH if missing
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -split ";" -notcontains $InstallDir) {
    Write-Host "==> Adding $InstallDir to User PATH..." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    $env:Path += ";$InstallDir"
    Write-Host "Please restart your terminal to reload the updated PATH." -ForegroundColor Yellow
}

Write-Host "`nRun 'flowpilot --help' or 'flowpilot chat .' to get started!" -ForegroundColor Cyan
