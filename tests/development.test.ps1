# Headless script/config regressions. Compatible with Windows PowerShell 5.1;
# Mock the installed Wails executable and go install; never start an application.
$ErrorActionPreference = 'Stop'
$ProjectRoot = Split-Path $PSScriptRoot -Parent
$DevScript = Join-Path $ProjectRoot 'dev.ps1'
$SavedData = $env:CAF_DATA_DIR
$SavedExitCode = $global:LASTEXITCODE
$SavedLocation = (Get-Location).Path
$MockWailsExe = Join-Path $env:USERPROFILE 'go\bin\wails.exe'
$WailsVersion = (Select-String -LiteralPath (Join-Path $ProjectRoot 'go.mod') -Pattern 'github.com/wailsapp/wails/v2 (v[0-9.]+)').Matches[0].Groups[1].Value
$MockState = [pscustomobject]@{
    ExitCode = 0
    Throws = $false
    Calls = 0
    ObservedData = $null
    ObservedLocation = $null
    ObservedArgs = $null
    Installed = $true
    InstallExitCode = 0
    InstallCalls = @()
}

function Assert-Equal($Actual, $Expected, [string]$Message) {
    if ($Actual -cne $Expected) {
        throw "${Message}: expected '$Expected', got '$Actual'"
    }
}

function Invoke-MockWails {
    $MockState.Calls++
    $MockState.ObservedData = $env:CAF_DATA_DIR
    $MockState.ObservedLocation = (Get-Location).Path
    $MockState.ObservedArgs = $args -join '|'
    $global:LASTEXITCODE = $MockState.ExitCode
    if ($MockState.Throws) { throw 'mock command failure' }
}

Set-Alias -Name $MockWailsExe -Value Invoke-MockWails

function Test-Path {
    param([string]$Path)
    if ($Path -eq $MockWailsExe) { return $MockState.Installed }
    return Microsoft.PowerShell.Management\Test-Path -LiteralPath $Path
}

function go {
    $MockState.InstallCalls += ($args -join '|')
    $global:LASTEXITCODE = $MockState.InstallExitCode
    if ($MockState.InstallExitCode -eq 0) { $MockState.Installed = $true }
}

function Assert-DevFailure([string]$ExpectedMessage) {
    $failure = $null
    try { & $DevScript } catch { $failure = $_ }
    if ($null -eq $failure -or $failure.Exception.Message -ne $ExpectedMessage) {
        throw "Expected dev script failure: $ExpectedMessage"
    }
}

