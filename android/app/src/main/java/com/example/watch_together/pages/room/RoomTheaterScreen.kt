package com.example.watch_together.pages.room

import android.os.SystemClock
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.media3.common.Player
import com.example.watch_together.config.AppConfig
import com.example.watch_together.pages.video.MediaEpisode
import com.example.watch_together.ui.player.AndroidExoPlayerAdapter
import com.example.watch_together.ui.player.PlayerAdapter
import com.example.watch_together.ui.player.PlayerDebugLog
import com.example.watch_together.ui.player.PlayerEvent
import com.example.watch_together.ui.player.PlayerRuntimeState
import com.example.watch_together.ui.player.PlayerVideoQualityPreference
import com.example.watch_together.ui.player.toDebugLabel
import com.example.watch_together.ui.player.toPlaybackStateLabel
import com.example.watch_together.sync.RoomHttpClient
import com.example.watch_together.sync.RoomMember
import com.example.watch_together.sync.RoomMedia
import com.example.watch_together.sync.RoomSessionController
import com.example.watch_together.sync.RoomSyncState
import com.example.watch_together.sync.RoomWebSocketListener
import com.example.watch_together.sync.protocol.EndedPayload
import com.example.watch_together.sync.protocol.PausePayload
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.RoomDeviceSwitchRequestPayload
import com.example.watch_together.sync.protocol.RoomDeviceSwitchResultPayload
import com.example.watch_together.sync.protocol.RoomMembersChangedPayload
import com.example.watch_together.sync.protocol.SeekPayload
import com.example.watch_together.sync.protocol.SetPlaybackRatePayload
import com.example.watch_together.ui.theme.Watch_togetherTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

