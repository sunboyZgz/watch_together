package com.example.watch_together.ui.player_default

import androidx.media3.ui.PlayerView
import com.example.watch_together.sync.RoomSyncCoordinator
import com.example.watch_together.sync.RoomSyncState
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.RoomStatePayload
import com.example.watch_together.sync.protocol.SeekPayload
import com.example.watch_together.sync.toRoomSyncState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class RoomPlayerSyncEventHandlerTest {

    @Test
    fun `room_state updates page state and playback speed`() {
        val handler = RoomPlayerSyncEventHandler(RoomSyncCoordinator(FakePlayerAdapter()))
        val current = RoomPlayerUiState()
        val roomState = RoomStatePayload(
            roomId = "ROOM01",
            mediaId = "sample_001",
            hostUserId = "user_a",
            paused = false,
            ended = false,
            positionMs = 12_000L,
            playbackRate = 1.5,
            seq = 3L
        ).toRoomSyncState(appliedAtMs = 1_000L)

        val result = handler.onRoomState(current, roomState)

        assertEquals("ROOM01", result.uiState.currentRoomId)
        assertEquals("ROOM01", result.uiState.joinRoomInput)
        assertEquals(SyncStatus.RoomStateApplied, result.uiState.syncStatus)
        assertEquals(1.5f, result.uiState.player.playbackSpeed, 0.0f)
        assertTrue(result.logs.any { it.contains("Applied room_state") })
    }

    @Test
    fun `stale play event is ignored`() {
        val handler = RoomPlayerSyncEventHandler(RoomSyncCoordinator(FakePlayerAdapter()))
        val current = RoomPlayerUiState(
            latestSyncState = RoomSyncState(
                roomId = "ROOM01",
                mediaId = "sample_001",
                hostUserId = "user_a",
                paused = false,
                ended = false,
                positionMs = 10_000L,
                playbackRate = 1.0,
                seq = 4L
            )
        )

        val result = handler.onPlay(
            current,
            PlayPayload(
                roomId = "ROOM01",
                userId = "user_a",
                positionMs = 12_000L,
                seq = 4L
            )
        )

        assertEquals(current, result.uiState)
        assertTrue(result.logs.any { it.contains("Ignored stale play") })
    }

    @Test
    fun `heartbeat only updates sync status`() {
        val handler = RoomPlayerSyncEventHandler(RoomSyncCoordinator(FakePlayerAdapter()))
        val current = RoomPlayerUiState(syncStatus = SyncStatus.JoiningAsViewer)

        val result = handler.onHeartbeat(current, serverTimeMs = 123L)

        assertEquals(SyncStatus.Connected, result.uiState.syncStatus)
        assertTrue(result.logs.any { it.contains("heartbeat acknowledged") })
    }

    @Test
    fun `seek event enables post seek recovery window`() {
        val handler = RoomPlayerSyncEventHandler(RoomSyncCoordinator(FakePlayerAdapter()))
        val current = RoomPlayerUiState(
            latestSyncState = RoomSyncState(
                roomId = "ROOM01",
                mediaId = "sample_001",
                hostUserId = "user_a",
                paused = false,
                ended = false,
                positionMs = 10_000L,
                playbackRate = 1.0,
                seq = 4L
            )
        )

        val result = handler.onSeek(
            current,
            SeekPayload(
                roomId = "ROOM01",
                userId = "user_a",
                positionMs = 42_000L,
                seq = 5L
            )
        )

        assertTrue(result.uiState.awaitingFirstFrameAfterSeek)
        assertTrue(result.uiState.seekRecoveryDeadlineAtMs > 0L)
    }
}

private class FakePlayerAdapter : PlayerAdapter {
    override fun attach(playerView: PlayerView) = Unit
    override fun detach() = Unit
    override fun load(url: String) = Unit
    override fun play() = Unit
    override fun pause() = Unit
    override fun seekTo(positionMs: Long) = Unit
    override fun reset() = Unit
    override fun getCurrentPosition(): Long = 0L
    override fun getDuration(): Long = 0L
    override fun isPlaying(): Boolean = false
    override fun setPlaybackSpeed(speed: Float) = Unit
    override fun release() = Unit
    override fun setEventListener(listener: ((PlayerEvent) -> Unit)?) = Unit
}
