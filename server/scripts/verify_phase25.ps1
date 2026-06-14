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

function Invoke-WithRollbackAuthorityEnv {
    param([scriptblock] $Block)
    $names = @(
        'AUTHORITY_SERVICE_MODE',
        'ROOM_RUNTIME_MODE',
        'WS_CROSS_INSTANCE_BROADCAST_ENABLED'
    )
    $previous = @{}
    foreach ($name in $names) {
        $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
    }
    try {
        $env:AUTHORITY_SERVICE_MODE = 'local'
        $env:ROOM_RUNTIME_MODE = 'local_process'
        $env:WS_CROSS_INSTANCE_BROADCAST_ENABLED = 'false'
        & $Block
    } finally {
        foreach ($name in $names) {
            if ($null -eq $previous[$name]) {
                Remove-Item "Env:\$name" -ErrorAction SilentlyContinue
            } else {
                [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process')
            }
        }
    }
}

Invoke-Step 'Phase 23 full RPC baseline verification' {
    Push-Location $ServerRoot
    try {
        & .\scripts\verify_phase23.ps1
    } finally {
        Pop-Location
    }
}

Invoke-Step 'authority engine and ingress semantic tests' {
    Push-Location $ServerRoot
    try {
        & go test `
            ./internal/authority `
            ./internal/transport `
            ./cmd/roomauthorityservice `
            ./internal/config `
            ./internal/store
    } finally {
        Pop-Location
    }
}

Invoke-Step 'prod authority local rollback compose config' {
    Push-Location $ServerRoot
    try {
        Invoke-WithRollbackAuthorityEnv {
            & docker compose -f compose.prod.yaml config | Out-Null
        }
    } finally {
        Pop-Location
    }
}

if ($RunSmoke) {
    Invoke-Step 'Phase 23 full RPC smoke with Phase 25 authority engine' {
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

Write-Host 'Phase 25 verification completed.'
