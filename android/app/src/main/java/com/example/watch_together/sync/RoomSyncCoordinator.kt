package com.example.watch_together.sync

import com.example.watch_together.config.AppConfig
import com.example.watch_together.sync.protocol.PausePayload
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.SeekPayload
import com.example.watch_together.ui.player.PlayerAdapter
import kotlin.math.abs
import kotlin.math.roundToLong

data class DriftCheck(
    val localPositionMs: Long,
    val expectedPositionMs: Long,
    val driftMs: Long,
    val shouldCorrect: Boolean
)

class RoomSyncCoordinator(
    private val playerAdapter: PlayerAdapter
) {

    companion object {
        const val DEFAULT_DRIFT_THRESHOLD_MS = 750L
        const val DEFAULT_CORRECTION_INTERVAL_MS = 1_000L
    }

    // applyInitialState treats room_state as the authoritative baseline when a client
    // joins a room and maps that state onto the local player.
    fun applyInitialState(
        roomState: RoomSyncState,
        appliedAtMs: Long = System.currentTimeMillis()
    ): RoomSyncState {
        playerAdapter.load(AppConfig.mediaUrlFor(roomState.mediaId))
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
            seq = payload.seq,
            authorityAppliedAtMs = appliedAtMs
        )
    }

    // estimateExpectedPositionMs extrapolates where the local player should be now
    // based on the latest authoritative baseline and the elapsed local wall-clock time.
    fun estimateExpectedPositionMs(authorityState: RoomSyncState, nowMs: Long): Long {
        if (authorityState.paused || authorityState.authorityAppliedAtMs <= 0L) {
            return authorityState.positionMs
        }

        val elapsedMs = (nowMs - authorityState.authorityAppliedAtMs).coerceAtLeast(0L)
        val progressedMs = (elapsedMs * authorityState.playbackRate).roundToLong()
        return authorityState.positionMs + progressedMs
    }

    // evaluateDrift compares the local player with the extrapolated authority timeline
    // and decides whether one correction seek is worth doing.
    fun evaluateDrift(
        authorityState: RoomSyncState,
        nowMs: Long,
        lastCorrectionAtMs: Long,
        thresholdMs: Long = DEFAULT_DRIFT_THRESHOLD_MS,
        correctionIntervalMs: Long = DEFAULT_CORRECTION_INTERVAL_MS
    ): DriftCheck {
        val localPositionMs = playerAdapter.getCurrentPosition().coerceAtLeast(0L)
        val expectedPositionMs = estimateExpectedPositionMs(authorityState, nowMs)
        val driftMs = localPositionMs - expectedPositionMs

        val shouldCorrect = !authorityState.paused &&
            authorityState.authorityAppliedAtMs > 0L &&
            nowMs - authorityState.authorityAppliedAtMs >= correctionIntervalMs &&
            (lastCorrectionAtMs <= 0L || nowMs - lastCorrectionAtMs >= correctionIntervalMs) &&
            abs(driftMs) >= thresholdMs

        return DriftCheck(
            localPositionMs = localPositionMs,
            expectedPositionMs = expectedPositionMs,
            driftMs = driftMs,
            shouldCorrect = shouldCorrect
        )
    }

    // applyDriftCorrection pulls the local player back to the extrapolated authority
    // timeline without emitting a new sync event.
    fun applyDriftCorrection(check: DriftCheck, authorityState: RoomSyncState) {
        if (!check.shouldCorrect) return

        playerAdapter.seekTo(check.expectedPositionMs)
        if (authorityState.paused) {
            playerAdapter.pause()
        } else {
            playerAdapter.play()
        }
    }
}
