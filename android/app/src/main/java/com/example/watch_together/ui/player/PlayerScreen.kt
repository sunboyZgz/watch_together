package com.example.watch_together.ui.player

import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.Player
import androidx.media3.ui.PlayerView
import com.example.watch_together.config.AppConfig
import com.example.watch_together.sync.RoomHttpClient
import com.example.watch_together.sync.RoomSyncCoordinator
import com.example.watch_together.sync.RoomSyncState
import com.example.watch_together.sync.RoomWebSocketClient
import com.example.watch_together.sync.RoomWebSocketListener
import com.example.watch_together.sync.SyncMessage
import com.example.watch_together.sync.isNewerThan
import com.example.watch_together.ui.theme.Watch_togetherTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.UUID

private enum class SyncStatus(val label: String) {
    Idle("Idle"),
    CreatingRoom("Creating room"),
    JoiningAsHost("Joining as host"),
    JoiningAsViewer("Joining as viewer"),
    RejoiningCurrentUser("Rejoining current user"),
    Connected("Connected"),
    RoomStateApplied("room_state applied"),
    PlayApplied("play applied"),
    PauseApplied("pause applied"),
    SeekApplied("seek applied"),
    SyncFailed("Sync failed"),
    CreateAndJoinFailed("Create and join failed")
}

