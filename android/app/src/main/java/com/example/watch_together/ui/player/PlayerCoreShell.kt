package com.example.watch_together.ui.player

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.unit.dp
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import kotlin.math.roundToLong

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
) {
    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .aspectRatio(16 / 9f)
                .background(Color.Black, RoundedCornerShape(24.dp))
        ) {
            AndroidView(
                modifier = Modifier.fillMaxSize(),
                factory = { context ->
                    PlayerView(context).apply {
                        useController = false
                        resizeMode = AspectRatioFrameLayout.RESIZE_MODE_FIT
                        setShutterBackgroundColor(android.graphics.Color.BLACK)
                        adapter.attach(this)
                    }
                },
                update = { view -> adapter.attach(view) }
            )
            DisposableEffect(adapter) {
                onDispose { adapter.detach() }
            }
            PlayerOverlay(
                state = state,
                controlHint = controlHint,
                onPlaybackToggleClick = onPlaybackToggleClick,
                onSeekBackwardClick = onSeekBackwardClick,
                onSeekForwardClick = onSeekForwardClick,
                onPlaybackSpeedChange = onPlaybackSpeedChange,
                onVideoQualityPreferenceChange = onVideoQualityPreferenceChange,
                modifier = Modifier.align(Alignment.BottomCenter)
            )
        }
        Text(
            text = mediaTitle,
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.onBackground
        )
        Text(
            text = "$mediaMeta · ${formatMs(state.currentPosition)} / ${formatMs(state.duration)}",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        val safeDuration = state.duration.takeIf { it > 0L } ?: 1L
        Slider(
            value = state.currentPosition.coerceIn(0L, safeDuration).toFloat(),
            onValueChange = {},
            onValueChangeFinished = {},
            valueRange = 0f..safeDuration.toFloat(),
            enabled = false,
            modifier = Modifier.fillMaxWidth()
        )
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(4.dp)
        ) {
            LinearProgressIndicator(
                progress = { (state.bufferedPercentage / 100f).coerceIn(0f, 1f) },
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}

@Composable
private fun PlayerOverlay(
    state: PlayerRuntimeState,
    controlHint: String,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(Color(0x99000000))
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = state.videoVariant.displayLabel,
                style = MaterialTheme.typography.labelMedium,
                color = Color.White,
                modifier = Modifier
                    .background(Color(0x663A4160), RoundedCornerShape(999.dp))
                    .padding(horizontal = 10.dp, vertical = 5.dp)
            )
            Spacer(Modifier.width(8.dp))
            Text(
                text = controlHint,
                style = MaterialTheme.typography.labelSmall,
                color = Color(0xCCFFFFFF)
            )
        }
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
            OverlayButton(if (state.isPlaying) "暂停" else "播放", state.canControlPlayback, onPlaybackToggleClick)
            OverlayButton("-10", state.canControlPlayback, onSeekBackwardClick)
            OverlayButton("+10", state.canControlPlayback, onSeekForwardClick)
            Spacer(Modifier.weight(1f))
            SpeedMenu(state.playbackSpeed, onPlaybackSpeedChange)
            QualityMenu(
                options = state.availableVideoQualities,
                selected = state.videoQualityPreference,
                onSelect = onVideoQualityPreferenceChange
            )
        }
    }
}

@Composable
private fun OverlayButton(label: String, enabled: Boolean, onClick: () -> Unit) {
    Text(
        text = label,
        color = if (enabled) Color.White else Color(0x77FFFFFF),
        style = MaterialTheme.typography.labelLarge,
        modifier = Modifier
            .background(Color(0x663A4160), RoundedCornerShape(999.dp))
            .clickable(enabled = enabled, onClick = onClick)
            .padding(horizontal = 12.dp, vertical = 8.dp)
    )
}

@Composable
private fun SpeedMenu(selected: Float, onSelect: (Float) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        OverlayButton("${selected}x", true) { expanded = true }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            listOf(0.75f, 1f, 1.25f, 1.5f, 2f).forEach { speed ->
                DropdownMenuItem(
                    text = { Text("${speed}x") },
                    onClick = {
                        expanded = false
                        onSelect(speed)
                    }
                )
            }
        }
    }
}

@Composable
private fun QualityMenu(
    options: List<PlayerVideoQualityOption>,
    selected: PlayerVideoQualityPreference,
    onSelect: (PlayerVideoQualityPreference) -> Unit
) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        OverlayButton(selected.label, true) { expanded = true }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            options.forEach { option ->
                DropdownMenuItem(
                    text = { Text(option.label) },
                    onClick = {
                        expanded = false
                        onSelect(PlayerVideoQualityPreference(option.height))
                    }
                )
            }
        }
    }
}

private fun formatMs(value: Long): String {
    val totalSeconds = (value.coerceAtLeast(0L) / 1_000.0).roundToLong()
    val minutes = totalSeconds / 60
    val seconds = totalSeconds % 60
    return "%d:%02d".format(minutes, seconds)
}
