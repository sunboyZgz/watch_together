package com.example.watch_together.sync.protocol

enum class ProtocolEventType(
    val wireName: String,
    val direction: ProtocolDirection
) {
    JoinRoom(
        wireName = "join_room",
        direction = ProtocolDirection.ClientToServer
    ),
    RoomState(
        wireName = "room_state",
        direction = ProtocolDirection.ServerToClient
    ),
    Play(
        wireName = "play",
        direction = ProtocolDirection.ClientToServerAndServerToClients
    ),
    Pause(
        wireName = "pause",
        direction = ProtocolDirection.ClientToServerAndServerToClients
    ),
    Seek(
        wireName = "seek",
        direction = ProtocolDirection.ClientToServerAndServerToClients
    ),
    Error(
        wireName = "error",
        direction = ProtocolDirection.ServerToClient
    )
}
