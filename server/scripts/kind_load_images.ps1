param(
    [string] $ClusterName = 'watch-together-dev',
    [switch] $SkipAppImages
)

$ErrorActionPreference = 'Stop'

function Test-ImagePresent {
    param([string] $Image)
    & docker image inspect $Image *> $null
    return $LASTEXITCODE -eq 0
}

if ($null -eq (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'docker is not available.'
}
if ($null -eq (Get-Command kind -ErrorAction SilentlyContinue)) {
    throw 'kind is not available.'
}

$clusters = @(& kind get clusters)
if ((@($clusters | Where-Object { $_ -eq $ClusterName })).Count -eq 0) {
    throw "kind cluster '$ClusterName' does not exist. Run scripts/kind_create.ps1 first."
}

$images = @(
    'postgres:16-alpine',
    'redis:7.2-alpine',
    'nats:2.10-alpine',
    'apache/kafka:3.7.0',
    'quay.io/minio/minio:latest',
    'quay.io/minio/mc:latest',
    'nginx:1.27-alpine',
    'migrate/migrate:v4.17.1'
)
if (-not $SkipAppImages) {
    $images = @('watch-together-roomserver:dev', 'watch-together-nginx:dev') + $images
}

$missing = @()
foreach ($image in $images) {
    if (-not (Test-ImagePresent -Image $image)) {
        $missing += $image
    }
}
if ($missing.Count -gt 0) {
    Write-Host 'Missing local images. Pull or build them manually, then rerun this script:'
    foreach ($image in $missing) {
        if ($image.StartsWith('watch-together-')) {
            Write-Host "# build $image from this repo"
        } else {
            Write-Host "docker pull $image"
        }
    }
    throw 'kind image load failed because required images are missing locally.'
}

foreach ($image in $images) {
    Write-Host "==> kind load $image"
    & kind load docker-image --name $ClusterName $image
    if ($LASTEXITCODE -ne 0) {
        throw "kind load docker-image failed for $image"
    }
}

Write-Host "Loaded Phase 30 images into kind cluster '$ClusterName'."
