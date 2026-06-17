param(
    [switch] $ResetVolumes,
    [switch] $DownAfterRun,
    [switch] $KeepRunning,
    [switch] $SkipBuild
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ServerRoot = (Resolve-Path (Join-Path $ScriptDir '..')).Path

$BaseUrl = 'http://127.0.0.1:8081'
$PrimaryRoomserverReadyUrl = 'http://127.0.0.1:8098/readyz'
$PrimaryRoomserverHealthUrl = 'http://127.0.0.1:8098/healthz'
$SecondaryRoomserverReadyUrl = 'http://127.0.0.1:8099/readyz'
$SecondaryRoomserverMetricsUrl = 'http://127.0.0.1:8099/metrics'
$PrimaryWsUrl = 'ws://127.0.0.1:8098/ws'
$RollingWsUrl = 'ws://127.0.0.1:8081/ws'

$MainDatabase = 'anime_watch_dev'
$IdentityDatabase = 'anime_watch_identity_dev'
$RoomDatabase = 'anime_watch_room_dev'
$ProgressDatabase = 'anime_watch_progress_dev'
$TimelineDatabase = 'anime_watch_timeline_dev'
$SmokeEpisodeID = '00000000-0000-0000-0000-000000230102'

function Invoke-Step {
    param(
        [string] $Name,
        [scriptblock] $Block
    )
    Write-Host "==> $Name"
    & $Block
}

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
        throw 'Docker daemon is not available. Start Docker Desktop, then rerun scripts/verify_phase29.ps1 -RunSmoke.'
    }
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
        & docker compose --profile app --profile rolling-smoke ps
        $services = @(
            'apigateway',
            'roomserver',
            'roomserver-rolling',
            'identityservice',
            'roomservice',
            'mediaservice',
            'progressservice',
            'homecompositionservice',
            'timelineservice',
            'roomauthorityservice',
            'nginx',
            'nginx-rolling'
        )
        foreach ($service in $services) {
            Write-Host "---- logs: $service ----"
            & docker compose --profile app --profile rolling-smoke logs --tail 100 $service
        }
    } catch {
        Write-Host "failed to collect diagnostics: $($_.Exception.Message)"
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
            if ($response -is [string] -and $response.Trim() -eq 'ok') {
                return
            }
            if ($response.status -eq 'ready' -or $response.status -eq 'ok') {
                return
            }
        } catch {
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "$Name did not become ready at $Url"
}

function Wait-ServiceStopped {
    param(
        [string] $Service,
        [int] $TimeoutSeconds = 30
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        Push-Location $ServerRoot
        try {
            $containerID = ((& docker compose ps -q $Service) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -First 1)
            if ($null -ne $containerID) {
                $containerID = $containerID.Trim()
            }
            if ([string]::IsNullOrWhiteSpace($containerID)) {
                return
            }
            $running = (& docker inspect -f '{{.State.Running}}' $containerID).Trim()
            if ($running -ne 'true') {
                return
            }
        } finally {
            Pop-Location
        }
        Start-Sleep -Milliseconds 500
    } while ((Get-Date) -lt $deadline)
    throw "compose service $Service did not stop within $TimeoutSeconds seconds"
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
        password = 'Phase29Smoke123!'
        nickname = $Nickname
    }
    return @{
        userId = $response.data.user.id
        token = $response.data.accessToken
    }
}

