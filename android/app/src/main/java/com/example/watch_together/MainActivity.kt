package com.example.watch_together

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import com.example.watch_together.pages.WatchTogetherApp
import com.example.watch_together.ui.theme.Watch_togetherTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            Watch_togetherTheme {
                WatchTogetherApp()
            }
        }
    }
}
