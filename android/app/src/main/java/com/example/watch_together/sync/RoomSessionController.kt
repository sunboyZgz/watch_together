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

    suspend fun getRoomDetail(accessToken: String, roomCode: String): RoomDetailResult {
        return roomHttpClient.getRoomDetail(accessToken = accessToken, roomCode = roomCode)
    }

    suspend fun joinRoomByCode(accessToken: String, roomCode: String): JoinRoomResult {
        return roomHttpClient.joinRoomByCode(accessToken = accessToken, roomCode = roomCode)
    }

    fun startSession(
        roomId: String,
        userId: String,
        accessToken: String,
        listener: RoomWebSocketListener
    ) {
        roomWebSocketClient.joinRoom(
            wsUrl = AppConfig.wsBaseUrl,
            roomId = roomId,
            userId = userId,
            accessToken = accessToken,
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

    fun sendRoomDeviceSwitchReply(requestId: String, approve: Boolean): Boolean {
        return roomWebSocketClient.sendRoomDeviceSwitchReply(requestId = requestId, approve = approve)
    }

    fun diagnostics(): String {
        return roomWebSocketClient.diagnostics()
    }
}