try {
    # Invoke from outside the project to ensure cwd is not used as data root.
    Set-Location (Split-Path $ProjectRoot -Parent)
    $CallerLocation = (Get-Location).Path
    $DefaultData = [System.IO.Path]::GetFullPath((Join-Path $ProjectRoot 'build/dev-data'))

    $env:CAF_DATA_DIR = $null
    & $DevScript
    Assert-Equal $MockState.ObservedData $DefaultData 'default data directory'
    Assert-Equal $MockState.ObservedLocation $ProjectRoot 'Wails working directory'
    Assert-Equal $MockState.ObservedArgs 'dev' 'installed Wails CLI command'
    Assert-Equal ([bool]$env:CAF_DATA_DIR) $false 'unset environment restoration'
    Assert-Equal (Get-Location).Path $CallerLocation 'caller working directory restoration'

    $InheritedData = [System.IO.Path]::GetFullPath((Join-Path $ProjectRoot 'build/inherited data'))
    $env:CAF_DATA_DIR = $InheritedData
    & $DevScript
    Assert-Equal $MockState.ObservedData $InheritedData 'inherited absolute override'
    Assert-Equal $env:CAF_DATA_DIR $InheritedData 'inherited environment restoration'

    $env:CAF_DATA_DIR = 'build/relative data'
    & $DevScript
    $RelativeData = [System.IO.Path]::GetFullPath((Join-Path $ProjectRoot 'build/relative data'))
    Assert-Equal $MockState.ObservedData $RelativeData 'relative environment resolved against project'
    Assert-Equal $env:CAF_DATA_DIR 'build/relative data' 'original relative environment restoration'

    $env:CAF_DATA_DIR = $InheritedData
    & $DevScript -DataDir 'build/custom data'
    $CustomData = [System.IO.Path]::GetFullPath((Join-Path $ProjectRoot 'build/custom data'))
    Assert-Equal $MockState.ObservedData $CustomData 'explicit relative argument overrides environment'
    Assert-Equal $env:CAF_DATA_DIR $InheritedData 'explicit argument does not leak into caller'

    & $DevScript -DataDir $CustomData
    Assert-Equal $MockState.ObservedData $CustomData 'explicit absolute argument'

    $MockState.ExitCode = 12
    Assert-DevFailure 'Wails dev failed'
    Assert-Equal $env:CAF_DATA_DIR $InheritedData 'environment restored after nonzero exit'
    Assert-Equal (Get-Location).Path $CallerLocation 'cwd restored after nonzero exit'

    $MockState.ExitCode = 0
    $MockState.Throws = $true
    Assert-DevFailure 'mock command failure'
    Assert-Equal $env:CAF_DATA_DIR $InheritedData 'environment restored after exception'
    Assert-Equal (Get-Location).Path $CallerLocation 'cwd restored after exception'
    Assert-Equal $MockState.Calls 7 'mock command invocations'

    $MockState.Throws = $false
    $MockState.Installed = $false
    & $DevScript
    Assert-Equal $MockState.InstallCalls.Count 1 'missing CLI installation'
    Assert-Equal $MockState.InstallCalls[0] "install|github.com/wailsapp/wails/v2/cmd/wails@$WailsVersion" 'CLI version from go.mod'
    Assert-Equal $MockState.Calls 8 'installed CLI runs dev'
    Assert-Equal $env:CAF_DATA_DIR $InheritedData 'environment restored after installation'

    $MockState.Installed = $false
    $MockState.InstallExitCode = 13
    Assert-DevFailure "go install wails@$WailsVersion failed"
    Assert-Equal $MockState.Calls 8 'failed installation must not run dev'
    Assert-Equal $env:CAF_DATA_DIR $InheritedData 'environment restored after failed installation'
    Assert-Equal (Get-Location).Path $CallerLocation 'cwd restored after failed installation'

    $Launch = Get-Content (Join-Path $ProjectRoot '.vscode/launch.json') -Raw | ConvertFrom-Json
    $Tasks = Get-Content (Join-Path $ProjectRoot '.vscode/tasks.json') -Raw | ConvertFrom-Json
    $Config = $Launch.configurations[0]
    $BuildTask = @($Tasks.tasks | Where-Object { $_.label -eq $Config.preLaunchTask })
    Assert-Equal $BuildTask.Count 1 'debug pre-launch task resolves uniquely'
    Assert-Equal $Config.mode 'exec' 'debugger uses Wails-built executable'
    Assert-Equal $Config.program '${workspaceFolder}/build/bin/CAF-debug.exe' 'debug executable path'
    Assert-Equal $Config.env.CAF_DATA_DIR '${workspaceFolder}/build/dev-data' 'debugger data path'
    Assert-Equal $BuildTask[0].options.env.CAF_DATA_DIR $Config.env.CAF_DATA_DIR 'pre-launch bindings share debugger data path'
    Assert-Equal $BuildTask[0].command '${env:USERPROFILE}\go\bin\wails.exe' 'debug build uses installed CLI'
    Assert-Equal ($BuildTask[0].args -join '|') 'build|-debug|-platform|windows/amd64|-o|CAF-debug.exe' 'debug build command'
    $EnsureTask = @($Tasks.tasks | Where-Object { $_.label -eq $BuildTask[0].dependsOn })
    Assert-Equal $EnsureTask.Count 1 'debug build ensures the CLI exists'
    $Workspace = Get-Content (Join-Path $ProjectRoot 'CAF.code-workspace') -Raw | ConvertFrom-Json
    Assert-Equal ([bool]($Workspace.PSObject.Properties.Name -contains 'launch')) $false 'no conflicting workspace launch settings'
    Write-Host 'Development script checks passed: 8 mocked Wails runs, CLI installation and failure, plus VS Code configuration checks.'
} finally {
    $env:CAF_DATA_DIR = $SavedData
    $global:LASTEXITCODE = $SavedExitCode
    Set-Location $SavedLocation
}
