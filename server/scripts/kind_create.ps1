param(
    [string] $ClusterName = 'watch-together-dev'
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ServerRoot = (Resolve-Path (Join-Path $ScriptDir '..')).Path
$KindConfig = Join-Path $ServerRoot 'k8s/overlays/kind/kind-cluster.yaml'

if ($null -eq (Get-Command kind -ErrorAction SilentlyContinue)) {
    throw 'kind is not available. Install kind before creating the Phase 30 local cluster.'
}

$existing = @((& kind get clusters) | Where-Object { $_ -eq $ClusterName })
if ($existing.Count -gt 0) {
    Write-Host "kind cluster '$ClusterName' already exists."
    exit 0
}

& kind create cluster --name $ClusterName --config $KindConfig
if ($LASTEXITCODE -ne 0) {
    throw "kind create cluster failed for $ClusterName"
}

Write-Host "kind cluster '$ClusterName' is ready. Public entry will be http://127.0.0.1:30080 after deploy."
