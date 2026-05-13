package com.example.watch_together.ui.player

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import android.content.pm.ActivityInfo
import android.content.res.Configuration
import android.os.Build
import android.view.WindowManager
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.input.pointer.PointerId
import androidx.compose.ui.input.pointer.positionChange
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.compose.ui.window.DialogWindowProvider
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import kotlinx.coroutines.delay
import kotlin.math.abs
import kotlin.math.roundToLong

private val PlayerText = Color(0xFFF9F3FF)
private val PlayerTextMuted = Color(0xC8D7D1E5)
private val PlayerPrimary = Color(0xFFFF82C9)
private val PlayerAccent = Color(0xFF8FE7FF)
private const val FullscreenMenuSpeed = "speed"
private const val FullscreenMenuQuality = "quality"
private const val FullscreenMenuDisplay = "display"
private const val FullscreenSwipeMaxSeekMs = 60_000L
private const val FullscreenControlGestureExclusionDp = 112
private const val FullscreenMenuGestureExclusionDp = 224
private const val FullscreenSwipeActivationDp = 28
private const val FullscreenSwipeMinCommitMs = 1_000L
private const val HorizontalGestureDominanceRatio = 1.35f

private enum class VideoDisplayMode(
    val label: String,
    val description: String,
    val resizeMode: Int
) {
    Fit(
        label = "适应",
        description = "完整显示",
        resizeMode = AspectRatioFrameLayout.RESIZE_MODE_FIT
    ),
    Fill(
        label = "填满",
        description = "铺满屏幕",
        resizeMode = AspectRatioFrameLayout.RESIZE_MODE_ZOOM
    )
}

@Composable
fun PlayerCoreShell(
    adapter: PlayerAdapter,
    state: PlayerRuntimeState,
    mediaTitle: String,
    mediaMeta: String,
    controlHint: String,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onProgressSeekCommit: (Long) -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    modifier: Modifier = Modifier,
    controlsEnabled: Boolean = true,
) {
    val playbackControlsEnabled = controlsEnabled && state.canControlPlayback
    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(14.dp)) {
        PlayerViewport(
            adapter = adapter,
            state = state,
            controlHint = controlHint,
            playbackControlsEnabled = playbackControlsEnabled,
            onPlaybackToggleClick = onPlaybackToggleClick,
            onSeekBackwardClick = onSeekBackwardClick,
            onSeekForwardClick = onSeekForwardClick,
            onProgressSeekCommit = onProgressSeekCommit,
            onPlaybackSpeedChange = onPlaybackSpeedChange,
            onVideoQualityPreferenceChange = onVideoQualityPreferenceChange
        )
        MediaSummary(
            mediaTitle = mediaTitle,
            mediaMeta = mediaMeta,
            currentPosition = state.currentPosition,
            duration = state.duration,
            bufferedPercentage = state.bufferedPercentage,
            seekEnabled = playbackControlsEnabled,
            onProgressSeekCommit = onProgressSeekCommit
        )
    }
}

