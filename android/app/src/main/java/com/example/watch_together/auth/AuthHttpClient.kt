package com.example.watch_together.auth

import com.example.watch_together.config.AppConfig
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

class AuthHttpClient(
    private val okHttpClient: OkHttpClient = OkHttpClient()
) {

    fun login(account: String, password: String): AuthSession {
        val requestBody = JSONObject()
            .put("account", account.trim())
            .put("password", password)
            .toString()
            .toRequestBody("application/json; charset=utf-8".toMediaType())

        val request = Request.Builder()
            .url(AppConfig.authLoginUrl())
            .post(requestBody)
            .build()

        okHttpClient.newCall(request).execute().use { response ->
            val responseBody = response.body?.string().orEmpty()
            if (!response.isSuccessful) {
                throw AuthRequestException(
                    message = errorMessageFrom(responseBody) ?: "登录失败，请稍后重试",
                    statusCode = response.code
                )
            }
            if (responseBody.isBlank()) {
                throw AuthRequestException("登录响应为空", response.code)
            }
            val data = JSONObject(responseBody).getJSONObject("data")
            val user = data.getJSONObject("user")
            return AuthSession(
                user = AuthUser(
                    id = user.getString("id"),
                    account = user.getString("account"),
                    nickname = user.getString("nickname"),
                    avatarSeed = user.getString("avatarSeed"),
                    avatarUrl = user.optString("avatarUrl").takeUnless { it.isBlank() || it == "null" }
                ),
                accessToken = data.getString("accessToken")
            )
        }
    }

    private fun errorMessageFrom(responseBody: String): String? {
        if (responseBody.isBlank()) return null
        return runCatching {
            JSONObject(responseBody)
                .getJSONObject("error")
                .optString("message")
                .takeIf { it.isNotBlank() }
        }.getOrNull()
    }
}

class AuthRequestException(
    override val message: String,
    val statusCode: Int
) : IllegalStateException(message)
