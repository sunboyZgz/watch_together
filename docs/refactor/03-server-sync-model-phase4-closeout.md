# Server Sync Model Phase 4 Closeout

> Purpose: document the Phase 4 sync model boundary after extracting the reusable timeline vector.

## Current Shape

The low-level realtime package owns only generic timing math:

```text
TimelineVector
  positionMs
  velocity
  serverTimeMs
  seq
  reason
  bounds optional
```

It does not know about:

```text
roomCode
mediaId
hostUserId
ended
playbackRate compatibility metadata
WebSocket payload shape
```

Those belong to `server/internal/room` and `server/internal/transport`.

## Runtime Authority

The room layer binds the generic vector to the current product context:

```text
room id
media id
host user id
active connections
ended compatibility state
intended playback rate
```

Accepted controls create a new server vector. Client `positionMs` is not authoritative except for seek-like controls where it is the requested target.

## Control Semantics

```text
play:
  derive current server position
  set velocity to intended playback rate
  ignore client position hint

pause:
  derive current server position
  set velocity to 0
  ignore client position hint

seek:
  use requested target position
  preserve current velocity

set_playback_rate:
  derive current server position
  update intended playbackRate
  if paused, keep vector velocity at 0

ended:
  room/media policy wraps generic StopAt with reason media_end
```

## Broadcast Boundary

Phase 4 keeps legacy control event types for compatibility:

```text
play
pause
seek
set_playback_rate
ended
```

The authoritative fields now come from the room state helper:

```text
positionMs
velocity
serverTimeMs
reason
seq
```

`room_state` remains the canonical snapshot for join, reconnect, and membership changes. A later protocol migration can make every accepted control broadcast `room_state` directly.

## High-Concurrency Broadcast Direction

The current broadcast implementation uses a bounded worker pool instead of spawning one goroutine per client. This is a stepping stone, not the final multi-instance design.

Future broadcast work should consider:

```text
room-scoped broadcaster interface
per-connection outbound queues
backpressure and slow-client policy
bounded write deadlines
Redis pub/sub or streams for multi-instance fan-out
metrics for queue depth and broadcast latency
```

Keep WebSocket connection objects in process memory. Do not put connections in Redis.

## Phase 4 Done Criteria

```text
[x] Realtime vector is generic and reusable.
[x] Room/media binding lives outside realtime.
[x] Client play/pause position is not authority.
[x] Pause derives server position before freezing.
[x] Seek preserves vector velocity.
[x] Paused rate change updates intended playbackRate without unpausing.
[x] Legacy control payloads include velocity, serverTimeMs, reason, seq.
[x] Broadcast implementation has a bounded concurrency boundary.
```
