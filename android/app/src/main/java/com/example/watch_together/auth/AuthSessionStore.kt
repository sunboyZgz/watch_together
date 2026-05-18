package com.example.watch_together.auth

import android.content.Context

interface AuthSessionStore {
    fun load(): AuthSession?
    fun save(session: AuthSession)
    fun clear()
}

class SharedPreferencesAuthSessionStore(
    context: Context
) : AuthSessionStore {
    private val preferences = context.applicationContext.getSharedPreferences(
        "watch_together_auth_session",
        Context.MODE_PRIVATE
    )

    override fun load(): AuthSession? {
        val userId = preferences.getString(KEY_USER_ID, null)?.takeIf { it.isNotBlank() } ?: return null
        val account = preferences.getString(KEY_ACCOUNT, null)?.takeIf { it.isNotBlank() } ?: return null
        val nickname = preferences.getString(KEY_NICKNAME, null)?.takeIf { it.isNotBlank() } ?: return null
        val avatarSeed = preferences.getString(KEY_AVATAR_SEED, null)?.takeIf { it.isNotBlank() } ?: account
        val accessToken = preferences.getString(KEY_ACCESS_TOKEN, null)?.takeIf { it.isNotBlank() } ?: return null
        val avatarUrl = preferences.getString(KEY_AVATAR_URL, null)?.takeIf { it.isNotBlank() }

        return AuthSession(
            user = AuthUser(
                id = userId,
                account = account,
                nickname = nickname,
                avatarSeed = avatarSeed,
                avatarUrl = avatarUrl
            ),
            accessToken = accessToken
        )
    }

    override fun save(session: AuthSession) {
        preferences.edit()
            .putString(KEY_USER_ID, session.user.id)
            .putString(KEY_ACCOUNT, session.user.account)
            .putString(KEY_NICKNAME, session.user.nickname)
            .putString(KEY_AVATAR_SEED, session.user.avatarSeed)
            .putString(KEY_AVATAR_URL, session.user.avatarUrl.orEmpty())
            .putString(KEY_ACCESS_TOKEN, session.accessToken)
            .apply()
    }

    override fun clear() {
        preferences.edit().clear().apply()
    }

    private companion object {
        const val KEY_USER_ID = "user_id"
        const val KEY_ACCOUNT = "account"
        const val KEY_NICKNAME = "nickname"
        const val KEY_AVATAR_SEED = "avatar_seed"
        const val KEY_AVATAR_URL = "avatar_url"
        const val KEY_ACCESS_TOKEN = "access_token"
    }
}