@Composable
fun PlayerScreen(modifier: Modifier = Modifier) {
    val adapter = rememberPlayerAdapter()
    val roomHttpClient = remember { RoomHttpClient() }
    val roomWebSocketClient = remember { RoomWebSocketClient() }
    val roomSyncCoordinator = remember(adapter) { RoomSyncCoordinator(adapter) }
    val coroutineScope = rememberCoroutineScope()

    val sampleUrl = remember { AppConfig.sampleHlsUrl() }
    val hostUserId = remember { "android_host_${UUID.randomUUID().toString().take(4)}" }
    val viewerUserId = remember { "android_viewer_${UUID.randomUUID().toString().take(4)}" }

    val playerEventLogs = remember { mutableStateListOf<String>() }
    val syncLogs = remember { mutableStateListOf<String>() }

    var joinRoomInput by remember { mutableStateOf("") }
    var activeUserId by remember { mutableStateOf<String?>(null) }
    var currentRoomId by remember { mutableStateOf<String?>(null) }
    var latestSyncState by remember { mutableStateOf<RoomSyncState?>(null) }
    var syncStatus by remember { mutableStateOf(SyncStatus.Idle) }
    var currentPosition by remember { mutableLongStateOf(0L) }
    var duration by remember { mutableLongStateOf(0L) }
    var isPlaying by remember { mutableStateOf(false) }
    var playbackState by remember { mutableIntStateOf(Player.STATE_IDLE) }
    var playbackSpeed by remember { mutableFloatStateOf(1f) }
    var lastDriftCorrectionAtMs by remember { mutableLongStateOf(0L) }

    val isHostController = remember(activeUserId, latestSyncState) {
        activeUserId != null && latestSyncState?.hostUserId == activeUserId
    }

    DisposableEffect(adapter) {
        adapter.setEventListener { event ->
            appendLog(playerEventLogs, event.toDebugLabel(), maxSize = 8)
            if (event is PlayerEvent.PlaybackStateChanged) {
                playbackState = event.playbackState
            }
        }
        onDispose {
            adapter.setEventListener(null)
        }
    }

    DisposableEffect(roomWebSocketClient) {
        onDispose {
            roomWebSocketClient.close()
        }
    }

    LaunchedEffect(adapter) {
        while (isActive) {
            currentPosition = adapter.getCurrentPosition()
            duration = adapter.getDuration().coerceAtLeast(0L)
            isPlaying = adapter.isPlaying()
            delay(500)
        }
    }

    LaunchedEffect(adapter, roomSyncCoordinator) {
        while (isActive) {
            delay(RoomSyncCoordinator.DEFAULT_CORRECTION_INTERVAL_MS)

            val authorityState = latestSyncState ?: continue
            val nowMs = System.currentTimeMillis()
            val driftCheck = roomSyncCoordinator.evaluateDrift(
                authorityState = authorityState,
                nowMs = nowMs,
                lastCorrectionAtMs = lastDriftCorrectionAtMs,
                durationMs = duration,
                playbackEnded = playbackState == Player.STATE_ENDED
            )

            if (!driftCheck.shouldCorrect) {
                continue
            }

            roomSyncCoordinator.applyDriftCorrection(driftCheck, authorityState)
            lastDriftCorrectionAtMs = nowMs
            appendLog(
                syncLogs,
                "drift correction local=${driftCheck.localPositionMs} expected=${driftCheck.expectedPositionMs} drift=${driftCheck.driftMs}"
            )
        }
    }

    fun applyAuthoritativeState(
        newState: RoomSyncState,
        status: SyncStatus,
        reason: String
    ) {
        if (!newState.isNewerThan(latestSyncState)) {
            appendLog(syncLogs, "Ignored stale $reason seq=${newState.seq}")
            return
        }

        latestSyncState = newState
        currentRoomId = newState.roomId
        joinRoomInput = newState.roomId
        playbackSpeed = newState.playbackRate.toFloat()
        syncStatus = status
        appendLog(syncLogs, "Applied $reason seq=${newState.seq} roomId=${newState.roomId}")
    }

    val syncListener = remember(roomSyncCoordinator, latestSyncState) {
        object : RoomWebSocketListener {
            override fun onLog(message: String) {
                coroutineScope.launch {
                    appendLog(syncLogs, message)
                }
            }

            override fun onRoomState(payload: RoomSyncState) {
                coroutineScope.launch {
                    val appliedState = roomSyncCoordinator.applyInitialState(payload)
                    applyAuthoritativeState(appliedState, SyncStatus.RoomStateApplied, "room_state")
                }
            }

            override fun onPlay(payload: com.example.watch_together.sync.protocol.PlayPayload) {
                coroutineScope.launch {
                    val previous = latestSyncState ?: run {
                        appendLog(syncLogs, "Ignored play before room_state")
                        return@launch
                    }
                    if (payload.seq <= previous.seq) {
                        appendLog(syncLogs, "Ignored stale play seq=${payload.seq}")
                        return@launch
                    }
                    val appliedState = roomSyncCoordinator.applyPlayEvent(previous, payload)
                    applyAuthoritativeState(appliedState, SyncStatus.PlayApplied, "play")
                }
            }

            override fun onPause(payload: com.example.watch_together.sync.protocol.PausePayload) {
                coroutineScope.launch {
                    val previous = latestSyncState ?: run {
                        appendLog(syncLogs, "Ignored pause before room_state")
                        return@launch
                    }
                    if (payload.seq <= previous.seq) {
                        appendLog(syncLogs, "Ignored stale pause seq=${payload.seq}")
                        return@launch
                    }
                    val appliedState = roomSyncCoordinator.applyPauseEvent(previous, payload)
                    applyAuthoritativeState(appliedState, SyncStatus.PauseApplied, "pause")
                }
            }

            override fun onSeek(payload: com.example.watch_together.sync.protocol.SeekPayload) {
                coroutineScope.launch {
                    val previous = latestSyncState ?: run {
                        appendLog(syncLogs, "Ignored seek before room_state")
                        return@launch
                    }
                    if (payload.seq <= previous.seq) {
                        appendLog(syncLogs, "Ignored stale seek seq=${payload.seq}")
                        return@launch
                    }
                    val appliedState = roomSyncCoordinator.applySeekEvent(previous, payload)
                    applyAuthoritativeState(appliedState, SyncStatus.SeekApplied, "seek")
                }
            }

            override fun onHeartbeat(serverTimeMs: Long) {
                coroutineScope.launch {
                    syncStatus = SyncStatus.Connected
                    appendLog(syncLogs, "heartbeat acknowledged serverTimeMs=$serverTimeMs")
                }
            }

            override fun onError(message: String) {
                coroutineScope.launch {
                    syncStatus = SyncStatus.SyncFailed
                    appendLog(syncLogs, "Sync error: $message")
                }
            }
        }
    }

    fun joinCurrentRoomAsUser(
        roomId: String,
        userId: String,
        status: SyncStatus,
        reason: String
    ) {
        roomWebSocketClient.close()
        latestSyncState = null
        lastDriftCorrectionAtMs = 0L
        currentRoomId = roomId
        joinRoomInput = roomId
        syncStatus = status
        appendLog(syncLogs, "$reason starting roomId=$roomId userId=$userId")

        roomWebSocketClient.joinRoom(
            wsUrl = AppConfig.wsBaseUrl,
            roomId = roomId,
            userId = userId,
            listener = syncListener
        )
    }

    fun createAndJoinAsHost() {
        coroutineScope.launch {
            runCatching {
                syncStatus = SyncStatus.CreatingRoom
                appendLog(syncLogs, "POST /rooms for hostUserId=$hostUserId")
                val createResult = withContext(Dispatchers.IO) {
                    roomHttpClient.createRoom(
                        userId = hostUserId,
                        mediaId = AppConfig.defaultMediaIdForRoom()
                    )
                }

                currentRoomId = createResult.roomId
                joinRoomInput = createResult.roomId
                activeUserId = hostUserId
                appendLog(
                    syncLogs,
                    "Created roomId=${createResult.roomId} mediaId=${createResult.roomState.mediaId}"
                )
                joinCurrentRoomAsUser(
                    roomId = createResult.roomId,
                    userId = hostUserId,
                    status = SyncStatus.JoiningAsHost,
                    reason = "host join"
                )
            }.onFailure { error: Throwable ->
                syncStatus = SyncStatus.CreateAndJoinFailed
                appendLog(syncLogs, "Create and join failed: ${error.message}")
            }
        }
    }

    fun joinAsViewer() {
        val roomId = joinRoomInput.trim()
        if (roomId.isBlank()) {
            appendLog(syncLogs, "Join aborted: roomId is empty")
            return
        }

        activeUserId = viewerUserId
        joinCurrentRoomAsUser(
            roomId = roomId,
            userId = viewerUserId,
            status = SyncStatus.JoiningAsViewer,
            reason = "viewer join"
        )
    }

    fun rejoinCurrentUser() {
        val userId = activeUserId ?: run {
            appendLog(syncLogs, "Rejoin aborted: no active user")
            return
        }
        val candidateRoomId = currentRoomId ?: joinRoomInput.trim().takeIf { it.isNotBlank() }
        if (candidateRoomId == null) {
            appendLog(syncLogs, "Rejoin aborted: no roomId")
            return
        }

        joinCurrentRoomAsUser(
            roomId = candidateRoomId,
            userId = userId,
            status = SyncStatus.RejoiningCurrentUser,
            reason = "repeated join"
        )
    }

    fun sendPlay() {
        val currentState = latestSyncState ?: return
        val sent = roomWebSocketClient.sendPlay(
            positionMs = currentPosition,
            seq = currentState.seq
        )
        appendLog(syncLogs, "play sent=$sent at ${currentPosition}ms")
    }

    fun sendPause() {
        val currentState = latestSyncState ?: return
        val sent = roomWebSocketClient.sendPause(
            positionMs = currentPosition,
            seq = currentState.seq
        )
        appendLog(syncLogs, "pause sent=$sent at ${currentPosition}ms")
    }

    fun sendSeek(targetPositionMs: Long) {
        val currentState = latestSyncState ?: return
        val sent = roomWebSocketClient.sendSeek(
            positionMs = targetPositionMs,
            seq = currentState.seq
        )
        appendLog(syncLogs, "seek sent=$sent to ${targetPositionMs}ms")
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        PlayerStatusHeader(
            sampleUrl = sampleUrl,
            currentRoomId = currentRoomId,
            syncStatus = syncStatus,
            activeUserId = activeUserId,
            isHostController = isHostController
        )
        JoinSyncActionsCard(
            hostUserId = hostUserId,
            viewerUserId = viewerUserId,
            currentRoomId = currentRoomId,
            syncStatus = syncStatus,
            joinRoomInput = joinRoomInput,
            onJoinRoomInputChange = { joinRoomInput = it },
            onCreateAndJoinAsHost = ::createAndJoinAsHost,
            onJoinAsViewer = ::joinAsViewer,
            onRejoinCurrentUser = ::rejoinCurrentUser,
            canRejoinCurrentUser = activeUserId != null && currentRoomId != null
        )
        PlayerViewport(adapter = adapter)
        PlayerControls(
            adapter = adapter,
            sampleUrl = sampleUrl,
            currentPosition = currentPosition,
            duration = duration,
            isPlaying = isPlaying,
            playbackSpeed = playbackSpeed,
            isHostController = isHostController,
            isJoinedToRoom = latestSyncState != null,
            onPlaySync = ::sendPlay,
            onPauseSync = ::sendPause,
            onSeekSync = ::sendSeek,
            onPlaybackSpeedChange = { speed ->
                playbackSpeed = speed
                adapter.setPlaybackSpeed(speed)
            }
        )
        SyncStatePanel(latestSyncState = latestSyncState)
        SyncDebugPanel(syncLogs = syncLogs)
        PlayerEventDebugPanel(eventLogs = playerEventLogs)
        ConfigInjectionHint()
    }
}

