package com.example.watch_together.sync

import com.example.watch_together.sync.protocol.JoinRoomPayload
import com.example.watch_together.sync.protocol.LeaveRoomPayload
import com.example.watch_together.sync.protocol.HeartbeatAckPayload
import com.example.watch_together.sync.protocol.EndedPayload
import com.example.watch_together.sync.protocol.PausePayload
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.RoomMembersChangedPayload
import com.example.watch_together.sync.protocol.RoomPresencePayload
import com.example.watch_together.sync.protocol.RoomDeviceSwitchReplyPayload
import com.example.watch_together.sync.protocol.RoomDeviceSwitchRequestPayload
import com.example.watch_together.sync.protocol.RoomDeviceSwitchResultPayload
import com.example.watch_together.sync.protocol.SeekPayload
import com.example.watch_together.sync.protocol.SetPlaybackRatePayload
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
    fun onRoomMembersChanged(payload: RoomMembersChangedPayload)
    fun onRoomPresence(payload: RoomPresencePayload)
    fun onRoomDeviceWaiting(payload: RoomDeviceSwitchRequestPayload)
    fun onRoomDeviceSwitchRequest(payload: RoomDeviceSwitchRequestPayload)
    fun onRoomDeviceSwitchResult(payload: RoomDeviceSwitchResultPayload)
    fun onPlay(payload: PlayPayload)
    fun onPause(payload: PausePayload)
    fun onSeek(payload: SeekPayload)
    fun onPlaybackRate(payload: SetPlaybackRatePayload)
    fun onEnded(payload: EndedPayload)
    fun onHeartbeat(serverTimeMs: Long)
    fun onError(message: String)
}

enum class RoomWebSocketConnectionState {
    Idle,
    Connecting,
    Open,
    Closing,
    Closed,
    Failed
}

