package com.example.watch_together.config

import com.example.watch_together.BuildConfig
import java.net.URI

object AppConfig {
    val appEnv: String = BuildConfig.APP_ENV
    val apiBaseUrl: String = BuildConfig.API_BASE_URL
    val wsBaseUrl: String = BuildConfig.WS_BASE_URL
    val mediaBaseUrl: String = BuildConfig.MEDIA_BASE_URL
    val mediaDefaultId: String = BuildConfig.MEDIA_DEFAULT_ID
    val rewriteLoopbackMediaUrls: Boolean = BuildConfig.REWRITE_LOOPBACK_MEDIA_URLS
    val debugSync: Boolean = BuildConfig.DEBUG_SYNC
    val wsReconnectInitialDelayMs: Long = BuildConfig.WS_RECONNECT_INITIAL_DELAY_MS
    val wsReconnectMaxDelayMs: Long = BuildConfig.WS_RECONNECT_MAX_DELAY_MS

    fun authLoginUrl(): String = "${apiBaseUrl.trimEnd('/')}/auth/login"

    fun homeSummaryUrl(): String = "${apiBaseUrl.trimEnd('/')}/home/summary"

    fun mediaTagsUrl(): String = "${apiBaseUrl.trimEnd('/')}/media/tags"

    fun mediaItemsUrl(): String = "${apiBaseUrl.trimEnd('/')}/media/items"

    fun roomsUrl(): String = "${apiBaseUrl.trimEnd('/')}/rooms"

    fun roomDetailUrl(roomCode: String): String = "${roomsUrl()}/${roomCode.trim().uppercase()}"

    fun roomJoinUrl(roomCode: String): String = "${roomDetailUrl(roomCode)}/join"

    fun mediaProgressUrl(episodeId: String): String =
        "${apiBaseUrl.trimEnd('/')}/me/media-progress/$episodeId"

    fun defaultMediaIdForRoom(): String = mediaDefaultId.ifBlank { "sample_001" }

    fun mediaUrlFor(mediaId: String): String {
        return if (mediaDefaultId.isBlank() && mediaId == "sample_001") {
            "https://storage.googleapis.com/shaka-demo-assets/angel-one-hls/hls.m3u8"
        } else {
            "${mediaBaseUrl.trimEnd('/')}/$mediaId/index.m3u8"
        }
    }

    fun playableMediaUrl(rawUrl: String): String =
        rewriteLoopbackMediaUrl(
            rawUrl = rawUrl,
            androidMediaBaseUrl = mediaBaseUrl,
            enabled = rewriteLoopbackMediaUrls
        )

    fun sampleHlsUrl(): String = mediaUrlFor(defaultMediaIdForRoom())
}

fun rewriteLoopbackMediaUrl(rawUrl: String, androidMediaBaseUrl: String, enabled: Boolean = true): String {
    if (!enabled) return rawUrl

    val sourceUri = runCatching { URI(rawUrl) }.getOrNull() ?: return rawUrl
    val sourceHost = sourceUri.host ?: return rawUrl
    if (!sourceHost.isLoopbackHost()) return rawUrl

    val androidBaseUri = runCatching { URI(androidMediaBaseUrl) }.getOrNull() ?: return rawUrl
    val androidHost = androidBaseUri.host ?: return rawUrl
    val rewrittenPort = if (sourceUri.port >= 0) sourceUri.port else androidBaseUri.port

    return URI(
        androidBaseUri.scheme ?: sourceUri.scheme,
        sourceUri.userInfo,
        androidHost,
        rewrittenPort,
        sourceUri.path,
        sourceUri.query,
        sourceUri.fragment
    ).toString()
}

private fun String.isLoopbackHost(): Boolean {
    val normalized = trim().lowercase()
    return normalized == "127.0.0.1" ||
        normalized == "localhost" ||
        normalized == "0.0.0.0" ||
        normalized == "::1"
}
