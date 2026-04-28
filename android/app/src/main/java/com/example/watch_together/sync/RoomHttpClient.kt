package com.example.watch_together.sync

import com.example.watch_together.config.AppConfig
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

data class CreateRoomResult(
    val roomId: String,
    val roomCode: String,
    val media: RoomMedia,
    val roomState: RoomSyncState
)

data class RoomMedia(
    val id: String,
    val title: String,
    val subtitle: String?,
    val mediaUrl: String,
    val durationMs: Long?,
    val seasonLabel: String?,
    val episodeLabel: String?
)

class RoomHttpClient(
    private val okHttpClient: OkHttpClient = OkHttpClient()
) {

    // createRoom uses the DB-backed POST /rooms entrypoint and returns the
    // business media data needed to load the selected video into the theater page.
    fun createRoom(accessToken: String, mediaItemId: String): CreateRoomResult {
        val requestBody = JSONObject()
            .put("mediaItemId", mediaItemId)
            .toString()
            .toRequestBody("application/json; charset=utf-8".toMediaType())

        val request = Request.Builder()
            .url(AppConfig.roomsUrl())
            .header("Authorization", "Bearer $accessToken")
            .post(requestBody)
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            val responseBody = response.body?.string().orEmpty()
            if (!response.isSuccessful) {
                throw RoomHttpRequestException(
                    message = errorMessageFrom(responseBody) ?: "Create room failed with ${response.code}",
                    statusCode = response.code
                )
            }
            if (responseBody.isBlank()) {
                throw RoomHttpRequestException("Create room returned an empty body", response.code)
            }
            val data = JSONObject(responseBody).getJSONObject("data")
            val roomJson = data.getJSONObject("room")
            val mediaJson = data.getJSONObject("media")
            val stateJson = data.getJSONObject("roomState")

            val roomCode = roomJson.getString("roomCode")
            val media = mediaJson.toRoomMedia()

            return CreateRoomResult(
                roomId = roomCode,
                roomCode = roomCode,
                media = media,
                roomState = RoomSyncState(
                    roomId = roomCode,
                    mediaId = media.id,
                    hostUserId = roomJson.getString("hostUserId"),
                    paused = stateJson.getBoolean("paused"),
                    ended = stateJson.optBoolean("ended", false),
                    positionMs = stateJson.getLong("positionMs"),
                    playbackRate = stateJson.getDouble("playbackRate"),
                    seq = stateJson.getLong("seq")
                )
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

class RoomHttpRequestException(
    override val message: String,
    val statusCode: Int? = null
) : Exception(message)

private fun JSONObject.toRoomMedia(): RoomMedia {
    return RoomMedia(
        id = getString("id"),
        title = getString("title"),
        subtitle = optNullableString("subtitle"),
        mediaUrl = getString("mediaUrl"),
        durationMs = optNullableLong("durationMs"),
        seasonLabel = optNullableString("seasonLabel"),
        episodeLabel = optNullableString("episodeLabel")
    )
}

private fun JSONObject.optNullableString(name: String): String? {
    if (!has(name) || isNull(name)) return null
    return optString(name).takeUnless { it.isBlank() || it == "null" }
}

private fun JSONObject.optNullableLong(name: String): Long? {
    if (!has(name) || isNull(name)) return null
    return optLong(name)
}
