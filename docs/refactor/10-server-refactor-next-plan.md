# Server Refactor Next Plan

> Purpose: define the next server refactor sequence after timeline authority, broadcast runtime, and the first Redis room_state cache boundary.

## Current Baseline

The server has already moved past the first timeline refactor milestones:

```text
Gin routing is in app assembly.
GORM is the PostgreSQL connection layer, while migrations remain SQL-first.
Redis infrastructure exists and is optional by default.
Room sync is modeled as an authoritative server timeline vector.
Clock sync is available over WebSocket.
Broadcast fan-out is behind a bounded transport boundary.
The first Redis-backed feature is best-effort latest room_state cache.
```

The next work should stabilize these boundaries before adding new product features.

## Authority Terminology

Use these terms consistently:

```text
authoritative model:
  realtime.TimelineVector

authoritative runtime binding:
  room.State

canonical protocol snapshot:
  protocol.RoomStatePayload / room_state

best-effort cache payload:
  Redis JSON copy of protocol.RoomStatePayload
```

`room_state` is not the authority model. It is the WebSocket snapshot that carries the latest authoritative timeline vector plus room/media compatibility fields.

Redis is not authority. Redis may cache the latest `room_state` snapshot, but the in-process room runtime still serializes accepted timeline transitions.

## Phase A: Build And Redis Boundary Stabilization

Goal: make the current Redis cache work boring and safe.

Status on 2026-05-16:

```text
[x] RoomStateCache no longer references RedisClient directly.
[x] RedisClient remains owned by internal/cache/redis.go.
[x] RoomStateCache depends on a narrow cache.JSONStore interface.
[x] WebSocket transport depends on a latestRoomStateWriter interface, not go-redis.
[x] REDIS_ADDR empty keeps Redis disabled and server assembly returns nil RedisClient.
[x] Cache write failures are isolated behind WebSocketHandler.cacheRoomState.
[x] Local Go tooling is available and targeted refactor packages pass tests.
```

Tasks:

```text
[x] Verify Go tooling runs from server/ module root.
[x] Fix any package or IDE issue that reports undefined RedisClient.
[x] Confirm RedisClient remains owned by internal/cache.
[x] Confirm transport depends on a narrow latestRoomStateWriter interface, not go-redis.
[x] Confirm server starts when REDIS_ADDR is empty by code path and existing app test coverage.
[ ] Confirm server starts when Redis is unavailable and REDIS_REQUIRED=false with go test or local run.
[ ] Confirm server fails fast when REDIS_REQUIRED=true and Redis is unavailable with go test or local run.
[x] Confirm room transitions and broadcasts succeed when cache writes fail by code path.
```

Acceptance:

```text
go test ./internal/cache ./internal/app ./internal/transport
```

If the local machine cannot run `go`, document that blocker before marking this phase complete.

## Phase B: Cache Write Semantics

Goal: make the latest room_state cache predictable without changing sync authority.

Status on 2026-05-17:

```text
[x] WebSocket join_room writes latest room_state after returning the join snapshot.
[x] Accepted play / pause / seek / set_playback_rate / ended write latest room_state before broadcast.
[x] host_left and host_rejoin room_state refreshes write latest room_state through broadcastRoomState.
[x] Cache writes use a short 200ms timeout and remain best-effort.
[x] Redis is not read for host authority, membership, or control permission.
[ ] Room-level integration tests are deferred until the next room testing pass.
```

Tasks:

```text
[x] Write latest room_state after join_room snapshot.
[x] Write latest room_state after accepted play / pause / seek / set_playback_rate / ended.
[x] Write latest room_state after host_left and host_rejoin state refreshes.
[x] Keep cache writes best-effort and short-timeout.
[x] Do not read Redis to decide host authority, membership, or control permission.
[ ] Add room-level integration tests for cache write points in the next room testing pass.
```

Acceptance:

```text
Redis failures are logged only when debug sync logging is enabled.
Accepted timeline transitions do not depend on Redis availability.
```

## Phase C: Canonical Snapshot Direction

Goal: move protocol consumers toward `room_state` as the canonical snapshot while preserving client compatibility.

Status on 2026-05-17:

```text
[x] Legacy accepted-control events are preserved for Android/Web compatibility.
[x] Accepted-control payloads carry authoritative vector fields.
[x] room_state.request is available for clients that detect missed or stale state.
[x] room_state.request returns the latest in-process room.State as protocol.RoomStatePayload.
[x] room_state.request does not read Redis and does not make room_state the authority model.
```

Tasks:

```text
[x] Keep legacy accepted-control events for Android/Web compatibility.
[x] Ensure every legacy control payload carries authoritative vector fields.
[x] Add or prepare room_state.request for clients that detect missed or stale state.
[x] Let room_state.request return the latest room.State as protocol.RoomStatePayload.
[x] Do not make clients infer authority from event type alone.
```

Important wording:

```text
Correct: room_state carries the authoritative timeline vector.
Incorrect: room_state is the timeline authority.
```

Acceptance:

