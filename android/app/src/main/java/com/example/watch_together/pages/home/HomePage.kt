package com.example.watch_together.pages.home

import android.annotation.SuppressLint
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.example.watch_together.ui.theme.Watch_togetherTheme

private val HomeBackground = Color(0xFF171A31)
private val HomeCard = Color(0xFF2A2F4C)
private val HomeCardMuted = Color(0xFF303654)
private val HomePrimary = Color(0xFFE675BC)
private val HomeTextPrimary = Color(0xFFF8F2FB)
private val HomeTextSecondary = Color(0xC9D8D0E6)
private val HomeChip = Color(0xFF7DCEF1)
private val HomeOutlineButton = Color(0xFF2A304A)
private val HomeOutlineStroke = Color(0x1FFFFFFF)

private val HomeGlowA = Brush.radialGradient(
    colors = listOf(Color(0xFF4A4798), Color(0x004A4798))
)
private val HomeGlowB = Brush.radialGradient(
    colors = listOf(Color(0xFF71456E), Color(0x0071456E))
)
private val HomeGlowC = Brush.radialGradient(
    colors = listOf(Color(0xFF3D5A74), Color(0x003D5A74))
)

private data class HomeUserProfile(
    val account: String,
    val nickname: String,
    val initials: String,
    val bio: String
)

private data class ContinueWatchItem(
    val title: String,
    val progress: String,
    val coverBrush: Brush
)

private enum class HomeFeatureDialogKind {
    Profile,
    CreateRoom,
    ResumeWatch
}

