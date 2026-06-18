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

function Import-ImageIntoKindNode {
    param(
        [string] $ClusterName,
        [string] $Image
    )

    $node = @(& kind get nodes --name $ClusterName | Select-Object -First 1)
    if ($node.Count -eq 0 -or [string]::IsNullOrWhiteSpace($node[0])) {
        throw "no kind node found for cluster '$ClusterName'"
    }

    $nodeName = $node[0]
    $id = [System.Guid]::NewGuid().ToString('N')
    $localTar = Join-Path ([System.IO.Path]::GetTempPath()) "kind-image-$id.tar"
    $nodeTar = "/root/kind-image-$id.tar"

    try {
        & docker save $Image -o $localTar
        if ($LASTEXITCODE -ne 0) {
            throw "docker save failed for $Image"
        }

        & docker cp $localTar "${nodeName}:$nodeTar"
        if ($LASTEXITCODE -ne 0) {
            throw "docker cp failed for $Image"
        }

        & docker exec --privileged $nodeName ctr --namespace=k8s.io images import --platform linux/amd64 --digests --snapshotter=overlayfs $nodeTar
        if ($LASTEXITCODE -ne 0) {
            throw "ctr image import failed for $Image"
        }
    } finally {
        if (Test-Path $localTar) {
            Remove-Item -LiteralPath $localTar -Force
        }
        & docker exec $nodeName rm -f $nodeTar *> $null
    }
}

function Load-ImageIntoKind {
    param(
        [string] $ClusterName,
        [string] $Image,
        [switch] $DirectImport
    )

    if ($DirectImport) {
        Write-Host "==> kind import $Image"
        Import-ImageIntoKindNode -ClusterName $ClusterName -Image $Image
        return
    }

    Write-Host "==> kind load $Image"
    & kind load docker-image --name $ClusterName $Image
    if ($LASTEXITCODE -eq 0) {
        return
    }

    Write-Host "kind load failed for $Image; retrying with linux/amd64 containerd import."
    Import-ImageIntoKindNode -ClusterName $ClusterName -Image $Image
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

$publicImages = @(
    'postgres:16-alpine',
    'redis:7.2-alpine',
    'nats:2.10-alpine',
    'apache/kafka:3.7.0',
    'quay.io/minio/minio:latest',
    'quay.io/minio/mc:latest',
    'nginx:1.27-alpine',
    'migrate/migrate:v4.17.1'
)
$appImages = @()
if (-not $SkipAppImages) {
    $appImages = @('watch-together-roomserver:dev', 'watch-together-nginx:dev')
}
$images = $appImages + $publicImages

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
    Load-ImageIntoKind -ClusterName $ClusterName -Image $image -DirectImport:($publicImages -contains $image)
}

Write-Host "Loaded Phase 30 images into kind cluster '$ClusterName'."
