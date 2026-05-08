package com.example.watch_together.ui.player

import androidx.media3.common.Player

sealed interface PlayerEvent {
    data object Ready : PlayerEvent

    data class VideoVariantChanged(
        val variant: PlayerVideoVariant,
        val reason: String = ""
    ) : PlayerEvent

    data class VideoQualitiesChanged(
        val options: List<PlayerVideoQualityOption>
    ) : PlayerEvent

    data class VideoQualitySwitchChanged(
        val state: PlayerVideoQualitySwitchState
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

    data object RenderedFirstFrame : PlayerEvent

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
        get() = if (qualityLabel == "自动") {
            "自动 · 识别中"
        } else if (adaptive) {
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

data class PlayerVideoQualityOption(
    val height: Int?,
    val label: String,
    val available: Boolean = true
) {
    val isAuto: Boolean
        get() = height == null

    companion object {
        val Auto = PlayerVideoQualityOption(height = null, label = "自动")
    }
}

data class PlayerVideoQualityPreference(
    val height: Int? = null
) {
    val isAuto: Boolean
        get() = height == null

    val label: String
        get() = height?.let { "${it}p" } ?: "自动"

    companion object {
        val Auto = PlayerVideoQualityPreference()
    }
}

enum class PlayerVideoQualitySwitchPhase {
    Idle,
    PendingRequest,
    WarmingTargetVariant,
    ReadyToCommit,
    Committed,
    Blocked,
    Fallback
}

data class PlayerVideoQualitySwitchState(
    val phase: PlayerVideoQualitySwitchPhase = PlayerVideoQualitySwitchPhase.Idle,
    val preference: PlayerVideoQualityPreference = PlayerVideoQualityPreference.Auto,
    val detail: String = ""
) {
    val inlineHintLabel: String
        get() = when (phase) {
            PlayerVideoQualitySwitchPhase.PendingRequest,
            PlayerVideoQualitySwitchPhase.WarmingTargetVariant -> "切换中 · ${preference.label}"
            PlayerVideoQualitySwitchPhase.ReadyToCommit -> "即将切到 ${preference.label}"
            else -> ""
        }

    val noticeLabel: String
        get() = when (phase) {
            PlayerVideoQualitySwitchPhase.Idle -> ""
            PlayerVideoQualitySwitchPhase.PendingRequest,
            PlayerVideoQualitySwitchPhase.WarmingTargetVariant ->
                "正在切换到 ${preference.label}，旧清晰度会继续播放。"
            PlayerVideoQualitySwitchPhase.ReadyToCommit ->
                "新清晰度已预热完成，正在平滑切换到 ${preference.label}。"
            PlayerVideoQualitySwitchPhase.Committed ->
                if (preference.isAuto) {
                    "已切回自动清晰度，会根据播放流畅度调整。"
                } else {
                    "已切到 ${preference.label}，卡顿时会优先保障流畅。"
                }
            PlayerVideoQualitySwitchPhase.Blocked ->
                detail.ifBlank { "当前网络或缓冲条件不足，暂不切换。" }
            PlayerVideoQualitySwitchPhase.Fallback ->
                detail.ifBlank { "目标清晰度预热失败，继续保持当前播放。" }
        }

    val isPending: Boolean
        get() = phase == PlayerVideoQualitySwitchPhase.PendingRequest ||
            phase == PlayerVideoQualitySwitchPhase.WarmingTargetVariant ||
            phase == PlayerVideoQualitySwitchPhase.ReadyToCommit
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
            "VideoVariantChanged(reason=$reason, ${variant.debugLabel})"

        is PlayerEvent.VideoQualitiesChanged ->
            "VideoQualitiesChanged(${options.joinToString { it.label }})"

        is PlayerEvent.VideoQualitySwitchChanged ->
            "VideoQualitySwitchChanged(phase=${state.phase}, preference=${state.preference.label})"

        is PlayerEvent.PlayWhenReadyChanged ->
            "PlayWhenReadyChanged(playWhenReady=$playWhenReady, reason=$reason)"

        is PlayerEvent.IsPlayingChanged ->
            "IsPlayingChanged(isPlaying=$isPlaying)"

        is PlayerEvent.PlaybackStateChanged ->
            "PlaybackStateChanged(state=${playbackState.toPlaybackStateLabel()})"

        is PlayerEvent.PositionDiscontinuity ->
            "PositionDiscontinuity(old=${oldPositionMs}ms, new=${newPositionMs}ms, reason=$reason)"

        PlayerEvent.RenderedFirstFrame ->
            "RenderedFirstFrame"

        is PlayerEvent.Error ->
            "Error(message=$message)"
    }
}
