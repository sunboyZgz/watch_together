package com.example.watch_together.ui.player

import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import com.example.watch_together.config.AppConfig
import com.example.watch_together.ui.theme.Watch_togetherTheme
import androidx.media3.ui.PlayerView
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive

@Composable
fun PlayerScreen(modifier: Modifier = Modifier) {
    val adapter = rememberPlayerAdapter()
    val sampleUrl = remember {
        AppConfig.sampleHlsUrl()
    }
    var currentPosition by remember { mutableLongStateOf(0L) }
    var duration by remember { mutableLongStateOf(0L) }
    var isPlaying by remember { mutableStateOf(false) }
    var playbackSpeed by remember { mutableFloatStateOf(1f) }

    LaunchedEffect(adapter) {
        while (isActive) {
            currentPosition = adapter.getCurrentPosition()
            duration = adapter.getDuration().coerceAtLeast(0L)
            isPlaying = adapter.isPlaying()
            delay(500)
        }
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        PlayerStatusHeader(sampleUrl = sampleUrl)
        PlayerViewport(adapter = adapter)
        PlayerControls(
            adapter = adapter,
            sampleUrl = sampleUrl,
            currentPosition = currentPosition,
            duration = duration,
            isPlaying = isPlaying,
            playbackSpeed = playbackSpeed,
            onPlaybackSpeedChange = { speed ->
                playbackSpeed = speed
                adapter.setPlaybackSpeed(speed)
            }
        )
        ConfigInjectionHint()
    }
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

@Composable
private fun PlayerStatusHeader(sampleUrl: String) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp)) {
            Text(
                text = "watch_together",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold
            )
            Spacer(modifier = Modifier.height(4.dp))
            Text(
                text = "Single-device player validation screen",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(modifier = Modifier.height(8.dp))
            Text(
                text = "Sample HLS URL: $sampleUrl",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
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
            Column(
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = { adapter.load(sampleUrl) }) {
                        Text("Load sample")
                    }
                    Button(onClick = { adapter.play() }) {
                        Text("Play")
                    }
                    OutlinedButton(onClick = { adapter.pause() }) {
                        Text("Pause")
                    }
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = {
                        adapter.seekTo((currentPosition - 10_000L).coerceAtLeast(0L))
                    }) {
                        Text("-10s")
                    }
                    OutlinedButton(onClick = {
                        val safeDuration = if (duration > 0L) duration else currentPosition + 10_000L
                        adapter.seekTo((currentPosition + 10_000L).coerceAtMost(safeDuration))
                    }) {
                        Text("+10s")
                    }
                }
            }
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                listOf(0.75f, 1.0f, 1.25f, 1.5f, 2.0f).forEach { speed ->
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

@Composable
private fun ConfigInjectionHint() {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Text(
                text = "Config injection entry",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            Text(
                text = "APP_ENV=${AppConfig.appEnv}",
                style = MaterialTheme.typography.bodyMedium
            )
            Text(
                text = "API_BASE_URL=${AppConfig.apiBaseUrl}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "WS_BASE_URL=${AppConfig.wsBaseUrl}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "MEDIA_BASE_URL=${AppConfig.mediaBaseUrl}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "MEDIA_DEFAULT_ID=${AppConfig.mediaDefaultId.ifBlank { "(empty)" }}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "DEBUG_SYNC=${AppConfig.debugSync}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

@Preview(showBackground = true, showSystemUi = true)
@Composable
private fun PlayerScreenPreview() {
    Watch_togetherTheme {
        PlayerScreen()
    }
}

private fun AppConfig.sampleHlsUrl(): String {
    return if (mediaDefaultId.isNotBlank()) {
        "${mediaBaseUrl.trimEnd('/')}/${mediaDefaultId}/index.m3u8"
    } else {
        "https://storage.googleapis.com/shaka-demo-assets/angel-one-hls/hls.m3u8"
    }
}

private fun formatMs(value: Long): String {
    if (value <= 0L) return "00:00"
    val totalSeconds = value / 1000
    val minutes = totalSeconds / 60
    val seconds = totalSeconds % 60
    return "%02d:%02d".format(minutes, seconds)
}
