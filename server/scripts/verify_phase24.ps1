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

function Invoke-WithAuthorityCanaryEnv {
    param([scriptblock] $Block)
    $names = @(
        'AUTHORITY_SERVICE_MODE',
        'AUTHORITY_SERVICE_ADDR',
        'AUTHORITY_LEASE_INSTANCE_ID',
        'ROOM_RUNTIME_MODE',
        'WS_CROSS_INSTANCE_BROADCAST_ENABLED'
    )
    $previous = @{}
    foreach ($name in $names) {
        $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
    }
    try {
        $env:AUTHORITY_SERVICE_MODE = 'rpc'
        $env:AUTHORITY_SERVICE_ADDR = 'http://roomauthorityservice:8090'
        $env:AUTHORITY_LEASE_INSTANCE_ID = 'roomauthorityservice-prod-1'
        $env:ROOM_RUNTIME_MODE = 'distributed_authority'
        $env:WS_CROSS_INSTANCE_BROADCAST_ENABLED = 'true'
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

Invoke-Step 'Phase 23 baseline verification' {
    Push-Location $ServerRoot
    try {
        & .\scripts\verify_phase23.ps1
    } finally {
        Pop-Location
    }
}

Invoke-Step 'authority RPC canary semantic tests' {
    Push-Location $ServerRoot
    try {
        & go test ./internal/transport ./cmd/roomauthorityservice
    } finally {
        Pop-Location
    }
}

Invoke-Step 'prod authority RPC canary compose config' {
    Push-Location $ServerRoot
    try {
        Invoke-WithAuthorityCanaryEnv {
            & docker compose -f compose.prod.yaml --profile authority-rpc-canary config | Out-Null
        }
    } finally {
        Pop-Location
    }
}

if ($RunSmoke) {
    Invoke-Step 'Phase 24 authority canary smoke' {
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
            & .\scripts\smoke_phase24_authority_canary.ps1 @smokeArgs
        } finally {
            Pop-Location
        }
    }
}

Write-Host 'Phase 24 verification completed.'
