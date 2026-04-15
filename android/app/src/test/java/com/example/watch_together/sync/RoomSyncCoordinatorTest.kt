package com.example.watch_together.sync

import androidx.media3.ui.PlayerView
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
            positionMs = 42_000L,
            playbackRate = 1.25,
            seq = 2L
        )

        coordinator.applyInitialState(roomState)

        assertTrue(fakePlayerAdapter.loadedUrl.endsWith("/custom_media/index.m3u8"))
        assertEquals(42_000L, fakePlayerAdapter.seekPositionMs)
        assertEquals(1.25f, fakePlayerAdapter.speed, 0.0f)
        assertTrue(fakePlayerAdapter.playCalled)
        assertFalse(fakePlayerAdapter.pauseCalled)
    }
}

private class FakePlayerAdapter : PlayerAdapter {
    var loadedUrl: String = ""
    var seekPositionMs: Long = -1L
    var speed: Float = 1f
    var playCalled: Boolean = false
    var pauseCalled: Boolean = false

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

    override fun getCurrentPosition(): Long = 0L

    override fun getDuration(): Long = 0L

    override fun isPlaying(): Boolean = false

    override fun setPlaybackSpeed(speed: Float) {
        this.speed = speed
    }

    override fun release() = Unit
}
