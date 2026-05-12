package com.example.watch_together.ui.player

import androidx.media3.common.Player
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class BandwidthAwarePlaybackAdvisorTest {

    @Test
    fun `plan blocks 1080p when throughput is not enough`() {
        val plan = BandwidthAwareQualitySwitchAdvisor.plan(
            QualitySwitchPlanningContext(
                playbackSpeed = 1f,
                bufferedAheadMs = 28_000L,
                effectiveBufferedAheadMs = 28_000L,
                estimatedSegmentsAhead = 4,
                rebufferCount = 0,
                currentVariant = PlayerVideoVariant(width = 1280, height = 720, bitrate = 2_000_000),
                targetVariant = PlayerVideoVariant(width = 1920, height = 1080, bitrate = 5_000_000),
                targetBandwidthBps = 5_000_000,
                bandwidthEstimate = BandwidthEstimate(
                    throughputEwmaBps = 4_000_000,
                    confidence = 0.8f,
                    sampleCount = 4
                ),
                cachedTargetSegments = 0
            )
        )

        assertFalse(plan.shouldAttemptWarmup)
        assertEquals("当前网络条件不足，暂不切到 1080p。", plan.blockedReason)
    }

    @Test
    fun `plan prefers aggressive warmup under healthy bandwidth`() {
        val plan = BandwidthAwareQualitySwitchAdvisor.plan(
            QualitySwitchPlanningContext(
                playbackSpeed = 1f,
                bufferedAheadMs = 30_000L,
                effectiveBufferedAheadMs = 30_000L,
                estimatedSegmentsAhead = 5,
                rebufferCount = 0,
                currentVariant = PlayerVideoVariant(width = 1280, height = 720, bitrate = 2_000_000),
                targetVariant = PlayerVideoVariant(width = 1280, height = 720, bitrate = 2_000_000),
                targetBandwidthBps = 2_000_000,
                bandwidthEstimate = BandwidthEstimate(
                    throughputEwmaBps = 3_500_000,
                    confidence = 0.9f,
                    sampleCount = 5
                ),
                cachedTargetSegments = 1
            )
        )

        assertTrue(plan.shouldAttemptWarmup)
        assertEquals(QualitySwitchStrategy.Aggressive, plan.strategy)
        assertEquals(2, plan.skipSegments)
        assertEquals(5, plan.warmSegments)
    }

    @Test
    fun `commit gate allows normal speed 1080p upgrade with bridge and healthy buffer`() {
        val decision = BandwidthAwareQualitySwitchAdvisor.evaluateCommit(
            QualitySwitchCommitContext(
                targetBandwidthBps = 5_000_000,
                effectiveBufferedAheadMs = 12_000L,
                warmedSegments = 2,
                bridgedSegments = 2,
                bandwidthEstimate = BandwidthEstimate(
                    throughputEwmaBps = 4_600_000,
                    confidence = 0.9f,
                    sampleCount = 5
                ),
                playbackSpeed = 1f,
                rebufferCount = 0,
                targetVariant = PlayerVideoVariant(width = 1920, height = 1080, bitrate = 5_000_000)
            )
        )

        assertTrue(decision.allowCommit)
    }

    @Test
    fun `commit gate blocks unstable 1080p upgrade after recent rebuffer`() {
        val decision = BandwidthAwareQualitySwitchAdvisor.evaluateCommit(
            QualitySwitchCommitContext(
                targetBandwidthBps = 5_000_000,
                effectiveBufferedAheadMs = 16_000L,
                warmedSegments = 4,
                bridgedSegments = 1,
                bandwidthEstimate = BandwidthEstimate(
                    throughputEwmaBps = 9_000_000,
                    confidence = 0.9f,
                    sampleCount = 5
                ),
                playbackSpeed = 1f,
                rebufferCount = 2,
                targetVariant = PlayerVideoVariant(width = 1920, height = 1080, bitrate = 5_000_000)
            )
        )

        assertFalse(decision.allowCommit)
        assertEquals("近期缓冲不稳定，暂不切到 1080p。", decision.reason)
    }

    @Test
    fun `commit gate blocks switch when warmed target is still detached from playhead`() {
        val decision = BandwidthAwareQualitySwitchAdvisor.evaluateCommit(
            QualitySwitchCommitContext(
                targetBandwidthBps = 5_000_000,
                effectiveBufferedAheadMs = 24_000L,
                warmedSegments = 4,
                bridgedSegments = 0,
                bandwidthEstimate = BandwidthEstimate(
                    throughputEwmaBps = 9_000_000,
                    confidence = 0.9f,
                    sampleCount = 5
                ),
                playbackSpeed = 1f,
                rebufferCount = 0,
                targetVariant = PlayerVideoVariant(width = 1920, height = 1080, bitrate = 5_000_000)
            )
        )

        assertFalse(decision.allowCommit)
        assertEquals("新清晰度尚未预热到当前播放窗口附近。", decision.reason)
    }

    @Test
    fun `fallback fires when 1080p switch stays buffering`() {
        val shouldFallback = BandwidthAwareQualitySwitchAdvisor.shouldFallback(
            QualitySwitchFallbackObservation(
                advancedPositionMs = 2_000L,
                renderedFirstFrame = true,
                effectiveBufferedAheadMs = 12_000L,
                playbackState = Player.STATE_BUFFERING,
                targetVariant = PlayerVideoVariant(width = 1920, height = 1080, bitrate = 5_000_000)
            )
        )

        assertTrue(shouldFallback)
    }

    @Test
    fun `preparation plan eagerly warms 1080p at normal speed when budget is healthy`() {
        val plan = BandwidthAwareQualitySwitchAdvisor.planPreparation(
            VariantPreparationContext(
                playbackSpeed = 1f,
                effectiveBufferedAheadMs = 32_000L,
                estimatedSegmentsAhead = 4,
                rebufferCount = 0,
                currentVariant = PlayerVideoVariant(width = 1280, height = 720, bitrate = 2_000_000),
                availableVariants = listOf(
                    PlayerVideoVariant(width = 1280, height = 720, bitrate = 2_000_000),
                    PlayerVideoVariant(width = 1920, height = 1080, bitrate = 5_000_000),
                ),
                bandwidthEstimate = BandwidthEstimate(
                    throughputEwmaBps = 6_000_000,
                    confidence = 0.8f,
                    sampleCount = 5
                )
            )
        )

        assertTrue(plan.shouldPrepare)
        assertEquals(1080, plan.targetVariant?.height)
        assertEquals(4, plan.skipSegments)
        assertEquals(10, plan.warmSegments)
    }

    @Test
    fun `preparation plan stays idle when recent rebuffer exists`() {
        val plan = BandwidthAwareQualitySwitchAdvisor.planPreparation(
            VariantPreparationContext(
                playbackSpeed = 1f,
                effectiveBufferedAheadMs = 32_000L,
                estimatedSegmentsAhead = 4,
                rebufferCount = 1,
                currentVariant = PlayerVideoVariant(width = 1280, height = 720, bitrate = 2_000_000),
                availableVariants = listOf(
                    PlayerVideoVariant(width = 1280, height = 720, bitrate = 2_000_000),
                    PlayerVideoVariant(width = 1920, height = 1080, bitrate = 5_000_000),
                ),
                bandwidthEstimate = BandwidthEstimate(
                    throughputEwmaBps = 7_000_000,
                    confidence = 0.8f,
                    sampleCount = 5
                )
            )
        )

        assertFalse(plan.shouldPrepare)
        assertEquals("recent rebuffer", plan.reason)
    }
}
