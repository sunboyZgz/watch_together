param(
    [switch] $ResetCluster,
    [switch] $DeleteAfterRun,
    [switch] $KeepCluster,
    [switch] $SkipBuild,
    [switch] $SkipImageLoad,
    [string] $ClusterName = 'watch-together-dev'
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ServerRoot = (Resolve-Path (Join-Path $ScriptDir '..')).Path
$Namespace = 'watch-together'
$BaseUrl = 'http://127.0.0.1:30080'
$WsUrl = 'ws://127.0.0.1:30080/ws'
$SmokeEpisodeID = '00000000-0000-0000-0000-000000230102'
$shouldDeleteCluster = $DeleteAfterRun -and -not $KeepCluster

Add-Type -TypeDefinition @'
using System;
using System.Collections.Concurrent;
using System.IO;
using System.Net.WebSockets;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading;
using System.Threading.Tasks;

public sealed class Phase30SmokeWebSocket : IDisposable
{
    private readonly BlockingCollection<string> messages = new BlockingCollection<string>();
    private readonly SemaphoreSlim sendLock = new SemaphoreSlim(1, 1);
    private readonly CancellationTokenSource receiveCancel = new CancellationTokenSource();
    private readonly Task receiveTask;
    private bool disposed;

    public ClientWebSocket Socket { get; private set; }
    public WebSocketCloseStatus? CloseStatus { get; private set; }
    public string CloseDescription { get; private set; }
    public Exception ReceiveError { get; private set; }

    private Phase30SmokeWebSocket(ClientWebSocket socket)
    {
        Socket = socket;
        receiveTask = Task.Run((Func<Task>)ReceiveLoop);
    }

    public static Phase30SmokeWebSocket Connect(string url, string token, int connectTimeoutSeconds)
    {
        ClientWebSocket socket = new ClientWebSocket();
        socket.Options.SetRequestHeader("Authorization", "Bearer " + token);
        CancellationTokenSource connectCancel = new CancellationTokenSource(TimeSpan.FromSeconds(connectTimeoutSeconds));
        try
        {
            socket.ConnectAsync(new Uri(url), connectCancel.Token).GetAwaiter().GetResult();
            return new Phase30SmokeWebSocket(socket);
        }
        catch
        {
            socket.Dispose();
            throw;
        }
        finally
        {
            connectCancel.Dispose();
        }
    }

    public void Send(string text)
    {
        SendStringAsync(text).GetAwaiter().GetResult();
    }

    public string Receive(int timeoutSeconds)
    {
        string message;
        if (messages.TryTake(out message, TimeSpan.FromSeconds(timeoutSeconds)))
        {
            return message;
        }
        if (ReceiveError != null)
        {
            throw new InvalidOperationException("websocket receive loop failed", ReceiveError);
        }
        if (CloseStatus.HasValue)
        {
            throw new InvalidOperationException("websocket closed before expected message: " + CloseStatus.Value + " " + CloseDescription);
        }
        throw new TimeoutException("timed out waiting for websocket message");
    }

    public bool WaitClosed(int timeoutSeconds)
    {
        DateTime deadline = DateTime.UtcNow.AddSeconds(timeoutSeconds);
        while (DateTime.UtcNow < deadline)
        {
            if (CloseStatus.HasValue ||
                Socket.State == WebSocketState.Closed ||
                Socket.State == WebSocketState.Aborted)
            {
                return true;
            }
            Thread.Sleep(100);
        }
        return false;
    }

    private async Task ReceiveLoop()
    {
        byte[] buffer = new byte[8192];
        try
        {
            while (!receiveCancel.IsCancellationRequested)
            {
                using (MemoryStream stream = new MemoryStream())
                {
                    WebSocketReceiveResult result;
                    do
                    {
                        ArraySegment<byte> segment = new ArraySegment<byte>(buffer);
                        result = await Socket.ReceiveAsync(segment, receiveCancel.Token).ConfigureAwait(false);
                        if (result.MessageType == WebSocketMessageType.Close)
                        {
                            CloseStatus = result.CloseStatus;
                            CloseDescription = result.CloseStatusDescription;
                            messages.CompleteAdding();
                            return;
                        }
                        stream.Write(buffer, 0, result.Count);
                    } while (!result.EndOfMessage);

                    string text = Encoding.UTF8.GetString(stream.ToArray());
                    string heartbeatAck;
                    if (TryBuildHeartbeatAck(text, out heartbeatAck))
                    {
                        await SendStringAsync(heartbeatAck).ConfigureAwait(false);
                    }
                    else
                    {
                        messages.Add(text, receiveCancel.Token);
                    }
                }
            }
        }
        catch (OperationCanceledException)
        {
        }
        catch (Exception ex)
        {
            ReceiveError = ex;
            messages.CompleteAdding();
        }
    }

    private async Task SendStringAsync(string text)
    {
        byte[] bytes = Encoding.UTF8.GetBytes(text);
        ArraySegment<byte> segment = new ArraySegment<byte>(bytes);
        await sendLock.WaitAsync().ConfigureAwait(false);
        try
        {
            await Socket.SendAsync(segment, WebSocketMessageType.Text, true, CancellationToken.None).ConfigureAwait(false);
        }
        finally
        {
            sendLock.Release();
        }
    }

    private static bool TryBuildHeartbeatAck(string text, out string heartbeatAck)
    {
        heartbeatAck = null;
        if (!Regex.IsMatch(text, "\"type\"\\s*:\\s*\"heartbeat\""))
        {
            return false;
        }
        Match match = Regex.Match(text, "\"serverTimeMs\"\\s*:\\s*(\\d+)");
        if (!match.Success)
        {
            return false;
        }
        long clientTimeMs = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
        heartbeatAck = "{\"type\":\"heartbeat_ack\",\"payload\":{\"serverTimeMs\":" + match.Groups[1].Value + ",\"clientTimeMs\":" + clientTimeMs + "}}";
        return true;
    }

    public void Dispose()
    {
        if (disposed)
        {
            return;
        }
        disposed = true;
        receiveCancel.Cancel();
        try
        {
            if (Socket.State == WebSocketState.Open)
            {
                Socket.CloseAsync(WebSocketCloseStatus.NormalClosure, "smoke done", CancellationToken.None).Wait(TimeSpan.FromSeconds(5));
            }
        }
        catch
        {
        }
        Socket.Dispose();
        receiveCancel.Dispose();
        sendLock.Dispose();
        messages.Dispose();
    }
}
'@

function Invoke-Step {
    param(
        [string] $Name,
        [scriptblock] $Block
    )
    Write-Host "==> $Name"
    & $Block
}

function Require-Command {
    param([string] $Name)
    if ($null -eq (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name is not available."
    }
}

function Invoke-Kubectl {
    param([string[]] $Arguments)
    & kubectl @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "kubectl $($Arguments -join ' ') failed"
    }
}

function Invoke-KindScript {
    param(
        [string] $Name,
        [string[]] $Arguments = @()
    )
    Push-Location $ServerRoot
    try {
        & ".\scripts\$Name" @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "$Name failed"
        }
    } finally {
        Pop-Location
    }
}

