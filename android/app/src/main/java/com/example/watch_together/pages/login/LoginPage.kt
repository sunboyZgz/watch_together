package com.example.watch_together.pages.login

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.example.watch_together.ui.theme.Watch_togetherTheme
import kotlin.random.Random

private val LoginPageBackground = Color(0xFF13162A)
private val LoginPagePrimary = Color(0xFFE675BC)
private val LoginPageTextPrimary = Color(0xFFF7F2FB)
private val LoginPageTextSecondary = Color(0xBFD0C8E1)
private val LoginPageChip = Color(0x1AFFFFFF)
private val LoginPageChipText = Color(0xC9E2D8F2)

private val BackgroundGlowA = Brush.radialGradient(
    colors = listOf(Color(0xFF44408D), Color(0x0044408D))
)
private val BackgroundGlowB = Brush.radialGradient(
    colors = listOf(Color(0xFF68406C), Color(0x0068406C))
)
private val BackgroundGlowC = Brush.radialGradient(
    colors = listOf(Color(0xFF344A67), Color(0x00344A67))
)

@Composable
fun LoginPage(
    onLoginConfirmed: (String) -> Unit,
    modifier: Modifier = Modifier
) {
    var isDialogVisible by rememberSaveable { mutableStateOf(false) }
    var nickname by rememberSaveable { mutableStateOf("星野一起看") }

    Box(
        modifier = modifier
            .fillMaxSize()
            .background(LoginPageBackground)
    ) {
        DreamyBackdrop()

        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 28.dp, vertical = 32.dp)
        ) {
            Spacer(modifier = Modifier.height(280.dp))

            Text(
                text = "一起钻进\n今夜的放映室",
                style = MaterialTheme.typography.displaySmall.copy(
                    color = LoginPageTextPrimary,
                    fontWeight = FontWeight.Bold
                )
            )

            Spacer(modifier = Modifier.height(18.dp))

            Text(
                text = "登录后即可继续创建或加入你的放映室",
                style = MaterialTheme.typography.bodyLarge.copy(
                    color = LoginPageTextSecondary
                )
            )

            Spacer(modifier = Modifier.height(76.dp))

            Button(
                onClick = { isDialogVisible = true },
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(32.dp),
                colors = ButtonDefaults.buttonColors(
                    containerColor = LoginPagePrimary,
                    contentColor = Color(0xFFFDF8FF)
                )
            ) {
                Text(
                    text = "登录",
                    style = MaterialTheme.typography.titleMedium.copy(
                        fontWeight = FontWeight.SemiBold
                    ),
                    modifier = Modifier.padding(vertical = 8.dp)
                )
            }

            Spacer(modifier = Modifier.weight(1f))

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                LoginFeaturePill("双人同步")
                LoginFeaturePill("追番陪伴")
                LoginFeaturePill("梦幻夜色")
            }

            Spacer(modifier = Modifier.height(18.dp))

            Text(
                text = "Tonight, stay in sync.",
                style = MaterialTheme.typography.bodyLarge.copy(
                    color = Color(0xAFA99BC6)
                ),
                modifier = Modifier.align(Alignment.CenterHorizontally)
            )

            Spacer(modifier = Modifier.height(18.dp))
        }

        if (isDialogVisible) {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(Color(0x8A0A0B18))
                    .clickable(enabled = true, onClick = {}),
                contentAlignment = Alignment.Center
            ) {
                LoginDialog(
                    nickname = nickname,
                    onNicknameChange = { nickname = it },
                    onRandomNicknameClick = {
                        nickname = generateRandomNickname()
                    },
                    onConfirmClick = {
                        onLoginConfirmed(nickname.trim())
                        isDialogVisible = false
                    },
                    onDismissClick = {
                        isDialogVisible = false
                    },
                    modifier = Modifier.padding(horizontal = 28.dp)
                )
            }
        }
    }
}

@Composable
private fun DreamyBackdrop() {
    Box(modifier = Modifier.fillMaxSize()) {
        Box(
            modifier = Modifier
                .size(260.dp)
                .align(Alignment.TopStart)
                .clip(CircleShape)
                .background(BackgroundGlowA)
                .alpha(0.95f)
        )
        Box(
            modifier = Modifier
                .size(230.dp)
                .align(Alignment.TopEnd)
                .padding(top = 84.dp, end = 6.dp)
                .clip(CircleShape)
                .background(BackgroundGlowB)
                .alpha(0.88f)
        )
        Box(
            modifier = Modifier
                .size(280.dp)
                .align(Alignment.BottomEnd)
                .padding(bottom = 4.dp)
                .clip(CircleShape)
                .background(BackgroundGlowC)
                .alpha(0.92f)
        )
        Box(
            modifier = Modifier
                .size(150.dp)
                .align(Alignment.TopCenter)
                .padding(top = 112.dp)
                .clip(CircleShape)
                .background(Color(0x18FFFFFF))
        )
        Box(
            modifier = Modifier
                .size(96.dp)
                .align(Alignment.TopCenter)
                .padding(top = 138.dp)
                .clip(CircleShape)
                .background(Color(0xFFD5CEDB))
                .alpha(0.92f)
        )
        Box(
            modifier = Modifier
                .size(18.dp)
                .align(Alignment.TopCenter)
                .padding(top = 156.dp, start = 68.dp)
                .clip(CircleShape)
                .background(Color(0xFFF090D0))
        )
        Box(
            modifier = Modifier
                .size(width = 120.dp, height = 24.dp)
                .align(Alignment.TopCenter)
                .padding(top = 12.dp)
                .clip(RoundedCornerShape(999.dp))
                .background(Color(0x1BFFFFFF))
        )
    }
}

@Composable
private fun LoginFeaturePill(label: String) {
    Surface(
        shape = RoundedCornerShape(999.dp),
        color = LoginPageChip
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelLarge.copy(
                color = LoginPageChipText,
                fontWeight = FontWeight.Medium
            ),
            modifier = Modifier.padding(horizontal = 18.dp, vertical = 10.dp),
            textAlign = TextAlign.Center
        )
    }
}

private fun generateRandomNickname(): String {
    val prefixes = listOf("星野", "月岛", "樱井", "朝雾", "千夏", "琥珀")
    val suffixes = listOf("一起看", "追番中", "看片会", "放映室", "深夜番", "同步中")
    return "${prefixes.random(Random.Default)}${suffixes.random(Random.Default)}"
}

@Preview(showBackground = true, widthDp = 390, heightDp = 844)
@Composable
private fun LoginPagePreview() {
    Watch_togetherTheme {
        LoginPage(onLoginConfirmed = {})
    }
}
