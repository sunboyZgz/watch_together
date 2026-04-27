package com.example.watch_together.auth

data class AuthSession(
    val user: AuthUser,
    val accessToken: String
)

data class AuthUser(
    val id: String,
    val account: String,
    val nickname: String,
    val avatarSeed: String,
    val avatarUrl: String?
)
