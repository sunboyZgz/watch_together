package com.example.watch_together.ui.player

import android.content.Context
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackParameters
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.PlayerView

class AndroidExoPlayerAdapter(
    context: Context
) : PlayerAdapter {

    private val exoPlayer: ExoPlayer = ExoPlayer.Builder(context).build()
    private var attachedPlayerView: PlayerView? = null

    override fun attach(playerView: PlayerView) {
        attachedPlayerView = playerView
        playerView.player = exoPlayer
    }

    override fun detach() {
        attachedPlayerView?.player = null
        attachedPlayerView = null
    }

    override fun load(url: String) {
        val mediaItem = MediaItem.fromUri(url)
        exoPlayer.setMediaItem(mediaItem)
        exoPlayer.prepare()
    }

    override fun play() {
        exoPlayer.play()
    }

    override fun pause() {
        exoPlayer.pause()
    }

    override fun seekTo(positionMs: Long) {
        exoPlayer.seekTo(positionMs)
    }

    override fun getCurrentPosition(): Long = exoPlayer.currentPosition

    override fun getDuration(): Long = exoPlayer.duration

    override fun isPlaying(): Boolean = exoPlayer.isPlaying

    override fun setPlaybackSpeed(speed: Float) {
        exoPlayer.playbackParameters = PlaybackParameters(speed)
    }

    override fun release() {
        detach()
        exoPlayer.release()
    }
}
