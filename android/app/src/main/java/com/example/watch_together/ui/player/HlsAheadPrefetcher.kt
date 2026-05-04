package com.example.watch_together.ui.player

import android.net.Uri
import android.util.Log
import androidx.annotation.OptIn
import androidx.media3.common.C
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.DataSpec
import androidx.media3.datasource.cache.CacheDataSource
import androidx.media3.datasource.cache.CacheWriter
import java.io.ByteArrayOutputStream
import java.net.URI
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.math.abs

@OptIn(UnstableApi::class)
internal class HlsAheadPrefetcher(
    private val dataSourceFactory: CacheDataSource.Factory,
    private val logTag: String = "WatchTogetherPrefetch",
) {
    private val executor: ExecutorService = Executors.newSingleThreadExecutor { runnable ->
        Thread(runnable, "watch-together-hls-prefetch").apply {
            isDaemon = true
        }
    }
    private val prefetching = AtomicBoolean(false)
    private var lastRequestKey: String = ""
    private var latestMediaUrl: String = ""
    private var latestVariantProfile: String = ""
    private var cachedPlaylist: HlsMediaPlaylist? = null

    fun update(request: HlsAheadPrefetchRequest) {
        if (!request.mediaUrl.isHlsPlaylistUrl()) return
        val window = request.prefetchWindowSegments()
        if (window <= 0) return

        val currentSegmentIndex = request.estimatedCurrentSegmentIndex()
        val startSegmentIndex = currentSegmentIndex + 1
        val requestKey = listOf(
            request.mediaUrl,
            startSegmentIndex,
            window,
            request.playbackSpeed,
            request.rebufferCount,
            request.videoVariant.height
        ).joinToString("|")

        if (requestKey == lastRequestKey) return
        if (!prefetching.compareAndSet(false, true)) return
        lastRequestKey = requestKey

        executor.execute {
            try {
                prefetch(request, startSegmentIndex, window)
            } catch (error: Throwable) {
                Log.d(logTag, "prefetch failed ${error.message}")
            } finally {
                prefetching.set(false)
            }
        }
    }

    fun release() {
        executor.shutdownNow()
    }

    private fun prefetch(
        request: HlsAheadPrefetchRequest,
        startSegmentIndex: Int,
        window: Int
    ) {
        val playlist = getPlaylist(request) ?: return
        val segmentUrls = playlist.segments
            .drop(startSegmentIndex.coerceAtLeast(0))
            .take(window)
            .map { segment -> segment.url }

        if (segmentUrls.isEmpty()) return

        Log.d(
            logTag,
            "prefetch start speed=${request.playbackSpeed}x currentSegment=${startSegmentIndex - 1} " +
                "count=${segmentUrls.size} effectiveAhead=${request.effectiveBufferedAheadMs}ms " +
                "segmentsAhead=${request.estimatedSegmentsAhead} variant=${request.videoVariant.displayLabel}"
        )

        var completed = 0
        segmentUrls.forEach { segmentUrl ->
            runCatching {
                CacheWriter(
                    dataSourceFactory.createDataSource(),
                    DataSpec(Uri.parse(segmentUrl)),
                    null,
                    null
                ).cache()
                completed += 1
            }.onFailure { error ->
                Log.d(logTag, "segment prefetch failed url=$segmentUrl error=${error.message}")
            }
        }

        Log.d(
            logTag,
            "prefetch done completed=$completed requested=${segmentUrls.size} " +
                "fromSegment=$startSegmentIndex window=$window"
        )
    }

    private fun getPlaylist(request: HlsAheadPrefetchRequest): HlsMediaPlaylist? {
        val variantProfile = request.preferredVariantProfile()
        val reusable = latestMediaUrl == request.mediaUrl &&
            latestVariantProfile == variantProfile &&
            cachedPlaylist != null
        if (reusable) return cachedPlaylist

        latestMediaUrl = request.mediaUrl
        latestVariantProfile = variantProfile
        cachedPlaylist = null

        val rootText = readText(request.mediaUrl)
        val rootMediaPlaylist = parseHlsMediaPlaylist(request.mediaUrl, rootText)
        val mediaPlaylist = if (rootMediaPlaylist.segments.isNotEmpty()) {
            rootMediaPlaylist
        } else {
            val rootMasterPlaylist = parseHlsMasterPlaylist(request.mediaUrl, rootText)
            val variantUrl = rootMasterPlaylist.selectVariantUrl(request) ?: run {
                Log.d(logTag, "prefetch skipped: no playable variant in master url=${request.mediaUrl}")
                return null
            }
            val variantText = readText(variantUrl)
            parseHlsMediaPlaylist(variantUrl, variantText)
        }

        if (mediaPlaylist.segments.isEmpty()) {
            Log.d(logTag, "prefetch skipped: media playlist has no segments url=${mediaPlaylist.url}")
            return null
        }

        cachedPlaylist = mediaPlaylist
        Log.d(
            logTag,
            "playlist ready url=${mediaPlaylist.url} segments=${mediaPlaylist.segments.size} " +
                "avgSegmentMs=${mediaPlaylist.averageSegmentMs}"
        )
        return mediaPlaylist
    }

    private fun readText(url: String): String {
        val dataSource = dataSourceFactory.createDataSource()
        return try {
            dataSource.open(DataSpec(Uri.parse(url)))
            val output = ByteArrayOutputStream()
            val buffer = ByteArray(DEFAULT_READ_BUFFER_BYTES)
            while (true) {
                val read = dataSource.read(buffer, 0, buffer.size)
                if (read == C.RESULT_END_OF_INPUT) break
                if (read > 0) output.write(buffer, 0, read)
            }
            output.toString(Charsets.UTF_8.name())
        } finally {
            runCatching { dataSource.close() }
        }
    }

    private fun HlsMasterPlaylist.selectVariantUrl(request: HlsAheadPrefetchRequest): String? {
        if (variants.isEmpty()) return null
        val preferredHeight = when {
            request.playbackSpeed >= HIGH_SPEED_PREFETCH_THRESHOLD || request.rebufferCount >= REBUFFER_PREFETCH_THRESHOLD -> 720
            request.videoVariant.height > 0 -> request.videoVariant.height
            else -> 720
        }
        return variants
            .sortedWith(
                compareBy<HlsVariant> { variant ->
                    if (variant.url.contains("720p-fast", ignoreCase = true) &&
                        preferredHeight <= 720
                    ) {
                        0
                    } else {
                        1
                    }
                }.thenBy { variant ->
                    if (variant.height > 0) abs(variant.height - preferredHeight) else Int.MAX_VALUE
                }.thenBy { variant -> variant.bandwidth }
            )
            .firstOrNull()
            ?.url
    }

    private companion object {
        const val DEFAULT_READ_BUFFER_BYTES = 16 * 1024
        const val HIGH_SPEED_PREFETCH_THRESHOLD = 2.0f
        const val MEDIUM_SPEED_PREFETCH_THRESHOLD = 1.5f
        const val REBUFFER_PREFETCH_THRESHOLD = 2
        const val LOW_EFFECTIVE_AHEAD_MS = 12_000L

        fun HlsAheadPrefetchRequest.prefetchWindowSegments(): Int {
            return when {
                playbackSpeed >= HIGH_SPEED_PREFETCH_THRESHOLD -> 10
                rebufferCount >= REBUFFER_PREFETCH_THRESHOLD -> 8
                playbackSpeed >= MEDIUM_SPEED_PREFETCH_THRESHOLD -> 5
                effectiveBufferedAheadMs < LOW_EFFECTIVE_AHEAD_MS -> 3
                else -> 0
            }
        }

        fun HlsAheadPrefetchRequest.estimatedCurrentSegmentIndex(): Int {
            val segmentMs = DEFAULT_HLS_SEGMENT_DURATION_MS
            return (currentPositionMs / segmentMs).toInt().coerceAtLeast(0)
        }

        fun HlsAheadPrefetchRequest.preferredVariantProfile(): String {
            return when {
                playbackSpeed >= HIGH_SPEED_PREFETCH_THRESHOLD || rebufferCount >= REBUFFER_PREFETCH_THRESHOLD -> "mobile_fast_720p"
                videoVariant.height > 0 -> "${videoVariant.height}p"
                else -> "auto_720p"
            }
        }
    }
}

