package com.example.watch_together.sync

import com.example.watch_together.config.AppConfig
import com.example.watch_together.ui.player.PlayerAdapter

class RoomSyncCoordinator(
    private val playerAdapter: PlayerAdapter
) {

    // applyInitialState treats room_state as the authoritative baseline when a client
    // joins a room and maps that state onto the local player.
    fun applyInitialState(roomState: RoomSyncState): RoomSyncState {
        playerAdapter.load(AppConfig.mediaUrlFor(roomState.mediaId))
        playerAdapter.seekTo(roomState.positionMs)
        playerAdapter.setPlaybackSpeed(roomState.playbackRate.toFloat())

        // Join-time sync prefers matching the server paused flag over preserving any local playback.
        if (roomState.paused) {
            playerAdapter.pause()
        } else {
            playerAdapter.play()
        }

        return roomState
    }
}
