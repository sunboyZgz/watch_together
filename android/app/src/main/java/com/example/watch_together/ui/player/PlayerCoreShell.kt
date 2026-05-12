package com.example.watch_together.ui.player

import android.os.Build
import android.view.LayoutInflater
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.WindowManager
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
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
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Surface
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogWindowProvider
import androidx.compose.ui.window.DialogProperties
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import com.example.watch_together.R
import kotlinx.coroutines.delay

private val PlayerText = Color(0xFFF9F3FF)
private val PlayerTextMuted = Color(0xC8D7D1E5)
private val PlayerPrimary = Color(0xFFFF82C9)
private val PlayerAccent = Color(0xFF8FE7FF)
private val PlayerOutline = Color(0x22FFFFFF)
private const val FullscreenMenuSpeed = "speed"
private const val FullscreenMenuQuality = "quality"

@Composable
internal fun PlayerCoreShell(
    adapter: PlayerAdapter,
    mediaTitle: String,
    mediaMeta: String,
    currentPosition: Long,
    duration: Long,
    isPlaying: Boolean,
    playbackSpeed: Float,
    videoQualityLabel: String,
    availableVideoQualities: List<PlayerVideoQualityOption>,
    videoQualityPreference: PlayerVideoQualityPreference,
    videoQualitySwitchState: PlayerVideoQualitySwitchState,
    videoQualityNotice: String,
    controlHint: String,
    playbackButtonEnabled: Boolean,
    secondaryControlsEnabled: Boolean,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onProgressSeekCommit: (Long) -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    modifier: Modifier = Modifier,
    compactWidth: Boolean = false
) {
    Column(
        modifier = modifier,
        verticalArrangement = Arrangement.spacedBy(if (compactWidth) 12.dp else 14.dp)
    ) {
        PlayerViewport(
            adapter = adapter,
            isPlaying = isPlaying,
            playbackSpeed = playbackSpeed,
            videoQualityLabel = videoQualityLabel,
            availableVideoQualities = availableVideoQualities,
            videoQualityPreference = videoQualityPreference,
            videoQualitySwitchState = videoQualitySwitchState,
            videoQualityNotice = videoQualityNotice,
            controlHint = controlHint,
            playbackButtonEnabled = playbackButtonEnabled,
            secondaryControlsEnabled = secondaryControlsEnabled,
            onPlaybackToggleClick = onPlaybackToggleClick,
            onSeekBackwardClick = onSeekBackwardClick,
            onSeekForwardClick = onSeekForwardClick,
            onPlaybackSpeedChange = onPlaybackSpeedChange,
            onVideoQualityPreferenceChange = onVideoQualityPreferenceChange
        )
        MediaSummary(
            mediaTitle = mediaTitle,
            mediaMeta = mediaMeta,
            currentPosition = currentPosition,
            duration = duration,
            seekEnabled = secondaryControlsEnabled,
            onProgressSeekCommit = onProgressSeekCommit
        )
    }
}