@Composable
private fun PlayerViewport(
    adapter: PlayerAdapter,
    state: PlayerRuntimeState,
    controlHint: String,
    playbackControlsEnabled: Boolean,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onProgressSeekCommit: (Long) -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit
) {
    val activity = LocalContext.current.findActivity()
    val configuration = LocalConfiguration.current
    var controlsVisible by rememberSaveable { mutableStateOf(true) }
    var settingsVisible by rememberSaveable { mutableStateOf(false) }
    var fullscreenRequested by rememberSaveable { mutableStateOf(false) }
    var fullscreenVisible by rememberSaveable { mutableStateOf(false) }
    var originalOrientation by rememberSaveable { mutableStateOf<Int?>(null) }
    var videoDisplayMode by rememberSaveable { mutableStateOf(VideoDisplayMode.Fit) }
    var interactionTick by rememberSaveable { mutableStateOf(0) }
    var attachGeneration by rememberSaveable { mutableStateOf(0) }

    fun keepControlsVisible() {
        controlsVisible = true
        interactionTick += 1
    }

    fun closeFullscreen() {
        fullscreenRequested = false
        settingsVisible = false
        fullscreenVisible = false
        originalOrientation?.let { activity?.requestedOrientation = it }
        originalOrientation = null
        attachGeneration += 1
        keepControlsVisible()
    }

    fun requestFullscreen() {
        settingsVisible = false
        originalOrientation = originalOrientation ?: activity?.requestedOrientation
        activity?.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
        fullscreenRequested = true
        keepControlsVisible()
    }

    LaunchedEffect(controlsVisible, settingsVisible, interactionTick) {
        if (!controlsVisible || settingsVisible) return@LaunchedEffect
        delay(3_200L)
        controlsVisible = false
    }

    LaunchedEffect(playbackControlsEnabled) {
        if (!playbackControlsEnabled) settingsVisible = false
    }

    LaunchedEffect(fullscreenRequested, configuration.orientation) {
        fullscreenVisible = fullscreenRequested && configuration.orientation == Configuration.ORIENTATION_LANDSCAPE
    }

    PlayerSurface(
        adapter = adapter,
        attachGeneration = attachGeneration,
        state = state,
        controlHint = controlHint,
        videoDisplayMode = videoDisplayMode,
        controlsVisible = controlsVisible,
        fullscreenVisible = false,
        playbackControlsEnabled = playbackControlsEnabled,
        onControlInteraction = { keepControlsVisible() },
        onTap = { keepControlsVisible() },
        onPlaybackToggleClick = {
            keepControlsVisible()
            onPlaybackToggleClick()
        },
        onSeekBackwardClick = {
            keepControlsVisible()
            onSeekBackwardClick()
        },
        onSeekForwardClick = {
            keepControlsVisible()
            onSeekForwardClick()
        },
        onProgressSeekCommit = onProgressSeekCommit,
        onSettingsClick = {
            settingsVisible = true
            keepControlsVisible()
        },
        onFullscreenClick = {
            requestFullscreen()
        },
        onPlaybackSpeedChange = {
            onPlaybackSpeedChange(it)
            keepControlsVisible()
        },
        onVideoQualityPreferenceChange = {
            onVideoQualityPreferenceChange(it)
            keepControlsVisible()
        },
        onVideoDisplayModeChange = { mode ->
            videoDisplayMode = mode
            keepControlsVisible()
        },
        modifier = Modifier
            .fillMaxWidth()
            .aspectRatio(16f / 9f),
        shape = RoundedCornerShape(18.dp)
    )

    if (fullscreenVisible) {
        Dialog(
            onDismissRequest = { closeFullscreen() },
            properties = DialogProperties(
                usePlatformDefaultWidth = false,
                decorFitsSystemWindows = false
            )
        ) {
            FullscreenSystemUiEffect()
            PlayerSurface(
                adapter = adapter,
                attachGeneration = attachGeneration,
                state = state,
                controlHint = controlHint,
                videoDisplayMode = videoDisplayMode,
                controlsVisible = controlsVisible,
                fullscreenVisible = true,
                playbackControlsEnabled = playbackControlsEnabled,
                onControlInteraction = { keepControlsVisible() },
                onTap = {
                    controlsVisible = !controlsVisible
                    interactionTick += 1
                },
                onPlaybackToggleClick = {
                    keepControlsVisible()
                    onPlaybackToggleClick()
                },
                onSeekBackwardClick = {
                    keepControlsVisible()
                    onSeekBackwardClick()
                },
                onSeekForwardClick = {
                    keepControlsVisible()
                    onSeekForwardClick()
                },
                onProgressSeekCommit = {
                    keepControlsVisible()
                    onProgressSeekCommit(it)
                },
                onSettingsClick = { keepControlsVisible() },
                onFullscreenClick = { closeFullscreen() },
                onPlaybackSpeedChange = {
                    onPlaybackSpeedChange(it)
                    keepControlsVisible()
                },
                onVideoQualityPreferenceChange = {
                    onVideoQualityPreferenceChange(it)
                    keepControlsVisible()
                },
                onVideoDisplayModeChange = { mode ->
                    videoDisplayMode = mode
                    keepControlsVisible()
                },
                modifier = Modifier
                    .fillMaxSize()
                    .background(Color.Black),
                shape = RoundedCornerShape(0.dp)
            )
        }
    }

    if (settingsVisible && !fullscreenVisible) {
        PlayerSettingsDrawer(
            playbackSpeed = state.playbackSpeed,
            availableVideoQualities = state.availableVideoQualities,
            videoQualityPreference = state.videoQualityPreference,
            videoDisplayMode = videoDisplayMode,
            enabled = playbackControlsEnabled,
            onDismiss = {
                settingsVisible = false
                keepControlsVisible()
            },
            onPlaybackSpeedChange = { speed ->
                onPlaybackSpeedChange(speed)
                keepControlsVisible()
            },
            onVideoQualityPreferenceChange = { preference ->
                onVideoQualityPreferenceChange(preference)
                keepControlsVisible()
            },
            onVideoDisplayModeChange = { mode ->
                videoDisplayMode = mode
                keepControlsVisible()
            }
        )
    }
}

