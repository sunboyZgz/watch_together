package com.example.watch_together.ui.player_default

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.example.watch_together.config.AppConfig
import com.example.watch_together.sync.RoomSyncState

@Composable
internal fun RoomPlayerDebugShell(
    latestSyncState: RoomSyncState?,
    syncLogs: List<String>,
    playerEventLogs: List<String>,
) {
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        SyncStatePanel(latestSyncState = latestSyncState)
        SyncDebugPanel(syncLogs = syncLogs)
        PlayerEventDebugPanel(eventLogs = playerEventLogs)
        ConfigInjectionHint()
    }
}

@Composable
private fun SyncStatePanel(latestSyncState: RoomSyncState?) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Text(
                text = "Latest sync state",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            if (latestSyncState == null) {
                Text(
                    text = "No synced state applied yet.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            } else {
                Text("roomId=${latestSyncState.roomId}", style = MaterialTheme.typography.bodySmall)
                Text("mediaId=${latestSyncState.mediaId}", style = MaterialTheme.typography.bodySmall)
                Text("hostUserId=${latestSyncState.hostUserId}", style = MaterialTheme.typography.bodySmall)
                Text("paused=${latestSyncState.paused}", style = MaterialTheme.typography.bodySmall)
                Text("ended=${latestSyncState.ended}", style = MaterialTheme.typography.bodySmall)
                Text("positionMs=${latestSyncState.positionMs}", style = MaterialTheme.typography.bodySmall)
                Text(
                    "playbackRate=${latestSyncState.playbackRate} · seq=${latestSyncState.seq}",
                    style = MaterialTheme.typography.bodySmall
                )
            }
        }
    }
}

@Composable
private fun SyncDebugPanel(syncLogs: List<String>) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = "Sync log",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            if (syncLogs.isEmpty()) {
                Text(
                    text = "No sync events yet.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            } else {
                syncLogs.forEach { line ->
                    Text(
                        text = line,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
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
            Text(text = "APP_ENV=${AppConfig.appEnv}", style = MaterialTheme.typography.bodyMedium)
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

@Composable
private fun PlayerEventDebugPanel(eventLogs: List<String>) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = "Player event log",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            if (eventLogs.isEmpty()) {
                Text(
                    text = "No events emitted yet.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            } else {
                eventLogs.forEach { line ->
                    Text(
                        text = line,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
        }
    }
}
