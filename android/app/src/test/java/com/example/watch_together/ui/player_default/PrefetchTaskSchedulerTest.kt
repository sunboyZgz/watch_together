package com.example.watch_together.ui.player_default

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

class PrefetchTaskSchedulerTest {

    @Test
    fun `manual task preempts background task`() {
        val scheduler = PrefetchTaskScheduler {}
        val backgroundStarted = CountDownLatch(1)
        val manualRan = CountDownLatch(1)
        val backgroundInterrupted = AtomicBoolean(false)

        try {
            scheduler.submitBackground("background-1") {
                backgroundStarted.countDown()
                try {
                    Thread.sleep(5_000L)
                } catch (_: InterruptedException) {
                    backgroundInterrupted.set(true)
                    throw InterruptedException("background interrupted")
                }
            }

            assertTrue(backgroundStarted.await(1, TimeUnit.SECONDS))

            scheduler.submitManual(generation = 1L, label = "1080p") {
                manualRan.countDown()
            }

            assertTrue(manualRan.await(1, TimeUnit.SECONDS))
            assertTrue(backgroundInterrupted.get())
        } finally {
            scheduler.release()
        }
    }

    @Test
    fun `stale manual result is ignored after newer generation arrives`() {
        val scheduler = PrefetchTaskScheduler {}
        val firstDelivered = AtomicBoolean(false)
        val secondDelivered = AtomicBoolean(false)
        val secondRan = CountDownLatch(1)

        try {
            scheduler.submitManual(generation = 1L, label = "720p") {
                Thread.sleep(150L)
                firstDelivered.set(scheduler.shouldDeliverManualResult(1L))
            }

            scheduler.submitManual(generation = 2L, label = "1080p") {
                secondDelivered.set(scheduler.shouldDeliverManualResult(2L))
                secondRan.countDown()
            }

            assertTrue(secondRan.await(1, TimeUnit.SECONDS))
            Thread.sleep(200L)
            assertFalse(firstDelivered.get())
            assertTrue(secondDelivered.get())
        } finally {
            scheduler.release()
        }
    }
}