@Composable
fun RoomTheaterScreen(
    accessToken: String = "",
    currentUserId: String = "",
    currentUserNickname: String = "",
    selectedEpisode: MediaEpisode = defaultSelectedEpisode(),
    initialRoomCode: String = "",
    autoCreateAsHost: Boolean = false,
    autoJoinAsViewer: Boolean = false,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    val coroutineScope = rememberCoroutineScope()
    val adapter = remember(context.applicationContext) { AndroidExoPlayerAdapter(context.applicationContext) }
    val roomHttpClient = remember { RoomHttpClient() }
    val roomSessionController = remember { RoomSessionController(roomHttpClient = roomHttpClient) }
    var playerState by remember { mutableStateOf(PlayerRuntimeState()) }
    var mediaLoadError by remember { mutableStateOf<String?>(null) }
    var isMediaLoading by remember { mutableStateOf(false) }
    var activeEpisode by remember { mutableStateOf(selectedEpisode) }
    var activeRoomCode by remember { mutableStateOf(initialRoomCode.trim().uppercase()) }
    var activeRoomRole by remember { mutableStateOf<String?>(null) }
    var activeRoomUserId by remember { mutableStateOf<String?>(null) }
    var latestRoomState by remember { mutableStateOf<RoomSyncState?>(null) }
    var isApplyingRemoteEvent by remember { mutableStateOf(false) }
    var roomBootstrapStatus by remember { mutableStateOf(roomBootstrapInitialStatus(autoCreateAsHost, autoJoinAsViewer, initialRoomCode)) }
    var pendingPostSeekCalibrationJob by remember { mutableStateOf<Job?>(null) }
    var pendingRoomDeviceSwitchRequest by remember { mutableStateOf<RoomDeviceSwitchRequestPayload?>(null) }
    val playerLogs = remember { mutableStateListOf<String>() }
    val currentActiveRoomUserId by rememberUpdatedState(activeRoomUserId)
    val currentPlayerState by rememberUpdatedState(playerState)
    val currentLatestRoomState by rememberUpdatedState(latestRoomState)
    val currentIsApplyingRemoteEvent by rememberUpdatedState(isApplyingRemoteEvent)
    val roomHint = buildRoomHint(
        roomCode = activeRoomCode,
        role = activeRoomRole,
        bootstrapStatus = roomBootstrapStatus
    )
    val canApplyLocalPlaybackControl = activeRoomCode.isBlank() || activeRoomRole == "host"

    LaunchedEffect(autoCreateAsHost, selectedEpisode.episodeId, accessToken) {
        if (!autoCreateAsHost) return@LaunchedEffect
        if (selectedEpisode.episodeId.isBlank()) return@LaunchedEffect
        if (accessToken.isBlank()) {
            roomBootstrapStatus = "创建房间失败：缺少登录 token"
            appendLog(playerLogs, roomBootstrapStatus, maxSize = 10)
            return@LaunchedEffect
        }

        roomBootstrapStatus = "正在创建房间..."
        appendLog(playerLogs, "POST /rooms episodeId=${selectedEpisode.episodeId}", maxSize = 10)
        runCatching {
            withContext(Dispatchers.IO) {
                roomHttpClient.createRoom(accessToken = accessToken, episodeId = selectedEpisode.episodeId)
            }
        }.onSuccess { result ->
            activeRoomCode = result.roomCode
            activeRoomRole = "host"
            activeRoomUserId = result.roomState.hostUserId
            latestRoomState = result.roomState
            activeEpisode = result.media.toMediaEpisode()
            roomBootstrapStatus = "房间已创建 · 正在连接同步"
            appendLog(playerLogs, "room created code=${result.roomCode} media=${result.media.title}", maxSize = 10)
        }.onFailure { error ->
            roomBootstrapStatus = "创建房间失败：${error.message ?: "未知错误"}"
            appendLog(playerLogs, roomBootstrapStatus, maxSize = 10)
        }
    }

    LaunchedEffect(autoJoinAsViewer, initialRoomCode, accessToken) {
        val normalizedRoomCode = initialRoomCode.trim().uppercase()
        if (!autoJoinAsViewer) return@LaunchedEffect
        if (normalizedRoomCode.isBlank()) return@LaunchedEffect
        if (accessToken.isBlank()) {
            roomBootstrapStatus = "加入房间失败：缺少登录 token"
            appendLog(playerLogs, roomBootstrapStatus, maxSize = 10)
            return@LaunchedEffect
        }

        roomBootstrapStatus = "正在加入房间..."
        appendLog(playerLogs, "POST /rooms/$normalizedRoomCode/join", maxSize = 10)
        runCatching {
            withContext(Dispatchers.IO) {
                val joinResult = roomHttpClient.joinRoomByCode(accessToken = accessToken, roomCode = normalizedRoomCode)
                val detail = roomHttpClient.getRoomDetail(joinResult.roomCode)
                joinResult to detail
            }
        }.onSuccess { (joinResult, detail) ->
            activeRoomCode = joinResult.roomCode
            activeRoomRole = joinResult.memberRole
            activeRoomUserId = joinResult.memberUserId
            activeEpisode = detail.media.toMediaEpisode()
            roomBootstrapStatus = "房间已加入 · 正在连接同步"
            appendLog(playerLogs, "room joined code=${joinResult.roomCode} media=${detail.media.title}", maxSize = 10)
        }.onFailure { error ->
            roomBootstrapStatus = "加入房间失败：${error.message ?: "未知错误"}"
            appendLog(playerLogs, roomBootstrapStatus, maxSize = 10)
        }
    }

    val syncListener = remember {
        object : RoomWebSocketListener {
            override fun onLog(message: String) {
                coroutineScope.launch {
                    appendPlayerLog(playerLogs, message, maxSize = 10)
                }
            }

            override fun onRoomState(payload: RoomSyncState) {
                coroutineScope.launch {
                    latestRoomState = payload
                    roomBootstrapStatus = "同步已连接 · room_state=${payload.seq}"
                    isApplyingRemoteEvent = true
                    adapter.seekTo(payload.positionMs)
                    adapter.setPlaybackSpeed(payload.playbackRate.toFloat())
                    playerState = playerState.copy(playbackSpeed = payload.playbackRate.toFloat())
                    if (payload.paused) adapter.pause() else adapter.play()
                    isApplyingRemoteEvent = false
                    appendPlayerLog(
                        playerLogs,
                        "room_state applied seq=${payload.seq} paused=${payload.paused} pos=${payload.positionMs} rate=${payload.playbackRate}",
                        maxSize = 10
                    )
                }
            }

            override fun onRoomMembersChanged(payload: RoomMembersChangedPayload) {
                coroutineScope.launch {
                    appendPlayerLog(playerLogs, "members changed reason=${payload.reason}", maxSize = 10)
                }
            }

            override fun onRoomDeviceWaiting(payload: RoomDeviceSwitchRequestPayload) {
                coroutineScope.launch {
                    roomBootstrapStatus = "等待旧设备确认房间切换..."
                    appendPlayerLog(
                        playerLogs,
                        "device switch waiting requestId=${payload.requestId} target=${payload.targetRoomId}",
                        maxSize = 10
                    )
                }
            }

            override fun onRoomDeviceSwitchRequest(payload: RoomDeviceSwitchRequestPayload) {
                coroutineScope.launch {
                    pendingRoomDeviceSwitchRequest = payload
                    appendPlayerLog(
                        playerLogs,
                        "device switch request requestId=${payload.requestId} target=${payload.targetRoomId}",
                        maxSize = 10
                    )
                }
            }

            override fun onRoomDeviceSwitchResult(payload: RoomDeviceSwitchResultPayload) {
                coroutineScope.launch {
                    appendPlayerLog(
                        playerLogs,
                        "device switch result requestId=${payload.requestId} approved=${payload.approved} reason=${payload.reason}",
                        maxSize = 10
                    )
                    if (!payload.approved) {
                        roomBootstrapStatus = if (payload.reason.isBlank()) {
                            "房间设备切换被拒绝"
                        } else {
                            "房间设备切换失败：${payload.reason}"
                        }
                    }
                    if (pendingRoomDeviceSwitchRequest?.requestId == payload.requestId) {
                        pendingRoomDeviceSwitchRequest = null
                    }
                }
            }

            override fun onPlay(payload: PlayPayload) {
                coroutineScope.launch {
                    if (payload.userId == currentActiveRoomUserId) {
                        latestRoomState = currentLatestRoomState?.copy(
                            paused = false,
                            ended = false,
                            positionMs = payload.positionMs,
                            seq = payload.seq
                        )
                        appendPlayerLog(playerLogs, "self play ack seq=${payload.seq} pos=${payload.positionMs}", maxSize = 10)
                        return@launch
                    }
                    appendRemoteEventReceivedLog(playerLogs, adapter, "play", payload.seq, payload.positionMs)
                    latestRoomState = currentLatestRoomState?.copy(
                        paused = false,
                        ended = false,
                        positionMs = payload.positionMs,
                        seq = payload.seq
                    )
                    isApplyingRemoteEvent = true
                    adapter.seekTo(payload.positionMs)
                    adapter.play()
                    isApplyingRemoteEvent = false
                    appendPlayerLog(playerLogs, "remote play seq=${payload.seq} pos=${payload.positionMs}", maxSize = 10)
                    scheduleRemoteEventObservation(coroutineScope, adapter, playerLogs, "play", payload.seq)
                }
            }

            override fun onPause(payload: PausePayload) {
                coroutineScope.launch {
                    if (payload.userId == currentActiveRoomUserId) {
                        latestRoomState = currentLatestRoomState?.copy(
                            paused = true,
                            ended = false,
                            positionMs = payload.positionMs,
                            seq = payload.seq
                        )
                        appendPlayerLog(playerLogs, "self pause ack seq=${payload.seq} pos=${payload.positionMs}", maxSize = 10)
                        return@launch
                    }
                    appendRemoteEventReceivedLog(playerLogs, adapter, "pause", payload.seq, payload.positionMs)
                    latestRoomState = currentLatestRoomState?.copy(
                        paused = true,
                        ended = false,
                        positionMs = payload.positionMs,
                        seq = payload.seq
                    )
                    isApplyingRemoteEvent = true
                    adapter.seekTo(payload.positionMs)
                    adapter.pause()
                    isApplyingRemoteEvent = false
                    appendPlayerLog(playerLogs, "remote pause seq=${payload.seq} pos=${payload.positionMs}", maxSize = 10)
                    scheduleRemoteEventObservation(coroutineScope, adapter, playerLogs, "pause", payload.seq)
                }
            }

            override fun onSeek(payload: SeekPayload) {
                coroutineScope.launch {
                    if (payload.userId == currentActiveRoomUserId) {
                        latestRoomState = currentLatestRoomState?.copy(
                            positionMs = payload.positionMs,
                            seq = payload.seq
                        )
                        appendPlayerLog(playerLogs, "self seek ack seq=${payload.seq} pos=${payload.positionMs}", maxSize = 10)
                        return@launch
                    }
                    appendRemoteEventReceivedLog(playerLogs, adapter, "seek", payload.seq, payload.positionMs)
                    latestRoomState = currentLatestRoomState?.copy(
                        positionMs = payload.positionMs,
                        seq = payload.seq
                    )
                    applyRemoteSeekWithThreshold(
                        adapter = adapter,
                        logs = playerLogs,
                        payload = payload,
                        coroutineScope = coroutineScope,
                        setApplyingRemoteEvent = { isApplyingRemoteEvent = it }
                    )
                }
            }

            override fun onPlaybackRate(payload: SetPlaybackRatePayload) {
                coroutineScope.launch {
                    if (payload.userId == currentActiveRoomUserId) {
                        latestRoomState = currentLatestRoomState?.copy(
                            positionMs = payload.positionMs,
                            playbackRate = payload.playbackRate,
                            seq = payload.seq
                        )
                        appendPlayerLog(playerLogs, "self rate ack seq=${payload.seq} rate=${payload.playbackRate}", maxSize = 10)
                        return@launch
                    }
                    appendRemoteEventReceivedLog(playerLogs, adapter, "rate", payload.seq, payload.positionMs)
                    latestRoomState = currentLatestRoomState?.copy(
                        positionMs = payload.positionMs,
                        playbackRate = payload.playbackRate,
                        seq = payload.seq
                    )
                    isApplyingRemoteEvent = true
                    adapter.seekTo(payload.positionMs)
                    adapter.setPlaybackSpeed(payload.playbackRate.toFloat())
                    playerState = playerState.copy(playbackSpeed = payload.playbackRate.toFloat())
                    isApplyingRemoteEvent = false
                    appendPlayerLog(playerLogs, "remote rate seq=${payload.seq} rate=${payload.playbackRate}", maxSize = 10)
                    scheduleRemoteEventObservation(coroutineScope, adapter, playerLogs, "rate", payload.seq)
                }
            }

            override fun onEnded(payload: EndedPayload) {
                coroutineScope.launch {
                    if (payload.userId == currentActiveRoomUserId) {
                        latestRoomState = currentLatestRoomState?.copy(
                            paused = true,
                            ended = true,
                            positionMs = payload.positionMs,
                            seq = payload.seq
                        )
                        appendPlayerLog(playerLogs, "self ended ack seq=${payload.seq}", maxSize = 10)
                        return@launch
                    }
                    appendRemoteEventReceivedLog(playerLogs, adapter, "ended", payload.seq, payload.positionMs)
                    isApplyingRemoteEvent = true
                    adapter.seekTo(payload.positionMs)
                    adapter.pause()
                    isApplyingRemoteEvent = false
                    appendPlayerLog(playerLogs, "remote ended seq=${payload.seq}", maxSize = 10)
                    scheduleRemoteEventObservation(coroutineScope, adapter, playerLogs, "ended", payload.seq)
                }
            }

            override fun onHeartbeat(serverTimeMs: Long) = Unit

            override fun onError(message: String) {
                coroutineScope.launch {
                    roomBootstrapStatus = "同步错误：$message"
                    appendPlayerLog(playerLogs, roomBootstrapStatus, maxSize = 10)
                }
            }
        }
    }

    LaunchedEffect(activeRoomCode, activeRoomUserId, accessToken) {
        val roomCode = activeRoomCode
        val userId = activeRoomUserId
        if (roomCode.isBlank() || userId.isNullOrBlank() || accessToken.isBlank()) return@LaunchedEffect
        appendPlayerLog(playerLogs, "starting ws roomId=$roomCode userId=$userId", maxSize = 10)
        roomSessionController.startSession(
            roomId = roomCode,
            userId = userId,
            accessToken = accessToken,
            listener = syncListener
        )
    }

    DisposableEffect(adapter, roomSessionController) {
        adapter.setEventListener { event ->
            appendLog(playerLogs, event.toDebugLabel(), maxSize = 10)
            playerState = reducePlayerEvent(playerState, event, adapter)
        }
        onDispose {
            pendingPostSeekCalibrationJob?.cancel()
            adapter.setEventListener(null)
            roomSessionController.closeSession()
            adapter.release()
        }
    }

    LaunchedEffect(activeEpisode.episodeId, activeEpisode.mediaUrl) {
        isMediaLoading = true
        mediaLoadError = null
        playerState = playerState.copy(
            telemetry = playerState.telemetry.beginLoad(SystemClock.elapsedRealtime())
        )
        val mediaUrl = activeEpisode.mediaUrl
        if (mediaUrl.isNullOrBlank()) {
            mediaLoadError = "GET /media/items 未返回 mediaUrl，无法播放。"
            appendLog(playerLogs, "media missing mediaUrl episodeId=${activeEpisode.episodeId}", maxSize = 10)
        } else {
            appendLog(playerLogs, "media selected episodeId=${activeEpisode.episodeId} url=$mediaUrl", maxSize = 10)
            PlayerDebugLog.d(TELEMETRY_LOG_TAG, "load episodeId=${activeEpisode.episodeId} url=$mediaUrl")
            adapter.load(mediaUrl)
        }
        isMediaLoading = false
    }

    LaunchedEffect(adapter) {
        while (isActive) {
            playerState = snapshotFromAdapter(
                adapter = adapter,
                playbackState = playerState.playbackState,
                current = playerState
            )
            delay(500L)
        }
    }

    pendingRoomDeviceSwitchRequest?.let { request ->
        AlertDialog(
            onDismissRequest = {
                roomSessionController.sendRoomDeviceSwitchReply(request.requestId, approve = false)
                pendingRoomDeviceSwitchRequest = null
            },
            title = { Text(text = "切换房间设备") },
            text = { Text(text = "设备 ${request.targetRoomId} 请求接管当前房间。是否允许切换？") },
            confirmButton = {
                TextButton(onClick = {
                    roomSessionController.sendRoomDeviceSwitchReply(request.requestId, approve = true)
                    pendingRoomDeviceSwitchRequest = null
                }) {
                    Text(text = "允许")
                }
            },
            dismissButton = {
                TextButton(onClick = {
                    roomSessionController.sendRoomDeviceSwitchReply(request.requestId, approve = false)
                    pendingRoomDeviceSwitchRequest = null
                }) {
                    Text(text = "拒绝")
                }
            }
        )
    }

    RoomTheaterPage(
        modifier = modifier,
        adapter = adapter,
        playerState = playerState,
        roomCode = activeRoomCode,
        roomRole = activeRoomRole,
        roomStatusLabel = when {
            isMediaLoading -> "正在加载 GET /media/items 返回的媒体..."
            mediaLoadError != null -> mediaLoadError.orEmpty()
            else -> roomHint
        },
        roomMembers = roomMembersForUi(
            roomCode = activeRoomCode,
            roomRole = activeRoomRole,
            activeRoomUserId = activeRoomUserId,
            currentUserId = currentUserId,
            currentUserNickname = currentUserNickname,
            latestRoomState = latestRoomState
        ),
        mediaTitle = activeEpisode.title.ifBlank { "等待媒体选择" },
        mediaSeasonLabel = activeEpisode.seasonLabel,
        mediaEpisodeLabel = activeEpisode.episodeLabel,
        isHostController = activeRoomRole == "host",
        controlsEnabled = canApplyLocalPlaybackControl,
                onPlaybackToggleClick = {
                    if (playerState.isPlaying) {
                        if (!currentIsApplyingRemoteEvent && activeRoomRole == "host") {
                            val sent = roomSessionController.sendPause(
                                positionMs = currentPlayerState.currentPosition,
                                seq = currentLatestRoomState?.seq ?: 0L
                            )
                            appendHostSendLog(playerLogs, roomSessionController, "pause", sent, "pos=${currentPlayerState.currentPosition}")
                        }
                        if (canApplyLocalPlaybackControl) {
                            adapter.pause()
                        } else {
                            appendPlayerLog(playerLogs, "viewer local pause ignored", maxSize = 10)
                        }
                    } else {
                        playerState = playerState.copy(
                            telemetry = playerState.telemetry.markPlayRequested(SystemClock.elapsedRealtime())
                        )
                        if (!currentIsApplyingRemoteEvent && activeRoomRole == "host") {
                            val sent = roomSessionController.sendPlay(
                                positionMs = currentPlayerState.currentPosition,
                                seq = currentLatestRoomState?.seq ?: 0L
                            )
                            appendHostSendLog(playerLogs, roomSessionController, "play", sent, "pos=${currentPlayerState.currentPosition}")
                        }
                        if (canApplyLocalPlaybackControl) {
                            adapter.play()
                        } else {
                            appendPlayerLog(playerLogs, "viewer local play ignored", maxSize = 10)
                        }
                    }
                },
                onSeekBackwardClick = {
                    val target = (currentPlayerState.currentPosition - SeekStepMs).coerceAtLeast(0L)
                    if (!currentIsApplyingRemoteEvent && activeRoomRole == "host") {
                        val sent = roomSessionController.sendSeek(positionMs = target, seq = currentLatestRoomState?.seq ?: 0L)
                        appendHostSendLog(playerLogs, roomSessionController, "seek", sent, "pos=$target")
                        if (sent) {
                            pendingPostSeekCalibrationJob?.cancel()
                            pendingPostSeekCalibrationJob = schedulePostSeekCalibration(
                                coroutineScope = coroutineScope,
                                roomSessionController = roomSessionController,
                                adapter = adapter,
                                latestSeq = { currentLatestRoomState?.seq ?: 0L },
                                logs = playerLogs
                            )
                        }
                    }
                    if (canApplyLocalPlaybackControl) {
                        adapter.seekTo(target)
                    } else {
                        appendPlayerLog(playerLogs, "viewer local seek ignored", maxSize = 10)
                    }
                },
                onSeekForwardClick = {
                    val target = currentPlayerState.currentPosition + SeekStepMs
                    val duration = currentPlayerState.duration.takeIf { it > 0L }
                    val safeTarget = duration?.let { target.coerceAtMost(it) } ?: target
                    if (!currentIsApplyingRemoteEvent && activeRoomRole == "host") {
                        val sent = roomSessionController.sendSeek(positionMs = safeTarget, seq = currentLatestRoomState?.seq ?: 0L)
                        appendHostSendLog(playerLogs, roomSessionController, "seek", sent, "pos=$safeTarget")
                        if (sent) {
                            pendingPostSeekCalibrationJob?.cancel()
                            pendingPostSeekCalibrationJob = schedulePostSeekCalibration(
                                coroutineScope = coroutineScope,
                                roomSessionController = roomSessionController,
                                adapter = adapter,
                                latestSeq = { currentLatestRoomState?.seq ?: 0L },
                                logs = playerLogs
                            )
                        }
                    }
                    if (canApplyLocalPlaybackControl) {
                        adapter.seekTo(safeTarget)
                    } else {
                        appendPlayerLog(playerLogs, "viewer local seek ignored", maxSize = 10)
                    }
                },
                onProgressSeekCommit = { positionMs ->
                    if (!currentIsApplyingRemoteEvent && activeRoomRole == "host") {
                        val sent = roomSessionController.sendSeek(positionMs = positionMs, seq = currentLatestRoomState?.seq ?: 0L)
                        appendHostSendLog(playerLogs, roomSessionController, "seek", sent, "pos=$positionMs")
                        if (sent) {
                            pendingPostSeekCalibrationJob?.cancel()
                            pendingPostSeekCalibrationJob = schedulePostSeekCalibration(
                                coroutineScope = coroutineScope,
                                roomSessionController = roomSessionController,
                                adapter = adapter,
                                latestSeq = { currentLatestRoomState?.seq ?: 0L },
                                logs = playerLogs
                            )
                        }
                    }
                    if (canApplyLocalPlaybackControl) {
                        adapter.seekTo(positionMs)
                    } else {
                        appendPlayerLog(playerLogs, "viewer local seek ignored", maxSize = 10)
                    }
                },
                onPlaybackSpeedChange = { speed ->
                    if (!currentIsApplyingRemoteEvent && activeRoomRole == "host") {
                        val sent = roomSessionController.sendPlaybackRate(
                            playbackRate = speed.toDouble(),
                            positionMs = currentPlayerState.currentPosition,
                            seq = currentLatestRoomState?.seq ?: 0L
                        )
                        appendHostSendLog(
                            playerLogs,
                            roomSessionController,
                            "rate",
                            sent,
                            "speed=$speed pos=${currentPlayerState.currentPosition}"
                        )
                    }
                    if (canApplyLocalPlaybackControl) {
                        playerState = playerState.copy(playbackSpeed = speed)
                        adapter.setPlaybackSpeed(speed)
                    } else {
                        appendPlayerLog(playerLogs, "viewer local rate ignored", maxSize = 10)
                    }
                },
                onVideoQualityPreferenceChange = { preference ->
                    val nowMs = SystemClock.elapsedRealtime()
                    playerState = playerState.copy(
                        videoQualityPreference = preference,
                        videoQualityStatus = if (preference.isAuto) {
                            "自动模式：优先最高可用清晰度，允许 ABR 回落"
                        } else {
                            "手动模式：尝试锁定 ${preference.label}"
                        },
                        telemetry = playerState.telemetry.beginQualitySwitch(preference.height, nowMs)
                    )
                    adapter.setVideoQualityPreference(preference)
                },
        onInviteClick = {}
    )
}

