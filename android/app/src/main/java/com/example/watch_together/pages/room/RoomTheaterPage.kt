package com.example.watch_together.pages.room

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.media3.common.Player
import androidx.media3.ui.PlayerView
import com.example.watch_together.R
import com.example.watch_together.sync.RoomMember
import com.example.watch_together.ui.player.PlayerAdapter
import com.example.watch_together.ui.player.PlayerScreen
import com.example.watch_together.ui.player.PlayerEvent
import com.example.watch_together.ui.player.PlayerRuntimeState
import com.example.watch_together.ui.player.PlayerVideoQualityOption
import com.example.watch_together.ui.player.PlayerVideoQualityPreference
import com.example.watch_together.ui.theme.Watch_togetherTheme

private val RoomBackground = Color(0xFF080D22)
private val RoomCard = Color(0xB81B2541)
private val RoomCardStrong = Color(0xD61C2744)
private val RoomPrimary = Color(0xFFFF76C8)
private val RoomPrimarySoft = Color(0xFFFFA0DD)
private val RoomPurple = Color(0xFFA875FF)
private val RoomAccent = Color(0xFF92E8FF)
private val RoomSuccess = Color(0xFF5EF09B)
private val RoomText = Color(0xFFF9F3FF)
private val RoomTextMuted = Color(0xC8D7D1E5)
private val RoomTextDim = Color(0x8FD7D1E5)
private val RoomOutline = Color(0x26FFFFFF)

private val RoomPrimaryGradient = Brush.horizontalGradient(
    listOf(RoomPrimary, RoomPurple)
)
@Composable
internal fun RoomTheaterPage(
    playerState: PlayerRuntimeState,
    adapter: PlayerAdapter,
    roomCode: String,
    roomRole: String?,
    roomStatusLabel: String,
    roomMembers: List<RoomMember>,
    mediaTitle: String,
    mediaSeasonLabel: String?,
    mediaEpisodeLabel: String?,
    isHostController: Boolean,
    controlsEnabled: Boolean,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onProgressSeekCommit: (Long) -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit,
    onInviteClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val clipboard = LocalClipboardManager.current

    BoxWithConstraints(
        modifier = modifier
            .fillMaxSize()
            .background(RoomBackground)
    ) {
        val compactWidth = maxWidth < 380.dp
        val compactHeight = maxHeight < 760.dp
        val horizontalPadding = if (compactWidth) 16.dp else 18.dp
        val sectionGap = if (compactHeight) 14.dp else 16.dp
        val displayRoomCode = roomCode.take(6).uppercase().ifBlank { "A7K2M9" }
        val hasActiveRoom = roomCode.isNotBlank()
        val onlineCount = roomMembers.size.coerceAtLeast(if (hasActiveRoom) 1 else 2)
        val displayTitle = mediaTitle.takeUnless { it.isBlank() || it == "等待选择影片" } ?: "CLANNAD After Story"
        val currentEpisodeNumber = episodeNumberFromLabel(mediaEpisodeLabel) ?: 14
        val currentEpisodeLabel = "EP %02d".format(currentEpisodeNumber)
        val displayPosition = playerState.currentPosition.takeIf { it > 0L } ?: (17 * 60 + 32) * 1000L

        RoomNightBackdrop()

        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = horizontalPadding)
                .padding(top = if (compactHeight) 18.dp else 28.dp, bottom = 24.dp),
            verticalArrangement = Arrangement.spacedBy(sectionGap)
        ) {
            RoomHeader(
                roomCode = displayRoomCode,
                onlineCount = onlineCount,
                roomMembers = roomMembers,
                onShareClick = { clipboard.setText(AnnotatedString(displayRoomCode)) }
            )

            RoomPlayerCard(
                adapter = adapter,
                state = playerState,
                title = displayTitle,
                seasonLabel = mediaSeasonLabel,
                episodeLabel = currentEpisodeLabel,
                controlHint = roomStatusLabel,
                isHostController = isHostController,
                onlineCount = onlineCount,
                roomRole = roomRole,
                controlsEnabled = controlsEnabled,
                onProgressSeekCommit = onProgressSeekCommit,
                onPlaybackToggleClick = onPlaybackToggleClick,
                onSeekBackwardClick = onSeekBackwardClick,
                onSeekForwardClick = onSeekForwardClick,
                onPlaybackSpeedChange = onPlaybackSpeedChange,
                onVideoQualityPreferenceChange = onVideoQualityPreferenceChange
            )

            WorkInfoCard(
                title = displayTitle,
                currentEpisodeNumber = currentEpisodeNumber,
                compactWidth = compactWidth
            )

            EpisodeSwitcherCard(
                selectedEpisode = currentEpisodeNumber,
                episodeCount = 24
            )

            BottomActionBar(
                hasRoom = hasActiveRoom,
                currentPosition = displayPosition,
                isPlaying = playerState.isPlaying,
                onInviteClick = {
                    if (hasActiveRoom) clipboard.setText(AnnotatedString(displayRoomCode))
                    onInviteClick()
                },
                onContinueClick = onPlaybackToggleClick
            )
        }
    }
}

