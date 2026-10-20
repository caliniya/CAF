$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    & go mod download
    if ($LASTEXITCODE -ne 0) { throw 'go mod download failed' }
    & go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 build -platform windows/amd64
    if ($LASTEXITCODE -ne 0) { throw 'Wails build failed' }
    Write-Host 'Built: build/bin/CAF.exe'
} finally {
    Pop-Location
}