internal data class HlsAheadPrefetchRequest(
    val mediaUrl: String,
    val currentPositionMs: Long,
    val playbackSpeed: Float,
    val effectiveBufferedAheadMs: Long,
    val estimatedSegmentsAhead: Int,
    val rebufferCount: Int,
    val videoVariant: PlayerVideoVariant,
)

internal data class HlsMediaPlaylist(
    val url: String,
    val segments: List<HlsSegment>
) {
    val averageSegmentMs: Long
        get() {
            if (segments.isEmpty()) return DEFAULT_HLS_SEGMENT_DURATION_MS
            return segments.map { it.durationMs }.average().toLong()
        }
}

internal data class HlsSegment(
    val url: String,
    val durationMs: Long
)

private data class HlsMasterPlaylist(
    val variants: List<HlsVariant>
)

private data class HlsVariant(
    val url: String,
    val bandwidth: Int,
    val height: Int
)

internal fun parseHlsMediaPlaylist(url: String, content: String): HlsMediaPlaylist {
    val lines = content.lineSequence()
        .map { line -> line.trim() }
        .filter { line -> line.isNotEmpty() }
        .toList()

    val segments = mutableListOf<HlsSegment>()
    var pendingDurationMs: Long? = null
    lines.forEach { line ->
        when {
            line.startsWith("#EXTINF:") -> {
                pendingDurationMs = line
                    .substringAfter("#EXTINF:")
                    .substringBefore(",")
                    .toDoubleOrNull()
                    ?.let { seconds -> (seconds * 1_000).toLong() }
                    ?: DEFAULT_HLS_SEGMENT_DURATION_MS
            }
            !line.startsWith("#") && pendingDurationMs != null -> {
                segments += HlsSegment(
                    url = resolveHlsUrl(url, line),
                    durationMs = pendingDurationMs ?: DEFAULT_HLS_SEGMENT_DURATION_MS
                )
                pendingDurationMs = null
            }
        }
    }

    return HlsMediaPlaylist(url = url, segments = segments)
}