@Deprecated("Legacy player_default entry is archived; active app uses the ui.player RoomTheaterPage overload.")
@Composable
internal fun RoomTheaterPage(
    uiState: com.example.watch_together.ui.player_default.RoomPlayerUiState,
    adapter: com.example.watch_together.ui.player_default.PlayerAdapter,
    hostUserId: String,
    mediaTitle: String,
    mediaEpisodeLabel: String?,
    isHostController: Boolean,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onProgressSeekCommit: (Long) -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (com.example.watch_together.ui.player_default.PlayerVideoQualityPreference) -> Unit,
    onJoinRoomInputChange: (String) -> Unit,
    onCreateAndJoinAsHost: () -> Unit,
    onJoinAsViewer: () -> Unit,
    onRejoinCurrentUser: () -> Unit,
    modifier: Modifier = Modifier
) {
    Box(
        modifier = modifier
            .fillMaxSize()
            .background(RoomBackground)
            .padding(18.dp),
        contentAlignment = Alignment.Center
    ) {
        Surface(
            color = RoomCard,
            shape = RoundedCornerShape(24.dp),
            border = BorderStroke(1.dp, RoomOutline)
        ) {
            Column(
                modifier = Modifier.padding(18.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Text(
                    text = "旧播放器页面已归档",
                    color = RoomText,
                    fontSize = 20.sp,
                    fontWeight = FontWeight.Black
                )
                Text(
                    text = "当前 App 入口已切到新版 ui.player 播放器；legacy player_default 仅保留为可编译参考。",
                    color = RoomTextMuted,
                    fontSize = 13.sp,
                    lineHeight = 18.sp
                )
            }
        }
    }
}

@Composable
private fun RoomHeader(
    roomCode: String,
    onlineCount: Int,
    roomMembers: List<RoomMember>,
    onShareClick: () -> Unit
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.Top
    ) {
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = "星夜放映室",
                    color = RoomText,
                    fontWeight = FontWeight.Black,
                    fontSize = 34.sp,
                    lineHeight = 38.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                Text(
                    text = " ✦",
                    color = Color(0xFFC995FF),
                    fontSize = 24.sp,
                    fontWeight = FontWeight.Black
                )
            }
            Text(
                text = "一起看，才更有星空的味道 ✦",
                color = RoomTextMuted,
                fontSize = 15.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }

        Column(
            horizontalAlignment = Alignment.End,
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(9.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                RoomCodeChip(roomCode = roomCode)
                GlassIconButton(iconRes = R.drawable.share_upload, onClick = onShareClick)
            }
            OnlineMemberPreview(
                onlineCount = onlineCount,
                roomMembers = roomMembers
            )
        }
    }
}

@Composable
private fun RoomCodeChip(roomCode: String) {
    Surface(
        color = Color(0x1CFFFFFF),
        shape = RoundedCornerShape(999.dp),
        border = BorderStroke(1.dp, RoomOutline)
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 14.dp, vertical = 9.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text("房间码", color = RoomTextMuted, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            Text(roomCode, color = RoomText, fontSize = 15.sp, fontWeight = FontWeight.Bold)
        }
    }
}

@Composable
private fun GlassIconButton(
    iconRes: Int,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Surface(
        modifier = modifier.size(48.dp),
        color = Color(0x22FFFFFF),
        shape = RoundedCornerShape(18.dp),
        border = BorderStroke(1.dp, RoomOutline),
        onClick = onClick
    ) {
        Box(contentAlignment = Alignment.Center) {
            Icon(
                painter = painterResource(iconRes),
                contentDescription = null,
                tint = Color.Unspecified,
                modifier = Modifier.size(25.dp)
            )
        }
    }
}

