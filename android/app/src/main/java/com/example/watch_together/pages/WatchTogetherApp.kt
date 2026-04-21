package com.example.watch_together.pages

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import com.example.watch_together.pages.home.HomePage
import com.example.watch_together.pages.login.LoginPage

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
        HomePage(sessionAccount = sessionAccount.orEmpty())
    }
}
