package com.example.watch_together.sync

import com.example.watch_together.config.AppConfig
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

data class ProgressUpdateResult(
    val mediaItemId: String,
    val lastPositionSeconds: Long,
    val durationSeconds: Long,
    val completed: Boolean
)

class ProgressHttpClient(
    private val okHttpClient: OkHttpClient = OkHttpClient()
) {
    // updateProgress is intentionally low frequency; it backs business history,
    // not realtime playback sync.
    fun updateProgress(
        accessToken: String,
        mediaItemId: String,
        lastPositionSeconds: Long,
        durationSeconds: Long,
        completed: Boolean,
        completionSource: String? = null
    ): ProgressUpdateResult {
        val requestJson = JSONObject()
            .put("lastPositionSeconds", lastPositionSeconds.coerceAtLeast(0L))
            .put("durationSeconds", durationSeconds.coerceAtLeast(1L))
            .put("completed", completed)
        if (completionSource != null) {
            requestJson.put("completionSource", completionSource)
        }

        val request = Request.Builder()
            .url(AppConfig.mediaProgressUrl(mediaItemId))
            .header("Authorization", "Bearer $accessToken")
            .put(requestJson.toString().toRequestBody("application/json; charset=utf-8".toMediaType()))
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            val responseBody = response.body?.string().orEmpty()
            if (!response.isSuccessful) {
                throw ProgressHttpRequestException(
                    message = errorMessageFrom(responseBody)
                        ?: "Update progress failed with ${response.code}",
                    statusCode = response.code
                )
            }
            if (responseBody.isBlank()) {
                throw ProgressHttpRequestException("Update progress returned an empty body", response.code)
            }

            val data = JSONObject(responseBody).getJSONObject("data")
            return ProgressUpdateResult(
                mediaItemId = data.getString("mediaItemId"),
                lastPositionSeconds = data.getLong("lastPositionSeconds"),
                durationSeconds = data.getLong("durationSeconds"),
                completed = data.getBoolean("completed")
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

class ProgressHttpRequestException(
    override val message: String,
    val statusCode: Int? = null
) : Exception(message)
