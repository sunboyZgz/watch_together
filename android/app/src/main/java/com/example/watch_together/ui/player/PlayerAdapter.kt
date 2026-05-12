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
    fun setPlaybackSpeed(speed: Float)
    fun setVideoQualityPreference(preference: PlayerVideoQualityPreference)
    fun getCurrentPosition(): Long
    fun getDuration(): Long
    fun getBufferedPosition(): Long
    fun getBufferedPercentage(): Int
    fun isPlaying(): Boolean
    fun release()
}