private fun parseHlsMasterPlaylist(url: String, content: String): HlsMasterPlaylist {
    val lines = content.lineSequence()
        .map { line -> line.trim() }
        .filter { line -> line.isNotEmpty() }
        .toList()
    val variants = mutableListOf<HlsVariant>()
    var pendingStreamInf: String? = null
    lines.forEach { line ->
        when {
            line.startsWith("#EXT-X-STREAM-INF:") -> pendingStreamInf = line
            !line.startsWith("#") && pendingStreamInf != null -> {
                val streamInf = pendingStreamInf.orEmpty()
                variants += HlsVariant(
                    url = resolveHlsUrl(url, line),
                    bandwidth = streamInf.attributeInt("BANDWIDTH"),
                    height = streamInf
                        .substringAfter("RESOLUTION=", "")
                        .substringBefore(",", "")
                        .substringAfter("x", "")
                        .toIntOrNull()
                        ?: 0
                )
                pendingStreamInf = null
            }
        }
    }
    return HlsMasterPlaylist(variants = variants)
}

private fun String.attributeInt(name: String): Int {
    return substringAfter("$name=", "")
        .substringBefore(",", "")
        .toIntOrNull()
        ?: 0
}

internal fun resolveHlsUrl(baseUrl: String, childUrl: String): String {
    return try {
        URI(baseUrl).resolve(childUrl).toString()
    } catch (_: IllegalArgumentException) {
        childUrl
    }
}

internal fun String.isHlsPlaylistUrl(): Boolean {
    return substringBefore('?').substringBefore('#').endsWith(".m3u8", ignoreCase = true)
}

private const val DEFAULT_HLS_SEGMENT_DURATION_MS = 6_000L
