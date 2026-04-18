package com.example.watch_together.sync

import com.example.watch_together.sync.protocol.JoinRoomPayload
import com.example.watch_together.sync.protocol.HeartbeatAckPayload
import com.example.watch_together.sync.protocol.PausePayload
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.SeekPayload
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
    fun onPlay(payload: PlayPayload)
    fun onPause(payload: PausePayload)
    fun onSeek(payload: SeekPayload)
    fun onHeartbeat(serverTimeMs: Long)
    fun onError(message: String)
}

class RoomWebSocketClient(
    private val okHttpClient: OkHttpClient = OkHttpClient(),
    private val decoder: SyncMessageDecoder = SyncMessageDecoder()
) {
    private var webSocket: WebSocket? = null
    private var activeRoomId: String? = null
    private var activeUserId: String? = null

    // joinRoom opens the shared /ws endpoint, sends join_room, and forwards the
    // first protocol messages back to the UI layer.
    fun joinRoom(wsUrl: String, roomId: String, userId: String, listener: RoomWebSocketListener) {
        close()
        activeRoomId = roomId
        activeUserId = userId

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
                try {
                    when (val message = decoder.decode(text)) {
                        is SyncMessage.RoomState -> listener.onRoomState(message.payload.toRoomSyncState())
                        is SyncMessage.Play -> listener.onPlay(message.payload)
                        is SyncMessage.Pause -> listener.onPause(message.payload)
                        is SyncMessage.Seek -> listener.onSeek(message.payload)
                        is SyncMessage.Heartbeat -> {
                            sendHeartbeatAck(message.payload.serverTimeMs)
                            listener.onHeartbeat(message.payload.serverTimeMs)
                        }
                        is SyncMessage.Error -> listener.onError(message.payload.message)
                    }
                } catch (error: Throwable) {
                    listener.onError(error.message ?: "Failed to handle WebSocket message")
                }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                listener.onError(t.message ?: "Unknown WebSocket failure")
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                listener.onLog("WebSocket closing: $code / $reason")
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                listener.onLog("WebSocket closed: $code / $reason")
            }
        })
    }

    fun sendPlay(positionMs: Long, seq: Long): Boolean {
        return sendControl(
            PlayPayload(
                roomId = activeRoomId ?: return false,
                userId = activeUserId ?: return false,
                positionMs = positionMs,
                seq = seq
            ).toEnvelope()
        )
    }

    fun sendPause(positionMs: Long, seq: Long): Boolean {
        return sendControl(
            PausePayload(
                roomId = activeRoomId ?: return false,
                userId = activeUserId ?: return false,
                positionMs = positionMs,
                seq = seq
            ).toEnvelope()
        )
    }

    fun sendSeek(positionMs: Long, seq: Long): Boolean {
        return sendControl(
            SeekPayload(
                roomId = activeRoomId ?: return false,
                userId = activeUserId ?: return false,
                positionMs = positionMs,
                seq = seq
            ).toEnvelope()
        )
    }

    fun close() {
        webSocket?.close(1000, "client closed")
        webSocket = null
        activeRoomId = null
        activeUserId = null
    }

    private fun sendControl(envelope: Any): Boolean {
        val webSocket = webSocket ?: return false
        val rawMessage = envelopeToJson(envelope)
        return webSocket.send(rawMessage)
    }

    private fun envelopeToJson(envelope: Any): String {
        return when (envelope) {
            is com.example.watch_together.sync.protocol.ProtocolEnvelope<*> -> {
                JSONObject()
                    .put("type", envelope.type)
                    .put("payload", JSONObject(envelope.payloadAsMap()))
                    .toString()
            }

            else -> error("Unsupported envelope type: ${envelope::class.java.simpleName}")
        }
    }

    private fun com.example.watch_together.sync.protocol.ProtocolEnvelope<*>.payloadAsMap(): Map<String, Any> {
        return when (val payload = payload) {
            is JoinRoomPayload -> mapOf(
                "roomId" to payload.roomId,
                "userId" to payload.userId
            )

            is PlayPayload -> mapOf(
                "roomId" to payload.roomId,
                "userId" to payload.userId,
                "positionMs" to payload.positionMs,
                "seq" to payload.seq
            )

            is PausePayload -> mapOf(
                "roomId" to payload.roomId,
                "userId" to payload.userId,
                "positionMs" to payload.positionMs,
                "seq" to payload.seq
            )

            is SeekPayload -> mapOf(
                "roomId" to payload.roomId,
                "userId" to payload.userId,
                "positionMs" to payload.positionMs,
                "seq" to payload.seq
            )

            is HeartbeatAckPayload -> mapOf(
                "serverTimeMs" to payload.serverTimeMs,
                "clientTimeMs" to payload.clientTimeMs
            )

            else -> error("Unsupported payload type: ${payload::class.java.simpleName}")
        }
    }

    private fun sendHeartbeatAck(serverTimeMs: Long) {
        val webSocket = webSocket ?: return
        val sent = webSocket.send(
            envelopeToJson(
                HeartbeatAckPayload(
                    serverTimeMs = serverTimeMs,
                    clientTimeMs = System.currentTimeMillis()
                ).toEnvelope()
            )
        )

        if (!sent) {
            throw IllegalStateException("Failed to send heartbeat_ack")
        }
    }
}
