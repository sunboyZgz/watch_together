package com.example.watch_together.ui.player

import androidx.media3.common.Player

data class PlayerVideoVariant(
    val width: Int = 0,
    val height: Int = 0,
    val bitrate: Int = 0,
    val codecs: String? = null,
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
        get() = if (height > 0) "自动 · $qualityLabel" else "自动 · 识别中"

    val debugLabel: String
        get() {
            val size = if (width > 0 && height > 0) "${width}x$height" else "unknown-size"
            val bitrateLabel = if (bitrate > 0) "${bitrate / 1_000}kbps" else "unknown-bitrate"
            return "$displayLabel size=$size bitrate=$bitrateLabel codecs=${codecs ?: "unknown-codec"}"
        }
}

data class PlayerVideoQualityOption(
    val height: Int?,
    val label: String,
) {
    val isAuto: Boolean get() = height == null

    companion object {
        val Auto = PlayerVideoQualityOption(height = null, label = "自动")
    }
}

data class PlayerVideoQualityPreference(val height: Int? = null) {
    val isAuto: Boolean get() = height == null
    val label: String get() = height?.let { "${it}p" } ?: "自动"

    companion object {
        val Auto = PlayerVideoQualityPreference()
    }
}

data class PlayerRuntimeState(
    val currentPosition: Long = 0L,
    val duration: Long = 0L,
    val bufferedPosition: Long = 0L,
    val bufferedPercentage: Int = 0,
    val isPlaying: Boolean = false,
    val playbackState: Int = Player.STATE_IDLE,
    val playbackSpeed: Float = 1f,
    val videoVariant: PlayerVideoVariant = PlayerVideoVariant(),
    val availableVideoQualities: List<PlayerVideoQualityOption> = listOf(PlayerVideoQualityOption.Auto),
    val videoQualityPreference: PlayerVideoQualityPreference = PlayerVideoQualityPreference.Auto,
    val statusMessage: String = "",
) {
    val bufferedAheadMs: Long get() = (bufferedPosition - currentPosition).coerceAtLeast(0L)
    val effectiveBufferedAheadMs: Long get() = (bufferedAheadMs / playbackSpeed.coerceAtLeast(0.25f)).toLong()
    val canControlPlayback: Boolean get() = playbackState == Player.STATE_READY
}

sealed interface PlayerEvent {
    data object Ready : PlayerEvent
    data class PlaybackStateChanged(val playbackState: Int) : PlayerEvent
    data class IsPlayingChanged(val isPlaying: Boolean) : PlayerEvent
    data class VideoVariantChanged(val variant: PlayerVideoVariant, val reason: String) : PlayerEvent
    data class VideoQualitiesChanged(val options: List<PlayerVideoQualityOption>) : PlayerEvent
    data class Error(val message: String) : PlayerEvent
}

fun Int.toPlaybackStateLabel(): String = when (this) {
    Player.STATE_IDLE -> "IDLE"
    Player.STATE_BUFFERING -> "BUFFERING"
    Player.STATE_READY -> "READY"
    Player.STATE_ENDED -> "ENDED"
    else -> "UNKNOWN($this)"
}

fun PlayerEvent.toDebugLabel(): String = when (this) {
    PlayerEvent.Ready -> "Ready"
    is PlayerEvent.PlaybackStateChanged -> "PlaybackStateChanged(${playbackState.toPlaybackStateLabel()})"
    is PlayerEvent.IsPlayingChanged -> "IsPlayingChanged($isPlaying)"
    is PlayerEvent.VideoVariantChanged -> "VideoVariantChanged(reason=$reason, ${variant.debugLabel})"
    is PlayerEvent.VideoQualitiesChanged -> "VideoQualitiesChanged(${options.joinToString { it.label }})"
    is PlayerEvent.Error -> "Error($message)"
}
