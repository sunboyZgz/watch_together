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

Invoke-Step 'Phase 27 RPC-only owner database baseline verification' {
    Push-Location $ServerRoot
    try {
        & .\scripts\verify_phase27.ps1
    } finally {
        Pop-Location
    }
}

Invoke-Step 'Phase 29 rolling smoke compose config' {
    Push-Location $ServerRoot
    try {
        & docker compose --profile app --profile rolling-smoke config | Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw 'docker compose --profile app --profile rolling-smoke config failed'
        }
    } finally {
        Pop-Location
    }
}

Invoke-Step 'Phase 29 rolling drain guards' {
    Push-Location $ServerRoot
    try {
        & go test `
            ./internal/app `
            ./internal/transport `
            ./internal/observability `
            ./internal/store
    } finally {
        Pop-Location
    }
}

if ($RunSmoke) {
    Invoke-Step 'Phase 29 dual-roomserver rolling drain smoke' {
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
            & .\scripts\smoke_phase29_rolling_drain.ps1 @smokeArgs
        } finally {
            Pop-Location
        }
    }
}

Write-Host 'Phase 29 verification completed.'
