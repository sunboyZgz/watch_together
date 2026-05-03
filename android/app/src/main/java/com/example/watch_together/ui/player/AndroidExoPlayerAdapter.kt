package com.example.watch_together.ui.player

import android.content.Context
import android.util.Log
import androidx.annotation.OptIn
import androidx.media3.common.C
import androidx.media3.common.Format
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.PlaybackParameters
import androidx.media3.common.Player
import androidx.media3.common.Tracks
import androidx.media3.common.VideoSize
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.DefaultLoadControl
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.analytics.AnalyticsListener
import androidx.media3.exoplayer.source.MediaLoadData
import androidx.media3.exoplayer.trackselection.DefaultTrackSelector
import androidx.media3.ui.PlayerView

@OptIn(UnstableApi::class)
class AndroidExoPlayerAdapter(
    context: Context
) : PlayerAdapter {

    private val trackSelector = DefaultTrackSelector(context).apply {
        setParameters(
            buildUponParameters()
                .setMinVideoSize(MinVideoWidth, MinVideoHeight)
                .setExceedVideoConstraintsIfNecessary(false)
                .setAllowVideoNonSeamlessAdaptiveness(true)
        )
    }
    private val loadControl = DefaultLoadControl.Builder()
        .setBufferDurationsMs(
            MinBufferMs,
            MaxBufferMs,
            BufferForPlaybackMs,
            BufferForPlaybackAfterRebufferMs
        )
        .build()
    private val exoPlayer: ExoPlayer = ExoPlayer.Builder(context)
        .setTrackSelector(trackSelector)
        .setLoadControl(loadControl)
        .build()
    private var attachedPlayerView: PlayerView? = null
    private var eventListener: ((PlayerEvent) -> Unit)? = null
    private var latestVideoVariant = PlayerVideoVariant()
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

        override fun onTracksChanged(tracks: Tracks) {
            logAvailableVideoTracks(tracks)
        }

        override fun onVideoSizeChanged(videoSize: VideoSize) {
            if (videoSize.height <= 0 || videoSize.width <= 0) return
            val updated = latestVideoVariant.copy(
                width = videoSize.width,
                height = videoSize.height
            )
            emitVideoVariantIfChanged(updated, reason = "video-size")
        }
    }
    private val analyticsListener = object : AnalyticsListener {
        override fun onDownstreamFormatChanged(
            eventTime: AnalyticsListener.EventTime,
            mediaLoadData: MediaLoadData
        ) {
            if (mediaLoadData.trackType != C.TRACK_TYPE_VIDEO) return
            val format = mediaLoadData.trackFormat ?: return
            emitVideoVariantIfChanged(format.toVideoVariant(), reason = "abr")
        }
    }

    init {
        exoPlayer.addListener(playerListener)
        exoPlayer.addAnalyticsListener(analyticsListener)
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
        latestVideoVariant = PlayerVideoVariant()
        emit(PlayerEvent.VideoVariantChanged(latestVideoVariant))
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

    override fun getBufferedPosition(): Long = exoPlayer.bufferedPosition

    override fun getBufferedPercentage(): Int = exoPlayer.bufferedPercentage

    override fun isPlaying(): Boolean = exoPlayer.isPlaying

    override fun setPlaybackSpeed(speed: Float) {
        exoPlayer.playbackParameters = PlaybackParameters(speed)
    }

    override fun release() {
        detach()
        exoPlayer.removeAnalyticsListener(analyticsListener)
        exoPlayer.removeListener(playerListener)
        eventListener = null
        exoPlayer.release()
    }

    private fun emit(event: PlayerEvent) {
        eventListener?.invoke(event)
    }

    private fun emitVideoVariantIfChanged(variant: PlayerVideoVariant, reason: String) {
        if (variant == latestVideoVariant) return
        latestVideoVariant = variant
        Log.d(ABR_LOG_TAG, "variant changed reason=$reason ${variant.debugLabel}")
        emit(PlayerEvent.VideoVariantChanged(variant))
    }

    private fun logAvailableVideoTracks(tracks: Tracks) {
        val variants = tracks.groups
            .filter { group -> group.type == C.TRACK_TYPE_VIDEO }
            .flatMap { group ->
                (0 until group.length)
                    .filter { index -> group.isTrackSupported(index, false) }
                    .map { index -> group.getTrackFormat(index).toVideoVariant() }
            }
            .distinctBy { variant -> variant.height to variant.bitrate }
            .sortedWith(compareBy<PlayerVideoVariant> { it.height }.thenBy { it.bitrate })

        if (variants.isEmpty()) return
        Log.d(
            ABR_LOG_TAG,
            "available variants ${variants.joinToString { it.debugLabel }}"
        )
    }

    private fun Format.toVideoVariant(): PlayerVideoVariant {
        return PlayerVideoVariant(
            width = width.coerceAtLeast(0),
            height = height.coerceAtLeast(0),
            bitrate = bitrate.coerceAtLeast(0),
            codecs = codecs,
            adaptive = true
        )
    }

    private companion object {
        // Keep a larger VOD buffer so 2x playback does not immediately drain short HLS segments.
        const val ABR_LOG_TAG = "WatchTogetherABR"
        const val MinVideoWidth = 1_280
        const val MinVideoHeight = 720
        const val MinBufferMs = 30_000
        const val MaxBufferMs = 90_000
        const val BufferForPlaybackMs = 2_500
        const val BufferForPlaybackAfterRebufferMs = 5_000
    }
}
