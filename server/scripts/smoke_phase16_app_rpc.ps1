param(
    [switch] $ResetVolumes,
    [switch] $DownAfterRun,
    [switch] $KeepRunning,
    [switch] $SkipBuild
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ServerRoot = (Resolve-Path (Join-Path $ScriptDir '..')).Path
$RepoRoot = (Resolve-Path (Join-Path $ServerRoot '..')).Path

$BaseUrl = 'http://127.0.0.1:8080'
$IdentityServiceReadyUrl = 'http://127.0.0.1:8093/readyz'
$MediaServiceReadyUrl = 'http://127.0.0.1:8090/readyz'
$TimelineServiceReadyUrl = 'http://127.0.0.1:8091/readyz'
$AuthorityServiceReadyUrl = 'http://127.0.0.1:8092/readyz'
$AuthorityMetricsUrl = 'http://127.0.0.1:8092/metrics'
$MainDatabaseUrl = 'postgres://app:app@postgres:5432/anime_watch_dev?sslmode=disable'
$MediaDatabaseUrl = 'postgres://app:app@postgres:5432/anime_watch_media_dev?sslmode=disable'
$TimelineDatabaseUrl = 'postgres://app:app@postgres:5432/anime_watch_timeline_dev?sslmode=disable'
$SmokeEpisodeID = '00000000-0000-0000-0000-000000160102'

function Test-DockerAvailable {
    $previousErrorActionPreference = $ErrorActionPreference
    $exitCode = 1
    try {
        $ErrorActionPreference = 'Continue'
        & docker version *>$null
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    if ($exitCode -ne 0) {
        throw 'Docker daemon is not available. Start Docker Desktop, then rerun scripts/verify_phase16.ps1 -RunSmoke.'
    }
}

function Invoke-Step {
    param(
        [string] $Name,
        [scriptblock] $Block
    )
    Write-Host "==> $Name"
    & $Block
}

function Invoke-Compose {
    param([string[]] $Arguments)
    Push-Location $ServerRoot
    try {
        & docker compose @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "docker compose $($Arguments -join ' ') failed"
        }
    } finally {
        Pop-Location
    }
}

function Wait-PostgresDatabase {
    param(
        [string] $Database,
        [int] $TimeoutSeconds = 120
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        Push-Location $ServerRoot
        try {
            & docker compose exec -T postgres pg_isready -U app -d $Database | Out-Null
            if ($LASTEXITCODE -eq 0) {
                return
            }
        } finally {
            Pop-Location
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "database $Database did not become ready"
}

function Get-ComposeNetwork {
    Push-Location $ServerRoot
    try {
        $containerID = (& docker compose ps -q postgres).Trim()
        if ([string]::IsNullOrWhiteSpace($containerID)) {
            throw 'postgres container is not running'
        }
        $networks = & docker inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' $containerID
        $network = ($networks | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -First 1)
        if ([string]::IsNullOrWhiteSpace($network)) {
            throw 'compose network not found'
        }
        return $network.Trim()
    } finally {
        Pop-Location
    }
}

function Invoke-Migrations {
    param(
        [string] $DatabaseUrl,
        [string] $MigrationsDir
    )
    $network = Get-ComposeNetwork
    $mount = "${MigrationsDir}:/migrations:ro"
    & docker run --rm --network $network -v $mount migrate/migrate:v4.17.1 -path=/migrations -database $DatabaseUrl up
    if ($LASTEXITCODE -ne 0) {
        throw "migrations failed for $DatabaseUrl"
    }
}

function Invoke-PostgresSQL {
    param(
        [string] $Database,
        [string] $Sql,
        [switch] $TupleOnly
    )
    Push-Location $ServerRoot
    try {
        $dockerArgs = @('compose', 'exec', '-T', 'postgres', 'psql', '-v', 'ON_ERROR_STOP=1')
        if ($TupleOnly) {
            $dockerArgs += @('-A', '-t')
        }
        $dockerArgs += @('-U', 'app', '-d', $Database)
        $output = $Sql | & docker @dockerArgs
        if ($LASTEXITCODE -ne 0) {
            throw "psql failed for database $Database"
        }
        return ($output -join "`n").Trim()
    } finally {
        Pop-Location
    }
}

function Wait-HttpReady {
    param(
        [string] $Url,
        [string] $Name,
        [int] $TimeoutSeconds = 180
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        try {
            $response = Invoke-RestMethod -Method Get -Uri $Url -TimeoutSec 5
            if ($response.status -eq 'ready') {
                return
            }
        } catch {
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "$Name did not become ready at $Url"
}

function Invoke-JsonRequest {
    param(
        [string] $Method,
        [string] $Uri,
        [object] $Body = $null,
        [string] $Token = ''
    )
    $headers = @{}
    if (-not [string]::IsNullOrWhiteSpace($Token)) {
        $headers['Authorization'] = "Bearer $Token"
    }
    if ($null -eq $Body) {
        return Invoke-RestMethod -Method $Method -Uri $Uri -Headers $headers -TimeoutSec 15
    }
    $json = $Body | ConvertTo-Json -Depth 20 -Compress
    return Invoke-RestMethod -Method $Method -Uri $Uri -Headers $headers -ContentType 'application/json' -Body $json -TimeoutSec 15
}

function New-SmokeUser {
    param(
        [string] $Account,
        [string] $Nickname
    )
    $response = Invoke-JsonRequest -Method Post -Uri "$BaseUrl/auth/register" -Body @{
        account = $Account
        password = 'Phase16Smoke123!'
        nickname = $Nickname
    }
    return @{
        userId = $response.data.user.id
        token = $response.data.accessToken
    }
}

function New-SmokeWebSocket {
    param([string] $Token)
    $socket = [System.Net.WebSockets.ClientWebSocket]::new()
    $socket.Options.SetRequestHeader('Authorization', "Bearer $Token")
    $cts = [System.Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(15))
    $socket.ConnectAsync([Uri]'ws://127.0.0.1:8080/ws', $cts.Token).GetAwaiter().GetResult()
    return $socket
}

function Send-WsJson {
    param(
        [System.Net.WebSockets.ClientWebSocket] $Socket,
        [object] $Message
    )
    $json = $Message | ConvertTo-Json -Depth 20 -Compress
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
    $segment = [System.ArraySegment[byte]]::new($bytes, 0, $bytes.Length)
    $Socket.SendAsync($segment, [System.Net.WebSockets.WebSocketMessageType]::Text, $true, [System.Threading.CancellationToken]::None).GetAwaiter().GetResult()
}

function Receive-WsJson {
    param(
        [System.Net.WebSockets.ClientWebSocket] $Socket,
        [int] $TimeoutSeconds = 15
    )
    $buffer = New-Object byte[] 8192
    $stream = [System.IO.MemoryStream]::new()
    $cts = [System.Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds($TimeoutSeconds))
    do {
        $segment = [System.ArraySegment[byte]]::new($buffer, 0, $buffer.Length)
        $result = $Socket.ReceiveAsync($segment, $cts.Token).GetAwaiter().GetResult()
        if ($result.MessageType -eq [System.Net.WebSockets.WebSocketMessageType]::Close) {
            throw 'websocket closed before expected message'
        }
        $stream.Write($buffer, 0, $result.Count)
    } while (-not $result.EndOfMessage)
    $text = [System.Text.Encoding]::UTF8.GetString($stream.ToArray())
    return $text | ConvertFrom-Json
}

function Read-WsUntil {
    param(
        [System.Net.WebSockets.ClientWebSocket] $Socket,
        [string] $Type,
        [string] $RequestID = '',
        [int] $TimeoutSeconds = 20
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        $remaining = [Math]::Max(1, [int][Math]::Ceiling(($deadline - (Get-Date)).TotalSeconds))
        $envelope = Receive-WsJson -Socket $Socket -TimeoutSeconds $remaining
        if ($envelope.type -eq 'heartbeat') {
            Send-WsJson -Socket $Socket -Message @{
                type = 'heartbeat_ack'
                payload = @{
                    serverTimeMs = $envelope.payload.serverTimeMs
                    clientTimeMs = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
                }
            }
            continue
        }
        if ($envelope.type -eq 'error') {
            throw "websocket error: $($envelope.payload.message)"
        }
        if ($envelope.type -eq $Type) {
            if ([string]::IsNullOrWhiteSpace($RequestID) -or $envelope.payload.requestId -eq $RequestID) {
                return $envelope
            }
        }
    } while ((Get-Date) -lt $deadline)
    throw "timed out waiting for websocket message type=$Type requestId=$RequestID"
}

function Close-SmokeWebSocket {
    param([System.Net.WebSockets.ClientWebSocket] $Socket)
    if ($null -eq $Socket) {
        return
    }
    if ($Socket.State -eq [System.Net.WebSockets.WebSocketState]::Open) {
        $Socket.CloseAsync([System.Net.WebSockets.WebSocketCloseStatus]::NormalClosure, 'smoke done', [System.Threading.CancellationToken]::None).GetAwaiter().GetResult()
    }
    $Socket.Dispose()
}

function Seed-SmokeMedia {
    $sql = @"
INSERT INTO media_tags (id, slug, name, sort_order, is_featured, is_active, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000160100', 'phase16-smoke', 'Phase 16 Smoke', 0, true, true, NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET
    slug = EXCLUDED.slug,
    name = EXCLUDED.name,
    is_featured = EXCLUDED.is_featured,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

INSERT INTO media_seasons (
    id, slug, title, original_title, description, cover_url, category, production_team,
    search_aliases, season_number, season_label, sort_order, status, created_at, updated_at
)
VALUES (
    '00000000-0000-0000-0000-000000160101',
    'phase16-smoke-season',
    'Phase16 Smoke Season',
    NULL,
    'Phase16 smoke media for local app RPC verification',
    NULL,
    'smoke',
    'watch_together',
    '["phase16","smoke"]'::jsonb,
    1,
    'Smoke Season',
    0,
    'active',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    updated_at = NOW();

INSERT INTO media_episodes (
    id, season_id, title, subtitle, description, cover_url, media_url, duration_ms,
    episode_number, episode_label, source_key, source_hash, sort_order, status, created_at, updated_at
)
VALUES (
    '$SmokeEpisodeID',
    '00000000-0000-0000-0000-000000160101',
    'Phase16 Smoke Episode',
    'Full RPC app path',
    'Deterministic playable episode for Phase16 smoke verification',
    NULL,
    'phase16-smoke/master.m3u8',
    120000,
    1,
    'Episode 1',
    'phase16-smoke/source.mp4',
    'phase16-smoke-hash',
    0,
    'active',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    subtitle = EXCLUDED.subtitle,
    description = EXCLUDED.description,
    media_url = EXCLUDED.media_url,
    duration_ms = EXCLUDED.duration_ms,
    status = EXCLUDED.status,
    updated_at = NOW();

INSERT INTO media_season_tags (season_id, media_tag_id, created_at)
VALUES ('00000000-0000-0000-0000-000000160101', '00000000-0000-0000-0000-000000160100', NOW())
ON CONFLICT DO NOTHING;
"@
    Invoke-PostgresSQL -Database 'anime_watch_media_dev' -Sql $sql | Out-Null
}

$hostSocket = $null
$viewerSocket = $null
$shouldDown = $DownAfterRun -and -not $KeepRunning
$composeStarted = $false

try {
    Invoke-Step 'check Docker daemon' {
        Test-DockerAvailable
    }

    Invoke-Step 'start postgres and database init jobs' {
        if ($ResetVolumes) {
            Invoke-Compose -Arguments @('--profile', 'app', 'down', '-v', '--remove-orphans')
        }
        Invoke-Compose -Arguments @('--profile', 'app', 'up', '-d', 'postgres', 'media-postgres-init', 'timeline-postgres-init')
        $composeStarted = $true
        Wait-PostgresDatabase -Database 'anime_watch_dev'
        Wait-PostgresDatabase -Database 'anime_watch_media_dev'
        Wait-PostgresDatabase -Database 'anime_watch_timeline_dev'
    }

    Invoke-Step 'apply main, media, and timeline migrations' {
        Invoke-Migrations -DatabaseUrl $MainDatabaseUrl -MigrationsDir (Join-Path $ServerRoot 'migrations')
        Invoke-Migrations -DatabaseUrl $MediaDatabaseUrl -MigrationsDir (Join-Path $ServerRoot 'media_migrations')
        Invoke-Migrations -DatabaseUrl $TimelineDatabaseUrl -MigrationsDir (Join-Path $ServerRoot 'timeline_migrations')
    }

    Invoke-Step 'seed deterministic media episode' {
        Seed-SmokeMedia
    }

    Invoke-Step 'start local app full RPC profile' {
        $composeArgs = @('--profile', 'app', 'up', '-d')
        if (-not $SkipBuild) {
            $composeArgs += '--build'
        }
        Invoke-Compose -Arguments $composeArgs
    }

    Invoke-Step 'wait for roomserver and internal services readiness' {
        Wait-HttpReady -Url "$BaseUrl/readyz" -Name 'roomserver'
        Wait-HttpReady -Url $IdentityServiceReadyUrl -Name 'identityservice'
        Wait-HttpReady -Url $MediaServiceReadyUrl -Name 'mediaservice'
        Wait-HttpReady -Url $TimelineServiceReadyUrl -Name 'timelineservice'
        Wait-HttpReady -Url $AuthorityServiceReadyUrl -Name 'roomauthorityservice'
    }

    $suffix = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    $hostUser = New-SmokeUser -Account "phase16_host_$suffix" -Nickname 'Phase16 Host'
    $viewerUser = New-SmokeUser -Account "phase16_viewer_$suffix" -Nickname 'Phase16 Viewer'

    Invoke-Step 'verify media discovery through roomserver RPC adapter' {
        $media = Invoke-JsonRequest -Method Get -Uri "$BaseUrl/media/items?query=Phase16&limit=5" -Token $hostUser.token
        $ids = @($media.data.items | ForEach-Object { $_.id })
        if ($ids -notcontains $SmokeEpisodeID) {
            throw "smoke media episode $SmokeEpisodeID not found in media/items response"
        }
    }

    $room = Invoke-JsonRequest -Method Post -Uri "$BaseUrl/rooms" -Token $hostUser.token -Body @{ mediaItemId = $SmokeEpisodeID }
    $roomCode = $room.data.room.roomCode
    if ([string]::IsNullOrWhiteSpace($roomCode)) {
        throw 'room create did not return a room code'
    }
    Invoke-JsonRequest -Method Post -Uri "$BaseUrl/rooms/$roomCode/join" -Token $viewerUser.token | Out-Null

    Invoke-Step 'verify websocket join and accepted controls through authority RPC' {
        $hostSocket = New-SmokeWebSocket -Token $hostUser.token
        $viewerSocket = New-SmokeWebSocket -Token $viewerUser.token

        Send-WsJson -Socket $hostSocket -Message @{
            type = 'join_room'
            payload = @{
                roomId = $roomCode
                userId = $hostUser.userId
                deviceId = 'phase16-host-device'
            }
        }
        $hostState = Read-WsUntil -Socket $hostSocket -Type 'room_state'

        Send-WsJson -Socket $viewerSocket -Message @{
            type = 'join_room'
            payload = @{
                roomId = $roomCode
                userId = $viewerUser.userId
                deviceId = 'phase16-viewer-device'
            }
        }
        Read-WsUntil -Socket $viewerSocket -Type 'room_state' | Out-Null

        $seq = [int64]$hostState.payload.seq
        $controls = @(
            @{ type = 'play'; requestId = "phase16-play-$suffix"; positionMs = 1000 },
            @{ type = 'seek'; requestId = "phase16-seek-$suffix"; positionMs = 5000 },
            @{ type = 'pause'; requestId = "phase16-pause-$suffix"; positionMs = 6000 }
        )
        foreach ($control in $controls) {
            Start-Sleep -Milliseconds 350
            Send-WsJson -Socket $hostSocket -Message @{
                type = $control.type
                payload = @{
                    roomId = $roomCode
                    userId = $hostUser.userId
                    requestId = $control.requestId
                    positionMs = $control.positionMs
                    seq = $seq
                }
            }
            $acceptedHost = Read-WsUntil -Socket $hostSocket -Type $control.type -RequestID $control.requestId
            $acceptedViewer = Read-WsUntil -Socket $viewerSocket -Type $control.type -RequestID $control.requestId
            $seq = [int64]$acceptedHost.payload.seq
            if ([int64]$acceptedViewer.payload.seq -ne $seq) {
                throw "viewer received seq $($acceptedViewer.payload.seq), host received seq $seq"
            }
        }
    }

    Invoke-Step 'verify authority metrics and timeline outbox' {
        $metrics = (Invoke-WebRequest -Uri $AuthorityMetricsUrl -UseBasicParsing -TimeoutSec 15).Content
        if (-not $metrics.Contains('authority_rpc_requests_total') -or -not $metrics.Contains('ApplyRoomControl')) {
            throw 'authority metrics do not include ApplyRoomControl RPC records'
        }
        $countSql = "SELECT COUNT(*) FROM room_timeline_outbox WHERE room_id = '$roomCode' AND event_type = 'room.control.accepted';"
        $count = Invoke-PostgresSQL -Database 'anime_watch_timeline_dev' -Sql $countSql -TupleOnly
        if ([int]$count -lt 1) {
            throw "expected accepted control timeline outbox rows for room $roomCode"
        }
    }

    Write-Host 'Phase 16 app RPC smoke completed.'
} finally {
    Close-SmokeWebSocket -Socket $hostSocket
    Close-SmokeWebSocket -Socket $viewerSocket
    if ($shouldDown -and $composeStarted) {
        Invoke-Compose -Arguments @('--profile', 'app', 'down', '--remove-orphans')
    }
}
