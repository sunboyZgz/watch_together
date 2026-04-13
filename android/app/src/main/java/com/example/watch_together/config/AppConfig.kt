package com.example.watch_together.config

import com.example.watch_together.BuildConfig

object AppConfig {
    val appEnv: String = BuildConfig.APP_ENV
    val apiBaseUrl: String = BuildConfig.API_BASE_URL
    val wsBaseUrl: String = BuildConfig.WS_BASE_URL
    val mediaBaseUrl: String = BuildConfig.MEDIA_BASE_URL
    val mediaDefaultId: String = BuildConfig.MEDIA_DEFAULT_ID
    val debugSync: Boolean = BuildConfig.DEBUG_SYNC
}
