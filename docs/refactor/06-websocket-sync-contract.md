# WebSocket Sync Contract

> Purpose: describe the current server sync protocol after the timeline-vector refactor.

## Envelope

All WebSocket messages use:

```json
{
  "type": "event_name",
  "payload": {}
}
```

## Authentication

WebSocket `/ws` requires:

```http
Authorization: Bearer <accessToken>
```

The server verifies the token during the WebSocket handshake and binds the authenticated user id to the connection. Payload `userId` is kept only for protocol compatibility and mismatch detection; it is not trusted for room membership, host authorization, or control authority.

`join_room` also requires the authenticated user to already be an active business member of the room. Clients must call `POST /rooms/{roomCode}/join` before WebSocket `join_room`. If membership is missing, the server returns an `error` envelope with `room membership required` and does not add the connection to the runtime room.

## Active Room Device

One account can log in on multiple devices, but only one WebSocket connection can be the active room device for that `userId`.

If a second device sends `join_room` while the user already has an active room device:

```text
the new device does not enter the room immediately
the new device receives room_device.waiting
the current active device receives room_device.switch_request
the current active device replies with room_device.switch_reply
only approve=true switches active room participation to the new device
reject, timeout, or active-device disconnect keeps the new device inactive
```

Host control authorization checks both:

```text
authenticated user is the host
the WebSocket connection is the current active room device
```

This is process-local in the current single-instance MVP. A future multi-instance deployment should move the active device lease to Redis or another shared coordination layer.

## Authority Model

The server owns the room timeline vector:

```text
positionMs
velocity
serverTimeMs
seq
reason
```

Clients send control intent. The server creates and broadcasts the authoritative result.

Client `positionMs` is a hint for `play` and `pause`. It is the requested target for `seek`.

## Canonical Snapshot

`room_state` is the canonical room snapshot for:

```text
join
reconnect
host disconnect / host reconnect
membership-triggered state refresh
explicit room_state.request resync
```

Payload:

```json
{
  "roomId": "ROOM01",
  "mediaId": "sample_001",
  "mediaDurationMs": 1458000,
  "hostUserId": "user_a",
  "paused": false,
  "ended": false,
  "positionMs": 120000,
  "velocity": 1.0,
  "serverTimeMs": 1710000000000,
  "reason": "play",
  "playbackRate": 1.0,
  "seq": 2
}
```

Compatibility fields:

```text
paused = velocity == 0
playbackRate = intended room playback rate
ended = room/media completion view
mediaDurationMs = optional media bound used by server timeline policy
```

## Explicit Resync

Clients can request the latest canonical snapshot without reconnecting:

```json
{
  "type": "room_state.request",
  "payload": {
    "roomId": "ROOM01",
    "userId": "user_a",
    "seq": 5
  }
}
```

Server behavior:

```text
the client must already be joined as the same roomId + userId
the server reads the latest in-process room.State
the server replies only to the requesting client with room_state
the server may log client seq vs server seq for diagnostics
the server does not read Redis and does not broadcast this response
```

`seq` in `room_state.request` is the client's last known server seq. It is diagnostic data and does not gate the response.

## Leave Room

Clients should send `leave_room` before intentionally leaving a room:

```json
{
  "type": "leave_room",
  "payload": {
    "roomId": "ROOM01",
    "userId": "user_a"
  }
}
```

Server behavior:

```text
the client must already be joined as the same roomId + userId
the server marks the persistent room_members row inactive when room storage is available
the server removes the runtime connection from the in-process room
if the room becomes empty, it is destroyed immediately instead of entering grace_period
if other clients remain, they receive room_members_changed with reason leave
if the leaving client is the host, remaining clients also receive room_state with reason host_left
transient network disconnects do not send leave_room and still use the 5 minute reconnect grace period
```

## Control Events

The server still emits legacy control event types for Android/Web compatibility:

```text
play
pause
seek
set_playback_rate
ended
```

Each accepted control broadcast includes the authoritative vector fields:

```text
positionMs
velocity
serverTimeMs
reason
seq
```

`set_playback_rate` also includes `playbackRate`.

Control requests may include optional `requestId`:

