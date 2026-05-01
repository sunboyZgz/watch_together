package com.example.watch_together.pages.video

import com.example.watch_together.config.AppConfig
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject

data class MediaTag(
    val id: String,
    val slug: String,
    val name: String
)

data class MediaCatalogTags(
    val featuredTags: List<MediaTag>,
    val allTags: List<MediaTag>
)

data class MediaEpisode(
    val episodeId: String,
    val title: String,
    val subtitle: String?,
    val description: String?,
    val coverUrl: String?,
    val durationMs: Long,
    val seasonLabel: String?,
    val episodeLabel: String?,
    val tags: List<MediaTag>
)

data class MediaItemsPage(
    val items: List<MediaEpisode>,
    val nextCursor: String?
)

class MediaCatalogClient(
    private val okHttpClient: OkHttpClient = OkHttpClient()
) {
    fun fetchTags(): MediaCatalogTags {
        val request = Request.Builder()
            .url(AppConfig.mediaTagsUrl())
            .get()
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            val responseBody = response.body?.string().orEmpty()
            if (!response.isSuccessful) {
                throw MediaCatalogRequestException(
                    message = errorMessageFrom(responseBody) ?: "标签加载失败，请稍后重试",
                    statusCode = response.code
                )
            }

            val data = JSONObject(responseBody).getJSONObject("data")
            return MediaCatalogTags(
                featuredTags = data.optJSONArray("featuredTags").toTags(),
                allTags = data.optJSONArray("allTags").toTags()
            )
        }
    }

    fun fetchItems(
        query: String,
        tagSlug: String?,
        limit: Int = 20,
        cursor: String? = null
    ): MediaItemsPage {
        val url = AppConfig.mediaItemsUrl()
            .toHttpUrl()
            .newBuilder()
            .addQueryParameter("limit", limit.toString())
            .apply {
                if (query.isNotBlank()) addQueryParameter("query", query.trim())
                if (!tagSlug.isNullOrBlank()) addQueryParameter("tag", tagSlug)
                if (!cursor.isNullOrBlank()) addQueryParameter("cursor", cursor)
            }
            .build()

        val request = Request.Builder()
            .url(url)
            .get()
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            val responseBody = response.body?.string().orEmpty()
            if (!response.isSuccessful) {
                throw MediaCatalogRequestException(
                    message = errorMessageFrom(responseBody) ?: "影片加载失败，请稍后重试",
                    statusCode = response.code
                )
            }

            val root = JSONObject(responseBody)
            val data = root.getJSONObject("data")
            val page = root.optJSONObject("meta")?.optJSONObject("page")
            return MediaItemsPage(
                items = data.optJSONArray("items").toMediaItems(),
                nextCursor = page?.optNullableString("nextCursor")
            )
        }
    }

    private fun errorMessageFrom(responseBody: String): String? {
        if (responseBody.isBlank()) return null
        return runCatching {
            val error = JSONObject(responseBody).optJSONObject("error")
            error?.optString("message")?.takeUnless { it.isBlank() }
        }.getOrNull()
    }
}

class MediaCatalogRequestException(
    override val message: String,
    val statusCode: Int? = null
) : Exception(message)

private fun org.json.JSONArray?.toTags(): List<MediaTag> {
    if (this == null) return emptyList()
    return buildList {
        for (index in 0 until length()) {
            val item = getJSONObject(index)
            add(
                MediaTag(
                    id = item.optString("id"),
                    slug = item.getString("slug"),
                    name = item.getString("name")
                )
            )
        }
    }
}

private fun org.json.JSONArray?.toMediaItems(): List<MediaEpisode> {
    if (this == null) return emptyList()
    return buildList {
        for (index in 0 until length()) {
            val item = getJSONObject(index)
            add(
                MediaEpisode(
                    episodeId = item.getString("id"),
                    title = item.getString("title"),
                    subtitle = item.optNullableString("subtitle"),
                    description = item.optNullableString("description"),
                    coverUrl = item.optNullableString("coverUrl"),
                    durationMs = item.optLong("durationMs"),
                    seasonLabel = item.optNullableString("seasonLabel"),
                    episodeLabel = item.optNullableString("episodeLabel"),
                    tags = item.optJSONArray("tags").toTags()
                )
            )
        }
    }
}

private fun JSONObject.optNullableString(name: String): String? {
    if (!has(name) || isNull(name)) return null
    return optString(name).takeUnless { it.isBlank() || it == "null" }
}
