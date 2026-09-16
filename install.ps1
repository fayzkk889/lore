param(
    [string]$Version = $(if ($env:LORE_VERSION) { $env:LORE_VERSION } else { "0.10.0-alpha.7" }),
    [string]$InstallDir = $(if ($env:LORE_INSTALL_DIR) { $env:LORE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "lore" }),
    [switch]$NoPath,
    [string]$ExpectedPublisher = ''
)

$ErrorActionPreference = "Stop"

$Repo = "fayzkk889/lore"

# Detect architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Error "32-bit systems are not supported"; exit 1
}

$Filename = "lore_${Version}_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/v$Version/$Filename"
$ChecksumsUrl = "https://github.com/$Repo/releases/download/v$Version/checksums.txt"
$TempDir = Join-Path $env:TEMP ("lore-install-" + [guid]::NewGuid().ToString("N"))

Write-Host "Downloading Lore $Version for Windows/$Arch..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path $TempDir | Out-Null
try {
    Invoke-WebRequest -Uri $Url -OutFile (Join-Path $TempDir $Filename)
    Invoke-WebRequest -Uri $ChecksumsUrl -OutFile (Join-Path $TempDir "checksums.txt")

    Write-Host "Verifying checksum..." -ForegroundColor Cyan
    $ExpectedLines = @(Get-Content (Join-Path $TempDir "checksums.txt") | Where-Object { $_ -match "^[a-fA-F0-9]{64}\s+$([regex]::Escape($Filename))$" })
    if ($ExpectedLines.Count -ne 1) {
        throw "Expected exactly one valid checksum for $Filename"
    }
    $ExpectedLine = $ExpectedLines[0]
    $Expected = ($ExpectedLine -split "\s+")[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 (Join-Path $TempDir $Filename)).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) {
        throw "Checksum verification failed"
    }

    Write-Host "Extracting..." -ForegroundColor Cyan
    Expand-Archive -Path (Join-Path $TempDir $Filename) -DestinationPath $TempDir -Force

    $Candidate = Join-Path $TempDir 'lore.exe'
    if ($ExpectedPublisher) {
        $Signature = Get-AuthenticodeSignature -LiteralPath $Candidate
        if ($Signature.Status -ne 'Valid' -or $Signature.SignerCertificate.Subject -ne $ExpectedPublisher -or -not $Signature.TimeStamperCertificate) {
            throw 'The downloaded binary does not have the expected valid, timestamped publisher signature'
        }
    }
    try {
        $CandidateVersion = (& $Candidate --version 2>&1 | Out-String)
        if ($LASTEXITCODE -ne 0 -or -not $CandidateVersion.Contains("lore version $Version")) { throw 'Downloaded binary returned an unexpected version' }
    } catch {
        throw "Lore could not run before installation. An existing installation was left unchanged. Check Windows protection or managed-device policy. Details: $_"
    }

    Write-Host "Installing to $InstallDir..." -ForegroundColor Cyan
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item (Join-Path $TempDir "lore.exe") -Destination $InstallDir -Force

    # Add one exact path entry. Substring matching breaks for folders such as
    # C:\Tools and C:\Tools-Old.
    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $PathParts = @($UserPath -split ";" | Where-Object { $_ })
    $AlreadyOnPath = $PathParts | Where-Object {
        [string]::Equals($_.TrimEnd("\\"), $InstallDir.TrimEnd("\\"), [StringComparison]::OrdinalIgnoreCase)
    }
    if (-not $NoPath -and -not $AlreadyOnPath) {
        [Environment]::SetEnvironmentVariable("Path", (@($PathParts) + $InstallDir -join ";"), "User")
        Write-Host "Added $InstallDir to your PATH." -ForegroundColor Green
    }
} finally {
    if (Test-Path -LiteralPath $TempDir) {
        Remove-Item -LiteralPath $TempDir -Recurse -Force
    }
}

Write-Host ""
Write-Host "Lore $Version installed successfully!" -ForegroundColor Green
if ($NoPath) {
    Write-Host "Run $InstallDir\lore.exe connect to connect memory." -ForegroundColor Cyan
} else {
    Write-Host "Restart your terminal, then connect memory with: lore connect" -ForegroundColor Cyan
}
Write-Host "No API key is needed for memory. Lore's original coding agent is configured separately." -ForegroundColor DarkGray
