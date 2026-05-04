package com.example.watch_together.pages.room

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.media3.common.Player
import androidx.media3.ui.PlayerView
import com.example.watch_together.ui.player.PlayerAdapter
import com.example.watch_together.ui.player.PlayerCoreShell
import com.example.watch_together.ui.player.PlayerEvent
import com.example.watch_together.ui.player.PlayerRuntimeUiState
import com.example.watch_together.ui.player.RoomPlayerUiState
import com.example.watch_together.ui.player.SyncStatus
import com.example.watch_together.ui.theme.Watch_togetherTheme

private val RoomBackground = Color(0xFF0F1325)
private val RoomPanel = Color(0xB8242B45)
private val RoomPanelSoft = Color(0x66333A58)
private val RoomPrimary = Color(0xFFFF78C6)
private val RoomAccent = Color(0xFF8FE7FF)
private val RoomText = Color(0xFFF9F3FF)
private val RoomTextMuted = Color(0xC8D7D1E5)
private val RoomOutline = Color(0x22FFFFFF)

private val RoomBackdropGlowA = Brush.radialGradient(
    colors = listOf(Color(0xFF514AA7), Color(0x00514AA7))
)
private val RoomBackdropGlowB = Brush.radialGradient(
    colors = listOf(Color(0xFF7A466F), Color(0x007A466F))
)
private val RoomBackdropGlowC = Brush.radialGradient(
    colors = listOf(Color(0xFF426B82), Color(0x00426B82))
)

@Composable
internal fun RoomTheaterPage(
    uiState: RoomPlayerUiState,
    adapter: PlayerAdapter,
    hostUserId: String,
    viewerUserId: String,
    mediaTitle: String,
    mediaEpisodeLabel: String?,
    isHostController: Boolean,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onProgressSeekCommit: (Long) -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onJoinRoomInputChange: (String) -> Unit,
    onCreateAndJoinAsHost: () -> Unit,
    onJoinAsViewer: () -> Unit,
    onRejoinCurrentUser: () -> Unit,
    modifier: Modifier = Modifier
) {
    BoxWithConstraints(
        modifier = modifier
            .fillMaxSize()
            .background(RoomBackground)
    ) {
        val compactHeight = maxHeight < 760.dp
        val compactWidth = maxWidth < 380.dp
        val horizontalPadding = if (compactWidth) 16.dp else 20.dp
        val sectionGap = if (compactHeight) 14.dp else 18.dp
        val roomCode = uiState.currentRoomId?.take(6)?.uppercase() ?: "A7K2M9"
        val playbackStatusLabel = playbackStatusLabel(uiState, isHostController)
        val mediaMeta = buildString {
            append(mediaEpisodeLabel ?: "当前影片")
            append(" · ")
            append(playbackStatusLabel)
        }

        RoomPlayerBackdrop()

        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = horizontalPadding)
                .padding(top = if (compactHeight) 18.dp else 28.dp, bottom = 28.dp),
            verticalArrangement = Arrangement.spacedBy(sectionGap)
        ) {
            RoomHeader(
                roomCode = roomCode,
                syncStatus = uiState.syncStatus
            )

            PlayerCoreShell(
                adapter = adapter,
                mediaTitle = mediaTitle,
                mediaMeta = mediaMeta,
                currentPosition = uiState.player.currentPosition,
                duration = uiState.player.duration,
                isPlaying = uiState.player.isPlaying,
                playbackSpeed = uiState.player.playbackSpeed,
                videoQualityLabel = uiState.player.videoVariant.displayLabel,
                controlHint = when {
                    uiState.player.playbackState == Player.STATE_BUFFERING -> "正在缓冲，播放器恢复后会继续跟随同步。"
                    uiState.player.playbackState == Player.STATE_ENDED -> "当前视频已播放结束。"
                    !uiState.canControlPlayback -> "视频载入后可操作。"
                    isHostController -> "房主操作会自动同步给房间成员。"
                    uiState.isJoinedToRoom -> "你正在跟随房主，播放控制由房主同步。"
                    else -> "先创建或加入房间，视频会自动载入。"
                },
                playbackButtonEnabled = uiState.canControlPlayback &&
                    (isHostController || !uiState.isJoinedToRoom),
                secondaryControlsEnabled = uiState.canControlPlayback,
                compactWidth = compactWidth,
                onPlaybackToggleClick = onPlaybackToggleClick,
                onSeekBackwardClick = onSeekBackwardClick,
                onSeekForwardClick = onSeekForwardClick,
                onProgressSeekCommit = onProgressSeekCommit,
                onPlaybackSpeedChange = onPlaybackSpeedChange
            )

            TheaterStatusPanel(
                hostUserId = hostUserId,
                viewerUserId = viewerUserId,
                activeUserId = uiState.activeUserId,
                isHostController = isHostController,
                playbackSpeed = uiState.player.playbackSpeed,
                videoQualityLabel = uiState.player.videoVariant.displayLabel,
                isJoinedToRoom = uiState.isJoinedToRoom,
                syncStatus = uiState.syncStatus,
                latestSeq = uiState.latestSyncState?.seq
            )

            SessionQuickActions(
                currentRoomId = uiState.currentRoomId,
                syncStatus = uiState.syncStatus,
                joinRoomInput = uiState.joinRoomInput,
                onJoinRoomInputChange = onJoinRoomInputChange,
                onCreateAndJoinAsHost = onCreateAndJoinAsHost,
                onJoinAsViewer = onJoinAsViewer,
                onRejoinCurrentUser = onRejoinCurrentUser,
                canRejoinCurrentUser = uiState.activeUserId != null && uiState.currentRoomId != null
            )
        }
    }
}

