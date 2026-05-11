package com.example.watch_together.ui.player

import androidx.media3.common.Player
import kotlin.math.max

internal enum class QualitySwitchStrategy {
    Conservative,
    Balanced,
    Aggressive,
}

internal data class ThroughputSample(
    val bytesTransferred: Long,
    val durationMs: Long,
    val source: String,
    val capturedAtMs: Long = System.currentTimeMillis(),
)

internal data class BandwidthEstimate(
    val throughputBps: Long = 0L,
    val throughputEwmaBps: Long = 0L,
    val confidence: Float = 0f,
    val sampleCount: Int = 0,
    val lastUpdatedAtMs: Long = 0L,
) {
    val hasSignal: Boolean
        get() = throughputEwmaBps > 0L && confidence > 0f
}

internal class BandwidthAwareThroughputEstimator(
    private val alpha: Double = 0.25
) {
    private var estimate = BandwidthEstimate()

    fun currentEstimate(): BandwidthEstimate = estimate

    fun record(sample: ThroughputSample): BandwidthEstimate {
        if (sample.bytesTransferred <= 0L || sample.durationMs <= 0L) return estimate

        val instantBps = ((sample.bytesTransferred * 8_000.0) / sample.durationMs.toDouble())
            .toLong()
            .coerceAtLeast(0L)
        val ewma = if (estimate.throughputEwmaBps <= 0L) {
            instantBps
        } else {
            (estimate.throughputEwmaBps * (1 - alpha) + instantBps * alpha)
                .toLong()
                .coerceAtLeast(0L)
        }
        val sampleCount = estimate.sampleCount + 1
        val confidence = (sampleCount / 6f).coerceIn(0f, 1f)
        estimate = BandwidthEstimate(
            throughputBps = instantBps,
            throughputEwmaBps = ewma,
            confidence = confidence,
            sampleCount = sampleCount,
            lastUpdatedAtMs = sample.capturedAtMs
        )
        return estimate
    }
}

internal data class QualitySwitchPlanningContext(
    val playbackSpeed: Float,
    val bufferedAheadMs: Long,
    val effectiveBufferedAheadMs: Long,
    val estimatedSegmentsAhead: Int,
    val rebufferCount: Int,
    val currentVariant: PlayerVideoVariant,
    val targetVariant: PlayerVideoVariant,
    val targetBandwidthBps: Int,
    val bandwidthEstimate: BandwidthEstimate,
    val cachedTargetSegments: Int,
)

internal data class QualitySwitchPlan(
    val strategy: QualitySwitchStrategy,
    val skipSegments: Int,
    val backfillSegments: Int,
    val bridgeSegments: Int,
    val warmSegments: Int,
    val targetWarmupWindowMs: Long,
    val graceWindowMs: Long,
    val commitMinEffectiveAheadMs: Long,
    val safetyThroughputMultiplier: Double,
    val shouldAttemptWarmup: Boolean,
    val blockedReason: String = "",
)

internal data class QualitySwitchCommitContext(
    val targetBandwidthBps: Int,
    val effectiveBufferedAheadMs: Long,
    val warmedSegments: Int,
    val bridgedSegments: Int,
    val bandwidthEstimate: BandwidthEstimate,
    val playbackSpeed: Float,
    val rebufferCount: Int,
    val targetVariant: PlayerVideoVariant,
)

internal data class QualitySwitchCommitDecision(
    val allowCommit: Boolean,
    val reason: String = "",
)

internal data class QualitySwitchFallbackObservation(
    val advancedPositionMs: Long,
    val renderedFirstFrame: Boolean,
    val effectiveBufferedAheadMs: Long,
    val playbackState: Int,
    val targetVariant: PlayerVideoVariant,
)

internal data class VariantPreparationContext(
    val playbackSpeed: Float,
    val effectiveBufferedAheadMs: Long,
    val estimatedSegmentsAhead: Int,
    val rebufferCount: Int,
    val currentVariant: PlayerVideoVariant,
    val availableVariants: List<PlayerVideoVariant>,
    val bandwidthEstimate: BandwidthEstimate,
)

internal data class VariantPreparationPlan(
    val targetVariant: PlayerVideoVariant? = null,
    val skipSegments: Int = 0,
    val warmSegments: Int = 0,
    val keepAheadWindowMs: Long = 0L,
    val reason: String = "",
) {
    val shouldPrepare: Boolean
        get() = targetVariant != null && warmSegments > 0
}