private fun roomBootstrapInitialStatus(
    autoCreateAsHost: Boolean,
    autoJoinAsViewer: Boolean,
    initialRoomCode: String
): String {
    return when {
        autoCreateAsHost -> "等待创建房间"
        autoJoinAsViewer -> "等待加入房间"
        initialRoomCode.isNotBlank() -> "等待加入房间 ${initialRoomCode.trim().uppercase()}"
        else -> "本地播放模式"
    }
}

private fun buildRoomHint(roomCode: String, role: String?, bootstrapStatus: String): String {
    val roomLabel = roomCode.takeIf { it.isNotBlank() }?.let { "房间 $it" }
    val roleLabel = role?.takeIf { it.isNotBlank() }?.let { if (it == "host") "host" else it }
    return listOfNotNull(roomLabel, roleLabel, bootstrapStatus).joinToString(" · ")
        .ifBlank { "同步功能重构中 · 当前仅加载后端媒体播放" }
}

private fun roomMembersForUi(
    roomCode: String,
    roomRole: String?,
    activeRoomUserId: String?,
    currentUserId: String,
    currentUserNickname: String,
    latestRoomState: RoomSyncState?
): List<RoomMember> {
    if (roomCode.isBlank()) return emptyList()
    val hostUserId = latestRoomState?.hostUserId
    val selfUserId = activeRoomUserId ?: currentUserId
    val selfNickname = currentUserNickname.ifBlank {
        if (roomRole == "host") "房主" else "我"
    }
    val members = mutableListOf<RoomMember>()
    if (hostUserId != null) {
        members += RoomMember(
            userId = hostUserId,
            nickname = if (roomRole == "host" && selfUserId == hostUserId) selfNickname else "房主",
            avatarSeed = hostUserId,
            avatarUrl = null,
            role = "host"
        )
    }
    if (roomRole != null && selfUserId.isNotBlank() && members.none { it.userId == selfUserId }) {
        members += RoomMember(
            userId = selfUserId,
            nickname = selfNickname,
            avatarSeed = selfUserId,
            avatarUrl = null,
            role = roomRole
        )
    }
    return members
}

