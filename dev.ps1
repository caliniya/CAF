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
    # Ensure wails CLI (version from go.mod).
    $WailsExe = Join-Path $env:USERPROFILE 'go\bin\wails.exe'
    if (-not (Test-Path $WailsExe)) {
        $v = (Select-String -Path go.mod -Pattern 'github.com/wailsapp/wails/v2 (v[0-9.]+)').Matches[0].Groups[1].Value
        Write-Host "wails CLI not found, installing $v ..."
        go install "github.com/wailsapp/wails/v2/cmd/wails@$v"
        if ($LASTEXITCODE -ne 0) { throw "go install wails@$v failed" }
    }
    & $WailsExe dev
    if ($LASTEXITCODE -ne 0) { throw 'Wails dev failed' }
} finally {
    $env:CAF_DATA_DIR = $PreviousData
    Pop-Location
}
