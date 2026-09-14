param(
    [string]$Version = $(if ($env:LORE_VERSION) { $env:LORE_VERSION } else { "0.10.0-alpha.2" }),
    [string]$InstallDir = $(if ($env:LORE_INSTALL_DIR) { $env:LORE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "lore" }),
    [switch]$NoPath
)

$ErrorActionPreference = "Stop"

$Repo = "fayzkk889/lore"

# Detect architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Error "32-bit systems are not supported"; exit 1
}

$Filename = "lore_${Version}_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/v$Version/$Filename"
$ChecksumsUrl = "https://github.com/$Repo/releases/download/v$Version/checksums.txt"
$TempDir = Join-Path $env:TEMP ("lore-install-" + [guid]::NewGuid().ToString("N"))

Write-Host "Downloading Lore $Version for Windows/$Arch..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path $TempDir | Out-Null
Invoke-WebRequest -Uri $Url -OutFile (Join-Path $TempDir $Filename)
Invoke-WebRequest -Uri $ChecksumsUrl -OutFile (Join-Path $TempDir "checksums.txt")

Write-Host "Verifying checksum..." -ForegroundColor Cyan
$ExpectedLine = Get-Content (Join-Path $TempDir "checksums.txt") | Where-Object { $_ -match "\s$([regex]::Escape($Filename))$" } | Select-Object -First 1
if (-not $ExpectedLine) {
    Write-Error "Checksum for $Filename not found"; exit 1
}
$Expected = ($ExpectedLine -split "\s+")[0].ToLowerInvariant()
$Actual = (Get-FileHash -Algorithm SHA256 (Join-Path $TempDir $Filename)).Hash.ToLowerInvariant()
if ($Actual -ne $Expected) {
    Write-Error "Checksum verification failed"; exit 1
}

Write-Host "Extracting..." -ForegroundColor Cyan
Expand-Archive -Path (Join-Path $TempDir $Filename) -DestinationPath $TempDir -Force

Write-Host "Installing to $InstallDir..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item (Join-Path $TempDir "lore.exe") -Destination $InstallDir -Force

# Add to PATH if not already there
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not $NoPath -and $UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to your PATH." -ForegroundColor Green
}

# Cleanup
Remove-Item -Recurse -Force $TempDir

Write-Host ""
Write-Host "Lore $Version installed successfully!" -ForegroundColor Green
if ($NoPath) {
    Write-Host "Run $InstallDir\lore.exe connect to connect memory." -ForegroundColor Cyan
} else {
    Write-Host "Restart your terminal, then connect memory with: lore connect" -ForegroundColor Cyan
}
Write-Host "No API key is needed for memory. Lore's original coding agent is configured separately." -ForegroundColor DarkGray
