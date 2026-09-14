param(
    [switch]$PurgeData,
    [string]$InstallDir = $(if ($env:LORE_INSTALL_DIR) { $env:LORE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "lore" }),
    [switch]$SkipDisconnect
)

$ErrorActionPreference = "Stop"
$Executable = Join-Path $InstallDir "lore.exe"

if (-not $SkipDisconnect -and (Test-Path -LiteralPath $Executable)) {
    & $Executable disconnect --all
}

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath) {
    $parts = $UserPath -split ";" | Where-Object {
        $_ -and -not [string]::Equals($_.TrimEnd("\\"), $InstallDir.TrimEnd("\\"), [StringComparison]::OrdinalIgnoreCase)
    }
    [Environment]::SetEnvironmentVariable("Path", ($parts -join ";"), "User")
}

if (Test-Path -LiteralPath $InstallDir) {
    Remove-Item -LiteralPath $InstallDir -Recurse -Force
}

if ($PurgeData) {
    $DataDir = Join-Path $HOME ".lore"
    if (Test-Path -LiteralPath $DataDir) {
        Remove-Item -LiteralPath $DataDir -Recurse -Force
    }
    Write-Host "Lore and local Lore data were removed." -ForegroundColor Green
} else {
    Write-Host "Lore was removed. Local memory remains in ~/.lore." -ForegroundColor Green
}
