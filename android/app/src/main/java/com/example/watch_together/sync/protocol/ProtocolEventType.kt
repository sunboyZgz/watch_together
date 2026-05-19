package com.example.watch_together.sync.protocol

enum class ProtocolEventType(
    val wireName: String,
    val direction: ProtocolDirection
) {
    JoinRoom(
        wireName = "join_room",
        direction = ProtocolDirection.ClientToServer
    ),
    LeaveRoom(
        wireName = "leave_room",
        direction = ProtocolDirection.ClientToServer
    ),
    RoomState(
        wireName = "room_state",
        direction = ProtocolDirection.ServerToClient
    ),
    RoomMembersChanged(
        wireName = "room_members_changed",
        direction = ProtocolDirection.ServerToClient
    ),
    RoomDeviceWaiting(
        wireName = "room_device.waiting",
        direction = ProtocolDirection.ServerToClient
    ),
    RoomDeviceSwitchRequest(
        wireName = "room_device.switch_request",
        direction = ProtocolDirection.ServerToClient
    ),
    RoomDeviceSwitchReply(
        wireName = "room_device.switch_reply",
        direction = ProtocolDirection.ClientToServer
    ),
    RoomDeviceSwitchResult(
        wireName = "room_device.switch_result",
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
    SetPlaybackRate(
        wireName = "set_playback_rate",
        direction = ProtocolDirection.ClientToServerAndServerToClients
    ),
    Ended(
        wireName = "ended",
        direction = ProtocolDirection.ClientToServerAndServerToClients
    ),
    Heartbeat(
        wireName = "heartbeat",
        direction = ProtocolDirection.ServerToClient
    ),
    HeartbeatAck(
        wireName = "heartbeat_ack",
        direction = ProtocolDirection.ClientToServer
    ),
    Error(
        wireName = "error",
        direction = ProtocolDirection.ServerToClient
    )
}
