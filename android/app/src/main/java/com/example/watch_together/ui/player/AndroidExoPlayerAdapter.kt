package com.example.watch_together.ui.player

import android.content.Context
import androidx.annotation.OptIn
import androidx.media3.common.C
import androidx.media3.common.Format
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.PlaybackParameters
import androidx.media3.common.Player
import androidx.media3.common.TrackGroup
import androidx.media3.common.TrackSelectionOverride
import androidx.media3.common.Tracks
import androidx.media3.common.VideoSize
import androidx.media3.common.util.UnstableApi
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
class AndroidExoPlayerAdapter(context: Context) : PlayerAdapter {
    private val appContext = context.applicationContext
    private val cache = PlayerCacheProvider.get(appContext)
    private val dataSourceFactory = CacheDataSource.Factory()
        .setCache(cache)
        .setUpstreamDataSourceFactory(createHttpDataSourceFactory())
        .setFlags(CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR)
    private val hlsMediaSourceFactory = HlsMediaSource.Factory(dataSourceFactory)
    private val trackSelector = DefaultTrackSelector(context).apply {
        setParameters(
            buildUponParameters()
                .setMinVideoSize(MinVideoWidth, MinVideoHeight)
                .setAllowVideoNonSeamlessAdaptiveness(true)
                .setExceedVideoConstraintsIfNecessary(false)
        )
    }
    private val loadControl = DefaultLoadControl.Builder()
        .setBufferDurationsMs(MinBufferMs, MaxBufferMs, BufferForPlaybackMs, BufferForPlaybackAfterRebufferMs)
        .build()
    private val exoPlayer = ExoPlayer.Builder(context)
        .setTrackSelector(trackSelector)
        .setLoadControl(loadControl)
        .build()
    private var attachedPlayerView: PlayerView? = null
    private var eventListener: ((PlayerEvent) -> Unit)? = null
    private var latestVideoVariant = PlayerVideoVariant()
    private var latestQualityPreference = PlayerVideoQualityPreference.Auto

    private val playerListener = object : Player.Listener {
        override fun onPlaybackStateChanged(playbackState: Int) {
            emit(PlayerEvent.PlaybackStateChanged(playbackState))
            if (playbackState == Player.STATE_READY) emit(PlayerEvent.Ready)
        }

        override fun onIsPlayingChanged(isPlaying: Boolean) {
            emit(PlayerEvent.IsPlayingChanged(isPlaying))
        }

        override fun onPlayerError(error: PlaybackException) {
            emit(PlayerEvent.Error(error.message ?: "Unknown playback error"))
        }

        override fun onTracksChanged(tracks: Tracks) {
            val variants = videoVariantsFromTracks(tracks, selectedOnly = false)
            if (variants.isNotEmpty()) {
                PlayerDebugLog.d(ABR_LOG_TAG, "available variants ${variants.joinToString { it.debugLabel }}")
            }
            emit(PlayerEvent.VideoQualitiesChanged(variants.toQualityOptions()))
            videoVariantsFromTracks(tracks, selectedOnly = true)
                .firstOrNull { it.height >= MinVideoHeight }
                ?.let { emitVideoVariantIfChanged(it, "selected-track") }
        }

        override fun onVideoSizeChanged(videoSize: VideoSize) {
            if (videoSize.width <= 0 || videoSize.height <= 0) return
            emitVideoVariantIfChanged(
                latestVideoVariant.copy(width = videoSize.width, height = videoSize.height),
                "video-size"
            )
        }
    }

    private val analyticsListener = object : AnalyticsListener {
        override fun onDownstreamFormatChanged(eventTime: AnalyticsListener.EventTime, mediaLoadData: MediaLoadData) {
            if (mediaLoadData.trackType != C.TRACK_TYPE_VIDEO) return
            val format = mediaLoadData.trackFormat ?: return
            emitVideoVariantIfChanged(format.toVideoVariant(), "abr")
        }
    }

    init {
        exoPlayer.addListener(playerListener)
        exoPlayer.addAnalyticsListener(analyticsListener)
    }

    override fun attach(playerView: PlayerView) {
        if (attachedPlayerView !== playerView) attachedPlayerView?.player = null
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
        latestQualityPreference = PlayerVideoQualityPreference.Auto
        applyTrackSelection()
        emit(PlayerEvent.VideoVariantChanged(latestVideoVariant, "load"))
        emit(PlayerEvent.VideoQualitiesChanged(listOf(PlayerVideoQualityOption.Auto)))
        val mediaItem = MediaItem.fromUri(url)
        if (url.substringBefore('?').substringBefore('#').endsWith(".m3u8", ignoreCase = true)) {
            PlayerDebugLog.d(ABR_LOG_TAG, "load hls from backend mediaUrl=$url cacheSizeBytes=${cache.cacheSpace}")
            exoPlayer.setMediaSource(hlsMediaSourceFactory.createMediaSource(mediaItem))
        } else {
            PlayerDebugLog.d(ABR_LOG_TAG, "load mediaUrl=$url")
            exoPlayer.setMediaItem(mediaItem)
        }
        exoPlayer.prepare()
    }

    override fun play() = exoPlayer.play()
    override fun pause() = exoPlayer.pause()
    override fun seekTo(positionMs: Long) = exoPlayer.seekTo(positionMs)
    override fun setPlaybackSpeed(speed: Float) {
        exoPlayer.playbackParameters = PlaybackParameters(speed)
    }