private fun defaultSelectedEpisode(): MediaEpisode {
    return MediaEpisode(
        episodeId = AppConfig.defaultMediaIdForRoom(),
        title = "",
        subtitle = null,
        description = null,
        coverUrl = null,
        mediaUrl = null,
        durationMs = 0,
        seasonLabel = null,
        episodeLabel = null,
        tags = emptyList()
    )
}

private fun RoomMedia.toMediaEpisode(): MediaEpisode {
    return MediaEpisode(
        episodeId = id,
        title = title,
        subtitle = subtitle,
        description = null,
        coverUrl = null,
        mediaUrl = mediaUrl,
        durationMs = durationMs ?: 0L,
        seasonLabel = seasonLabel,
        episodeLabel = episodeLabel,
        tags = emptyList()
    )
}

@Composable
private fun LocalPlaybackDiagnostics(playerState: PlayerRuntimeState, logs: List<String>) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant, MaterialTheme.shapes.medium)
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Text(
            text = "本地播放诊断",
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        Text(
            text = "state=${playerState.playbackState.toPlaybackStateLabel()} · " +
                "pos=${playerState.currentPosition}ms · " +
                "buffer=${playerState.bufferedAheadMs}ms · " +
                "effective=${playerState.effectiveBufferedAheadMs}ms · " +
                "speed=${playerState.playbackSpeed}x · " +
                "selected=${playerState.videoQualityPreference.label} · " +
                "actual=${playerState.videoVariant.displayLabel} · " +
                "qualityStatus=${playerState.videoQualityStatus}",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        Text(
            text = "firstReady=${playerState.telemetry.firstReadyMs?.let { "${it}ms" } ?: "待测"} · " +
                "playStart=${playerState.telemetry.playStartMs?.let { "${it}ms" } ?: "待测"} · " +
                "lastSwitch=${playerState.telemetry.lastQualitySwitchMs?.let { "${it}ms" } ?: "待测"} · " +
                "rebuffer=${playerState.telemetry.rebufferCount}",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        Spacer(Modifier.height(2.dp))
        logs.forEach { line ->
            Text(
                text = line,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

private fun reducePlayerEvent(current: PlayerRuntimeState, event: PlayerEvent, adapter: PlayerAdapter): PlayerRuntimeState {
    val nowMs = SystemClock.elapsedRealtime()
    return when (event) {
        PlayerEvent.Ready -> snapshotFromAdapter(adapter, Player.STATE_READY, current)
            .copy(telemetry = current.telemetry.markReady(nowMs))
        is PlayerEvent.PlaybackStateChanged -> {
            val isPlaybackRebuffer = event.playbackState == Player.STATE_BUFFERING &&
                current.playbackState == Player.STATE_READY &&
                current.currentPosition > 0L
            val telemetry = if (isPlaybackRebuffer) current.telemetry.markRebuffer(nowMs) else current.telemetry
            snapshotFromAdapter(adapter, event.playbackState, current).copy(telemetry = telemetry)
        }
        is PlayerEvent.IsPlayingChanged -> {
            val telemetry = if (event.isPlaying) current.telemetry.markFirstFrame(nowMs) else current.telemetry
            current.copy(isPlaying = event.isPlaying, telemetry = telemetry)
        }
        is PlayerEvent.VideoVariantChanged -> current.copy(
            videoVariant = event.variant,
            telemetry = current.telemetry.markQualitySwitchComplete(event.variant.height, nowMs)
        )
        is PlayerEvent.VideoQualitiesChanged -> current.copy(availableVideoQualities = event.options)
        is PlayerEvent.Error -> snapshotFromAdapter(adapter, current.playbackState, current).copy(statusMessage = event.message)
    }
}

private fun snapshotFromAdapter(adapter: PlayerAdapter, playbackState: Int, current: PlayerRuntimeState): PlayerRuntimeState {
    return current.copy(
        currentPosition = adapter.getCurrentPosition().coerceAtLeast(0L),
        duration = adapter.getDuration().coerceAtLeast(0L),
        bufferedPosition = adapter.getBufferedPosition().coerceAtLeast(0L),
        bufferedPercentage = adapter.getBufferedPercentage().coerceIn(0, 100),
        isPlaying = adapter.isPlaying(),
        playbackState = playbackState
    )
}

private fun appendPlayerLog(logs: MutableList<String>, line: String, maxSize: Int) {
    logs.add(0, line)
    while (logs.size > maxSize) logs.removeAt(logs.lastIndex)
    PlayerDebugLog.d(SYNC_LOG_TAG, line)
}

private fun appendLog(logs: MutableList<String>, line: String, maxSize: Int) {
    appendPlayerLog(logs, line, maxSize)
}

private fun appendHostSendLog(
    logs: MutableList<String>,
    roomSessionController: RoomSessionController,
    action: String,
    sent: Boolean,
    detail: String
) {
    val diagnostics = if (sent) "" else " ${roomSessionController.diagnostics()}"
    appendPlayerLog(logs, "host $action sent=$sent $detail$diagnostics", maxSize = 10)
}

private fun schedulePostSeekCalibration(
    coroutineScope: kotlinx.coroutines.CoroutineScope,
    roomSessionController: RoomSessionController,
    adapter: PlayerAdapter,
    latestSeq: () -> Long,
    logs: MutableList<String>
): Job {
    return coroutineScope.launch {
        delay(PostSeekCalibrationDelayMs)
        val positionMs = adapter.getCurrentPosition().coerceAtLeast(0L)
        val sent = roomSessionController.sendSeek(positionMs = positionMs, seq = latestSeq())
        appendHostSendLog(logs, roomSessionController, "seek-calibration", sent, "pos=$positionMs")
    }
}

private fun appendRemoteEventReceivedLog(
    logs: MutableList<String>,
    adapter: PlayerAdapter,
    action: String,
    seq: Long,
    remotePositionMs: Long
) {
    appendPlayerLog(
        logs,
        "remote $action received seq=$seq remote=$remotePositionMs ${adapterSyncSnapshot(adapter)}",
        maxSize = 10
    )
}

private fun scheduleRemoteEventObservation(
    coroutineScope: kotlinx.coroutines.CoroutineScope,
    adapter: PlayerAdapter,
    logs: MutableList<String>,
    action: String,
    seq: Long
): Job {
    return coroutineScope.launch {
        delay(RemoteEventObservationDelayMs)
        appendPlayerLog(logs, "remote $action after seq=$seq ${adapterSyncSnapshot(adapter)}", maxSize = 10)
    }
}

private fun adapterSyncSnapshot(adapter: PlayerAdapter): String {
    val positionMs = adapter.getCurrentPosition().coerceAtLeast(0L)
    val bufferedPositionMs = adapter.getBufferedPosition().coerceAtLeast(0L)
    val bufferedAheadMs = (bufferedPositionMs - positionMs).coerceAtLeast(0L)
    return "local=$positionMs bufferAhead=${bufferedAheadMs}ms isPlaying=${adapter.isPlaying()}"
}

private fun applyRemoteSeekWithThreshold(
    adapter: PlayerAdapter,
    logs: MutableList<String>,
    payload: SeekPayload,
    coroutineScope: kotlinx.coroutines.CoroutineScope,
    setApplyingRemoteEvent: (Boolean) -> Unit
) {
    val localPositionMs = adapter.getCurrentPosition().coerceAtLeast(0L)
    val driftMs = kotlin.math.abs(localPositionMs - payload.positionMs)
    if (driftMs < RemoteSeekCorrectionThresholdMs) {
        appendPlayerLog(
            logs,
            "remote seek skipped seq=${payload.seq} drift=${driftMs}ms local=$localPositionMs remote=${payload.positionMs}",
            maxSize = 10
        )
        return
    }

    setApplyingRemoteEvent(true)
    adapter.seekTo(payload.positionMs)
    setApplyingRemoteEvent(false)
    appendPlayerLog(
        logs,
        "remote seek applied seq=${payload.seq} drift=${driftMs}ms local=$localPositionMs remote=${payload.positionMs}",
        maxSize = 10
    )
    scheduleRemoteEventObservation(coroutineScope, adapter, logs, "seek", payload.seq)
}

private const val TELEMETRY_LOG_TAG = "WatchTogetherTelemetry"
private const val SYNC_LOG_TAG = "WatchTogetherSync"
private const val SeekStepMs = 2_000L
private const val PostSeekCalibrationDelayMs = 900L
private const val RemoteSeekCorrectionThresholdMs = 700L
private const val RemoteEventObservationDelayMs = 1_200L

@Preview(showBackground = true, showSystemUi = true)
@Composable
private fun RoomTheaterScreenPreview() {
    Watch_togetherTheme {
        Box(Modifier.fillMaxSize()) {
            RoomTheaterScreen()
        }
    }
}