@Composable
private fun PlayerSurface(
    adapter: PlayerAdapter,
    attachGeneration: Int,
    state: PlayerRuntimeState,
    controlHint: String,
    videoDisplayMode: VideoDisplayMode,
    controlsVisible: Boolean,
    fullscreenVisible: Boolean,
    playbackControlsEnabled: Boolean,
    onControlInteraction: () -> Unit,
    onTap: () -> Unit,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onProgressSeekCommit: (Long) -> Unit,
    onSettingsClick: () -> Unit,
    onFullscreenClick: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    onVideoDisplayModeChange: (VideoDisplayMode) -> Unit,
    modifier: Modifier,
    shape: RoundedCornerShape
) {
    var fullscreenMenu by rememberSaveable { mutableStateOf("") }
    var scrubGestureActive by rememberSaveable { mutableStateOf(false) }
    var scrubStartPositionMs by rememberSaveable { mutableStateOf(0L) }
    var scrubAccumulatedPx by rememberSaveable { mutableStateOf(0f) }
    var scrubTargetPositionMs by rememberSaveable { mutableStateOf(0L) }
    var scrubDeltaMs by rememberSaveable { mutableStateOf(0L) }
    val scrubDurationMs = state.duration.coerceAtLeast(0L)

    fun closeFullscreenMenu() {
        fullscreenMenu = ""
    }

    val gestureModifier = Modifier.pointerInput(fullscreenVisible, playbackControlsEnabled, controlsVisible, scrubDurationMs) {
        awaitEachGesture {
            val down = awaitFirstDown(requireUnconsumed = false)
            val controlStripHeightPx = FullscreenControlGestureExclusionDp.dp.toPx()
            val activationDistancePx = FullscreenSwipeActivationDp.dp.toPx()
            val startsInControlStrip = controlsVisible && down.position.y >= size.height - controlStripHeightPx
            val canScrub = fullscreenVisible && playbackControlsEnabled && scrubDurationMs > 0L && !startsInControlStrip
            val canToggleControls = !fullscreenVisible || !startsInControlStrip
            var pointerId: PointerId = down.id
            var totalX = 0f
            var totalY = 0f
            var gestureResolved = false
            var horizontalScrub = false

            scrubGestureActive = false
            scrubAccumulatedPx = 0f
            scrubDeltaMs = 0L
            scrubStartPositionMs = adapter.getCurrentPosition().coerceIn(0L, scrubDurationMs)
            scrubTargetPositionMs = scrubStartPositionMs

            while (true) {
                val event = awaitPointerEvent()
                val change = event.changes.firstOrNull { it.id == pointerId } ?: event.changes.firstOrNull()
                if (change == null) {
                    scrubGestureActive = false
                    break
                }
                pointerId = change.id

                if (!change.pressed) {
                    if (horizontalScrub && abs(scrubDeltaMs) >= FullscreenSwipeMinCommitMs) {
                        onProgressSeekCommit(scrubTargetPositionMs.coerceIn(0L, scrubDurationMs))
                    } else if (!horizontalScrub && !gestureResolved && canToggleControls) {
                        closeFullscreenMenu()
                        onTap()
                    }
                    scrubGestureActive = false
                    scrubAccumulatedPx = 0f
                    scrubDeltaMs = 0L
                    break
                }

                val delta = change.positionChange()
                totalX += delta.x
                totalY += delta.y

                if (!gestureResolved) {
                    val horizontalDistance = abs(totalX)
                    val verticalDistance = abs(totalY)
                    if (horizontalDistance < activationDistancePx && verticalDistance < activationDistancePx) {
                        continue
                    }
                    gestureResolved = true
                    horizontalScrub = canScrub && horizontalDistance >= verticalDistance * HorizontalGestureDominanceRatio
                    if (horizontalScrub) {
                        closeFullscreenMenu()
                        scrubGestureActive = true
                        scrubAccumulatedPx = totalX
                    } else if (verticalDistance > horizontalDistance) {
                        continue
                    }
                }

                if (horizontalScrub) {
                    change.consume()
                    scrubAccumulatedPx = totalX
                    val widthPx = size.width.coerceAtLeast(1)
                    scrubDeltaMs = ((scrubAccumulatedPx / widthPx) * FullscreenSwipeMaxSeekMs).roundToLong()
                    scrubTargetPositionMs = (scrubStartPositionMs + scrubDeltaMs).coerceIn(0L, scrubDurationMs)
                }
            }
        }
    }
    val surfaceGestureModifier = Modifier.then(gestureModifier)
    val tapOnlyModifier = Modifier.pointerInput(fullscreenVisible, controlsVisible) {
        awaitEachGesture {
            val down = awaitFirstDown(requireUnconsumed = false)
            val controlStripHeightPx = FullscreenControlGestureExclusionDp.dp.toPx()
            val startsInControlStrip = controlsVisible && down.position.y >= size.height - controlStripHeightPx
            var moved = false
            while (true) {
                val event = awaitPointerEvent()
                val change = event.changes.firstOrNull { it.id == down.id } ?: event.changes.firstOrNull()
                if (change == null) break
                val delta = change.positionChange()
                if (abs(delta.x) > 2f || abs(delta.y) > 2f) moved = true
                if (!change.pressed) {
                    if (!moved && (!fullscreenVisible || !startsInControlStrip)) {
                        closeFullscreenMenu()
                        onTap()
                    }
                    break
                }
            }
        }
    }
    val activeGestureModifier = if (fullscreenVisible) {
        surfaceGestureModifier
    } else {
        tapOnlyModifier
    }
    val gestureBottomPadding = when {
        fullscreenVisible && controlsVisible && fullscreenMenu.isNotBlank() -> FullscreenMenuGestureExclusionDp.dp
        fullscreenVisible && controlsVisible -> FullscreenControlGestureExclusionDp.dp
        else -> 0.dp
    }

    Surface(
        modifier = modifier,
        color = Color.Black,
        shape = shape,
        tonalElevation = 0.dp
    ) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(
                    Brush.linearGradient(
                        listOf(Color(0xFF31285C), Color(0xFF172038), Color.Black)
                    )
                )
        ) {
            AndroidView(
                modifier = Modifier.fillMaxSize(),
                factory = { context ->
                    PlayerView(context).apply {
                        useController = false
                        resizeMode = videoDisplayMode.resizeMode
                        setShutterBackgroundColor(android.graphics.Color.BLACK)
                        adapter.attach(this)
                    }
                },
                update = { playerView ->
                    attachGeneration
                    playerView.useController = false
                    playerView.resizeMode = videoDisplayMode.resizeMode
                    adapter.attach(playerView)
                }
            )
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(bottom = gestureBottomPadding)
                    .then(activeGestureModifier)
            )

            if (controlsVisible) {
                QualitySelectorAnchor(
                    selectedPreference = state.videoQualityPreference,
                    currentQualityLabel = state.videoVariant.displayLabel,
                    statusHint = state.videoQualityStatus,
                    modifier = Modifier
                        .align(Alignment.TopEnd)
                        .padding(12.dp)
                )
            }

            if (fullscreenVisible && scrubGestureActive) {
                FullscreenSeekGestureOverlay(
                    targetPositionMs = scrubTargetPositionMs,
                    durationMs = scrubDurationMs,
                    deltaMs = scrubDeltaMs,
                    modifier = Modifier.align(Alignment.Center)
                )
            }

            if (fullscreenVisible && controlsVisible && fullscreenMenu.isNotBlank()) {
                FullscreenFloatingSelectorPanel(
                    menu = fullscreenMenu,
                    playbackSpeed = state.playbackSpeed,
                    availableVideoQualities = state.availableVideoQualities,
                    videoQualityPreference = state.videoQualityPreference,
                    videoDisplayMode = videoDisplayMode,
                    enabled = playbackControlsEnabled,
                    onPlaybackSpeedChange = { speed ->
                        closeFullscreenMenu()
                        onPlaybackSpeedChange(speed)
                    },
                    onVideoQualityPreferenceChange = { preference ->
                        closeFullscreenMenu()
                        onVideoQualityPreferenceChange(preference)
                    },
                    onVideoDisplayModeChange = { mode ->
                        closeFullscreenMenu()
                        onVideoDisplayModeChange(mode)
                    },
                    modifier = Modifier
                        .align(Alignment.BottomEnd)
                        .padding(end = 18.dp, bottom = 96.dp)
                )
            }

            if (controlsVisible) {
                PlayerOverlayControls(
                    state = state,
                    controlHint = controlHint,
                    playbackControlsEnabled = playbackControlsEnabled,
                    fullscreenVisible = fullscreenVisible,
                    fullscreenMenu = fullscreenMenu,
                    onPlaybackToggleClick = onPlaybackToggleClick,
                    onSeekBackwardClick = onSeekBackwardClick,
                    onSeekForwardClick = onSeekForwardClick,
                    onSettingsClick = onSettingsClick,
                    onFullscreenClick = onFullscreenClick,
                    onFullscreenSpeedClick = {
                        onControlInteraction()
                        fullscreenMenu = if (fullscreenMenu == FullscreenMenuSpeed) "" else FullscreenMenuSpeed
                    },
                    onFullscreenQualityClick = {
                        onControlInteraction()
                        fullscreenMenu = if (fullscreenMenu == FullscreenMenuQuality) "" else FullscreenMenuQuality
                    },
                    onFullscreenDisplayClick = {
                        onControlInteraction()
                        fullscreenMenu = if (fullscreenMenu == FullscreenMenuDisplay) "" else FullscreenMenuDisplay
                    },
                    onProgressSeekCommit = onProgressSeekCommit,
                    modifier = Modifier.align(Alignment.BottomCenter)
                )
            }
        }
    }
}