private fun playbackStatusLabel(
    uiState: RoomPlayerUiState,
    isHostController: Boolean
): String {
    return when (uiState.player.playbackState) {
        Player.STATE_BUFFERING -> "缓冲中"
        Player.STATE_ENDED -> "已结束"
        Player.STATE_IDLE -> "等待载入"
        Player.STATE_READY -> when {
            uiState.player.isPlaying -> "正在播放"
            uiState.isJoinedToRoom && !isHostController -> "跟随同步中"
            else -> "已暂停"
        }
        else -> "准备中"
    }
}

@Composable
private fun RoomHeader(
    roomCode: String,
    syncStatus: SyncStatus
) {
    val clipboard = LocalClipboardManager.current

    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.SpaceBetween
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(7.dp)) {
            Text(
                text = "星夜放映室",
                color = RoomText,
                fontWeight = FontWeight.Black,
                fontSize = 32.sp,
                lineHeight = 36.sp
            )
            Text(
                text = "同步观影 · ${syncStatus.label}",
                color = RoomTextMuted,
                fontSize = 14.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }

        Surface(
            modifier = Modifier.clickable {
                clipboard.setText(AnnotatedString(roomCode))
            },
            color = Color(0x1CFFFFFF),
            shape = RoundedCornerShape(999.dp),
            border = BorderStroke(1.dp, RoomOutline)
        ) {
            Column(
                modifier = Modifier.padding(horizontal = 14.dp, vertical = 8.dp),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Text(
                    text = "房间码",
                    color = RoomTextMuted,
                    fontSize = 11.sp,
                    fontWeight = FontWeight.SemiBold
                )
                Text(
                    text = roomCode,
                    color = RoomText,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.Bold
                )
                Text(
                    text = "点击复制",
                    color = RoomAccent.copy(alpha = 0.9f),
                    fontSize = 10.sp,
                    fontWeight = FontWeight.SemiBold
                )
            }
        }
    }
}

