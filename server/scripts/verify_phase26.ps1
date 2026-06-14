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

Invoke-Step 'Phase 23 full RPC baseline verification' {
    Push-Location $ServerRoot
    try {
        & .\scripts\verify_phase23.ps1
    } finally {
        Pop-Location
    }
}

Invoke-Step 'API gateway and session gateway edge tests' {
    Push-Location $ServerRoot
    try {
        & go test `
            ./internal/app `
            ./internal/transport `
            ./internal/store `
            ./internal/config `
            ./cmd/apigateway `
            ./cmd/roomserver
    } finally {
        Pop-Location
    }
}

if ($RunSmoke) {
    Invoke-Step 'Phase 26 full RPC gateway/session smoke' {
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

Write-Host 'Phase 26 verification completed.'
