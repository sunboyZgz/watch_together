package com.example.watch_together.ui.player

import android.view.ViewGroup
import androidx.activity.ComponentActivity
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class KeepScreenAwakeEffectTest {

    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun keepScreenOn_followsComposableState() {
        var shouldKeepScreenOn by mutableStateOf(false)

        composeRule.setContent {
            KeepScreenAwakeEffect(shouldKeepScreenOn = shouldKeepScreenOn)
        }

        composeRule.runOnIdle {
            assertFalse(composeHostView().keepScreenOn)
            shouldKeepScreenOn = true
        }

        composeRule.runOnIdle {
            assertTrue(composeHostView().keepScreenOn)
            shouldKeepScreenOn = false
        }

        composeRule.runOnIdle {
            assertFalse(composeHostView().keepScreenOn)
        }
    }

    private fun composeHostView(): android.view.View {
        val content = composeRule.activity.findViewById<ViewGroup>(android.R.id.content)
        return content.getChildAt(0)
    }
}
