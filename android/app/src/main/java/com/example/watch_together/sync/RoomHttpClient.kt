package com.example.watch_together.sync

import com.example.watch_together.config.AppConfig
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

data class CreateRoomResult(
    val roomId: String,
    val roomState: RoomSyncState
)

class RoomHttpClient(
    private val okHttpClient: OkHttpClient = OkHttpClient()
) {

    // createRoom uses the existing POST /rooms server entrypoint to get a fresh roomId
    // plus the authoritative initial room_state.
    fun createRoom(userId: String, mediaId: String): CreateRoomResult {
        val requestBody = JSONObject()
            .put("userId", userId)
            .put("mediaId", mediaId)
            .toString()
            .toRequestBody("application/json; charset=utf-8".toMediaType())

        val request = Request.Builder()
            .url(AppConfig.roomsUrl())
            .post(requestBody)
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            check(response.isSuccessful) {
                "Create room failed with ${response.code}"
            }
            val responseBody = checkNotNull(response.body?.string()) {
                "Create room returned an empty body"
            }
            val json = JSONObject(responseBody)
            val stateJson = json.getJSONObject("roomState")

            return CreateRoomResult(
                roomId = json.getString("roomId"),
                roomState = RoomSyncState(
                    roomId = stateJson.getString("roomId"),
                    mediaId = stateJson.getString("mediaId"),
                    hostUserId = stateJson.getString("hostUserId"),
                    paused = stateJson.getBoolean("paused"),
                    ended = stateJson.optBoolean("ended", false),
                    positionMs = stateJson.getLong("positionMs"),
                    playbackRate = stateJson.getDouble("playbackRate"),
                    seq = stateJson.getLong("seq")
                )
            )
        }
    }
}
