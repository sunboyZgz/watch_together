# Runtime Boundaries

This document records the runtime ownership boundaries for the room system. `local_process` remains the default runtime. Phase 4 adds a `distributed_authority` MVP for multi-instance room authority, Phase 5 adds fenced authority recovery from the Kafka canonical timeline, Phase 6 adds distributed seek rate limiting plus observability, and Phase 7 adds service foundation boundaries with optional media/timeline RPC adapters and logical database ownership.

## Phase 1 Boundary Markers

`roomserver` now has two runtime-boundary settings:

```text
SERVER_INSTANCE_ID=
ROOM_RUNTIME_MODE=local_process
```

- `SERVER_INSTANCE_ID` is optional metadata for logs, health checks, and load-balancer diagnostics. Set a unique value per roomserver replica in multi-instance experiments.
- `ROOM_RUNTIME_MODE=local_process` keeps playback authority in one process.
- `ROOM_RUNTIME_MODE=distributed_authority` enables Redis authority leases, Redis active-device ownership, Redis seek rate limiting, NATS control forwarding, PostgreSQL outbox, Kafka timeline logging, and authority recovery.

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
| Active room playback authority | `internal/room.Manager` on the authority instance | Local in `local_process`; guarded by Redis authority lease in `distributed_authority`. |
| Latest room snapshot cache | Optional Redis `room_state` cache | Written from HTTP runtime bootstrap, WebSocket state transitions, and completed recovery; readable for quick snapshots, but not the write authority. |
| Cross-instance WebSocket fan-out | Optional NATS Core subject | When enabled, forwards already-built WebSocket envelopes between roomserver instances; not a state authority. |
| Room authority routing | Redis lease + NATS request/reply | Only in `distributed_authority`; non-authority instances forward controls to the authority instance and include the current authority epoch. |
| Active-device ownership | Redis lease | Only in `distributed_authority`; guards `deviceId + connectionId` ownership per room user. |
| User-level presence | Redis presence registry | Only in `distributed_authority`; stores internal device/connection details but exposes user-level `room_presence` snapshots. |
| Durable room timeline | PostgreSQL outbox + Kafka | Only in `distributed_authority`; Kafka is a result log and recovery source, not a command ingress or online fan-out path. |
| Authority recovery | Recovery service + Redis + Kafka + PostgreSQL outbox | Begins a recovering lease after expiry, replays Kafka, merges unpublished outbox rows, and completes a new active epoch. |
| Control idempotency | Local memory or Redis request registry | `local_process` uses process-local deduplication. `distributed_authority` uses Redis `requestId` records and backfills recovered accepted requests after Kafka replay. |
| Seek rate limiting | Local memory or Redis control rate registry | `local_process` uses process-local throttling. `distributed_authority` uses Redis `wt:room:control_rate:{roomId}:seek:v1`. |
| Observability | `internal/observability` | `/healthz`, `/readyz`, `/metrics`, Prometheus metrics, and worker metric hooks. |
| Service foundation metadata | `internal/servicekit` | Request id, service name/version, deadlines, internal auth, and trace metadata. |
| Optional internal RPC | ConnectRPC helper layer | `media` and `timeline` can run through local adapters or optional RPC services. |
| OpenTelemetry tracing | `internal/telemetry` | Optional traces across HTTP, WebSocket control, RPC, Redis, NATS, Kafka, and PostgreSQL. |
| Logical database ownership | PostgreSQL + docs | Phase 7 assigns table owners but does not physically split the database. |
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

Published events contain `instanceId`, `roomId`, envelope `type`, envelope `payload`, `seq`, `publishedAtMs`, and, in `distributed_authority`, `authorityEpoch`. Clients still receive the original WebSocket envelope only; no client-visible payload fields are added. The subscriber drops messages from its own `instanceId`, drops stale authority epochs, looks up current local clients for the target room, and enqueues the original envelope to those connections.

NATS Core is only real-time distribution in this phase. It is not Redis authority, Kafka authority, JetStream storage, durable replay, distributed control ordering, distributed deduplication, distributed seek rate limiting, presence, or active-device ownership. Remote broadcasts do not create rooms, do not mutate `internal/room.Manager`, and do not write Redis `room_state` snapshots.

