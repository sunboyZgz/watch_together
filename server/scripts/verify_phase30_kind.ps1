param(
    [switch] $RunSmoke,
    [switch] $ResetCluster,
    [switch] $DeleteAfterRun,
    [switch] $KeepCluster,
    [switch] $SkipBuild,
    [switch] $SkipImageLoad
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ServerRoot = (Resolve-Path (Join-Path $ScriptDir '..')).Path
$OverlayPath = Join-Path $ServerRoot 'k8s/overlays/kind'

function Invoke-Step {
    param(
        [string] $Name,
        [scriptblock] $Block
    )
    Write-Host "==> $Name"
    & $Block
}

function Assert-Contains {
    param(
        [string] $Text,
        [string] $Needle,
        [string] $Message
    )
    if (-not $Text.Contains($Needle)) {
        throw $Message
    }
}

function Assert-NotContains {
    param(
        [string] $Text,
        [string] $Needle,
        [string] $Message
    )
    if ($Text.Contains($Needle)) {
        throw $Message
    }
}

function Get-DeploymentBlock {
    param(
        [string] $Manifest,
        [string] $Name
    )
    $pattern = "(?ms)^apiVersion: apps/v1\s+kind: Deployment\s+metadata:\s+(?:(?!^---).)*?\n  name: $([regex]::Escape($Name))\s+(?:(?!^---).)*?(?=^---|\z)"
    $match = [regex]::Match($Manifest, $pattern)
    if (-not $match.Success) {
        throw "rendered kind manifest does not include deployment/$Name"
    }
    return $match.Value
}

function Assert-NoDatabaseEnv {
    param(
        [string] $Manifest,
        [string] $Deployment
    )
    $block = Get-DeploymentBlock -Manifest $Manifest -Name $Deployment
    foreach ($key in @(
        'DATABASE_URL',
        'IDENTITY_DATABASE_URL',
        'ROOM_DATABASE_URL',
        'MEDIA_DATABASE_URL',
        'PROGRESS_DATABASE_URL',
        'TIMELINE_DATABASE_URL'
    )) {
        if ($block.Contains("name: $key")) {
            throw "deployment/$Deployment must not receive direct $key"
        }
    }
}

function Assert-DeploymentContains {
    param(
        [string] $Manifest,
        [string] $Deployment,
        [string] $Needle
    )
    $block = Get-DeploymentBlock -Manifest $Manifest -Name $Deployment
    Assert-Contains -Text $block -Needle $Needle -Message "deployment/$Deployment does not contain required fragment: $Needle"
}

Invoke-Step 'Phase 30 readiness preflight' {
    Push-Location $ServerRoot
    try {
        & .\scripts\verify_phase30_ready.ps1
    } finally {
        Pop-Location
    }
}

$script:rendered = ''
Invoke-Step 'render kind Kustomize overlay' {
    Push-Location $ServerRoot
    try {
        $script:rendered = (& kubectl kustomize .\k8s\overlays\kind) -join "`n"
        if ($LASTEXITCODE -ne 0) {
            throw 'kubectl kustomize server/k8s/overlays/kind failed'
        }
    } finally {
        Pop-Location
    }
}

Invoke-Step 'Phase 30 kind manifest guards' {
    Assert-Contains -Text $rendered -Needle 'name: roomserver' -Message 'kind manifest must include roomserver deployment'
    Assert-Contains -Text $rendered -Needle 'replicas: 2' -Message 'kind overlay must run roomserver with two replicas'
    Assert-Contains -Text $rendered -Needle 'type: NodePort' -Message 'nginx service must be NodePort in kind overlay'
    Assert-Contains -Text $rendered -Needle 'nodePort: 30080' -Message 'nginx NodePort must be 30080'
    Assert-Contains -Text $rendered -Needle 'server apigateway:8080;' -Message 'nginx must route public REST to apigateway'
    Assert-Contains -Text $rendered -Needle 'server roomserver:8080;' -Message 'nginx must route /ws to roomserver Service'
    Assert-NotContains -Text $rendered -Needle 'bitnami/kafka' -Message 'kind manifests must not use bitnami/kafka'
    Assert-NotContains -Text $rendered -Needle 'KAFKA_CFG_' -Message 'kind manifests must not use Bitnami KAFKA_CFG_* env'
    Assert-NotContains -Text $rendered -Needle '/bitnami/kafka' -Message 'kind manifests must not mount Bitnami Kafka paths'

    foreach ($edgeDeployment in @('apigateway', 'roomserver', 'roomauthorityservice')) {
        Assert-NoDatabaseEnv -Manifest $rendered -Deployment $edgeDeployment
    }
    Assert-DeploymentContains -Manifest $rendered -Deployment 'identityservice' -Needle 'name: IDENTITY_DATABASE_URL'
    Assert-DeploymentContains -Manifest $rendered -Deployment 'roomservice' -Needle 'name: ROOM_DATABASE_URL'
    Assert-DeploymentContains -Manifest $rendered -Deployment 'mediaservice' -Needle 'name: MEDIA_DATABASE_URL'
    Assert-DeploymentContains -Manifest $rendered -Deployment 'progressservice' -Needle 'name: PROGRESS_DATABASE_URL'
    Assert-DeploymentContains -Manifest $rendered -Deployment 'timelineservice' -Needle 'name: TIMELINE_DATABASE_URL'
}

Invoke-Step 'Go regression including kind architecture guards' {
    Push-Location $ServerRoot
    try {
        & go test ./...
        if ($LASTEXITCODE -ne 0) {
            throw 'go test ./... failed'
        }
    } finally {
        Pop-Location
    }
}

Invoke-Step 'Compose baseline still renders' {
    Push-Location $ServerRoot
    try {
        & docker compose -f .\compose.yaml --profile app config *> $null
        if ($LASTEXITCODE -ne 0) {
            throw 'docker compose app config failed'
        }
        & docker compose -f .\compose.yaml --profile rpc-pilot config *> $null
        if ($LASTEXITCODE -ne 0) {
            throw 'docker compose rpc-pilot config failed'
        }
        & docker compose -f .\compose.prod.yaml config *> $null
        if ($LASTEXITCODE -ne 0) {
            throw 'docker compose prod config failed'
        }
    } finally {
        Pop-Location
    }
}

if ($RunSmoke) {
    Invoke-Step 'Phase 30 kind rolling restart smoke' {
        Push-Location $ServerRoot
        try {
            $smokeArgs = @()
            if ($ResetCluster) {
                $smokeArgs += '-ResetCluster'
            }
            if ($DeleteAfterRun) {
                $smokeArgs += '-DeleteAfterRun'
            }
            if ($KeepCluster) {
                $smokeArgs += '-KeepCluster'
            }
            if ($SkipBuild) {
                $smokeArgs += '-SkipBuild'
            }
            if ($SkipImageLoad) {
                $smokeArgs += '-SkipImageLoad'
            }
            & .\scripts\smoke_phase30_kind.ps1 @smokeArgs
        } finally {
            Pop-Location
        }
    }
}

Write-Host 'Phase 30 kind verification completed.'