@Composable
private fun PlayerViewport(
    adapter: PlayerAdapter,
    isPlaying: Boolean,
    playbackSpeed: Float,
    videoQualityLabel: String,
    availableVideoQualities: List<PlayerVideoQualityOption>,
    videoQualityPreference: PlayerVideoQualityPreference,
    videoQualitySwitchState: PlayerVideoQualitySwitchState,
    videoQualityNotice: String,
    controlHint: String,
    playbackButtonEnabled: Boolean,
    secondaryControlsEnabled: Boolean,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit
) {
    var controlsVisible by rememberSaveable { mutableStateOf(false) }
    var settingsVisible by rememberSaveable { mutableStateOf(false) }
    var fullscreenVisible by rememberSaveable { mutableStateOf(false) }
    var interactionTick by rememberSaveable { mutableStateOf(0) }
    var attachGeneration by rememberSaveable { mutableStateOf(0) }

    fun keepControlsVisible() {
        controlsVisible = true
        interactionTick += 1
    }

    fun closeFullscreen() {
        fullscreenVisible = false
        settingsVisible = false
        attachGeneration += 1
        keepControlsVisible()
    }

    LaunchedEffect(controlsVisible, settingsVisible, interactionTick) {
        if (!controlsVisible) return@LaunchedEffect
        if (settingsVisible) return@LaunchedEffect
        delay(3_200L)
        controlsVisible = false
    }

    LaunchedEffect(secondaryControlsEnabled) {
        if (!secondaryControlsEnabled) {
            settingsVisible = false
        }
    }

    LaunchedEffect(fullscreenVisible) {
        if (fullscreenVisible) {
            settingsVisible = false
        }
    }

    PlayerSurface(
        adapter = adapter,
        attachGeneration = attachGeneration,
        isPlaying = isPlaying,
        playbackSpeed = playbackSpeed,
        videoQualityLabel = videoQualityLabel,
        availableVideoQualities = availableVideoQualities,
        videoQualityPreference = videoQualityPreference,
        videoQualitySwitchState = videoQualitySwitchState,
        videoQualityNotice = videoQualityNotice,
        controlHint = controlHint,
        playbackButtonEnabled = playbackButtonEnabled,
        secondaryControlsEnabled = secondaryControlsEnabled,
        controlsVisible = controlsVisible,
        fullscreenVisible = false,
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
        onSettingsClick = {
            keepControlsVisible()
            settingsVisible = true
        },
        onFullscreenClick = {
            settingsVisible = false
            fullscreenVisible = true
            keepControlsVisible()
        },
        onPlaybackSpeedChange = {
            onPlaybackSpeedChange(it)
            keepControlsVisible()
        },
        onVideoQualityPreferenceChange = {
            onVideoQualityPreferenceChange(it)
            keepControlsVisible()
        },
        modifier = Modifier
            .fillMaxWidth()
            .aspectRatio(16f / 9f),
        shape = RoundedCornerShape(14.dp)
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
                isPlaying = isPlaying,
                playbackSpeed = playbackSpeed,
                videoQualityLabel = videoQualityLabel,
                availableVideoQualities = availableVideoQualities,
                videoQualityPreference = videoQualityPreference,
                videoQualitySwitchState = videoQualitySwitchState,
                videoQualityNotice = videoQualityNotice,
                controlHint = controlHint,
                playbackButtonEnabled = playbackButtonEnabled,
                secondaryControlsEnabled = secondaryControlsEnabled,
                controlsVisible = true,
                fullscreenVisible = true,
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
                onSettingsClick = {
                    keepControlsVisible()
                },
                onFullscreenClick = {
                    closeFullscreen()
                },
                onPlaybackSpeedChange = {
                    onPlaybackSpeedChange(it)
                    keepControlsVisible()
                },
                onVideoQualityPreferenceChange = {
                    onVideoQualityPreferenceChange(it)
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
            playbackSpeed = playbackSpeed,
            availableVideoQualities = availableVideoQualities,
            videoQualityPreference = videoQualityPreference,
            secondaryControlsEnabled = secondaryControlsEnabled,
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
            }
        )
    }
}

