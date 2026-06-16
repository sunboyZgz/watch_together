package com.example.watch_together.sync

import com.example.watch_together.sync.protocol.EndedPayload
import com.example.watch_together.sync.protocol.PausePayload
import com.example.watch_together.sync.protocol.PlayPayload
import com.example.watch_together.sync.protocol.RoomDeviceSwitchRequestPayload
import com.example.watch_together.sync.protocol.RoomDeviceSwitchResultPayload
import com.example.watch_together.sync.protocol.RoomMembersChangedPayload
import com.example.watch_together.sync.protocol.RoomPresencePayload
import com.example.watch_together.sync.protocol.SeekPayload
import com.example.watch_together.sync.protocol.SetPlaybackRatePayload
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.assertFalse
import org.junit.Test

class RoomWebSocketClientReconnectTest {
    @Test
    fun `server drain close schedules reconnect and resends join with same device`() {
        val factory = FakeWebSocketFactory()
        val scheduler = FakeReconnectScheduler()
        val listener = RecordingRoomWebSocketListener()
        val client = RoomWebSocketClient(
            reconnectScheduler = scheduler,
            reconnectInitialDelayMs = 500,
            reconnectMaxDelayMs = 8000,
            reconnectJitterFraction = { 0.0 },
            webSocketFactory = factory
        )

        client.joinRoom(
            wsUrl = "ws://example.test/ws",
            roomId = "ROOM01",
            userId = "user-1",
            deviceId = "device-stable",
            accessToken = "token-1",
            listener = listener
        )
        factory.open(0)
        assertJoinPayload(factory.sockets[0], "ROOM01", "user-1", "device-stable")

        factory.listeners[0].onClosed(factory.sockets[0], 1012, "server draining")

        assertEquals(1, scheduler.tasks.size)
        assertEquals(500, scheduler.tasks[0].delayMs)
        scheduler.tasks[0].run()
        factory.open(1)

        assertEquals(2, factory.sockets.size)
        assertJoinPayload(factory.sockets[1], "ROOM01", "user-1", "device-stable")
        assertTrue(listener.logs.any { it.contains("reconnect scheduled") })
    }

    @Test
    fun `manual close does not schedule reconnect`() {
        val factory = FakeWebSocketFactory()
        val scheduler = FakeReconnectScheduler()
        val client = RoomWebSocketClient(
            reconnectScheduler = scheduler,
            reconnectInitialDelayMs = 500,
            reconnectMaxDelayMs = 8000,
            reconnectJitterFraction = { 0.0 },
            webSocketFactory = factory
        )

        client.joinRoom("ws://example.test/ws", "ROOM01", "user-1", "device-stable", "token-1", RecordingRoomWebSocketListener())
        factory.open(0)
        client.close()
        factory.listeners[0].onClosed(factory.sockets[0], 1000, "client closed")

        assertTrue(factory.sockets[0].sentMessages.any { it.contains("\"leave_room\"") })
        assertTrue(scheduler.tasks.isEmpty())
    }

    @Test
    fun `unauthorized websocket failure does not reconnect`() {
        val factory = FakeWebSocketFactory()
        val scheduler = FakeReconnectScheduler()
        val listener = RecordingRoomWebSocketListener()
        val client = RoomWebSocketClient(
            reconnectScheduler = scheduler,
            reconnectInitialDelayMs = 500,
            reconnectMaxDelayMs = 8000,
            reconnectJitterFraction = { 0.0 },
            webSocketFactory = factory
        )

        client.joinRoom("ws://example.test/ws", "ROOM01", "user-1", "device-stable", "token-1", listener)
        factory.listeners[0].onFailure(
            factory.sockets[0],
            RuntimeException("unauthorized"),
            testWebSocketResponse(401)
        )

        assertTrue(scheduler.tasks.isEmpty())
        assertTrue(listener.errors.contains("unauthorized"))
    }

    private fun assertJoinPayload(socket: FakeWebSocket, roomId: String, userId: String, deviceId: String) {
        val raw = socket.sentMessages.lastOrNull { it.contains("\"join_room\"") }
        assertTrue("expected join_room message in ${socket.sentMessages}", raw != null)
        val rawMessage = raw ?: error("join_room message missing")
        val payload = JSONObject(rawMessage).getJSONObject("payload")
        assertEquals(roomId, payload.getString("roomId"))
        assertEquals(userId, payload.getString("userId"))
        assertEquals(deviceId, payload.getString("deviceId"))
    }

    private class FakeWebSocketFactory : RoomWebSocketFactory {
        val listeners = mutableListOf<WebSocketListener>()
        val sockets = mutableListOf<FakeWebSocket>()

        override fun newWebSocket(request: Request, listener: WebSocketListener): WebSocket {
            val socket = FakeWebSocket(request)
            sockets += socket
            listeners += listener
            return socket
        }

        fun open(index: Int) {
            listeners[index].onOpen(sockets[index], testWebSocketResponse(101))
        }
    }

    private class FakeReconnectScheduler : ReconnectScheduler {
        val tasks = mutableListOf<ScheduledTask>()

        override fun schedule(delayMs: Long, task: () -> Unit): ReconnectTask {
            val scheduled = ScheduledTask(delayMs, task)
            tasks += scheduled
            return scheduled
        }
    }

    private class ScheduledTask(
        val delayMs: Long,
        private val task: () -> Unit
    ) : ReconnectTask {
        var canceled: Boolean = false

        override fun cancel() {
            canceled = true
        }

        fun run() {
            assertFalse(canceled)
            task()
        }
    }

    private class FakeWebSocket(private val request: Request) : WebSocket {
        val sentMessages = mutableListOf<String>()
        val closeReasons = mutableListOf<String?>()

        override fun request(): Request = request
        override fun queueSize(): Long = 0

        override fun send(text: String): Boolean {
            sentMessages += text
            return true
        }

        override fun send(bytes: ByteString): Boolean = true

        override fun close(code: Int, reason: String?): Boolean {
            closeReasons += reason
            return true
        }

        override fun cancel() = Unit
    }

    private class RecordingRoomWebSocketListener : RoomWebSocketListener {
        val logs = mutableListOf<String>()
        val errors = mutableListOf<String>()

        override fun onLog(message: String) {
            logs += message
        }

        override fun onRoomState(payload: RoomSyncState) = Unit
        override fun onRoomMembersChanged(payload: RoomMembersChangedPayload) = Unit
        override fun onRoomPresence(payload: RoomPresencePayload) = Unit
        override fun onRoomDeviceWaiting(payload: RoomDeviceSwitchRequestPayload) = Unit
        override fun onRoomDeviceSwitchRequest(payload: RoomDeviceSwitchRequestPayload) = Unit
        override fun onRoomDeviceSwitchResult(payload: RoomDeviceSwitchResultPayload) = Unit
        override fun onPlay(payload: PlayPayload) = Unit
        override fun onPause(payload: PausePayload) = Unit
        override fun onSeek(payload: SeekPayload) = Unit
        override fun onPlaybackRate(payload: SetPlaybackRatePayload) = Unit
        override fun onEnded(payload: EndedPayload) = Unit
        override fun onHeartbeat(serverTimeMs: Long) = Unit

        override fun onError(message: String) {
            errors += message
        }
    }
}

private fun testWebSocketResponse(code: Int): Response {
    return Response.Builder()
        .request(Request.Builder().url("ws://example.test/ws").build())
        .protocol(Protocol.HTTP_1_1)
        .code(code)
        .message("test")
        .build()
}
