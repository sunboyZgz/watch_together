package com.example.watch_together.sync.protocol

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ProtocolDraftTest {

    @Test
    fun `event types stay aligned with INT-19 wire names`() {
        assertEquals("join_room", ProtocolEventType.JoinRoom.wireName)
        assertEquals("room_state", ProtocolEventType.RoomState.wireName)
        assertEquals("play", ProtocolEventType.Play.wireName)
        assertEquals("pause", ProtocolEventType.Pause.wireName)
        assertEquals("seek", ProtocolEventType.Seek.wireName)
        assertEquals("error", ProtocolEventType.Error.wireName)
    }

    @Test
    fun `direction rules stay aligned with protocol draft`() {
        assertEquals(ProtocolDirection.ClientToServer, ProtocolEventType.JoinRoom.direction)
        assertEquals(ProtocolDirection.ServerToClient, ProtocolEventType.RoomState.direction)
        assertEquals(
            ProtocolDirection.ClientToServerAndServerToClients,
            ProtocolEventType.Play.direction
        )
        assertEquals(
            ProtocolDirection.ClientToServerAndServerToClients,
            ProtocolEventType.Pause.direction
        )
        assertEquals(
            ProtocolDirection.ClientToServerAndServerToClients,
            ProtocolEventType.Seek.direction
        )
        assertEquals(ProtocolDirection.ServerToClient, ProtocolEventType.Error.direction)
    }

    @Test
    fun `join room payload wraps with shared envelope`() {
        val envelope = JoinRoomPayload(
            roomId = "room_001",
            userId = "user_a"
        ).toEnvelope()

        assertEquals("join_room", envelope.type)
        assertEquals("room_001", envelope.payload.roomId)
        assertEquals("user_a", envelope.payload.userId)
    }

    @Test
    fun `room state payload uses paused position and seq semantics`() {
        val payload = RoomStatePayload(
            roomId = "room_001",
            mediaId = "sample_001",
            hostUserId = "user_a",
            paused = false,
            positionMs = 125_000L,
            playbackRate = 1.0,
            seq = 3L
        )

        assertFalse(payload.paused)
        assertEquals(125_000L, payload.positionMs)
        assertEquals(1.0, payload.playbackRate, 0.0)
        assertEquals(3L, payload.seq)
    }

    @Test
    fun `encoding decision stays json text first`() {
        assertEquals("json", ProtocolSemantics.messageEncoding)
        assertEquals("utf8_text", ProtocolSemantics.frameType)
        assertEquals("milliseconds", ProtocolSemantics.positionUnit)
        assertEquals("server", ProtocolSemantics.sequenceAuthority)
    }

    @Test
    fun `control events keep minimal shared payload shape`() {
        val playEnvelope = PlayPayload(
            roomId = "room_001",
            userId = "user_a",
            positionMs = 125_000L,
            seq = 4L
        ).toEnvelope()

        val pauseEnvelope = PausePayload(
            roomId = "room_001",
            userId = "user_a",
            positionMs = 130_500L,
            seq = 5L
        ).toEnvelope()

        val seekEnvelope = SeekPayload(
            roomId = "room_001",
            userId = "user_a",
            positionMs = 210_000L,
            seq = 6L
        ).toEnvelope()

        assertEquals("play", playEnvelope.type)
        assertEquals("pause", pauseEnvelope.type)
        assertEquals("seek", seekEnvelope.type)
        assertTrue(seekEnvelope.payload.positionMs > pauseEnvelope.payload.positionMs)
        assertTrue(pauseEnvelope.payload.seq > playEnvelope.payload.seq)
    }
}
