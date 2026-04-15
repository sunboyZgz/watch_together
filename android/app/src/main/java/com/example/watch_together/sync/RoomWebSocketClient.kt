package com.example.watch_together.sync

import com.example.watch_together.sync.protocol.JoinRoomPayload
import com.example.watch_together.sync.protocol.toEnvelope
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject

interface RoomWebSocketListener {
    fun onLog(message: String)
    fun onRoomState(payload: RoomSyncState)
    fun onError(message: String)
}

class RoomWebSocketClient(
    private val okHttpClient: OkHttpClient = OkHttpClient(),
    private val decoder: SyncMessageDecoder = SyncMessageDecoder()
) {
    private var webSocket: WebSocket? = null

    // joinRoom opens the shared /ws endpoint, sends join_room, and forwards the
    // first protocol messages back to the UI layer.
    fun joinRoom(wsUrl: String, roomId: String, userId: String, listener: RoomWebSocketListener) {
        close()

        val request = Request.Builder()
            .url(wsUrl)
            .build()

        webSocket = okHttpClient.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                listener.onLog("WebSocket connected to $wsUrl")

                val joinPayload = JoinRoomPayload(
                    roomId = roomId,
                    userId = userId
                )
                val envelope = joinPayload.toEnvelope()
                val rawMessage = JSONObject()
                    .put("type", envelope.type)
                    .put(
                        "payload",
                        JSONObject()
                            .put("roomId", joinPayload.roomId)
                            .put("userId", joinPayload.userId)
                    )
                    .toString()

                webSocket.send(rawMessage)
                listener.onLog("join_room sent for roomId=$roomId userId=$userId")
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                when (val message = decoder.decode(text)) {
                    is SyncMessage.RoomState -> listener.onRoomState(message.payload.toRoomSyncState())
                    is SyncMessage.Error -> listener.onError(message.payload.message)
                }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                listener.onError(t.message ?: "Unknown WebSocket failure")
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                listener.onLog("WebSocket closing: $code / $reason")
            }
        })
    }

    fun close() {
        webSocket?.close(1000, "client closed")
        webSocket = null
    }
}