@Composable
private fun PlayerSurface(
    adapter: PlayerAdapter,
    attachGeneration: Int,
    isPlaying: Boolean,
    playbackSpeed: Float,
    videoQualityLabel: String,
    availableVideoQualities: List<PlayerVideoQualityOption>,
    videoQualityPreference: PlayerVideoQualityPreference,
    videoQualitySwitchState: PlayerVideoQualitySwitchState,
    videoQualityNotice: String,
    controlHint: String,
    playbackButtonEnabled: Boolean,
    secondaryControlsEnabled: Boolean,
    controlsVisible: Boolean,
    fullscreenVisible: Boolean,
    onTap: () -> Unit,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onSettingsClick: () -> Unit,
    onFullscreenClick: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    modifier: Modifier,
    shape: RoundedCornerShape
) {
    var fullscreenMenu by rememberSaveable { mutableStateOf("") }

    fun closeFullscreenMenu() {
        fullscreenMenu = ""
    }

    Surface(
        modifier = Modifier
            .then(modifier),
        color = Color.Black,
        shape = shape,
        tonalElevation = 0.dp
    ) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .clickable {
                    closeFullscreenMenu()
                    onTap()
                }
                .background(
                    brush = Brush.linearGradient(
                        listOf(
                            Color(0xFF3A376F),
                            Color(0xFF172038),
                            Color.Black
                        )
                    )
                )
        ) {
            AndroidView(
                factory = { context ->
                    (LayoutInflater.from(context)
                        .inflate(R.layout.view_watch_together_player, null) as PlayerView).apply {
                        layoutParams = android.view.ViewGroup.LayoutParams(MATCH_PARENT, MATCH_PARENT)
                        useController = false
                        resizeMode = if (fullscreenVisible) {
                            AspectRatioFrameLayout.RESIZE_MODE_ZOOM
                        } else {
                            AspectRatioFrameLayout.RESIZE_MODE_FIT
                        }
                        adapter.attach(this)
                    }
                },
                modifier = Modifier.fillMaxSize(),
                update = { playerView ->
                    attachGeneration
                    playerView.useController = false
                    playerView.resizeMode = if (fullscreenVisible) {
                        AspectRatioFrameLayout.RESIZE_MODE_ZOOM
                    } else {
                        AspectRatioFrameLayout.RESIZE_MODE_FIT
                    }
                    adapter.attach(playerView)
                }
            )

            if (fullscreenVisible && controlsVisible && fullscreenMenu.isNotBlank()) {
                FullscreenFloatingSelectorPanel(
                    menu = fullscreenMenu,
                    playbackSpeed = playbackSpeed,
                    availableVideoQualities = availableVideoQualities,
                    videoQualityPreference = videoQualityPreference,
                    secondaryControlsEnabled = secondaryControlsEnabled,
                    onPlaybackSpeedChange = { speed ->
                        closeFullscreenMenu()
                        onPlaybackSpeedChange(speed)
                    },
                    onVideoQualityPreferenceChange = { preference ->
                        closeFullscreenMenu()
                        onVideoQualityPreferenceChange(preference)
                    },
                    modifier = Modifier
                        .align(Alignment.BottomEnd)
                        .padding(end = 18.dp, bottom = 96.dp)
                )
            }

            if (controlsVisible || videoQualitySwitchState.isPending) {
                QualitySelectorAnchor(
                    selectedPreference = videoQualityPreference,
                    currentQualityLabel = videoQualityLabel,
                    inlineHintLabel = videoQualitySwitchState.inlineHintLabel,
                    modifier = Modifier
                        .align(Alignment.TopEnd)
                        .padding(12.dp)
                )
            }

            if (controlsVisible) {
                PlayerOverlayControls(
                    isPlaying = isPlaying,
                    videoQualityNotice = videoQualityNotice,
                    controlHint = controlHint,
                    playbackButtonEnabled = playbackButtonEnabled,
                    secondaryControlsEnabled = secondaryControlsEnabled,
                    fullscreenVisible = fullscreenVisible,
                    playbackSpeed = playbackSpeed,
                    availableVideoQualities = availableVideoQualities,
                    videoQualityPreference = videoQualityPreference,
                    fullscreenMenu = fullscreenMenu,
                    onPlaybackToggleClick = onPlaybackToggleClick,
                    onSeekBackwardClick = onSeekBackwardClick,
                    onSeekForwardClick = onSeekForwardClick,
                    onSettingsClick = onSettingsClick,
                    onFullscreenClick = onFullscreenClick,
                    onFullscreenSpeedClick = {
                        fullscreenMenu = if (fullscreenMenu == FullscreenMenuSpeed) "" else FullscreenMenuSpeed
                    },
                    onFullscreenQualityClick = {
                        fullscreenMenu = if (fullscreenMenu == FullscreenMenuQuality) "" else FullscreenMenuQuality
                    },
                    onPlaybackSpeedChange = onPlaybackSpeedChange,
                    onVideoQualityPreferenceChange = onVideoQualityPreferenceChange,
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
    inlineHintLabel: String,
    modifier: Modifier = Modifier
) {
    val targetQualityHint = if (!selectedPreference.isAuto) {
        "目标 · ${selectedPreference.label}"
    } else {
        ""
    }
    val statusHint = listOf(inlineHintLabel, targetQualityHint)
        .filter { hint -> hint.isNotBlank() }
        .joinToString(" · ")
    Column(
        modifier = modifier,
        horizontalAlignment = Alignment.End,
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        QualityPill(
            selectedPreference = selectedPreference,
            currentQualityLabel = currentQualityLabel
        )
        if (statusHint.isNotBlank()) {
            Text(
                text = statusHint,
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
private fun FullscreenSystemUiEffect() {
    val view = LocalView.current
    DisposableEffect(view) {
        val window = (view.parent as? DialogWindowProvider)?.window
        if (window == null) {
            onDispose { }
        } else {
            WindowCompat.setDecorFitsSystemWindows(window, false)
            val originalCutoutMode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                window.attributes.layoutInDisplayCutoutMode
            } else {
                WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_DEFAULT
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                window.attributes = window.attributes.apply {
                    layoutInDisplayCutoutMode =
                        WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES
                }
            }
            val controller = WindowCompat.getInsetsController(window, window.decorView)
            controller.systemBarsBehavior =
                WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            controller.hide(WindowInsetsCompat.Type.systemBars())

            onDispose {
                controller.show(WindowInsetsCompat.Type.systemBars())
                WindowCompat.setDecorFitsSystemWindows(window, true)
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                    window.attributes = window.attributes.apply {
                        layoutInDisplayCutoutMode = originalCutoutMode
                    }
                }
            }
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
            Text(
                text = label,
                color = PlayerText,
                fontSize = 11.sp,
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
    seekEnabled: Boolean,
    onProgressSeekCommit: (Long) -> Unit
) {
    var dragging by rememberSaveable { mutableStateOf(false) }
    var dragPositionMs by rememberSaveable { mutableStateOf(0L) }
    val safeDuration = duration.coerceAtLeast(0L)
    val displayPosition = if (dragging) dragPositionMs else currentPosition
    val progress = if (safeDuration > 0L) {
        (displayPosition.toFloat() / safeDuration.toFloat()).coerceIn(0f, 1f)
    } else 0f

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
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            SeekProgressBar(
                progress = progress,
                enabled = seekEnabled && safeDuration > 0L,
                onProgressChange = { nextProgress ->
                    dragging = true
                    dragPositionMs = (safeDuration * nextProgress).toLong()
                        .coerceIn(0L, safeDuration)
                },
                onProgressCommit = {
                    if (dragging && safeDuration > 0L) {
                        onProgressSeekCommit(dragPositionMs.coerceIn(0L, safeDuration))
                    }
                    dragging = false
                }
            )
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(text = formatMs(displayPosition), color = PlayerTextMuted, fontSize = 12.sp)
                Text(text = formatMs(duration), color = PlayerTextMuted, fontSize = 12.sp)
            }
        }
    }
}

@Composable
@OptIn(ExperimentalMaterial3Api::class)
private fun SeekProgressBar(
    progress: Float,
    enabled: Boolean,
    onProgressChange: (Float) -> Unit,
    onProgressCommit: () -> Unit
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
        modifier = Modifier
            .fillMaxWidth()
            .height(24.dp),
        thumb = {
            Box(
                modifier = Modifier
                    .size(if (enabled) 14.dp else 10.dp)
                    .clip(CircleShape)
                    .background(
                        Brush.radialGradient(
                            listOf(PlayerText, PlayerPrimary)
                        )
                    )
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
    isPlaying: Boolean,
    videoQualityNotice: String,
    controlHint: String,
    playbackButtonEnabled: Boolean,
    secondaryControlsEnabled: Boolean,
    fullscreenVisible: Boolean,
    playbackSpeed: Float,
    availableVideoQualities: List<PlayerVideoQualityOption>,
    videoQualityPreference: PlayerVideoQualityPreference,
    fullscreenMenu: String,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onSettingsClick: () -> Unit,
    onFullscreenClick: () -> Unit,
    onFullscreenSpeedClick: () -> Unit,
    onFullscreenQualityClick: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(
                Brush.verticalGradient(
                    listOf(Color.Transparent, Color(0xF2090A11))
                )
            )
            .padding(horizontal = 12.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(9.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(7.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                FullscreenIconButton(
                    fullscreenVisible = fullscreenVisible,
                    onClick = onFullscreenClick
                )
                PlayPauseIconButton(
                    isPlaying = isPlaying,
                    enabled = playbackButtonEnabled,
                    onClick = onPlaybackToggleClick
                )
                OverlayControlChip(
                    label = "-10s",
                    enabled = secondaryControlsEnabled,
                    onClick = onSeekBackwardClick
                )
                OverlayControlChip(
                    label = "+10s",
                    enabled = secondaryControlsEnabled,
                    onClick = onSeekForwardClick
                )
            }

            if (fullscreenVisible) {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    FullscreenBottomMenuChip(
                        label = playbackSpeed.toSpeedLabel(),
                        selected = fullscreenMenu == FullscreenMenuSpeed,
                        enabled = secondaryControlsEnabled,
                        onClick = onFullscreenSpeedClick
                    )
                    FullscreenBottomMenuChip(
                        label = if (videoQualityPreference.isAuto) {
                            "自动"
                        } else {
                            videoQualityPreference.label
                        },
                        selected = fullscreenMenu == FullscreenMenuQuality,
                        enabled = secondaryControlsEnabled,
                        onClick = onFullscreenQualityClick
                    )
                }
            } else {
                SettingsIconButton(
                    enabled = secondaryControlsEnabled,
                    onClick = onSettingsClick
                )
            }
        }
        Text(
            text = videoQualityNotice.ifBlank { controlHint },
            color = PlayerTextMuted,
            fontSize = 11.sp,
            lineHeight = 15.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

@Composable
private fun FullscreenFloatingSelectorPanel(
    menu: String,
    playbackSpeed: Float,
    availableVideoQualities: List<PlayerVideoQualityOption>,
    videoQualityPreference: PlayerVideoQualityPreference,
    secondaryControlsEnabled: Boolean,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    modifier: Modifier = Modifier
) {
    val scrollState = rememberScrollState()
    val title = if (menu == FullscreenMenuSpeed) "倍速" else "清晰度"
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
            Text(
                text = title,
                color = PlayerTextMuted,
                fontSize = 11.sp,
                fontWeight = FontWeight.Black,
                maxLines = 1
            )
            Row(
                modifier = Modifier.horizontalScroll(scrollState),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                if (menu == FullscreenMenuSpeed) {
                    listOf(1.0f, 1.25f, 1.5f, 2.0f).forEach { speed ->
                        FullscreenMenuOptionChip(
                            label = speed.toSpeedLabel(),
                            selected = playbackSpeed.toSpeedLabel() == speed.toSpeedLabel(),
                            enabled = secondaryControlsEnabled,
                            onClick = { onPlaybackSpeedChange(speed) }
                        )
                    }
                } else {
                    availableVideoQualities.ifEmpty { listOf(PlayerVideoQualityOption.Auto) }
                        .forEach { option ->
                            val preference = PlayerVideoQualityPreference(option.height)
                            val selected = if (preference.isAuto) {
                                videoQualityPreference.isAuto
                            } else {
                                videoQualityPreference.height == preference.height
                            }
                            FullscreenMenuOptionChip(
                                label = option.settingsLabel(),
                                selected = selected,
                                enabled = secondaryControlsEnabled,
                                onClick = { onVideoQualityPreferenceChange(preference) }
                            )
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
        border = BorderStroke(
            width = 1.dp,
            color = if (selected) PlayerPrimary.copy(alpha = 0.9f) else Color(0x26FFFFFF)
        ),
        onClick = {
            if (enabled) onClick()
        }
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
        color = when {
            selected -> PlayerPrimary.copy(alpha = 0.24f)
            else -> Color(0x33121420)
        },
        shape = RoundedCornerShape(999.dp),
        border = BorderStroke(
            width = 1.dp,
            color = if (selected) PlayerPrimary.copy(alpha = 0.9f) else Color(0x22FFFFFF)
        ),
        onClick = {
            if (enabled) onClick()
        }
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
private fun PlayerSettingsDrawer(
    playbackSpeed: Float,
    availableVideoQualities: List<PlayerVideoQualityOption>,
    videoQualityPreference: PlayerVideoQualityPreference,
    secondaryControlsEnabled: Boolean,
    onDismiss: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit
) {
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
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
                        enabled = secondaryControlsEnabled,
                        onSelect = { label -> onPlaybackSpeedChange(label.toFloat()) }
                    )
                    PlayerSettingsDivider()
                    PlayerSettingsRow(
                        title = "清晰度",
                        options = availableVideoQualities.ifEmpty {
                            listOf(PlayerVideoQualityOption.Auto)
                        }.map { option -> option.settingsLabel() },
                        selected = if (videoQualityPreference.isAuto) {
                            PlayerVideoQualityOption.Auto.settingsLabel()
                        } else {
                            videoQualityPreference.label
                        },
                        enabled = secondaryControlsEnabled,
                        onSelect = { label ->
                            val nextPreference = availableVideoQualities
                                .firstOrNull { option -> option.settingsLabel() == label }
                                ?.let { option -> PlayerVideoQualityPreference(option.height) }
                                ?: PlayerVideoQualityPreference.Auto
                            onVideoQualityPreferenceChange(nextPreference)
                        }
                    )
                    Text(
                        text = "自动会根据流畅度调整；手动档位卡顿时仍会优先保障同步播放。",
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
    Surface(
        color = Color.Transparent,
        shape = RoundedCornerShape(999.dp),
        onClick = {
            if (enabled) onClick()
        }
    ) {
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
private fun FullscreenIconButton(
    fullscreenVisible: Boolean,
    onClick: () -> Unit
) {
    Surface(
        modifier = Modifier.size(40.dp),
        color = Color.Transparent,
        shape = CircleShape,
        onClick = onClick
    ) {
        FullscreenGlyph(
            fullscreenVisible = fullscreenVisible,
            modifier = Modifier
                .fillMaxSize()
                .padding(10.dp)
        )
    }
}

@Composable
private fun FullscreenGlyph(
    fullscreenVisible: Boolean,
    modifier: Modifier = Modifier
) {
    Canvas(modifier = modifier) {
        val strokeWidth = 2.2.dp.toPx()
        val segment = size.minDimension * 0.34f
        val inset = strokeWidth / 2
        val stroke = Stroke(width = strokeWidth, cap = StrokeCap.Square)

        if (fullscreenVisible) {
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(inset, segment),
                end = androidx.compose.ui.geometry.Offset(segment, segment),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(segment, inset),
                end = androidx.compose.ui.geometry.Offset(segment, segment),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(size.width - segment, segment),
                end = androidx.compose.ui.geometry.Offset(size.width - inset, segment),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(size.width - segment, inset),
                end = androidx.compose.ui.geometry.Offset(size.width - segment, segment),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(segment, size.height - segment),
                end = androidx.compose.ui.geometry.Offset(segment, size.height - inset),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(inset, size.height - segment),
                end = androidx.compose.ui.geometry.Offset(segment, size.height - segment),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(size.width - segment, size.height - segment),
                end = androidx.compose.ui.geometry.Offset(size.width - segment, size.height - inset),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(size.width - segment, size.height - segment),
                end = androidx.compose.ui.geometry.Offset(size.width - inset, size.height - segment),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
        } else {
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(inset, inset),
                end = androidx.compose.ui.geometry.Offset(segment, inset),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(inset, inset),
                end = androidx.compose.ui.geometry.Offset(inset, segment),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(size.width - segment, inset),
                end = androidx.compose.ui.geometry.Offset(size.width - inset, inset),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(size.width - inset, inset),
                end = androidx.compose.ui.geometry.Offset(size.width - inset, segment),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(inset, size.height - segment),
                end = androidx.compose.ui.geometry.Offset(inset, size.height - inset),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(inset, size.height - inset),
                end = androidx.compose.ui.geometry.Offset(segment, size.height - inset),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(size.width - inset, size.height - segment),
                end = androidx.compose.ui.geometry.Offset(size.width - inset, size.height - inset),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
            drawLine(
                color = PlayerText,
                start = androidx.compose.ui.geometry.Offset(size.width - segment, size.height - inset),
                end = androidx.compose.ui.geometry.Offset(size.width - inset, size.height - inset),
                strokeWidth = stroke.width,
                cap = stroke.cap
            )
        }
    }
}

@Composable
private fun PlayPauseIconButton(
    isPlaying: Boolean,
    enabled: Boolean,
    onClick: () -> Unit
) {
    val icon = if (isPlaying) "Ⅱ" else "▶"
    val backgroundColor = Color.Transparent
    val textColor = if (enabled) PlayerText else PlayerTextMuted.copy(alpha = 0.55f)

    Surface(
        modifier = Modifier.size(38.dp),
        color = backgroundColor,
        shape = CircleShape,
        onClick = {
            if (enabled) onClick()
        }
    ) {
        Box(contentAlignment = Alignment.Center) {
            Text(
                text = icon,
                color = textColor,
                fontSize = if (isPlaying) 19.sp else 18.sp,
                fontWeight = FontWeight.Black
            )
        }
    }
}

@Composable
private fun OverlayControlChip(
    label: String,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    onClick: () -> Unit
) {
    val backgroundColor = if (enabled) Color(0x24FFFFFF) else Color(0x18FFFFFF)
    val textColor = if (enabled) PlayerText else PlayerTextMuted.copy(alpha = 0.55f)

    Surface(
        modifier = modifier,
        color = backgroundColor,
        shape = RoundedCornerShape(999.dp),
        onClick = {
            if (enabled) onClick()
        }
    ) {
        Text(
            text = label,
            color = textColor,
            fontSize = 12.sp,
            fontWeight = FontWeight.Bold,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 9.dp)
        )
    }
}

@Composable
private fun SettingsIconButton(
    enabled: Boolean,
    onClick: () -> Unit
) {
    Surface(
        modifier = Modifier.size(38.dp),
        color = Color.Transparent,
        shape = CircleShape,
        onClick = {
            if (enabled) onClick()
        }
    ) {
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

private fun formatMs(value: Long): String {
    if (value <= 0L) return "00:00"
    val totalSeconds = value / 1000
    val minutes = totalSeconds / 60
    val seconds = totalSeconds % 60
    return "%02d:%02d".format(minutes, seconds)
}

private fun Float.toSpeedLabel(): String {
    return if (this % 1f == 0f) {
        "%.1f".format(this)
    } else {
        this.toString()
    }
}

private fun PlayerVideoQualityOption.settingsLabel(): String {
    return if (isAuto) {
        "自动"
    } else {
        label
    }
}
