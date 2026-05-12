package com.example.watch_together.ui.player_default

import android.content.Context
import androidx.annotation.OptIn
import androidx.media3.common.util.UnstableApi
import androidx.media3.database.StandaloneDatabaseProvider
import androidx.media3.datasource.cache.LeastRecentlyUsedCacheEvictor
import androidx.media3.datasource.cache.SimpleCache
import java.io.File

@OptIn(UnstableApi::class)
internal object PlayerCacheProvider {
    private const val CacheDirectoryName = "watch_together_media_cache"
    private const val MaxCacheBytes = 512L * 1024L * 1024L
    private const val LOG_TAG = "WatchTogetherCache"

    @Volatile
    private var cache: SimpleCache? = null

    fun get(context: Context): SimpleCache {
        return cache ?: synchronized(this) {
            cache ?: createCache(context.applicationContext).also { created ->
                cache = created
            }
        }
    }

    private fun createCache(context: Context): SimpleCache {
        val cacheDirectory = File(context.cacheDir, CacheDirectoryName)
        val databaseProvider = StandaloneDatabaseProvider(context)
        val evictor = LeastRecentlyUsedCacheEvictor(MaxCacheBytes)
        return SimpleCache(cacheDirectory, evictor, databaseProvider).also { created ->
            PlayerDebugLog.d(
                LOG_TAG,
                "cache initialized dir=${cacheDirectory.absolutePath} maxBytes=$MaxCacheBytes usedBytes=${created.cacheSpace}"
            )
        }
    }
}