@Composable
private fun OnlineMemberPreview(
    onlineCount: Int,
    roomMembers: List<RoomMember>
) {
    Row(
        modifier = Modifier.clickable { },
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Box(modifier = Modifier.width(66.dp).height(34.dp)) {
            RoomAvatar(
                label = roomMembers.getOrNull(0)?.nickname?.firstOrNull()?.toString() ?: "朋",
                gradient = Brush.linearGradient(listOf(Color(0xFF84D9FF), Color(0xFF6D7BFF))),
                modifier = Modifier.align(Alignment.CenterStart)
            )
            RoomAvatar(
                label = roomMembers.getOrNull(1)?.nickname?.firstOrNull()?.toString() ?: "渚",
                gradient = Brush.linearGradient(listOf(Color(0xFFFF89CB), Color(0xFFFFB27A))),
                modifier = Modifier.align(Alignment.CenterStart).offset(x = 28.dp)
            )
        }
        Text(
            text = "$onlineCount 人在线",
            color = RoomText,
            fontSize = 15.sp,
            fontWeight = FontWeight.SemiBold
        )
        Text(text = "›", color = RoomText, fontSize = 28.sp, lineHeight = 28.sp)
    }
}

@Composable
private fun RoomAvatar(
    label: String,
    gradient: Brush,
    modifier: Modifier = Modifier
) {
    Box(
        modifier = modifier
            .size(34.dp)
            .clip(CircleShape)
            .background(gradient),
        contentAlignment = Alignment.Center
    ) {
        Text(label, color = RoomText, fontSize = 13.sp, fontWeight = FontWeight.Black)
        Box(
            modifier = Modifier
                .align(Alignment.BottomEnd)
                .size(9.dp)
                .clip(CircleShape)
                .background(RoomSuccess)
        )
    }
}

@Composable
private fun RoomPlayerCard(
    adapter: PlayerAdapter,
    state: PlayerRuntimeState,
    title: String,
    seasonLabel: String?,
    episodeLabel: String,
    controlHint: String,
    isHostController: Boolean,
    onlineCount: Int,
    roomRole: String?,
    controlsEnabled: Boolean,
    onProgressSeekCommit: (Long) -> Unit,
    onPlaybackToggleClick: () -> Unit,
    onSeekBackwardClick: () -> Unit,
    onSeekForwardClick: () -> Unit,
    onPlaybackSpeedChange: (Float) -> Unit,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = RoomCard,
        shape = RoundedCornerShape(28.dp),
        border = BorderStroke(1.dp, RoomOutline)
    ) {
        Column(
            modifier = Modifier.padding(10.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            PlayerScreen(
                adapter = adapter,
                state = state,
                mediaTitle = title,
                mediaMeta = listOfNotNull(seasonLabel, episodeLabel)
                    .joinToString(" · ")
                    .ifBlank { "正在一起看到 ${formatMs(state.currentPosition)}" },
                controlHint = controlHint,
                controlsEnabled = controlsEnabled,
                onPlaybackToggleClick = onPlaybackToggleClick,
                onSeekBackwardClick = onSeekBackwardClick,
                onSeekForwardClick = onSeekForwardClick,
                onProgressSeekCommit = onProgressSeekCommit,
                onPlaybackSpeedChange = onPlaybackSpeedChange,
                onVideoQualityPreferenceChange = onVideoQualityPreferenceChange,
                modifier = Modifier.fillMaxWidth()
            )

            RoomActionStrip(
                onlineCount = onlineCount,
                isHostController = isHostController,
                qualityLabel = state.videoVariant.displayLabel.ifBlank { state.videoQualityPreference.label },
                roleLabel = roomRole,
                availableVideoQualities = state.availableVideoQualities,
                videoQualityPreference = state.videoQualityPreference,
                onVideoQualityPreferenceChange = onVideoQualityPreferenceChange
            )
        }
    }
}

