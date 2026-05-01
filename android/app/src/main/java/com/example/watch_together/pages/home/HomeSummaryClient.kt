package com.example.watch_together.pages.home

import com.example.watch_together.config.AppConfig
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject

data class HomeSummary(
    val user: HomeSummaryUser,
    val lastWatched: HomeWatchProgress?,
    val continueWatching: List<HomeWatchProgress>
)

data class HomeSummaryUser(
    val nickname: String,
    val avatarSeed: String,
    val avatarUrl: String?
)

data class HomeWatchProgress(
    val episodeId: String,
    val title: String,
    val coverUrl: String?,
    val lastPositionSeconds: Int,
    val durationSeconds: Int
)

class HomeSummaryClient(
    private val okHttpClient: OkHttpClient = OkHttpClient()
) {
    fun fetch(accessToken: String): HomeSummary {
        val request = Request.Builder()
            .url(AppConfig.homeSummaryUrl())
            .header("Authorization", "Bearer $accessToken")
            .get()
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            val responseBody = response.body?.string().orEmpty()
            if (!response.isSuccessful) {
                throw HomeSummaryRequestException(
                    message = errorMessageFrom(responseBody) ?: "首页数据加载失败，请稍后重试",
                    statusCode = response.code
                )
            }
            if (responseBody.isBlank()) {
                throw HomeSummaryRequestException("首页数据响应为空", response.code)
            }

            val data = JSONObject(responseBody).getJSONObject("data")
            val user = data.getJSONObject("user")
            val continueWatching = data.optJSONArray("continueWatching")

            return HomeSummary(
                user = HomeSummaryUser(
                    nickname = user.getString("nickname"),
                    avatarSeed = user.optString("avatarSeed"),
                    avatarUrl = user.optNullableString("avatarUrl")
                ),
                lastWatched = data.optJSONObject("lastWatched")?.toWatchProgress(),
                continueWatching = buildList {
                    if (continueWatching == null) return@buildList
                    for (index in 0 until continueWatching.length()) {
                        add(continueWatching.getJSONObject(index).toWatchProgress())
                    }
                }
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

class HomeSummaryRequestException(
    override val message: String,
    val statusCode: Int? = null
) : Exception(message)

private fun JSONObject.toWatchProgress(): HomeWatchProgress {
    return HomeWatchProgress(
        episodeId = getString("mediaItemId"),
        title = getString("title"),
        coverUrl = optNullableString("coverUrl"),
        lastPositionSeconds = optInt("lastPositionSeconds"),
        durationSeconds = optInt("durationSeconds")
    )
}

private fun JSONObject.optNullableString(name: String): String? {
    if (!has(name) || isNull(name)) return null
    return optString(name).takeUnless { it.isBlank() || it == "null" }
}