If NATS is unavailable while cross-instance broadcast is disabled, startup is unchanged. If the feature is enabled but NATS cannot be opened, `roomserver` logs the failure and continues in local-process mode with cross-instance fan-out disabled.

## Phase 4 Distributed Authority Boundary

Phase 4 adds:

```text
ROOM_RUNTIME_MODE=distributed_authority
NATS_SUBJECT_ROOM_CONTROL=wt.room.control.v1
KAFKA_BROKERS=kafka:9092
KAFKA_TOPIC_ROOM_TIMELINE=wt.room.timeline.v1
KAFKA_TOPIC_ROOM_CONTROL_RESULT=wt.room.control_result.v1
KAFKA_TOPIC_ROOM_MEMBERSHIP=wt.room.membership.v1
```

`distributed_authority` requires `SERVER_INSTANCE_ID`, PostgreSQL, Redis, NATS, Kafka broker config, and `WS_CROSS_INSTANCE_BROADCAST_ENABLED=true`. Missing required config fails config loading; missing required runtime dependencies fail startup.

Redis keys:

```text
wt:room:authority:{roomId}:v1
wt:room:active_device:{roomId}:{userId}:v1
wt:room:control_request:{roomId}:{requestId}:v1
wt:room:control_rate:{roomId}:seek:v1
wt:room:presence:{roomId}:v1
```

The authority lease value contains `instanceId`, `epoch`, `status`, and `leaseUntilMs`. `status=active` identifies the instance allowed to mutate `internal/room.Manager` for that room. `status=recovering` fences concurrent takeover attempts while one instance rebuilds room state. Non-authority instances do not apply controls; they forward the original envelope and connection context over NATS request/reply to the authority instance with the expected `epoch`.

The active-device lease stores `deviceId`, `instanceId`, `connectionId`, and `leaseUntilMs`. Disconnect and leave release only when both `deviceId` and `connectionId` match, preventing old sockets from clearing newer ownership.

The control request registry stores recent `requestId` outcomes with `status=pending|accepted|rejected`, `authorityEpoch`, `seq`, accepted envelope, error text, and TTL. A duplicate accepted request returns the original accepted envelope. A duplicate pending request returns `room authority processing`.

The control rate registry stores the current seek throttle reservation for a room. In `distributed_authority`, the authority instance reserves this key before applying seek. If the seek is rate-limited, the server returns current `room_state`, finalizes the request as rejected, and does not broadcast an accepted seek.

The presence registry stores one online entry per room user. Internal values may include `deviceId`, `connectionId`, and `instanceId`, but WebSocket `room_presence` exposes only user-level fields: `userId`, `role`, `isHost`, and per-recipient `isSelf`.

PostgreSQL `room_timeline_outbox` stores canonical timeline result events. `cmd/outboxworker` publishes them to Kafka, and `cmd/derivedworker` derives control-result and membership topics. Online WebSocket fan-out remains NATS + local connection tables.

## Phase 5 Authority Recovery Boundary

Phase 5 adds these recovery controls:

```text
AUTHORITY_RENEW_INTERVAL_MS=10000
AUTHORITY_TAKEOVER_SCAN_INTERVAL_MS=30000
AUTHORITY_RECOVERY_TIMEOUT_MS=5000
KAFKA_REPLAY_TIMEOUT_MS=1000
CONTROL_IDEMPOTENCY_TTL_MS=600000
PRESENCE_LEASE_TTL_MS=45000
PRESENCE_REFRESH_INTERVAL_MS=15000
```

Healthy authority instances renew active leases without changing `epoch`. After a lease is missing or expired, one instance can enter `status=recovering`, increment `epoch`, rebuild the room from the Kafka canonical timeline, merge same-room `pending` and `publishing` PostgreSQL outbox rows, register recovered state in `room.Manager`, write the latest Redis snapshot, and complete the lease as `status=active`.

Recovery uses Kafka as the result log. It replays `room.control.accepted` events to rebuild playback state. Rejected control events and membership events remain audit/dedup context and do not change playback state. PostgreSQL outbox rows only close the gap for decisions that were already written but not yet published to Kafka.

