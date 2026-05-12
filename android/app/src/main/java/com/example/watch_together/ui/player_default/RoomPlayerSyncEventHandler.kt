package com.example.watch_together.ui.player_default

import com.example.watch_together.sync.RoomSyncCoordinator
import com.example.watch_together.sync.RoomSyncState
import com.example.watch_together.sync.isNewerThan
import com.example.watch_together.sync.protocol.EndedPayload
import com.example.watch_together.sync.protocol.PausePayload
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.SetPlaybackRatePayload
import com.example.watch_together.sync.protocol.SeekPayload

data class RoomPlayerSyncEventResult(
    val uiState: RoomPlayerUiState,
    val logs: List<String> = emptyList()
)

class RoomPlayerSyncEventHandler(
    private val roomSyncCoordinator: RoomSyncCoordinator
) {
    companion object {
        private const val SEEK_RECOVERY_GRACE_WINDOW_MS = 2_500L
    }

    fun onRoomState(
        currentUiState: RoomPlayerUiState,
        payload: RoomSyncState
    ): RoomPlayerSyncEventResult {
        val appliedState = roomSyncCoordinator.applyInitialState(payload)
        return applyAuthoritativeState(
            currentUiState = currentUiState,
            newState = appliedState,
            status = SyncStatus.RoomStateApplied,
            reason = "room_state"
        )
    }

    fun onPlay(
        currentUiState: RoomPlayerUiState,
        payload: PlayPayload
    ): RoomPlayerSyncEventResult {
        val previous = currentUiState.latestSyncState
            ?: return ignored(currentUiState, "Ignored play before room_state")
        if (payload.seq <= previous.seq) {
            return ignored(currentUiState, "Ignored stale play seq=${payload.seq}")
        }

        val appliedState = roomSyncCoordinator.applyPlayEvent(previous, payload)
        return applyAuthoritativeState(currentUiState, appliedState, SyncStatus.PlayApplied, "play")
    }

    fun onPause(
        currentUiState: RoomPlayerUiState,
        payload: PausePayload
    ): RoomPlayerSyncEventResult {
        val previous = currentUiState.latestSyncState
            ?: return ignored(currentUiState, "Ignored pause before room_state")
        if (payload.seq <= previous.seq) {
            return ignored(currentUiState, "Ignored stale pause seq=${payload.seq}")
        }

        val appliedState = roomSyncCoordinator.applyPauseEvent(previous, payload)
        return applyAuthoritativeState(currentUiState, appliedState, SyncStatus.PauseApplied, "pause")
    }

    fun onSeek(
        currentUiState: RoomPlayerUiState,
        payload: SeekPayload
    ): RoomPlayerSyncEventResult {
        val previous = currentUiState.latestSyncState
            ?: return ignored(currentUiState, "Ignored seek before room_state")
        if (payload.seq <= previous.seq) {
            return ignored(currentUiState, "Ignored stale seek seq=${payload.seq}")
        }

        val appliedState = roomSyncCoordinator.applySeekEvent(previous, payload)
        return applyAuthoritativeState(currentUiState, appliedState, SyncStatus.SeekApplied, "seek")
    }

    fun onPlaybackRate(
        currentUiState: RoomPlayerUiState,
        payload: SetPlaybackRatePayload
    ): RoomPlayerSyncEventResult {
        val previous = currentUiState.latestSyncState
            ?: return ignored(currentUiState, "Ignored playback rate before room_state")
        if (payload.seq <= previous.seq) {
            return ignored(currentUiState, "Ignored stale playback rate seq=${payload.seq}")
        }

        val appliedState = roomSyncCoordinator.applyPlaybackRateEvent(previous, payload)
        return applyAuthoritativeState(
            currentUiState = currentUiState,
            newState = appliedState,
            status = SyncStatus.PlaybackRateApplied,
            reason = "set_playback_rate",
            extraLogs = listOf(
                "received set_playback_rate rate=${payload.playbackRate} seq=${payload.seq} position=${payload.positionMs}"
            )
        )
    }

    fun onEnded(
        currentUiState: RoomPlayerUiState,
        payload: EndedPayload
    ): RoomPlayerSyncEventResult {
        val previous = currentUiState.latestSyncState
            ?: return ignored(currentUiState, "Ignored ended before room_state")
        if (payload.seq <= previous.seq) {
            return ignored(currentUiState, "Ignored stale ended seq=${payload.seq}")
        }

        val appliedState = roomSyncCoordinator.applyEndedEvent(previous, payload)
        return applyAuthoritativeState(
            currentUiState = currentUiState,
            newState = appliedState,
            status = SyncStatus.EndedApplied,
            reason = "ended",
            extraLogs = listOf(
                "received ended seq=${payload.seq} position=${payload.positionMs}"
            )
        )
    }

    fun onHeartbeat(
        currentUiState: RoomPlayerUiState,
        serverTimeMs: Long
    ): RoomPlayerSyncEventResult {
        return RoomPlayerSyncEventResult(
            uiState = currentUiState.copy(syncStatus = SyncStatus.Connected),
            logs = listOf("heartbeat acknowledged serverTimeMs=$serverTimeMs")
        )
    }

    fun onError(
        currentUiState: RoomPlayerUiState,
        message: String
    ): RoomPlayerSyncEventResult {
        return RoomPlayerSyncEventResult(
            uiState = currentUiState.copy(syncStatus = SyncStatus.SyncFailed),
            logs = listOf("Sync error: $message")
        )
    }

    private fun applyAuthoritativeState(
        currentUiState: RoomPlayerUiState,
        newState: RoomSyncState,
        status: SyncStatus,
        reason: String,
        extraLogs: List<String> = emptyList()
    ): RoomPlayerSyncEventResult {
        if (!newState.isNewerThan(currentUiState.latestSyncState)) {
            return ignored(currentUiState, "Ignored stale $reason seq=${newState.seq}")
        }

        val nextUiState = currentUiState.copy(
            currentRoomId = newState.roomId,
            joinRoomInput = newState.roomId,
            latestSyncState = newState,
            syncStatus = status,
            player = currentUiState.player.copy(playbackSpeed = newState.playbackRate.toFloat()),
            awaitingFirstFrameAfterSeek = when (reason) {
                "seek" -> true
                "pause", "ended" -> false
                else -> currentUiState.awaitingFirstFrameAfterSeek
            },
            seekRecoveryDeadlineAtMs = if (reason == "seek") {
                System.currentTimeMillis() + SEEK_RECOVERY_GRACE_WINDOW_MS
            } else if (reason == "pause" || reason == "ended") {
                0L
            } else {
                currentUiState.seekRecoveryDeadlineAtMs
            },
            seekRecoveryRetryCount = if (reason == "seek") {
                0
            } else if (reason == "pause" || reason == "ended") {
                0
            } else {
                currentUiState.seekRecoveryRetryCount
            },
            lastDriftCorrectionAtMs = if (reason == "seek") {
                System.currentTimeMillis()
            } else {
                currentUiState.lastDriftCorrectionAtMs
            }
        )

        return RoomPlayerSyncEventResult(
            uiState = nextUiState,
            logs = extraLogs + "Applied $reason seq=${newState.seq} roomId=${newState.roomId}"
        )
    }

    private fun ignored(
        currentUiState: RoomPlayerUiState,
        message: String
    ): RoomPlayerSyncEventResult {
        return RoomPlayerSyncEventResult(
            uiState = currentUiState,
            logs = listOf(message)
        )
    }
}
