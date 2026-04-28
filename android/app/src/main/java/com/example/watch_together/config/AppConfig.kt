package com.example.watch_together.config

import com.example.watch_together.BuildConfig

object AppConfig {
    val appEnv: String = BuildConfig.APP_ENV
    val apiBaseUrl: String = BuildConfig.API_BASE_URL
    val wsBaseUrl: String = BuildConfig.WS_BASE_URL
    val mediaBaseUrl: String = BuildConfig.MEDIA_BASE_URL
    val mediaDefaultId: String = BuildConfig.MEDIA_DEFAULT_ID
    val debugSync: Boolean = BuildConfig.DEBUG_SYNC

    fun authLoginUrl(): String = "${apiBaseUrl.trimEnd('/')}/auth/login"

    fun homeSummaryUrl(): String = "${apiBaseUrl.trimEnd('/')}/home/summary"

    fun roomsUrl(): String = "${apiBaseUrl.trimEnd('/')}/rooms"

    fun defaultMediaIdForRoom(): String = mediaDefaultId.ifBlank { "sample_001" }

    fun mediaUrlFor(mediaId: String): String {
        return if (mediaDefaultId.isBlank() && mediaId == "sample_001") {
            "https://storage.googleapis.com/shaka-demo-assets/angel-one-hls/hls.m3u8"
        } else {
            "${mediaBaseUrl.trimEnd('/')}/$mediaId/index.m3u8"
        }
    }

    fun sampleHlsUrl(): String = mediaUrlFor(defaultMediaIdForRoom())
}