@Composable
private fun QualitySelectorAnchor(
    selectedPreference: PlayerVideoQualityPreference,
    currentQualityLabel: String,
    statusHint: String,
    modifier: Modifier = Modifier
) {
    val targetQualityHint = if (!selectedPreference.isAuto) "目标 · ${selectedPreference.label}" else ""
    val hint = listOf(statusHint, targetQualityHint)
        .filter { it.isNotBlank() }
        .joinToString(" · ")
    Column(
        modifier = modifier,
        horizontalAlignment = Alignment.End,
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        QualityPill(selectedPreference = selectedPreference, currentQualityLabel = currentQualityLabel)
        if (hint.isNotBlank()) {
            Text(
                text = hint,
                color = PlayerTextMuted,
                fontSize = 10.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }
    }
}

@Composable
private fun QualityPill(
    selectedPreference: PlayerVideoQualityPreference,
    currentQualityLabel: String,
    modifier: Modifier = Modifier
) {
    val label = currentQualityLabel.ifBlank { selectedPreference.label }
    val dotColor = if (selectedPreference.isAuto) PlayerAccent else PlayerPrimary
    Surface(
        modifier = modifier,
        color = Color(0xA6121420),
        shape = RoundedCornerShape(999.dp),
        border = BorderStroke(1.dp, Color(0x18FFFFFF))
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Box(
                modifier = Modifier
                    .size(6.dp)
                    .clip(CircleShape)
                    .background(dotColor)
            )
            Text(text = label, color = PlayerText, fontSize = 11.sp, fontWeight = FontWeight.Bold)
        }
    }
}

@Composable
private fun FullscreenSeekGestureOverlay(
    targetPositionMs: Long,
    durationMs: Long,
    deltaMs: Long,
    modifier: Modifier = Modifier
) {
    Surface(
        modifier = modifier,
        color = Color(0xD6141827),
        shape = RoundedCornerShape(24.dp),
        border = BorderStroke(1.dp, Color(0x26FFFFFF))
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 22.dp, vertical = 16.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(
                text = formatSignedDelta(deltaMs),
                color = PlayerText,
                fontSize = 24.sp,
                lineHeight = 28.sp,
                fontWeight = FontWeight.Black
            )
            Text(
                text = "${formatMs(targetPositionMs)} / ${formatMs(durationMs)}",
                color = PlayerTextMuted,
                fontSize = 13.sp,
                fontWeight = FontWeight.Bold
            )
        }
    }
}

