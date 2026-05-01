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

data class JoinRoomResult(
    val roomId: String,
    val roomCode: String,
    val hostUserId: String,
    val memberUserId: String,
    val memberRole: String
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

data class RoomMember(
    val userId: String,
    val nickname: String,
    val avatarSeed: String,
    val avatarUrl: String?,
    val role: String
)

data class RoomDetailResult(
    val roomId: String,
    val roomCode: String,
    val hostUserId: String,
    val status: String,
    val media: RoomMedia,
    val members: List<RoomMember>
)

class RoomHttpClient(
    private val okHttpClient: OkHttpClient = OkHttpClient()
) {

    // createRoom sends the selected episode id through the temporary mediaItemId
    // HTTP field while the API contract finishes its episode-backed migration.
    fun createRoom(accessToken: String, episodeId: String): CreateRoomResult {
        val requestBody = JSONObject()
            .put("mediaItemId", episodeId)
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

    // joinRoomByCode persists the current user as a room member before WebSocket join_room.
    fun joinRoomByCode(accessToken: String, roomCode: String): JoinRoomResult {
        val normalizedRoomCode = roomCode.trim().uppercase()
        val request = Request.Builder()
            .url(AppConfig.roomJoinUrl(normalizedRoomCode))
            .header("Authorization", "Bearer $accessToken")
            .post(ByteArray(0).toRequestBody(null))
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            val responseBody = response.body?.string().orEmpty()
            if (!response.isSuccessful) {
                throw RoomHttpRequestException(
                    message = errorMessageFrom(responseBody) ?: "Join room failed with ${response.code}",
                    statusCode = response.code
                )
            }
            if (responseBody.isBlank()) {
                throw RoomHttpRequestException("Join room returned an empty body", response.code)
            }

            val data = JSONObject(responseBody).getJSONObject("data")
            val roomJson = data.getJSONObject("room")
            val memberJson = data.getJSONObject("member")
            val joinedRoomCode = roomJson.getString("roomCode")
            return JoinRoomResult(
                roomId = joinedRoomCode,
                roomCode = joinedRoomCode,
                hostUserId = roomJson.getString("hostUserId"),
                memberUserId = memberJson.getString("userId"),
                memberRole = memberJson.getString("role")
            )
        }
    }

    // getRoomDetail reads business bootstrap data for the theater page.
    // Runtime playback authority still comes from WebSocket room_state.
    fun getRoomDetail(roomCode: String): RoomDetailResult {
        val normalizedRoomCode = roomCode.trim().uppercase()
        val request = Request.Builder()
            .url(AppConfig.roomDetailUrl(normalizedRoomCode))
            .get()
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            val responseBody = response.body?.string().orEmpty()
            if (!response.isSuccessful) {
                throw RoomHttpRequestException(
                    message = errorMessageFrom(responseBody) ?: "Get room detail failed with ${response.code}",
                    statusCode = response.code
                )
            }
            if (responseBody.isBlank()) {
                throw RoomHttpRequestException("Get room detail returned an empty body", response.code)
            }

            val data = JSONObject(responseBody).getJSONObject("data")
            val roomJson = data.getJSONObject("room")
            return RoomDetailResult(
                roomId = roomJson.getString("id"),
                roomCode = roomJson.getString("roomCode"),
                hostUserId = roomJson.getString("hostUserId"),
                status = roomJson.getString("status"),
                media = data.getJSONObject("media").toRoomMedia(),
                members = data.optJSONArray("members").toRoomMembers()
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
        mediaUrl = AppConfig.playableMediaUrl(getString("mediaUrl")),
        durationMs = optNullableLong("durationMs"),
        seasonLabel = optNullableString("seasonLabel"),
        episodeLabel = optNullableString("episodeLabel")
    )
}

private fun org.json.JSONArray?.toRoomMembers(): List<RoomMember> {
    if (this == null) return emptyList()
    return buildList {
        for (index in 0 until length()) {
            val member = getJSONObject(index)
            add(
                RoomMember(
                    userId = member.getString("userId"),
                    nickname = member.optString("nickname"),
                    avatarSeed = member.optString("avatarSeed"),
                    avatarUrl = member.optNullableString("avatarUrl"),
                    role = member.getString("role")
                )
            )
        }
    }
}

private fun JSONObject.optNullableString(name: String): String? {
    if (!has(name) || isNull(name)) return null
    return optString(name).takeUnless { it.isBlank() || it == "null" }
}

private fun JSONObject.optNullableLong(name: String): Long? {
    if (!has(name) || isNull(name)) return null
    return optLong(name)
}
