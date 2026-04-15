package com.example.watch_together.sync.protocol

object ProtocolSemantics {
    const val messageEncoding = "json"
    const val frameType = "utf8_text"

    const val positionUnit = "milliseconds"
    const val sequenceAuthority = "server"

    const val joinRoomFieldRoomId = "roomId"
    const val joinRoomFieldUserId = "userId"

    const val stateFieldMediaId = "mediaId"
    const val stateFieldHostUserId = "hostUserId"
    const val stateFieldPaused = "paused"
    const val stateFieldPositionMs = "positionMs"
    const val stateFieldPlaybackRate = "playbackRate"
    const val stateFieldSeq = "seq"
    const val errorFieldMessage = "message"
}