@Composable
private fun MediaSummary(
    mediaTitle: String,
    mediaMeta: String,
    currentPosition: Long,
    duration: Long,
    bufferedPercentage: Int,
    seekEnabled: Boolean,
    onProgressSeekCommit: (Long) -> Unit
) {
    var dragging by rememberSaveable { mutableStateOf(false) }
    var dragPositionMs by rememberSaveable { mutableStateOf(0L) }
    val safeDuration = duration.coerceAtLeast(0L)
    val displayPosition = if (dragging) dragPositionMs else currentPosition
    val progress = if (safeDuration > 0L) {
        (displayPosition.toFloat() / safeDuration.toFloat()).coerceIn(0f, 1f)
    } else {
        0f
    }

    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text(
            text = mediaTitle,
            color = PlayerText,
            fontSize = 24.sp,
            lineHeight = 28.sp,
            fontWeight = FontWeight.Black,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
        Text(
            text = "$mediaMeta · ${formatMs(displayPosition)} / ${formatMs(duration)}",
            color = PlayerTextMuted,
            fontSize = 14.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
        SeekProgressBar(
            progress = progress,
            bufferedProgress = bufferedPercentage / 100f,
            enabled = seekEnabled && safeDuration > 0L,
            onProgressChange = { nextProgress ->
                dragging = true
                dragPositionMs = (safeDuration * nextProgress).toLong().coerceIn(0L, safeDuration)
            },
            onProgressCommit = {
                if (dragging && safeDuration > 0L) {
                    onProgressSeekCommit(dragPositionMs.coerceIn(0L, safeDuration))
                }
                dragging = false
            }
        )
    }
}

@Composable
@OptIn(ExperimentalMaterial3Api::class)
private fun SeekProgressBar(
    progress: Float,
    bufferedProgress: Float,
    enabled: Boolean,
    onProgressChange: (Float) -> Unit,
    onProgressCommit: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = SliderDefaults.colors(
        thumbColor = PlayerText,
        activeTrackColor = PlayerPrimary,
        inactiveTrackColor = Color(0x22FFFFFF),
        disabledThumbColor = PlayerTextMuted.copy(alpha = 0.55f),
        disabledActiveTrackColor = PlayerPrimary.copy(alpha = 0.45f),
        disabledInactiveTrackColor = Color(0x18FFFFFF)
    )
    Slider(
        value = progress.coerceIn(0f, 1f),
        onValueChange = { value -> onProgressChange(value.coerceIn(0f, 1f)) },
        onValueChangeFinished = onProgressCommit,
        enabled = enabled,
        colors = colors,
        modifier = modifier
            .fillMaxWidth()
            .height(26.dp),
        thumb = {
            Box(
                modifier = Modifier
                    .size(if (enabled) 14.dp else 10.dp)
                    .clip(CircleShape)
                    .background(Brush.radialGradient(listOf(PlayerText, PlayerPrimary)))
            )
        },
        track = { sliderState ->
            val activeProgress = sliderState.value.coerceIn(0f, 1f)
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(8.dp)
                    .clip(RoundedCornerShape(999.dp))
                    .background(Color(0x22FFFFFF))
            ) {
                Box(
                    modifier = Modifier
                        .fillMaxWidth(bufferedProgress.coerceIn(0f, 1f))
                        .height(8.dp)
                        .clip(RoundedCornerShape(999.dp))
                        .background(Color(0x33FFFFFF))
                )
                Box(
                    modifier = Modifier
                        .fillMaxWidth(activeProgress)
                        .height(8.dp)
                        .clip(RoundedCornerShape(999.dp))
                        .background(Brush.horizontalGradient(listOf(PlayerPrimary, PlayerAccent)))
                )
            }
        }
    )
}

