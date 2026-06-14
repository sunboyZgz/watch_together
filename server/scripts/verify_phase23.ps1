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

function Test-MarkdownLocalLinks {
    $failures = @()
    Get-ChildItem -Path $RepoRoot -Recurse -Filter *.md -File |
        Where-Object { $_.FullName -notmatch '\\.git\\|\\node_modules\\|\\build\\|\\dist\\|\\.gradle\\' } |
        ForEach-Object {
            $file = $_.FullName
            $text = Get-Content -LiteralPath $file -Raw
            $matches = [regex]::Matches($text, '\[[^\]]+\]\(([^)]+)\)')
            foreach ($match in $matches) {
                $target = $match.Groups[1].Value.Trim()
                if ($target.StartsWith('#') -or $target -match '^[a-zA-Z][a-zA-Z0-9+.-]*:' -or $target.StartsWith('mailto:')) {
                    continue
                }
                $path = $target -replace '#.*$', ''
                if ([string]::IsNullOrWhiteSpace($path)) {
                    continue
                }
                $path = [uri]::UnescapeDataString($path)
                if ($path.StartsWith('/')) {
                    $candidate = Join-Path $RepoRoot ($path.TrimStart('/') -replace '/', [IO.Path]::DirectorySeparatorChar)
                } else {
                    $candidate = Join-Path (Split-Path -Parent $file) ($path -replace '/', [IO.Path]::DirectorySeparatorChar)
                }
                if (-not (Test-Path -LiteralPath $candidate)) {
                    $relative = Resolve-Path -LiteralPath $file -Relative
                    $failures += "$relative -> $target"
                }
            }
        }
    if ($failures.Count -gt 0) {
        $failures | ForEach-Object { Write-Output $_ }
        throw "Markdown local link check failed: $($failures.Count) broken links"
    }
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

Invoke-Step 'Go Phase 23 target tests' {
    Push-Location $ServerRoot
    try {
        & go test `
            ./internal/app `
            ./internal/store `
            ./internal/config `
            ./internal/transport `
            ./internal/roomapi `
            ./internal/auth `
            ./internal/progress `
            ./internal/home `
            ./internal/media `
            ./internal/authority `
            ./cmd/identityservice `
            ./cmd/roomservice `
            ./cmd/mediaservice `
            ./cmd/progressservice `
            ./cmd/homecompositionservice `
            ./cmd/apigateway `
            ./cmd/timelineservice `
            ./cmd/roomauthorityservice `
            ./cmd/identitydbsync `
            ./cmd/roomdbsync `
            ./cmd/mediadbsync `
            ./cmd/progressdbsync
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

if ($RunSmoke) {
    Invoke-Step 'Phase 23 full RPC multi database smoke' {
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

Invoke-Step 'Markdown local link check' {
    Test-MarkdownLocalLinks
}

Invoke-Step 'git diff --check' {
    Push-Location $RepoRoot
    try {
        & git diff --check
    } finally {
        Pop-Location
    }
}

Write-Host 'Phase 23 verification completed.'
