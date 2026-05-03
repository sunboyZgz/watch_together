package com.example.watch_together.ui.player

import androidx.media3.common.Player

sealed interface PlayerEvent {
    data object Ready : PlayerEvent

    data class VideoVariantChanged(
        val variant: PlayerVideoVariant
    ) : PlayerEvent

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

data class PlayerVideoVariant(
    val width: Int = 0,
    val height: Int = 0,
    val bitrate: Int = 0,
    val codecs: String? = null,
    val adaptive: Boolean = true,
) {
    val qualityLabel: String
        get() = when {
            height >= 2160 -> "4K"
            height >= 1080 -> "1080p"
            height >= 720 -> "720p"
            height > 0 -> "${height}p"
            else -> "自动"
        }

    val displayLabel: String
        get() = if (adaptive) {
            "自动 · $qualityLabel"
        } else {
            qualityLabel
        }

    val debugLabel: String
        get() {
            val size = if (width > 0 && height > 0) "${width}x$height" else "unknown-size"
            val bitrateLabel = if (bitrate > 0) "${bitrate / 1_000}kbps" else "unknown-bitrate"
            val codecLabel = codecs ?: "unknown-codec"
            return "$displayLabel size=$size bitrate=$bitrateLabel codecs=$codecLabel"
        }
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
        is PlayerEvent.VideoVariantChanged ->
            "VideoVariantChanged(${variant.debugLabel})"

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
