package com.example.watch_together.sync.protocol

data class JoinRoomPayload(
    val roomId: String,
    val userId: String
) : ProtocolPayload

data class RoomStatePayload(
    val roomId: String,
    val mediaId: String,
    val hostUserId: String,
    val paused: Boolean,
    val ended: Boolean,
    val positionMs: Long,
    val playbackRate: Double,
    val seq: Long
) : ProtocolPayload

data class RoomMembersChangedPayload(
    val roomId: String,
    val reason: String
) : ProtocolPayload

data class PlayPayload(
    val roomId: String,
    val userId: String,
    val positionMs: Long,
    val seq: Long
) : ProtocolPayload

data class PausePayload(
    val roomId: String,
    val userId: String,
    val positionMs: Long,
    val seq: Long
) : ProtocolPayload

data class SeekPayload(
    val roomId: String,
    val userId: String,
    val positionMs: Long,
    val seq: Long
) : ProtocolPayload

data class SetPlaybackRatePayload(
    val roomId: String,
    val userId: String,
    val positionMs: Long,
    val playbackRate: Double,
    val seq: Long
) : ProtocolPayload

data class EndedPayload(
    val roomId: String,
    val userId: String,
    val positionMs: Long,
    val seq: Long
) : ProtocolPayload

data class HeartbeatPayload(
    val serverTimeMs: Long
) : ProtocolPayload

data class HeartbeatAckPayload(
    val serverTimeMs: Long,
    val clientTimeMs: Long
) : ProtocolPayload

data class ErrorPayload(
    val roomId: String,
    val message: String
) : ProtocolPayload

fun JoinRoomPayload.toEnvelope(): ProtocolEnvelope<JoinRoomPayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.JoinRoom.wireName,
        payload = this
    )
}

fun RoomStatePayload.toEnvelope(): ProtocolEnvelope<RoomStatePayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.RoomState.wireName,
        payload = this
    )
}

fun PlayPayload.toEnvelope(): ProtocolEnvelope<PlayPayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.Play.wireName,
        payload = this
    )
}

fun PausePayload.toEnvelope(): ProtocolEnvelope<PausePayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.Pause.wireName,
        payload = this
    )
}

fun SeekPayload.toEnvelope(): ProtocolEnvelope<SeekPayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.Seek.wireName,
        payload = this
    )
}

fun SetPlaybackRatePayload.toEnvelope(): ProtocolEnvelope<SetPlaybackRatePayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.SetPlaybackRate.wireName,
        payload = this
    )
}

fun EndedPayload.toEnvelope(): ProtocolEnvelope<EndedPayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.Ended.wireName,
        payload = this
    )
}

fun HeartbeatPayload.toEnvelope(): ProtocolEnvelope<HeartbeatPayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.Heartbeat.wireName,
        payload = this
    )
}

fun HeartbeatAckPayload.toEnvelope(): ProtocolEnvelope<HeartbeatAckPayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.HeartbeatAck.wireName,
        payload = this
    )
}

fun ErrorPayload.toEnvelope(): ProtocolEnvelope<ErrorPayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.Error.wireName,
        payload = this
    )
}
