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
host transfer
membership-triggered state refresh
```

Payload:

```json
{
  "roomId": "ROOM01",
  "mediaId": "sample_001",
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

Later protocol migration may replace accepted-control broadcasts with `room_state`, but this branch keeps legacy event types stable.

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
| `room_state` | server -> client |
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
