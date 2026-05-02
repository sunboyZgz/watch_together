package com.example.watch_together.sync

import androidx.media3.ui.PlayerView
import com.example.watch_together.sync.protocol.EndedPayload
import com.example.watch_together.sync.protocol.PausePayload
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.SetPlaybackRatePayload
import com.example.watch_together.sync.protocol.SeekPayload
import com.example.watch_together.ui.player.PlayerAdapter
import com.example.watch_together.ui.player.PlayerEvent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RoomSyncCoordinatorTest {

    @Test
    fun `applyInitialState maps room_state fields onto player adapter`() {
        val fakePlayerAdapter = FakePlayerAdapter()
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val roomState = RoomSyncState(
            roomId = "ROOM01",
            mediaId = "custom_media",
            hostUserId = "user_a",
            paused = false,
            ended = false,
            positionMs = 42_000L,
            playbackRate = 1.25,
            seq = 2L
        )

        val applied = coordinator.applyInitialState(roomState, appliedAtMs = 1_000L)

        assertTrue(fakePlayerAdapter.loadedUrl.endsWith("/custom_media/index.m3u8"))
        assertEquals(42_000L, fakePlayerAdapter.seekPositionMs)
        assertEquals(1.25f, fakePlayerAdapter.speed, 0.0f)
        assertTrue(fakePlayerAdapter.playCalled)
        assertFalse(fakePlayerAdapter.pauseCalled)
        assertEquals(1_000L, applied.authorityAppliedAtMs)
    }

    @Test
    fun `applyPlayEvent seeks then plays and advances seq`() {
        val fakePlayerAdapter = FakePlayerAdapter()
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val previous = RoomSyncState("ROOM01", "sample_001", "user_a", true, false, 10_000L, 1.0, 1L)

        val next = coordinator.applyPlayEvent(
            previous,
            PlayPayload("ROOM01", "user_a", 12_000L, 2L),
            appliedAtMs = 2_000L
        )

        assertEquals(12_000L, fakePlayerAdapter.seekPositionMs)
        assertTrue(fakePlayerAdapter.playCalled)
        assertFalse(next.paused)
        assertEquals(2L, next.seq)
        assertEquals(2_000L, next.authorityAppliedAtMs)
    }

    @Test
    fun `applyPauseEvent seeks then pauses and advances seq`() {
        val fakePlayerAdapter = FakePlayerAdapter()
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val previous = RoomSyncState("ROOM01", "sample_001", "user_a", false, false, 10_000L, 1.0, 2L)

        val next = coordinator.applyPauseEvent(
            previous,
            PausePayload("ROOM01", "user_a", 15_000L, 3L),
            appliedAtMs = 3_000L
        )

        assertEquals(15_000L, fakePlayerAdapter.seekPositionMs)
        assertTrue(fakePlayerAdapter.pauseCalled)
        assertTrue(next.paused)
        assertEquals(3L, next.seq)
        assertEquals(3_000L, next.authorityAppliedAtMs)
    }

    @Test
    fun `applySeekEvent preserves paused flag while moving position`() {
        val fakePlayerAdapter = FakePlayerAdapter()
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val previous = RoomSyncState("ROOM01", "sample_001", "user_a", true, false, 15_000L, 1.0, 3L)

        val next = coordinator.applySeekEvent(
            previous,
            SeekPayload("ROOM01", "user_a", 42_000L, 4L),
            appliedAtMs = 4_000L
        )

        assertEquals(42_000L, fakePlayerAdapter.seekPositionMs)
        assertTrue(fakePlayerAdapter.pauseCalled)
        assertTrue(next.paused)
        assertEquals(42_000L, next.positionMs)
        assertEquals(4L, next.seq)
        assertEquals(4_000L, next.authorityAppliedAtMs)
    }

    @Test
    fun `applyPlaybackRateEvent updates speed and preserves play state`() {
        val fakePlayerAdapter = FakePlayerAdapter()
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val previous = RoomSyncState("ROOM01", "sample_001", "user_a", false, false, 15_000L, 1.0, 4L)

        val next = coordinator.applyPlaybackRateEvent(
            previous,
            SetPlaybackRatePayload("ROOM01", "user_a", 18_000L, 1.5, 5L),
            appliedAtMs = 5_000L
        )

        assertEquals(18_000L, fakePlayerAdapter.seekPositionMs)
        assertEquals(1.5f, fakePlayerAdapter.speed, 0.0f)
        assertTrue(fakePlayerAdapter.playCalled)
        assertEquals(18_000L, next.positionMs)
        assertEquals(1.5, next.playbackRate, 0.0)
        assertEquals(5L, next.seq)
        assertEquals(5_000L, next.authorityAppliedAtMs)
    }

    @Test
    fun `applyEndedEvent freezes player into completed state`() {
        val fakePlayerAdapter = FakePlayerAdapter()
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val previous = RoomSyncState("ROOM01", "sample_001", "user_a", false, false, 18_000L, 1.5, 5L)

        val next = coordinator.applyEndedEvent(
            previous,
            EndedPayload("ROOM01", "user_a", 210_000L, 6L),
            appliedAtMs = 6_000L
        )

        assertEquals(210_000L, fakePlayerAdapter.seekPositionMs)
        assertTrue(fakePlayerAdapter.pauseCalled)
        assertTrue(next.paused)
        assertTrue(next.ended)
        assertEquals(6L, next.seq)
    }

    @Test
    fun `estimateExpectedPositionMs extrapolates from authority baseline`() {
        val fakePlayerAdapter = FakePlayerAdapter()
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val authority = RoomSyncState(
            roomId = "ROOM01",
            mediaId = "sample_001",
            hostUserId = "user_a",
            paused = false,
            ended = false,
            positionMs = 10_000L,
            playbackRate = 1.0,
            seq = 5L,
            authorityAppliedAtMs = 2_000L
        )

        val expected = coordinator.estimateExpectedPositionMs(authority, nowMs = 5_000L)

        assertEquals(13_000L, expected)
    }

    @Test
    fun `evaluateDrift requests correction when drift exceeds threshold`() {
        val fakePlayerAdapter = FakePlayerAdapter().apply {
            currentPositionMs = 15_000L
        }
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val authority = RoomSyncState(
            roomId = "ROOM01",
            mediaId = "sample_001",
            hostUserId = "user_a",
            paused = false,
            ended = false,
            positionMs = 10_000L,
            playbackRate = 1.0,
            seq = 5L,
            authorityAppliedAtMs = 2_000L
        )

        val check = coordinator.evaluateDrift(
            authorityState = authority,
            nowMs = 5_000L,
            lastCorrectionAtMs = 0L,
            thresholdMs = 750L,
            correctionIntervalMs = 1_000L
        )

        assertEquals(15_000L, check.localPositionMs)
        assertEquals(13_000L, check.expectedPositionMs)
        assertEquals(2_000L, check.driftMs)
        assertTrue(check.shouldCorrect)
    }

    @Test
    fun `evaluateDrift skips correction after playback ended`() {
        val fakePlayerAdapter = FakePlayerAdapter().apply {
            currentPositionMs = 15_000L
        }
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val authority = RoomSyncState(
            roomId = "ROOM01",
            mediaId = "sample_001",
            hostUserId = "user_a",
            paused = false,
            ended = false,
            positionMs = 10_000L,
            playbackRate = 1.0,
            seq = 5L,
            authorityAppliedAtMs = 2_000L
        )

        val check = coordinator.evaluateDrift(
            authorityState = authority,
            nowMs = 5_000L,
            lastCorrectionAtMs = 0L,
            durationMs = 15_000L,
            playbackEnded = true,
            thresholdMs = 750L,
            correctionIntervalMs = 1_000L
        )

        assertFalse(check.shouldCorrect)
    }

    @Test
    fun `evaluateDrift skips correction while local player is buffering`() {
        val fakePlayerAdapter = FakePlayerAdapter().apply {
            currentPositionMs = 15_000L
        }
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val authority = RoomSyncState(
            roomId = "ROOM01",
            mediaId = "sample_001",
            hostUserId = "user_a",
            paused = false,
            ended = false,
            positionMs = 10_000L,
            playbackRate = 1.0,
            seq = 5L,
            authorityAppliedAtMs = 2_000L
        )

        val check = coordinator.evaluateDrift(
            authorityState = authority,
            nowMs = 5_000L,
            lastCorrectionAtMs = 0L,
            playbackBuffering = true,
            thresholdMs = 750L,
            correctionIntervalMs = 1_000L
        )

        assertEquals(2_000L, check.driftMs)
        assertFalse(check.shouldCorrect)
    }

    @Test
    fun `evaluateDrift caps expected position to duration`() {
        val fakePlayerAdapter = FakePlayerAdapter().apply {
            currentPositionMs = 10_000L
        }
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val authority = RoomSyncState(
            roomId = "ROOM01",
            mediaId = "sample_001",
            hostUserId = "user_a",
            paused = false,
            ended = false,
            positionMs = 9_000L,
            playbackRate = 1.0,
            seq = 5L,
            authorityAppliedAtMs = 2_000L
        )

        val check = coordinator.evaluateDrift(
            authorityState = authority,
            nowMs = 5_000L,
            lastCorrectionAtMs = 0L,
            durationMs = 10_000L,
            playbackEnded = false,
            thresholdMs = 750L,
            correctionIntervalMs = 1_000L
        )

        assertEquals(10_000L, check.expectedPositionMs)
        assertFalse(check.shouldCorrect)
    }

    @Test
    fun `applyDriftCorrection seeks to expected position without changing authority seq`() {
        val fakePlayerAdapter = FakePlayerAdapter()
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val authority = RoomSyncState(
            roomId = "ROOM01",
            mediaId = "sample_001",
            hostUserId = "user_a",
            paused = false,
            ended = false,
            positionMs = 10_000L,
            playbackRate = 1.0,
            seq = 5L,
            authorityAppliedAtMs = 2_000L
        )
        val check = DriftCheck(
            localPositionMs = 15_000L,
            expectedPositionMs = 13_000L,
            driftMs = 2_000L,
            shouldCorrect = true
        )

        coordinator.applyDriftCorrection(check, authority)

        assertEquals(13_000L, fakePlayerAdapter.seekPositionMs)
        assertTrue(fakePlayerAdapter.playCalled)
    }

    @Test
    fun `evaluateDrift skips correction when authority state is ended`() {
        val fakePlayerAdapter = FakePlayerAdapter().apply {
            currentPositionMs = 15_000L
        }
        val coordinator = RoomSyncCoordinator(fakePlayerAdapter)
        val authority = RoomSyncState(
            roomId = "ROOM01",
            mediaId = "sample_001",
            hostUserId = "user_a",
            paused = true,
            ended = true,
            positionMs = 210_000L,
            playbackRate = 1.0,
            seq = 6L,
            authorityAppliedAtMs = 2_000L
        )

        val check = coordinator.evaluateDrift(
            authorityState = authority,
            nowMs = 5_000L,
            lastCorrectionAtMs = 0L,
            thresholdMs = 750L,
            correctionIntervalMs = 1_000L
        )

        assertFalse(check.shouldCorrect)
        assertEquals(210_000L, check.expectedPositionMs)
    }
}

private class FakePlayerAdapter : PlayerAdapter {
    var loadedUrl: String = ""
    var seekPositionMs: Long = -1L
    var speed: Float = 1f
    var playCalled: Boolean = false
    var pauseCalled: Boolean = false
    var currentPositionMs: Long = 0L

    override fun attach(playerView: PlayerView) = Unit

    override fun detach() = Unit

    override fun setEventListener(listener: ((PlayerEvent) -> Unit)?) = Unit

    override fun load(url: String) {
        loadedUrl = url
    }

    override fun play() {
        playCalled = true
    }

    override fun pause() {
        pauseCalled = true
    }

    override fun seekTo(positionMs: Long) {
        seekPositionMs = positionMs
    }

    override fun getCurrentPosition(): Long = currentPositionMs

    override fun getDuration(): Long = 0L

    override fun isPlaying(): Boolean = false

    override fun setPlaybackSpeed(speed: Float) {
        this.speed = speed
    }

    override fun release() = Unit
}
