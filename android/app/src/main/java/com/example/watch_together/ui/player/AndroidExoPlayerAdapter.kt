package com.example.watch_together.ui.player

import android.content.Context
import androidx.media3.common.PlaybackException
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackParameters
import androidx.media3.common.Player
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.PlayerView

class AndroidExoPlayerAdapter(
    context: Context
) : PlayerAdapter {

    private val exoPlayer: ExoPlayer = ExoPlayer.Builder(context).build()
    private var attachedPlayerView: PlayerView? = null
    private var eventListener: ((PlayerEvent) -> Unit)? = null
    private val playerListener = object : Player.Listener {
        override fun onPlaybackStateChanged(playbackState: Int) {
            emit(PlayerEvent.PlaybackStateChanged(playbackState))
            if (playbackState == Player.STATE_READY) {
                emit(PlayerEvent.Ready)
            }
        }

        override fun onPlayWhenReadyChanged(playWhenReady: Boolean, reason: Int) {
            emit(PlayerEvent.PlayWhenReadyChanged(playWhenReady, reason))
        }

        override fun onIsPlayingChanged(isPlaying: Boolean) {
            emit(PlayerEvent.IsPlayingChanged(isPlaying))
        }

        override fun onPositionDiscontinuity(
            oldPosition: Player.PositionInfo,
            newPosition: Player.PositionInfo,
            reason: Int
        ) {
            emit(
                PlayerEvent.PositionDiscontinuity(
                    oldPositionMs = oldPosition.positionMs,
                    newPositionMs = newPosition.positionMs,
                    reason = reason
                )
            )
        }

        override fun onPlayerError(error: PlaybackException) {
            emit(PlayerEvent.Error(error.message ?: "Unknown playback error"))
        }
    }

    init {
        exoPlayer.addListener(playerListener)
    }

    override fun attach(playerView: PlayerView) {
        attachedPlayerView = playerView
        playerView.player = exoPlayer
    }

    override fun detach() {
        attachedPlayerView?.player = null
        attachedPlayerView = null
    }

    override fun setEventListener(listener: ((PlayerEvent) -> Unit)?) {
        eventListener = listener
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
        exoPlayer.removeListener(playerListener)
        eventListener = null
        exoPlayer.release()
    }

    private fun emit(event: PlayerEvent) {
        eventListener?.invoke(event)
    }
}