```text
Late join, reconnect, and explicit resync can recover from one latest timeline snapshot.
Clients can ignore stale seq and request a fresh snapshot without reconnecting.
```

## Phase D: Seq Diagnostics And Request Id Dedup

Goal: improve race and retry behavior without breaking existing clients.

Status on 2026-05-17:

```text
[x] Client seq is recorded in sync debug logs and compared with previous/new server seq.
[x] Client seq remains soft diagnostic data and does not reject controls.
[x] Accepted transitions emit structured key-value stdout logs when debug sync logging is enabled.
[x] Optional requestId is accepted on play / pause / seek / set_playback_rate / ended.
[x] Accepted-control broadcasts echo requestId when provided.
[x] One-process short-TTL requestId dedup prevents duplicate accepted controls from advancing seq again.
[x] One-process dedup is sharded and bounded so high room counts do not share one global lock or unbounded map.
[ ] Redis-backed cross-instance requestId dedup is deferred until multi-instance room authority is designed.
```

Recommended order:

```text
1. Add soft client seq diagnostics. Done.
2. Add structured transition logs. Done.
3. Add optional requestId to control payloads. Done.
4. Add one-process short-TTL requestId dedup. Done.
5. Add Redis-backed cross-instance dedup only after room authority placement is designed.
```

Soft seq mode:

```text
client seq is recorded for logs and debugging
client seq does not reject controls yet
server seq remains the only authoritative version
```

Future strict mode can reject stale controls only after clients are known to send reliable expectations.

Request id dedup candidate key:

```text
wt:room:{roomId}:control_req:{requestId}
```

Do not store durable control history only in Redis.

Current dedup behavior:

```text
requestId is optional
dedup scope is one server process
dedup key is roomId + requestId
dedup TTL is short and in-memory
dedup storage is split into lock shards
dedup storage has a hard entry budget and fails open if saturated
expired entries are cleaned inside the touched shard instead of scanning one global map per request
duplicate accepted controls return the latest room_state to the requester
duplicate controls do not broadcast again and do not increment seq again
failed controls remove the reserved requestId so the client can retry
```

Logging direction:

```text
Use structured stdout logs from the application process.
Let product deployments persist and query logs through a logging pipeline such as Loki, OpenSearch, or a cloud log service.
Do not write normal application logs into PostgreSQL business tables.
Keep Prometheus-style metrics as a separate follow-up for counters, latency histograms, queue depth, and error rates.
```

## Phase E: App And Store Assembly Cleanup

Goal: reduce infrastructure duplication while preserving API behavior.

Status on 2026-05-17:

```text
[x] PostgreSQL is opened once in app assembly.
[x] auth, home, media, room, and progress stores share one *gorm.DB.
[x] database/sql pool defaults are set by store.OpenPostgres.
[x] Server has Shutdown and Close methods for HTTP, cleanup loops, Redis, and PostgreSQL resources.
[x] roomserver handles interrupt / SIGTERM with graceful shutdown.
[x] Targeted go test verification passes for internal/app and related refactor packages.
```

Tasks:

```text
[x] Open PostgreSQL once during app assembly when DATABASE_URL is configured.
[x] Share the same *gorm.DB across auth, home, media, room, and progress stores.
[x] Keep SQL-first migrations as the source of schema truth.
[x] Keep service DTOs separate from GORM model structs.
[x] Keep complex read queries as raw SQL when clearer than GORM chains.
```

Acceptance:

```text
HTTP API paths and envelopes stay unchanged.
Room creation, join, detail, media search, home summary, auth, and progress tests keep passing.
```

## Phase F: Test And Documentation Closeout

Goal: finish each refactor step with a small safety net.

Required checks:

```text
[ ] go test ./... currently fails in internal/mediactl on Windows because .sh ffprobe stubs are not Win32 executables.
[x] go test ./internal/protocol ./internal/transport ./internal/room ./internal/cache ./internal/app
[ ] docs/refactor updated when a phase boundary changes.
[x] docs/refactor updated when a phase boundary changes.
[x] docs/websocket-event-protocol.md updated because room_state.request and requestId changed the external protocol.
[ ] docs/backend-api-contract.md updated only when HTTP contract changes.
[ ] Redis keys and TTLs documented when new Redis use cases are added.
```

## Do Not Do In This Pass

```text
Do not make Redis the timeline authority.
Do not add multi-instance fan-out before room authority placement is designed.
Do not add buffering policy without explicit product rules.
Do not remove legacy control broadcasts until clients migrate.
Do not remove room.State.Paused until compatibility views are fully migrated.
Do not add continuous current-position broadcasts.
```

## Suggested Execution Order

```text
1. Phase A: build and Redis boundary stabilization
2. Phase B: cache write semantics
3. Phase E: app and store assembly cleanup
4. Phase C: canonical snapshot direction
5. Phase D: seq diagnostics and request id dedup
6. Phase F: closeout after each step
```

This order keeps the server buildable, avoids changing client behavior too early, and preserves the central rule: synchronize the room timeline, not individual players.
