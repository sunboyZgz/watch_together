package com.example.watch_together.pages

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import com.example.watch_together.pages.login.LoginPage
import com.example.watch_together.ui.player.PlayerScreen

@Composable
fun WatchTogetherApp() {
    var sessionAccount by rememberSaveable { mutableStateOf<String?>(null) }

    if (sessionAccount == null) {
        LoginPage(
            onLoginConfirmed = { confirmedAccount ->
                sessionAccount = confirmedAccount
            }
        )
    } else {
        PlayerScreen()
    }
}
