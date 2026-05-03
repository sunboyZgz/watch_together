package com.example.watch_together.ui.player

import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
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
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
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
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.media3.ui.PlayerView
import kotlinx.coroutines.delay

private val PlayerText = Color(0xFFF9F3FF)
private val PlayerTextMuted = Color(0xC8D7D1E5)
private val PlayerPrimary = Color(0xFFFF82C9)
private val PlayerAccent = Color(0xFF8FE7FF)
private val PlayerOutline = Color(0x22FFFFFF)

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
    controlHint: String,
    playbackButtonEnabled: Boolean,
    secondaryControlsEnabled: Boolean,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
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
            controlHint = controlHint,
            playbackButtonEnabled = playbackButtonEnabled,
            secondaryControlsEnabled = secondaryControlsEnabled,
            onPlaybackToggleClick = onPlaybackToggleClick,
            onSeekBackwardClick = onSeekBackwardClick,
            onSeekForwardClick = onSeekForwardClick,
            onPlaybackSpeedChange = onPlaybackSpeedChange
        )
        MediaSummary(
            mediaTitle = mediaTitle,
            mediaMeta = mediaMeta,
            currentPosition = currentPosition,
            duration = duration
        )
    }
}

@Composable
private fun PlayerViewport(
    adapter: PlayerAdapter,
    isPlaying: Boolean,
    playbackSpeed: Float,
    videoQualityLabel: String,
    controlHint: String,
    playbackButtonEnabled: Boolean,
    secondaryControlsEnabled: Boolean,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit
) {
    var controlsVisible by rememberSaveable { mutableStateOf(false) }
    var speedPickerVisible by rememberSaveable { mutableStateOf(false) }
    var fullscreenVisible by rememberSaveable { mutableStateOf(false) }
    var interactionTick by rememberSaveable { mutableStateOf(0) }

    fun keepControlsVisible() {
        controlsVisible = true
        interactionTick += 1
    }

    LaunchedEffect(controlsVisible, speedPickerVisible, interactionTick) {
        if (!controlsVisible) return@LaunchedEffect
        delay(3_200L)
        controlsVisible = false
        speedPickerVisible = false
    }

    LaunchedEffect(secondaryControlsEnabled) {
        if (!secondaryControlsEnabled) {
            speedPickerVisible = false
        }
    }

    PlayerSurface(
        adapter = adapter,
        isPlaying = isPlaying,
        playbackSpeed = playbackSpeed,
        videoQualityLabel = videoQualityLabel,
        controlHint = controlHint,
        playbackButtonEnabled = playbackButtonEnabled,
        secondaryControlsEnabled = secondaryControlsEnabled,
        controlsVisible = controlsVisible,
        speedPickerVisible = speedPickerVisible,
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
        onSpeedClick = {
            keepControlsVisible()
            speedPickerVisible = !speedPickerVisible
        },
        onFullscreenClick = {
            fullscreenVisible = true
            keepControlsVisible()
        },
        onPlaybackSpeedChange = { speed ->
            keepControlsVisible()
            speedPickerVisible = false
            onPlaybackSpeedChange(speed)
        },
        modifier = Modifier
            .fillMaxWidth()
            .aspectRatio(16f / 9f),
        shape = RoundedCornerShape(14.dp)
    )

    if (fullscreenVisible) {
        Dialog(
            onDismissRequest = { fullscreenVisible = false },
            properties = DialogProperties(
                usePlatformDefaultWidth = false,
                decorFitsSystemWindows = false
            )
        ) {
            PlayerSurface(
                adapter = adapter,
                isPlaying = isPlaying,
                playbackSpeed = playbackSpeed,
                videoQualityLabel = videoQualityLabel,
                controlHint = controlHint,
                playbackButtonEnabled = playbackButtonEnabled,
                secondaryControlsEnabled = secondaryControlsEnabled,
                controlsVisible = true,
                speedPickerVisible = speedPickerVisible,
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
                onSpeedClick = {
                    keepControlsVisible()
                    speedPickerVisible = !speedPickerVisible
                },
                onFullscreenClick = {
                    fullscreenVisible = false
                    speedPickerVisible = false
                },
                onPlaybackSpeedChange = { speed ->
                    keepControlsVisible()
                    speedPickerVisible = false
                    onPlaybackSpeedChange(speed)
                },
                modifier = Modifier
                    .fillMaxSize()
                    .background(Color.Black),
                shape = RoundedCornerShape(0.dp)
            )
        }
    }
}

