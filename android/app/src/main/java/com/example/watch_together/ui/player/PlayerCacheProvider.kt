package com.example.watch_together.ui.player

import android.content.Context
import androidx.annotation.OptIn
import androidx.media3.common.util.UnstableApi
import androidx.media3.database.StandaloneDatabaseProvider
import androidx.media3.datasource.cache.LeastRecentlyUsedCacheEvictor
import androidx.media3.datasource.cache.SimpleCache
import java.io.File

@OptIn(UnstableApi::class)
object PlayerCacheProvider {
    private const val CacheMaxBytes = 256L * 1024L * 1024L

    @Volatile
    private var cache: SimpleCache? = null

    fun get(context: Context): SimpleCache {
        return cache ?: synchronized(this) {
            cache ?: buildCache(context.applicationContext).also { cache = it }
        }
    }

    private fun buildCache(context: Context): SimpleCache {
        val cacheDir = File(context.cacheDir, "watch_together_media_cache")
        val databaseProvider = StandaloneDatabaseProvider(context)
        val evictor = LeastRecentlyUsedCacheEvictor(CacheMaxBytes)
        return SimpleCache(cacheDir, evictor, databaseProvider)
    }
}
