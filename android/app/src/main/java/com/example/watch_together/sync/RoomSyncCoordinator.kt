package com.example.watch_together.sync

import com.example.watch_together.config.AppConfig
import com.example.watch_together.sync.protocol.EndedPayload
import com.example.watch_together.sync.protocol.PausePayload
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.SetPlaybackRatePayload
import com.example.watch_together.sync.protocol.SeekPayload
import com.example.watch_together.ui.player_default.PlayerAdapter
import kotlin.math.min
import kotlin.math.abs
import kotlin.math.roundToLong

enum class DriftCorrectionType {
    None,
    SpeedNudge,
    Seek
}

data class DriftCheck(
    val localPositionMs: Long,
    val expectedPositionMs: Long,
    val driftMs: Long,
    val shouldCorrect: Boolean,
    val correctionType: DriftCorrectionType = DriftCorrectionType.None,
    val speedNudgeRate: Float = 1f,
    val speedNudgeDurationMs: Long = RoomSyncCoordinator.DEFAULT_SPEED_NUDGE_DURATION_MS
)

class RoomSyncCoordinator(
    private val playerAdapter: PlayerAdapter,
    private val mediaUrlFor: (String) -> String = AppConfig::mediaUrlFor
) {

    companion object {
        const val DEFAULT_DRIFT_THRESHOLD_MS = 150L
        const val DEFAULT_CORRECTION_INTERVAL_MS = 1_000L
        const val DEFAULT_SEEK_DRIFT_THRESHOLD_MS = 2_000L
        const val DEFAULT_SPEED_NUDGE_DURATION_MS = 1_500L
        private const val SPEED_NUDGE_DELTA = 0.03f
        private const val MIN_SPEED_NUDGE_RATE = 0.25f
        private const val MAX_SPEED_NUDGE_RATE = 2.25f
    }

    // applyInitialState treats room_state as the authoritative baseline when a client
    // joins a room and maps that state onto the local player.
    fun applyInitialState(
        roomState: RoomSyncState,
        appliedAtMs: Long = System.currentTimeMillis()
    ): RoomSyncState {
        playerAdapter.load(mediaUrlFor(roomState.mediaId))
        playerAdapter.seekTo(roomState.positionMs)
        playerAdapter.setPlaybackSpeed(roomState.playbackRate.toFloat())

        // Join-time sync prefers matching the server paused flag over preserving any local playback.
        if (roomState.paused) {
            playerAdapter.pause()
        } else {
            playerAdapter.play()
        }

        return roomState.copy(authorityAppliedAtMs = appliedAtMs)
    }

    // applyPlayEvent updates the local baseline from an authoritative play broadcast.
    fun applyPlayEvent(
        previous: RoomSyncState,
        payload: PlayPayload,
        appliedAtMs: Long = System.currentTimeMillis()
    ): RoomSyncState {
        playerAdapter.seekTo(payload.positionMs)
        playerAdapter.play()

        return previous.copy(
            positionMs = payload.positionMs,
            paused = false,
            ended = false,
            seq = payload.seq,
            authorityAppliedAtMs = appliedAtMs
        )
    }

    // applyPauseEvent updates the local baseline from an authoritative pause broadcast.
    fun applyPauseEvent(
        previous: RoomSyncState,
        payload: PausePayload,
        appliedAtMs: Long = System.currentTimeMillis()
    ): RoomSyncState {
        playerAdapter.seekTo(payload.positionMs)
        playerAdapter.pause()

        return previous.copy(
            positionMs = payload.positionMs,
            paused = true,
            ended = false,
            seq = payload.seq,
            authorityAppliedAtMs = appliedAtMs
        )
    }

    // applySeekEvent keeps the local paused flag aligned while moving to the new position.
    fun applySeekEvent(
        previous: RoomSyncState,
        payload: SeekPayload,
        appliedAtMs: Long = System.currentTimeMillis()
    ): RoomSyncState {
        playerAdapter.seekTo(payload.positionMs)
        if (previous.paused) {
            playerAdapter.pause()
        } else {
            playerAdapter.play()
        }

        return previous.copy(
            positionMs = payload.positionMs,
            ended = false,
            seq = payload.seq,
            authorityAppliedAtMs = appliedAtMs
        )
    }

    // applyPlaybackRateEvent keeps timeline continuity by applying the settled position and rate together.
    fun applyPlaybackRateEvent(
        previous: RoomSyncState,
        payload: SetPlaybackRatePayload,
        appliedAtMs: Long = System.currentTimeMillis()
    ): RoomSyncState {
        playerAdapter.seekTo(payload.positionMs)
        playerAdapter.setPlaybackSpeed(payload.playbackRate.toFloat())
        if (previous.paused) {
            playerAdapter.pause()
        } else {
            playerAdapter.play()
        }

        return previous.copy(
            positionMs = payload.positionMs,
            playbackRate = payload.playbackRate,
            seq = payload.seq,
            authorityAppliedAtMs = appliedAtMs
        )
    }

    // applyEndedEvent moves the local player into a stable completed state.
    fun applyEndedEvent(
        previous: RoomSyncState,
        payload: EndedPayload,
        appliedAtMs: Long = System.currentTimeMillis()
    ): RoomSyncState {
        playerAdapter.seekTo(payload.positionMs)
        playerAdapter.pause()

        return previous.copy(
            paused = true,
            ended = true,
            positionMs = payload.positionMs,
            seq = payload.seq,
            authorityAppliedAtMs = appliedAtMs
        )
    }

    // estimateExpectedPositionMs extrapolates where the local player should be now
    // based on the latest authoritative baseline and the elapsed local wall-clock time.
    fun estimateExpectedPositionMs(authorityState: RoomSyncState, nowMs: Long): Long {
        if (authorityState.paused || authorityState.ended || authorityState.authorityAppliedAtMs <= 0L) {
            return authorityState.positionMs
        }

        val elapsedMs = (nowMs - authorityState.authorityAppliedAtMs).coerceAtLeast(0L)
        val progressedMs = (elapsedMs * authorityState.playbackRate).roundToLong()
        return authorityState.positionMs + progressedMs
    }

    fun capExpectedPositionToDuration(expectedPositionMs: Long, durationMs: Long): Long {
        if (durationMs <= 0L) {
            return expectedPositionMs
        }
        return min(expectedPositionMs, durationMs)
    }

    // evaluateDrift compares the local player with the extrapolated authority timeline.
    // Small drift is corrected with a temporary speed nudge; large drift falls back to seek.
    fun evaluateDrift(
        authorityState: RoomSyncState,
        nowMs: Long,
        lastCorrectionAtMs: Long,
        durationMs: Long = 0L,
        playbackEnded: Boolean = false,
        playbackBuffering: Boolean = false,
        thresholdMs: Long = DEFAULT_DRIFT_THRESHOLD_MS,
        seekThresholdMs: Long = DEFAULT_SEEK_DRIFT_THRESHOLD_MS,
        correctionIntervalMs: Long = DEFAULT_CORRECTION_INTERVAL_MS
    ): DriftCheck {
        val localPositionMs = playerAdapter.getCurrentPosition().coerceAtLeast(0L)
        val expectedPositionMs = capExpectedPositionToDuration(
            estimateExpectedPositionMs(authorityState, nowMs),
            durationMs
        )
        val driftMs = localPositionMs - expectedPositionMs

        val canCorrect = !playbackEnded &&
            !playbackBuffering &&
            !authorityState.paused &&
            !authorityState.ended &&
            authorityState.authorityAppliedAtMs > 0L &&
            nowMs - authorityState.authorityAppliedAtMs >= correctionIntervalMs &&
            (lastCorrectionAtMs <= 0L || nowMs - lastCorrectionAtMs >= correctionIntervalMs) &&
            (durationMs <= 0L || localPositionMs < durationMs)

        val correctionType = when {
            !canCorrect || abs(driftMs) < thresholdMs -> DriftCorrectionType.None
            abs(driftMs) >= seekThresholdMs -> DriftCorrectionType.Seek
            else -> DriftCorrectionType.SpeedNudge
        }

        return DriftCheck(
            localPositionMs = localPositionMs,
            expectedPositionMs = expectedPositionMs,
            driftMs = driftMs,
            shouldCorrect = correctionType != DriftCorrectionType.None,
            correctionType = correctionType,
            speedNudgeRate = calculateSpeedNudgeRate(authorityState.playbackRate, driftMs)
        )
    }

    // applyDriftCorrection adjusts local playback without emitting a new sync event.
    fun applyDriftCorrection(check: DriftCheck, authorityState: RoomSyncState) {
        if (!check.shouldCorrect) return

        when (check.correctionType) {
            DriftCorrectionType.None -> Unit
            DriftCorrectionType.SpeedNudge -> {
                playerAdapter.setPlaybackSpeed(check.speedNudgeRate)
            }
            DriftCorrectionType.Seek -> {
                playerAdapter.seekTo(check.expectedPositionMs)
                if (authorityState.paused) {
                    playerAdapter.pause()
                } else {
                    playerAdapter.play()
                }
            }
        }
    }

    fun restoreAuthorityPlaybackRate(authorityState: RoomSyncState) {
        playerAdapter.setPlaybackSpeed(authorityState.playbackRate.toFloat())
    }

    private fun calculateSpeedNudgeRate(authorityPlaybackRate: Double, driftMs: Long): Float {
        val baseRate = authorityPlaybackRate.toFloat()
        val multiplier = if (driftMs > 0L) {
            1f - SPEED_NUDGE_DELTA
        } else {
            1f + SPEED_NUDGE_DELTA
        }
        return (baseRate * multiplier).coerceIn(MIN_SPEED_NUDGE_RATE, MAX_SPEED_NUDGE_RATE)
    }
}
