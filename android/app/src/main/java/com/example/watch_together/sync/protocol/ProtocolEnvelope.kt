package com.example.watch_together.sync.protocol

data class ProtocolEnvelope<T : ProtocolPayload>(
    val type: String,
    val payload: T
)

sealed interface ProtocolPayload
