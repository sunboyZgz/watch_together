param(
    [switch] $ResetVolumes,
    [switch] $DownAfterRun,
    [switch] $KeepRunning,
    [switch] $SkipBuild
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ServerRoot = (Resolve-Path (Join-Path $ScriptDir '..')).Path

$BaseUrl = 'http://127.0.0.1:8080'
$APIGatewayReadyUrl = 'http://127.0.0.1:8097/readyz'
$RoomserverReadyUrl = 'http://127.0.0.1:8098/readyz'
$IdentityServiceReadyUrl = 'http://127.0.0.1:8093/readyz'
$MediaServiceReadyUrl = 'http://127.0.0.1:8090/readyz'
$TimelineServiceReadyUrl = 'http://127.0.0.1:8091/readyz'
$AuthorityServiceReadyUrl = 'http://127.0.0.1:8092/readyz'
$RoomServiceReadyUrl = 'http://127.0.0.1:8094/readyz'
$ProgressServiceReadyUrl = 'http://127.0.0.1:8095/readyz'
$HomeServiceReadyUrl = 'http://127.0.0.1:8096/readyz'
$AuthorityMetricsUrl = 'http://127.0.0.1:8092/metrics'

$MainDatabaseUrl = 'postgres://app:app@postgres:5432/anime_watch_dev?sslmode=disable'
$IdentityDatabaseUrl = 'postgres://app:app@postgres:5432/anime_watch_identity_dev?sslmode=disable'
$RoomDatabaseUrl = 'postgres://app:app@postgres:5432/anime_watch_room_dev?sslmode=disable'
$MediaDatabaseUrl = 'postgres://app:app@postgres:5432/anime_watch_media_dev?sslmode=disable'
$ProgressDatabaseUrl = 'postgres://app:app@postgres:5432/anime_watch_progress_dev?sslmode=disable'
$TimelineDatabaseUrl = 'postgres://app:app@postgres:5432/anime_watch_timeline_dev?sslmode=disable'

$SmokeEpisodeID = '00000000-0000-0000-0000-000000230102'

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
        throw 'Docker daemon is not available. Start Docker Desktop, then rerun scripts/verify_phase23.ps1 -RunSmoke.'
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

function Write-SmokeDiagnostics {
    Write-Host '==> compose diagnostics'
    Push-Location $ServerRoot
    try {
        & docker compose --profile app ps
        $services = @(
            'postgres',
            'identity-postgres-init',
            'room-postgres-init',
            'media-postgres-init',
            'progress-postgres-init',
            'timeline-postgres-init',
            'apigateway',
            'roomserver',
            'identityservice',
            'roomservice',
            'mediaservice',
            'progressservice',
            'homecompositionservice',
            'timelineservice',
            'roomauthorityservice',
            'nginx'
        )
        foreach ($service in $services) {
            Write-Host "---- logs: $service ----"
            & docker compose --profile app logs --tail 80 $service
        }
    } catch {
        Write-Host "failed to collect diagnostics: $($_.Exception.Message)"
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

function Invoke-ScalarInt {
    param(
        [string] $Database,
        [string] $Sql
    )
    $value = Invoke-PostgresSQL -Database $Database -Sql $Sql -TupleOnly
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "query returned no scalar value for database $Database"
    }
    return [int]$value
}

function Assert-EqualInt {
    param(
        [string] $Name,
        [int] $Actual,
        [int] $Expected
    )
    if ($Actual -ne $Expected) {
        throw "$Name expected $Expected, got $Actual"
    }
}

function Assert-AtLeastInt {
    param(
        [string] $Name,
        [int] $Actual,
        [int] $Minimum
    )
    if ($Actual -lt $Minimum) {
        throw "$Name expected at least $Minimum, got $Actual"
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
        password = 'Phase23Smoke123!'
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
    [void]$socket.Options.SetRequestHeader('Authorization', "Bearer $Token")
    $cts = [System.Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(15))
    [void]$socket.ConnectAsync([Uri]'ws://127.0.0.1:8080/ws', $cts.Token).GetAwaiter().GetResult()
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
    [void]$Socket.SendAsync($segment, [System.Net.WebSockets.WebSocketMessageType]::Text, $true, [System.Threading.CancellationToken]::None).GetAwaiter().GetResult()
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
        [void]$Socket.CloseAsync([System.Net.WebSockets.WebSocketCloseStatus]::NormalClosure, 'smoke done', [System.Threading.CancellationToken]::None).GetAwaiter().GetResult()
    }
    $Socket.Dispose()
}

function Seed-SmokeMedia {
    $sql = @"
INSERT INTO media_tags (id, slug, name, sort_order, is_featured, is_active, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000230100', 'phase23-smoke', 'Phase 23 Smoke', 0, true, true, NOW(), NOW())
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
    '00000000-0000-0000-0000-000000230101',
    'phase23-smoke-season',
    'Phase23 Smoke Season',
    NULL,
    'Phase23 smoke media for full RPC multi database verification',
    NULL,
    'smoke',
    'watch_together',
    '["phase23","smoke"]'::jsonb,
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
    '00000000-0000-0000-0000-000000230101',
    'Phase23 Smoke Episode',
    'Full RPC multi database path',
    'Deterministic playable episode for Phase23 smoke verification',
    NULL,
    'phase23-smoke/master.m3u8',
    120000,
    1,
    'Episode 1',
    'phase23-smoke/source.mp4',
    'phase23-smoke-hash',
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
VALUES ('00000000-0000-0000-0000-000000230101', '00000000-0000-0000-0000-000000230100', NOW())
ON CONFLICT DO NOTHING;
"@
    Invoke-PostgresSQL -Database 'anime_watch_media_dev' -Sql $sql | Out-Null
}

$hostSocket = $null
$viewerSocket = $null
$shouldDown = $DownAfterRun -and -not $KeepRunning
$composeStarted = $false
$hostUser = $null
$viewerUser = $null
$roomCode = ''

try {
    Invoke-Step 'check Docker daemon' {
        Test-DockerAvailable
    }

    Invoke-Step 'start postgres and owner database init jobs' {
        if ($ResetVolumes) {
            Invoke-Compose -Arguments @('--profile', 'app', 'down', '-v', '--remove-orphans')
        }
        Invoke-Compose -Arguments @('--profile', 'app', 'up', '-d', 'postgres', 'redis', 'nats', 'kafka', 'minio')
        $composeStarted = $true
        Wait-PostgresDatabase -Database 'anime_watch_dev'
        foreach ($job in @(
            'identity-postgres-init',
            'room-postgres-init',
            'media-postgres-init',
            'progress-postgres-init',
            'timeline-postgres-init',
            'minio-init'
        )) {
            Invoke-Compose -Arguments @('--profile', 'app', 'up', '--no-deps', $job)
        }
        Wait-PostgresDatabase -Database 'anime_watch_identity_dev'
        Wait-PostgresDatabase -Database 'anime_watch_room_dev'
        Wait-PostgresDatabase -Database 'anime_watch_media_dev'
        Wait-PostgresDatabase -Database 'anime_watch_progress_dev'
        Wait-PostgresDatabase -Database 'anime_watch_timeline_dev'
    }

    Invoke-Step 'apply main and owner database migrations' {
        Invoke-Migrations -DatabaseUrl $MainDatabaseUrl -MigrationsDir (Join-Path $ServerRoot 'migrations')
        Invoke-Migrations -DatabaseUrl $IdentityDatabaseUrl -MigrationsDir (Join-Path $ServerRoot 'identity_migrations')
        Invoke-Migrations -DatabaseUrl $RoomDatabaseUrl -MigrationsDir (Join-Path $ServerRoot 'room_migrations')
        Invoke-Migrations -DatabaseUrl $MediaDatabaseUrl -MigrationsDir (Join-Path $ServerRoot 'media_migrations')
        Invoke-Migrations -DatabaseUrl $ProgressDatabaseUrl -MigrationsDir (Join-Path $ServerRoot 'progress_migrations')
        Invoke-Migrations -DatabaseUrl $TimelineDatabaseUrl -MigrationsDir (Join-Path $ServerRoot 'timeline_migrations')
    }

    Invoke-Step 'seed deterministic media episode in media database' {
        Seed-SmokeMedia
    }

    Invoke-Step 'start local app full RPC profile' {
        $composeArgs = @('--profile', 'app', 'up', '-d')
        if (-not $SkipBuild) {
            $composeArgs += '--build'
        }
        Invoke-Compose -Arguments $composeArgs
    }

    Invoke-Step 'wait for gateway, roomserver, and internal service readiness' {
        Wait-HttpReady -Url "$BaseUrl/readyz" -Name 'public nginx gateway'
        Wait-HttpReady -Url $APIGatewayReadyUrl -Name 'apigateway'
        Wait-HttpReady -Url $RoomserverReadyUrl -Name 'roomserver'
        Wait-HttpReady -Url $IdentityServiceReadyUrl -Name 'identityservice'
        Wait-HttpReady -Url $RoomServiceReadyUrl -Name 'roomservice'
        Wait-HttpReady -Url $MediaServiceReadyUrl -Name 'mediaservice'
        Wait-HttpReady -Url $TimelineServiceReadyUrl -Name 'timelineservice'
        Wait-HttpReady -Url $AuthorityServiceReadyUrl -Name 'roomauthorityservice'
        Wait-HttpReady -Url $ProgressServiceReadyUrl -Name 'progressservice'
        Wait-HttpReady -Url $HomeServiceReadyUrl -Name 'homecompositionservice'
    }

    $suffix = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    $hostUser = New-SmokeUser -Account "phase23_host_$suffix" -Nickname 'Phase23 Host'
    $viewerUser = New-SmokeUser -Account "phase23_viewer_$suffix" -Nickname 'Phase23 Viewer'

    Invoke-Step 'verify media discovery through roomserver RPC adapter' {
        $media = Invoke-JsonRequest -Method Get -Uri "$BaseUrl/media/items?query=Phase23&limit=5" -Token $hostUser.token
        $ids = @($media.data.items | ForEach-Object { $_.id })
        if ($ids -notcontains $SmokeEpisodeID) {
            throw "smoke media episode $SmokeEpisodeID not found in media/items response"
        }
    }

    Invoke-Step 'create room, join viewer, and fetch room detail through room RPC' {
        $room = Invoke-JsonRequest -Method Post -Uri "$BaseUrl/rooms" -Token $hostUser.token -Body @{ mediaItemId = $SmokeEpisodeID }
        $script:roomCode = $room.data.room.roomCode
        if ([string]::IsNullOrWhiteSpace($script:roomCode)) {
            throw 'room create did not return a room code'
        }
        Invoke-JsonRequest -Method Post -Uri "$BaseUrl/rooms/$script:roomCode/join" -Token $viewerUser.token | Out-Null
        $detail = Invoke-JsonRequest -Method Get -Uri "$BaseUrl/rooms/$script:roomCode" -Token $viewerUser.token
        if ($detail.data.room.roomCode -ne $script:roomCode) {
            throw "room detail returned $($detail.data.room.roomCode), want $script:roomCode"
        }
        if (@($detail.data.members).Count -lt 2) {
            throw 'room detail did not include both smoke members'
        }
    }

    Invoke-Step 'write progress and verify home summary composition' {
        $progress = Invoke-JsonRequest -Method Put -Uri "$BaseUrl/me/media-progress/$SmokeEpisodeID" -Token $hostUser.token -Body @{
            lastPositionSeconds = 42
            durationSeconds = 120
            completed = $false
        }
        if ($progress.data.mediaItemId -ne $SmokeEpisodeID) {
            throw "progress response mediaItemId=$($progress.data.mediaItemId), want $SmokeEpisodeID"
        }
        $homeSummary = Invoke-JsonRequest -Method Get -Uri "$BaseUrl/home/summary" -Token $hostUser.token
        if ($null -eq $homeSummary.data.lastWatched) {
            throw 'home summary did not return lastWatched after progress update'
        }
        if ($homeSummary.data.lastWatched.mediaItemId -ne $SmokeEpisodeID) {
            throw "home lastWatched mediaItemId=$($homeSummary.data.lastWatched.mediaItemId), want $SmokeEpisodeID"
        }
        if ($homeSummary.data.lastWatched.title -ne 'Phase23 Smoke Season') {
            throw "home lastWatched title=$($homeSummary.data.lastWatched.title), want Phase23 Smoke Season"
        }
    }

    Invoke-Step 'verify websocket join and accepted controls through authority RPC' {
        $hostSocket = New-SmokeWebSocket -Token $hostUser.token
        $viewerSocket = New-SmokeWebSocket -Token $viewerUser.token

        Send-WsJson -Socket $hostSocket -Message @{
            type = 'join_room'
            payload = @{
                roomId = $script:roomCode
                userId = $hostUser.userId
                deviceId = 'phase23-host-device'
            }
        }
        $hostState = Read-WsUntil -Socket $hostSocket -Type 'room_state'

        Send-WsJson -Socket $viewerSocket -Message @{
            type = 'join_room'
            payload = @{
                roomId = $script:roomCode
                userId = $viewerUser.userId
                deviceId = 'phase23-viewer-device'
            }
        }
        Read-WsUntil -Socket $viewerSocket -Type 'room_state' | Out-Null

        $seq = [int64]$hostState.payload.seq
        $controls = @(
            @{ type = 'play'; requestId = "phase23-play-$suffix"; positionMs = 1000 },
            @{ type = 'seek'; requestId = "phase23-seek-$suffix"; positionMs = 5000 },
            @{ type = 'pause'; requestId = "phase23-pause-$suffix"; positionMs = 6000 }
        )
        foreach ($control in $controls) {
            Start-Sleep -Milliseconds 350
            Send-WsJson -Socket $hostSocket -Message @{
                type = $control.type
                payload = @{
                    roomId = $script:roomCode
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

    Invoke-Step 'verify owner databases received writes and main owner tables are absent' {
        $identityCount = Invoke-ScalarInt -Database 'anime_watch_identity_dev' -Sql "SELECT COUNT(*) FROM users WHERE id IN ('$($hostUser.userId)', '$($viewerUser.userId)');"
        Assert-EqualInt -Name 'identity DB smoke users' -Actual $identityCount -Expected 2

        $roomCount = Invoke-ScalarInt -Database 'anime_watch_room_dev' -Sql "SELECT COUNT(*) FROM rooms WHERE room_code = '$script:roomCode';"
        Assert-EqualInt -Name 'room DB smoke room' -Actual $roomCount -Expected 1
        $memberCount = Invoke-ScalarInt -Database 'anime_watch_room_dev' -Sql "SELECT COUNT(*) FROM rooms r JOIN room_members m ON m.room_id = r.id WHERE r.room_code = '$script:roomCode' AND m.user_id IN ('$($hostUser.userId)', '$($viewerUser.userId)');"
        Assert-EqualInt -Name 'room DB smoke members' -Actual $memberCount -Expected 2

        $progressCount = Invoke-ScalarInt -Database 'anime_watch_progress_dev' -Sql "SELECT COUNT(*) FROM user_media_progress WHERE user_id = '$($hostUser.userId)' AND media_episode_id = '$SmokeEpisodeID';"
        Assert-EqualInt -Name 'progress DB smoke progress' -Actual $progressCount -Expected 1

        $timelineCount = Invoke-ScalarInt -Database 'anime_watch_timeline_dev' -Sql "SELECT COUNT(*) FROM room_timeline_outbox WHERE room_id = '$script:roomCode' AND event_type = 'room.control.accepted';"
        Assert-AtLeastInt -Name 'timeline DB accepted outbox rows' -Actual $timelineCount -Minimum 3

        $mainOwnerTableCount = Invoke-ScalarInt -Database 'anime_watch_dev' -Sql @"
SELECT COUNT(*)
FROM (
    VALUES
        ('users'),
        ('rooms'),
        ('room_members'),
        ('media_tags'),
        ('media_seasons'),
        ('media_episodes'),
        ('media_season_tags'),
        ('media_episode_variants'),
        ('media_items'),
        ('media_item_tags'),
        ('user_media_progress'),
        ('room_timeline_outbox')
) AS shadow(table_name)
WHERE to_regclass('public.' || shadow.table_name) IS NOT NULL;
"@
        Assert-EqualInt -Name 'main DB owner table count' -Actual $mainOwnerTableCount -Expected 0
    }

    Invoke-Step 'verify authority RPC metrics include control application' {
        $metrics = (Invoke-WebRequest -Uri $AuthorityMetricsUrl -UseBasicParsing -TimeoutSec 15).Content
        if (-not $metrics.Contains('authority_rpc_requests_total') -or -not $metrics.Contains('ApplyRoomControl')) {
            throw 'authority metrics do not include ApplyRoomControl RPC records'
        }
    }

    Write-Host 'Phase 27 full RPC owner-database smoke completed.'
} catch {
    Write-SmokeDiagnostics
    throw
} finally {
    Close-SmokeWebSocket -Socket $hostSocket
    Close-SmokeWebSocket -Socket $viewerSocket
    if ($shouldDown -and $composeStarted) {
        Invoke-Compose -Arguments @('--profile', 'app', 'down', '--remove-orphans')
    }
}
