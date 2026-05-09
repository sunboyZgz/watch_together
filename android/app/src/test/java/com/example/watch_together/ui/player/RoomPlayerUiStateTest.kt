package com.example.watch_together.ui.player

import androidx.media3.common.Player
import com.example.watch_together.sync.RoomMember
import com.example.watch_together.sync.RoomSyncState
import org.junit.Assert.assertFalse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class RoomPlayerUiStateTest {

    @Test
    fun `idle player cannot be controlled`() {
        val state = RoomPlayerUiState(
            player = PlayerRuntimeUiState(playbackState = Player.STATE_IDLE)
        )

        assertFalse(state.hasPlayableMedia)
        assertFalse(state.canControlPlayback)
    }

    @Test
    fun `ready player can be controlled`() {
        val state = RoomPlayerUiState(
            player = PlayerRuntimeUiState(playbackState = Player.STATE_READY)
        )

        assertTrue(state.hasPlayableMedia)
        assertTrue(state.canControlPlayback)
    }

    @Test
    fun `buffering player is loaded but cannot be controlled yet`() {
        val state = RoomPlayerUiState(
            player = PlayerRuntimeUiState(playbackState = Player.STATE_BUFFERING)
        )

        assertTrue(state.hasPlayableMedia)
        assertFalse(state.canControlPlayback)
    }

    @Test
    fun `ended player is loaded but cannot start normal controls`() {
        val state = RoomPlayerUiState(
            player = PlayerRuntimeUiState(playbackState = Player.STATE_ENDED)
        )

        assertTrue(state.hasPlayableMedia)
        assertFalse(state.canControlPlayback)
    }

    @Test
    fun `buffered ahead never returns negative value`() {
        val state = PlayerRuntimeUiState(
            currentPosition = 12_000L,
            bufferedPosition = 10_000L
        )

        assertFalse(state.bufferedAheadMs < 0L)
    }

    @Test
    fun `display playback speed prefers authoritative sync rate over temporary local nudge`() {
        val state = RoomPlayerUiState(
            latestSyncState = RoomSyncState(
                roomId = "A7K2M9",
                mediaId = "episode-01",
                hostUserId = "host-01",
                positionMs = 12_000L,
                paused = false,
                playbackRate = 2.0,
                ended = false,
                seq = 8L
            ),
            player = PlayerRuntimeUiState(
                playbackState = Player.STATE_READY,
                playbackSpeed = 2.06f
            )
        )

        assertEquals(2.0f, state.displayPlaybackSpeed)
    }

    @Test
    fun `should keep screen on during active synchronized playback`() {
        val state = RoomPlayerUiState(
            latestSyncState = RoomSyncState(
                roomId = "A7K2M9",
                mediaId = "episode-01",
                hostUserId = "host-01",
                positionMs = 12_000L,
                paused = false,
                playbackRate = 1.0,
                ended = false,
                seq = 8L
            ),
            player = PlayerRuntimeUiState(
                playbackState = Player.STATE_READY,
                isPlaying = true
            )
        )

        assertTrue(state.shouldKeepScreenOn)
    }

    @Test
    fun `should release screen wake lock when synchronized playback is paused`() {
        val state = RoomPlayerUiState(
            latestSyncState = RoomSyncState(
                roomId = "A7K2M9",
                mediaId = "episode-01",
                hostUserId = "host-01",
                positionMs = 12_000L,
                paused = true,
                playbackRate = 1.0,
                ended = false,
                seq = 8L
            ),
            player = PlayerRuntimeUiState(
                playbackState = Player.STATE_READY,
                isPlaying = false
            )
        )

        assertFalse(state.shouldKeepScreenOn)
    }

    @Test
    fun `should keep screen on while recovering from playback buffering`() {
        val state = RoomPlayerUiState(
            latestSyncState = RoomSyncState(
                roomId = "A7K2M9",
                mediaId = "episode-01",
                hostUserId = "host-01",
                positionMs = 12_000L,
                paused = false,
                playbackRate = 1.0,
                ended = false,
                seq = 8L
            ),
            player = PlayerRuntimeUiState(
                playbackState = Player.STATE_BUFFERING,
                isPlaying = false,
                currentPosition = 12_000L,
                bufferedPosition = 18_000L
            )
        )

        assertTrue(state.shouldKeepScreenOn)
    }

    @Test
    fun `host authority should pause when app backgrounds during active playback`() {
        val state = RoomPlayerUiState(
            latestSyncState = RoomSyncState(
                roomId = "A7K2M9",
                mediaId = "episode-01",
                hostUserId = "host-01",
                positionMs = 12_000L,
                paused = false,
                playbackRate = 1.0,
                ended = false,
                seq = 8L
            ),
            player = PlayerRuntimeUiState(
                playbackState = Player.STATE_READY,
                isPlaying = true
            )
        )

        assertTrue(state.shouldPauseAuthorityOnBackground)
    }

    @Test
    fun `background pause is skipped when synchronized playback is already paused`() {
        val state = RoomPlayerUiState(
            latestSyncState = RoomSyncState(
                roomId = "A7K2M9",
                mediaId = "episode-01",
                hostUserId = "host-01",
                positionMs = 12_000L,
                paused = true,
                playbackRate = 1.0,
                ended = false,
                seq = 8L
            ),
            player = PlayerRuntimeUiState(
                playbackState = Player.STATE_READY,
                isPlaying = false
            )
        )

        assertFalse(state.shouldPauseAuthorityOnBackground)
    }

    @Test
    fun `join failure clears stale room context when no active room session exists`() {
        val state = RoomPlayerUiState(
            joinRoomInput = "OLD999",
            activeUserId = "viewer-01",
            currentRoomId = "OLD999",
            roomMembers = listOf(
                RoomMember(
                    userId = "viewer-01",
                    nickname = "Viewer",
                    avatarSeed = "viewer-01",
                    avatarUrl = null,
                    role = "member"
                )
            ),
            syncStatus = SyncStatus.JoiningAsViewer,
            player = PlayerRuntimeUiState(
                playbackState = Player.STATE_READY,
                isPlaying = true
            ),
            telemetry = PlayerTelemetryUiState(currentMediaUrl = "http://example.com/old.m3u8"),
            lastEndedReportedSeq = 12L,
            awaitingFirstFrameAfterSeek = true,
            seekRecoveryDeadlineAtMs = 999L,
            seekRecoveryRetryCount = 1
        )

        val failed = state.afterJoinFailure("BAD123")

        assertEquals("BAD123", failed.joinRoomInput)
        assertEquals(null, failed.currentRoomId)
        assertEquals(null, failed.activeUserId)
        assertTrue(failed.roomMembers.isEmpty())
        assertEquals(SyncStatus.SyncFailed, failed.syncStatus)
        assertEquals(Player.STATE_IDLE, failed.player.playbackState)
        assertEquals("", failed.telemetry.currentMediaUrl)
        assertFalse(failed.awaitingFirstFrameAfterSeek)
        assertEquals(0L, failed.seekRecoveryDeadlineAtMs)
    }

    @Test
    fun `join failure preserves current room only when retrying the same room`() {
        val syncState = RoomSyncState(
            roomId = "ROOM01",
            mediaId = "episode-01",
            hostUserId = "host-01",
            positionMs = 12_000L,
            paused = false,
            playbackRate = 1.0,
            ended = false,
            seq = 8L
        )
        val state = RoomPlayerUiState(
            joinRoomInput = "ROOM01",
            activeUserId = "viewer-01",
            currentRoomId = "ROOM01",
            roomMembers = listOf(
                RoomMember(
                    userId = "viewer-01",
                    nickname = "Viewer",
                    avatarSeed = "viewer-01",
                    avatarUrl = null,
                    role = "member"
                )
            ),
            latestSyncState = syncState,
            syncStatus = SyncStatus.Connected
        )

        val failed = state.afterJoinFailure("ROOM01")

        assertEquals("ROOM01", failed.joinRoomInput)
        assertEquals("ROOM01", failed.currentRoomId)
        assertEquals(syncState, failed.latestSyncState)
        assertEquals(SyncStatus.Connected, failed.syncStatus)
        assertEquals(1, failed.roomMembers.size)
    }

    @Test
    fun `join failure clears current room when switching to a different room`() {
        val syncState = RoomSyncState(
            roomId = "ROOM01",
            mediaId = "episode-01",
            hostUserId = "host-01",
            positionMs = 12_000L,
            paused = false,
            playbackRate = 1.0,
            ended = false,
            seq = 8L
        )
        val state = RoomPlayerUiState(
            joinRoomInput = "ROOM01",
            activeUserId = "viewer-01",
            currentRoomId = "ROOM01",
            roomMembers = listOf(
                RoomMember(
                    userId = "viewer-01",
                    nickname = "Viewer",
                    avatarSeed = "viewer-01",
                    avatarUrl = null,
                    role = "member"
                )
            ),
            latestSyncState = syncState,
            syncStatus = SyncStatus.Connected,
            player = PlayerRuntimeUiState(
                playbackState = Player.STATE_READY,
                isPlaying = true
            ),
            telemetry = PlayerTelemetryUiState(currentMediaUrl = "http://example.com/room01.m3u8")
        )

        val failed = state.afterJoinFailure("BAD123")

        assertEquals("BAD123", failed.joinRoomInput)
        assertEquals(null, failed.currentRoomId)
        assertEquals(null, failed.latestSyncState)
        assertTrue(failed.roomMembers.isEmpty())
        assertEquals(SyncStatus.SyncFailed, failed.syncStatus)
        assertEquals(Player.STATE_IDLE, failed.player.playbackState)
        assertEquals("", failed.telemetry.currentMediaUrl)
    }
}
