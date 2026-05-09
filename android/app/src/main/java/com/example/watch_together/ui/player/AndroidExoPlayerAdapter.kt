package com.example.watch_together.ui.player

import android.content.Context
import android.os.Handler
import android.os.Looper
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
            PlayerDebugLog.d(
                CACHE_LOG_TAG,
                "cache hit cachedBytesRead=$cachedBytesRead cacheSizeBytes=$cacheSizeBytes"
            )
        }

        override fun onCacheIgnored(reason: Int) {
            PlayerDebugLog.d(CACHE_LOG_TAG, "cache ignored reason=${reason.toCacheIgnoreReasonLabel()}")
        }
    }
    private val cacheDataSourceFactory: CacheDataSource.Factory = CacheDataSource.Factory()
        .setCache(cache)
        .setUpstreamDataSourceFactory(upstreamDataSourceFactory)
        .setFlags(CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR)
        .setEventListener(cacheEventListener)
    private val hlsMediaSourceFactory = HlsMediaSource.Factory(cacheDataSourceFactory)
    private val hlsAheadPrefetcher = HlsAheadPrefetcher(
        dataSourceFactory = cacheDataSourceFactory,
        cache = cache,
    ) { metrics ->
        if (metrics.bytesDownloaded > 0L && metrics.durationMs > 0L) {
            val estimate = bandwidthEstimator.record(
                ThroughputSample(
                    bytesTransferred = metrics.bytesDownloaded,
                    durationMs = metrics.durationMs,
                    source = metrics.source
                )
            )
            PlayerDebugLog.d(
                ABR_LOG_TAG,
                "prefetch metrics source=${metrics.source} target=${metrics.targetHeight}p " +
                    "segments=${metrics.segmentCount} downloaded=${metrics.bytesDownloaded}B " +
                    "duration=${metrics.durationMs}ms ewma=${estimate.throughputEwmaBps}bps"
            )
        }
    }
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
    private val mainHandler = Handler(Looper.getMainLooper())
    private val bandwidthEstimator = BandwidthAwareThroughputEstimator()
    private var latestVideoVariant = PlayerVideoVariant()
    private var latestAvailableVideoVariants: List<PlayerVideoVariant> = emptyList()
    private var latestPlaybackStrategy = PlayerPlaybackStrategy.Auto
    private var latestQualityPreference = PlayerVideoQualityPreference.Auto
    private var latestRebufferCount = 0
    private var currentMediaUrl: String = ""
    private var qualitySwitchGeneration: Long = 0L
    private var pendingQualitySwitch: PendingQualitySwitch? = null
    private var committedQualitySwitch: CommittedQualitySwitch? = null
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
            latestAvailableVideoVariants = variants
            logAvailableVideoTracks(variants)
            emit(PlayerEvent.VideoQualitiesChanged(variants.toQualityOptions()))
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

        override fun onRenderedFirstFrame() {
            committedQualitySwitch = committedQualitySwitch?.copy(renderedFirstFrame = true)
            emit(PlayerEvent.RenderedFirstFrame)
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
        if (attachedPlayerView !== playerView) {
            attachedPlayerView?.player = null
        }
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
        currentMediaUrl = url
        pendingQualitySwitch = null
        committedQualitySwitch = null
        qualitySwitchGeneration += 1L
        latestVideoVariant = PlayerVideoVariant()
        latestAvailableVideoVariants = emptyList()
        emit(PlayerEvent.VideoVariantChanged(latestVideoVariant, reason = "load"))
        emit(PlayerEvent.VideoQualitiesChanged(listOf(PlayerVideoQualityOption.Auto)))
        emit(
            PlayerEvent.VideoQualitySwitchChanged(
                PlayerVideoQualitySwitchState(
                    effectivePreference = latestQualityPreference
                )
            )
        )
        val mediaItem = MediaItem.fromUri(url)
        if (url.isHlsPlaylistUrl()) {
            PlayerDebugLog.d(CACHE_LOG_TAG, "load HLS with local cache url=$url cacheSizeBytes=${cache.cacheSpace}")
            exoPlayer.setMediaSource(hlsMediaSourceFactory.createMediaSource(mediaItem))
        } else {
            PlayerDebugLog.d(CACHE_LOG_TAG, "load without HLS cache url=$url")
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

    override fun reset() {
        currentMediaUrl = ""
        pendingQualitySwitch = null
        committedQualitySwitch = null
        qualitySwitchGeneration += 1L
        latestQualityPreference = PlayerVideoQualityPreference.Auto
        latestVideoVariant = PlayerVideoVariant()
        latestAvailableVideoVariants = emptyList()
        hlsAheadPrefetcher.reset()
        applyTrackSelection()
        emit(PlayerEvent.VideoVariantChanged(latestVideoVariant, reason = "reset"))
        emit(PlayerEvent.VideoQualitiesChanged(listOf(PlayerVideoQualityOption.Auto)))
        emit(
            PlayerEvent.VideoQualitySwitchChanged(
                PlayerVideoQualitySwitchState(
                    effectivePreference = latestQualityPreference
                )
            )
        )
        exoPlayer.stop()
        exoPlayer.clearMediaItems()
    }

    override fun getCurrentPosition(): Long = exoPlayer.currentPosition

    override fun getDuration(): Long = exoPlayer.duration

    override fun getBufferedPosition(): Long = exoPlayer.bufferedPosition

    override fun getBufferedPercentage(): Int = exoPlayer.bufferedPercentage

    override fun isPlaying(): Boolean = exoPlayer.isPlaying

    override fun setPlaybackSpeed(speed: Float) {
        exoPlayer.playbackParameters = PlaybackParameters(speed)
    }

    override fun setVideoQualityPreference(preference: PlayerVideoQualityPreference) {
        if (preference == latestQualityPreference && pendingQualitySwitch == null) return
        val previousPreference = latestQualityPreference
        if (preference.isAuto) {
            qualitySwitchGeneration += 1L
            pendingQualitySwitch = null
            committedQualitySwitch = null
            latestQualityPreference = preference
            applyTrackSelection()
            emit(
                PlayerEvent.VideoQualitySwitchChanged(
                    PlayerVideoQualitySwitchState(
                        phase = PlayerVideoQualitySwitchPhase.Committed,
                        preference = preference,
                        effectivePreference = preference
                    )
                )
            )
            PlayerDebugLog.d(
                ABR_LOG_TAG,
                "quality preference=${preference.label} strategy=${latestPlaybackStrategy.logLabel}"
            )
            return
        }

        if (latestVideoVariant.height == preference.height) {
            qualitySwitchGeneration += 1L
            pendingQualitySwitch = null
            committedQualitySwitch = null
            latestQualityPreference = preference
            applyTrackSelection()
            emit(
                PlayerEvent.VideoQualitySwitchChanged(
                    PlayerVideoQualitySwitchState(
                        phase = PlayerVideoQualitySwitchPhase.Committed,
                        preference = preference,
                        effectivePreference = preference
                    )
                )
            )
            PlayerDebugLog.d(
                ABR_LOG_TAG,
                "quality preference=${preference.label} strategy=${latestPlaybackStrategy.logLabel}"
            )
            return
        }

        val generation = qualitySwitchGeneration + 1L
        qualitySwitchGeneration = generation
        val targetVariant = latestAvailableVideoVariants
            .filter { variant -> variant.height >= MinVideoHeight }
            .minByOrNull { variant ->
                kotlin.math.abs(variant.height - (preference.height ?: latestVideoVariant.height))
            }
            ?: latestVideoVariant.copy(height = preference.height ?: latestVideoVariant.height)
        val plan = BandwidthAwareQualitySwitchAdvisor.plan(
            QualitySwitchPlanningContext(
                playbackSpeed = exoPlayer.playbackParameters.speed,
                bufferedAheadMs = exoPlayer.bufferedPosition - exoPlayer.currentPosition,
                effectiveBufferedAheadMs = ((exoPlayer.bufferedPosition - exoPlayer.currentPosition)
                    .coerceAtLeast(0L) / exoPlayer.playbackParameters.speed.coerceAtLeast(0.25f)).toLong(),
                estimatedSegmentsAhead = (((exoPlayer.bufferedPosition - exoPlayer.currentPosition)
                    .coerceAtLeast(0L) / exoPlayer.playbackParameters.speed.coerceAtLeast(0.25f)).toLong() / 6_000L).toInt(),
                rebufferCount = latestRebufferCount,
                currentVariant = latestVideoVariant,
                targetVariant = targetVariant,
                targetBandwidthBps = targetVariant.bitrate,
                bandwidthEstimate = bandwidthEstimator.currentEstimate(),
                cachedTargetSegments = 0
            )
        )
        if (!plan.shouldAttemptWarmup) {
            pendingQualitySwitch = null
            emit(
                PlayerEvent.VideoQualitySwitchChanged(
                    PlayerVideoQualitySwitchState(
                        phase = PlayerVideoQualitySwitchPhase.Blocked,
                        preference = preference,
                        effectivePreference = previousPreference,
                        detail = plan.blockedReason
                    )
                )
            )
            PlayerDebugLog.d(
                ABR_LOG_TAG,
                "quality switch blocked target=${preference.label} reason=${plan.blockedReason}"
            )
            return
        }
        PlayerDebugLog.d(
            ABR_LOG_TAG,
            "quality switch plan target=${preference.label} strategy=${plan.strategy} " +
                "skip=${plan.skipSegments} warm=${plan.warmSegments} " +
                "windowMs=${plan.targetWarmupWindowMs} graceMs=${plan.graceWindowMs} " +
                "estimate=${bandwidthEstimator.currentEstimate().throughputEwmaBps}bps " +
                "bufferAhead=${exoPlayer.bufferedPosition - exoPlayer.currentPosition}ms"
        )
        pendingQualitySwitch = PendingQualitySwitch(
            generation = generation,
            preference = preference,
            fallbackPreference = previousPreference,
            plan = plan
        )
        emit(
            PlayerEvent.VideoQualitySwitchChanged(
                PlayerVideoQualitySwitchState(
                    phase = PlayerVideoQualitySwitchPhase.PendingRequest,
                    preference = preference,
                    effectivePreference = previousPreference
                )
            )
        )

        val mediaUrl = currentMediaUrl
        if (!mediaUrl.isHlsPlaylistUrl()) {
            latestQualityPreference = preference
            pendingQualitySwitch = null
            applyTrackSelection()
            emit(
                PlayerEvent.VideoQualitySwitchChanged(
                    PlayerVideoQualitySwitchState(
                        phase = PlayerVideoQualitySwitchPhase.Committed,
                        preference = preference,
                        effectivePreference = preference
                    )
                )
            )
            PlayerDebugLog.d(
                ABR_LOG_TAG,
                "quality preference=${preference.label} strategy=${latestPlaybackStrategy.logLabel}"
            )
            return
        }

        emit(
            PlayerEvent.VideoQualitySwitchChanged(
                PlayerVideoQualitySwitchState(
                    phase = PlayerVideoQualitySwitchPhase.WarmingTargetVariant,
                    preference = preference,
                    effectivePreference = previousPreference
                )
            )
        )
        hlsAheadPrefetcher.warmupQualitySwitch(
            HlsQualitySwitchWarmupRequest(
                mediaUrl = mediaUrl,
                currentPositionMs = exoPlayer.currentPosition.coerceAtLeast(0L),
                targetHeight = preference.height ?: MinVideoHeight,
                skipSegments = plan.skipSegments,
                segmentWindow = plan.warmSegments
            )
        ) { result ->
            mainHandler.post {
                val pending = pendingQualitySwitch
                if (pending == null || pending.generation != generation) return@post
                val estimate = bandwidthEstimator.record(
                    ThroughputSample(
                        bytesTransferred = result.bytesDownloaded,
                        durationMs = result.durationMs,
                        source = "quality_warmup_${preference.label}"
                    )
                )
                if (!result.success) {
                    pendingQualitySwitch = null
                    emit(
                        PlayerEvent.VideoQualitySwitchChanged(
                            PlayerVideoQualitySwitchState(
                                phase = PlayerVideoQualitySwitchPhase.Fallback,
                                preference = preference,
                                effectivePreference = previousPreference,
                                detail = "目标清晰度预热失败，继续保持当前播放。"
                            )
                        )
                    )
                    return@post
                }
                val commitDecision = BandwidthAwareQualitySwitchAdvisor.evaluateCommit(
                    QualitySwitchCommitContext(
                        targetBandwidthBps = result.targetBandwidthBps.coerceAtLeast(targetVariant.bitrate),
                        effectiveBufferedAheadMs = ((exoPlayer.bufferedPosition - exoPlayer.currentPosition)
                            .coerceAtLeast(0L) / exoPlayer.playbackParameters.speed.coerceAtLeast(0.25f)).toLong(),
                        warmedSegments = result.warmedSegments + result.reusedSegments,
                        bandwidthEstimate = estimate,
                        playbackSpeed = exoPlayer.playbackParameters.speed,
                        rebufferCount = latestRebufferCount,
                        targetVariant = targetVariant
                    )
                )
                if (!commitDecision.allowCommit) {
                    pendingQualitySwitch = null
                    emit(
                        PlayerEvent.VideoQualitySwitchChanged(
                            PlayerVideoQualitySwitchState(
                                phase = PlayerVideoQualitySwitchPhase.Blocked,
                                preference = preference,
                                effectivePreference = previousPreference,
                                detail = commitDecision.reason
                            )
                        )
                    )
                    PlayerDebugLog.d(
                        ABR_LOG_TAG,
                        "quality switch commit blocked target=${preference.label} reason=${commitDecision.reason}"
                    )
                    return@post
                }

                emit(
                    PlayerEvent.VideoQualitySwitchChanged(
                        PlayerVideoQualitySwitchState(
                            phase = PlayerVideoQualitySwitchPhase.ReadyToCommit,
                            preference = preference,
                            effectivePreference = previousPreference,
                            detail = "warmedSegments=${result.warmedSegments}"
                        )
                    )
                )
                latestQualityPreference = preference
                committedQualitySwitch = CommittedQualitySwitch(
                    generation = generation,
                    targetPreference = preference,
                    fallbackPreference = pending.fallbackPreference,
                    targetVariant = targetVariant,
                    startedAtMs = System.currentTimeMillis(),
                    startPositionMs = exoPlayer.currentPosition.coerceAtLeast(0L),
                    graceWindowMs = pending.plan.graceWindowMs
                )
                applyTrackSelection()
                scheduleCommittedQualitySwitchCheck(committedQualitySwitch!!)
            }
        }
        PlayerDebugLog.d(ABR_LOG_TAG, "quality preference=${preference.label} strategy=${latestPlaybackStrategy.logLabel}")
    }

    override fun updatePlaybackStrategy(playbackSpeed: Float, rebufferCount: Int) {
        latestRebufferCount = rebufferCount
        val nextStrategy = PlayerPlaybackStrategy.forPlayback(
            playbackSpeed = playbackSpeed,
            rebufferCount = rebufferCount
        )
        if (nextStrategy == latestPlaybackStrategy) return
        latestPlaybackStrategy = nextStrategy
        applyTrackSelection()
        PlayerDebugLog.d(
            ABR_LOG_TAG,
            "playback strategy=${nextStrategy.logLabel} speed=${playbackSpeed}x " +
                "rebufferCount=$rebufferCount qualityPreference=${latestQualityPreference.label}"
        )
    }

    override fun updateAheadPrefetch(
        mediaUrl: String,
        currentPositionMs: Long,
        playbackSpeed: Float,
        effectiveBufferedAheadMs: Long,
        estimatedSegmentsAhead: Int,
        rebufferCount: Int,
        videoVariant: PlayerVideoVariant
    ) {
        val preparationPlan = BandwidthAwareQualitySwitchAdvisor.planPreparation(
            VariantPreparationContext(
                playbackSpeed = playbackSpeed,
                effectiveBufferedAheadMs = effectiveBufferedAheadMs,
                estimatedSegmentsAhead = estimatedSegmentsAhead,
                rebufferCount = rebufferCount,
                currentVariant = videoVariant,
                availableVariants = latestAvailableVideoVariants,
                bandwidthEstimate = bandwidthEstimator.currentEstimate()
            )
        )
        hlsAheadPrefetcher.update(
            HlsAheadPrefetchRequest(
                mediaUrl = mediaUrl,
                currentPositionMs = currentPositionMs,
                playbackSpeed = playbackSpeed,
                effectiveBufferedAheadMs = effectiveBufferedAheadMs,
                estimatedSegmentsAhead = estimatedSegmentsAhead,
                rebufferCount = rebufferCount,
                videoVariant = videoVariant,
                preparationPlan = preparationPlan.takeIf { it.shouldPrepare }
            )
        )
        if (preparationPlan.shouldPrepare) {
            PlayerDebugLog.d(
                ABR_LOG_TAG,
                "prepare plan target=${preparationPlan.targetVariant?.qualityLabel} " +
                    "skip=${preparationPlan.skipSegments} warm=${preparationPlan.warmSegments} " +
                    "keepWindowMs=${preparationPlan.keepAheadWindowMs} reason=${preparationPlan.reason}"
            )
        }
    }

    override fun release() {
        detach()
        hlsAheadPrefetcher.release()
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
        PlayerDebugLog.d(ABR_LOG_TAG, "variant changed reason=$reason ${variant.debugLabel}")
        emit(PlayerEvent.VideoVariantChanged(variant, reason = reason))
        val pending = pendingQualitySwitch
        if (pending != null && pending.preference.height == variant.height) {
            pendingQualitySwitch = null
            emit(
                PlayerEvent.VideoQualitySwitchChanged(
                    PlayerVideoQualitySwitchState(
                        phase = PlayerVideoQualitySwitchPhase.Committed,
                        preference = pending.preference,
                        effectivePreference = pending.preference
                    )
                )
            )
            committedQualitySwitch = committedQualitySwitch?.copy(
                renderedFirstFrame = false,
                startPositionMs = exoPlayer.currentPosition.coerceAtLeast(0L),
                startedAtMs = System.currentTimeMillis()
            )
        }
    }

    private fun scheduleCommittedQualitySwitchCheck(state: CommittedQualitySwitch) {
        mainHandler.postDelayed({
            val active = committedQualitySwitch ?: return@postDelayed
            if (active.generation != state.generation) return@postDelayed
            if (latestVideoVariant.height != active.targetVariant.height) return@postDelayed
            val advancedPositionMs = (exoPlayer.currentPosition - active.startPositionMs).coerceAtLeast(0L)
            val observation = QualitySwitchFallbackObservation(
                advancedPositionMs = advancedPositionMs,
                renderedFirstFrame = active.renderedFirstFrame,
                effectiveBufferedAheadMs = ((exoPlayer.bufferedPosition - exoPlayer.currentPosition)
                    .coerceAtLeast(0L) / exoPlayer.playbackParameters.speed.coerceAtLeast(0.25f)).toLong(),
                playbackState = exoPlayer.playbackState,
                targetVariant = active.targetVariant
            )
            if (!BandwidthAwareQualitySwitchAdvisor.shouldFallback(observation)) {
                committedQualitySwitch = null
                return@postDelayed
            }
            fallbackQualitySwitch(active, "切到 ${active.targetPreference.label} 后恢复不稳定，已自动回退。")
        }, state.graceWindowMs)
    }

    private fun fallbackQualitySwitch(state: CommittedQualitySwitch, detail: String) {
        committedQualitySwitch = null
        pendingQualitySwitch = null
        latestQualityPreference = state.fallbackPreference
        applyTrackSelection()
        val resume = exoPlayer.playWhenReady
        val currentPosition = exoPlayer.currentPosition.coerceAtLeast(0L)
        exoPlayer.seekTo(currentPosition)
        if (resume) {
            exoPlayer.play()
        }
        emit(
            PlayerEvent.VideoQualitySwitchChanged(
                PlayerVideoQualitySwitchState(
                    phase = PlayerVideoQualitySwitchPhase.Fallback,
                    preference = state.targetPreference,
                    effectivePreference = state.fallbackPreference,
                    detail = detail
                )
            )
        )
    }

    private fun applyTrackSelection() {
        trackSelector.setParameters(
            trackSelector.buildUponParameters().apply {
                setExceedVideoConstraintsIfNecessary(false)
                setAllowVideoNonSeamlessAdaptiveness(true)
                setMinVideoSize(MinVideoWidth, MinVideoHeight)
                clearVideoSizeConstraints()
                setMinVideoSize(MinVideoWidth, MinVideoHeight)
                setForceHighestSupportedBitrate(false)
                setForceLowestBitrate(false)

                val manualHeight = latestQualityPreference.height
                when {
                    latestPlaybackStrategy.lockToMobileFast720p -> {
                        setMaxVideoSize(MobileFastVideoWidth, MobileFastVideoHeight)
                    }
                    manualHeight != null -> {
                        setMaxVideoSize(Int.MAX_VALUE, manualHeight)
                        setForceHighestSupportedBitrate(true)
                    }
                    else -> Unit
                }
            }
        )
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
        PlayerDebugLog.d(
            ABR_LOG_TAG,
            "available variants ${variants.joinToString { it.debugLabel }}"
        )
    }

    private fun List<PlayerVideoVariant>.toQualityOptions(): List<PlayerVideoQualityOption> {
        val manualOptions = mapNotNull { variant ->
            variant.height.takeIf { it >= MinVideoHeight }
        }
            .distinct()
            .sortedDescending()
            .map { height -> PlayerVideoQualityOption(height = height, label = "${height}p") }
        return listOf(PlayerVideoQualityOption.Auto) + manualOptions
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

private data class PendingQualitySwitch(
    val generation: Long,
    val preference: PlayerVideoQualityPreference,
    val fallbackPreference: PlayerVideoQualityPreference,
    val plan: QualitySwitchPlan,
)

private data class CommittedQualitySwitch(
    val generation: Long,
    val targetPreference: PlayerVideoQualityPreference,
    val fallbackPreference: PlayerVideoQualityPreference,
    val targetVariant: PlayerVideoVariant,
    val startedAtMs: Long,
    val startPositionMs: Long,
    val graceWindowMs: Long,
    val renderedFirstFrame: Boolean = false,
)

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
