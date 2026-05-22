package com.example.watch_together.ui.player

import org.junit.After
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.IOException
import java.net.CookieHandler
import java.net.CookieManager
import java.net.URI

class MediaHttpCookieBridgeTest {
    private var previousHandler: CookieHandler? = null

    @Before
    fun rememberCookieHandler() {
        previousHandler = CookieHandler.getDefault()
    }

    @After
    fun restoreCookieHandler() {
        CookieHandler.setDefault(previousHandler)
    }

    @Test
    fun installsCookieManagerWhenMissing() {
        CookieHandler.setDefault(null)

        ensureMediaHttpCookieManagerInstalled()

        assertTrue(CookieHandler.getDefault() is CookieManager)
    }

    @Test
    fun keepsExistingCookieHandler() {
        val existingHandler = object : CookieHandler() {
            override fun get(uri: URI?, requestHeaders: MutableMap<String, MutableList<String>>?): MutableMap<String, MutableList<String>> {
                return mutableMapOf()
            }

            @Throws(IOException::class)
            override fun put(uri: URI?, responseHeaders: MutableMap<String, MutableList<String>>?) = Unit
        }
        CookieHandler.setDefault(existingHandler)

        ensureMediaHttpCookieManagerInstalled()

        assertSame(existingHandler, CookieHandler.getDefault())
    }
}
