package com.example.watch_together.ui.player

import androidx.media3.common.Player
import com.example.watch_together.sync.RoomMember
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
    val availableVideoQualities: List<PlayerVideoQualityOption> = listOf(PlayerVideoQualityOption.Auto),
    val videoQualityPreference: PlayerVideoQualityPreference = PlayerVideoQualityPreference.Auto,
    val videoQualitySwitchState: PlayerVideoQualitySwitchState = PlayerVideoQualitySwitchState(),
    val videoQualityNotice: String = "",
) {
    val qualityPreferenceForSelectionUi: PlayerVideoQualityPreference
        get() = if (videoQualitySwitchState.isPending) {
            videoQualitySwitchState.preference
        } else {
            videoQualityPreference
        }

    val bufferedAheadMs: Long
        get() = (bufferedPosition - currentPosition).coerceAtLeast(0L)

    val effectiveBufferedAheadMs: Long
        get() {
            val safeSpeed = playbackSpeed.coerceAtLeast(0.25f)
            return (bufferedAheadMs / safeSpeed).toLong()
        }

    val estimatedSegmentsAhead: Int
        get() {
            if (effectiveBufferedAheadMs <= 0L) return 0
            return (effectiveBufferedAheadMs / DEFAULT_HLS_SEGMENT_DURATION_MS).toInt()
        }
}

private const val DEFAULT_HLS_SEGMENT_DURATION_MS = 6_000L

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
    val postSeekRecoveryCount: Int = 0,
    val lastSeekRecoveryReason: String = "",
    val lastRenderedFirstFrameAtMs: Long = 0L,
) {
    val isRebuffering: Boolean
        get() = activeRebufferStartedAtMs > 0L
}

data class RoomPlayerUiState(
    val joinRoomInput: String = "",
    val activeUserId: String? = null,
    val currentRoomId: String? = null,
    val roomMembers: List<RoomMember> = emptyList(),
    val latestSyncState: RoomSyncState? = null,
    val syncStatus: SyncStatus = SyncStatus.Idle,
    val player: PlayerRuntimeUiState = PlayerRuntimeUiState(),
    val telemetry: PlayerTelemetryUiState = PlayerTelemetryUiState(),
    val lastDriftCorrectionAtMs: Long = 0L,
    val lastEndedReportedSeq: Long = -1L,
    val awaitingFirstFrameAfterSeek: Boolean = false,
    val seekRecoveryDeadlineAtMs: Long = 0L,
    val seekRecoveryRetryCount: Int = 0,
) {
    val hasActiveRoomSession: Boolean
        get() = currentRoomId != null && latestSyncState != null

    val isJoinedToRoom: Boolean
        get() = latestSyncState != null

    val displayPlaybackSpeed: Float
        get() = latestSyncState?.playbackRate?.toFloat() ?: player.playbackSpeed

    val shouldKeepScreenOn: Boolean
        get() {
            if (latestSyncState?.ended == true) return false
            if (latestSyncState?.paused == true) return false
            if (player.isPlaying) return true
            return player.playbackState == Player.STATE_BUFFERING &&
                (player.currentPosition > 0L || player.bufferedPosition > 0L)
        }

    val shouldPauseAuthorityOnBackground: Boolean
        get() {
            if (latestSyncState == null) return false
            if (latestSyncState.ended || latestSyncState.paused) return false
            if (!isJoinedToRoom) return false
            return player.isPlaying || player.playbackState == Player.STATE_BUFFERING
        }

    val hasPlayableMedia: Boolean
        get() = player.playbackState != Player.STATE_IDLE

    val canControlPlayback: Boolean
        get() = player.playbackState == Player.STATE_READY

    fun afterJoinFailure(attemptedRoomId: String): RoomPlayerUiState {
        val retryingCurrentRoom = currentRoomId != null && currentRoomId == attemptedRoomId && hasActiveRoomSession
        if (retryingCurrentRoom) {
            return copy(joinRoomInput = attemptedRoomId)
        }
        return copy(
            joinRoomInput = attemptedRoomId,
            activeUserId = null,
            currentRoomId = null,
            roomMembers = emptyList(),
            latestSyncState = null,
            syncStatus = SyncStatus.SyncFailed,
            player = PlayerRuntimeUiState(),
            telemetry = PlayerTelemetryUiState(),
            lastDriftCorrectionAtMs = 0L,
            lastEndedReportedSeq = -1L,
            awaitingFirstFrameAfterSeek = false,
            seekRecoveryDeadlineAtMs = 0L,
            seekRecoveryRetryCount = 0
        )
    }
}