```json
{
  "type": "play",
  "payload": {
    "roomId": "ROOM01",
    "userId": "user_a",
    "requestId": "client-generated-id",
    "positionMs": 120000,
    "seq": 5
  }
}
```

Current server behavior:

```text
requestId is optional and old clients can omit it
accepted-control broadcasts echo requestId when provided
dedup is one-process short-TTL dedup by roomId + requestId
the in-process dedup set is sharded and bounded
if the local dedup set is saturated, the server favors availability and lets the control proceed
duplicates return the latest room_state to the requester instead of advancing seq again
cross-instance Redis dedup is deferred until multi-instance room authority is designed
```

Client `seq` on control requests is the expected authoritative server seq. The server accepts a control only when client `seq == current room_state.seq`; any mismatch returns the latest `room_state` to the requester, does not advance the timeline, does not broadcast a control event, and does not reserve `requestId`.

`seek` has an additional room-scoped frequency guard. If a seek arrives inside `WS_SEEK_MIN_INTERVAL_MS`, the server returns the latest `room_state` to the requester and does not advance seq or broadcast the seek.

Known server reasons in this branch:

```text
init
play
pause
seek
rate_change
media_end
media_change
host_left
host_rejoin
```

Later protocol migration may replace accepted-control broadcasts with `room_state`, but this branch keeps legacy event types stable.

## Replay And Next Episode

The server does not automatically decide replay vs next episode after `ended`.

Replay current media uses existing controls:

```text
seek(positionMs=0)
play
```

Next episode should use a future explicit media-change API:

```text
change current room media to next mediaId
server broadcasts a new room_state with reason media_change
host/client sends play if the product wants auto-start
```

This keeps UI/product policy on the client side and timeline authority on the server side.

## Host Availability

The server does not transfer host control when the current host disconnects.

Host disconnect behavior:

```text
HostUserID becomes empty
timeline velocity becomes 0
paused becomes true
seq increments
reason becomes host_left
remaining clients receive room_state
viewer control events continue to be rejected with only-host errors
```

Only the original room host can reclaim host control by reconnecting:

```text
HostUserID becomes the original host user id
seq increments
reason becomes host_rejoin
remaining clients receive room_state
```

Clients must not continue room playback while `hostUserId` is empty.

## Clock Sync

Client sends:

```json
{
  "type": "clock_sync.ping",
  "payload": {
    "clientSendMonoMs": 100000
  }
}
```

Server replies:

```json
{
  "type": "clock_sync.pong",
  "payload": {
    "serverTimeMs": 1710000000000,
    "clientSendMonoMs": 100000
  }
}
```

Clock sync does not:

```text
look up a room
touch DB or Redis
broadcast
increment seq
refresh heartbeat ack
```

## Heartbeat

Heartbeat is only for connection liveness.

Server sends:

```json
{
  "type": "heartbeat",
  "payload": {
    "serverTimeMs": 1710000000000
  }
}
```

Client replies:

```json
{
  "type": "heartbeat_ack",
  "payload": {
    "serverTimeMs": 1710000000000,
    "clientTimeMs": 1710000000123
  }
}
```

Heartbeat and clock sync are intentionally separate:

```text
heartbeat: connection health
clock_sync: server-time estimation
```

## Event Direction

| Event | Direction |
| --- | --- |
| `join_room` | client -> server |
| `leave_room` | client -> server |
| `room_state` | server -> client |
| `room_state.request` | client -> server |
| `room_device.waiting` | server -> pending client |
| `room_device.switch_request` | server -> current active client |
| `room_device.switch_reply` | current active client -> server |
| `room_device.switch_result` | server -> pending/current clients |
| `play` | client -> server, server -> clients |
| `pause` | client -> server, server -> clients |
| `seek` | client -> server, server -> clients |
| `set_playback_rate` | client -> server, server -> clients |
| `ended` | client -> server, server -> clients |
| `clock_sync.ping` | client -> server |
| `clock_sync.pong` | server -> client |
| `heartbeat` | server -> client |
| `heartbeat_ack` | client -> server |
| `room_members_changed` | server -> clients except joining client |
| `error` | server -> client |
