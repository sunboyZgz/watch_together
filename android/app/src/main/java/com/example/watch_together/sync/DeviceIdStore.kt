package com.example.watch_together.sync

import android.content.Context
import java.util.UUID

class DeviceIdStore(context: Context) {
    private val preferences = context.applicationContext.getSharedPreferences(
        "device",
        Context.MODE_PRIVATE
    )

    fun getOrCreateDeviceId(): String {
        val existing = preferences.getString(KEY_DEVICE_ID, null)
        if (!existing.isNullOrBlank()) return existing

        val next = UUID.randomUUID().toString()
        preferences.edit().putString(KEY_DEVICE_ID, next).apply()
        return next
    }

    private companion object {
        const val KEY_DEVICE_ID = "device_id"
    }
}
