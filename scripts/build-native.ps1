param([ValidateSet('Release', 'Debug')][string]$Configuration = 'Release')
$ErrorActionPreference = 'Stop'
$Root = Split-Path $PSScriptRoot -Parent
$Source = Join-Path $Root 'build/native-src'
$Archive = Join-Path $Root 'build/native-src.zip'
$Publish = Join-Path $Root 'build/native-publish/win-x64'
Push-Location $Root
try {
    & git submodule update --init --recursive
    if ($LASTEXITCODE -ne 0) { throw 'Submodule initialization failed' }
    $Dirty = & git -C lagrange status --porcelain
    if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect Lagrange source state' }
    if ($Dirty) { throw 'Lagrange has local changes. Preserve them as a patch first; this build uses the pinned commit.' }
    $Commit = & git -C lagrange rev-parse HEAD
    if ($LASTEXITCODE -ne 0) { throw 'Cannot read Lagrange source commit' }
    New-Item -ItemType Directory -Force (Join-Path $Root 'build') | Out-Null
    if (Test-Path $Source) { Remove-Item -LiteralPath $Source -Recurse -Force }
    & git -C lagrange archive --format=zip "--output=$Archive" HEAD
    if ($LASTEXITCODE -ne 0) { throw 'Source archive failed' }
    Expand-Archive -LiteralPath $Archive -DestinationPath $Source -Force
    foreach ($Patch in (Get-ChildItem (Join-Path $Root 'patches') -Filter '*.patch' | Sort-Object Name)) {
        & git apply --check --directory=build/native-src $Patch.FullName
        if ($LASTEXITCODE -ne 0) { throw "Patch does not apply: $($Patch.Name)" }
        & git apply --directory=build/native-src $Patch.FullName
        if ($LASTEXITCODE -ne 0) { throw "Patch failed: $($Patch.Name)" }
    }
    & dotnet publish (Join-Path $Source 'Lagrange.Core.NativeAPI/Lagrange.Core.NativeAPI.csproj') -c $Configuration -r win-x64 -p:NativeLib=Shared -o $Publish --disable-build-servers
    if ($LASTEXITCODE -ne 0) { throw 'NativeAOT publish failed' }
    $Destination = Join-Path $Root 'natives/win-x64/Lagrange.Core.NativeAPI.dll'
    Copy-Item -LiteralPath (Join-Path $Publish 'Lagrange.Core.NativeAPI.dll') -Destination $Destination -Force
    $MetadataPath = Join-Path $Root 'natives/PROVENANCE.json'
    $Metadata = Get-Content -LiteralPath $MetadataPath -Raw | ConvertFrom-Json
    $Metadata.commit = $Commit.Trim()
    $Metadata.bytes = (Get-Item -LiteralPath $Destination).Length
    $Metadata.sha256 = (Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash.ToLowerInvariant()
    $Metadata.note = 'Built locally from the pinned submodule commit plus patches/*.patch; no PDB is bundled.'
    $Json = $Metadata | ConvertTo-Json -Depth 5
    [System.IO.File]::WriteAllText($MetadataPath, $Json + [Environment]::NewLine, (New-Object System.Text.UTF8Encoding($false)))
    Write-Host 'Updated natives/win-x64/Lagrange.Core.NativeAPI.dll. Rebuild CAF to embed it.'
} finally {
    Pop-Location
}