@Composable
private fun RoomActionStrip(
    onlineCount: Int,
    isHostController: Boolean,
    qualityLabel: String,
    roleLabel: String?,
    availableVideoQualities: List<PlayerVideoQualityOption>,
    videoQualityPreference: PlayerVideoQualityPreference,
    onVideoQualityPreferenceChange: (PlayerVideoQualityPreference) -> Unit
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = Color(0x20FFFFFF),
        shape = RoundedCornerShape(22.dp),
        border = BorderStroke(1.dp, RoomOutline)
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 9.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            RoomActionItem(
                iconRes = R.drawable.participants,
                text = "$onlineCount 人在线",
                modifier = Modifier.weight(1f)
            )
            RoomDivider()
            RoomActionItem(
                iconRes = R.drawable.crown,
                text = when {
                    isHostController -> "主控中"
                    roleLabel.isNullOrBlank() -> "待加入"
                    else -> "跟随中"
                },
                modifier = Modifier.weight(1f)
            )
            RoomDivider()
            RoomActionItem(
                iconRes = R.drawable.hd_quality,
                text = qualityLabel.ifBlank { "切换清晰度" },
                modifier = Modifier.weight(1.15f),
                onClick = {
                    onVideoQualityPreferenceChange(nextQualityPreference(availableVideoQualities, videoQualityPreference))
                }
            )
            RoomDivider()
            RoomActionItem(
                iconRes = R.drawable.chat,
                text = "一起聊天",
                modifier = Modifier.weight(1f)
            )
        }
    }
}

@Composable
private fun RoomActionItem(
    iconRes: Int,
    text: String,
    modifier: Modifier = Modifier,
    onClick: (() -> Unit)? = null
) {
    Row(
        modifier = modifier
            .clip(RoundedCornerShape(999.dp))
            .clickable(enabled = onClick != null) { onClick?.invoke() }
            .padding(horizontal = 2.dp, vertical = 2.dp),
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Icon(
            painter = painterResource(iconRes),
            contentDescription = null,
            tint = Color.Unspecified,
            modifier = Modifier.size(18.dp)
        )
        Text(
            text = text,
            color = RoomText,
            fontSize = 10.5.sp,
            fontWeight = FontWeight.SemiBold,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

@Composable
private fun RoomDivider() {
    Box(
        modifier = Modifier
            .width(1.dp)
            .height(20.dp)
            .background(RoomOutline)
    )
}

@Composable
private fun WorkInfoCard(
    title: String,
    currentEpisodeNumber: Int,
    compactWidth: Boolean
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = RoomCardStrong,
        shape = RoundedCornerShape(24.dp),
        border = BorderStroke(1.dp, RoomOutline)
    ) {
        if (compactWidth) {
            Column(
                modifier = Modifier.padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp)
            ) {
                WorkPoster(modifier = Modifier.fillMaxWidth().height(160.dp))
                WorkCopy(title = title, currentEpisodeNumber = currentEpisodeNumber)
            }
        } else {
            Row(
                modifier = Modifier.padding(14.dp),
                horizontalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                WorkPoster(modifier = Modifier.width(116.dp).height(160.dp))
                WorkCopy(
                    title = title,
                    currentEpisodeNumber = currentEpisodeNumber,
                    modifier = Modifier.weight(1f)
                )
            }
        }
    }
}

@Composable
private fun WorkPoster(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .clip(RoundedCornerShape(14.dp))
            .background(Brush.verticalGradient(listOf(Color(0xFF7FB9FF), Color(0xFFFFA7DD), Color(0xFF222A58))))
    ) {
        Canvas(modifier = Modifier.fillMaxSize()) {
            drawCircle(Color(0x55FFFFFF), radius = size.width * 0.5f, center = androidx.compose.ui.geometry.Offset(size.width * 0.26f, size.height * 0.08f))
            drawCircle(Color(0x44FF76C8), radius = size.width * 0.42f, center = androidx.compose.ui.geometry.Offset(size.width * 0.82f, size.height * 0.23f))
            drawLine(Color(0xAAFFFFFF), androidx.compose.ui.geometry.Offset(size.width * 0.30f, size.height * 0.55f), androidx.compose.ui.geometry.Offset(size.width * 0.30f, size.height * 0.78f), 3.dp.toPx())
            drawLine(Color(0xAAFFFFFF), androidx.compose.ui.geometry.Offset(size.width * 0.64f, size.height * 0.49f), androidx.compose.ui.geometry.Offset(size.width * 0.64f, size.height * 0.78f), 3.dp.toPx())
        }
        Text(
            text = "CLANNAD",
            color = RoomText,
            fontSize = 22.sp,
            fontWeight = FontWeight.Light,
            letterSpacing = 2.sp,
            modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 22.dp)
        )
    }
}

