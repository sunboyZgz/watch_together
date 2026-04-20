package com.example.watch_together.ui.player

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

@Composable
internal fun RoomPlayerPageShell(
    modifier: Modifier = Modifier,
    sampleUrl: String,
    hostUserId: String,
    viewerUserId: String,
    uiState: RoomPlayerUiState,
    adapter: PlayerAdapter,
    isHostController: Boolean,
    onJoinRoomInputChange: (String) -> Unit,
    onCreateAndJoinAsHost: () -> Unit,
    onJoinAsViewer: () -> Unit,
    onRejoinCurrentUser: () -> Unit,
    onPlaySync: () -> Unit,
    onPauseSync: () -> Unit,
    onSeekSync: (Long) -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        PlayerStatusHeader(
            sampleUrl = sampleUrl,
            currentRoomId = uiState.currentRoomId,
            syncStatus = uiState.syncStatus,
            activeUserId = uiState.activeUserId,
            isHostController = isHostController
        )
        JoinSyncActionsCard(
            hostUserId = hostUserId,
            viewerUserId = viewerUserId,
            currentRoomId = uiState.currentRoomId,
            syncStatus = uiState.syncStatus,
            joinRoomInput = uiState.joinRoomInput,
            onJoinRoomInputChange = onJoinRoomInputChange,
            onCreateAndJoinAsHost = onCreateAndJoinAsHost,
            onJoinAsViewer = onJoinAsViewer,
            onRejoinCurrentUser = onRejoinCurrentUser,
            canRejoinCurrentUser = uiState.activeUserId != null && uiState.currentRoomId != null
        )
        PlayerCoreShell(
            adapter = adapter,
            sampleUrl = sampleUrl,
            currentPosition = uiState.player.currentPosition,
            duration = uiState.player.duration,
            isPlaying = uiState.player.isPlaying,
            playbackSpeed = uiState.player.playbackSpeed,
            isHostController = isHostController,
            isJoinedToRoom = uiState.isJoinedToRoom,
            onPlaySync = onPlaySync,
            onPauseSync = onPauseSync,
            onSeekSync = onSeekSync,
            onPlaybackSpeedChange = onPlaybackSpeedChange
        )
    }
}

@Composable
private fun PlayerStatusHeader(
    sampleUrl: String,
    currentRoomId: String?,
    syncStatus: SyncStatus,
    activeUserId: String?,
    isHostController: Boolean
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp)) {
            Text(
                text = "watch_together",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.SemiBold
            )
            Spacer(modifier = Modifier.height(4.dp))
            Text(
                text = "Android control sync validation",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(modifier = Modifier.height(8.dp))
            Text(
                text = "Sample HLS URL: $sampleUrl",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Current room: ${currentRoomId ?: "(not joined yet)"}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Active user: ${activeUserId ?: "(none)"}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Control mode: ${if (isHostController) "host sync" else "local / viewer"}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Sync status: ${syncStatus.label}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

@Composable
private fun JoinSyncActionsCard(
    hostUserId: String,
    viewerUserId: String,
    currentRoomId: String?,
    syncStatus: SyncStatus,
    joinRoomInput: String,
    onJoinRoomInputChange: (String) -> Unit,
    onCreateAndJoinAsHost: () -> Unit,
    onJoinAsViewer: () -> Unit,
    onRejoinCurrentUser: () -> Unit,
    canRejoinCurrentUser: Boolean
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Text(
                text = "Room sync actions",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium
            )
            Text(
                text = "Host user: $hostUserId",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Viewer user: $viewerUserId",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Text(
                text = "Current room: ${currentRoomId ?: "(none)"} · ${syncStatus.label}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            OutlinedTextField(
                value = joinRoomInput,
                onValueChange = onJoinRoomInputChange,
                modifier = Modifier.fillMaxWidth(),
                label = { Text("Room ID") },
                singleLine = true
            )
            androidx.compose.foundation.layout.Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = onCreateAndJoinAsHost) {
                    Text("Create + Join as host")
                }
                OutlinedButton(
                    onClick = onJoinAsViewer,
                    enabled = joinRoomInput.isNotBlank()
                ) {
                    Text("Join as viewer")
                }
                OutlinedButton(
                    onClick = onRejoinCurrentUser,
                    enabled = canRejoinCurrentUser
                ) {
                    Text("Rejoin current user")
                }
            }
        }
    }
}
