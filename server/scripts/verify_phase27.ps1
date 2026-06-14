param(
    [switch] $RunSmoke,
    [switch] $ResetVolumes,
    [switch] $DownAfterRun,
    [switch] $KeepRunning,
    [switch] $SkipBuild
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ServerRoot = (Resolve-Path (Join-Path $ScriptDir '..')).Path

function Invoke-Step {
    param(
        [string] $Name,
        [scriptblock] $Block
    )
    Write-Host "==> $Name"
    & $Block
}

Invoke-Step 'Phase 26 gateway/session baseline verification' {
    Push-Location $ServerRoot
    try {
        & .\scripts\verify_phase26.ps1
    } finally {
        Pop-Location
    }
}

Invoke-Step 'Phase 27 RPC-only database boundary tests' {
    Push-Location $ServerRoot
    try {
        & go test `
            ./internal/app `
            ./internal/config `
            ./internal/store `
            ./cmd/identityservice `
            ./cmd/roomservice `
            ./cmd/mediaservice `
            ./cmd/progressservice `
            ./cmd/timelineservice `
            ./cmd/outboxworker `
            ./cmd/homecompositionservice
    } finally {
        Pop-Location
    }
}

if ($RunSmoke) {
    Invoke-Step 'Phase 27 full RPC smoke with absent main owner tables' {
        Push-Location $ServerRoot
        try {
            $smokeArgs = @()
            if ($ResetVolumes) {
                $smokeArgs += '-ResetVolumes'
            }
            if ($DownAfterRun) {
                $smokeArgs += '-DownAfterRun'
            }
            if ($KeepRunning) {
                $smokeArgs += '-KeepRunning'
            }
            if ($SkipBuild) {
                $smokeArgs += '-SkipBuild'
            }
            & .\scripts\smoke_phase23_full_rpc.ps1 @smokeArgs
        } finally {
            Pop-Location
        }
    }
}

Write-Host 'Phase 27 verification completed.'
