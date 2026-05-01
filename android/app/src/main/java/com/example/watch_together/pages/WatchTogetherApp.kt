package com.example.watch_together.pages

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import com.example.watch_together.config.AppConfig
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
    var sessionUserId by rememberSaveable { mutableStateOf("") }
    var sessionAccount by rememberSaveable { mutableStateOf("") }
    var sessionNickname by rememberSaveable { mutableStateOf("") }
    var sessionAccessToken by rememberSaveable { mutableStateOf("") }
    var selectedEpisodeId by rememberSaveable { mutableStateOf(AppConfig.defaultMediaIdForRoom()) }

    when (currentScreen) {
        AppScreen.Login -> LoginPage(
            onLoginConfirmed = { session ->
                sessionUserId = session.user.id
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
            onCreateRoomClick = { episodeId ->
                selectedEpisodeId = episodeId
                currentScreen = AppScreen.Player
            }
        )

        AppScreen.Player -> PlayerScreen(
            accessToken = sessionAccessToken,
            currentUserId = sessionUserId,
            selectedEpisodeId = selectedEpisodeId,
            autoCreateAsHost = true
        )
    }
}