@SuppressLint("UnusedBoxWithConstraintsScope")
@Composable
fun HomePage(
    sessionAccount: String,
    modifier: Modifier = Modifier
) {
    val profile = remember(sessionAccount) { buildProfile(sessionAccount) }
    val continueItems = remember {
        listOf(
            ContinueWatchItem(
                title = "孤独摇滚！",
                progress = "上次看到第 06 集",
                coverBrush = Brush.linearGradient(
                    listOf(Color(0xFF5D4CA4), Color(0xFF4F4A8D))
                )
            ),
            ContinueWatchItem(
                title = "请和我结婚",
                progress = "上次看到第 03 集",
                coverBrush = Brush.linearGradient(
                    listOf(Color(0xFF456B87), Color(0xFF3B5E77))
                )
            )
        )
    }

    var joinRoomCode by rememberSaveable { mutableStateOf("") }
    var isJoinDialogVisible by rememberSaveable { mutableStateOf(false) }
    var activeFeatureDialog by rememberSaveable { mutableStateOf<HomeFeatureDialogKind?>(null) }
    var activeResumeTitle by rememberSaveable { mutableStateOf("") }

    BoxWithConstraints(
        modifier = modifier
            .fillMaxSize()
            .background(HomeBackground)
    ) {
        val compactWidth = maxWidth < 380.dp
        val compactHeight = maxHeight < 760.dp
        val pagePadding = if (compactWidth) 18.dp else 28.dp
        val topSpacing = if (compactHeight) 20.dp else 30.dp
        val sectionSpacing = if (compactHeight) 16.dp else 20.dp

        HomeBackdrop()

        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = pagePadding, vertical = topSpacing),
            verticalArrangement = Arrangement.spacedBy(sectionSpacing)
        ) {
            HomeGreetingHeader(
                profile = profile,
                compactWidth = compactWidth,
                onAvatarClick = { activeFeatureDialog = HomeFeatureDialogKind.Profile }
            )

            LastWatchCard(
                compactWidth = compactWidth,
                compactHeight = compactHeight,
                onClick = {
                    activeResumeTitle = "紫罗兰永恒花园"
                    activeFeatureDialog = HomeFeatureDialogKind.ResumeWatch
                }
            )

            ActionButtonsRow(
                compactWidth = compactWidth,
                onCreateRoomClick = { activeFeatureDialog = HomeFeatureDialogKind.CreateRoom },
                onJoinRoomClick = { isJoinDialogVisible = true }
            )

            Text(
                text = "继续追番",
                style = if (compactWidth) {
                    MaterialTheme.typography.headlineSmall
                } else {
                    MaterialTheme.typography.headlineMedium
                }.copy(
                    color = HomeTextPrimary,
                    fontWeight = FontWeight.Bold
                )
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(if (compactWidth) 12.dp else 16.dp)
            ) {
                continueItems.forEach { item ->
                    ContinueWatchCard(
                        item = item,
                        modifier = Modifier.weight(1f),
                        compactHeight = compactHeight,
                        onClick = {
                            activeResumeTitle = item.title
                            activeFeatureDialog = HomeFeatureDialogKind.ResumeWatch
                        }
                    )
                }
            }

            SharedRoomInfoCard(compactWidth = compactWidth)

            Spacer(modifier = Modifier.height(8.dp))
        }

        if (isJoinDialogVisible) {
            HomeOverlayScrim {
                JoinRoomDialog(
                    roomCode = joinRoomCode,
                    onRoomCodeChange = { value ->
                        joinRoomCode = value
                            .uppercase()
                            .filter { it.isLetterOrDigit() }
                            .take(6)
                    },
                    onDismiss = { isJoinDialogVisible = false },
                    onConfirm = {
                        isJoinDialogVisible = false
                        activeFeatureDialog = HomeFeatureDialogKind.ResumeWatch
                        activeResumeTitle = "房间 ${joinRoomCode.ifBlank { "------" }}"
                    },
                    modifier = Modifier.padding(horizontal = pagePadding)
                )
            }
        }

        activeFeatureDialog?.let { dialogKind ->
            HomeOverlayScrim {
                FeatureHintDialog(
                    title = when (dialogKind) {
                        HomeFeatureDialogKind.Profile -> "个人中心即将接入"
                        HomeFeatureDialogKind.CreateRoom -> "选片页即将接入"
                        HomeFeatureDialogKind.ResumeWatch -> "继续观看流程待接入"
                    },
                    body = when (dialogKind) {
                        HomeFeatureDialogKind.Profile ->
                            "右上角头像入口已经预留。下一步会接入个人中心页面，用来管理头像、昵称和资料信息。"

                        HomeFeatureDialogKind.CreateRoom ->
                            "创建放映室下一步会跳到 `02A 选择视频`，当前先把首页页面和交互节奏收稳。"

                        HomeFeatureDialogKind.ResumeWatch ->
                            "`${activeResumeTitle}` 的恢复播放流程还在接入中。后续会基于 user_media_progress 直接恢复到上次看到的秒级进度。"
                    },
                    actionLabel = "知道了",
                    onDismiss = { activeFeatureDialog = null },
                    modifier = Modifier.padding(horizontal = pagePadding)
                )
            }
        }
    }
}

@Composable
private fun HomeGreetingHeader(
    profile: HomeUserProfile,
    compactWidth: Boolean,
    onAvatarClick: () -> Unit
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(16.dp),
        verticalAlignment = Alignment.Top
    ) {
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = "晚安，${profile.nickname}",
                style = if (compactWidth) {
                    MaterialTheme.typography.headlineMedium
                } else {
                    MaterialTheme.typography.displaySmall
                }.copy(
                    color = HomeTextPrimary,
                    fontWeight = FontWeight.Bold
                )
            )
            Text(
                text = "今晚想和谁一起补番？",
                style = MaterialTheme.typography.bodyLarge.copy(
                    color = HomeTextSecondary
                )
            )
        }

        Box(
            modifier = Modifier
                .size(if (compactWidth) 48.dp else 56.dp)
                .clip(CircleShape)
                .background(Color(0xFFF3CFE5))
                .clickable(onClick = onAvatarClick),
            contentAlignment = Alignment.Center
        ) {
            Text(
                text = profile.initials,
                style = MaterialTheme.typography.titleMedium.copy(
                    color = Color(0xFF6A3A67),
                    fontWeight = FontWeight.Bold
                )
            )
        }
    }
}

