$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ServerRoot = (Resolve-Path (Join-Path $ScriptDir '..')).Path
$RepoRoot = (Resolve-Path (Join-Path $ServerRoot '..')).Path

$GoBin = Join-Path $env:USERPROFILE 'go\bin'
if (Test-Path $GoBin) {
    $env:PATH = "$GoBin;$env:PATH"
}

function Invoke-Step {
    param(
        [string] $Name,
        [scriptblock] $Block
    )
    Write-Host "==> $Name"
    & $Block
}

Invoke-Step 'buf lint' {
    Push-Location $RepoRoot
    try {
        & buf lint
    } finally {
        Pop-Location
    }
}

Invoke-Step 'buf generate is repeatable' {
    Push-Location $RepoRoot
    try {
        $before = & git diff -- server/api/internal/v1 server/internal/rpcgen
        & buf generate
        $after = & git diff -- server/api/internal/v1 server/internal/rpcgen
        if (($before -join "`n") -ne ($after -join "`n")) {
            throw 'buf generate changed proto/generated output. Re-run buf generate and review the diff.'
        }
    } finally {
        Pop-Location
    }
}

Invoke-Step 'Go Phase 11 target tests' {
    Push-Location $ServerRoot
    try {
        & go test `
            ./internal/... `
            ./cmd/roomserver `
            ./cmd/outboxworker `
            ./cmd/derivedworker `
            ./cmd/mediaservice `
            ./cmd/timelineservice `
            ./cmd/mediadbsync
    } finally {
        Pop-Location
    }
}

Invoke-Step 'Go full test suite' {
    Push-Location $ServerRoot
    try {
        & go test ./...
    } finally {
        Pop-Location
    }
}

Invoke-Step 'compose app config' {
    Push-Location $ServerRoot
    try {
        & docker compose --profile app config | Out-Null
    } finally {
        Pop-Location
    }
}

Invoke-Step 'compose rpc-pilot config' {
    Push-Location $ServerRoot
    try {
        & docker compose --profile rpc-pilot config | Out-Null
    } finally {
        Pop-Location
    }
}

Invoke-Step 'prod compose config' {
    Push-Location $ServerRoot
    try {
        & docker compose -f compose.prod.yaml config | Out-Null
    } finally {
        Pop-Location
    }
}

Invoke-Step 'git diff --check' {
    Push-Location $RepoRoot
    try {
        & git diff --check
    } finally {
        Pop-Location
    }
}

Write-Host 'Phase 11 verification completed.'
