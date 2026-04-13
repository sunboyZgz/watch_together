package com.example.watch_together.ui.player

import androidx.media3.common.Player

sealed interface PlayerEvent {
    data object Ready : PlayerEvent
    data class PlayWhenReadyChanged(
        val playWhenReady: Boolean,
        val reason: Int
    ) : PlayerEvent

    data class IsPlayingChanged(
        val isPlaying: Boolean
    ) : PlayerEvent

    data class PlaybackStateChanged(
        val playbackState: Int
    ) : PlayerEvent

    data class PositionDiscontinuity(
        val oldPositionMs: Long,
        val newPositionMs: Long,
        val reason: Int
    ) : PlayerEvent

    data class Error(
        val message: String
    ) : PlayerEvent
}

fun Int.toPlaybackStateLabel(): String {
    return when (this) {
        Player.STATE_IDLE -> "IDLE"
        Player.STATE_BUFFERING -> "BUFFERING"
        Player.STATE_READY -> "READY"
        Player.STATE_ENDED -> "ENDED"
        else -> "UNKNOWN($this)"
    }
}

fun PlayerEvent.toDebugLabel(): String {
    return when (this) {
        PlayerEvent.Ready -> "Ready"
        is PlayerEvent.PlayWhenReadyChanged ->
            "PlayWhenReadyChanged(playWhenReady=$playWhenReady, reason=$reason)"

        is PlayerEvent.IsPlayingChanged ->
            "IsPlayingChanged(isPlaying=$isPlaying)"

        is PlayerEvent.PlaybackStateChanged ->
            "PlaybackStateChanged(state=${playbackState.toPlaybackStateLabel()})"

        is PlayerEvent.PositionDiscontinuity ->
            "PositionDiscontinuity(old=${oldPositionMs}ms, new=${newPositionMs}ms, reason=$reason)"

        is PlayerEvent.Error ->
            "Error(message=$message)"
    }
}