@Composable
private fun WorkCopy(
    title: String,
    currentEpisodeNumber: Int,
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier,
        verticalArrangement = Arrangement.spacedBy(9.dp)
    ) {
        Text(
            text = title,
            color = RoomText,
            fontSize = 25.sp,
            lineHeight = 29.sp,
            fontWeight = FontWeight.Black,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis
        )
        Text(
            text = "第 2 季 / 共 24 集 / 治愈 · 校园 · 恋爱",
            color = RoomTextMuted,
            fontSize = 14.sp,
            lineHeight = 19.sp
        )
        Text(
            text = "冈崎朋也与古河渚在毕业后步入新的人生阶段，围绕家庭、成长与羁绊展开更加深刻的故事。温柔而克制的叙事，让平凡日常闪耀出动人的情感力量。",
            color = RoomText,
            fontSize = 15.sp,
            lineHeight = 25.sp,
            maxLines = 5,
            overflow = TextOverflow.Ellipsis
        )
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            GenreTag("治愈")
            GenreTag("校园", accent = RoomAccent)
            GenreTag("恋爱", accent = RoomPrimary)
        }
        Text(
            text = "当前播放 EP %02d".format(currentEpisodeNumber),
            color = RoomTextDim,
            fontSize = 12.sp,
            fontWeight = FontWeight.SemiBold
        )
    }
}

@Composable
private fun GenreTag(label: String, accent: Color = RoomPurple) {
    Surface(
        color = accent.copy(alpha = 0.14f),
        shape = RoundedCornerShape(8.dp),
        border = BorderStroke(1.dp, accent.copy(alpha = 0.55f))
    ) {
        Text(
            text = label,
            color = accent.copy(alpha = 0.95f),
            fontSize = 13.sp,
            fontWeight = FontWeight.Bold,
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 5.dp)
        )
    }
}

@Composable
private fun EpisodeSwitcherCard(
    selectedEpisode: Int,
    episodeCount: Int
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = RoomCardStrong,
        shape = RoundedCornerShape(24.dp),
        border = BorderStroke(1.dp, RoomOutline)
    ) {
        Column(
            modifier = Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(13.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Icon(
                        painter = painterResource(R.drawable.play),
                        contentDescription = null,
                        tint = Color.Unspecified,
                        modifier = Modifier.size(24.dp)
                    )
                    Text(
                        text = "本季全部剧集",
                        color = RoomText,
                        fontSize = 17.sp,
                        fontWeight = FontWeight.Black
                    )
                }
                Row(horizontalArrangement = Arrangement.spacedBy(5.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        text = "第 2 季（共 24 集）",
                        color = RoomTextMuted,
                        fontSize = 13.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                    Icon(
                        painter = painterResource(R.drawable.chevron_down),
                        contentDescription = null,
                        tint = Color.Unspecified,
                        modifier = Modifier.size(18.dp)
                    )
                }
            }

            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                (1..episodeCount).chunked(6).forEach { episodeRow ->
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        episodeRow.forEach { episodeNumber ->
                            EpisodeCell(
                                episodeNumber = episodeNumber,
                                selected = episodeNumber == selectedEpisode,
                                modifier = Modifier.weight(1f)
                            )
                        }
                        repeat(6 - episodeRow.size) {
                            Spacer(modifier = Modifier.weight(1f))
                        }
                    }
                }
            }
            Text(
                text = "✦  左右滑动查看更多剧集",
                color = RoomTextDim,
                fontSize = 12.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.align(Alignment.CenterHorizontally)
            )
        }
    }
}

