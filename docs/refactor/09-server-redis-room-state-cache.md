# Server Redis Room State Cache

> Purpose: document the first sync-adjacent Redis feature after the in-process timeline authority became stable.

## Boundary

Redis is a cache, not timeline authority.
`room_state` is a protocol snapshot, not the timeline authority model.
The authority model remains the server-created timeline vector carried inside `room.State`.

The authoritative transition order remains:

```text
room runtime accepts and serializes control intent
room runtime creates the authoritative timeline vector
room runtime returns room.State with room/media binding metadata
transport broadcasts the accepted control result or room_state snapshot
transport writes best-effort latest room_state cache
```

If Redis is unavailable or the cache write fails, the room transition still succeeds.

## Current Cache

Key:

```text
wt:room:state:{roomId}:v1
```

Value:

```text
protocol.RoomStatePayload JSON
```

TTL:

```text
10 minutes
```

## Write Points

The WebSocket transport writes the latest room state after:

```text
join_room snapshot
accepted play / pause / seek / set_playback_rate / ended transition
host disconnect state refresh
host reconnect state refresh
other membership-triggered room_state refreshes
```

## Non-Goals

Do not use this cache as:

```text
the source of host authority
the source of playback control permission
the only source of room membership
durable room history
```

## Future Work

This cache can later support:

```text
debugging latest room timeline state
faster reconnect hints before in-process room lookup
multi-instance fan-out validation
Redis pub/sub or stream message payload reuse
```
