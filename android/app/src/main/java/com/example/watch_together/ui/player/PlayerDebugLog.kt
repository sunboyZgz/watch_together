package com.example.watch_together.ui.player

import android.util.Log
import com.example.watch_together.config.AppConfig

internal object PlayerDebugLog {
    fun d(tag: String, message: String) {
        if (!AppConfig.debugSync) return
        Log.d(tag, message)
    }
}
