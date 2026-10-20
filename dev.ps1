param(
    [string]$DataDir
)

$ErrorActionPreference = 'Stop'
$PreviousData = $env:CAF_DATA_DIR
Push-Location $PSScriptRoot
try {
    # Explicit argument > inherited environment > project-local development data.
    if ([string]::IsNullOrWhiteSpace($DataDir)) { $DataDir = $PreviousData }
    if ([string]::IsNullOrWhiteSpace($DataDir)) {
        $DataDir = Join-Path $PSScriptRoot 'build/dev-data'
    }
    # Resolve script overrides against the project, not Wails' temporary EXE.
    if (-not [System.IO.Path]::IsPathRooted($DataDir)) {
        $DataDir = Join-Path $PSScriptRoot $DataDir
    }
    $env:CAF_DATA_DIR = [System.IO.Path]::GetFullPath($DataDir)
    & go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 dev
    if ($LASTEXITCODE -ne 0) { throw 'Wails dev failed' }
} finally {
    $env:CAF_DATA_DIR = $PreviousData
    Pop-Location
}