internal object BandwidthAwareQualitySwitchAdvisor {
    fun plan(context: QualitySwitchPlanningContext): QualitySwitchPlan {
        val strategy = when {
            context.targetVariant.height >= 1080 && (
                context.rebufferCount > 0 ||
                    context.effectiveBufferedAheadMs < HIGH_RISK_BUFFER_MS ||
                    context.bandwidthEstimate.confidence < MIN_ESTIMATE_CONFIDENCE
                ) -> QualitySwitchStrategy.Conservative
            context.rebufferCount >= 2 ||
                context.effectiveBufferedAheadMs < LOW_BUFFER_MS ||
                context.bandwidthEstimate.confidence < MIN_ESTIMATE_CONFIDENCE -> QualitySwitchStrategy.Conservative
            context.effectiveBufferedAheadMs >= HEALTHY_BUFFER_MS &&
                context.bandwidthEstimate.throughputEwmaBps >= (context.targetBandwidthBps * HIGH_THROUGHPUT_MULTIPLIER).toLong()
                -> QualitySwitchStrategy.Aggressive
            else -> QualitySwitchStrategy.Balanced
        }

        val basePlan = when (strategy) {
            QualitySwitchStrategy.Conservative -> QualitySwitchPlan(
                strategy = strategy,
                skipSegments = 0,
                backfillSegments = if (context.targetVariant.height >= 1080) 2 else 1,
                bridgeSegments = if (context.targetVariant.height >= 1080) 2 else 1,
                warmSegments = if (context.targetVariant.height >= 1080) 4 else 3,
                targetWarmupWindowMs = if (context.targetVariant.height >= 1080) 24_000L else 18_000L,
                graceWindowMs = if (context.targetVariant.height >= 1080) 8_000L else 6_000L,
                commitMinEffectiveAheadMs = if (context.targetVariant.height >= 1080) 16_000L else 12_000L,
                safetyThroughputMultiplier = if (context.targetVariant.height >= 1080) 1.55 else 1.3,
                shouldAttemptWarmup = true
            )
            QualitySwitchStrategy.Balanced -> QualitySwitchPlan(
                strategy = strategy,
                skipSegments = 1,
                backfillSegments = 1,
                bridgeSegments = 2,
                warmSegments = 4,
                targetWarmupWindowMs = 18_000L,
                graceWindowMs = 6_000L,
                commitMinEffectiveAheadMs = 12_000L,
                safetyThroughputMultiplier = 1.2,
                shouldAttemptWarmup = true
            )
            QualitySwitchStrategy.Aggressive -> QualitySwitchPlan(
                strategy = strategy,
                skipSegments = 2,
                backfillSegments = 1,
                bridgeSegments = 3,
                warmSegments = 5,
                targetWarmupWindowMs = 24_000L,
                graceWindowMs = 5_000L,
                commitMinEffectiveAheadMs = 10_000L,
                safetyThroughputMultiplier = 1.05,
                shouldAttemptWarmup = true
            )
        }

        if (context.targetVariant.height >= 1080 &&
            context.bandwidthEstimate.hasSignal &&
            context.bandwidthEstimate.throughputEwmaBps <
            (context.targetBandwidthBps * BLOCK_1080P_THROUGHPUT_MULTIPLIER).toLong()
        ) {
            return basePlan.copy(
                shouldAttemptWarmup = false,
                blockedReason = "当前网络条件不足，暂不切到 1080p。"
            )
        }

        if (context.targetVariant.height >= 1080 &&
            context.effectiveBufferedAheadMs < MIN_1080P_EFFECTIVE_AHEAD_MS
        ) {
            return basePlan.copy(
                shouldAttemptWarmup = false,
                blockedReason = "当前缓冲不足，暂不切到 1080p。"
            )
        }

        return basePlan.copy(
            skipSegments = max(basePlan.skipSegments, context.cachedTargetSegments.coerceAtMost(2))
        )
    }