@Composable
private fun PlayerOverlayControls(
    state: PlayerRuntimeState,
    controlHint: String,
    playbackControlsEnabled: Boolean,
    fullscreenVisible: Boolean,
    fullscreenMenu: String,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onSettingsClick: () -> Unit,
    onFullscreenClick: () -> Unit,
    onFullscreenSpeedClick: () -> Unit,
    onFullscreenQualityClick: () -> Unit,
    onFullscreenDisplayClick: () -> Unit,
    onProgressSeekCommit: (Long) -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(Brush.verticalGradient(listOf(Color.Transparent, Color(0xF2090A11))))
            .padding(horizontal = 12.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(9.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(16.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(7.dp), verticalAlignment = Alignment.CenterVertically) {
                FullscreenIconButton(fullscreenVisible = fullscreenVisible, onClick = onFullscreenClick)
                PlayPauseIconButton(
                    isPlaying = state.isPlaying,
                    enabled = playbackControlsEnabled,
                    onClick = onPlaybackToggleClick
                )
                OverlayControlChip(label = "-2s", enabled = playbackControlsEnabled, onClick = onSeekBackwardClick)
                OverlayControlChip(label = "+2s", enabled = playbackControlsEnabled, onClick = onSeekForwardClick)
            }

            if (fullscreenVisible) {
                FullscreenControlProgressBar(
                    state = state,
                    enabled = playbackControlsEnabled,
                    onProgressSeekCommit = onProgressSeekCommit,
                    modifier = Modifier.weight(1f)
                )
            } else {
                Spacer(Modifier.weight(1f))
            }

            if (fullscreenVisible) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                    FullscreenBottomMenuChip(
                        label = state.playbackSpeed.toSpeedLabel(),
                        selected = fullscreenMenu == FullscreenMenuSpeed,
                        enabled = playbackControlsEnabled,
                        onClick = onFullscreenSpeedClick
                    )
                    FullscreenBottomMenuChip(
                        label = if (state.videoQualityPreference.isAuto) "自动" else state.videoQualityPreference.label,
                        selected = fullscreenMenu == FullscreenMenuQuality,
                        enabled = playbackControlsEnabled,
                        onClick = onFullscreenQualityClick
                    )
                    FullscreenBottomMenuChip(
                        label = "画面",
                        selected = fullscreenMenu == FullscreenMenuDisplay,
                        enabled = playbackControlsEnabled,
                        onClick = onFullscreenDisplayClick
                    )
                }
            } else {
                SettingsIconButton(enabled = playbackControlsEnabled, onClick = onSettingsClick)
            }
        }
        Text(
            text = state.videoQualityStatus.ifBlank { controlHint },
            color = PlayerTextMuted,
            fontSize = 11.sp,
            lineHeight = 15.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

@Composable
private fun FullscreenControlProgressBar(
    state: PlayerRuntimeState,
    enabled: Boolean,
    onProgressSeekCommit: (Long) -> Unit,
    modifier: Modifier = Modifier
) {
    var dragging by rememberSaveable { mutableStateOf(false) }
    var dragPositionMs by rememberSaveable { mutableStateOf(0L) }
    val safeDuration = state.duration.coerceAtLeast(0L)
    val displayPosition = if (dragging) dragPositionMs else state.currentPosition
    val progress = if (safeDuration > 0L) {
        (displayPosition.toFloat() / safeDuration.toFloat()).coerceIn(0f, 1f)
    } else {
        0f
    }

    Row(
        modifier = modifier,
        horizontalArrangement = Arrangement.spacedBy(9.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = formatMs(displayPosition),
            color = PlayerTextMuted,
            fontSize = 11.sp,
            fontWeight = FontWeight.Bold,
            maxLines = 1
        )
        SeekProgressBar(
            progress = progress,
            bufferedProgress = state.bufferedPercentage / 100f,
            enabled = enabled && safeDuration > 0L,
            onProgressChange = { nextProgress ->
                dragging = true
                dragPositionMs = (safeDuration * nextProgress).toLong().coerceIn(0L, safeDuration)
            },
            onProgressCommit = {
                if (dragging && safeDuration > 0L) {
                    onProgressSeekCommit(dragPositionMs.coerceIn(0L, safeDuration))
                }
                dragging = false
            },
            modifier = Modifier.weight(1f)
        )
        Text(
            text = formatMs(safeDuration),
            color = PlayerTextMuted,
            fontSize = 11.sp,
            fontWeight = FontWeight.Bold,
            maxLines = 1
        )
    }
}

@Composable
private fun PlayerSettingsDrawer(
    playbackSpeed: Float,
    availableVideoQualities: List<PlayerVideoQualityOption>,
    videoQualityPreference: PlayerVideoQualityPreference,
    videoDisplayMode: VideoDisplayMode,
    enabled: Boolean,
    onDismiss: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    onVideoDisplayModeChange: (VideoDisplayMode) -> Unit
) {
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(Color(0x66000000))
                .clickable { onDismiss() }
        ) {
            Surface(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .fillMaxWidth()
                    .clickable { },
                color = Color(0xFFF7F4FA),
                shape = RoundedCornerShape(topStart = 24.dp, topEnd = 24.dp)
            ) {
                Column(
                    modifier = Modifier.padding(horizontal = 18.dp, vertical = 14.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .align(Alignment.CenterHorizontally)
                            .width(44.dp)
                            .height(4.dp)
                            .clip(RoundedCornerShape(999.dp))
                            .background(Color(0x33000000))
                    )
                    PlayerSettingsRow(
                        title = "倍速",
                        options = listOf("0.75", "1.0", "1.25", "1.5", "2.0"),
                        selected = playbackSpeed.toSpeedLabel(),
                        enabled = enabled,
                        onSelect = { label -> onPlaybackSpeedChange(label.toFloat()) }
                    )
                    PlayerSettingsDivider()
                    PlayerSettingsRow(
                        title = "画面",
                        options = VideoDisplayMode.entries.map { it.label },
                        selected = videoDisplayMode.label,
                        enabled = enabled,
                        onSelect = { label ->
                            VideoDisplayMode.entries
                                .firstOrNull { mode -> mode.label == label }
                                ?.let(onVideoDisplayModeChange)
                        }
                    )
                    Text(
                        text = "${videoDisplayMode.label}：${videoDisplayMode.description}。填满会铺满屏幕，但可能裁切少量画面。",
                        color = Color(0xFF8B8494),
                        fontSize = 11.sp,
                        lineHeight = 15.sp
                    )
                    PlayerSettingsDivider()
                    PlayerSettingsRow(
                        title = "清晰度",
                        options = availableVideoQualities.ifEmpty { listOf(PlayerVideoQualityOption.Auto) }
                            .map { it.settingsLabel() },
                        selected = if (videoQualityPreference.isAuto) {
                            PlayerVideoQualityOption.Auto.settingsLabel()
                        } else {
                            videoQualityPreference.label
                        },
                        enabled = enabled,
                        onSelect = { label ->
                            val preference = availableVideoQualities
                                .firstOrNull { option -> option.settingsLabel() == label }
                                ?.let { option -> PlayerVideoQualityPreference(option.height) }
                                ?: PlayerVideoQualityPreference.Auto
                            onVideoQualityPreferenceChange(preference)
                        }
                    )
                    Text(
                        text = "清晰度只影响本机播放；同步观影不会强制其他设备使用同一档位。",
                        color = Color(0xFF8B8494),
                        fontSize = 11.sp,
                        lineHeight = 15.sp
                    )
                }
            }
        }
    }
}

@Composable
private fun PlayerSettingsRow(
    title: String,
    options: List<String>,
    selected: String,
    enabled: Boolean,
    onSelect: (String) -> Unit
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(14.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = title,
            color = Color(0xFF17141F),
            fontSize = 16.sp,
            fontWeight = FontWeight.Bold,
            modifier = Modifier.width(58.dp)
        )
        Row(
            modifier = Modifier.weight(1f),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            options.forEach { option ->
                PlayerSettingsTextOption(
                    label = option,
                    selected = option == selected,
                    enabled = enabled,
                    onClick = { onSelect(option) }
                )
            }
        }
    }
}

@Composable
private fun PlayerSettingsTextOption(
    label: String,
    selected: Boolean,
    enabled: Boolean,
    onClick: () -> Unit
) {
    Surface(color = Color.Transparent, shape = RoundedCornerShape(999.dp), onClick = { if (enabled) onClick() }) {
        Text(
            text = label,
            color = when {
                !enabled -> Color(0x558B8494)
                selected -> PlayerPrimary
                else -> Color(0xFF8B8494)
            },
            fontSize = 15.sp,
            fontWeight = if (selected) FontWeight.Black else FontWeight.Bold,
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 8.dp),
            maxLines = 1
        )
    }
}

