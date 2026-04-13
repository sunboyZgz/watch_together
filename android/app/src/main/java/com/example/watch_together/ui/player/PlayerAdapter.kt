package com.example.watch_together.ui.player

import androidx.media3.ui.PlayerView

interface PlayerAdapter {
    fun attach(playerView: PlayerView)
    fun detach()

    fun load(url: String)
    fun play()
    fun pause()
    fun seekTo(positionMs: Long)

    fun getCurrentPosition(): Long
    fun getDuration(): Long
    fun isPlaying(): Boolean

    fun setPlaybackSpeed(speed: Float)

    fun release()
}
