$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    & go test -count=1 ./tests
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    & go vet -tags bindings ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    & (Join-Path $PSScriptRoot 'tests/development.test.ps1')
} finally {
    Pop-Location
}
