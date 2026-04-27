package com.example.watch_together.pages.login

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp

private val ModalBackground = Color(0xF4252744)
private val ModalStroke = Color(0x26FFFFFF)
private val ModalTextPrimary = Color(0xFFF9F3FB)
private val ModalTextSecondary = Color(0xCCCFCAE4)
private val ModalButton = Color(0xFFE675BC)
private val ModalButtonDisabled = Color(0x667A6A82)
private val ModalField = Color(0x1FFFFFFF)
private val ModalFieldBorder = Color(0x24FFFFFF)
private val AccentGlow = Brush.radialGradient(
    colors = listOf(
        Color(0x59C475EA),
        Color(0x00C475EA)
    )
)

@Composable
fun LoginDialog(
    account: String,
    password: String,
    onAccountChange: (String) -> Unit,
    onPasswordChange: (String) -> Unit,
    onConfirmClick: () -> Unit,
    onDismissClick: () -> Unit,
    onRegisterClick: () -> Unit,
    isLoading: Boolean = false,
    errorMessage: String? = null,
    modifier: Modifier = Modifier
) {
    Surface(
        modifier = modifier.widthIn(max = 420.dp),
        shape = RoundedCornerShape(28.dp),
        color = ModalBackground,
        border = BorderStroke(1.dp, ModalStroke),
        tonalElevation = 0.dp,
        shadowElevation = 24.dp
    ) {
        Box {
            Box(
                modifier = Modifier
                    .align(Alignment.TopEnd)
                    .padding(top = 8.dp, end = 8.dp)
                    .size(140.dp)
                    .clip(CircleShape)
                    .background(AccentGlow)
            )

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
                        text = "轻量登录",
                        style = MaterialTheme.typography.labelMedium.copy(
                            fontWeight = FontWeight.SemiBold,
                            color = Color(0xFFF6C7E6)
                        )
                    )

                    Box(
                        modifier = Modifier
                            .size(28.dp)
                            .clip(CircleShape)
                            .background(Color(0x12FFFFFF))
                            .clickable(enabled = !isLoading, onClick = onDismissClick),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = "×",
                            style = MaterialTheme.typography.titleMedium.copy(
                                color = Color(0xB8F8F2FF),
                                fontWeight = FontWeight.SemiBold
                            )
                        )
                    }
                }

                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text = "账号登录",
                        style = MaterialTheme.typography.headlineSmall.copy(
                            color = ModalTextPrimary,
                            fontWeight = FontWeight.Bold
                        )
                    )
                    Text(
                        text = "使用账号与密码登录后继续进入我的放映室，昵称将在注册阶段设置。",
                        style = MaterialTheme.typography.bodyMedium.copy(
                            color = ModalTextSecondary
                        )
                    )
                }

                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text(
                        text = "账号",
                        style = MaterialTheme.typography.labelMedium.copy(
                            color = Color(0xE5E9DFFF),
                            fontWeight = FontWeight.Medium
                        )
                    )
                    OutlinedTextField(
                        value = account,
                        onValueChange = onAccountChange,
                        enabled = !isLoading,
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                        placeholder = {
                            Text(
                                text = "请输入账号",
                                color = Color(0x90F8F2FF)
                            )
                        },
                        shape = RoundedCornerShape(18.dp),
                        colors = OutlinedTextFieldDefaults.colors(
                            focusedContainerColor = ModalField,
                            unfocusedContainerColor = ModalField,
                            disabledContainerColor = ModalField,
                            focusedBorderColor = Color(0x33FFFFFF),
                            unfocusedBorderColor = ModalFieldBorder,
                            cursorColor = Color(0xFFF9F3FB),
                            focusedTextColor = ModalTextPrimary,
                            unfocusedTextColor = ModalTextPrimary
                        )
                    )
                    Text(
                        text = "支持用户名或邮箱地址",
                        style = MaterialTheme.typography.labelMedium.copy(
                            color = Color(0xBCA8B4D2)
                        )
                    )
                }

                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text(
                        text = "密码",
                        style = MaterialTheme.typography.labelMedium.copy(
                            color = Color(0xE5E9DFFF),
                            fontWeight = FontWeight.Medium
                        )
                    )
                    OutlinedTextField(
                        value = password,
                        onValueChange = onPasswordChange,
                        enabled = !isLoading,
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                        visualTransformation = PasswordVisualTransformation(),
                        placeholder = {
                            Text(
                                text = "请输入密码",
                                color = Color(0x90F8F2FF)
                            )
                        },
                        shape = RoundedCornerShape(18.dp),
                        colors = OutlinedTextFieldDefaults.colors(
                            focusedContainerColor = ModalField,
                            unfocusedContainerColor = ModalField,
                            disabledContainerColor = ModalField,
                            focusedBorderColor = Color(0x33FFFFFF),
                            unfocusedBorderColor = ModalFieldBorder,
                            cursorColor = Color(0xFFF9F3FB),
                            focusedTextColor = ModalTextPrimary,
                            unfocusedTextColor = ModalTextPrimary
                        )
                    )
                    Text(
                        text = "至少 8 位字符，区分大小写",
                        style = MaterialTheme.typography.labelMedium.copy(
                            color = Color(0xBCA8B4D2)
                        )
                    )
                }

                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    if (!errorMessage.isNullOrBlank()) {
                        Text(
                            text = errorMessage,
                            style = MaterialTheme.typography.bodyMedium.copy(
                                color = Color(0xFFFFB4C8),
                                fontWeight = FontWeight.Medium
                            )
                        )
                    }

                    Button(
                        onClick = onConfirmClick,
                        enabled = !isLoading && account.trim().isNotEmpty() && password.isNotBlank(),
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(26.dp),
                        colors = ButtonDefaults.buttonColors(
                            containerColor = ModalButton,
                            disabledContainerColor = ModalButtonDisabled,
                            contentColor = Color(0xFFFDF8FF),
                            disabledContentColor = Color(0x80FFF8FF)
                        )
                    ) {
                        if (isLoading) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(22.dp),
                                strokeWidth = 2.dp,
                                color = Color(0xFFFDF8FF)
                            )
                        } else {
                            Text(
                                text = "登录并继续",
                                style = MaterialTheme.typography.titleMedium.copy(
                                    fontWeight = FontWeight.SemiBold
                                ),
                                modifier = Modifier.padding(vertical = 6.dp)
                            )
                        }
                    }

                }
            }
        }
    }
}
