package com.example.watch_together.sync

import com.example.watch_together.config.AppConfig

class RoomSessionController(
    private val roomHttpClient: RoomHttpClient = RoomHttpClient(),
    private val roomWebSocketClient: RoomWebSocketClient = RoomWebSocketClient()
) {

    fun closeSession() {
        roomWebSocketClient.close()
    }

    suspend fun createRoom(accessToken: String, episodeId: String): CreateRoomResult {
        return roomHttpClient.createRoom(accessToken = accessToken, episodeId = episodeId)
    }

    suspend fun getRoomDetail(roomCode: String): RoomDetailResult {
        return roomHttpClient.getRoomDetail(roomCode = roomCode)
    }

    fun startSession(
        roomId: String,
        userId: String,
        listener: RoomWebSocketListener
    ) {
        roomWebSocketClient.joinRoom(
            wsUrl = AppConfig.wsBaseUrl,
            roomId = roomId,
            userId = userId,
            listener = listener
        )
    }

    fun sendPlay(positionMs: Long, seq: Long): Boolean {
        return roomWebSocketClient.sendPlay(positionMs = positionMs, seq = seq)
    }

    fun sendPause(positionMs: Long, seq: Long): Boolean {
        return roomWebSocketClient.sendPause(positionMs = positionMs, seq = seq)
    }

    fun sendSeek(positionMs: Long, seq: Long): Boolean {
        return roomWebSocketClient.sendSeek(positionMs = positionMs, seq = seq)
    }

    fun sendPlaybackRate(playbackRate: Double, positionMs: Long, seq: Long): Boolean {
        return roomWebSocketClient.sendPlaybackRate(
            playbackRate = playbackRate,
            positionMs = positionMs,
            seq = seq
        )
    }

    fun sendEnded(positionMs: Long, seq: Long): Boolean {
        return roomWebSocketClient.sendEnded(positionMs = positionMs, seq = seq)
    }
}
