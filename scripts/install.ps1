param(
    [switch]$AddToPath
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

& (Join-Path $Root "scripts\build.ps1")
if ($LASTEXITCODE -ne 0) { throw "build failed" }

$Destination = Join-Path $env:LOCALAPPDATA "Programs\ffbatch"
New-Item -ItemType Directory -Force -Path $Destination | Out-Null
Copy-Item -Force (Join-Path $Root "dist\ffbatch.exe") (Join-Path $Destination "ffbatch.exe")

Write-Host ""
Write-Host "Installed to $Destination\ffbatch.exe" -ForegroundColor Green

if ($AddToPath) {
    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $Parts = @()
    if ($UserPath) { $Parts = $UserPath.Split(";", [System.StringSplitOptions]::RemoveEmptyEntries) }
    if ($Parts -notcontains $Destination) {
        $NewPath = (($Parts + $Destination) -join ";")
        [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
        Write-Host "Added $Destination to the user PATH. Open a new terminal to use 'ffbatch'." -ForegroundColor Green
    } else {
        Write-Host "$Destination is already on the user PATH." -ForegroundColor Yellow
    }
} else {
    Write-Host "Add this folder to PATH, or rerun with -AddToPath:"
    Write-Host "  $Destination"
}

Write-Host ""
Write-Host "Next: ffbatch doctor && ffbatch serve --open"
