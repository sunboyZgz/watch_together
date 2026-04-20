package com.example.watch_together.ui.player

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.media3.common.Player
import com.example.watch_together.config.AppConfig
import com.example.watch_together.sync.RoomSessionController
import com.example.watch_together.sync.RoomSyncCoordinator
import com.example.watch_together.sync.RoomSyncState
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
    val roomSessionController = remember { RoomSessionController() }
    val roomSyncCoordinator = remember(adapter) { RoomSyncCoordinator(adapter) }
    val syncEventHandler = remember(roomSyncCoordinator) { RoomPlayerSyncEventHandler(roomSyncCoordinator) }
    val coroutineScope = rememberCoroutineScope()

    val sampleUrl = remember { AppConfig.sampleHlsUrl() }
    val hostUserId = remember { "android_host_${UUID.randomUUID().toString().take(4)}" }
    val viewerUserId = remember { "android_viewer_${UUID.randomUUID().toString().take(4)}" }

    val playerEventLogs = remember { mutableStateListOf<String>() }
    val syncLogs = remember { mutableStateListOf<String>() }

    var uiState by remember { mutableStateOf(RoomPlayerUiState()) }
    val currentUiState by rememberUpdatedState(uiState)

    val isHostController = remember(uiState.activeUserId, uiState.latestSyncState) {
        uiState.activeUserId != null && uiState.latestSyncState?.hostUserId == uiState.activeUserId
    }
    val latestSyncState = uiState.latestSyncState
    val playbackSnapshot = uiState.player

    fun updateUiState(transform: (RoomPlayerUiState) -> RoomPlayerUiState) {
        uiState = transform(uiState)
    }

    DisposableEffect(adapter) {
        adapter.setEventListener { event ->
            appendLog(playerEventLogs, event.toDebugLabel(), maxSize = 8)
            if (event is PlayerEvent.PlaybackStateChanged) {
                updateUiState { current ->
                    current.copy(
                        player = current.player.copy(playbackState = event.playbackState)
                    )
                }
            }
        }
        onDispose {
            adapter.setEventListener(null)
        }
    }

    DisposableEffect(roomSessionController) {
        onDispose {
            roomSessionController.closeSession()
        }
    }

    LaunchedEffect(adapter) {
        while (isActive) {
            updateUiState { current ->
                current.copy(
                    player = current.player.copy(
                        currentPosition = adapter.getCurrentPosition(),
                        duration = adapter.getDuration().coerceAtLeast(0L),
                        isPlaying = adapter.isPlaying()
                    )
                )
            }
            delay(500)
        }
    }

    LaunchedEffect(adapter, roomSyncCoordinator) {
        while (isActive) {
            delay(RoomSyncCoordinator.DEFAULT_CORRECTION_INTERVAL_MS)

            val authorityState = uiState.latestSyncState ?: continue
            val nowMs = System.currentTimeMillis()
            val driftCheck = roomSyncCoordinator.evaluateDrift(
                authorityState = authorityState,
                nowMs = nowMs,
                lastCorrectionAtMs = uiState.lastDriftCorrectionAtMs,
                durationMs = uiState.player.duration,
                playbackEnded = uiState.player.playbackState == Player.STATE_ENDED
            )

            if (!driftCheck.shouldCorrect) {
                continue
            }

            roomSyncCoordinator.applyDriftCorrection(driftCheck, authorityState)
            updateUiState { current -> current.copy(lastDriftCorrectionAtMs = nowMs) }
            appendLog(
                syncLogs,
                "drift correction local=${driftCheck.localPositionMs} expected=${driftCheck.expectedPositionMs} drift=${driftCheck.driftMs}"
            )
        }
    }

    fun applySyncEventResult(result: RoomPlayerSyncEventResult) {
        uiState = result.uiState
        result.logs.forEach { line ->
            appendLog(syncLogs, line)
        }
    }

    val syncListener = remember(syncEventHandler) {
        object : RoomWebSocketListener {
            override fun onLog(message: String) {
                coroutineScope.launch {
                    appendLog(syncLogs, message)
                }
            }

            override fun onRoomState(payload: RoomSyncState) {
                coroutineScope.launch {
                    applySyncEventResult(syncEventHandler.onRoomState(currentUiState, payload))
                }
            }

            override fun onPlay(payload: com.example.watch_together.sync.protocol.PlayPayload) {
                coroutineScope.launch {
                    applySyncEventResult(syncEventHandler.onPlay(currentUiState, payload))
                }
            }

            override fun onPause(payload: com.example.watch_together.sync.protocol.PausePayload) {
                coroutineScope.launch {
                    applySyncEventResult(syncEventHandler.onPause(currentUiState, payload))
                }
            }

            override fun onSeek(payload: com.example.watch_together.sync.protocol.SeekPayload) {
                coroutineScope.launch {
                    applySyncEventResult(syncEventHandler.onSeek(currentUiState, payload))
                }
            }

            override fun onPlaybackRate(
                payload: com.example.watch_together.sync.protocol.SetPlaybackRatePayload
            ) {
                coroutineScope.launch {
                    applySyncEventResult(syncEventHandler.onPlaybackRate(currentUiState, payload))
                }
            }

            override fun onEnded(payload: com.example.watch_together.sync.protocol.EndedPayload) {
                coroutineScope.launch {
                    applySyncEventResult(syncEventHandler.onEnded(currentUiState, payload))
                }
            }

            override fun onHeartbeat(serverTimeMs: Long) {
                coroutineScope.launch {
                    applySyncEventResult(syncEventHandler.onHeartbeat(currentUiState, serverTimeMs))
                }
            }

            override fun onError(message: String) {
                coroutineScope.launch {
                    applySyncEventResult(syncEventHandler.onError(currentUiState, message))
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
        roomSessionController.closeSession()
        updateUiState { current ->
            current.copy(
                currentRoomId = roomId,
                joinRoomInput = roomId,
                latestSyncState = null,
                syncStatus = status,
                lastDriftCorrectionAtMs = 0L
            )
        }
        appendLog(syncLogs, "$reason starting roomId=$roomId userId=$userId")

        roomSessionController.startSession(
            roomId = roomId,
            userId = userId,
            listener = syncListener
        )
    }

    fun createAndJoinAsHost() {
        coroutineScope.launch {
            runCatching {
                updateUiState { current ->
                    current.copy(syncStatus = SyncStatus.CreatingRoom)
                }
                appendLog(syncLogs, "POST /rooms for hostUserId=$hostUserId")
                val createResult = withContext(Dispatchers.IO) {
                    roomSessionController.createRoom(
                        userId = hostUserId,
                        mediaId = AppConfig.defaultMediaIdForRoom()
                    )
                }

                updateUiState { current ->
                    current.copy(
                        currentRoomId = createResult.roomId,
                        joinRoomInput = createResult.roomId,
                        activeUserId = hostUserId
                    )
                }
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
                updateUiState { current ->
                    current.copy(syncStatus = SyncStatus.CreateAndJoinFailed)
                }
                appendLog(syncLogs, "Create and join failed: ${error.message}")
            }
        }
    }

    fun joinAsViewer() {
        val roomId = uiState.joinRoomInput.trim()
        if (roomId.isBlank()) {
            appendLog(syncLogs, "Join aborted: roomId is empty")
            return
        }

        updateUiState { current -> current.copy(activeUserId = viewerUserId) }
        joinCurrentRoomAsUser(
            roomId = roomId,
            userId = viewerUserId,
            status = SyncStatus.JoiningAsViewer,
            reason = "viewer join"
        )
    }

    fun rejoinCurrentUser() {
        val userId = uiState.activeUserId ?: run {
            appendLog(syncLogs, "Rejoin aborted: no active user")
            return
        }
        val candidateRoomId =
            uiState.currentRoomId ?: uiState.joinRoomInput.trim().takeIf { it.isNotBlank() }
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
        val currentState = uiState.latestSyncState ?: return
        val sent = roomSessionController.sendPlay(
            positionMs = uiState.player.currentPosition,
            seq = currentState.seq
        )
        appendLog(syncLogs, "play sent=$sent at ${uiState.player.currentPosition}ms")
    }

    fun sendPause() {
        val currentState = uiState.latestSyncState ?: return
        val sent = roomSessionController.sendPause(
            positionMs = uiState.player.currentPosition,
            seq = currentState.seq
        )
        appendLog(syncLogs, "pause sent=$sent at ${uiState.player.currentPosition}ms")
    }

    fun sendSeek(targetPositionMs: Long) {
        val currentState = uiState.latestSyncState ?: return
        val sent = roomSessionController.sendSeek(
            positionMs = targetPositionMs,
            seq = currentState.seq
        )
        appendLog(syncLogs, "seek sent=$sent to ${targetPositionMs}ms")
    }

    fun sendPlaybackRateSync(speed: Float) {
        val currentState = uiState.latestSyncState ?: return
        val previousPlaybackSpeed = uiState.player.playbackSpeed
        updateUiState { current ->
            current.copy(
                latestSyncState = currentState.copy(
                    positionMs = current.player.currentPosition,
                    playbackRate = speed.toDouble(),
                    authorityAppliedAtMs = System.currentTimeMillis()
                ),
                player = current.player.copy(playbackSpeed = speed)
            )
        }
        adapter.setPlaybackSpeed(speed)
        val sent = roomSessionController.sendPlaybackRate(
            playbackRate = speed.toDouble(),
            positionMs = uiState.player.currentPosition,
            seq = currentState.seq
        )
        if (!sent) {
            updateUiState { current ->
                current.copy(
                    latestSyncState = currentState,
                    player = current.player.copy(playbackSpeed = previousPlaybackSpeed)
                )
            }
            adapter.setPlaybackSpeed(previousPlaybackSpeed)
        }
        appendLog(syncLogs, "playbackRate sent=$sent rate=${speed}x at ${uiState.player.currentPosition}ms")
    }

    fun sendEndedSync() {
        val currentState = uiState.latestSyncState ?: return
        val sent = roomSessionController.sendEnded(
            positionMs = uiState.player.currentPosition,
            seq = currentState.seq
        )
        appendLog(syncLogs, "ended sent=$sent at ${uiState.player.currentPosition}ms")
        if (sent) {
            updateUiState { current -> current.copy(lastEndedReportedSeq = currentState.seq) }
        }
    }

    LaunchedEffect(
        uiState.player.playbackState,
        isHostController,
        uiState.latestSyncState?.seq,
        uiState.latestSyncState?.ended
    ) {
        val currentState = uiState.latestSyncState ?: return@LaunchedEffect
        if (!isHostController) return@LaunchedEffect
        if (uiState.player.playbackState != Player.STATE_ENDED) return@LaunchedEffect
        if (currentState.ended) return@LaunchedEffect
        if (uiState.lastEndedReportedSeq == currentState.seq) return@LaunchedEffect
        sendEndedSync()
    }

    RoomPlayerPageShell(
        modifier = modifier,
        sampleUrl = sampleUrl,
        hostUserId = hostUserId,
        viewerUserId = viewerUserId,
        uiState = uiState,
        adapter = adapter,
        isHostController = isHostController,
        onJoinRoomInputChange = { value ->
            updateUiState { current -> current.copy(joinRoomInput = value) }
        },
        onCreateAndJoinAsHost = ::createAndJoinAsHost,
        onJoinAsViewer = ::joinAsViewer,
        onRejoinCurrentUser = ::rejoinCurrentUser,
        onPlaySync = ::sendPlay,
        onPauseSync = ::sendPause,
        onSeekSync = ::sendSeek,
        onPlaybackSpeedChange = { speed ->
            if (isHostController && uiState.latestSyncState != null) {
                appendLog(syncLogs, "playbackRate click path=sync rate=${speed}x")
                sendPlaybackRateSync(speed)
            } else {
                appendLog(
                    syncLogs,
                    "playbackRate click path=local rate=${speed}x host=$isHostController joined=${uiState.latestSyncState != null}"
                )
                updateUiState { current ->
                    current.copy(player = current.player.copy(playbackSpeed = speed))
                }
                adapter.setPlaybackSpeed(speed)
            }
        }
    )
    RoomPlayerDebugShell(
        latestSyncState = uiState.latestSyncState,
        syncLogs = syncLogs,
        playerEventLogs = playerEventLogs
    )
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
