package com.example.watch_together.ui.player

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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AssistChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.media3.common.Player
import com.example.watch_together.config.AppConfig
import com.example.watch_together.pages.video.MediaEpisode
import com.example.watch_together.ui.theme.Watch_togetherTheme
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive

@Composable
fun PlayerScreen(
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
    val adapter = remember(context.applicationContext) { AndroidExoPlayerAdapter(context.applicationContext) }
    var playerState by remember { mutableStateOf(PlayerRuntimeState()) }
    var mediaLoadError by remember { mutableStateOf<String?>(null) }
    var isMediaLoading by remember { mutableStateOf(false) }
    val playerLogs = remember { mutableStateListOf<String>() }
    val roomHint = when {
        autoCreateAsHost -> "房间创建链路重构中 · 当前仅加载后端媒体播放"
        autoJoinAsViewer -> "加入房间链路重构中 · 当前仅加载后端媒体播放"
        initialRoomCode.isNotBlank() -> "房间 $initialRoomCode 同步重构中 · 当前仅加载后端媒体播放"
        else -> "同步功能重构中 · 当前仅加载后端媒体播放"
    }

    DisposableEffect(adapter) {
        adapter.setEventListener { event ->
            appendLog(playerLogs, event.toDebugLabel(), maxSize = 10)
            playerState = reducePlayerEvent(playerState, event, adapter)
        }
        onDispose {
            adapter.setEventListener(null)
            adapter.release()
        }
    }

    LaunchedEffect(selectedEpisode.episodeId, selectedEpisode.mediaUrl) {
        isMediaLoading = true
        mediaLoadError = null
        playerState = playerState.copy(
            telemetry = playerState.telemetry.beginLoad(SystemClock.elapsedRealtime())
        )
        val mediaUrl = selectedEpisode.mediaUrl
        if (mediaUrl.isNullOrBlank()) {
            mediaLoadError = "GET /media/items 未返回 mediaUrl，无法播放。"
            appendLog(playerLogs, "media list missing mediaUrl episodeId=${selectedEpisode.episodeId}", maxSize = 10)
        } else {
            appendLog(playerLogs, "media list selected episodeId=${selectedEpisode.episodeId} url=$mediaUrl", maxSize = 10)
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

    Surface(modifier = modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .background(MaterialTheme.colorScheme.background)
                .verticalScroll(rememberScrollState())
                .padding(18.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            Text(
                text = "播放器重构 · Phase 2",
                style = MaterialTheme.typography.titleLarge,
                color = MaterialTheme.colorScheme.onBackground
            )
            AssistChip(onClick = {}, label = { Text(roomHint) })
            if (isMediaLoading || mediaLoadError != null) {
                AssistChip(
                    onClick = {},
                    label = { Text(if (isMediaLoading) "正在加载 GET /media/items 返回的媒体..." else mediaLoadError.orEmpty()) }
                )
            }
            PlayerCoreShell(
                modifier = Modifier.fillMaxWidth(),
                adapter = adapter,
                state = playerState,
                mediaTitle = selectedEpisode.title.ifBlank { "等待媒体选择" },
                mediaMeta = listOfNotNull(selectedEpisode.seasonLabel, selectedEpisode.episodeLabel).joinToString(" · ")
                    .ifBlank { selectedEpisode.episodeId },
                controlHint = roomHint,
                onPlaybackToggleClick = {
                    if (playerState.isPlaying) {
                        adapter.pause()
                    } else {
                        playerState = playerState.copy(
                            telemetry = playerState.telemetry.markPlayRequested(SystemClock.elapsedRealtime())
                        )
                        adapter.play()
                    }
                },
                onSeekBackwardClick = {
                    adapter.seekTo((playerState.currentPosition - 10_000L).coerceAtLeast(0L))
                },
                onSeekForwardClick = {
                    val target = playerState.currentPosition + 10_000L
                    val duration = playerState.duration.takeIf { it > 0L }
                    adapter.seekTo(duration?.let { target.coerceAtMost(it) } ?: target)
                },
                onProgressSeekCommit = adapter::seekTo,
                onPlaybackSpeedChange = { speed ->
                    playerState = playerState.copy(playbackSpeed = speed)
                    adapter.setPlaybackSpeed(speed)
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
                }
            )
            if (AppConfig.debugSync) {
                LocalPlaybackDiagnostics(playerState = playerState, logs = playerLogs)
            }
        }
    }
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

private fun appendLog(logs: MutableList<String>, line: String, maxSize: Int) {
    logs.add(0, line)
    while (logs.size > maxSize) logs.removeAt(logs.lastIndex)
}

@Preview(showBackground = true, showSystemUi = true)
@Composable
private fun PlayerScreenPreview() {
    Watch_togetherTheme {
        Box(Modifier.fillMaxSize()) {
            PlayerScreen()
        }
    }
}