@Composable
private fun rememberPlayerAdapter(): PlayerAdapter {
    val context = androidx.compose.ui.platform.LocalContext.current
    val adapter = remember(context) { AndroidExoPlayerAdapter(context) }

    DisposableEffect(adapter) {
        onDispose {
            adapter.release()
        }
    }

    return adapter
}

@Composable
private fun PlayerStatusHeader(
    sampleUrl: String,
    currentRoomId: String?,
    syncStatus: SyncStatus,
    activeUserId: String?,
    isHostController: Boolean
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp)) {
            Text(
                text = "watch_together",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold
            )
            Spacer(modifier = Modifier.height(4.dp))
            Text(
                text = "Android control sync validation",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(modifier = Modifier.height(8.dp))
            Text(
                text = "Sample HLS URL: $sampleUrl",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Current room: ${currentRoomId ?: "(not joined yet)"}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Active user: ${activeUserId ?: "(none)"}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Control mode: ${if (isHostController) "host sync" else "local / viewer"}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Sync status: ${syncStatus.label}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

@Composable
private fun JoinSyncActionsCard(
    hostUserId: String,
    viewerUserId: String,
    currentRoomId: String?,
    syncStatus: SyncStatus,
    joinRoomInput: String,
    onJoinRoomInputChange: (String) -> Unit,
    onCreateAndJoinAsHost: () -> Unit,
    onJoinAsViewer: () -> Unit,
    onRejoinCurrentUser: () -> Unit,
    canRejoinCurrentUser: Boolean
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Text(
                text = "Room sync actions",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            Text(
                text = "Host user: $hostUserId",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Viewer user: $viewerUserId",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Current room: ${currentRoomId ?: "(none)"} · ${syncStatus.label}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            OutlinedTextField(
                value = joinRoomInput,
                onValueChange = onJoinRoomInputChange,
                modifier = Modifier.fillMaxWidth(),
                label = { Text("Room ID") },
                singleLine = true
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = onCreateAndJoinAsHost) {
                    Text("Create + Join as host")
                }
                OutlinedButton(
                    onClick = onJoinAsViewer,
                    enabled = joinRoomInput.isNotBlank()
                ) {
                    Text("Join as viewer")
                }
                OutlinedButton(
                    onClick = onRejoinCurrentUser,
                    enabled = canRejoinCurrentUser
                ) {
                    Text("Rejoin current user")
                }
            }
        }
    }
}

@Composable
private fun PlayerViewport(adapter: PlayerAdapter) {
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .aspectRatio(16f / 9f),
        tonalElevation = 4.dp,
        shape = MaterialTheme.shapes.large
    ) {
        AndroidView(
            factory = { context ->
                PlayerView(context).apply {
                    layoutParams = android.view.ViewGroup.LayoutParams(MATCH_PARENT, MATCH_PARENT)
                    useController = true
                    adapter.attach(this)
                }
            },
            modifier = Modifier
                .fillMaxSize()
                .background(
                    brush = Brush.linearGradient(
                        listOf(
                            MaterialTheme.colorScheme.primary.copy(alpha = 0.18f),
                            MaterialTheme.colorScheme.secondary.copy(alpha = 0.12f),
                            Color.Black.copy(alpha = 0.9f)
                        )
                    )
                ),
            update = { playerView ->
                adapter.attach(playerView)
            }
        )
    }
}

