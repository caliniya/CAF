# Headless script/config regressions. Compatible with Windows PowerShell 5.1;
# mock go so these checks never open a GUI, download modules or start a server.
$ErrorActionPreference = 'Stop'
$ProjectRoot = Split-Path $PSScriptRoot -Parent
$DevScript = Join-Path $ProjectRoot 'dev.ps1'
$SavedData = $env:CAF_DATA_DIR
$SavedExitCode = $global:LASTEXITCODE
$SavedLocation = (Get-Location).Path
$MockState = [pscustomobject]@{
    ExitCode = 0
    Throws = $false
    Calls = 0
    ObservedData = $null
    ObservedLocation = $null
    ObservedArgs = $null
}

function Assert-Equal($Actual, $Expected, [string]$Message) {
    if ($Actual -cne $Expected) {
        throw "${Message}: expected '$Expected', got '$Actual'"
    }
}

function go {
    $MockState.Calls++
    $MockState.ObservedData = $env:CAF_DATA_DIR
    $MockState.ObservedLocation = (Get-Location).Path
    $MockState.ObservedArgs = $args -join '|'
    $global:LASTEXITCODE = $MockState.ExitCode
    if ($MockState.Throws) { throw 'mock command failure' }
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
    Assert-Equal $MockState.ObservedArgs 'run|github.com/wailsapp/wails/v2/cmd/wails@v2.17.0|dev' 'fixed Wails CLI command'
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

    $Launch = Get-Content (Join-Path $ProjectRoot '.vscode/launch.json') -Raw | ConvertFrom-Json
    $Tasks = Get-Content (Join-Path $ProjectRoot '.vscode/tasks.json') -Raw | ConvertFrom-Json
    $Config = $Launch.configurations[0]
    $BuildTask = @($Tasks.tasks | Where-Object { $_.label -eq $Config.preLaunchTask })
    Assert-Equal $BuildTask.Count 1 'debug pre-launch task resolves uniquely'
    Assert-Equal $Config.mode 'exec' 'debugger uses Wails-built executable'
    Assert-Equal $Config.program '${workspaceFolder}/build/bin/CAF-debug.exe' 'debug executable path'
    Assert-Equal $Config.env.CAF_DATA_DIR '${workspaceFolder}/build/dev-data' 'debugger data path'
    Assert-Equal ($BuildTask[0].args -join '|') 'run|github.com/wailsapp/wails/v2/cmd/wails@v2.17.0|build|-debug|-platform|windows/amd64|-o|CAF-debug.exe' 'debug build command'
    $Workspace = Get-Content (Join-Path $ProjectRoot 'CAF.code-workspace') -Raw | ConvertFrom-Json
    Assert-Equal ([bool]($Workspace.PSObject.Properties.Name -contains 'launch')) $false 'no conflicting workspace launch settings'
    Write-Host 'Development script checks passed: 7 mocked runs plus VS Code configuration checks.'
} finally {
    $env:CAF_DATA_DIR = $SavedData
    $global:LASTEXITCODE = $SavedExitCode
    Set-Location $SavedLocation
}