@Composable
private fun LastWatchCard(
    compactWidth: Boolean,
    compactHeight: Boolean,
    onClick: () -> Unit
) {
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
        shape = RoundedCornerShape(28.dp),
        color = HomeCard,
        border = BorderStroke(1.dp, HomeOutlineStroke),
        shadowElevation = 8.dp
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(
                    horizontal = if (compactWidth) 16.dp else 18.dp,
                    vertical = if (compactWidth) 16.dp else 18.dp
                ),
            horizontalArrangement = Arrangement.spacedBy(if (compactWidth) 12.dp else 16.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Column(
                modifier = Modifier
                    .weight(1f),
                verticalArrangement = Arrangement.spacedBy(if (compactWidth) 12.dp else 14.dp)
            ) {
                Surface(
                    shape = RoundedCornerShape(999.dp),
                    color = HomePrimary
                ) {
                    Text(
                        text = "上次观看",
                        style = MaterialTheme.typography.labelLarge.copy(
                            color = Color.White,
                            fontWeight = FontWeight.SemiBold
                        ),
                        modifier = Modifier.padding(horizontal = 14.dp, vertical = 8.dp)
                    )
                }

                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text = "紫罗兰永恒花园",
                        style = if (compactWidth) {
                            MaterialTheme.typography.headlineSmall
                        } else {
                            MaterialTheme.typography.headlineMedium
                        }.copy(
                            color = HomeTextPrimary,
                            fontWeight = FontWeight.Bold
                        ),
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis
                    )
                    Text(
                        text = "和搭子一起继续看到第 09 集，房间会自动同步进度与倍速。",
                        style = if (compactWidth) {
                            MaterialTheme.typography.bodyMedium
                        } else {
                            MaterialTheme.typography.bodyLarge
                        }.copy(
                            color = HomeTextSecondary
                        ),
                        maxLines = if (compactWidth) 3 else 4,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }

            Box(
                modifier = Modifier
                    .widthIn(
                        min = if (compactWidth) 92.dp else 118.dp,
                        max = if (compactWidth) 104.dp else 144.dp
                    )
                    .aspectRatio(if (compactHeight) 0.82f else 0.9f)
                    .clip(RoundedCornerShape(24.dp))
                    .background(
                        Brush.linearGradient(
                            listOf(Color(0xFF5762A5), Color(0xFF445584))
                        )
                    )
                    .alpha(0.9f)
            )
        }
    }
}

@Composable
private fun ActionButtonsRow(
    compactWidth: Boolean,
    onCreateRoomClick: () -> Unit,
    onJoinRoomClick: () -> Unit
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        Button(
            onClick = onCreateRoomClick,
            modifier = Modifier.weight(1f),
            shape = RoundedCornerShape(26.dp),
            colors = ButtonDefaults.buttonColors(
                containerColor = HomePrimary,
                contentColor = Color.White
            )
        ) {
            Text(
                text = "创建放映室",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.SemiBold
                ),
                textAlign = TextAlign.Center,
                modifier = Modifier.padding(vertical = if (compactWidth) 6.dp else 8.dp)
            )
        }

        Surface(
            modifier = Modifier
                .weight(1f)
                .clickable(onClick = onJoinRoomClick),
            shape = RoundedCornerShape(26.dp),
            color = HomeOutlineButton,
            border = BorderStroke(1.dp, HomeOutlineStroke)
        ) {
            Text(
                text = "加入房间",
                style = MaterialTheme.typography.titleMedium.copy(
                    color = HomeTextPrimary,
                    fontWeight = FontWeight.SemiBold
                ),
                textAlign = TextAlign.Center,
                modifier = Modifier.padding(vertical = if (compactWidth) 18.dp else 20.dp)
            )
        }
    }
}

@Composable
private fun ContinueWatchCard(
    item: ContinueWatchItem,
    modifier: Modifier = Modifier,
    compactHeight: Boolean,
    onClick: () -> Unit
) {
    Surface(
        modifier = modifier.clickable(onClick = onClick),
        shape = RoundedCornerShape(26.dp),
        color = HomeCardMuted,
        border = BorderStroke(1.dp, HomeOutlineStroke)
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .aspectRatio(if (compactHeight) 0.96f else 1.02f)
                    .clip(RoundedCornerShape(20.dp))
                    .background(item.coverBrush)
            )
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    text = item.title,
                    style = MaterialTheme.typography.titleLarge.copy(
                        color = HomeTextPrimary,
                        fontWeight = FontWeight.Bold
                    )
                )
                Text(
                    text = item.progress,
                    style = MaterialTheme.typography.bodyMedium.copy(
                        color = HomeTextSecondary
                    )
                )
            }
        }
    }
}