@Composable
private fun EpisodeCell(
    episodeNumber: Int,
    selected: Boolean,
    modifier: Modifier = Modifier
) {
    val background = if (selected) {
        RoomPrimaryGradient
    } else {
        Brush.verticalGradient(listOf(Color(0x2EFFFFFF), Color(0x18FFFFFF)))
    }
    Surface(
        modifier = modifier
            .height(54.dp)
            .clickable { },
        color = Color.Transparent,
        shape = RoundedCornerShape(9.dp),
        border = BorderStroke(1.dp, if (selected) RoomPrimarySoft else RoomOutline)
    ) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(background),
            contentAlignment = Alignment.Center
        ) {
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Text("EP", color = RoomTextMuted, fontSize = 11.sp, fontWeight = FontWeight.SemiBold)
                Text(
                    text = "%02d".format(episodeNumber),
                    color = RoomText,
                    fontSize = 18.sp,
                    lineHeight = 20.sp,
                    fontWeight = FontWeight.Bold
                )
            }
        }
    }
}

@Composable
private fun BottomActionBar(
    hasRoom: Boolean,
    currentPosition: Long,
    isPlaying: Boolean,
    onInviteClick: () -> Unit,
    onContinueClick: () -> Unit
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Surface(
            modifier = Modifier.weight(0.95f).height(64.dp),
            color = Color(0x22FFFFFF),
            shape = RoundedCornerShape(32.dp),
            border = BorderStroke(1.dp, RoomOutline),
            onClick = onInviteClick
        ) {
            Row(
                modifier = Modifier.padding(horizontal = 12.dp),
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Icon(
                    painter = painterResource(R.drawable.invite_friend),
                    contentDescription = null,
                    tint = Color.Unspecified,
                    modifier = Modifier.size(24.dp)
                )
                Spacer(Modifier.width(6.dp))
                Text(
                    text = if (hasRoom) "邀请好友" else "创建房间",
                    color = RoomText,
                    fontSize = 12.5.sp,
                    fontWeight = FontWeight.Bold
                )
            }
        }

        Surface(
            modifier = Modifier.weight(2.45f).height(64.dp),
            color = Color.Transparent,
            shape = RoundedCornerShape(32.dp),
            onClick = onContinueClick
        ) {
            Row(
                modifier = Modifier
                    .fillMaxSize()
                    .background(RoomPrimaryGradient)
                    .padding(horizontal = 14.dp),
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Icon(
                    painter = painterResource(R.drawable.play),
                    contentDescription = null,
                    tint = Color.Unspecified,
                    modifier = Modifier.size(27.dp)
                )
                Spacer(Modifier.width(8.dp))
                Column(horizontalAlignment = Alignment.Start) {
                    Text(
                        text = if (isPlaying) "正在观看" else "继续观看",
                        color = RoomText,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.Black
                    )
                    Text(
                        text = "从 ${formatMs(currentPosition)} 继续播放",
                        color = Color(0xDFFFFFFF),
                        fontSize = 11.sp,
                        fontWeight = FontWeight.SemiBold
                    )
                }
            }
        }
    }
}

