param()

$ErrorActionPreference = 'Stop'

function Invoke-Step {
    param(
        [string] $Name,
        [scriptblock] $Block
    )
    Write-Host "==> $Name"
    & $Block
}

function Test-CommandAvailable {
    param([string] $Name)
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Test-DockerImagePresent {
    param([string] $Image)
    & docker image inspect $Image *> $null
    return $LASTEXITCODE -eq 0
}

Invoke-Step 'Docker daemon and compose plugin' {
    if (-not (Test-CommandAvailable 'docker')) {
        throw 'docker command is not available. Install Docker Desktop before Phase 30 kind work.'
    }
    & docker info *> $null
    if ($LASTEXITCODE -ne 0) {
        throw 'Docker daemon is not available. Start Docker Desktop before Phase 30 kind work.'
    }
    & docker compose version *> $null
    if ($LASTEXITCODE -ne 0) {
        throw 'docker compose plugin is not available. Install a Docker Compose v2 plugin before Phase 30 kind work.'
    }
}

Invoke-Step 'Required local images for Compose and kind preflight' {
    $requiredImages = @(
        'apache/kafka:3.7.0',
        'postgres:16',
        'postgres:16-alpine',
        'redis:7.2-alpine',
        'nats:2.10-alpine',
        'nginx:1.27-alpine',
        'migrate/migrate:v4.17.1',
        'quay.io/minio/minio:latest',
        'quay.io/minio/mc:latest'
    )

    $missing = @()
    foreach ($image in $requiredImages) {
        if (-not (Test-DockerImagePresent -Image $image)) {
            $missing += $image
        }
    }

    if ($missing.Count -gt 0) {
        Write-Host 'Missing required local images. Pull them manually, then rerun this script:'
        foreach ($image in $missing) {
            Write-Host "docker pull $image"
        }
        throw 'Phase 30 readiness preflight failed because required images are missing.'
    }
}

Invoke-Step 'Optional kind and kubectl tools' {
    if (Test-CommandAvailable 'kind') {
        & kind version
    } else {
        Write-Host 'kind is not installed yet. Phase 30 will need kind before creating a local Kubernetes cluster.'
    }

    if (Test-CommandAvailable 'kubectl') {
        & kubectl version --client
    } else {
        Write-Host 'kubectl is not installed yet. Phase 30 will need kubectl for local Kubernetes deployment checks.'
    }
}

Write-Host 'Phase 30 readiness preflight completed.'
