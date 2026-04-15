package com.example.watch_together.sync

import com.example.watch_together.sync.protocol.ErrorPayload
import com.example.watch_together.sync.protocol.ProtocolEventType
import com.example.watch_together.sync.protocol.RoomStatePayload
import org.json.JSONObject

sealed interface SyncMessage {
    data class RoomState(val payload: RoomStatePayload) : SyncMessage
    data class Error(val payload: ErrorPayload) : SyncMessage
}

class SyncMessageDecoder {

    // decode converts the shared type + payload envelope into the minimum messages
    // the Android sync flow currently needs.
    fun decode(rawMessage: String): SyncMessage {
        val envelope = JSONObject(rawMessage)
        val type = envelope.getString("type")
        val payload = envelope.getJSONObject("payload")

        return when (type) {
            ProtocolEventType.RoomState.wireName -> {
                SyncMessage.RoomState(
                    RoomStatePayload(
                        roomId = payload.getString("roomId"),
                        mediaId = payload.getString("mediaId"),
                        hostUserId = payload.getString("hostUserId"),
                        paused = payload.getBoolean("paused"),
                        positionMs = payload.getLong("positionMs"),
                        playbackRate = payload.getDouble("playbackRate"),
                        seq = payload.getLong("seq")
                    )
                )
            }

            ProtocolEventType.Error.wireName -> {
                SyncMessage.Error(
                    ErrorPayload(
                        roomId = payload.optString("roomId"),
                        message = payload.getString("message")
                    )
                )
            }

            else -> error("Unsupported sync message type: $type")
        }
    }
}
