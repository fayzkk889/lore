param(
    [switch]$PurgeData
)

$ErrorActionPreference = "Stop"
$InstallDir = Join-Path $env:LOCALAPPDATA "lore"
$Executable = Join-Path $InstallDir "lore.exe"

if (Test-Path -LiteralPath $Executable) {
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