function Build-LocalImages {
    Push-Location $ServerRoot
    try {
        & docker build -t watch-together-roomserver:dev .
        if ($LASTEXITCODE -ne 0) {
            throw 'docker build watch-together-roomserver:dev failed'
        }
        & docker build -t watch-together-nginx:dev .\deploy\nginx
        if ($LASTEXITCODE -ne 0) {
            throw 'docker build watch-together-nginx:dev failed'
        }
    } finally {
        Pop-Location
    }
}

function Write-KindDiagnostics {
    Write-Host '==> kind diagnostics'
    try {
        & kubectl get pods,jobs,deployments,svc -n $Namespace -o wide
        foreach ($deployment in @(
            'postgres',
            'redis',
            'nats',
            'kafka',
            'minio',
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
        )) {
            Write-Host "---- logs: deployment/$deployment ----"
            & kubectl logs -n $Namespace deployment/$deployment --tail=100 --all-containers=true
        }
        foreach ($job in @(
            'postgres-init-databases',
            'minio-init',
            'migrate-main',
            'migrate-identity',
            'migrate-room',
            'migrate-media',
            'migrate-progress',
            'migrate-timeline',
            'seed-media'
        )) {
            Write-Host "---- logs: job/$job ----"
            & kubectl logs -n $Namespace job/$job --tail=100 --all-containers=true
        }
    } catch {
        Write-Host "failed to collect kind diagnostics: $($_.Exception.Message)"
    }
}

