package com.example.watch_together.pages

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import com.example.watch_together.auth.SharedPreferencesAuthSessionStore
import com.example.watch_together.config.AppConfig
import com.example.watch_together.pages.home.HomePage
import com.example.watch_together.pages.login.LoginPage
import com.example.watch_together.pages.room.RoomTheaterScreen
import com.example.watch_together.pages.video.MediaEpisode
import com.example.watch_together.pages.video.VideoSelectionPage

private enum class AppScreen {
    Login,
    Home,
    VideoSelection,
    Player
}

@Composable
fun WatchTogetherApp() {
    val context = LocalContext.current
    val authSessionStore = remember(context) {
        SharedPreferencesAuthSessionStore(context)
    }
    val savedSession = remember(authSessionStore) {
        authSessionStore.load()
    }

    var currentScreen by rememberSaveable {
        mutableStateOf(if (savedSession == null) AppScreen.Login else AppScreen.Home)
    }
    var sessionUserId by rememberSaveable { mutableStateOf(savedSession?.user?.id.orEmpty()) }
    var sessionAccount by rememberSaveable { mutableStateOf(savedSession?.user?.account.orEmpty()) }
    var sessionNickname by rememberSaveable { mutableStateOf(savedSession?.user?.nickname.orEmpty()) }
    var sessionAccessToken by rememberSaveable { mutableStateOf(savedSession?.accessToken.orEmpty()) }
    var selectedEpisodeId by rememberSaveable { mutableStateOf(AppConfig.defaultMediaIdForRoom()) }
    var selectedEpisodeTitle by rememberSaveable { mutableStateOf("") }
    var selectedEpisodeMediaUrl by rememberSaveable { mutableStateOf<String?>(null) }
    var selectedEpisodeSeasonLabel by rememberSaveable { mutableStateOf<String?>(null) }
    var selectedEpisodeEpisodeLabel by rememberSaveable { mutableStateOf<String?>(null) }
    var pendingJoinRoomCode by rememberSaveable { mutableStateOf("") }
    var shouldAutoCreateRoom by rememberSaveable { mutableStateOf(false) }
    var shouldAutoJoinRoom by rememberSaveable { mutableStateOf(false) }

    when (currentScreen) {
        AppScreen.Login -> LoginPage(
            onLoginConfirmed = { session ->
                authSessionStore.save(session)
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
            onCreateRoomClick = { currentScreen = AppScreen.VideoSelection },
            onJoinRoomConfirm = { roomCode ->
                pendingJoinRoomCode = roomCode
                shouldAutoCreateRoom = false
                shouldAutoJoinRoom = true
                currentScreen = AppScreen.Player
            }
        )

        AppScreen.VideoSelection -> VideoSelectionPage(
            onBackClick = { currentScreen = AppScreen.Home },
            accessToken = sessionAccessToken,
            onCreateRoomClick = { episode ->
                selectedEpisodeId = episode.episodeId
                selectedEpisodeTitle = episode.title
                selectedEpisodeMediaUrl = episode.mediaUrl
                selectedEpisodeSeasonLabel = episode.seasonLabel
                selectedEpisodeEpisodeLabel = episode.episodeLabel
                pendingJoinRoomCode = ""
                shouldAutoCreateRoom = true
                shouldAutoJoinRoom = false
                currentScreen = AppScreen.Player
            }
        )

        AppScreen.Player -> RoomTheaterScreen(
            accessToken = sessionAccessToken,
            currentUserId = sessionUserId,
            currentUserNickname = sessionNickname,
            selectedEpisode = MediaEpisode(
                episodeId = selectedEpisodeId,
                title = selectedEpisodeTitle,
                subtitle = null,
                description = null,
                coverUrl = null,
                mediaUrl = selectedEpisodeMediaUrl,
                durationMs = 0,
                seasonLabel = selectedEpisodeSeasonLabel,
                episodeLabel = selectedEpisodeEpisodeLabel,
                tags = emptyList()
            ),
            initialRoomCode = pendingJoinRoomCode,
            autoCreateAsHost = shouldAutoCreateRoom,
            autoJoinAsViewer = shouldAutoJoinRoom
        )
    }
}
