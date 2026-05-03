package com.example.watch_together.ui.player

import androidx.media3.common.Player
import com.example.watch_together.sync.RoomSyncState

enum class SyncStatus(val label: String) {
    Idle("Idle"),
    CreatingRoom("Creating room"),
    JoiningAsHost("Joining as host"),
    JoiningAsViewer("Joining as viewer"),
    RejoiningCurrentUser("Rejoining current user"),
    Connected("Connected"),
    RoomStateApplied("room_state applied"),
    PlayApplied("play applied"),
    PauseApplied("pause applied"),
    SeekApplied("seek applied"),
    PlaybackRateApplied("playback rate applied"),
    EndedApplied("ended applied"),
    SyncFailed("Sync failed"),
    CreateAndJoinFailed("Create and join failed")
}

data class PlayerRuntimeUiState(
    val currentPosition: Long = 0L,
    val duration: Long = 0L,
    val bufferedPosition: Long = 0L,
    val bufferedPercentage: Int = 0,
    val isPlaying: Boolean = false,
    val playbackState: Int = Player.STATE_IDLE,
    val playbackSpeed: Float = 1f,
    val videoVariant: PlayerVideoVariant = PlayerVideoVariant(),
) {
    val bufferedAheadMs: Long
        get() = (bufferedPosition - currentPosition).coerceAtLeast(0L)
}

data class PlayerTelemetryUiState(
    val currentMediaUrl: String = "",
    val rebufferCount: Int = 0,
    val totalRebufferDurationMs: Long = 0L,
    val lastRebufferDurationMs: Long = 0L,
    val activeRebufferStartedAtMs: Long = 0L,
    val driftCorrectionCount: Int = 0,
    val seekCorrectionCount: Int = 0,
    val speedNudgeCorrectionCount: Int = 0,
    val lastCorrectionReason: String = "",
    val lastCorrectionDriftMs: Long = 0L,
) {
    val isRebuffering: Boolean
        get() = activeRebufferStartedAtMs > 0L
}

data class RoomPlayerUiState(
    val joinRoomInput: String = "",
    val activeUserId: String? = null,
    val currentRoomId: String? = null,
    val latestSyncState: RoomSyncState? = null,
    val syncStatus: SyncStatus = SyncStatus.Idle,
    val player: PlayerRuntimeUiState = PlayerRuntimeUiState(),
    val telemetry: PlayerTelemetryUiState = PlayerTelemetryUiState(),
    val lastDriftCorrectionAtMs: Long = 0L,
    val lastEndedReportedSeq: Long = -1L,
) {
    val isJoinedToRoom: Boolean
        get() = latestSyncState != null

    val hasPlayableMedia: Boolean
        get() = player.playbackState != Player.STATE_IDLE

    val canControlPlayback: Boolean
        get() = player.playbackState == Player.STATE_READY
}
