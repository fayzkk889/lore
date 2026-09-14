param(
    [switch]$PurgeData,
    [string]$InstallDir = $(if ($env:LORE_INSTALL_DIR) { $env:LORE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "lore" }),
    [string]$DataDir = $(if ($env:LORE_DATA_DIR) { $env:LORE_DATA_DIR } else { Join-Path $HOME ".lore" }),
    [switch]$SkipDisconnect
)

$ErrorActionPreference = "Stop"
$Executable = Join-Path $InstallDir "lore.exe"

if (-not $SkipDisconnect -and (Test-Path -LiteralPath $Executable)) {
    & $Executable disconnect --all
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "Lore could not disconnect every client. The binary will still be removed; inspect client MCP settings if a stale Lore entry remains."
    }
}

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath) {
    $parts = $UserPath -split ";" | Where-Object {
        $_ -and -not [string]::Equals($_.TrimEnd("\\"), $InstallDir.TrimEnd("\\"), [StringComparison]::OrdinalIgnoreCase)
    }
    [Environment]::SetEnvironmentVariable("Path", ($parts -join ";"), "User")
}

if (Test-Path -LiteralPath $Executable) {
    Remove-Item -LiteralPath $Executable -Force
}
if ((Test-Path -LiteralPath $InstallDir) -and -not (Get-ChildItem -LiteralPath $InstallDir -Force | Select-Object -First 1)) {
    Remove-Item -LiteralPath $InstallDir -Force
}

if ($PurgeData) {
    if (Test-Path -LiteralPath $DataDir) {
        Remove-Item -LiteralPath $DataDir -Recurse -Force
    }
    Write-Host "Lore and local Lore data were removed." -ForegroundColor Green
} else {
    Write-Host "Lore was removed. Local memory remains in ~/.lore." -ForegroundColor Green
}
