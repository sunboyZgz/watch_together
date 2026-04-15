package com.example.watch_together.sync.protocol

import org.junit.Assert.assertEquals
import org.junit.Test

class ProtocolDraftTest {

    @Test
    fun `event metadata stays aligned with protocol draft`() {
        val expected = mapOf(
            ProtocolEventType.JoinRoom to ("join_room" to ProtocolDirection.ClientToServer),
            ProtocolEventType.RoomState to ("room_state" to ProtocolDirection.ServerToClient),
            ProtocolEventType.Play to (
                "play" to ProtocolDirection.ClientToServerAndServerToClients
            ),
            ProtocolEventType.Pause to (
                "pause" to ProtocolDirection.ClientToServerAndServerToClients
            ),
            ProtocolEventType.Seek to (
                "seek" to ProtocolDirection.ClientToServerAndServerToClients
            ),
            ProtocolEventType.Error to ("error" to ProtocolDirection.ServerToClient)
        )

        expected.forEach { (eventType, metadata) ->
            assertEquals(metadata.first, eventType.wireName)
            assertEquals(metadata.second, eventType.direction)
        }
    }

    @Test
    fun `encoding decision stays json text first`() {
        assertEquals("json", ProtocolSemantics.messageEncoding)
        assertEquals("utf8_text", ProtocolSemantics.frameType)
        assertEquals("milliseconds", ProtocolSemantics.positionUnit)
        assertEquals("server", ProtocolSemantics.sequenceAuthority)
    }

    @Test
    fun `envelope factories keep event to wire-name mapping stable`() {
        assertEquals("join_room", JoinRoomPayload("room_001", "user_a").toEnvelope().type)
        assertEquals(
            "room_state",
            RoomStatePayload("room_001", "sample_001", "user_a", false, 125_000L, 1.0, 3L)
                .toEnvelope()
                .type
        )
        assertEquals(
            "play",
            PlayPayload("room_001", "user_a", 125_000L, 4L).toEnvelope().type
        )
        assertEquals(
            "pause",
            PausePayload("room_001", "user_a", 130_500L, 5L).toEnvelope().type
        )
        assertEquals(
            "seek",
            SeekPayload("room_001", "user_a", 210_000L, 6L).toEnvelope().type
        )
        assertEquals(
            "error",
            ErrorPayload("room_001", "room not found").toEnvelope().type
        )
    }
}
