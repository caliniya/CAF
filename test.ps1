$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    & go test -count=1 ./tests
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    & go vet -tags bindings ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    & (Join-Path $PSScriptRoot 'tests/development.test.ps1')
    if (Get-Command node -ErrorAction SilentlyContinue) {
        & node --test tests/frontend.test.mjs
        if ($LASTEXITCODE -ne 0) { throw 'Frontend tests failed' }
    } else {
        Write-Host 'Node is optional; frontend unit tests skipped because it is not installed.'
    }
} finally {
    Pop-Location
}