@Composable
private fun SharedRoomInfoCard(compactWidth: Boolean) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(28.dp),
        color = Color(0xFF32405A),
        border = BorderStroke(1.dp, HomeOutlineStroke)
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 18.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = "·共享放映室",
                    style = MaterialTheme.typography.titleLarge.copy(
                        color = HomeTextPrimary,
                        fontWeight = FontWeight.Bold
                    )
                )
                Surface(
                    shape = RoundedCornerShape(999.dp),
                    color = HomeChip
                ) {
                    Text(
                        text = "房间码 6 位",
                        style = MaterialTheme.typography.labelLarge.copy(
                            color = Color(0xFF173042),
                            fontWeight = FontWeight.SemiBold
                        ),
                        modifier = Modifier.padding(horizontal = 14.dp, vertical = 8.dp)
                    )
                }
            }
            Text(
                text = if (compactWidth) {
                    "支持同一用户多设备加入、重复回房自动重同步，以及房主掉线后的即时房主切换。"
                } else {
                    "支持同一用户多设备加入、重复回房自动重同步以及房主掉线后的即时房主切换。"
                },
                style = MaterialTheme.typography.bodyLarge.copy(
                    color = HomeTextSecondary
                )
            )
        }
    }
}

@Composable
private fun JoinRoomDialog(
    roomCode: String,
    onRoomCodeChange: (String) -> Unit,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    Surface(
        modifier = modifier.widthIn(max = 420.dp),
        shape = RoundedCornerShape(28.dp),
        color = Color(0xF4252744),
        border = BorderStroke(1.dp, HomeOutlineStroke),
        shadowElevation = 24.dp
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 20.dp, vertical = 22.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = "加入房间",
                    style = MaterialTheme.typography.headlineSmall.copy(
                        color = HomeTextPrimary,
                        fontWeight = FontWeight.Bold
                    )
                )
                Text(
                    text = "×",
                    style = MaterialTheme.typography.titleMedium.copy(
                        color = HomeTextSecondary,
                        fontWeight = FontWeight.SemiBold
                    ),
                    modifier = Modifier
                        .clip(CircleShape)
                        .clickable(onClick = onDismiss)
                        .padding(horizontal = 8.dp, vertical = 2.dp)
                )
            }

            Text(
                text = "输入 6 位房间码，快速加入朋友的放映室。",
                style = MaterialTheme.typography.bodyMedium.copy(color = HomeTextSecondary)
            )

            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(
                    text = "房间码",
                    style = MaterialTheme.typography.labelMedium.copy(
                        color = HomeTextPrimary,
                        fontWeight = FontWeight.SemiBold
                    )
                )
                OutlinedTextField(
                    value = roomCode,
                    onValueChange = onRoomCodeChange,
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                    placeholder = {
                        Text("例如 A1B2C3", color = HomeTextSecondary)
                    },
                    keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Characters),
                    shape = RoundedCornerShape(18.dp),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedContainerColor = Color(0x1FFFFFFF),
                        unfocusedContainerColor = Color(0x1FFFFFFF),
                        focusedBorderColor = Color(0x33FFFFFF),
                        unfocusedBorderColor = HomeOutlineStroke,
                        focusedTextColor = HomeTextPrimary,
                        unfocusedTextColor = HomeTextPrimary,
                        cursorColor = HomeTextPrimary
                    )
                )
                Text(
                    text = "支持大写字母和数字",
                    style = MaterialTheme.typography.labelMedium.copy(color = HomeTextSecondary)
                )
            }

            Button(
                onClick = onConfirm,
                enabled = roomCode.length == 6,
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(26.dp),
                colors = ButtonDefaults.buttonColors(
                    containerColor = HomePrimary,
                    disabledContainerColor = Color(0x667A6A82),
                    contentColor = Color.White,
                    disabledContentColor = Color(0x80FFF8FF)
                )
            ) {
                Text(
                    text = "加入房间",
                    style = MaterialTheme.typography.titleMedium.copy(
                        fontWeight = FontWeight.SemiBold
                    ),
                    modifier = Modifier.padding(vertical = 6.dp)
                )
            }
        }
    }
}

