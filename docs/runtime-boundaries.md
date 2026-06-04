# Runtime Boundaries

This document records the architecture boundary cleanup for the current `local_process` room runtime. It does not claim the backend is distributed yet; it makes runtime ownership explicit so later phases can externalize room state and cross-instance broadcasting without changing API contracts first.

## Phase 1 Boundary Markers

`roomserver` now has two runtime-boundary settings:

```text
SERVER_INSTANCE_ID=
ROOM_RUNTIME_MODE=local_process
```

- `SERVER_INSTANCE_ID` is optional metadata for logs, health checks, and load-balancer diagnostics. Set a unique value per roomserver replica in multi-instance experiments.
- `ROOM_RUNTIME_MODE=local_process` is the only implemented mode. Unsupported values fail config loading so deployments cannot accidentally advertise a distributed room runtime that does not exist yet.

`GET /healthz` still returns `ok`, and now includes:

```text
X-Watch-Together-Instance-ID: <SERVER_INSTANCE_ID, when configured>
X-Watch-Together-Room-Runtime: local_process
```

## Current State Ownership

| State | Current Owner | Current Boundary |
| --- | --- | --- |
| JWT validation | Stateless server config | Multi-instance safe when `AUTH_JWT_SECRET` and TTL config are shared. |
| HTTP auth, home, media, room, progress APIs | PostgreSQL-backed services | Mostly stateless request handling; DB-backed handlers can run on any instance. |
| WebSocket connection objects | Local Go process memory | Must remain local; future cross-instance work should broadcast between local connection tables. |
| Active room playback authority | `internal/room.Manager` in one process | Still single-process; visible as `ROOM_RUNTIME_MODE=local_process`. |
| Latest room snapshot cache | Optional Redis `room_state` cache | Written from HTTP runtime bootstrap and WebSocket state transitions; readable for future recovery paths, but not the current write authority. |
| Cross-instance WebSocket fan-out | Optional NATS Core subject | When enabled, forwards already-built WebSocket envelopes between roomserver instances; not a state authority. |
| Control deduplication | Local WebSocket handler memory | Still process-local; future phases should externalize if multiple instances can host one room. |
| Seek rate limiting | Local WebSocket handler memory | Still process-local; future phases should externalize or shard room authority. |
| Room metadata and membership | PostgreSQL | Durable business state. |
| Media metadata | PostgreSQL | Durable catalog state. |
| HLS files | Local/object storage via mediactl | Served through signed playback paths and Nginx/object storage depending on delivery mode. |

## Phase 2 Snapshot Boundary

Redis `room_state` is now treated as the latest playback snapshot cache for the local-process room runtime. The cache can be written and read through `RoomStateCache`, and snapshot writes are attempted after HTTP room runtime bootstrap, WebSocket `join_room`, explicit `room_state.request`, successful playback controls, stale-control state replies, membership changes, and lifecycle state broadcasts.

Redis still does not own playback authority. `internal/room.Manager` remains the source of truth for applying controls, incrementing `seq`, validating host authority, and managing local WebSocket clients. Redis cache write failures are non-fatal and must not change HTTP or WebSocket response shapes.

The snapshot cache contains playback state only. It does not contain WebSocket connection objects, client send queues, heartbeat state, control deduplication records, seek rate-limit state, online presence, or active-device ownership.

## Phase 3 NATS Core Broadcast Boundary

Phase 3 adds an optional event bus for cross-instance WebSocket broadcast fan-out:

```text
WS_CROSS_INSTANCE_BROADCAST_ENABLED=false
WS_EVENT_BUS=nats_core
NATS_URL=nats://127.0.0.1:4222
NATS_NAME=watch-together-roomserver
NATS_SUBJECT_ROOM_BROADCAST=wt.room.broadcast.v1
```

The default remains disabled. When enabled, each `roomserver` opens a normal NATS Core subscription to the same room broadcast subject. Do not use a NATS queue group for this subject: queue groups divide messages across subscribers, while room broadcast fan-out requires every roomserver instance to receive each message.

Published events contain `instanceId`, `roomId`, envelope `type`, envelope `payload`, `seq`, and `publishedAtMs`. Clients still receive the original WebSocket envelope only; no client-visible payload fields are added. The subscriber drops messages from its own `instanceId`, looks up current local clients for the target room, and enqueues the original envelope to those connections.

NATS Core is only real-time distribution in this phase. It is not Redis authority, Kafka authority, JetStream storage, durable replay, distributed control ordering, distributed deduplication, distributed seek rate limiting, presence, or active-device ownership. Remote broadcasts do not create rooms, do not mutate `internal/room.Manager`, and do not write Redis `room_state` snapshots.

If NATS is unavailable while cross-instance broadcast is disabled, startup is unchanged. If the feature is enabled but NATS cannot be opened, `roomserver` logs the failure and continues in local-process mode with cross-instance fan-out disabled.

## Multi-Instance Readiness After Phase 1

Safe or mostly safe now:

- HTTP APIs that only read/write PostgreSQL state.
- Media signing when all instances share the same media signing secret.
- Health diagnostics can identify which instance handled a request.
- Cross-instance WebSocket broadcast fan-out for already-authoritative local events, when NATS Core is enabled.

Still not multi-instance safe:

- A single room with clients connected to different instances that expect distributed control authority.
- Cross-instance host/device occupancy checks.
- Cross-instance control event ordering, deduplication, and rate limiting.
- Room playback authority recovery after the instance that owns the in-memory room state restarts.

## Next Phase Boundary

The next small step should externalize cross-instance event distribution while keeping local WebSocket connection tables process-local. That work should keep WebSocket payloads and HTTP API shapes unchanged.

The HTTP room entrypoint already depends on a narrow room runtime registry boundary for room-state registration and snapshot bootstrap. The current adapter still wraps `internal/room.Manager` and writes optional Redis snapshots, so runtime behavior remains `local_process`; later phases can replace that adapter without changing the public HTTP handler constructors, API responses, or WebSocket payloads.