@Composable
private fun PlayerSettingsDivider() {
    Spacer(
        modifier = Modifier
            .fillMaxWidth()
            .height(1.dp)
            .background(Color(0x11000000))
    )
}

@Composable
private fun FullscreenFloatingSelectorPanel(
    menu: String,
    playbackSpeed: Float,
    availableVideoQualities: List<PlayerVideoQualityOption>,
    videoQualityPreference: PlayerVideoQualityPreference,
    videoDisplayMode: VideoDisplayMode,
    enabled: Boolean,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    onVideoDisplayModeChange: (VideoDisplayMode) -> Unit,
    modifier: Modifier = Modifier
) {
    val scrollState = rememberScrollState()
    val title = when (menu) {
        FullscreenMenuSpeed -> "倍速"
        FullscreenMenuQuality -> "清晰度"
        else -> "画面"
    }
    Surface(
        modifier = modifier,
        color = Color(0xD6141827),
        shape = RoundedCornerShape(22.dp),
        border = BorderStroke(1.dp, Color(0x26FFFFFF))
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 10.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
            horizontalAlignment = Alignment.End
        ) {
            Text(text = title, color = PlayerTextMuted, fontSize = 11.sp, fontWeight = FontWeight.Black, maxLines = 1)
            Row(
                modifier = Modifier.horizontalScroll(scrollState),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                when (menu) {
                    FullscreenMenuSpeed -> {
                        listOf(0.75f, 1.0f, 1.25f, 1.5f, 2.0f).forEach { speed ->
                            FullscreenMenuOptionChip(
                                label = speed.toSpeedLabel(),
                                selected = playbackSpeed.toSpeedLabel() == speed.toSpeedLabel(),
                                enabled = enabled,
                                onClick = { onPlaybackSpeedChange(speed) }
                            )
                        }
                    }
                    FullscreenMenuQuality -> {
                        availableVideoQualities.ifEmpty { listOf(PlayerVideoQualityOption.Auto) }.forEach { option ->
                            val preference = PlayerVideoQualityPreference(option.height)
                            FullscreenMenuOptionChip(
                                label = option.settingsLabel(),
                                selected = if (preference.isAuto) videoQualityPreference.isAuto else videoQualityPreference.height == preference.height,
                                enabled = enabled,
                                onClick = { onVideoQualityPreferenceChange(preference) }
                            )
                        }
                    }
                    FullscreenMenuDisplay -> {
                        VideoDisplayMode.entries.forEach { mode ->
                            FullscreenMenuOptionChip(
                                label = mode.label,
                                selected = videoDisplayMode == mode,
                                enabled = enabled,
                                onClick = { onVideoDisplayModeChange(mode) }
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun FullscreenBottomMenuChip(
    label: String,
    selected: Boolean,
    enabled: Boolean,
    onClick: () -> Unit
) {
    Surface(
        color = if (selected) Color(0xCC3A2946) else Color(0x80121420),
        shape = RoundedCornerShape(999.dp),
        border = BorderStroke(1.dp, if (selected) PlayerPrimary.copy(alpha = 0.9f) else Color(0x26FFFFFF)),
        onClick = { if (enabled) onClick() }
    ) {
        Text(
            text = label,
            color = if (enabled) PlayerText else PlayerTextMuted.copy(alpha = 0.45f),
            fontSize = 12.sp,
            fontWeight = FontWeight.Black,
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
            maxLines = 1
        )
    }
}

@Composable
private fun FullscreenMenuOptionChip(
    label: String,
    selected: Boolean,
    enabled: Boolean,
    onClick: () -> Unit
) {
    Surface(
        color = if (selected) PlayerPrimary.copy(alpha = 0.24f) else Color(0x33121420),
        shape = RoundedCornerShape(999.dp),
        border = BorderStroke(1.dp, if (selected) PlayerPrimary.copy(alpha = 0.9f) else Color(0x22FFFFFF)),
        onClick = { if (enabled) onClick() }
    ) {
        Text(
            text = label,
            color = when {
                !enabled -> PlayerTextMuted.copy(alpha = 0.45f)
                selected -> PlayerText
                else -> PlayerTextMuted
            },
            fontSize = 12.sp,
            fontWeight = if (selected) FontWeight.Black else FontWeight.Bold,
            modifier = Modifier.padding(horizontal = 11.dp, vertical = 7.dp),
            maxLines = 1
        )
    }
}

@Composable
private fun FullscreenIconButton(fullscreenVisible: Boolean, onClick: () -> Unit) {
    Surface(modifier = Modifier.size(40.dp), color = Color.Transparent, shape = CircleShape, onClick = onClick) {
        FullscreenGlyph(
            fullscreenVisible = fullscreenVisible,
            modifier = Modifier
                .fillMaxSize()
                .padding(10.dp)
        )
    }
}

@Composable
private fun FullscreenGlyph(fullscreenVisible: Boolean, modifier: Modifier = Modifier) {
    Canvas(modifier = modifier) {
        val strokeWidth = 2.2.dp.toPx()
        val segment = size.minDimension * 0.34f
        val inset = strokeWidth / 2
        val stroke = Stroke(width = strokeWidth, cap = StrokeCap.Square)
        val lineColor = PlayerText

        if (fullscreenVisible) {
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(inset, segment), androidx.compose.ui.geometry.Offset(segment, segment), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(segment, inset), androidx.compose.ui.geometry.Offset(segment, segment), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(size.width - segment, segment), androidx.compose.ui.geometry.Offset(size.width - inset, segment), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(size.width - segment, inset), androidx.compose.ui.geometry.Offset(size.width - segment, segment), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(segment, size.height - segment), androidx.compose.ui.geometry.Offset(segment, size.height - inset), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(inset, size.height - segment), androidx.compose.ui.geometry.Offset(segment, size.height - segment), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(size.width - segment, size.height - segment), androidx.compose.ui.geometry.Offset(size.width - segment, size.height - inset), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(size.width - segment, size.height - segment), androidx.compose.ui.geometry.Offset(size.width - inset, size.height - segment), stroke.width, stroke.cap)
        } else {
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(inset, inset), androidx.compose.ui.geometry.Offset(segment, inset), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(inset, inset), androidx.compose.ui.geometry.Offset(inset, segment), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(size.width - segment, inset), androidx.compose.ui.geometry.Offset(size.width - inset, inset), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(size.width - inset, inset), androidx.compose.ui.geometry.Offset(size.width - inset, segment), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(inset, size.height - segment), androidx.compose.ui.geometry.Offset(inset, size.height - inset), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(inset, size.height - inset), androidx.compose.ui.geometry.Offset(segment, size.height - inset), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(size.width - inset, size.height - segment), androidx.compose.ui.geometry.Offset(size.width - inset, size.height - inset), stroke.width, stroke.cap)
            drawLine(lineColor, androidx.compose.ui.geometry.Offset(size.width - segment, size.height - inset), androidx.compose.ui.geometry.Offset(size.width - inset, size.height - inset), stroke.width, stroke.cap)
        }
    }
}

@Composable
private fun PlayPauseIconButton(isPlaying: Boolean, enabled: Boolean, onClick: () -> Unit) {
    val icon = if (isPlaying) "Ⅱ" else "▶"
    Surface(modifier = Modifier.size(38.dp), color = Color.Transparent, shape = CircleShape, onClick = { if (enabled) onClick() }) {
        Box(contentAlignment = Alignment.Center) {
            Text(
                text = icon,
                color = if (enabled) PlayerText else PlayerTextMuted.copy(alpha = 0.55f),
                fontSize = if (isPlaying) 19.sp else 18.sp,
                fontWeight = FontWeight.Black
            )
        }
    }
}

@Composable
private fun OverlayControlChip(label: String, enabled: Boolean, onClick: () -> Unit) {
    Surface(
        color = if (enabled) Color(0x24FFFFFF) else Color(0x18FFFFFF),
        shape = RoundedCornerShape(999.dp),
        onClick = { if (enabled) onClick() }
    ) {
        Text(
            text = label,
            color = if (enabled) PlayerText else PlayerTextMuted.copy(alpha = 0.55f),
            fontSize = 12.sp,
            fontWeight = FontWeight.Bold,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 9.dp)
        )
    }
}

@Composable
private fun SettingsIconButton(enabled: Boolean, onClick: () -> Unit) {
    Surface(modifier = Modifier.size(38.dp), color = Color.Transparent, shape = CircleShape, onClick = { if (enabled) onClick() }) {
        Box(contentAlignment = Alignment.Center) {
            Text(
                text = "⋮",
                color = if (enabled) PlayerText else PlayerTextMuted.copy(alpha = 0.55f),
                fontSize = 24.sp,
                lineHeight = 24.sp,
                fontWeight = FontWeight.Bold
            )
        }
    }
}

@Composable
@Suppress("DEPRECATION")
private fun FullscreenSystemUiEffect() {
    val view = LocalView.current
    DisposableEffect(view) {
        val window = (view.parent as? DialogWindowProvider)?.window
        if (window == null) {
            onDispose { }
        } else {
            WindowCompat.setDecorFitsSystemWindows(window, false)
            val originalStatusBarColor = window.statusBarColor
            val originalNavigationBarColor = window.navigationBarColor
            window.statusBarColor = android.graphics.Color.TRANSPARENT
            window.navigationBarColor = android.graphics.Color.TRANSPARENT
            val originalCutoutMode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                window.attributes.layoutInDisplayCutoutMode
            } else {
                WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_DEFAULT
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                window.attributes = window.attributes.apply {
                    layoutInDisplayCutoutMode = fullscreenCutoutMode()
                }
            }
            val controller = WindowCompat.getInsetsController(window, window.decorView)
            controller.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            controller.hide(WindowInsetsCompat.Type.systemBars())

            onDispose {
                controller.show(WindowInsetsCompat.Type.systemBars())
                WindowCompat.setDecorFitsSystemWindows(window, true)
                window.statusBarColor = originalStatusBarColor
                window.navigationBarColor = originalNavigationBarColor
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                    window.attributes = window.attributes.apply { layoutInDisplayCutoutMode = originalCutoutMode }
                }
            }
        }
    }
}

private fun formatMs(value: Long): String {
    if (value <= 0L) return "00:00"
    val totalSeconds = value / 1_000L
    val minutes = totalSeconds / 60
    val seconds = totalSeconds % 60
    return "%02d:%02d".format(minutes, seconds)
}

private fun formatSignedDelta(valueMs: Long): String {
    val absSeconds = kotlin.math.abs(valueMs) / 1_000L
    val sign = when {
        valueMs > 0L -> "+"
        valueMs < 0L -> "-"
        else -> ""
    }
    return "$sign${absSeconds}s"
}

private fun Float.toSpeedLabel(): String {
    return if (this % 1f == 0f) "%.1f".format(this) else this.toString()
}

private fun PlayerVideoQualityOption.settingsLabel(): String {
    return if (isAuto) "自动" else label
}

private fun fullscreenCutoutMode(): Int {
    return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
        WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_ALWAYS
    } else {
        WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES
    }
}

private tailrec fun Context.findActivity(): Activity? {
    return when (this) {
        is Activity -> this
        is ContextWrapper -> baseContext.findActivity()
        else -> null
    }
}