@Composable
private fun FeatureHintDialog(
    title: String,
    body: String,
    actionLabel: String,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    Surface(
        modifier = modifier.widthIn(max = 420.dp),
        shape = RoundedCornerShape(28.dp),
        color = Color(0xF4252744),
        border = BorderStroke(1.dp, HomeOutlineStroke),
        shadowElevation = 24.dp
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 20.dp, vertical = 22.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Text(
                text = title,
                style = MaterialTheme.typography.headlineSmall.copy(
                    color = HomeTextPrimary,
                    fontWeight = FontWeight.Bold
                )
            )
            Text(
                text = body,
                style = MaterialTheme.typography.bodyMedium.copy(
                    color = HomeTextSecondary
                )
            )
            Button(
                onClick = onDismiss,
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(26.dp),
                colors = ButtonDefaults.buttonColors(
                    containerColor = HomePrimary,
                    contentColor = Color.White
                )
            ) {
                Text(
                    text = actionLabel,
                    style = MaterialTheme.typography.titleMedium.copy(
                        fontWeight = FontWeight.SemiBold
                    ),
                    modifier = Modifier.padding(vertical = 6.dp)
                )
            }
        }
    }
}

@Composable
private fun HomeOverlayScrim(content: @Composable BoxScope.() -> Unit) {
    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color(0x8A0A0B18)),
        contentAlignment = Alignment.Center,
        content = content
    )
}

@Composable
private fun HomeBackdrop() {
    Box(modifier = Modifier.fillMaxSize()) {
        Box(
            modifier = Modifier
                .size(260.dp)
                .align(Alignment.TopStart)
                .clip(CircleShape)
                .background(HomeGlowA)
                .alpha(0.32f)
        )
        Box(
            modifier = Modifier
                .size(240.dp)
                .align(Alignment.TopEnd)
                .padding(top = 84.dp)
                .clip(CircleShape)
                .background(HomeGlowB)
                .alpha(0.30f)
        )
        Box(
            modifier = Modifier
                .size(280.dp)
                .align(Alignment.BottomEnd)
                .padding(bottom = 18.dp)
                .clip(CircleShape)
                .background(HomeGlowC)
                .alpha(0.28f)
        )
        repeat(5) { index ->
            Box(
                modifier = Modifier
                    .size(if (index % 2 == 0) 4.dp else 6.dp)
                    .clip(CircleShape)
                    .background(Color(0x80D6CCE5))
                    .align(
                        when (index) {
                            0 -> Alignment.TopStart
                            1 -> Alignment.TopCenter
                            2 -> Alignment.CenterEnd
                            3 -> Alignment.CenterStart
                            else -> Alignment.BottomEnd
                        }
                    )
                    .padding(
                        start = if (index == 0) 34.dp else 0.dp,
                        top = if (index <= 1) 92.dp else 0.dp,
                        end = if (index == 2 || index == 4) 36.dp else 0.dp,
                        bottom = if (index == 4) 92.dp else 0.dp
                    )
            )
        }
    }
}

private fun buildProfile(account: String): HomeUserProfile {
    val localPart = account.substringBefore('@').trim()
    val displayName = localPart
        .ifBlank { "星野" }
        .replaceFirstChar { char -> char.titlecase() }
        .take(10)
    val initials = displayName.take(2).uppercase()
    return HomeUserProfile(
        account = account,
        nickname = displayName,
        initials = initials,
        bio = "想把每一个夜晚都变成刚刚好的放映时间。"
    )
}

@Preview(showBackground = true, widthDp = 390, heightDp = 844)
@Composable
private fun HomePagePreview() {
    Watch_togetherTheme {
        HomePage(sessionAccount = "xingye@example.com")
    }
}