@Composable
private fun PlayerSurface(
    adapter: PlayerAdapter,
    isPlaying: Boolean,
    playbackSpeed: Float,
    videoQualityLabel: String,
    controlHint: String,
    playbackButtonEnabled: Boolean,
    secondaryControlsEnabled: Boolean,
    controlsVisible: Boolean,
    speedPickerVisible: Boolean,
    fullscreenVisible: Boolean,
    onTap: () -> Unit,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onSpeedClick: () -> Unit,
    onFullscreenClick: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    modifier: Modifier,
    shape: RoundedCornerShape
) {
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
                .clickable { onTap() }
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
                    PlayerView(context).apply {
                        layoutParams = android.view.ViewGroup.LayoutParams(MATCH_PARENT, MATCH_PARENT)
                        useController = false
                        adapter.attach(this)
                    }
                },
                modifier = Modifier.fillMaxSize(),
                update = { playerView ->
                    playerView.useController = false
                    adapter.attach(playerView)
                }
            )

            if (controlsVisible) {
                QualityPill(
                    label = videoQualityLabel,
                    modifier = Modifier
                        .align(Alignment.TopEnd)
                        .padding(12.dp)
                )
                PlayerOverlayControls(
                    isPlaying = isPlaying,
                    playbackSpeed = playbackSpeed,
                    controlHint = controlHint,
                    playbackButtonEnabled = playbackButtonEnabled,
                    secondaryControlsEnabled = secondaryControlsEnabled,
                    speedPickerVisible = speedPickerVisible,
                    fullscreenVisible = fullscreenVisible,
                    onPlaybackToggleClick = onPlaybackToggleClick,
                    onSeekBackwardClick = onSeekBackwardClick,
                    onSeekForwardClick = onSeekForwardClick,
                    onSpeedClick = onSpeedClick,
                    onFullscreenClick = onFullscreenClick,
                    onPlaybackSpeedChange = onPlaybackSpeedChange,
                    modifier = Modifier.align(Alignment.BottomCenter)
                )
            }
        }
    }
}

@Composable
private fun QualityPill(
    label: String,
    modifier: Modifier = Modifier
) {
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
                    .background(PlayerAccent)
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
    duration: Long
) {
    val progress = if (duration > 0L) {
        (currentPosition.toFloat() / duration.toFloat()).coerceIn(0f, 1f)
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
            text = "$mediaMeta · ${formatMs(currentPosition)} / ${formatMs(duration)}",
            color = PlayerTextMuted,
            fontSize = 14.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(8.dp)
                    .clip(RoundedCornerShape(999.dp))
                    .background(Color(0x22FFFFFF))
            ) {
                Box(
                    modifier = Modifier
                        .fillMaxWidth(progress)
                        .height(8.dp)
                        .clip(RoundedCornerShape(999.dp))
                        .background(
                            Brush.horizontalGradient(
                                listOf(PlayerPrimary, PlayerAccent)
                            )
                        )
                )
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(text = formatMs(currentPosition), color = PlayerTextMuted, fontSize = 12.sp)
                Text(text = formatMs(duration), color = PlayerTextMuted, fontSize = 12.sp)
            }
        }
    }
}

@Composable
private fun PlayerOverlayControls(
    isPlaying: Boolean,
    playbackSpeed: Float,
    controlHint: String,
    playbackButtonEnabled: Boolean,
    secondaryControlsEnabled: Boolean,
    speedPickerVisible: Boolean,
    fullscreenVisible: Boolean,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onSpeedClick: () -> Unit,
    onFullscreenClick: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
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
        if (speedPickerVisible) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.End
            ) {
                SpeedPickerSheet(
                    playbackSpeed = playbackSpeed,
                    onPlaybackSpeedChange = onPlaybackSpeedChange
                )
            }
        }
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

            SpeedEntryButton(
                playbackSpeed = playbackSpeed,
                expanded = speedPickerVisible,
                enabled = secondaryControlsEnabled,
                onClick = onSpeedClick
            )
        }
        Text(
            text = controlHint,
            color = PlayerTextMuted,
            fontSize = 11.sp,
            lineHeight = 15.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
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
private fun SpeedEntryButton(
    playbackSpeed: Float,
    expanded: Boolean,
    enabled: Boolean,
    onClick: () -> Unit
) {
    val textColor = when {
        !enabled -> PlayerTextMuted.copy(alpha = 0.55f)
        expanded -> PlayerPrimary
        else -> PlayerText
    }

    Surface(
        color = Color.Transparent,
        shape = RoundedCornerShape(999.dp),
        onClick = {
            if (enabled) onClick()
        }
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 9.dp),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "倍速 ${playbackSpeed}x",
                color = textColor,
                fontSize = 12.sp,
                fontWeight = FontWeight.Bold
            )
            Text(
                text = if (expanded) "⌄" else "⌃",
                color = PlayerTextMuted,
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold
            )
        }
    }
}

@Composable
private fun SpeedPickerSheet(
    playbackSpeed: Float,
    onPlaybackSpeedChange: (Float) -> Unit
) {
    Surface(
        modifier = Modifier.width(86.dp),
        color = Color(0xF0121420),
        shape = RoundedCornerShape(10.dp),
        border = BorderStroke(1.dp, Color(0x18FFFFFF))
    ) {
        Column(modifier = Modifier.padding(vertical = 4.dp)) {
            listOf(2.0f, 1.5f, 1.25f, 1.0f).forEach { speed ->
                SpeedOption(
                    speed = speed,
                    selected = speed == playbackSpeed,
                    onClick = { onPlaybackSpeedChange(speed) }
                )
            }
        }
    }
}

@Composable
private fun SpeedOption(
    speed: Float,
    selected: Boolean,
    onClick: () -> Unit
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = if (selected) Color(0x1AFF78C6) else Color.Transparent,
        shape = RoundedCornerShape(8.dp),
        onClick = onClick
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(7.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "•",
                color = if (selected) PlayerPrimary else Color.Transparent,
                fontSize = 12.sp,
                fontWeight = FontWeight.Black
            )
            Text(
                text = "${speed}x",
                color = if (selected) PlayerPrimary else PlayerText,
                fontSize = 12.sp,
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
