package com.example.watch_together.pages

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import com.example.watch_together.pages.home.HomePage
import com.example.watch_together.pages.login.LoginPage
import com.example.watch_together.pages.video.VideoSelectionPage
import com.example.watch_together.ui.player.PlayerScreen

private enum class AppScreen {
    Home,
    VideoSelection,
    Player
}

@Composable
fun WatchTogetherApp() {
    var sessionAccount by rememberSaveable { mutableStateOf<String?>(null) }
    var currentScreen by rememberSaveable { mutableStateOf(AppScreen.Home) }

    if (sessionAccount == null) {
        LoginPage(
            onLoginConfirmed = { confirmedAccount ->
                sessionAccount = confirmedAccount
                currentScreen = AppScreen.Home
            }
        )
    } else {
        when (currentScreen) {
            AppScreen.Home -> HomePage(
                sessionAccount = sessionAccount.orEmpty(),
                onCreateRoomClick = { currentScreen = AppScreen.VideoSelection }
            )

            AppScreen.VideoSelection -> VideoSelectionPage(
                onBackClick = { currentScreen = AppScreen.Home },
                onCreateRoomClick = { currentScreen = AppScreen.Player }
            )

            AppScreen.Player -> PlayerScreen()
        }
    }
}
