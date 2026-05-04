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
import androidx.media3.datasource.DataSource
import androidx.media3.datasource.DefaultHttpDataSource
import androidx.media3.datasource.cache.CacheDataSource
import androidx.media3.exoplayer.DefaultLoadControl
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.analytics.AnalyticsListener
import androidx.media3.exoplayer.hls.HlsMediaSource
import androidx.media3.exoplayer.source.MediaLoadData
import androidx.media3.exoplayer.trackselection.DefaultTrackSelector
import androidx.media3.ui.PlayerView

@OptIn(UnstableApi::class)
class AndroidExoPlayerAdapter(
    context: Context
) : PlayerAdapter {

    private val appContext = context.applicationContext
    private val cache = PlayerCacheProvider.get(appContext)
    private val upstreamDataSourceFactory = DefaultHttpDataSource.Factory()
    private val cacheEventListener = object : CacheDataSource.EventListener {
        override fun onCachedBytesRead(cacheSizeBytes: Long, cachedBytesRead: Long) {
            Log.d(
                CACHE_LOG_TAG,
                "cache hit cachedBytesRead=$cachedBytesRead cacheSizeBytes=$cacheSizeBytes"
            )
        }

        override fun onCacheIgnored(reason: Int) {
            Log.d(CACHE_LOG_TAG, "cache ignored reason=${reason.toCacheIgnoreReasonLabel()}")
        }
    }
    private val cacheDataSourceFactory: DataSource.Factory = CacheDataSource.Factory()
        .setCache(cache)
        .setUpstreamDataSourceFactory(upstreamDataSourceFactory)
        .setFlags(CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR)
        .setEventListener(cacheEventListener)
    private val hlsMediaSourceFactory = HlsMediaSource.Factory(cacheDataSourceFactory)
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
    private var latestPlaybackStrategy = PlayerPlaybackStrategy.Auto
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
            val variants = videoVariantsFromTracks(tracks, selectedOnly = false)
            logAvailableVideoTracks(variants)
            videoVariantsFromTracks(tracks, selectedOnly = true)
                .filter { variant -> variant.height >= MinVideoHeight }
                .minByOrNull { variant -> variant.height }
                ?.let { variant ->
                    emitVideoVariantIfChanged(variant, reason = "selected-track")
                }
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
        if (url.isHlsPlaylistUrl()) {
            Log.d(CACHE_LOG_TAG, "load HLS with local cache url=$url cacheSizeBytes=${cache.cacheSpace}")
            exoPlayer.setMediaSource(hlsMediaSourceFactory.createMediaSource(mediaItem))
        } else {
            Log.d(CACHE_LOG_TAG, "load without HLS cache url=$url")
            exoPlayer.setMediaItem(mediaItem)
        }
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

    override fun updatePlaybackStrategy(playbackSpeed: Float, rebufferCount: Int) {
        val nextStrategy = PlayerPlaybackStrategy.forPlayback(
            playbackSpeed = playbackSpeed,
            rebufferCount = rebufferCount
        )
        if (nextStrategy == latestPlaybackStrategy) return
        latestPlaybackStrategy = nextStrategy
        trackSelector.setParameters(
            trackSelector.buildUponParameters().apply {
                setMinVideoSize(MinVideoWidth, MinVideoHeight)
                setExceedVideoConstraintsIfNecessary(false)
                setAllowVideoNonSeamlessAdaptiveness(true)
                if (nextStrategy.lockToMobileFast720p) {
                    setMaxVideoSize(MobileFastVideoWidth, MobileFastVideoHeight)
                    setForceHighestSupportedBitrate(false)
                    setForceLowestBitrate(false)
                } else {
                    clearVideoSizeConstraints()
                    setMinVideoSize(MinVideoWidth, MinVideoHeight)
                }
            }
        )
        Log.d(
            ABR_LOG_TAG,
            "playback strategy=${nextStrategy.logLabel} speed=${playbackSpeed}x rebufferCount=$rebufferCount"
        )
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

    private fun videoVariantsFromTracks(
        tracks: Tracks,
        selectedOnly: Boolean
    ): List<PlayerVideoVariant> {
        return tracks.groups
            .filter { group -> group.type == C.TRACK_TYPE_VIDEO }
            .flatMap { group ->
                (0 until group.length)
                    .filter { index -> group.isTrackSupported(index, false) }
                    .filter { index -> !selectedOnly || group.isTrackSelected(index) }
                    .map { index -> group.getTrackFormat(index).toVideoVariant() }
            }
            .distinctBy { variant -> variant.height to variant.bitrate }
            .sortedWith(compareBy<PlayerVideoVariant> { it.height }.thenBy { it.bitrate })
    }

    private fun logAvailableVideoTracks(variants: List<PlayerVideoVariant>) {
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
        const val CACHE_LOG_TAG = "WatchTogetherCache"
        const val MinVideoWidth = 1_280
        const val MinVideoHeight = 720
        const val MobileFastVideoWidth = 1_280
        const val MobileFastVideoHeight = 720
        const val MinBufferMs = 30_000
        const val MaxBufferMs = 90_000
        const val BufferForPlaybackMs = 3_500
        const val BufferForPlaybackAfterRebufferMs = 10_000
    }
}

private fun String.isHlsPlaylistUrl(): Boolean {
    return substringBefore('?').substringBefore('#').endsWith(".m3u8", ignoreCase = true)
}

@OptIn(UnstableApi::class)
private fun Int.toCacheIgnoreReasonLabel(): String {
    return when (this) {
        CacheDataSource.CACHE_IGNORED_REASON_ERROR -> "error"
        CacheDataSource.CACHE_IGNORED_REASON_UNSET_LENGTH -> "unset_length"
        else -> "unknown_$this"
    }
}

private enum class PlayerPlaybackStrategy(
    val lockToMobileFast720p: Boolean,
    val logLabel: String
) {
    Auto(lockToMobileFast720p = false, logLabel = "auto"),
    MobileFast720p(lockToMobileFast720p = true, logLabel = "mobile_fast_720p");

    companion object {
        private const val HighSpeedThreshold = 2.0f
        private const val RebufferLockThreshold = 2

        fun forPlayback(playbackSpeed: Float, rebufferCount: Int): PlayerPlaybackStrategy {
            return if (playbackSpeed >= HighSpeedThreshold || rebufferCount >= RebufferLockThreshold) {
                MobileFast720p
            } else {
                Auto
            }
        }
    }
}