function New-SmokeWebSocket {
    param(
        [string] $Token,
        [string] $WsUrl
    )
    $socket = [System.Net.WebSockets.ClientWebSocket]::new()
    [void]$socket.Options.SetRequestHeader('Authorization', "Bearer $Token")
    $cts = [System.Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(15))
    [void]$socket.ConnectAsync([Uri]$WsUrl, $cts.Token).GetAwaiter().GetResult()
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
            throw "websocket closed before expected message: $($result.CloseStatus) $($result.CloseStatusDescription)"
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

function Wait-WsClosed {
    param(
        [System.Net.WebSockets.ClientWebSocket] $Socket,
        [string] $Name,
        [int] $TimeoutSeconds = 20
    )
    if ($null -eq $Socket) {
        return
    }
    $buffer = New-Object byte[] 4096
    $cts = [System.Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds($TimeoutSeconds))
    try {
        do {
            $segment = [System.ArraySegment[byte]]::new($buffer, 0, $buffer.Length)
            $result = $Socket.ReceiveAsync($segment, $cts.Token).GetAwaiter().GetResult()
            if ($result.MessageType -eq [System.Net.WebSockets.WebSocketMessageType]::Close) {
                $reason = [string]$result.CloseStatusDescription
                if (-not [string]::IsNullOrWhiteSpace($reason) -and $reason -ne 'server draining') {
                    throw "$Name closed with reason '$reason', want server draining"
                }
                return
            }
        } while ($true)
    } catch [System.OperationCanceledException] {
        throw "timed out waiting for $Name websocket to close"
    } catch {
        if ($Socket.State -eq [System.Net.WebSockets.WebSocketState]::Closed -or
            $Socket.State -eq [System.Net.WebSockets.WebSocketState]::Aborted) {
            return
        }
        throw
    }
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

function Join-SmokeRoom {
    param(
        [System.Net.WebSockets.ClientWebSocket] $Socket,
        [string] $RoomCode,
        [string] $UserID,
        [string] $DeviceID
    )
    Send-WsJson -Socket $Socket -Message @{
        type = 'join_room'
        payload = @{
            roomId = $RoomCode
            userId = $UserID
            deviceId = $DeviceID
        }
    }
    return Read-WsUntil -Socket $Socket -Type 'room_state'
}

function Assert-ControlsAccepted {
    param(
        [System.Net.WebSockets.ClientWebSocket] $HostSocket,
        [System.Net.WebSockets.ClientWebSocket] $ViewerSocket,
        [string] $RoomCode,
        [string] $HostUserID,
        [int64] $InitialSeq,
        [string] $RequestSuffix
    )
    $seq = $InitialSeq
    $controls = @(
        @{ type = 'play'; requestId = "phase29-play-$RequestSuffix"; positionMs = 1000 },
        @{ type = 'seek'; requestId = "phase29-seek-$RequestSuffix"; positionMs = 5000 },
        @{ type = 'pause'; requestId = "phase29-pause-$RequestSuffix"; positionMs = 6000 }
    )
    foreach ($control in $controls) {
        Start-Sleep -Milliseconds 350
        Send-WsJson -Socket $HostSocket -Message @{
            type = $control.type
            payload = @{
                roomId = $RoomCode
                userId = $HostUserID
                requestId = $control.requestId
                positionMs = $control.positionMs
                seq = $seq
            }
        }
        $acceptedHost = Read-WsUntil -Socket $HostSocket -Type $control.type -RequestID $control.requestId
        $acceptedViewer = Read-WsUntil -Socket $ViewerSocket -Type $control.type -RequestID $control.requestId
        $seq = [int64]$acceptedHost.payload.seq
        if ([int64]$acceptedViewer.payload.seq -ne $seq) {
            throw "viewer received seq $($acceptedViewer.payload.seq), host received seq $seq"
        }
    }
}

$hostSocket = $null
$viewerSocket = $null
$oldHostSocket = $null
$oldViewerSocket = $null
$shouldDown = $DownAfterRun -and -not $KeepRunning
$hostUser = $null
$viewerUser = $null
$roomCode = ''

try {
    Invoke-Step 'check Docker daemon' {
        Test-DockerAvailable
    }

    Invoke-Step 'prepare Phase 23 full-RPC owner-database baseline' {
        if ($ResetVolumes) {
            Invoke-Compose -Arguments @('--profile', 'app', '--profile', 'rolling-smoke', 'down', '-v', '--remove-orphans')
        }
        $phase23Args = @('-KeepRunning')
        if ($SkipBuild) {
            $phase23Args += '-SkipBuild'
        }
        Push-Location $ServerRoot
        try {
            & .\scripts\smoke_phase23_full_rpc.ps1 @phase23Args
        } finally {
            Pop-Location
        }
    }

    Invoke-Step 'start rolling-smoke roomserver and nginx profile' {
        Invoke-Compose -Arguments @('--profile', 'app', '--profile', 'rolling-smoke', 'up', '-d', 'roomserver-rolling', 'nginx-rolling')
    }

    Invoke-Step 'wait for primary, secondary, and rolling nginx readiness' {
        Wait-HttpReady -Url $PrimaryRoomserverReadyUrl -Name 'primary roomserver'
        Wait-HttpReady -Url $PrimaryRoomserverHealthUrl -Name 'primary roomserver health'
        Wait-HttpReady -Url $SecondaryRoomserverReadyUrl -Name 'secondary roomserver'
        Wait-HttpReady -Url "$BaseUrl/readyz" -Name 'rolling nginx public REST gateway'
    }

    $suffix = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    $hostUser = New-SmokeUser -Account "phase29_host_$suffix" -Nickname 'Phase29 Host'
    $viewerUser = New-SmokeUser -Account "phase29_viewer_$suffix" -Nickname 'Phase29 Viewer'

    Invoke-Step 'verify rolling nginx still routes public REST to apigateway' {
        $media = Invoke-JsonRequest -Method Get -Uri "$BaseUrl/media/items?query=Phase23&limit=5" -Token $hostUser.token
        $ids = @($media.data.items | ForEach-Object { $_.id })
        if ($ids -notcontains $SmokeEpisodeID) {
            throw "smoke media episode $SmokeEpisodeID not found through rolling nginx REST path"
        }
    }

    Invoke-Step 'create and join room through public REST before websocket drain' {
        $room = Invoke-JsonRequest -Method Post -Uri "$BaseUrl/rooms" -Token $hostUser.token -Body @{ mediaItemId = $SmokeEpisodeID }
        $script:roomCode = $room.data.room.roomCode
        if ([string]::IsNullOrWhiteSpace($script:roomCode)) {
            throw 'room create did not return a room code'
        }
        Invoke-JsonRequest -Method Post -Uri "$BaseUrl/rooms/$script:roomCode/join" -Token $viewerUser.token | Out-Null
        Invoke-JsonRequest -Method Put -Uri "$BaseUrl/me/media-progress/$SmokeEpisodeID" -Token $hostUser.token -Body @{
            lastPositionSeconds = 29
            durationSeconds = 120
            completed = $false
        } | Out-Null
    }

    Invoke-Step 'open host and viewer WebSockets on the primary roomserver' {
        $hostSocket = New-SmokeWebSocket -Token $hostUser.token -WsUrl $PrimaryWsUrl
        $viewerSocket = New-SmokeWebSocket -Token $viewerUser.token -WsUrl $PrimaryWsUrl
        $hostState = Join-SmokeRoom -Socket $hostSocket -RoomCode $script:roomCode -UserID $hostUser.userId -DeviceID 'phase29-host-device'
        Join-SmokeRoom -Socket $viewerSocket -RoomCode $script:roomCode -UserID $viewerUser.userId -DeviceID 'phase29-viewer-device' | Out-Null
        if ($hostState.type -ne 'room_state') {
            throw "expected host room_state, got $($hostState.type)"
        }
    }

    Invoke-Step 'drain the primary roomserver with SIGTERM' {
        $oldHostSocket = $hostSocket
        $oldViewerSocket = $viewerSocket
        Invoke-Compose -Arguments @('--profile', 'app', '--profile', 'rolling-smoke', 'kill', '-s', 'SIGTERM', 'roomserver')
        Wait-WsClosed -Socket $oldHostSocket -Name 'host primary'
        Wait-WsClosed -Socket $oldViewerSocket -Name 'viewer primary'
        Close-SmokeWebSocket -Socket $oldHostSocket
        Close-SmokeWebSocket -Socket $oldViewerSocket
        $hostSocket = $null
        $viewerSocket = $null
        Wait-ServiceStopped -Service 'roomserver'
        Wait-HttpReady -Url $SecondaryRoomserverReadyUrl -Name 'secondary roomserver after primary drain'
    }

    Invoke-Step 'reconnect through rolling nginx and recover room_state on the secondary roomserver' {
        $hostSocket = New-SmokeWebSocket -Token $hostUser.token -WsUrl $RollingWsUrl
        $viewerSocket = New-SmokeWebSocket -Token $viewerUser.token -WsUrl $RollingWsUrl
        $recoveredHostState = Join-SmokeRoom -Socket $hostSocket -RoomCode $script:roomCode -UserID $hostUser.userId -DeviceID 'phase29-host-device'
        Join-SmokeRoom -Socket $viewerSocket -RoomCode $script:roomCode -UserID $viewerUser.userId -DeviceID 'phase29-viewer-device' | Out-Null
        if ($recoveredHostState.payload.roomId -ne $script:roomCode) {
            throw "recovered room_state roomId=$($recoveredHostState.payload.roomId), want $script:roomCode"
        }
        Assert-ControlsAccepted -HostSocket $hostSocket -ViewerSocket $viewerSocket -RoomCode $script:roomCode -HostUserID $hostUser.userId -InitialSeq ([int64]$recoveredHostState.payload.seq) -RequestSuffix $suffix
    }

    Invoke-Step 'verify owner databases and absent main owner tables after reconnect controls' {
        $identityCount = Invoke-ScalarInt -Database $IdentityDatabase -Sql "SELECT COUNT(*) FROM users WHERE id IN ('$($hostUser.userId)', '$($viewerUser.userId)');"
        Assert-EqualInt -Name 'identity DB phase29 users' -Actual $identityCount -Expected 2

        $roomCount = Invoke-ScalarInt -Database $RoomDatabase -Sql "SELECT COUNT(*) FROM rooms WHERE room_code = '$script:roomCode';"
        Assert-EqualInt -Name 'room DB phase29 room' -Actual $roomCount -Expected 1
        $memberCount = Invoke-ScalarInt -Database $RoomDatabase -Sql "SELECT COUNT(*) FROM rooms r JOIN room_members m ON m.room_id = r.id WHERE r.room_code = '$script:roomCode' AND m.user_id IN ('$($hostUser.userId)', '$($viewerUser.userId)');"
        Assert-EqualInt -Name 'room DB phase29 members' -Actual $memberCount -Expected 2

        $progressCount = Invoke-ScalarInt -Database $ProgressDatabase -Sql "SELECT COUNT(*) FROM user_media_progress WHERE user_id = '$($hostUser.userId)' AND media_episode_id = '$SmokeEpisodeID';"
        Assert-EqualInt -Name 'progress DB phase29 progress' -Actual $progressCount -Expected 1

        $timelineCount = Invoke-ScalarInt -Database $TimelineDatabase -Sql "SELECT COUNT(*) FROM room_timeline_outbox WHERE room_id = '$script:roomCode' AND event_type = 'room.control.accepted';"
        Assert-AtLeastInt -Name 'timeline DB phase29 accepted outbox rows' -Actual $timelineCount -Minimum 3

        $mainOwnerTableCount = Invoke-ScalarInt -Database $MainDatabase -Sql @"
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

    Invoke-Step 'verify roomserver rolling metrics are exported' {
        $metrics = (Invoke-WebRequest -Uri $SecondaryRoomserverMetricsUrl -UseBasicParsing -TimeoutSec 15).Content
        foreach ($expected in @(
            'watch_together_roomserver_draining',
            'watch_together_websocket_drain_closes_total',
            'watch_together_websocket_reconnect_joins_total'
        )) {
            if (-not $metrics.Contains($expected)) {
                throw "secondary roomserver metrics do not include $expected"
            }
        }
    }

    Write-Host 'Phase 29 rolling drain smoke completed.'
} catch {
    Write-SmokeDiagnostics
    throw
} finally {
    Close-SmokeWebSocket -Socket $hostSocket
    Close-SmokeWebSocket -Socket $viewerSocket
    if ($shouldDown) {
        Invoke-Compose -Arguments @('--profile', 'app', '--profile', 'rolling-smoke', 'down', '--remove-orphans')
    }
}
