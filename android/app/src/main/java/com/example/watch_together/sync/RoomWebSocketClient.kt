package com.example.watch_together.sync

import com.example.watch_together.config.AppConfig
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
import java.util.concurrent.Executors
import java.util.concurrent.ThreadLocalRandom
import java.util.concurrent.TimeUnit

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

fun interface RoomWebSocketFactory {
    fun newWebSocket(request: Request, listener: WebSocketListener): WebSocket
}

interface ReconnectTask {
    fun cancel()
}

fun interface ReconnectScheduler {
    fun schedule(delayMs: Long, task: () -> Unit): ReconnectTask
}

private class ExecutorReconnectScheduler : ReconnectScheduler {
    private val executor = Executors.newSingleThreadScheduledExecutor { runnable ->
        Thread(runnable, "room-ws-reconnect").apply { isDaemon = true }
    }

    override fun schedule(delayMs: Long, task: () -> Unit): ReconnectTask {
        val future = executor.schedule(task, delayMs.coerceAtLeast(0L), TimeUnit.MILLISECONDS)
        return object : ReconnectTask {
            override fun cancel() {
                future.cancel(false)
            }
        }
    }
}

class RoomWebSocketClient(
    private val okHttpClient: OkHttpClient = OkHttpClient(),
    private val decoder: SyncMessageDecoder = SyncMessageDecoder(),
    private val reconnectScheduler: ReconnectScheduler = ExecutorReconnectScheduler(),
    private val reconnectInitialDelayMs: Long = AppConfig.wsReconnectInitialDelayMs,
    private val reconnectMaxDelayMs: Long = AppConfig.wsReconnectMaxDelayMs,
    private val reconnectJitterFraction: () -> Double = {
        ThreadLocalRandom.current().nextDouble(0.0, 0.25)
    },
    private val webSocketFactory: RoomWebSocketFactory = RoomWebSocketFactory { request, listener ->
        okHttpClient.newWebSocket(request, listener)
    }
) {
    private data class ActiveSession(
        val wsUrl: String,
        val roomId: String,
        val userId: String,
        val deviceId: String,
        val accessToken: String,
        val listener: RoomWebSocketListener
    )

    private var webSocket: WebSocket? = null
    private var activeRoomId: String? = null
    private var activeUserId: String? = null
    private var activeSession: ActiveSession? = null
    private var sessionGeneration: Long = 0L
    private var connectionState: RoomWebSocketConnectionState = RoomWebSocketConnectionState.Idle
    private var lastFailureMessage: String? = null
    private var manualClose: Boolean = false
    private var terminalBusinessError: Boolean = false
    private var reconnectAttempt: Int = 0
    private var pendingReconnect: ReconnectTask? = null

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
        val session = ActiveSession(
            wsUrl = wsUrl,
            roomId = roomId,
            userId = userId,
            deviceId = deviceId,
            accessToken = accessToken,
            listener = listener
        )
        activeRoomId = roomId
        activeUserId = userId
        activeSession = session
        manualClose = false
        terminalBusinessError = false
        reconnectAttempt = 0
        pendingReconnect?.cancel()
        pendingReconnect = null
        connectionState = RoomWebSocketConnectionState.Connecting
        lastFailureMessage = null
        openWebSocket(session, generation)
    }

    private fun openWebSocket(session: ActiveSession, generation: Long) {
        val request = Request.Builder()
            .url(session.wsUrl)
            .header("Authorization", "Bearer ${session.accessToken}")
            .build()

        connectionState = RoomWebSocketConnectionState.Connecting
        webSocket = webSocketFactory.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                if (!isCurrentSession(generation, webSocket)) return
                connectionState = RoomWebSocketConnectionState.Open
                session.listener.onLog("WebSocket connected to ${session.wsUrl}")

                val joinPayload = JoinRoomPayload(
                    roomId = session.roomId,
                    userId = session.userId,
                    deviceId = session.deviceId
                )
                webSocket.send(joinPayloadToJson(joinPayload))
                session.listener.onLog("join_room sent for roomId=${session.roomId} userId=${session.userId}")
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                if (!isCurrentSession(generation, webSocket)) return
                try {
                    when (val message = decoder.decode(text)) {
                        is SyncMessage.RoomState -> {
                            reconnectAttempt = 0
                            session.listener.onRoomState(message.payload.toRoomSyncState())
                        }
                        is SyncMessage.RoomMembersChanged -> session.listener.onRoomMembersChanged(message.payload)
                        is SyncMessage.RoomPresence -> session.listener.onRoomPresence(message.payload)
                        is SyncMessage.RoomDeviceWaiting -> session.listener.onRoomDeviceWaiting(message.payload)
                        is SyncMessage.RoomDeviceSwitchRequest -> session.listener.onRoomDeviceSwitchRequest(message.payload)
                        is SyncMessage.RoomDeviceSwitchResult -> session.listener.onRoomDeviceSwitchResult(message.payload)
                        is SyncMessage.Play -> session.listener.onPlay(message.payload)
                        is SyncMessage.Pause -> session.listener.onPause(message.payload)
                        is SyncMessage.Seek -> session.listener.onSeek(message.payload)
                        is SyncMessage.SetPlaybackRate -> session.listener.onPlaybackRate(message.payload)
                        is SyncMessage.Ended -> session.listener.onEnded(message.payload)
                        is SyncMessage.Heartbeat -> {
                            sendHeartbeatAck(webSocket, message.payload.serverTimeMs)
                            session.listener.onHeartbeat(message.payload.serverTimeMs)
                        }
                        is SyncMessage.Error -> {
                            val messageText = message.payload.message
                            if (isTerminalBusinessError(messageText)) {
                                terminalBusinessError = true
                            }
                            session.listener.onError(messageText)
                        }
                    }
                } catch (error: Throwable) {
                    session.listener.onError(error.message ?: "Failed to handle WebSocket message")
                }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                if (!isCurrentSession(generation, webSocket)) return
                connectionState = RoomWebSocketConnectionState.Failed
                lastFailureMessage = t.message ?: "Unknown WebSocket failure"
                this@RoomWebSocketClient.webSocket = null
                if (response?.code == 401) {
                    terminalBusinessError = true
                }
                session.listener.onError(lastFailureMessage.orEmpty())
                scheduleReconnect(generation, "failure")
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                if (!isCurrentSession(generation, webSocket)) return
                connectionState = RoomWebSocketConnectionState.Closing
                session.listener.onLog("WebSocket closing: $code / $reason")
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                if (!isCurrentSession(generation, webSocket)) return
                connectionState = RoomWebSocketConnectionState.Closed
                this@RoomWebSocketClient.webSocket = null
                session.listener.onLog("WebSocket closed: $code / $reason")
                scheduleReconnect(generation, "closed $code")
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
        manualClose = true
        pendingReconnect?.cancel()
        pendingReconnect = null
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
        activeSession = null
        terminalBusinessError = false
        reconnectAttempt = 0
        connectionState = RoomWebSocketConnectionState.Closed
    }

    fun diagnostics(): String {
        return "state=$connectionState roomId=${activeRoomId ?: "none"} userId=${activeUserId ?: "none"} " +
            "lastFailure=${lastFailureMessage ?: "none"} reconnectAttempt=$reconnectAttempt"
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

    private fun joinPayloadToJson(payload: JoinRoomPayload): String {
        val envelope = payload.toEnvelope()
        return JSONObject()
            .put("type", envelope.type)
            .put(
                "payload",
                JSONObject()
                    .put("roomId", payload.roomId)
                    .put("userId", payload.userId)
                    .put("deviceId", payload.deviceId)
            )
            .toString()
    }

    private fun scheduleReconnect(generation: Long, reason: String) {
        val session = activeSession ?: return
        if (manualClose || terminalBusinessError || generation != sessionGeneration) {
            return
        }
        pendingReconnect?.cancel()
        reconnectAttempt += 1
        val delayMs = reconnectDelayMs(reconnectAttempt)
        session.listener.onLog("WebSocket reconnect scheduled in ${delayMs}ms after $reason")
        pendingReconnect = reconnectScheduler.schedule(delayMs) {
            if (manualClose || terminalBusinessError || generation != sessionGeneration) {
                return@schedule
            }
            openWebSocket(session, generation)
        }
    }

    private fun reconnectDelayMs(attempt: Int): Long {
        val initial = reconnectInitialDelayMs.coerceAtLeast(1L)
        val max = reconnectMaxDelayMs.coerceAtLeast(initial)
        var base = initial
        repeat((attempt - 1).coerceAtLeast(0).coerceAtMost(16)) {
            base = (base * 2).coerceAtMost(max)
        }
        val jitter = (base * reconnectJitterFraction().coerceIn(0.0, 0.25)).toLong()
        return (base + jitter).coerceAtMost(max)
    }

    private fun isTerminalBusinessError(message: String): Boolean {
        return when (message.trim().lowercase()) {
            "room identity mismatch",
            "room membership required",
            "room membership is invalid",
            "room not found",
            "missing deviceid",
            "invalid deviceid" -> true
            else -> false
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
