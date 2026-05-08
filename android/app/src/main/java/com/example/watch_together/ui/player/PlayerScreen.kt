package com.example.watch_together.ui.player

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.tooling.preview.Preview
import androidx.media3.common.Player
import androidx.core.content.ContextCompat
import com.example.watch_together.config.AppConfig
import com.example.watch_together.pages.room.RoomTheaterPage
import com.example.watch_together.sync.ProgressHttpClient
import com.example.watch_together.sync.DriftCorrectionType
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
    initialRoomCode: String = "",
    autoCreateAsHost: Boolean = false,
    autoJoinAsViewer: Boolean = false,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
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
    val viewerUserId = remember(currentUserId) {
        currentUserId.ifBlank { "android_viewer_${UUID.randomUUID().toString().take(4)}" }
    }

    val playerEventLogs = remember { mutableStateListOf<String>() }
    val syncLogs = remember { mutableStateListOf<String>() }

    var uiState by remember { mutableStateOf(RoomPlayerUiState()) }
    var loadedRoomId by remember { mutableStateOf<String?>(null) }
    var autoCreatedEpisodeId by rememberSaveable { mutableStateOf<String?>(null) }
    var autoJoinedRoomCode by rememberSaveable { mutableStateOf<String?>(null) }
    var loadedRoomDetailCode by rememberSaveable { mutableStateOf<String?>(null) }
    var lastProgressReportAtMs by rememberSaveable { mutableStateOf(0L) }
    var lastProgressReportPositionSeconds by rememberSaveable { mutableStateOf(-1L) }
    var lastBufferLogAtMs by rememberSaveable { mutableStateOf(0L) }
    var lastBufferLogState by rememberSaveable { mutableStateOf(-1) }
    var lastBufferingDriftSkipAtMs by rememberSaveable { mutableStateOf(0L) }
    var bufferingStartedAtMs by rememberSaveable { mutableStateOf(0L) }
    var bufferingCountsAsRebuffer by rememberSaveable { mutableStateOf(false) }
    var activeSpeedNudgeRestoreAtMs by rememberSaveable { mutableStateOf(0L) }
    var lastBackgroundPauseSeq by rememberSaveable { mutableStateOf(-1L) }
    val currentUiState by rememberUpdatedState(uiState)

    val isHostController = remember(uiState.activeUserId, uiState.latestSyncState) {
        uiState.activeUserId != null && uiState.latestSyncState?.hostUserId == uiState.activeUserId
    }
    val currentIsHostController by rememberUpdatedState(isHostController)
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
            updateUiState { current ->
                current.copy(roomMembers = detail.members)
            }
            appendLog(
                syncLogs,
                "room detail loaded roomCode=${detail.roomCode} media=${detail.media.title}"
            )
        }.onFailure { error ->
            appendLog(syncLogs, "room detail failed: ${error.message}")
        }.isSuccess
    }

    fun beginPostSeekRecovery(reason: String, targetPositionMs: Long) {
        val nowMs = System.currentTimeMillis()
        updateUiState { current ->
            current.copy(
                awaitingFirstFrameAfterSeek = true,
                seekRecoveryDeadlineAtMs = nowMs + POST_SEEK_FIRST_FRAME_TIMEOUT_MS,
                seekRecoveryRetryCount = 0,
                lastDriftCorrectionAtMs = nowMs,
                telemetry = current.telemetry.copy(
                    lastSeekRecoveryReason = "seek_started:$reason"
                )
            )
        }
        appendLog(syncLogs, "post-seek recovery start reason=$reason target=${targetPositionMs}ms")
    }

    fun clearPostSeekRecovery(reason: String, renderedFirstFrame: Boolean) {
        if (!currentUiState.awaitingFirstFrameAfterSeek) return
        val nowMs = System.currentTimeMillis()
        updateUiState { current ->
            current.copy(
                awaitingFirstFrameAfterSeek = false,
                seekRecoveryDeadlineAtMs = 0L,
                seekRecoveryRetryCount = 0,
                telemetry = current.telemetry.copy(
                    lastSeekRecoveryReason = reason,
                    lastRenderedFirstFrameAtMs = if (renderedFirstFrame) nowMs else current.telemetry.lastRenderedFirstFrameAtMs
                )
            )
        }
        appendLog(syncLogs, "post-seek recovery end reason=$reason")
    }

    DisposableEffect(adapter) {
        adapter.setEventListener { event ->
            appendLog(playerEventLogs, event.toDebugLabel(), maxSize = 8)
            if (event is PlayerEvent.VideoVariantChanged) {
                updateUiState { current ->
                    val qualityNotice = current.player.nextQualityNotice(event)
                    current.copy(
                        player = current.player.copy(
                            videoVariant = event.variant,
                            videoQualityNotice = qualityNotice
                        )
                    )
                }
                appendLog(syncLogs, "video variant ${event.variant.debugLabel}")
            }
            if (event is PlayerEvent.VideoQualitySwitchChanged) {
                updateUiState { current ->
                    current.copy(
                        player = current.player.copy(
                            videoQualitySwitchState = event.state,
                            videoQualityNotice = event.state.noticeLabel.ifBlank {
                                current.player.videoQualityNotice
                            }
                        )
                    )
                }
                appendLog(
                    syncLogs,
                    "quality switch phase=${event.state.phase} target=${event.state.preference.label}"
                )
            }
            if (event is PlayerEvent.VideoQualitiesChanged) {
                updateUiState { current ->
                    current.copy(
                        player = current.player.copy(availableVideoQualities = event.options)
                    )
                }
            }
            if (event is PlayerEvent.PlaybackStateChanged) {
                val nowMs = System.currentTimeMillis()
                val previousPlayer = currentUiState.player
                val previousTelemetry = currentUiState.telemetry
                val snapshot = playerSnapshotFromAdapter(
                    adapter = adapter,
                    playbackState = event.playbackState,
                    currentPlayer = currentUiState.player
                )
                val telemetry = updateRebufferTelemetry(
                    previousPlayer = previousPlayer,
                    previousTelemetry = previousTelemetry,
                    nextPlaybackState = event.playbackState,
                    nowMs = nowMs,
                    bufferingStartedAtMs = bufferingStartedAtMs,
                    bufferingCountsAsRebuffer = bufferingCountsAsRebuffer,
                    onBufferingSessionUpdate = { startedAtMs, countsAsRebuffer ->
                        bufferingStartedAtMs = startedAtMs
                        bufferingCountsAsRebuffer = countsAsRebuffer
                    },
                    onLog = { line -> logTelemetry(syncLogs, line) }
                )
                updateUiState { current ->
                    current.copy(
                        player = snapshot,
                        telemetry = telemetry
                    )
                }
                logBufferDebug(syncLogs, bufferDebugLogLine("buffer state", snapshot))
                lastBufferLogState = event.playbackState
            }
            if (event is PlayerEvent.RenderedFirstFrame) {
                clearPostSeekRecovery(
                    reason = "first_frame_rendered",
                    renderedFirstFrame = true
                )
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
            val snapshot = playerSnapshotFromAdapter(
                adapter = adapter,
                playbackState = uiState.player.playbackState,
                currentPlayer = uiState.player
            )
            updateUiState { current ->
                current.copy(
                    player = snapshot
                )
            }
            val nowMs = System.currentTimeMillis()
            val shouldLogBuffer = snapshot.playbackSpeed > 1.0f &&
                snapshot.hasActivePlaybackState() &&
                (nowMs - lastBufferLogAtMs >= BUFFER_DEBUG_LOG_INTERVAL_MS ||
                    snapshot.playbackState != lastBufferLogState)
            if (shouldLogBuffer) {
                logBufferDebug(syncLogs, bufferDebugLogLine("buffer tick", snapshot))
                lastBufferLogAtMs = nowMs
                lastBufferLogState = snapshot.playbackState
            }
            if (snapshot.hasActivePlaybackState()) {
                adapter.updateAheadPrefetch(
                    mediaUrl = uiState.telemetry.currentMediaUrl,
                    currentPositionMs = snapshot.currentPosition,
                    playbackSpeed = snapshot.playbackSpeed,
                    effectiveBufferedAheadMs = snapshot.effectiveBufferedAheadMs,
                    estimatedSegmentsAhead = snapshot.estimatedSegmentsAhead,
                    rebufferCount = uiState.telemetry.rebufferCount,
                    videoVariant = snapshot.videoVariant
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
        bufferingStartedAtMs = 0L
        bufferingCountsAsRebuffer = false
        updateUiState { current ->
            current.copy(telemetry = PlayerTelemetryUiState(currentMediaUrl = mediaUrl))
        }
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
            val playbackBuffering = uiState.player.playbackState == Player.STATE_BUFFERING

            if (uiState.awaitingFirstFrameAfterSeek) {
                if (nowMs >= uiState.seekRecoveryDeadlineAtMs) {
                    if (uiState.seekRecoveryRetryCount < MAX_POST_SEEK_RECOVERY_RETRIES &&
                        uiState.player.playbackState == Player.STATE_READY &&
                        !authorityState.paused
                    ) {
                        val retryPositionMs = maxOf(
                            uiState.player.currentPosition,
                            authorityState.positionMs
                        )
                        appendLog(
                            syncLogs,
                            "post-seek first frame timeout: retry=${uiState.seekRecoveryRetryCount + 1} target=${retryPositionMs}ms"
                        )
                        adapter.pause()
                        adapter.seekTo(retryPositionMs)
                        adapter.play()
                        updateUiState { current ->
                            current.copy(
                                awaitingFirstFrameAfterSeek = true,
                                seekRecoveryDeadlineAtMs = nowMs + POST_SEEK_RETRY_TIMEOUT_MS,
                                seekRecoveryRetryCount = current.seekRecoveryRetryCount + 1,
                                lastDriftCorrectionAtMs = nowMs,
                                telemetry = current.telemetry.copy(
                                    postSeekRecoveryCount = current.telemetry.postSeekRecoveryCount + 1,
                                    lastSeekRecoveryReason = "retry_after_timeout"
                                )
                            )
                        }
                    } else {
                        clearPostSeekRecovery(
                            reason = "timeout_without_first_frame",
                            renderedFirstFrame = false
                        )
                    }
                } else {
                    continue
                }
            }

            if (activeSpeedNudgeRestoreAtMs > 0L) {
                if (nowMs >= activeSpeedNudgeRestoreAtMs) {
                    roomSyncCoordinator.restoreAuthorityPlaybackRate(authorityState)
                    activeSpeedNudgeRestoreAtMs = 0L
                    updateUiState { current ->
                        current.copy(
                            player = current.player.copy(
                                playbackSpeed = authorityState.playbackRate.toFloat()
                            )
                        )
                    }
                    logTelemetry(
                        syncLogs,
                        "correction type=speed_nudge_restore targetRate=${authorityState.playbackRate}x"
                    )
                } else {
                    continue
                }
            }

            val driftCheck = roomSyncCoordinator.evaluateDrift(
                authorityState = authorityState,
                nowMs = nowMs,
                lastCorrectionAtMs = uiState.lastDriftCorrectionAtMs,
                durationMs = uiState.player.duration,
                playbackEnded = uiState.player.playbackState == Player.STATE_ENDED,
                playbackBuffering = playbackBuffering,
                seekThresholdMs = uiState.player.dynamicSeekFallbackThresholdMs()
            )

            if (playbackBuffering && nowMs - lastBufferingDriftSkipAtMs >= BUFFER_DEBUG_LOG_INTERVAL_MS) {
                appendLog(
                    syncLogs,
                    "drift correction skipped: local player is buffering ahead=${uiState.player.bufferedAheadMs}ms"
                )
                lastBufferingDriftSkipAtMs = nowMs
            }

            if (!driftCheck.shouldCorrect) {
                continue
            }

            roomSyncCoordinator.applyDriftCorrection(driftCheck, authorityState)
            val correctionReason = when (driftCheck.correctionType) {
                DriftCorrectionType.None -> "drift_none"
                DriftCorrectionType.SpeedNudge -> "drift_speed_nudge"
                DriftCorrectionType.Seek -> "drift_seek"
            }
            if (driftCheck.correctionType == DriftCorrectionType.SpeedNudge) {
                activeSpeedNudgeRestoreAtMs = nowMs + driftCheck.speedNudgeDurationMs
            }
            updateUiState { current ->
                current.copy(
                    lastDriftCorrectionAtMs = nowMs,
                    player = if (driftCheck.correctionType == DriftCorrectionType.SpeedNudge) {
                        current.player.copy(playbackSpeed = driftCheck.speedNudgeRate)
                    } else {
                        current.player
                    },
                    telemetry = current.telemetry.copy(
                        driftCorrectionCount = current.telemetry.driftCorrectionCount + 1,
                        seekCorrectionCount = current.telemetry.seekCorrectionCount +
                            if (driftCheck.correctionType == DriftCorrectionType.Seek) 1 else 0,
                        speedNudgeCorrectionCount = current.telemetry.speedNudgeCorrectionCount +
                            if (driftCheck.correctionType == DriftCorrectionType.SpeedNudge) 1 else 0,
                        lastCorrectionReason = correctionReason,
                        lastCorrectionDriftMs = driftCheck.driftMs
                    )
                )
            }
            when (driftCheck.correctionType) {
                DriftCorrectionType.None -> Unit
                DriftCorrectionType.SpeedNudge -> {
                    appendLog(
                        syncLogs,
                        "drift speed nudge local=${driftCheck.localPositionMs} expected=${driftCheck.expectedPositionMs} " +
                            "drift=${driftCheck.driftMs} rate=${driftCheck.speedNudgeRate}x"
                    )
                    logTelemetry(
                        syncLogs,
                        "correction type=speed_nudge count=${currentUiState.telemetry.driftCorrectionCount + 1} " +
                            "drift=${driftCheck.driftMs}ms rate=${driftCheck.speedNudgeRate}x " +
                            "restoreIn=${driftCheck.speedNudgeDurationMs}ms"
                    )
                }
                DriftCorrectionType.Seek -> {
                    appendLog(
                        syncLogs,
                        "drift seek correction local=${driftCheck.localPositionMs} expected=${driftCheck.expectedPositionMs} drift=${driftCheck.driftMs}"
                    )
                    logTelemetry(
                        syncLogs,
                        "correction type=drift_seek count=${currentUiState.telemetry.driftCorrectionCount + 1} drift=${driftCheck.driftMs}ms"
                    )
                }
            }
        }
    }

    fun applySyncEventResult(result: RoomPlayerSyncEventResult) {
        if (activeSpeedNudgeRestoreAtMs > 0L) {
            result.uiState.latestSyncState?.let(roomSyncCoordinator::restoreAuthorityPlaybackRate)
        }
        activeSpeedNudgeRestoreAtMs = 0L
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
        activeSpeedNudgeRestoreAtMs = 0L
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

    fun joinAsViewer(roomCodeOverride: String? = null) {
        val roomId = (roomCodeOverride ?: uiState.joinRoomInput).trim().uppercase()
        if (roomId.isBlank()) {
            appendLog(syncLogs, "Join aborted: roomId is empty")
            return
        }
        if (accessToken.isBlank()) {
            updateUiState { current -> current.copy(syncStatus = SyncStatus.SyncFailed) }
            appendLog(syncLogs, "Join aborted: missing access token")
            return
        }

        coroutineScope.launch {
            runCatching {
                updateUiState { current ->
                    current.copy(
                        joinRoomInput = roomId,
                        activeUserId = viewerUserId,
                        syncStatus = SyncStatus.JoiningAsViewer
                    )
                }
                appendLog(syncLogs, "POST /rooms/$roomId/join userId=$viewerUserId")
                val joinResult = withContext(Dispatchers.IO) {
                    roomSessionController.joinRoomByCode(
                        accessToken = accessToken,
                        roomCode = roomId
                    )
                }
                appendLog(
                    syncLogs,
                    "joined roomCode=${joinResult.roomCode} role=${joinResult.memberRole}"
                )
                joinCurrentRoomAsUser(
                    roomId = joinResult.roomCode,
                    userId = joinResult.memberUserId,
                    status = SyncStatus.JoiningAsViewer,
                    reason = "viewer join"
                )
            }.onFailure { error ->
                updateUiState { current -> current.copy(syncStatus = SyncStatus.SyncFailed) }
                appendLog(syncLogs, "Join viewer failed: ${error.message}")
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

    LaunchedEffect(autoJoinAsViewer, initialRoomCode, accessToken) {
        val normalizedRoomCode = initialRoomCode.trim().uppercase()
        if (!autoJoinAsViewer) return@LaunchedEffect
        if (normalizedRoomCode.length != 6) return@LaunchedEffect
        if (autoJoinedRoomCode == normalizedRoomCode) return@LaunchedEffect

        autoJoinedRoomCode = normalizedRoomCode
        joinAsViewer(normalizedRoomCode)
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
        if (!uiState.player.canStartHighSpeedPlayback()) {
            appendLog(
                syncLogs,
                "play delayed: 2.0x needs more effective buffer effectiveAhead=${uiState.player.effectiveBufferedAheadMs}ms"
            )
            logTelemetry(
                syncLogs,
                "high_speed_start_gate blocked speed=${uiState.player.playbackSpeed}x " +
                    "ahead=${uiState.player.bufferedAheadMs}ms " +
                    "effectiveAhead=${uiState.player.effectiveBufferedAheadMs}ms " +
                    "segments=${uiState.player.estimatedSegmentsAhead}"
            )
            return
        }
        val sent = roomSessionController.sendPlay(
            positionMs = uiState.player.currentPosition,
            seq = currentState.seq
        )
        appendLog(syncLogs, "play sent=$sent at ${uiState.player.currentPosition}ms")
    }

    fun sendPause(reason: String = "user", force: Boolean = false) {
        val currentState = uiState.latestSyncState ?: return
        if (!force && !uiState.canControlPlayback) {
            appendLog(syncLogs, "pause ignored: media is not ready")
            return
        }
        adapter.pause()
        val sent = roomSessionController.sendPause(
            positionMs = uiState.player.currentPosition,
            seq = currentState.seq
        )
        appendLog(syncLogs, "pause sent=$sent reason=$reason at ${uiState.player.currentPosition}ms")
        reportProgress(force = true)
    }

    fun pauseAuthorityIfNeeded(reason: String) {
        val snapshot = currentUiState
        val syncState = snapshot.latestSyncState ?: return
        if (!currentIsHostController) return
        if (!snapshot.shouldPauseAuthorityOnBackground) return
        if (lastBackgroundPauseSeq == syncState.seq) return

        lastBackgroundPauseSeq = syncState.seq
        appendLog(syncLogs, "lifecycle $reason: host left active playback, pausing room authority")
        sendPause(reason = reason, force = true)
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

    fun commitProgressSeek(targetPositionMs: Long) {
        if (!uiState.canControlPlayback) {
            appendLog(syncLogs, "progress seek ignored: media is not ready")
            return
        }
        val safeDuration = uiState.player.duration.takeIf { it > 0L }
        val target = if (safeDuration != null) {
            targetPositionMs.coerceIn(0L, safeDuration)
        } else {
            targetPositionMs.coerceAtLeast(0L)
        }
        updateUiState { current ->
            current.copy(lastDriftCorrectionAtMs = System.currentTimeMillis())
        }
        beginPostSeekRecovery(reason = "local_seek_commit", targetPositionMs = target)
        if (isHostController && uiState.latestSyncState != null) {
            appendLog(syncLogs, "progress seek commit path=sync target=${target}ms")
            sendSeek(target)
        } else {
            appendLog(syncLogs, "progress seek commit path=local target=${target}ms")
            adapter.seekTo(target)
        }
    }

    fun sendPlaybackRateSync(speed: Float) {
        val currentState = uiState.latestSyncState ?: return
        if (!uiState.canControlPlayback) {
            appendLog(syncLogs, "playbackRate ignored: media is not ready")
            return
        }
        activeSpeedNudgeRestoreAtMs = 0L
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

    LaunchedEffect(uiState.player.playbackSpeed, uiState.telemetry.rebufferCount) {
        adapter.updatePlaybackStrategy(
            playbackSpeed = uiState.player.playbackSpeed,
            rebufferCount = uiState.telemetry.rebufferCount
        )
    }

    KeepScreenAwakeEffect(uiState.shouldKeepScreenOn)

    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event != Lifecycle.Event.ON_STOP) return@LifecycleEventObserver
            pauseAuthorityIfNeeded(reason = "backgrounded")
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose {
            lifecycleOwner.lifecycle.removeObserver(observer)
        }
    }

    DisposableEffect(context) {
        val receiver = object : BroadcastReceiver() {
            override fun onReceive(context: Context?, intent: Intent?) {
                if (intent?.action != Intent.ACTION_SCREEN_OFF) return
                pauseAuthorityIfNeeded(reason = "screen_off")
            }
        }
        ContextCompat.registerReceiver(
            context,
            receiver,
            IntentFilter(Intent.ACTION_SCREEN_OFF),
            ContextCompat.RECEIVER_NOT_EXPORTED
        )
        onDispose {
            runCatching { context.unregisterReceiver(receiver) }
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
                if (!uiState.player.canStartHighSpeedPlayback()) {
                    appendLog(
                        syncLogs,
                        "playback ignored: 2.0x needs more effective buffer effectiveAhead=${uiState.player.effectiveBufferedAheadMs}ms"
                    )
                    logTelemetry(
                        syncLogs,
                        "high_speed_start_gate blocked speed=${uiState.player.playbackSpeed}x " +
                            "ahead=${uiState.player.bufferedAheadMs}ms " +
                            "effectiveAhead=${uiState.player.effectiveBufferedAheadMs}ms " +
                            "segments=${uiState.player.estimatedSegmentsAhead}"
                    )
                    return@playbackToggle
                }
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
        onProgressSeekCommit = { targetPositionMs ->
            commitProgressSeek(targetPositionMs)
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
        onVideoQualityPreferenceChange = { preference ->
            appendLog(syncLogs, "quality preference click value=${preference.label}")
            updateUiState { current ->
                current.copy(
                    player = current.player.copy(
                        videoQualityPreference = preference,
                        videoQualitySwitchState = PlayerVideoQualitySwitchState(
                            phase = if (preference.isAuto) {
                                PlayerVideoQualitySwitchPhase.Committed
                            } else {
                                PlayerVideoQualitySwitchPhase.PendingRequest
                            },
                            preference = preference
                        ),
                        videoQualityNotice = if (preference.isAuto) {
                            PlayerVideoQualitySwitchState(
                                phase = PlayerVideoQualitySwitchPhase.Committed,
                                preference = preference
                            ).noticeLabel
                        } else {
                            PlayerVideoQualitySwitchState(
                                phase = PlayerVideoQualitySwitchPhase.PendingRequest,
                                preference = preference
                            ).noticeLabel
                        }
                    )
                )
            }
            adapter.setVideoQualityPreference(preference)
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
internal fun KeepScreenAwakeEffect(shouldKeepScreenOn: Boolean) {
    val rootView = LocalView.current
    DisposableEffect(rootView, shouldKeepScreenOn) {
        rootView.keepScreenOn = shouldKeepScreenOn
        onDispose {
            rootView.keepScreenOn = false
        }
    }
}

@Composable
private fun rememberPlayerAdapter(): PlayerAdapter {
    val context = androidx.compose.ui.platform.LocalContext.current
    val appContext = context.applicationContext
    val adapter = remember(appContext) { AndroidExoPlayerAdapter(appContext) }

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

private const val BUFFER_DEBUG_LOG_INTERVAL_MS = 3_000L
private const val BUFFER_DEBUG_LOG_TAG = "WatchTogetherBuffer"
private const val TELEMETRY_LOG_TAG = "WatchTogetherTelemetry"
private const val POST_SEEK_FIRST_FRAME_TIMEOUT_MS = 2_500L
private const val POST_SEEK_RETRY_TIMEOUT_MS = 2_000L
private const val MAX_POST_SEEK_RECOVERY_RETRIES = 1
private const val HIGH_SPEED_PLAYBACK_THRESHOLD = 2.0f
private const val HIGH_SPEED_START_EFFECTIVE_AHEAD_MS = 12_000L
private const val HIGH_SPEED_START_SEGMENTS_AHEAD = 2
private const val HIGH_SPEED_SEEK_FALLBACK_THRESHOLD_MS = 3_000L

private fun logBufferDebug(logs: MutableList<String>, line: String) {
    appendLog(logs, line)
    PlayerDebugLog.d(BUFFER_DEBUG_LOG_TAG, line)
}

private fun logTelemetry(logs: MutableList<String>, line: String) {
    appendLog(logs, line)
    PlayerDebugLog.d(TELEMETRY_LOG_TAG, line)
}

private fun updateRebufferTelemetry(
    previousPlayer: PlayerRuntimeUiState,
    previousTelemetry: PlayerTelemetryUiState,
    nextPlaybackState: Int,
    nowMs: Long,
    bufferingStartedAtMs: Long,
    bufferingCountsAsRebuffer: Boolean,
    onBufferingSessionUpdate: (startedAtMs: Long, countsAsRebuffer: Boolean) -> Unit,
    onLog: (String) -> Unit,
): PlayerTelemetryUiState {
    if (nextPlaybackState == Player.STATE_BUFFERING && bufferingStartedAtMs <= 0L) {
        val countsAsRebuffer = previousPlayer.hasPlaybackStartedForTelemetry()
        onBufferingSessionUpdate(nowMs, countsAsRebuffer)
        val nextTelemetry = if (countsAsRebuffer) {
            previousTelemetry.copy(
                rebufferCount = previousTelemetry.rebufferCount + 1,
                activeRebufferStartedAtMs = nowMs
            )
        } else {
            previousTelemetry
        }
        val type = if (countsAsRebuffer) "rebuffer" else "initial_buffer"
        onLog(
            "buffering start type=$type count=${nextTelemetry.rebufferCount} " +
                "pos=${previousPlayer.currentPosition}ms ahead=${previousPlayer.bufferedAheadMs}ms " +
                "effectiveAhead=${previousPlayer.effectiveBufferedAheadMs}ms " +
                "segments=${previousPlayer.estimatedSegmentsAhead} " +
                "speed=${previousPlayer.playbackSpeed}x variant=${previousPlayer.videoVariant.displayLabel}"
        )
        return nextTelemetry
    }

    if (previousPlayer.playbackState == Player.STATE_BUFFERING &&
        nextPlaybackState != Player.STATE_BUFFERING &&
        bufferingStartedAtMs > 0L
    ) {
        val durationMs = (nowMs - bufferingStartedAtMs).coerceAtLeast(0L)
        onBufferingSessionUpdate(0L, false)
        if (!bufferingCountsAsRebuffer) {
            onLog("buffering end type=initial_buffer duration=${durationMs}ms")
            return previousTelemetry
        }
        val nextTelemetry = previousTelemetry.copy(
            totalRebufferDurationMs = previousTelemetry.totalRebufferDurationMs + durationMs,
            lastRebufferDurationMs = durationMs,
            activeRebufferStartedAtMs = 0L
        )
        onLog(
            "rebuffer end duration=${durationMs}ms " +
                "count=${nextTelemetry.rebufferCount} total=${nextTelemetry.totalRebufferDurationMs}ms"
        )
        return nextTelemetry
    }

    return previousTelemetry
}

private fun bufferDebugLogLine(prefix: String, snapshot: PlayerRuntimeUiState): String {
    return "$prefix state=${snapshot.playbackState.toPlaybackStateLabel()} " +
        "pos=${snapshot.currentPosition}ms " +
        "buffered=${snapshot.bufferedPosition}ms " +
        "ahead=${snapshot.bufferedAheadMs}ms " +
        "effectiveAhead=${snapshot.effectiveBufferedAheadMs}ms " +
        "segments=${snapshot.estimatedSegmentsAhead} " +
        "percent=${snapshot.bufferedPercentage}% " +
        "speed=${snapshot.playbackSpeed}x " +
        "variant=${snapshot.videoVariant.displayLabel}"
}

private fun PlayerRuntimeUiState.hasActivePlaybackState(): Boolean {
    return playbackState == Player.STATE_BUFFERING ||
        playbackState == Player.STATE_READY
}

private fun PlayerRuntimeUiState.hasPlaybackStartedForTelemetry(): Boolean {
    return playbackState == Player.STATE_READY ||
        currentPosition > 0L ||
        bufferedPosition > 0L ||
        isPlaying
}

private fun PlayerRuntimeUiState.canStartHighSpeedPlayback(): Boolean {
    if (playbackSpeed < HIGH_SPEED_PLAYBACK_THRESHOLD) return true
    return effectiveBufferedAheadMs >= HIGH_SPEED_START_EFFECTIVE_AHEAD_MS &&
        estimatedSegmentsAhead >= HIGH_SPEED_START_SEGMENTS_AHEAD
}

private fun PlayerRuntimeUiState.dynamicSeekFallbackThresholdMs(): Long {
    return if (playbackSpeed >= HIGH_SPEED_PLAYBACK_THRESHOLD) {
        HIGH_SPEED_SEEK_FALLBACK_THRESHOLD_MS
    } else {
        RoomSyncCoordinator.DEFAULT_SEEK_DRIFT_THRESHOLD_MS
    }
}

private fun PlayerRuntimeUiState.nextQualityNotice(event: PlayerEvent.VideoVariantChanged): String {
    val previousHeight = videoVariant.height
    val nextHeight = event.variant.height
    if (nextHeight <= 0) return videoQualityNotice
    if (!videoQualityPreference.isAuto && nextHeight < (videoQualityPreference.height ?: 0)) {
        return "当前网络或设备压力较高，已优先保障流畅播放。"
    }
    if (videoQualityPreference.isAuto &&
        previousHeight > 0 &&
        nextHeight < previousHeight &&
        (event.reason == "abr" || event.reason == "selected-track")
    ) {
        return "播放不流畅，已自动切到 ${event.variant.qualityLabel}。"
    }
    if (videoQualityPreference.isAuto &&
        previousHeight > 0 &&
        nextHeight > previousHeight &&
        event.reason == "abr"
    ) {
        return "网络恢复，已自动升到 ${event.variant.qualityLabel}。"
    }
    return videoQualityNotice
}

private fun playerSnapshotFromAdapter(
    adapter: PlayerAdapter,
    playbackState: Int,
    currentPlayer: PlayerRuntimeUiState
): PlayerRuntimeUiState {
    return currentPlayer.copy(
        currentPosition = adapter.getCurrentPosition(),
        duration = adapter.getDuration().coerceAtLeast(0L),
        bufferedPosition = adapter.getBufferedPosition().coerceAtLeast(0L),
        bufferedPercentage = adapter.getBufferedPercentage().coerceIn(0, 100),
        isPlaying = adapter.isPlaying(),
        playbackState = playbackState
    )
}
