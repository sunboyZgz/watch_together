package com.example.watch_together.ui.player_default

import org.junit.Assert.assertEquals
import org.junit.Test

class PlayerVideoQualitySwitchStateTest {

    @Test
    fun `warming state explains old quality continues playing`() {
        val state = PlayerVideoQualitySwitchState(
            phase = PlayerVideoQualitySwitchPhase.Warming,
            preference = PlayerVideoQualityPreference(height = 1080)
        )

        assertEquals(
            "正在切换到 1080p，旧清晰度会继续播放。",
            state.noticeLabel
        )
    }

    @Test
    fun `warming state exposes compact inline hint`() {
        val state = PlayerVideoQualitySwitchState(
            phase = PlayerVideoQualitySwitchPhase.Warming,
            preference = PlayerVideoQualityPreference(height = 1080)
        )

        assertEquals("切换中 · 1080p", state.inlineHintLabel)
    }

    @Test
    fun `committed auto state explains adaptive playback`() {
        val state = PlayerVideoQualitySwitchState(
            phase = PlayerVideoQualitySwitchPhase.Committed,
            preference = PlayerVideoQualityPreference.Auto
        )

        assertEquals(
            "已切回自动清晰度，会根据播放流畅度调整。",
            state.noticeLabel
        )
    }

    @Test
    fun `selection ui can temporarily show requested quality while switch is pending`() {
        val runtime = PlayerRuntimeUiState(
            videoQualityPreference = PlayerVideoQualityPreference(height = 720),
            videoQualitySwitchState = PlayerVideoQualitySwitchState(
                phase = PlayerVideoQualitySwitchPhase.Requested,
                preference = PlayerVideoQualityPreference(height = 1080),
                effectivePreference = PlayerVideoQualityPreference(height = 720)
            )
        )

        assertEquals("1080p", runtime.qualityPreferenceForSelectionUi.label)
    }

    @Test
    fun `selection ui falls back to effective quality after blocked switch`() {
        val runtime = PlayerRuntimeUiState(
            videoQualityPreference = PlayerVideoQualityPreference(height = 720),
            videoQualitySwitchState = PlayerVideoQualitySwitchState(
                phase = PlayerVideoQualitySwitchPhase.Blocked,
                preference = PlayerVideoQualityPreference(height = 1080),
                effectivePreference = PlayerVideoQualityPreference(height = 720),
                detail = "当前网络或缓冲条件不足，暂不切换。"
            )
        )

        assertEquals("720p", runtime.qualityPreferenceForSelectionUi.label)
    }

    @Test
    fun `commit attempt timeout keeps a sensible lower bound`() {
        assertEquals(2_500L, commitAttemptTimeoutMs(2_000L, bridgedSegments = 3))
    }

    @Test
    fun `commit attempt timeout is capped for long grace windows`() {
        assertEquals(6_000L, commitAttemptTimeoutMs(20_000L, bridgedSegments = 1))
    }

    @Test
    fun `commit attempt notice communicates final switch attempt instead of warmup`() {
        val state = PlayerVideoQualitySwitchState(
            phase = PlayerVideoQualitySwitchPhase.CommitAttempt,
            preference = PlayerVideoQualityPreference(height = 1080),
            detail = "当前播放窗口附近已预热完成，正在尝试接管到 1080p。"
        )

        assertEquals("切换中 · 1080p", state.inlineHintLabel)
        assertEquals("当前播放窗口附近已预热完成，正在尝试接管到 1080p。", state.noticeLabel)
    }
}
