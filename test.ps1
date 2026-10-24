$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    # go:embed 需要 frontend/dist 存在；缺失（如新克隆）时先构建前端。
    if (-not (Test-Path (Join-Path $PSScriptRoot 'frontend/dist/index.html'))) {
        Push-Location (Join-Path $PSScriptRoot 'frontend')
        try {
            & npm install
            if ($LASTEXITCODE -ne 0) { throw 'npm install failed' }
            & npm run build
            if ($LASTEXITCODE -ne 0) { throw 'npm run build failed' }
        } finally {
            Pop-Location
        }
    }
    & go test -count=1 ./tests
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    & go vet -tags bindings ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    & (Join-Path $PSScriptRoot 'tests/development.test.ps1')
} finally {
    Pop-Location
}