Old authority results are fenced by epoch. Local apply, NATS control replies, and NATS broadcasts are accepted only when the observed Redis authority lease is still active for the expected instance and epoch. A recovering room returns `room authority recovering` or a current snapshot; WebSocket connections are never moved between instances.

Phase 5 hardening requires accepted controls in `distributed_authority` to write `room_timeline_outbox` before Redis snapshot writes or WebSocket/NATS broadcast. If the outbox write fails, the accepted envelope is not broadcast and the client receives an error or current `room_state`. Rejected controls remain best-effort outbox writes.

Heartbeat acks refresh Redis presence leases at `PRESENCE_REFRESH_INTERVAL_MS`. Join, leave, disconnect, active-device lease loss, and device switch publish a fresh user-level `room_presence` snapshot. Presence is runtime state only; it is not PostgreSQL membership and is not written to Kafka.

## Phase 6 Observability Boundary

Phase 6 adds:

```text
METRICS_ENABLED=true
METRICS_ADDR=
METRICS_PATH=/metrics
READINESS_PATH=/readyz
```

`GET /healthz` remains lightweight liveness. `GET /readyz` reports JSON dependency readiness for PostgreSQL, Redis, NATS, Kafka, outbox, and recovery. `GET /metrics` exposes Prometheus metrics when enabled. `roomserver` exposes metrics on the main HTTP server; worker processes expose metrics only when `METRICS_ADDR` is set. The observability module only records and exposes runtime state; it does not participate in room authority, playback decisions, or recovery decisions.

## Phase 7/8 Service Foundation Boundary

Phase 7 adds:

```text
SERVICE_NAME=watch-together-roomserver
SERVICE_VERSION=dev
INTERNAL_RPC_ENABLED=false
INTERNAL_RPC_ADDR=:8090
INTERNAL_RPC_PATH_PREFIX=/internal.rpc
INTERNAL_RPC_TIMEOUT_MS=1000
INTERNAL_RPC_AUTH_TOKEN=
SERVICE_DISCOVERY_MODE=static
MEDIA_SERVICE_MODE=local
MEDIA_SERVICE_ADDR=
TIMELINE_SERVICE_MODE=local
TIMELINE_SERVICE_ADDR=
OTEL_TRACING_ENABLED=false
OTEL_SERVICE_NAME=watch-together-roomserver
OTEL_EXPORTER_OTLP_ENDPOINT=
OTEL_TRACE_SAMPLE_RATIO=0.1
```

`MEDIA_SERVICE_MODE` and `TIMELINE_SERVICE_MODE` accept `local` or `rpc`. `local` keeps current in-process adapters. `rpc` calls optional `cmd/mediaservice` or `cmd/timelineservice` over ConnectRPC. Phase 8 uses generated typed ConnectRPC contracts from `server/api/internal/v1` into `server/internal/rpcgen/v1`; Android HTTP/WebSocket payloads are unchanged. Service discovery is static endpoint configuration in this phase; Consul, etcd, and Kubernetes service discovery integration are not required by the code.

OpenTelemetry tracing is opt-in. Trace metadata can cross internal RPC, NATS, and Kafka boundaries, but it is not exposed in Android HTTP/WebSocket payloads.

Database ownership is logical only. PostgreSQL remains a single database, but table owners and cross-context access rules are documented in [Database Ownership](./database-ownership.md) and enforced from `server/internal/store/db_ownership.yaml`. Future physical database splitting must replace cross-context writes and reads with service calls, events, or read models first.

## Multi-Instance Readiness After Phase 1

Safe or mostly safe now:

- HTTP APIs that only read/write PostgreSQL state.
- Media signing when all instances share the same media signing secret.
- Health diagnostics can identify which instance handled a request.
- Cross-instance WebSocket broadcast fan-out for already-authoritative local events, when NATS Core is enabled.
- Distributed seek rate limiting in `distributed_authority`.
- Readiness and Prometheus metrics endpoints.

Still not complete:

- Automatic takeover before Redis lease expiry.
- Device-level presence management and full multi-device UI.
- WebSocket connection migration between instances.
- Kafka command-ingress logging.
- Physical PostgreSQL splitting by service.
- Extracting `room authority` to a standalone microservice.

See [distributed-architecture.md](./distributed-architecture.md) for the current module map, business flows, and monitoring data flow.