@Composable
private fun TheaterStatusPanel(
    hostUserId: String,
    viewerUserId: String,
    activeUserId: String?,
    isHostController: Boolean,
    playbackSpeed: Float,
    videoQualityLabel: String,
    isJoinedToRoom: Boolean,
    syncStatus: SyncStatus,
    latestSeq: Long?
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = RoomPanel,
        shape = RoundedCornerShape(26.dp),
        border = BorderStroke(1.dp, RoomOutline)
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(
                        text = "房间同步",
                        color = RoomText,
                        fontSize = 17.sp,
                        fontWeight = FontWeight.Black
                    )
                    Text(
                        text = if (isHostController) {
                            "你的播放器操作会成为房间权威状态。"
                        } else {
                            "当前设备正在跟随房主状态。"
                        },
                        color = RoomTextMuted,
                        fontSize = 12.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
                LiveStatusPill(
                    label = if (isJoinedToRoom) "在线" else "待加入",
                    active = isJoinedToRoom
                )
            }

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                CompactMetric(
                    label = "房主",
                    value = if (isHostController) "你" else shortUser(hostUserId),
                    modifier = Modifier.weight(1f)
                )
                CompactMetric(
                    label = "成员",
                    value = if (activeUserId == null) "等待" else shortUser(viewerUserId),
                    modifier = Modifier.weight(1f)
                )
                CompactMetric(
                    label = "倍速",
                    value = "${playbackSpeed}x",
                    modifier = Modifier.weight(1f),
                    accent = RoomAccent
                )
            }

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                StatusChip(if (isHostController) "主控中" else "跟随中")
                StatusChip(videoQualityLabel)
                StatusChip("seq ${latestSeq ?: "-"}")
                StatusChip(syncStatus.label)
            }
        }
    }
}

@Composable
private fun LiveStatusPill(
    label: String,
    active: Boolean
) {
    Surface(
        color = if (active) Color(0x227DFFB0) else Color(0x22FFC36F),
        shape = RoundedCornerShape(999.dp),
        border = BorderStroke(1.dp, if (active) Color(0x667DFFB0) else Color(0x66FFC36F))
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 7.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Box(
                modifier = Modifier
                    .size(7.dp)
                    .clip(CircleShape)
                    .background(if (active) Color(0xFF7DFFB0) else Color(0xFFFFC36F))
            )
            Text(
                text = label,
                color = RoomText,
                fontSize = 12.sp,
                fontWeight = FontWeight.Bold
            )
        }
    }
}

@Composable
private fun CompactMetric(
    label: String,
    value: String,
    modifier: Modifier = Modifier,
    accent: Color = RoomPrimary
) {
    Surface(
        modifier = modifier,
        color = RoomPanelSoft,
        shape = RoundedCornerShape(18.dp),
        border = BorderStroke(1.dp, RoomOutline)
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 11.dp, vertical = 9.dp),
            verticalArrangement = Arrangement.spacedBy(3.dp)
        ) {
            Text(text = label, color = RoomTextMuted, fontSize = 11.sp)
            Text(
                text = value,
                color = accent,
                fontSize = 15.sp,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }
    }
}

@Composable
private fun StatusChip(label: String) {
    Surface(
        color = Color(0x18FFFFFF),
        shape = RoundedCornerShape(999.dp)
    ) {
        Text(
            text = label,
            color = RoomText,
            fontSize = 12.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp)
        )
    }
}

