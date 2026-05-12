package com.example.watch_together.ui.player_default

import org.junit.Assert.assertEquals
import org.junit.Test

class HlsAheadPrefetcherTest {
    @Test
    fun parsesMediaPlaylistSegmentsWithResolvedUrls() {
        val playlist = parseHlsMediaPlaylist(
            url = "http://10.0.2.2:9000/media/show/season-01/episode-01/hls/720p-fast/index.m3u8",
            content = """
                #EXTM3U
                #EXT-X-VERSION:6
                #EXTINF:6.000000,
                segment_00000.ts
                #EXTINF:4.500000,
                segment_00001.ts
                #EXT-X-ENDLIST
            """.trimIndent()
        )

        assertEquals(2, playlist.segments.size)
        assertEquals(
            "http://10.0.2.2:9000/media/show/season-01/episode-01/hls/720p-fast/segment_00000.ts",
            playlist.segments[0].url
        )
        assertEquals(6_000L, playlist.segments[0].durationMs)
        assertEquals(4_500L, playlist.segments[1].durationMs)
    }

    @Test
    fun resolvesRelativeHlsUrlsFromMasterPlaylistPath() {
        val resolved = resolveHlsUrl(
            baseUrl = "http://10.0.2.2:9000/media/show/season-01/episode-01/hls/master.m3u8",
            childUrl = "720p-fast/index.m3u8"
        )

        assertEquals(
            "http://10.0.2.2:9000/media/show/season-01/episode-01/hls/720p-fast/index.m3u8",
            resolved
        )
    }

    @Test
    fun qualitySwitchSegmentIndices_bridgeCurrentWindowBeforeFarAnchor() {
        val indices = qualitySwitchSegmentIndices(
            playlistSize = 20,
            currentSegmentIndex = 2,
            skipSegments = 4,
            backfillSegments = 1,
            bridgeSegments = 2,
            segmentWindow = 3
        )

        assertEquals(listOf(3, 4, 5, 6, 7, 8), indices)
    }

    @Test
    fun qualitySwitchSegmentIndices_keepsBridgeBeforeBackfillAroundCurrentWindow() {
        val indices = qualitySwitchSegmentIndices(
            playlistSize = 20,
            currentSegmentIndex = 10,
            skipSegments = 0,
            backfillSegments = 2,
            bridgeSegments = 2,
            segmentWindow = 4
        )

        assertEquals(listOf(11, 12, 8, 9, 10, 13), indices)
    }

    @Test
    fun contiguousBridgedSegments_countsContinuousPreparedRunFromCurrentWindow() {
        val bridged = contiguousBridgedSegments(
            currentSegmentIndex = 2,
            preparedIndices = linkedSetOf(3, 4, 6, 7)
        )

        assertEquals(2, bridged)
    }
}
