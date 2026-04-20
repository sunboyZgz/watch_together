package com.example.watch_together.ui.player

import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.ui.PlayerView

@Composable
internal fun PlayerCoreShell(
    adapter: PlayerAdapter,
    sampleUrl: String,
    currentPosition: Long,
    duration: Long,
    isPlaying: Boolean,
    playbackSpeed: Float,
    isHostController: Boolean,
    isJoinedToRoom: Boolean,
    onPlaySync: () -> Unit,
    onPauseSync: () -> Unit,
    onSeekSync: (Long) -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        PlayerViewport(adapter = adapter)
        PlayerControls(
            adapter = adapter,
            sampleUrl = sampleUrl,
            currentPosition = currentPosition,
            duration = duration,
            isPlaying = isPlaying,
            playbackSpeed = playbackSpeed,
            isHostController = isHostController,
            isJoinedToRoom = isJoinedToRoom,
            onPlaySync = onPlaySync,
            onPauseSync = onPauseSync,
            onSeekSync = onSeekSync,
            onPlaybackSpeedChange = onPlaybackSpeedChange
        )
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
    isHostController: Boolean,
    isJoinedToRoom: Boolean,
    onPlaySync: () -> Unit,
    onPauseSync: () -> Unit,
    onSeekSync: (Long) -> Unit,
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
            Text(
                text = when {
                    isHostController -> "Buttons will send sync events to the server."
                    isJoinedToRoom -> "Viewer mode: incoming sync is applied, local control stays local."
                    else -> "Local-only mode until you join a room."
                },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = { adapter.load(sampleUrl) }) {
                        Text("Load sample")
                    }
                    Button(onClick = {
                        if (isHostController) {
                            onPlaySync()
                        } else {
                            adapter.play()
                        }
                    }) {
                        Text(if (isHostController) "Play sync" else "Play")
                    }
                    OutlinedButton(onClick = {
                        if (isHostController) {
                            onPauseSync()
                        } else {
                            adapter.pause()
                        }
                    }) {
                        Text(if (isHostController) "Pause sync" else "Pause")
                    }
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = {
                        val target = (currentPosition - 10_000L).coerceAtLeast(0L)
                        if (isHostController) {
                            onSeekSync(target)
                        } else {
                            adapter.seekTo(target)
                        }
                    }) {
                        Text(if (isHostController) "-10s sync" else "-10s")
                    }
                    OutlinedButton(onClick = {
                        val safeDuration = if (duration > 0L) duration else currentPosition + 10_000L
                        val target = (currentPosition + 10_000L).coerceAtMost(safeDuration)
                        if (isHostController) {
                            onSeekSync(target)
                        } else {
                            adapter.seekTo(target)
                        }
                    }) {
                        Text(if (isHostController) "+10s sync" else "+10s")
                    }
                }
            }
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                listOf(
                    listOf(0.75f, 1.0f, 1.25f),
                    listOf(1.5f, 2.0f)
                ).forEach { rowSpeeds ->
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        rowSpeeds.forEach { speed ->
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
    }
}

private fun formatMs(value: Long): String {
    if (value <= 0L) return "00:00"
    val totalSeconds = value / 1000
    val minutes = totalSeconds / 60
    val seconds = totalSeconds % 60
    return "%02d:%02d".format(minutes, seconds)
}

