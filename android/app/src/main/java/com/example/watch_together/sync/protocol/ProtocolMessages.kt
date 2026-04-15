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
    val positionMs: Long,
    val playbackRate: Double,
    val seq: Long
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

fun ErrorPayload.toEnvelope(): ProtocolEnvelope<ErrorPayload> {
    return ProtocolEnvelope(
        type = ProtocolEventType.Error.wireName,
        payload = this
    )
}
