package com.example.watch_together.ui.player

import androidx.media3.common.Player
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RoomPlayerUiStateTest {

    @Test
    fun `idle player cannot be controlled`() {
        val state = RoomPlayerUiState(
            player = PlayerRuntimeUiState(playbackState = Player.STATE_IDLE)
        )

        assertFalse(state.hasPlayableMedia)
        assertFalse(state.canControlPlayback)
    }

    @Test
    fun `ready player can be controlled`() {
        val state = RoomPlayerUiState(
            player = PlayerRuntimeUiState(playbackState = Player.STATE_READY)
        )

        assertTrue(state.hasPlayableMedia)
        assertTrue(state.canControlPlayback)
    }

    @Test
    fun `buffering player is loaded but cannot be controlled yet`() {
        val state = RoomPlayerUiState(
            player = PlayerRuntimeUiState(playbackState = Player.STATE_BUFFERING)
        )

        assertTrue(state.hasPlayableMedia)
        assertFalse(state.canControlPlayback)
    }

    @Test
    fun `ended player is loaded but cannot start normal controls`() {
        val state = RoomPlayerUiState(
            player = PlayerRuntimeUiState(playbackState = Player.STATE_ENDED)
        )

        assertTrue(state.hasPlayableMedia)
        assertFalse(state.canControlPlayback)
    }

    @Test
    fun `buffered ahead never returns negative value`() {
        val state = PlayerRuntimeUiState(
            currentPosition = 12_000L,
            bufferedPosition = 10_000L
        )

        assertFalse(state.bufferedAheadMs < 0L)
    }
}