class RoomWebSocketClient(
    private val okHttpClient: OkHttpClient = OkHttpClient(),
    private val decoder: SyncMessageDecoder = SyncMessageDecoder()
) {
    private var webSocket: WebSocket? = null
    private var activeRoomId: String? = null
    private var activeUserId: String? = null
    private var sessionGeneration: Long = 0L
    private var connectionState: RoomWebSocketConnectionState = RoomWebSocketConnectionState.Idle
    private var lastFailureMessage: String? = null

    // joinRoom opens the shared /ws endpoint, sends join_room, and forwards the
    // first protocol messages back to the UI layer.
    fun joinRoom(
        wsUrl: String,
        roomId: String,
        userId: String,
        deviceId: String,
        accessToken: String,
        listener: RoomWebSocketListener
    ) {
        close()
        val generation = ++sessionGeneration
        activeRoomId = roomId
        activeUserId = userId
        connectionState = RoomWebSocketConnectionState.Connecting
        lastFailureMessage = null

        val request = Request.Builder()
            .url(wsUrl)
            .header("Authorization", "Bearer $accessToken")
            .build()

        webSocket = okHttpClient.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                if (!isCurrentSession(generation, webSocket)) return
                connectionState = RoomWebSocketConnectionState.Open
                listener.onLog("WebSocket connected to $wsUrl")

                val joinPayload = JoinRoomPayload(
                    roomId = roomId,
                    userId = userId,
                    deviceId = deviceId
                )
                val envelope = joinPayload.toEnvelope()
                val rawMessage = JSONObject()
                    .put("type", envelope.type)
                    .put(
                        "payload",
                        JSONObject()
                            .put("roomId", joinPayload.roomId)
                            .put("userId", joinPayload.userId)
                            .put("deviceId", joinPayload.deviceId)
                    )
                    .toString()

                webSocket.send(rawMessage)
                listener.onLog("join_room sent for roomId=$roomId userId=$userId")
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                if (!isCurrentSession(generation, webSocket)) return
                try {
                    when (val message = decoder.decode(text)) {
                        is SyncMessage.RoomState -> listener.onRoomState(message.payload.toRoomSyncState())
                        is SyncMessage.RoomMembersChanged -> listener.onRoomMembersChanged(message.payload)
                        is SyncMessage.RoomPresence -> listener.onRoomPresence(message.payload)
                        is SyncMessage.RoomDeviceWaiting -> listener.onRoomDeviceWaiting(message.payload)
                        is SyncMessage.RoomDeviceSwitchRequest -> listener.onRoomDeviceSwitchRequest(message.payload)
                        is SyncMessage.RoomDeviceSwitchResult -> listener.onRoomDeviceSwitchResult(message.payload)
                        is SyncMessage.Play -> listener.onPlay(message.payload)
                        is SyncMessage.Pause -> listener.onPause(message.payload)
                        is SyncMessage.Seek -> listener.onSeek(message.payload)
                        is SyncMessage.SetPlaybackRate -> listener.onPlaybackRate(message.payload)
                        is SyncMessage.Ended -> listener.onEnded(message.payload)
                        is SyncMessage.Heartbeat -> {
                            sendHeartbeatAck(webSocket, message.payload.serverTimeMs)
                            listener.onHeartbeat(message.payload.serverTimeMs)
                        }
                        is SyncMessage.Error -> listener.onError(message.payload.message)
                    }
                } catch (error: Throwable) {
                    listener.onError(error.message ?: "Failed to handle WebSocket message")
                }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                if (!isCurrentSession(generation, webSocket)) return
                connectionState = RoomWebSocketConnectionState.Failed
                lastFailureMessage = t.message ?: "Unknown WebSocket failure"
                listener.onError(lastFailureMessage.orEmpty())
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                if (!isCurrentSession(generation, webSocket)) return
                connectionState = RoomWebSocketConnectionState.Closing
                listener.onLog("WebSocket closing: $code / $reason")
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                if (!isCurrentSession(generation, webSocket)) return
                connectionState = RoomWebSocketConnectionState.Closed
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

    fun sendPlaybackRate(playbackRate: Double, positionMs: Long, seq: Long): Boolean {
        return sendControl(
            SetPlaybackRatePayload(
                roomId = activeRoomId ?: return false,
                userId = activeUserId ?: return false,
                positionMs = positionMs,
                playbackRate = playbackRate,
                seq = seq
            ).toEnvelope()
        )
    }

    fun sendEnded(positionMs: Long, seq: Long): Boolean {
        return sendControl(
            EndedPayload(
                roomId = activeRoomId ?: return false,
                userId = activeUserId ?: return false,
                positionMs = positionMs,
                seq = seq
            ).toEnvelope()
        )
    }

    fun sendRoomDeviceSwitchReply(requestId: String, approve: Boolean): Boolean {
        return sendControl(
            RoomDeviceSwitchReplyPayload(
                roomId = activeRoomId ?: return false,
                userId = activeUserId ?: return false,
                requestId = requestId,
                approve = approve
            ).toEnvelope()
        )
    }

    fun close() {
        sessionGeneration++
        val roomId = activeRoomId
        val userId = activeUserId
        val socket = webSocket
        if (socket != null && !roomId.isNullOrBlank() && !userId.isNullOrBlank()) {
            socket.send(
                envelopeToJson(
                    LeaveRoomPayload(
                        roomId = roomId,
                        userId = userId
                    ).toEnvelope()
                )
            )
        }
        webSocket?.close(1000, "client closed")
        webSocket = null
        activeRoomId = null
        activeUserId = null
        connectionState = RoomWebSocketConnectionState.Closed
    }

    fun diagnostics(): String {
        return "state=$connectionState roomId=${activeRoomId ?: "none"} userId=${activeUserId ?: "none"} " +
            "lastFailure=${lastFailureMessage ?: "none"}"
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
                "userId" to payload.userId,
                "deviceId" to payload.deviceId
            )

            is LeaveRoomPayload -> mapOf(
                "roomId" to payload.roomId,
                "userId" to payload.userId
            )

            is RoomDeviceSwitchReplyPayload -> mapOf(
                "roomId" to payload.roomId,
                "userId" to payload.userId,
                "requestId" to payload.requestId,
                "approve" to payload.approve
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

            is SetPlaybackRatePayload -> mapOf(
                "roomId" to payload.roomId,
                "userId" to payload.userId,
                "positionMs" to payload.positionMs,
                "playbackRate" to payload.playbackRate,
                "seq" to payload.seq
            )

            is EndedPayload -> mapOf(
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

    private fun sendHeartbeatAck(webSocket: WebSocket, serverTimeMs: Long) {
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

    private fun isCurrentSession(generation: Long, candidate: WebSocket): Boolean {
        return generation == sessionGeneration && candidate == webSocket
    }
}
