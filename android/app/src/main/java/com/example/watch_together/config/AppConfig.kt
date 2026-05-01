package com.example.watch_together.config

import com.example.watch_together.BuildConfig
import java.net.URI

object AppConfig {
    val appEnv: String = BuildConfig.APP_ENV
    val apiBaseUrl: String = BuildConfig.API_BASE_URL
    val wsBaseUrl: String = BuildConfig.WS_BASE_URL
    val mediaBaseUrl: String = BuildConfig.MEDIA_BASE_URL
    val mediaDefaultId: String = BuildConfig.MEDIA_DEFAULT_ID
    val debugSync: Boolean = BuildConfig.DEBUG_SYNC

    fun authLoginUrl(): String = "${apiBaseUrl.trimEnd('/')}/auth/login"

    fun homeSummaryUrl(): String = "${apiBaseUrl.trimEnd('/')}/home/summary"

    fun mediaTagsUrl(): String = "${apiBaseUrl.trimEnd('/')}/media/tags"

    fun mediaItemsUrl(): String = "${apiBaseUrl.trimEnd('/')}/media/items"

    fun roomsUrl(): String = "${apiBaseUrl.trimEnd('/')}/rooms"

    fun roomDetailUrl(roomCode: String): String = "${roomsUrl()}/${roomCode.trim().uppercase()}"

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
        rewriteLoopbackMediaUrl(rawUrl = rawUrl, androidMediaBaseUrl = mediaBaseUrl)

    fun sampleHlsUrl(): String = mediaUrlFor(defaultMediaIdForRoom())
}

fun rewriteLoopbackMediaUrl(rawUrl: String, androidMediaBaseUrl: String): String {
    val sourceUri = runCatching { URI(rawUrl) }.getOrNull() ?: return rawUrl
    val sourceHost = sourceUri.host ?: return rawUrl
    if (!sourceHost.isLoopbackHost()) return rawUrl

    val androidBaseUri = runCatching { URI(androidMediaBaseUrl) }.getOrNull() ?: return rawUrl
    val androidHost = androidBaseUri.host ?: return rawUrl

    return URI(
        androidBaseUri.scheme ?: sourceUri.scheme,
        sourceUri.userInfo,
        androidHost,
        androidBaseUri.port,
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
