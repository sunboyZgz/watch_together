package com.example.watch_together.ui.player

import androidx.media3.ui.PlayerView

interface PlayerAdapter {
    fun attach(playerView: PlayerView)
    fun detach()

    fun setEventListener(listener: ((PlayerEvent) -> Unit)?)

    fun load(url: String)
    fun play()
    fun pause()
    fun seekTo(positionMs: Long)

    fun getCurrentPosition(): Long
    fun getDuration(): Long
    fun getBufferedPosition(): Long = 0L
    fun getBufferedPercentage(): Int = 0
    fun isPlaying(): Boolean

    fun setPlaybackSpeed(speed: Float)
    fun setVideoQualityPreference(preference: PlayerVideoQualityPreference) = Unit
    fun updatePlaybackStrategy(playbackSpeed: Float, rebufferCount: Int) = Unit
    fun updateAheadPrefetch(
        mediaUrl: String,
        currentPositionMs: Long,
        playbackSpeed: Float,
        effectiveBufferedAheadMs: Long,
        estimatedSegmentsAhead: Int,
        rebufferCount: Int,
        videoVariant: PlayerVideoVariant
    ) = Unit

    fun release()
}
