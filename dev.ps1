$ErrorActionPreference = 'Stop'
$PreviousData = $env:CAF_DATA_DIR
Push-Location $PSScriptRoot
try {
    if (-not $env:CAF_DATA_DIR) { $env:CAF_DATA_DIR = Join-Path $PSScriptRoot 'data' }
    & go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 dev
    if ($LASTEXITCODE -ne 0) { throw 'Wails dev failed' }
} finally {
    $env:CAF_DATA_DIR = $PreviousData + "\build"
    Pop-Location
}