@Composable
private fun RoomNightBackdrop() {
    Box(modifier = Modifier.fillMaxSize()) {
        Box(
            modifier = Modifier
                .align(Alignment.TopStart)
                .offset(x = (-72).dp, y = (-44).dp)
                .size(260.dp)
                .background(Brush.radialGradient(listOf(Color(0x773E4BC7), Color.Transparent)))
        )
        Box(
            modifier = Modifier
                .align(Alignment.TopEnd)
                .offset(x = 96.dp, y = (-28).dp)
                .size(250.dp)
                .background(Brush.radialGradient(listOf(Color(0x775C285E), Color.Transparent)))
        )
        Box(
            modifier = Modifier
                .align(Alignment.Center)
                .offset(x = (-70).dp, y = (-80).dp)
                .size(300.dp)
                .background(Brush.radialGradient(listOf(Color(0x334C74A8), Color.Transparent)))
        )
        Canvas(modifier = Modifier.fillMaxSize()) {
            drawIntoCanvas { canvas ->
                val paint = android.graphics.Paint().apply {
                    color = android.graphics.Color.argb(150, 255, 255, 255)
                    textAlign = android.graphics.Paint.Align.CENTER
                    textSize = 18.sp.toPx()
                }
                canvas.nativeCanvas.drawText("✦", size.width * 0.78f, size.height * 0.035f, paint)
                canvas.nativeCanvas.drawText("✦", size.width * 0.43f, size.height * 0.095f, paint)
                canvas.nativeCanvas.drawText("✦", size.width * 0.36f, size.height * 0.225f, paint)
            }
            drawCircle(Color(0x88FFFFFF), radius = 1.5.dp.toPx(), center = androidx.compose.ui.geometry.Offset(size.width * 0.28f, size.height * 0.14f))
            drawCircle(Color(0x66FFFFFF), radius = 1.2.dp.toPx(), center = androidx.compose.ui.geometry.Offset(size.width * 0.64f, size.height * 0.19f))
            drawCircle(Color(0x66FFFFFF), radius = 1.4.dp.toPx(), center = androidx.compose.ui.geometry.Offset(size.width * 0.18f, size.height * 0.33f))
        }
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

private fun nextQualityPreference(
    availableVideoQualities: List<PlayerVideoQualityOption>,
    currentPreference: PlayerVideoQualityPreference
): PlayerVideoQualityPreference {
    val options = availableVideoQualities.ifEmpty { listOf(PlayerVideoQualityOption.Auto) }
    val currentIndex = options.indexOfFirst { option -> option.height == currentPreference.height }
    val nextOption = options[(currentIndex + 1).floorMod(options.size)]
    return PlayerVideoQualityPreference(nextOption.height)
}

private fun Int.floorMod(modulus: Int): Int {
    if (modulus <= 0) return 0
    return ((this % modulus) + modulus) % modulus
}

private fun episodeNumberFromLabel(label: String?): Int? {
    return label
        ?.let { Regex("\\d+").find(it)?.value?.toIntOrNull() }
        ?.coerceIn(1, 24)
}

private fun formatMs(value: Long): String {
    if (value <= 0L) return "00:00"
    val totalSeconds = value / 1_000L
    val minutes = totalSeconds / 60
    val seconds = totalSeconds % 60
    return "%02d:%02d".format(minutes, seconds)
}

private fun shortUser(userId: String): String {
    return userId.substringAfter("android_").replace('_', ' ').take(10)
}

internal fun memberDisplayName(
    member: RoomMember?,
    activeUserId: String?,
    fallbackUserId: String
): String {
    if (member == null) return shortUser(fallbackUserId)
    if (activeUserId != null && member.userId == activeUserId) return "你"
    return member.nickname.ifBlank { shortUser(member.userId) }
}

internal fun viewerSlotDisplayName(
    viewerMember: RoomMember?,
    activeUserId: String?
): String {
    return viewerMember?.let {
        memberDisplayName(
            member = it,
            activeUserId = activeUserId,
            fallbackUserId = it.userId
        )
    } ?: "待加入"
}

@Preview(showBackground = true, widthDp = 390, heightDp = 844)
@Composable
private fun RoomTheaterPagePreview() {
    Watch_togetherTheme {
        RoomTheaterPage(
            playerState = PlayerRuntimeState(
                currentPosition = 17 * 60 * 1000L + 32 * 1000L,
                duration = 24 * 60 * 1000L,
                isPlaying = true,
                playbackState = Player.STATE_READY,
                playbackSpeed = 1.25f
            ),
            adapter = PreviewPlayerAdapter,
            roomCode = "A7K2M9",
            roomRole = "host",
            roomStatusLabel = "房间 A7K2M9 · host · 同步已连接",
            roomMembers = listOf(
                RoomMember(
                    userId = "host_user",
                    nickname = "朋也",
                    avatarSeed = "host",
                    avatarUrl = null,
                    role = "host"
                ),
                RoomMember(
                    userId = "viewer_user",
                    nickname = "渚",
                    avatarSeed = "viewer",
                    avatarUrl = null,
                    role = "member"
                )
            ),
            mediaTitle = "CLANNAD After Story",
            mediaSeasonLabel = "第 2 季",
            mediaEpisodeLabel = "EP 14",
            isHostController = true,
            controlsEnabled = true,
            onPlaybackToggleClick = {},
            onSeekBackwardClick = {},
            onSeekForwardClick = {},
            onProgressSeekCommit = {},
            onPlaybackSpeedChange = {},
            onVideoQualityPreferenceChange = {},
            onInviteClick = {}
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
    override fun getBufferedPosition(): Long = 0L
    override fun getBufferedPercentage(): Int = 0
    override fun isPlaying(): Boolean = false
    override fun setPlaybackSpeed(speed: Float) = Unit
    override fun setVideoQualityPreference(preference: PlayerVideoQualityPreference) = Unit
    override fun release() = Unit
}
