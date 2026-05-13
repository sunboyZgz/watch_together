package com.example.watch_together.ui.player

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier

@Composable
fun PlayerScreen(
    adapter: PlayerAdapter,
    state: PlayerRuntimeState,
    mediaTitle: String,
    mediaMeta: String,
    controlHint: String,
    controlsEnabled: Boolean,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onProgressSeekCommit: (Long) -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    modifier: Modifier = Modifier
) {
    PlayerCoreShell(
        modifier = modifier,
        adapter = adapter,
        state = state,
        mediaTitle = mediaTitle,
        mediaMeta = mediaMeta,
        controlHint = controlHint,
        controlsEnabled = controlsEnabled,
        onPlaybackToggleClick = onPlaybackToggleClick,
        onSeekBackwardClick = onSeekBackwardClick,
        onSeekForwardClick = onSeekForwardClick,
        onProgressSeekCommit = onProgressSeekCommit,
        onPlaybackSpeedChange = onPlaybackSpeedChange,
        onVideoQualityPreferenceChange = onVideoQualityPreferenceChange
    )
}