    override fun setVideoQualityPreference(preference: PlayerVideoQualityPreference) {
        latestQualityPreference = preference
        applyTrackSelection()
        PlayerDebugLog.d(ABR_LOG_TAG, "quality preference=${preference.label}")
    }

    override fun getCurrentPosition(): Long = exoPlayer.currentPosition
    override fun getDuration(): Long = exoPlayer.duration
    override fun getBufferedPosition(): Long = exoPlayer.bufferedPosition
    override fun getBufferedPercentage(): Int = exoPlayer.bufferedPercentage
    override fun isPlaying(): Boolean = exoPlayer.isPlaying

    override fun release() {
        detach()
        exoPlayer.removeAnalyticsListener(analyticsListener)
        exoPlayer.removeListener(playerListener)
        eventListener = null
        exoPlayer.release()
    }

    private fun applyTrackSelection() {
        val manualOverride = latestQualityPreference.height?.let(::findVideoTrackOverrideForHeight)
        trackSelector.setParameters(
            trackSelector.buildUponParameters().apply {
                clearOverridesOfType(C.TRACK_TYPE_VIDEO)
                clearVideoSizeConstraints()
                setMinVideoSize(MinVideoWidth, MinVideoHeight)
                setForceLowestBitrate(false)
                setAllowVideoNonSeamlessAdaptiveness(true)
                setExceedVideoConstraintsIfNecessary(false)
                setMaxVideoSize(Int.MAX_VALUE, Int.MAX_VALUE)
                setViewportSize(Int.MAX_VALUE, Int.MAX_VALUE, true)
                if (manualOverride != null) {
                    setOverrideForType(manualOverride)
                    setForceHighestSupportedBitrate(false)
                } else if (latestQualityPreference.isAuto) {
                    setForceHighestSupportedBitrate(true)
                } else {
                    setForceHighestSupportedBitrate(false)
                }
            }
        )
        if (latestQualityPreference.height != null && manualOverride == null) {
            PlayerDebugLog.d(ABR_LOG_TAG, "quality preference=${latestQualityPreference.label} no matching supported track yet")
        }
    }

    private fun findVideoTrackOverrideForHeight(height: Int): TrackSelectionOverride? {
        val trackGroups = exoPlayer.currentTracks.groups
            .filter { it.type == C.TRACK_TYPE_VIDEO }
        var bestGroup: TrackGroup? = null
        var bestTrackIndex: Int? = null
        var bestBitrate = -1
        for (group in trackGroups) {
            for (trackIndex in 0 until group.length) {
                if (!group.isTrackSupported(trackIndex, false)) continue
                val format = group.getTrackFormat(trackIndex)
                if (format.height != height) continue
                val bitrate = format.bitrate.coerceAtLeast(0)
                if (bitrate > bestBitrate) {
                    bestGroup = group.mediaTrackGroup
                    bestTrackIndex = trackIndex
                    bestBitrate = bitrate
                }
            }
        }
        val mediaTrackGroup = bestGroup ?: return null
        return TrackSelectionOverride(mediaTrackGroup, bestTrackIndex ?: return null)
    }

    private fun emit(event: PlayerEvent) {
        eventListener?.invoke(event)
    }

    private fun emitVideoVariantIfChanged(variant: PlayerVideoVariant, reason: String) {
        if (variant == latestVideoVariant) return
        latestVideoVariant = variant
        PlayerDebugLog.d(ABR_LOG_TAG, "variant changed reason=$reason ${variant.debugLabel}")
        emit(PlayerEvent.VideoVariantChanged(variant, reason))
    }

    private fun videoVariantsFromTracks(tracks: Tracks, selectedOnly: Boolean): List<PlayerVideoVariant> {
        return tracks.groups
            .filter { it.type == C.TRACK_TYPE_VIDEO }
            .flatMap { group ->
                (0 until group.length)
                    .filter { group.isTrackSupported(it, false) }
                    .filter { !selectedOnly || group.isTrackSelected(it) }
                    .map { group.getTrackFormat(it).toVideoVariant() }
            }
            .distinctBy { it.height to it.bitrate }
            .sortedWith(compareBy<PlayerVideoVariant> { it.height }.thenBy { it.bitrate })
    }

    private fun List<PlayerVideoVariant>.toQualityOptions(): List<PlayerVideoQualityOption> {
        return listOf(PlayerVideoQualityOption.Auto) + mapNotNull { it.height.takeIf { height -> height >= MinVideoHeight } }
            .distinct()
            .sortedDescending()
            .map { PlayerVideoQualityOption(height = it, label = "${it}p") }
    }

    private fun Format.toVideoVariant(): PlayerVideoVariant {
        return PlayerVideoVariant(
            width = width.coerceAtLeast(0),
            height = height.coerceAtLeast(0),
            bitrate = bitrate.coerceAtLeast(0),
            codecs = codecs
        )
    }

    private companion object {
        fun createHttpDataSourceFactory(): DefaultHttpDataSource.Factory {
            ensureMediaHttpCookieManagerInstalled()
            return DefaultHttpDataSource.Factory()
        }

        const val ABR_LOG_TAG = "WatchTogetherABR2"
        const val MinVideoWidth = 1_280
        const val MinVideoHeight = 720
        const val MinBufferMs = 20_000
        const val MaxBufferMs = 60_000
        const val BufferForPlaybackMs = 2_500
        const val BufferForPlaybackAfterRebufferMs = 6_000
    }
}
