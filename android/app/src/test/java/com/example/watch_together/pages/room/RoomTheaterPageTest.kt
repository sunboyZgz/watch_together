package com.example.watch_together.pages.room

import com.example.watch_together.sync.RoomMember
import org.junit.Assert.assertEquals
import org.junit.Test

class RoomTheaterPageTest {

    @Test
    fun `viewer slot shows pending when no joined viewer exists`() {
        assertEquals("待加入", viewerSlotDisplayName(viewerMember = null, activeUserId = "host_user"))
    }

    @Test
    fun `viewer slot shows self when active user is joined viewer`() {
        val viewer = RoomMember(
            userId = "viewer_user",
            nickname = "小明",
            avatarSeed = "seed",
            avatarUrl = null,
            role = "member"
        )

        assertEquals("你", viewerSlotDisplayName(viewerMember = viewer, activeUserId = "viewer_user"))
    }

    @Test
    fun `viewer slot prefers nickname for joined member`() {
        val viewer = RoomMember(
            userId = "viewer_user",
            nickname = "小明",
            avatarSeed = "seed",
            avatarUrl = null,
            role = "member"
        )

        assertEquals("小明", viewerSlotDisplayName(viewerMember = viewer, activeUserId = "host_user"))
    }
}
