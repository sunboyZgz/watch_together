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
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.media3.common.Player
import com.example.watch_together.config.AppConfig
import com.example.watch_together.pages.room.RoomTheaterPage
import com.example.watch_together.sync.ProgressHttpClient
import com.example.watch_together.sync.RoomMedia
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
fun PlayerScreen(
    accessToken: String = "",
    currentUserId: String = "",
    selectedEpisodeId: String = AppConfig.defaultMediaIdForRoom(),
    autoCreateAsHost: Boolean = false,
    modifier: Modifier = Modifier
) {
    val adapter = rememberPlayerAdapter()
    val roomSessionController = remember { RoomSessionController() }
    val progressHttpClient = remember { ProgressHttpClient() }
    var currentRoomMedia by remember { mutableStateOf<RoomMedia?>(null) }
    val mediaUrlResolver = remember(currentRoomMedia) {
        { mediaId: String ->
            currentRoomMedia
                ?.takeIf { it.id == mediaId }
                ?.mediaUrl
                ?.let(AppConfig::playableMediaUrl)
                ?: AppConfig.mediaUrlFor(mediaId)
        }
    }
    val roomSyncCoordinator = remember(adapter, mediaUrlResolver) {
        RoomSyncCoordinator(adapter, mediaUrlResolver)
    }
    val syncEventHandler = remember(roomSyncCoordinator) { RoomPlayerSyncEventHandler(roomSyncCoordinator) }
    val coroutineScope = rememberCoroutineScope()

    val hostUserId = remember(currentUserId) {
        currentUserId.ifBlank { "android_host_${UUID.randomUUID().toString().take(4)}" }
    }
    val viewerUserId = remember { "android_viewer_${UUID.randomUUID().toString().take(4)}" }

    val playerEventLogs = remember { mutableStateListOf<String>() }
    val syncLogs = remember { mutableStateListOf<String>() }

    var uiState by remember { mutableStateOf(RoomPlayerUiState()) }
    var loadedRoomId by remember { mutableStateOf<String?>(null) }
    var autoCreatedEpisodeId by rememberSaveable { mutableStateOf<String?>(null) }
    var loadedRoomDetailCode by rememberSaveable { mutableStateOf<String?>(null) }
    var lastProgressReportAtMs by rememberSaveable { mutableStateOf(0L) }
    var lastProgressReportPositionSeconds by rememberSaveable { mutableStateOf(-1L) }
    val currentUiState by rememberUpdatedState(uiState)

    val isHostController = remember(uiState.activeUserId, uiState.latestSyncState) {
        uiState.activeUserId != null && uiState.latestSyncState?.hostUserId == uiState.activeUserId
    }
    val latestSyncState = uiState.latestSyncState
    val playbackSnapshot = uiState.player

    fun updateUiState(transform: (RoomPlayerUiState) -> RoomPlayerUiState) {
        uiState = transform(uiState)
    }

    suspend fun loadRoomDetail(roomCode: String): Boolean {
        if (loadedRoomDetailCode == roomCode && currentRoomMedia != null) return true
        return runCatching {
            withContext(Dispatchers.IO) {
                roomSessionController.getRoomDetail(roomCode)
            }
        }.onSuccess { detail ->
            loadedRoomDetailCode = roomCode
            currentRoomMedia = detail.media
            appendLog(
                syncLogs,
                "room detail loaded roomCode=${detail.roomCode} media=${detail.media.title}"
            )
        }.onFailure { error ->
            appendLog(syncLogs, "room detail failed: ${error.message}")
        }.isSuccess
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

    LaunchedEffect(uiState.currentRoomId, currentRoomMedia?.mediaUrl) {
        val roomId = uiState.currentRoomId ?: return@LaunchedEffect
        val mediaUrl = currentRoomMedia?.mediaUrl?.let(AppConfig::playableMediaUrl)
            ?: return@LaunchedEffect
        if (loadedRoomId == roomId) return@LaunchedEffect

        adapter.load(mediaUrl)
        loadedRoomId = roomId
        appendLog(syncLogs, "media auto-loaded for roomId=$roomId url=$mediaUrl")
    }

    LaunchedEffect(uiState.currentRoomId) {
        val roomCode = uiState.currentRoomId ?: return@LaunchedEffect
        loadRoomDetail(roomCode)
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

    fun reportProgress(
        completed: Boolean = false,
        completionSource: String? = null,
        force: Boolean = false
    ) {
        val media = currentRoomMedia ?: return
        if (accessToken.isBlank()) return

        val nowMs = System.currentTimeMillis()
        val currentPositionSeconds = (currentUiState.player.currentPosition / 1_000L).coerceAtLeast(0L)
        val durationSeconds = (
            currentUiState.player.duration
                .takeIf { it > 0L }
                ?: media.durationMs
                ?: 1L
            ) / 1_000L
        val safeDurationSeconds = durationSeconds.coerceAtLeast(1L)
        val safePositionSeconds = currentPositionSeconds.coerceAtMost(safeDurationSeconds)

        if (!force) {
            val tooSoon = nowMs - lastProgressReportAtMs < 30_000L
            val sameSecond = safePositionSeconds == lastProgressReportPositionSeconds
            if (tooSoon || sameSecond) return
        }

        lastProgressReportAtMs = nowMs
        lastProgressReportPositionSeconds = safePositionSeconds
        coroutineScope.launch {
            runCatching {
                withContext(Dispatchers.IO) {
                    progressHttpClient.updateProgress(
                        accessToken = accessToken,
                        episodeId = media.id,
                        lastPositionSeconds = safePositionSeconds,
                        durationSeconds = safeDurationSeconds,
                        completed = completed,
                        completionSource = completionSource
                    )
                }
            }.onSuccess {
                appendLog(
                    syncLogs,
                    "progress reported episodeId=${it.episodeId} pos=${it.lastPositionSeconds}s completed=${it.completed}"
                )
            }.onFailure { error ->
                appendLog(syncLogs, "progress report failed: ${error.message}")
            }
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

        coroutineScope.launch {
            // Load business media metadata before WebSocket room_state can trigger player loading.
            loadRoomDetail(roomId)
            roomSessionController.startSession(
                roomId = roomId,
                userId = userId,
                listener = syncListener
            )
        }
    }

    fun createAndJoinAsHost() {
        coroutineScope.launch {
            runCatching {
                if (accessToken.isBlank()) {
                    error("Missing access token for DB-backed room creation")
                }
                updateUiState { current ->
                    current.copy(syncStatus = SyncStatus.CreatingRoom)
                }
                appendLog(syncLogs, "POST /rooms episodeId=$selectedEpisodeId hostUserId=$hostUserId")
                val createResult = withContext(Dispatchers.IO) {
                    roomSessionController.createRoom(
                        accessToken = accessToken,
                        episodeId = selectedEpisodeId
                    )
                }

                currentRoomMedia = createResult.media
                loadedRoomDetailCode = createResult.roomId
                updateUiState { current ->
                    current.copy(
                        currentRoomId = createResult.roomId,
                        joinRoomInput = createResult.roomId,
                        activeUserId = createResult.roomState.hostUserId
                    )
                }
                appendLog(
                    syncLogs,
                    "Created roomId=${createResult.roomId} media=${createResult.media.title} mediaId=${createResult.media.id}"
                )
                joinCurrentRoomAsUser(
                    roomId = createResult.roomId,
                    userId = createResult.roomState.hostUserId,
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

    LaunchedEffect(autoCreateAsHost, selectedEpisodeId, accessToken) {
        if (!autoCreateAsHost) return@LaunchedEffect
        if (selectedEpisodeId.isBlank()) return@LaunchedEffect
        if (autoCreatedEpisodeId == selectedEpisodeId) return@LaunchedEffect

        autoCreatedEpisodeId = selectedEpisodeId
        createAndJoinAsHost()
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
        if (!uiState.canControlPlayback) {
            appendLog(syncLogs, "play ignored: media is not ready")
            return
        }
        val sent = roomSessionController.sendPlay(
            positionMs = uiState.player.currentPosition,
            seq = currentState.seq
        )
        appendLog(syncLogs, "play sent=$sent at ${uiState.player.currentPosition}ms")
    }

    fun sendPause() {
        val currentState = uiState.latestSyncState ?: return
        if (!uiState.canControlPlayback) {
            appendLog(syncLogs, "pause ignored: media is not ready")
            return
        }
        val sent = roomSessionController.sendPause(
            positionMs = uiState.player.currentPosition,
            seq = currentState.seq
        )
        appendLog(syncLogs, "pause sent=$sent at ${uiState.player.currentPosition}ms")
        reportProgress(force = true)
    }

    fun sendSeek(targetPositionMs: Long) {
        val currentState = uiState.latestSyncState ?: return
        if (!uiState.canControlPlayback) {
            appendLog(syncLogs, "seek ignored: media is not ready")
            return
        }
        val sent = roomSessionController.sendSeek(
            positionMs = targetPositionMs,
            seq = currentState.seq
        )
        appendLog(syncLogs, "seek sent=$sent to ${targetPositionMs}ms")
    }

    fun sendPlaybackRateSync(speed: Float) {
        val currentState = uiState.latestSyncState ?: return
        if (!uiState.canControlPlayback) {
            appendLog(syncLogs, "playbackRate ignored: media is not ready")
            return
        }
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
        reportProgress(completed = true, completionSource = "ended", force = true)
    }

    LaunchedEffect(accessToken, currentRoomMedia?.id) {
        while (isActive) {
            delay(30_000L)
            reportProgress()
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

    RoomTheaterPage(
        modifier = modifier,
        hostUserId = hostUserId,
        viewerUserId = viewerUserId,
        mediaTitle = currentRoomMedia?.title ?: "等待选择影片",
        mediaEpisodeLabel = currentRoomMedia?.episodeLabel,
        uiState = uiState,
        adapter = adapter,
        isHostController = isHostController,
        onPlaybackToggleClick = playbackToggle@{
            if (!uiState.canControlPlayback) {
                appendLog(syncLogs, "playback ignored: media is not ready")
                return@playbackToggle
            }
            if (uiState.player.isPlaying) {
                if (isHostController) {
                    sendPause()
                } else {
                    adapter.pause()
                    reportProgress(force = true)
                }
            } else {
                if (isHostController) {
                    sendPlay()
                } else {
                    adapter.play()
                }
            }
        },
        onSeekBackwardClick = seekBackward@{
            if (!uiState.canControlPlayback) {
                appendLog(syncLogs, "seek ignored: media is not ready")
                return@seekBackward
            }
            val target = (uiState.player.currentPosition - 10_000L).coerceAtLeast(0L)
            if (isHostController) {
                sendSeek(target)
            } else {
                adapter.seekTo(target)
            }
        },
        onSeekForwardClick = seekForward@{
            if (!uiState.canControlPlayback) {
                appendLog(syncLogs, "seek ignored: media is not ready")
                return@seekForward
            }
            val safeDuration = if (uiState.player.duration > 0L) {
                uiState.player.duration
            } else {
                uiState.player.currentPosition + 10_000L
            }
            val target = (uiState.player.currentPosition + 10_000L).coerceAtMost(safeDuration)
            if (isHostController) {
                sendSeek(target)
            } else {
                adapter.seekTo(target)
            }
        },
        onPlaybackSpeedChange = speedChange@{ speed ->
            if (!uiState.canControlPlayback) {
                appendLog(syncLogs, "playbackRate ignored: media is not ready")
                return@speedChange
            }
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
        },
        onJoinRoomInputChange = { value ->
            updateUiState { current -> current.copy(joinRoomInput = value) }
        },
        onCreateAndJoinAsHost = ::createAndJoinAsHost,
        onJoinAsViewer = ::joinAsViewer,
        onRejoinCurrentUser = ::rejoinCurrentUser
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
