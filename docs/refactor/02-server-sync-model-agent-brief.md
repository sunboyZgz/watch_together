# Server Sync Model Agent Brief

> Purpose: compact execution brief for agents implementing the new server synchronization model.

## One Sentence

Model every active watch room as one authoritative server timeline vector, and make clients follow that vector instead of synchronizing players directly.

## Current Problem

The current runtime state has these fields:

```text
Paused
Ended
PositionMs
PlaybackRate
Seq
authorityAt
```

This already approximates a timeline, but several old semantics remain risky:

```text
play and pause can accept client position as authority
paused and playbackRate are independent state fields
room_state does not include serverTimeMs
clients do not have a dedicated clock sync message
sync protocol is still event-shaped instead of vector-shaped
```

The refactor should preserve current business behavior while replacing these semantics with a timeline vector.

## Target Runtime State

Use a state shape equivalent to:

```go
type RoomTimelineState struct {
    RoomCode      string
    MediaID       string
    PositionMs    int64
    Velocity      float64
    ServerTimeMs  int64
    Seq           int64
    Reason        string
    HostUserID    string
    MediaDurationMs *int64
}
```

Exact type names may differ, but the model must include:

```text
positionMs
velocity
serverTimeMs
seq
reason
```

## Central Query Function

All transitions must use one shared current-position derivation function:

```text
elapsedMs = nowServerMs - state.serverTimeMs
currentPositionMs = state.positionMs + state.velocity * elapsedMs
```

Rules:

```text
if velocity == 0, current position is positionMs
if mediaDurationMs is known, clamp to [0, mediaDurationMs]
do not store constantly changing currentPositionMs
```

## Control Transition Rules

Every accepted control must:

```text
validate room
validate user and host/control permission
derive current authoritative position from the old vector
create a new vector
assign serverTimeMs from server clock
increment seq
store runtime state
broadcast authoritative room_state to all connected clients
include the requester in the broadcast
```

Specific transitions:

```text
play          -> derive current position, velocity = previous/intended rate or 1.0, reason = play
pause         -> derive current position, velocity = 0.0, reason = pause
seek          -> position = validated target, preserve current velocity unless explicit policy says otherwise, reason = seek
rate_change   -> derive current position, velocity = requested rate, reason = rate_change
media_change  -> mediaId changes, position = 0, velocity = 0.0, reason = media_change
ended         -> position = media end or clamped value, velocity = 0.0, reason = media_end
```

## Client Payload Semantics

During compatibility migration, clients may still send payloads containing:

```text
positionMs
playbackRate
seq
```

Treat these as:

```text
positionMs   -> hint or diagnostics, not authority
playbackRate -> requested velocity for rate change
seq          -> optional stale expectation, not the server's next seq
```

Do not let client wall-clock time or client player position define authoritative server timeline state.

## Protocol Direction

`room_state` should become the canonical broadcast for accepted timeline updates.

Target payload fields:

```text
roomId
mediaId
hostUserId
positionMs
velocity
serverTimeMs
seq
reason
paused derived compatibility field
playbackRate derived compatibility field
ended derived or explicit compatibility field
```

Existing event messages such as `play`, `pause`, `seek`, and `set_playback_rate` may remain during migration, but the authoritative result should be expressible as the latest `room_state`.

## Clock Sync

Add lightweight WebSocket support:

```text
clock_sync.ping
clock_sync.pong
```

The server pong must include:

```text
serverTimeMs
clientSendMonoMs
```

Do not perform database or Redis work before replying to clock sync.

## Redis Use In Sync Work

Redis is useful for sync-adjacent data, but it must not muddy authority.

Good Redis candidates:

```text
latest timeline vector cache for reconnect/debug
short-lived control request id deduplication
presence summaries
future room event pub/sub for multi-instance deployments
```

Avoid:

```text
storing websocket connections in Redis
treating Redis and Go memory as two competing authorities
using Redis cache as the only durable source of room membership or media binding
```

If Redis is used for latest timeline vector, the per-room runtime must still serialize transitions deterministically.

## Module Boundary

Recommended split:

```text
realtime/timeline_state.go       -> state structs and derived view helpers
realtime/timeline_transition.go  -> play/pause/seek/rate transition logic
realtime/clock.go                -> centralized server timestamp source
realtime/room_runtime.go         -> per-room serialization and clients snapshot
realtime/room_hub.go             -> room registry and lifecycle
realtime/sync_protocol.go        -> realtime DTO conversion helpers
realtime/clock_sync.go           -> clock sync handling
```

Keep service and transport boundaries clean:

```text
transport decodes protocol and writes responses
realtime owns authoritative timeline decisions
store owns PostgreSQL business persistence
cache owns Redis access
```

## Must Not

```text
do not broadcast current position every 200ms as the primary sync model
do not let a client player state become global truth
do not update position without serverTimeMs semantics
do not increment seq on rejected controls
do not skip requester during authoritative broadcast
do not mix buffering reports directly into timeline changes without policy
do not introduce new product features while refactoring
```

## Acceptance Checklist

Before marking sync refactor work complete:

```text
[ ] Room has one latest authoritative vector.
[ ] Vector includes positionMs, velocity, serverTimeMs, seq, reason.
[ ] Current position is derived through one shared function.
[ ] Play derives current position before setting velocity.
[ ] Pause derives current position before freezing.
[ ] Seek creates a new vector at target position.
[ ] Rate change derives current position before changing velocity.
[ ] Every accepted transition increments seq.
[ ] Rejected controls do not increment seq.
[ ] Late join receives latest vector.
[ ] Reconnect receives latest vector.
[ ] Requester follows server broadcast.
[ ] Client position is not authoritative.
[ ] Clock sync pong exists.
[ ] Tests cover transition math and seq behavior.
```

