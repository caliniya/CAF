$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    & go test -tags bindings -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    & go vet -tags bindings ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    if (Get-Command node -ErrorAction SilentlyContinue) {
        & node --test tests/frontend.test.mjs
        if ($LASTEXITCODE -ne 0) { throw 'Frontend tests failed' }
    } else {
        Write-Host 'Node is optional; frontend unit tests skipped because it is not installed.'
    }
} finally {
    Pop-Location
}