@Composable
private fun PlayerControls(
    adapter: PlayerAdapter,
    sampleUrl: String,
    currentPosition: Long,
    duration: Long,
    isPlaying: Boolean,
    playbackSpeed: Float,
    isHostController: Boolean,
    isJoinedToRoom: Boolean,
    onPlaySync: () -> Unit,
    onPauseSync: () -> Unit,
    onSeekSync: (Long) -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Text(
                text = "Player controls",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            Text(
                text = "Current position: ${formatMs(currentPosition)} / ${formatMs(duration)}",
                style = MaterialTheme.typography.bodyMedium
            )
            Text(
                text = "Playing: $isPlaying · Speed: ${playbackSpeed}x",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = when {
                    isHostController -> "Buttons will send sync events to the server."
                    isJoinedToRoom -> "Viewer mode: incoming sync is applied, local control stays local."
                    else -> "Local-only mode until you join a room."
                },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = { adapter.load(sampleUrl) }) {
                        Text("Load sample")
                    }
                    Button(onClick = {
                        if (isHostController) {
                            onPlaySync()
                        } else {
                            adapter.play()
                        }
                    }) {
                        Text(if (isHostController) "Play sync" else "Play")
                    }
                    OutlinedButton(onClick = {
                        if (isHostController) {
                            onPauseSync()
                        } else {
                            adapter.pause()
                        }
                    }) {
                        Text(if (isHostController) "Pause sync" else "Pause")
                    }
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = {
                        val target = (currentPosition - 10_000L).coerceAtLeast(0L)
                        if (isHostController) {
                            onSeekSync(target)
                        } else {
                            adapter.seekTo(target)
                        }
                    }) {
                        Text(if (isHostController) "-10s sync" else "-10s")
                    }
                    OutlinedButton(onClick = {
                        val safeDuration = if (duration > 0L) duration else currentPosition + 10_000L
                        val target = (currentPosition + 10_000L).coerceAtMost(safeDuration)
                        if (isHostController) {
                            onSeekSync(target)
                        } else {
                            adapter.seekTo(target)
                        }
                    }) {
                        Text(if (isHostController) "+10s sync" else "+10s")
                    }
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                listOf(0.75f, 1.0f, 1.25f, 1.5f, 2.0f).forEach { speed ->
                    val selected = speed == playbackSpeed
                    val buttonText = if (selected) "${speed}x ✓" else "${speed}x"
                    val onClick = { onPlaybackSpeedChange(speed) }
                    if (selected) {
                        Button(onClick = onClick) {
                            Text(buttonText)
                        }
                    } else {
                        OutlinedButton(onClick = onClick) {
                            Text(buttonText)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SyncStatePanel(latestSyncState: RoomSyncState?) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Text(
                text = "Latest sync state",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            if (latestSyncState == null) {
                Text(
                    text = "No synced state applied yet.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            } else {
                Text("roomId=${latestSyncState.roomId}", style = MaterialTheme.typography.bodySmall)
                Text("mediaId=${latestSyncState.mediaId}", style = MaterialTheme.typography.bodySmall)
                Text("hostUserId=${latestSyncState.hostUserId}", style = MaterialTheme.typography.bodySmall)
                Text("paused=${latestSyncState.paused}", style = MaterialTheme.typography.bodySmall)
                Text("positionMs=${latestSyncState.positionMs}", style = MaterialTheme.typography.bodySmall)
                Text(
                    "playbackRate=${latestSyncState.playbackRate} · seq=${latestSyncState.seq}",
                    style = MaterialTheme.typography.bodySmall
                )
            }
        }
    }
}

@Composable
private fun SyncDebugPanel(syncLogs: List<String>) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = "Sync log",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            if (syncLogs.isEmpty()) {
                Text(
                    text = "No sync events yet.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            } else {
                syncLogs.forEach { line ->
                    Text(
                        text = line,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
        }
    }
}

@Composable
private fun ConfigInjectionHint() {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Text(
                text = "Config injection entry",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            Text(text = "APP_ENV=${AppConfig.appEnv}", style = MaterialTheme.typography.bodyMedium)
            Text(
                text = "API_BASE_URL=${AppConfig.apiBaseUrl}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "WS_BASE_URL=${AppConfig.wsBaseUrl}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "MEDIA_BASE_URL=${AppConfig.mediaBaseUrl}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "MEDIA_DEFAULT_ID=${AppConfig.mediaDefaultId.ifBlank { "(empty)" }}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "DEBUG_SYNC=${AppConfig.debugSync}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

@Composable
private fun PlayerEventDebugPanel(eventLogs: List<String>) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = "Player event log",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            if (eventLogs.isEmpty()) {
                Text(
                    text = "No events emitted yet.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            } else {
                eventLogs.forEach { line ->
                    Text(
                        text = line,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
        }
    }
}

@Preview(showBackground = true, showSystemUi = true)
@Composable
private fun PlayerScreenPreview() {
    Watch_togetherTheme {
        PlayerScreen()
    }
}

private fun appendLog(
    logs: MutableList<String>,
    line: String,
    maxSize: Int = 10
) {
    logs.add(0, line)
    while (logs.size > maxSize) {
        logs.removeAt(logs.lastIndex)
    }
}

private fun formatMs(value: Long): String {
    if (value <= 0L) return "00:00"
    val totalSeconds = value / 1000
    val minutes = totalSeconds / 60
    val seconds = totalSeconds % 60
    return "%02d:%02d".format(minutes, seconds)
}
