package com.example.watch_together.ui.player_default

import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.Future
import java.util.concurrent.atomic.AtomicLong

internal class PrefetchTaskScheduler(
    private val log: (String) -> Unit,
) {
    private val backgroundExecutor: ExecutorService = Executors.newSingleThreadExecutor { runnable ->
        Thread(runnable, "watch-together-hls-prefetch").apply {
            isDaemon = true
        }
    }
    private val manualExecutor: ExecutorService = Executors.newSingleThreadExecutor { runnable ->
        Thread(runnable, "watch-together-quality-warmup").apply {
            isDaemon = true
        }
    }
    private val schedulerLock = Any()
    private val latestManualGeneration = AtomicLong(0L)
    @Volatile
    private var backgroundFuture: Future<*>? = null
    @Volatile
    private var manualFuture: Future<*>? = null
    private var lastBackgroundRequestKey: String = ""

    fun hasActiveManualTask(): Boolean {
        val future = manualFuture
        return future != null && !future.isDone && !future.isCancelled
    }

    fun cancelBackgroundTask() {
        synchronized(schedulerLock) {
            if (backgroundFuture != null) {
                log("manual_switch_warmup_preempted_background")
            }
            backgroundFuture?.cancel(true)
            backgroundFuture = null
            lastBackgroundRequestKey = ""
        }
    }

    fun cancelStaleManualTasks(generation: Long) {
        latestManualGeneration.set(generation)
        synchronized(schedulerLock) {
            manualFuture?.cancel(true)
        }
    }

    fun submitBackground(
        requestKey: String,
        task: () -> Unit
    ) {
        if (hasActiveManualTask()) return
        if (requestKey == lastBackgroundRequestKey) return
        lastBackgroundRequestKey = requestKey
        synchronized(schedulerLock) {
            backgroundFuture?.cancel(true)
            backgroundFuture = backgroundExecutor.submit {
                try {
                    task()
                } catch (error: Throwable) {
                    if (error is InterruptedException) {
                        log("background_prefetch_cancelled")
                    } else {
                        log("prefetch failed ${error.message}")
                    }
                }
            }
        }
    }

    fun submitManual(
        generation: Long,
        label: String,
        task: () -> Unit
    ) {
        latestManualGeneration.set(generation)
        cancelBackgroundTask()
        log("manual_switch_warmup_started generation=$generation target=$label")
        synchronized(schedulerLock) {
            manualFuture?.cancel(true)
            manualFuture = manualExecutor.submit {
                task()
            }
        }
    }

    fun shouldDeliverManualResult(generation: Long): Boolean {
        return generation == latestManualGeneration.get() && !Thread.currentThread().isInterrupted
    }

    fun reset() {
        cancelBackgroundTask()
        synchronized(schedulerLock) {
            manualFuture?.cancel(true)
            manualFuture = null
        }
    }

    fun release() {
        backgroundExecutor.shutdownNow()
        manualExecutor.shutdownNow()
    }
}
