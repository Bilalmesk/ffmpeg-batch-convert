param(
    [string]$Version = "0.2.1",
    [string]$OutDir = "dist"
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if (-not [System.IO.Path]::IsPathRooted($OutDir)) {
    $OutDir = Join-Path $Root $OutDir
}

Push-Location $Root
try {
    $Commit = "none"
    if (Test-Path (Join-Path $Root ".git")) {
        try { $Commit = (git rev-parse --short HEAD).Trim() } catch { $Commit = "none" }
    }
    $Date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    $Package = "github.com/Bilalmesk/ffmpeg-batch-convert/internal/version"
    $LdFlags = "-s -w -X $Package.Version=$Version -X $Package.Commit=$Commit -X $Package.Date=$Date"

    New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
    $Binary = Join-Path $OutDir "ffbatch.exe"

    Write-Host "Building $Binary ..." -ForegroundColor Cyan
    go build -trimpath -ldflags $LdFlags -o $Binary .
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }

    $Size = [math]::Round((Get-Item $Binary).Length / 1MB, 2)
    Write-Host ("Built {0} ({1} MiB)" -f $Binary, $Size) -ForegroundColor Green
    & $Binary version
} finally {
    Pop-Location
}
