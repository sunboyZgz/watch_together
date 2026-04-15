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
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
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
import androidx.media3.ui.PlayerView
import com.example.watch_together.config.AppConfig
import com.example.watch_together.sync.RoomHttpClient
import com.example.watch_together.sync.RoomSyncCoordinator
import com.example.watch_together.sync.RoomSyncState
import com.example.watch_together.sync.RoomWebSocketClient
import com.example.watch_together.sync.RoomWebSocketListener
import com.example.watch_together.ui.theme.Watch_togetherTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.UUID

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

    var currentRoomId by remember { mutableStateOf<String?>(null) }
    var latestSyncState by remember { mutableStateOf<RoomSyncState?>(null) }
    var syncStatus by remember { mutableStateOf("Idle") }
    var currentPosition by remember { mutableLongStateOf(0L) }
    var duration by remember { mutableLongStateOf(0L) }
    var isPlaying by remember { mutableStateOf(false) }
    var playbackSpeed by remember { mutableFloatStateOf(1f) }

    DisposableEffect(adapter) {
        adapter.setEventListener { event ->
            appendLog(
                logs = playerEventLogs,
                line = event.toDebugLabel(),
                maxSize = 8
            )
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

    val syncListener = remember(roomSyncCoordinator, coroutineScope) {
        object : RoomWebSocketListener {
            override fun onLog(message: String) {
                coroutineScope.launch {
                    appendLog(syncLogs, message)
                }
            }

            override fun onRoomState(payload: RoomSyncState) {
                coroutineScope.launch {
                    latestSyncState = roomSyncCoordinator.applyInitialState(payload)
                    currentRoomId = payload.roomId
                    playbackSpeed = payload.playbackRate.toFloat()
                    syncStatus = "room_state applied"
                    appendLog(
                        logs = syncLogs,
                        line = "Applied room_state seq=${payload.seq} roomId=${payload.roomId}"
                    )
                }
            }

            override fun onError(message: String) {
                coroutineScope.launch {
                    syncStatus = "Sync failed"
                    appendLog(syncLogs, "Sync error: $message")
                }
            }
        }
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
            syncStatus = syncStatus
        )
        JoinSyncActionsCard(
            hostUserId = hostUserId,
            viewerUserId = viewerUserId,
            currentRoomId = currentRoomId,
            syncStatus = syncStatus,
            onCreateAndJoin = {
                coroutineScope.launch {
                    runCatching {
                        syncStatus = "Creating room"
                        appendLog(syncLogs, "POST /rooms for hostUserId=$hostUserId")
                        val createResult = withContext(Dispatchers.IO) {
                            roomHttpClient.createRoom(
                                userId = hostUserId,
                                mediaId = AppConfig.defaultMediaIdForRoom()
                            )
                        }

                        currentRoomId = createResult.roomId
                        syncStatus = "Joining room"
                        appendLog(
                            syncLogs,
                            "Created roomId=${createResult.roomId} mediaId=${createResult.roomState.mediaId}"
                        )

                        roomWebSocketClient.joinRoom(
                            wsUrl = AppConfig.wsBaseUrl,
                            roomId = createResult.roomId,
                            userId = viewerUserId,
                            listener = syncListener
                        )
                    }.onFailure { error ->
                        syncStatus = "Create and join failed"
                        appendLog(syncLogs, "Create and join failed: ${error.message}")
                    }
                }
            },
            onRejoin = {
                val roomId = currentRoomId ?: return@JoinSyncActionsCard
                syncStatus = "Rejoining room"
                appendLog(syncLogs, "Rejoining roomId=$roomId as $viewerUserId")
                roomWebSocketClient.joinRoom(
                    wsUrl = AppConfig.wsBaseUrl,
                    roomId = roomId,
                    userId = viewerUserId,
                    listener = syncListener
                )
            }
        )
        PlayerViewport(adapter = adapter)
        PlayerControls(
            adapter = adapter,
            sampleUrl = sampleUrl,
            currentPosition = currentPosition,
            duration = duration,
            isPlaying = isPlaying,
            playbackSpeed = playbackSpeed,
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
    syncStatus: String
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
                text = "Android initial state sync on join validation",
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
                text = "Current room: ${currentRoomId ?: "(not created yet)"}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Sync status: $syncStatus",
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
    syncStatus: String,
    onCreateAndJoin: () -> Unit,
    onRejoin: () -> Unit
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Text(
                text = "Join-time sync actions",
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
                text = "Current room: ${currentRoomId ?: "(none)"} · $syncStatus",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = onCreateAndJoin) {
                    Text("Create + Join")
                }
                OutlinedButton(
                    onClick = onRejoin,
                    enabled = currentRoomId != null
                ) {
                    Text("Rejoin current room")
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
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = { adapter.load(sampleUrl) }) {
                        Text("Load sample")
                    }
                    Button(onClick = { adapter.play() }) {
                        Text("Play")
                    }
                    OutlinedButton(onClick = { adapter.pause() }) {
                        Text("Pause")
                    }
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = {
                        adapter.seekTo((currentPosition - 10_000L).coerceAtLeast(0L))
                    }) {
                        Text("-10s")
                    }
                    OutlinedButton(onClick = {
                        val safeDuration = if (duration > 0L) duration else currentPosition + 10_000L
                        adapter.seekTo((currentPosition + 10_000L).coerceAtMost(safeDuration))
                    }) {
                        Text("+10s")
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
                text = "Latest room_state",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            if (latestSyncState == null) {
                Text(
                    text = "No room_state applied yet.",
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
