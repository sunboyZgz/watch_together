package com.example.watch_together.sync

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SyncMessageDecoderTest {

    private val decoder = SyncMessageDecoder()

    @Test
    fun `room_state message decodes into shared payload shape`() {
        val rawMessage = """
            {
              "type": "room_state",
              "payload": {
                "roomId": "ABC123",
                "mediaId": "sample_001",
                "hostUserId": "user_a",
                "paused": false,
                "positionMs": 125000,
                "playbackRate": 1.25,
                "seq": 3
              }
            }
        """.trimIndent()

        val decoded = decoder.decode(rawMessage) as SyncMessage.RoomState

        assertEquals("ABC123", decoded.payload.roomId)
        assertEquals("sample_001", decoded.payload.mediaId)
        assertFalse(decoded.payload.paused)
        assertEquals(125_000L, decoded.payload.positionMs)
        assertEquals(1.25, decoded.payload.playbackRate, 0.0)
        assertEquals(3L, decoded.payload.seq)
    }

    @Test
    fun `error message keeps server message and room id`() {
        val rawMessage = """
            {
              "type": "error",
              "payload": {
                "roomId": "MISSING",
                "message": "room not found"
              }
            }
        """.trimIndent()

        val decoded = decoder.decode(rawMessage) as SyncMessage.Error

        assertEquals("MISSING", decoded.payload.roomId)
        assertEquals("room not found", decoded.payload.message)
    }

    @Test
    fun `room_state payload converts to room sync state`() {
        val syncState = decoder.decode(
            """
                {
                  "type": "room_state",
                  "payload": {
                    "roomId": "ROOM42",
                    "mediaId": "sample_001",
                    "hostUserId": "host_a",
                    "paused": true,
                    "positionMs": 0,
                    "playbackRate": 1.0,
                    "seq": 1
                  }
                }
            """.trimIndent()
        ) as SyncMessage.RoomState

        val roomSyncState = syncState.payload.toRoomSyncState()

        assertEquals("ROOM42", roomSyncState.roomId)
        assertEquals("host_a", roomSyncState.hostUserId)
        assertTrue(roomSyncState.paused)
    }

    @Test
    fun `control events decode with shared room id position and seq fields`() {
        val play = decoder.decode(
            """
                {
                  "type": "play",
                  "payload": {
                    "roomId": "ROOM42",
                    "userId": "host_a",
                    "positionMs": 12000,
                    "seq": 2
                  }
                }
            """.trimIndent()
        ) as SyncMessage.Play

        val pause = decoder.decode(
            """
                {
                  "type": "pause",
                  "payload": {
                    "roomId": "ROOM42",
                    "userId": "host_a",
                    "positionMs": 15000,
                    "seq": 3
                  }
                }
            """.trimIndent()
        ) as SyncMessage.Pause

        val seek = decoder.decode(
            """
                {
                  "type": "seek",
                  "payload": {
                    "roomId": "ROOM42",
                    "userId": "host_a",
                    "positionMs": 42000,
                    "seq": 4
                  }
                }
            """.trimIndent()
        ) as SyncMessage.Seek

        assertEquals("ROOM42", play.payload.roomId)
        assertEquals(12_000L, play.payload.positionMs)
        assertEquals(2L, play.payload.seq)
        assertEquals(15_000L, pause.payload.positionMs)
        assertEquals(3L, pause.payload.seq)
        assertEquals(42_000L, seek.payload.positionMs)
        assertEquals(4L, seek.payload.seq)
    }

    @Test
    fun `heartbeat message decodes into heartbeat payload`() {
        val rawMessage = """
            {
              "type": "heartbeat",
              "payload": {
                "serverTimeMs": 1710000000000
              }
            }
        """.trimIndent()

        val decoded = decoder.decode(rawMessage) as SyncMessage.Heartbeat

        assertEquals(1_710_000_000_000L, decoded.payload.serverTimeMs)
    }
}