function Apply-MigrationConfigMap {
    param(
        [string] $Name,
        [string] $Directory
    )
    $resolved = (Resolve-Path $Directory).Path
    & kubectl -n $Namespace create configmap $Name "--from-file=$resolved" --dry-run=client -o yaml |
        kubectl apply -f -
    if ($LASTEXITCODE -ne 0) {
        throw "apply migration configmap $Name failed"
    }
}

function Prepare-KindManifests {
    $namespaceFile = Join-Path $ServerRoot 'k8s/base/namespace.yaml'
    $overlay = Join-Path $ServerRoot 'k8s/overlays/kind'

    Invoke-Kubectl -Arguments @('apply', '-f', $namespaceFile)
    Apply-MigrationConfigMap -Name 'migrations-main' -Directory (Join-Path $ServerRoot 'migrations')
    Apply-MigrationConfigMap -Name 'migrations-identity' -Directory (Join-Path $ServerRoot 'identity_migrations')
    Apply-MigrationConfigMap -Name 'migrations-room' -Directory (Join-Path $ServerRoot 'room_migrations')
    Apply-MigrationConfigMap -Name 'migrations-media' -Directory (Join-Path $ServerRoot 'media_migrations')
    Apply-MigrationConfigMap -Name 'migrations-progress' -Directory (Join-Path $ServerRoot 'progress_migrations')
    Apply-MigrationConfigMap -Name 'migrations-timeline' -Directory (Join-Path $ServerRoot 'timeline_migrations')

    & kubectl -n $Namespace delete job `
        postgres-init-databases `
        minio-init `
        migrate-main `
        migrate-identity `
        migrate-room `
        migrate-media `
        migrate-progress `
        migrate-timeline `
        seed-media `
        --ignore-not-found
    if ($LASTEXITCODE -ne 0) {
        throw 'delete old kind jobs failed'
    }

    Invoke-Kubectl -Arguments @('apply', '-k', $overlay)
}

function Wait-KindBaseline {
    foreach ($deployment in @('postgres', 'redis', 'nats', 'kafka', 'minio')) {
        Invoke-Kubectl -Arguments @('-n', $Namespace, 'rollout', 'status', "deployment/$deployment", '--timeout=240s')
    }
    foreach ($job in @(
        'postgres-init-databases',
        'minio-init',
        'migrate-main',
        'migrate-identity',
        'migrate-room',
        'migrate-media',
        'migrate-progress',
        'migrate-timeline',
        'seed-media'
    )) {
        Invoke-Kubectl -Arguments @('-n', $Namespace, 'wait', '--for=condition=complete', "job/$job", '--timeout=240s')
    }
    foreach ($deployment in @(
        'identityservice',
        'roomservice',
        'mediaservice',
        'progressservice',
        'homecompositionservice',
        'timelineservice',
        'roomauthorityservice',
        'outboxworker',
        'derivedworker',
        'apigateway',
        'roomserver',
        'nginx'
    )) {
        Invoke-Kubectl -Arguments @('-n', $Namespace, 'rollout', 'status', "deployment/$deployment", '--timeout=240s')
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
        return Invoke-RestMethod -Method $Method -Uri $Uri -Headers $headers -TimeoutSec 20
    }
    $json = $Body | ConvertTo-Json -Depth 20 -Compress
    return Invoke-RestMethod -Method $Method -Uri $Uri -Headers $headers -ContentType 'application/json' -Body $json -TimeoutSec 20
}