@Composable
private fun SessionQuickActions(
    currentRoomId: String?,
    syncStatus: SyncStatus,
    joinRoomInput: String,
    onJoinRoomInputChange: (String) -> Unit,
    onCreateAndJoinAsHost: () -> Unit,
    onJoinAsViewer: () -> Unit,
    onRejoinCurrentUser: () -> Unit,
    canRejoinCurrentUser: Boolean
) {
    var expanded by rememberSaveable { mutableStateOf(false) }

    Surface(
        color = Color(0x0FFFFFFF),
        shape = RoundedCornerShape(22.dp),
        border = BorderStroke(1.dp, RoomOutline)
    ) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clickable { expanded = !expanded },
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column {
                    Text(
                        text = "开发联调入口",
                        color = RoomText,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.Bold
                    )
                    Text(
                        text = "当前房间：${currentRoomId ?: "未创建"} · ${syncStatus.label}",
                        color = RoomTextMuted,
                        fontSize = 12.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
                Text(
                    text = if (expanded) "收起" else "展开",
                    color = RoomPrimary,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Bold
                )
            }

            if (expanded) {
                OutlinedTextField(
                    value = joinRoomInput,
                    onValueChange = onJoinRoomInputChange,
                    modifier = Modifier.fillMaxWidth(),
                    label = { Text("Room ID") },
                    singleLine = true,
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedTextColor = RoomText,
                        unfocusedTextColor = RoomText,
                        focusedLabelColor = RoomPrimary,
                        unfocusedLabelColor = RoomTextMuted,
                        focusedBorderColor = RoomPrimary,
                        unfocusedBorderColor = RoomOutline,
                        focusedContainerColor = Color(0x10FFFFFF),
                        unfocusedContainerColor = Color(0x10FFFFFF)
                    )
                )
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(
                        onClick = onCreateAndJoinAsHost,
                        modifier = Modifier.fillMaxWidth(),
                        colors = ButtonDefaults.buttonColors(containerColor = RoomPrimary)
                    ) {
                        Text("Create + Join as host", color = RoomText, fontWeight = FontWeight.Bold)
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        OutlinedButton(
                            onClick = onJoinAsViewer,
                            enabled = joinRoomInput.isNotBlank(),
                            modifier = Modifier.weight(1f)
                        ) {
                            Text("Join viewer")
                        }
                        OutlinedButton(
                            onClick = onRejoinCurrentUser,
                            enabled = canRejoinCurrentUser,
                            modifier = Modifier.weight(1f)
                        ) {
                            Text("Rejoin")
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun RoomPlayerBackdrop() {
    Box(modifier = Modifier.fillMaxSize()) {
        Box(
            modifier = Modifier
                .align(Alignment.TopStart)
                .size(230.dp)
                .background(RoomBackdropGlowA)
        )
        Box(
            modifier = Modifier
                .align(Alignment.TopEnd)
                .padding(top = 80.dp)
                .size(240.dp)
                .background(RoomBackdropGlowB)
        )
        Box(
            modifier = Modifier
                .align(Alignment.BottomCenter)
                .size(300.dp)
                .background(RoomBackdropGlowC)
        )
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .fillMaxHeight()
                .background(
                    Brush.verticalGradient(
                        colors = listOf(Color.Transparent, RoomBackground.copy(alpha = 0.98f))
                    )
                )
        )
    }
}

private fun shortUser(userId: String): String {
    return userId.substringAfter("android_").replace('_', ' ').take(10)
}

@Preview(showBackground = true, widthDp = 390, heightDp = 844)
@Composable
private fun RoomTheaterPagePreview() {
    Watch_togetherTheme {
        RoomTheaterPage(
            uiState = RoomPlayerUiState(
                joinRoomInput = "A7K2M9",
                activeUserId = "android_host_xi",
                currentRoomId = "A7K2M9",
                syncStatus = SyncStatus.Connected,
                player = PlayerRuntimeUiState(
                    currentPosition = 9 * 60 * 1000L + 24 * 1000L,
                    duration = 24 * 60 * 1000L + 18 * 1000L,
                    isPlaying = true,
                    playbackState = androidx.media3.common.Player.STATE_READY,
                    playbackSpeed = 1.25f
                )
            ),
            adapter = PreviewPlayerAdapter,
            hostUserId = "android_host_xi",
            viewerUserId = "android_viewer_yuki",
            mediaTitle = "紫罗兰永恒花园",
            mediaEpisodeLabel = "第 09 集",
            isHostController = true,
            onPlaybackToggleClick = {},
            onSeekBackwardClick = {},
            onSeekForwardClick = {},
            onProgressSeekCommit = {},
            onPlaybackSpeedChange = {},
            onJoinRoomInputChange = {},
            onCreateAndJoinAsHost = {},
            onJoinAsViewer = {},
            onRejoinCurrentUser = {}
        )
    }
}

private object PreviewPlayerAdapter : PlayerAdapter {
    override fun attach(playerView: PlayerView) = Unit
    override fun detach() = Unit
    override fun setEventListener(listener: ((PlayerEvent) -> Unit)?) = Unit
    override fun load(url: String) = Unit
    override fun play() = Unit
    override fun pause() = Unit
    override fun seekTo(positionMs: Long) = Unit
    override fun getCurrentPosition(): Long = 0L
    override fun getDuration(): Long = 0L
    override fun isPlaying(): Boolean = false
    override fun setPlaybackSpeed(speed: Float) = Unit
    override fun release() = Unit
}