    fun evaluateCommit(context: QualitySwitchCommitContext): QualitySwitchCommitDecision {
        if (context.warmedSegments <= 0) {
            return QualitySwitchCommitDecision(
                allowCommit = false,
                reason = "目标清晰度预热不足。"
            )
        }
        if (context.bridgedSegments <= 0) {
            return QualitySwitchCommitDecision(
                allowCommit = false,
                reason = "新清晰度尚未预热到当前播放窗口附近。"
            )
        }
        if (context.effectiveBufferedAheadMs < if (context.targetVariant.height >= 1080) 16_000L else 10_000L) {
            return QualitySwitchCommitDecision(
                allowCommit = false,
                reason = "当前有效缓冲不足，延后切换。"
            )
        }
        if (context.bandwidthEstimate.hasSignal) {
            val requiredBps = (context.targetBandwidthBps * if (context.targetVariant.height >= 1080) 1.4 else 1.1)
                .toLong()
            if (context.bandwidthEstimate.throughputEwmaBps < requiredBps) {
                return QualitySwitchCommitDecision(
                    allowCommit = false,
                    reason = "当前吞吐不足，延后切换。"
                )
            }
        }
        if (context.targetVariant.height >= 1080 && context.rebufferCount > 0) {
            return QualitySwitchCommitDecision(
                allowCommit = false,
                reason = "近期缓冲不稳定，暂不切到 1080p。"
            )
        }
        return QualitySwitchCommitDecision(allowCommit = true)
    }

    fun shouldFallback(observation: QualitySwitchFallbackObservation): Boolean {
        if (observation.playbackState == Player.STATE_BUFFERING) return true
        if (observation.targetVariant.height >= 1080) {
            if (!observation.renderedFirstFrame) return true
            if (observation.advancedPositionMs < 1_200L) return true
            if (observation.effectiveBufferedAheadMs < 6_000L) return true
        } else {
            if (observation.advancedPositionMs < 600L && !observation.renderedFirstFrame) return true
        }
        return false
    }

    fun planPreparation(context: VariantPreparationContext): VariantPreparationPlan {
        val targetVariant = context.availableVariants
            .filter { it.height > context.currentVariant.height }
            .maxByOrNull { it.height }
            ?: return VariantPreparationPlan(reason = "no higher variant")

        if (targetVariant.height < 1080) {
            return VariantPreparationPlan(reason = "higher variant below 1080p")
        }
        if (context.rebufferCount > 0) {
            return VariantPreparationPlan(reason = "recent rebuffer")
        }
        if (context.effectiveBufferedAheadMs < PREPARE_MIN_EFFECTIVE_AHEAD_MS) {
            return VariantPreparationPlan(reason = "buffer ahead too small")
        }

        val estimate = context.bandwidthEstimate
        if (estimate.hasSignal) {
            val requiredBps = (targetVariant.bitrate * PREPARE_THROUGHPUT_MULTIPLIER).toLong()
            if (estimate.throughputEwmaBps < requiredBps) {
                return VariantPreparationPlan(reason = "throughput below preparation threshold")
            }
        }

        val warmSegments = when {
            context.playbackSpeed <= 1.05f && context.effectiveBufferedAheadMs >= PREPARE_LONG_AHEAD_MS -> 10
            context.playbackSpeed <= 1.05f -> 8
            context.playbackSpeed <= 1.5f -> 6
            else -> 4
        }
        val skipSegments = context.estimatedSegmentsAhead
            .coerceAtLeast(1)
            .coerceAtMost(4)
        val keepAheadWindowMs = when {
            context.playbackSpeed <= 1.05f -> 60_000L
            context.playbackSpeed <= 1.5f -> 36_000L
            else -> 24_000L
        }

        return VariantPreparationPlan(
            targetVariant = targetVariant,
            skipSegments = skipSegments,
            warmSegments = warmSegments,
            keepAheadWindowMs = keepAheadWindowMs,
            reason = "prepare ${targetVariant.qualityLabel} cache for future sync pressure"
        )
    }

    private const val MIN_ESTIMATE_CONFIDENCE = 0.35f
    private const val LOW_BUFFER_MS = 10_000L
    private const val HEALTHY_BUFFER_MS = 24_000L
    private const val HIGH_RISK_BUFFER_MS = 18_000L
    private const val MIN_1080P_EFFECTIVE_AHEAD_MS = 14_000L
    private const val HIGH_THROUGHPUT_MULTIPLIER = 1.45
    private const val BLOCK_1080P_THROUGHPUT_MULTIPLIER = 1.15
    private const val PREPARE_MIN_EFFECTIVE_AHEAD_MS = 18_000L
    private const val PREPARE_LONG_AHEAD_MS = 30_000L
    private const val PREPARE_THROUGHPUT_MULTIPLIER = 1.05
}
