package com.example.watch_together.sync

import com.example.watch_together.config.AppConfig
import com.example.watch_together.sync.protocol.PausePayload
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.SeekPayload
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

    // applyPlayEvent updates the local baseline from an authoritative play broadcast.
    fun applyPlayEvent(previous: RoomSyncState, payload: PlayPayload): RoomSyncState {
        playerAdapter.seekTo(payload.positionMs)
        playerAdapter.play()

        return previous.copy(
            positionMs = payload.positionMs,
            paused = false,
            seq = payload.seq
        )
    }

    // applyPauseEvent updates the local baseline from an authoritative pause broadcast.
    fun applyPauseEvent(previous: RoomSyncState, payload: PausePayload): RoomSyncState {
        playerAdapter.seekTo(payload.positionMs)
        playerAdapter.pause()

        return previous.copy(
            positionMs = payload.positionMs,
            paused = true,
            seq = payload.seq
        )
    }

    // applySeekEvent keeps the local paused flag aligned while moving to the new position.
    fun applySeekEvent(previous: RoomSyncState, payload: SeekPayload): RoomSyncState {
        playerAdapter.seekTo(payload.positionMs)
        if (previous.paused) {
            playerAdapter.pause()
        } else {
            playerAdapter.play()
        }

        return previous.copy(
            positionMs = payload.positionMs,
            seq = payload.seq
        )
    }
}
