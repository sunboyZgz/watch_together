package com.example.watch_together.sync

import com.example.watch_together.sync.protocol.ErrorPayload
import com.example.watch_together.sync.protocol.HeartbeatPayload
import com.example.watch_together.sync.protocol.EndedPayload
import com.example.watch_together.sync.protocol.PausePayload
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.ProtocolEventType
import com.example.watch_together.sync.protocol.RoomMembersChangedPayload
import com.example.watch_together.sync.protocol.RoomStatePayload
import com.example.watch_together.sync.protocol.SetPlaybackRatePayload
import com.example.watch_together.sync.protocol.SeekPayload
import org.json.JSONObject

sealed interface SyncMessage {
    data class RoomState(val payload: RoomStatePayload) : SyncMessage
    data class RoomMembersChanged(val payload: RoomMembersChangedPayload) : SyncMessage
    data class Play(val payload: PlayPayload) : SyncMessage
    data class Pause(val payload: PausePayload) : SyncMessage
    data class Seek(val payload: SeekPayload) : SyncMessage
    data class SetPlaybackRate(val payload: SetPlaybackRatePayload) : SyncMessage
    data class Ended(val payload: EndedPayload) : SyncMessage
    data class Heartbeat(val payload: HeartbeatPayload) : SyncMessage
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
                        ended = payload.getBoolean("ended"),
                        positionMs = payload.getLong("positionMs"),
                        playbackRate = payload.getDouble("playbackRate"),
                        seq = payload.getLong("seq")
                    )
                )
            }

            ProtocolEventType.RoomMembersChanged.wireName -> {
                SyncMessage.RoomMembersChanged(
                    RoomMembersChangedPayload(
                        roomId = payload.getString("roomId"),
                        reason = payload.optString("reason")
                    )
                )
            }

            ProtocolEventType.Play.wireName -> {
                SyncMessage.Play(
                    PlayPayload(
                        roomId = payload.getString("roomId"),
                        userId = payload.getString("userId"),
                        positionMs = payload.getLong("positionMs"),
                        seq = payload.getLong("seq")
                    )
                )
            }

            ProtocolEventType.Pause.wireName -> {
                SyncMessage.Pause(
                    PausePayload(
                        roomId = payload.getString("roomId"),
                        userId = payload.getString("userId"),
                        positionMs = payload.getLong("positionMs"),
                        seq = payload.getLong("seq")
                    )
                )
            }

            ProtocolEventType.Seek.wireName -> {
                SyncMessage.Seek(
                    SeekPayload(
                        roomId = payload.getString("roomId"),
                        userId = payload.getString("userId"),
                        positionMs = payload.getLong("positionMs"),
                        seq = payload.getLong("seq")
                    )
                )
            }

            ProtocolEventType.SetPlaybackRate.wireName -> {
                SyncMessage.SetPlaybackRate(
                    SetPlaybackRatePayload(
                        roomId = payload.getString("roomId"),
                        userId = payload.getString("userId"),
                        positionMs = payload.getLong("positionMs"),
                        playbackRate = payload.getDouble("playbackRate"),
                        seq = payload.getLong("seq")
                    )
                )
            }

            ProtocolEventType.Ended.wireName -> {
                SyncMessage.Ended(
                    EndedPayload(
                        roomId = payload.getString("roomId"),
                        userId = payload.getString("userId"),
                        positionMs = payload.getLong("positionMs"),
                        seq = payload.getLong("seq")
                    )
                )
            }

            ProtocolEventType.Heartbeat.wireName -> {
                SyncMessage.Heartbeat(
                    HeartbeatPayload(
                        serverTimeMs = payload.getLong("serverTimeMs")
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
