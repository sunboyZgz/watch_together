package com.example.watch_together.config

import org.junit.Assert.assertEquals
import org.junit.Test

class AppConfigTest {

    @Test
    fun `loopback media url rewrites to android reachable media host`() {
        val rewritten = rewriteLoopbackMediaUrl(
            rawUrl = "http://127.0.0.1:9000/media/tmp/media/sample-show/season-01/episode-01/hls/index.m3u8",
            androidMediaBaseUrl = "http://10.0.2.2:9000/media/tmp"
        )

        assertEquals(
            "http://10.0.2.2:9000/media/tmp/media/sample-show/season-01/episode-01/hls/index.m3u8",
            rewritten
        )
    }

    @Test
    fun `loopback media url keeps backend port when rewriting host`() {
        val rewritten = rewriteLoopbackMediaUrl(
            rawUrl = "http://127.0.0.1:9100/watch-together-media/media/sample-show/season-01/episode-01/hls/master.m3u8",
            androidMediaBaseUrl = "http://10.0.2.2:9000/media/tmp"
        )

        assertEquals(
            "http://10.0.2.2:9100/watch-together-media/media/sample-show/season-01/episode-01/hls/master.m3u8",
            rewritten
        )
    }

    @Test
    fun `public media url is not rewritten`() {
        val publicUrl = "https://cdn.example.com/media/sample/index.m3u8"

        assertEquals(
            publicUrl,
            rewriteLoopbackMediaUrl(
                rawUrl = publicUrl,
                androidMediaBaseUrl = "http://10.0.2.2:9000/media/tmp"
            )
        )
    }

    @Test
    fun `loopback rewrite can be disabled`() {
        val rawUrl = "http://127.0.0.1:9100/watch-together-media/media/sample-show/season-01/episode-01/hls/master.m3u8"

        assertEquals(
            rawUrl,
            rewriteLoopbackMediaUrl(
                rawUrl = rawUrl,
                androidMediaBaseUrl = "http://10.0.2.2:9000/media/tmp",
                enabled = false
            )
        )
    }
}
