package com.example.watch_together.ui.player

import java.net.CookieHandler
import java.net.CookieManager
import java.net.CookiePolicy

fun ensureMediaHttpCookieManagerInstalled() {
    if (CookieHandler.getDefault() == null) {
        CookieHandler.setDefault(CookieManager(null, CookiePolicy.ACCEPT_ORIGINAL_SERVER))
    }
}
