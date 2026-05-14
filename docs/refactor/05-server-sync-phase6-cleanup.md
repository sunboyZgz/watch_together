# Server Sync Phase 6 Cleanup

> Purpose: track cleanup decisions after the generic timeline vector, clock sync, and compatibility payloads are in place.

## Compatibility View Rule

The room timeline authority is still:

```text
positionMs
velocity
serverTimeMs
seq
reason
```

The external `paused` field remains for Android/Web compatibility, but transport payloads derive it from velocity:

```text
paused = velocity == 0
```

This prevents `paused` from becoming a second source of truth when room state is converted into:

```text
WebSocket room_state
HTTP create-room roomState
```

## Current Cleanup Status

```text
[x] Added centralized realtime clock boundary.
[x] Clock sync pong uses the shared clock boundary.
[x] Heartbeat liveness timing uses the shared clock boundary.
[x] Clock sync has protocol decoder tests.
[x] WebSocket clock sync test uses deterministic server time.
[x] Transport compatibility payloads derive paused from velocity.
[x] HTTP and WebSocket room state responses share `newRoomSyncView`.
[x] Current WebSocket sync contract is documented in `06-websocket-sync-contract.md`.
```

## Remaining Cleanup Candidates

```text
[ ] Consider removing Paused from room.State after Android/Web migration.
[ ] Move playbackRate fully to intended-rate metadata once client contract is stable.
[ ] Make room_state the canonical accepted-control broadcast after legacy event migration.
[ ] Add stale seq/request-id rejection or deduplication if clients send request ids.
[ ] Add Redis-backed optional latest-vector cache only after runtime authority is stable.
[ ] Add room-scoped broadcaster and per-client outbound queues for high-concurrency fan-out.
```

## Do Not Do Yet

These changes are intentionally postponed because they alter client behavior or runtime architecture:

```text
Do not remove legacy play/pause/seek/set_playback_rate/ended broadcasts until Android/Web migrate to room_state.
Do not reject stale seq yet; clients currently send seq as compatibility metadata, not a strict precondition.
Do not move latest room vectors into Redis as authority; Redis may cache or fan out later, but room runtime remains authoritative.
Do not remove Paused from room.State until all callers use velocity-derived compatibility views.
```

## Agent Rule

When adding future response fields, keep this direction:

```text
realtime vector -> room binding metadata -> transport compatibility view
```

Do not push transport compatibility fields back into the generic realtime vector.
