package com.example.watch_together

import android.annotation.SuppressLint
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.material3.Scaffold
import androidx.compose.ui.Modifier
import androidx.compose.foundation.layout.fillMaxSize
import com.example.watch_together.ui.theme.Watch_togetherTheme
import com.example.watch_together.ui.player.PlayerScreen

class MainActivity : ComponentActivity() {
    @SuppressLint("UnusedMaterial3ScaffoldPaddingParameter")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            Watch_togetherTheme {
                Scaffold(modifier = Modifier.fillMaxSize()) {
                    PlayerScreen()
                }
            }
        }
    }
}
