param(
    [string] $ClusterName = 'watch-together-dev'
)

$ErrorActionPreference = 'Stop'

if ($null -eq (Get-Command kind -ErrorAction SilentlyContinue)) {
    throw 'kind is not available.'
}

& kind delete cluster --name $ClusterName
if ($LASTEXITCODE -ne 0) {
    throw "kind delete cluster failed for $ClusterName"
}
