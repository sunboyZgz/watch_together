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
    Login,
    Home,
    VideoSelection,
    Player
}

@Composable
fun WatchTogetherApp() {
    var currentScreen by rememberSaveable { mutableStateOf(AppScreen.Login) }
    var sessionAccount by rememberSaveable { mutableStateOf("") }
    var sessionNickname by rememberSaveable { mutableStateOf("") }
    var sessionAccessToken by rememberSaveable { mutableStateOf("") }

    when (currentScreen) {
        AppScreen.Login -> LoginPage(
            onLoginConfirmed = { session ->
                sessionAccount = session.user.account
                sessionNickname = session.user.nickname
                sessionAccessToken = session.accessToken
                currentScreen = AppScreen.Home
            }
        )

        AppScreen.Home -> HomePage(
            sessionAccount = sessionNickname.ifBlank { sessionAccount },
            accessToken = sessionAccessToken,
            onCreateRoomClick = { currentScreen = AppScreen.VideoSelection }
        )

        AppScreen.VideoSelection -> VideoSelectionPage(
            onBackClick = { currentScreen = AppScreen.Home },
            onCreateRoomClick = { _ -> currentScreen = AppScreen.Player }
        )

        AppScreen.Player -> PlayerScreen()
    }
}
