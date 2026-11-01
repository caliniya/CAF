$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    & go mod download
    if ($LASTEXITCODE -ne 0) { throw 'go mod download failed' }
    # Ensure wails CLI (version from go.mod).
    $WailsExe = Join-Path $env:USERPROFILE 'go\bin\wails.exe'
    if (-not (Test-Path $WailsExe)) {
        $v = (Select-String -Path go.mod -Pattern 'github.com/wailsapp/wails/v2 (v[0-9.]+)').Matches[0].Groups[1].Value
        Write-Host "wails CLI not found, installing $v ..."
        go install "github.com/wailsapp/wails/v2/cmd/wails@$v"
        if ($LASTEXITCODE -ne 0) { throw "go install wails@$v failed" }
    }
    & $WailsExe build -platform windows/amd64
    if ($LASTEXITCODE -ne 0) { throw 'Wails build failed' }
    Write-Host 'Built: build/bin/CAF.exe'
} finally {
    Pop-Location
}