function New-SmokeUser {
    param(
        [string] $Account,
        [string] $Nickname
    )
    $response = Invoke-JsonRequest -Method Post -Uri "$BaseUrl/auth/register" -Body @{
        account = $Account
        password = 'Phase30Smoke123!'
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
        [int] $TimeoutSeconds = 60
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $attempt = 0
    $lastError = ''
    do {
        try {
            return [Phase30SmokeWebSocket]::Connect($WsUrl, $Token, 20)
        } catch {
            $lastError = $_.Exception.Message
            $attempt++
            if ((Get-Date) -ge $deadline) {
                break
            }
            $delayMs = [Math]::Min(8000, [int](500 * [Math]::Pow(2, [Math]::Min($attempt - 1, 4))))
            Start-Sleep -Milliseconds $delayMs
        }
    } while ((Get-Date) -lt $deadline)

    throw "websocket connect to $WsUrl failed after $attempt attempts: $lastError"
}

function Send-WsJson {
    param(
        [Phase30SmokeWebSocket] $Socket,
        [object] $Message
    )
    $json = $Message | ConvertTo-Json -Depth 20 -Compress
    $Socket.Send($json)
}

function Receive-WsJson {
    param(
        [Phase30SmokeWebSocket] $Socket,
        [int] $TimeoutSeconds = 20
    )
    $text = $Socket.Receive($TimeoutSeconds)
    return $text | ConvertFrom-Json
}

function Read-WsUntil {
    param(
        [Phase30SmokeWebSocket] $Socket,
        [string] $Type,
        [string] $RequestID = '',
        [int] $TimeoutSeconds = 25
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
        [Phase30SmokeWebSocket] $Socket,
        [string] $Name,
        [int] $TimeoutSeconds = 120
    )
    if ($null -eq $Socket) {
        return
    }
    if (-not $Socket.WaitClosed($TimeoutSeconds)) {
        throw "timed out waiting for $Name websocket to close during rollout"
    }
    $reason = [string]$Socket.CloseDescription
    if (-not [string]::IsNullOrWhiteSpace($reason) -and $reason -ne 'server draining') {
        throw "$Name closed with reason '$reason', want server draining"
    }
}

function Close-SmokeWebSocket {
    param([Phase30SmokeWebSocket] $Socket)
    if ($null -eq $Socket) {
        return
    }
    $Socket.Dispose()
}

function Join-SmokeRoom {
    param(
        [Phase30SmokeWebSocket] $Socket,
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
        [Phase30SmokeWebSocket] $HostSocket,
        [Phase30SmokeWebSocket] $ViewerSocket,
        [string] $RoomCode,
        [string] $HostUserID,
        [int64] $InitialSeq,
        [string] $RequestSuffix
    )
    $seq = $InitialSeq
    $controls = @(
        @{ type = 'play'; requestId = "phase30-play-$RequestSuffix"; positionMs = 1000 },
        @{ type = 'seek'; requestId = "phase30-seek-$RequestSuffix"; positionMs = 5000 },
        @{ type = 'pause'; requestId = "phase30-pause-$RequestSuffix"; positionMs = 6000 }
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

function Invoke-PostgresSQL {
    param(
        [string] $Database,
        [string] $Sql,
        [switch] $TupleOnly
    )
    $args = @('exec', '-i', '-n', $Namespace, 'deploy/postgres', '--', 'psql', '-v', 'ON_ERROR_STOP=1')
    if ($TupleOnly) {
        $args += @('-A', '-t')
    }
    $args += @('-U', 'app', '-d', $Database)
    $output = $Sql | & kubectl @args
    if ($LASTEXITCODE -ne 0) {
        throw "psql failed for database $Database"
    }
    return ($output -join "`n").Trim()
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
    param([string] $Name, [int] $Actual, [int] $Expected)
    if ($Actual -ne $Expected) {
        throw "$Name expected $Expected, got $Actual"
    }
}

function Assert-AtLeastInt {
    param([string] $Name, [int] $Actual, [int] $Minimum)
    if ($Actual -lt $Minimum) {
        throw "$Name expected at least $Minimum, got $Actual"
    }
}

$hostSocket = $null
$viewerSocket = $null
$oldHostSocket = $null
$oldViewerSocket = $null
$hostUser = $null
$viewerUser = $null
$roomCode = ''

try {
    Invoke-Step 'check Phase 30 commands' {
        Require-Command docker
        Require-Command kind
        Require-Command kubectl
        & docker info *> $null
        if ($LASTEXITCODE -ne 0) {
            throw 'Docker daemon is not available.'
        }
    }

    if ($ResetCluster) {
        Invoke-Step 'reset kind cluster' {
            Invoke-KindScript -Name 'kind_delete.ps1' -Arguments @($ClusterName)
            Invoke-KindScript -Name 'kind_create.ps1' -Arguments @($ClusterName)
        }
    } else {
        Invoke-Step 'ensure kind cluster exists' {
            $clusters = @(& kind get clusters)
            if ((@($clusters | Where-Object { $_ -eq $ClusterName })).Count -eq 0) {
                Invoke-KindScript -Name 'kind_create.ps1' -Arguments @($ClusterName)
            }
        }
    }

    if (-not $SkipBuild) {
        Invoke-Step 'build local service images' {
            Build-LocalImages
        }
    }

    if (-not $SkipImageLoad) {
        Invoke-Step 'load images into kind' {
            Invoke-KindScript -Name 'kind_load_images.ps1' -Arguments @($ClusterName)
        }
    }

    Invoke-Step 'apply Kustomize overlay and migration configmaps' {
        Prepare-KindManifests
    }

    Invoke-Step 'wait for infrastructure, migrations, seed, and services' {
        Wait-KindBaseline
        Wait-HttpReady -Url "$BaseUrl/readyz" -Name 'kind NodePort gateway'
    }

    $suffix = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    $hostUser = New-SmokeUser -Account "phase30_host_$suffix" -Nickname 'Phase30 Host'
    $viewerUser = New-SmokeUser -Account "phase30_viewer_$suffix" -Nickname 'Phase30 Viewer'

    Invoke-Step 'verify public REST through NodePort apigateway' {
        $media = Invoke-JsonRequest -Method Get -Uri "$BaseUrl/media/items?query=Phase23&limit=5" -Token $hostUser.token
        $ids = @($media.data.items | ForEach-Object { $_.id })
        if ($ids -notcontains $SmokeEpisodeID) {
            throw "smoke media episode $SmokeEpisodeID not found through kind NodePort"
        }

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

        Invoke-JsonRequest -Method Put -Uri "$BaseUrl/me/media-progress/$SmokeEpisodeID" -Token $hostUser.token -Body @{
            lastPositionSeconds = 30
            durationSeconds = 120
            completed = $false
        } | Out-Null
        $homeSummary = Invoke-JsonRequest -Method Get -Uri "$BaseUrl/home/summary" -Token $hostUser.token
        if ($homeSummary.data.lastWatched.mediaItemId -ne $SmokeEpisodeID) {
            throw "home lastWatched mediaItemId=$($homeSummary.data.lastWatched.mediaItemId), want $SmokeEpisodeID"
        }
    }

    Invoke-Step 'verify WebSocket join and controls before rollout' {
        $hostSocket = New-SmokeWebSocket -Token $hostUser.token
        $viewerSocket = New-SmokeWebSocket -Token $viewerUser.token
        $hostState = Join-SmokeRoom -Socket $hostSocket -RoomCode $script:roomCode -UserID $hostUser.userId -DeviceID 'phase30-host-device'
        Join-SmokeRoom -Socket $viewerSocket -RoomCode $script:roomCode -UserID $viewerUser.userId -DeviceID 'phase30-viewer-device' | Out-Null
        Assert-ControlsAccepted -HostSocket $hostSocket -ViewerSocket $viewerSocket -RoomCode $script:roomCode -HostUserID $hostUser.userId -InitialSeq ([int64]$hostState.payload.seq) -RequestSuffix "before-$suffix"
    }

    Invoke-Step 'rollout restart roomserver and wait for drain close' {
        $oldHostSocket = $hostSocket
        $oldViewerSocket = $viewerSocket
        Invoke-Kubectl -Arguments @('-n', $Namespace, 'rollout', 'restart', 'deployment/roomserver')
        Wait-WsClosed -Socket $oldHostSocket -Name 'host kind roomserver'
        Wait-WsClosed -Socket $oldViewerSocket -Name 'viewer kind roomserver'
        Close-SmokeWebSocket -Socket $oldHostSocket
        Close-SmokeWebSocket -Socket $oldViewerSocket
        $hostSocket = $null
        $viewerSocket = $null
        Invoke-Kubectl -Arguments @('-n', $Namespace, 'rollout', 'status', 'deployment/roomserver', '--timeout=240s')
        Wait-HttpReady -Url "$BaseUrl/readyz" -Name 'kind NodePort gateway after roomserver rollout'
    }

    Invoke-Step 'reconnect after rollout and verify recovered room state' {
        $hostSocket = New-SmokeWebSocket -Token $hostUser.token
        $viewerSocket = New-SmokeWebSocket -Token $viewerUser.token
        $recoveredHostState = Join-SmokeRoom -Socket $hostSocket -RoomCode $script:roomCode -UserID $hostUser.userId -DeviceID 'phase30-host-device'
        Join-SmokeRoom -Socket $viewerSocket -RoomCode $script:roomCode -UserID $viewerUser.userId -DeviceID 'phase30-viewer-device' | Out-Null
        if ($recoveredHostState.payload.roomId -ne $script:roomCode) {
            throw "recovered room_state roomId=$($recoveredHostState.payload.roomId), want $script:roomCode"
        }
        Assert-ControlsAccepted -HostSocket $hostSocket -ViewerSocket $viewerSocket -RoomCode $script:roomCode -HostUserID $hostUser.userId -InitialSeq ([int64]$recoveredHostState.payload.seq) -RequestSuffix "after-$suffix"
    }

    Invoke-Step 'verify owner databases and absent main owner tables' {
        $identityCount = Invoke-ScalarInt -Database 'anime_watch_identity_dev' -Sql "SELECT COUNT(*) FROM users WHERE id IN ('$($hostUser.userId)', '$($viewerUser.userId)');"
        Assert-EqualInt -Name 'identity DB phase30 users' -Actual $identityCount -Expected 2

        $roomCount = Invoke-ScalarInt -Database 'anime_watch_room_dev' -Sql "SELECT COUNT(*) FROM rooms WHERE room_code = '$script:roomCode';"
        Assert-EqualInt -Name 'room DB phase30 room' -Actual $roomCount -Expected 1
        $memberCount = Invoke-ScalarInt -Database 'anime_watch_room_dev' -Sql "SELECT COUNT(*) FROM rooms r JOIN room_members m ON m.room_id = r.id WHERE r.room_code = '$script:roomCode' AND m.user_id IN ('$($hostUser.userId)', '$($viewerUser.userId)');"
        Assert-EqualInt -Name 'room DB phase30 members' -Actual $memberCount -Expected 2

        $progressCount = Invoke-ScalarInt -Database 'anime_watch_progress_dev' -Sql "SELECT COUNT(*) FROM user_media_progress WHERE user_id = '$($hostUser.userId)' AND media_episode_id = '$SmokeEpisodeID';"
        Assert-EqualInt -Name 'progress DB phase30 progress' -Actual $progressCount -Expected 1

        $timelineCount = Invoke-ScalarInt -Database 'anime_watch_timeline_dev' -Sql "SELECT COUNT(*) FROM room_timeline_outbox WHERE room_id = '$script:roomCode' AND event_type = 'room.control.accepted';"
        Assert-AtLeastInt -Name 'timeline DB phase30 accepted outbox rows' -Actual $timelineCount -Minimum 6

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

    Write-Host 'Phase 30 kind rolling restart smoke completed.'
} catch {
    Write-KindDiagnostics
    throw
} finally {
    Close-SmokeWebSocket -Socket $hostSocket
    Close-SmokeWebSocket -Socket $viewerSocket
    if ($shouldDeleteCluster) {
        Invoke-KindScript -Name 'kind_delete.ps1' -Arguments @($ClusterName)
    }
}
