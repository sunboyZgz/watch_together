package com.example.watch_together.sync

import com.example.watch_together.sync.protocol.RoomStatePayload

data class RoomSyncState(
    val roomId: String,
    val mediaId: String,
    val hostUserId: String,
    val paused: Boolean,
    val ended: Boolean,
    val positionMs: Long,
    val playbackRate: Double,
    val seq: Long,
    val authorityAppliedAtMs: Long = 0L
)

fun RoomSyncState.isNewerThan(previous: RoomSyncState?): Boolean {
    return previous == null || seq > previous.seq
}

fun RoomStatePayload.toRoomSyncState(appliedAtMs: Long = System.currentTimeMillis()): RoomSyncState {
    return RoomSyncState(
        roomId = roomId,
        mediaId = mediaId,
        hostUserId = hostUserId,
        paused = paused,
        ended = ended,
        positionMs = positionMs,
        playbackRate = playbackRate,
        seq = seq,
        authorityAppliedAtMs = appliedAtMs
    )
}
