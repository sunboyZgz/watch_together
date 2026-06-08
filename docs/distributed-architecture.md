# Distributed Architecture

Phase 4 introduced the distributed room infrastructure MVP while keeping the public HTTP API and playback control payloads small. `local_process` remains supported. `distributed_authority` adds Redis authority leases, Redis active-device ownership, NATS Core internal routing, PostgreSQL outbox, and Kafka timeline topics. Phase 5 adds authority recovery and hardening: expired authority leases can be fenced, recovered from Kafka canonical timeline events, completed as a new active authority epoch, resumed without moving WebSocket connections, protected by Redis `requestId` idempotency, and exposed through user-level Redis presence. Phase 6 adds distributed seek rate limiting and observability: seek throttling moves to Redis in `distributed_authority`, `/readyz` reports dependency readiness, and `/metrics` exposes Prometheus metrics. Phase 7 adds the service foundation: servicekit, internal ConnectRPC contracts, optional media/timeline service skeletons, OpenTelemetry tracing, static service discovery, and logical database ownership design. Phase 8 makes the service pilot verifiable: generated typed ConnectRPC contracts replace dynamic `Struct` RPC payloads for media/timeline, the optional RPC services can be run through a compose `rpc-pilot` profile, and database ownership is enforced by a machine-readable registry plus architecture tests. Phase 9 turns `media` into the first database-boundary pilot: `MEDIA_DATABASE_URL` can point media code at an independent media database in the same PostgreSQL server/container, while empty config keeps the single-database fallback. Phase 10 adds the timeline database boundary: `TIMELINE_DATABASE_URL` can move `room_timeline_outbox` to an independent timeline database, while empty config keeps the single-database fallback.

## Architecture Mode

Android remains a session client:

- HTTP bootstrap creates or joins durable room business state.
- WebSocket `/ws` carries the live room session.
- `join_room.payload.deviceId` is required. Android generates one local device ID, stores it in `device.xml`, and excludes it from cloud backup and device transfer.
- The client consumes server WebSocket envelopes and does not know whether the current `roomserver` is authoritative.
- Android prefers `room_presence.onlineCount` for online display and falls back to membership estimates when presence is absent.

Backend mode in `distributed_authority`:

- `transport` owns HTTP and WebSocket protocol handling.
- `room` owns in-process room state for the authoritative instance.
- `cache` owns Redis latest snapshots, authority leases, active-device leases, control request idempotency, distributed seek rate limiting, and user-level presence.
- `eventbus` owns NATS Core broadcast and control request/reply.
- `store` owns PostgreSQL durable business state and outbox rows.
- `timeline` owns Kafka JSON v1 event shape, outbox dispatch, and derived-topic dispatch.
- `cmd/outboxworker` publishes outbox rows to the canonical Kafka topic.
- `cmd/derivedworker` derives control-result and membership topics from the canonical timeline.
- `recovery` owns authority takeover, Kafka replay, unpublished outbox merge, and recovered `room.Manager` registration.
- `observability` owns readiness snapshots, Prometheus metrics, and worker metric hooks.
- `servicekit` owns internal request metadata, deadline, service identity, and internal auth conventions.
- `internalrpc` owns ConnectRPC server/client helpers for local/RPC dual-mode adapters.
- `telemetry` owns OpenTelemetry tracing setup and propagation.
- `cmd/mediaservice` and `cmd/timelineservice` are optional service skeletons for the first serviceization candidates.
- `internal/rpcgen/v1` contains generated typed internal RPC messages and ConnectRPC client/handler code.

## Service Evolution Architecture

Phase 7 is not a full microservice split. Phase 8 keeps that constraint but hardens the pilot. Phase 9 adds a database-boundary pilot for media, and Phase 10 adds the same kind of database-boundary pilot for timeline outbox storage, without changing Android HTTP/WebSocket payloads and without extracting `room authority`. The default deployment still runs `roomserver` with local adapters and a single database. `MEDIA_DATABASE_URL` enables a separate media database; `TIMELINE_DATABASE_URL` enables a separate timeline database. `MEDIA_SERVICE_MODE=rpc` and `TIMELINE_SERVICE_MODE=rpc` let `roomserver` call `cmd/mediaservice` and `cmd/timelineservice` without directly connecting to those databases. Local mode still works and uses the owner database URL when configured. The `.proto` files under `server/api/internal/v1` are the source contracts; generated Go code lives under `server/internal/rpcgen/v1`.

```mermaid
flowchart LR
  Roomserver[roomserver]
  App[app composition root]
  MediaPort[media port]
  TimelinePort[timeline port]
  MediaLocal[local media adapter]
  TimelineLocal[local timeline adapter]
  MediaRPC[media RPC adapter]
  TimelineRPC[timeline RPC adapter]
  MediaService[cmd/mediaservice]
  TimelineService[cmd/timelineservice]
  MainPG[(main PostgreSQL database)]
  MediaPG[(media PostgreSQL database)]
  TimelinePG[(timeline PostgreSQL database)]
  Kafka[(Kafka)]
  Redis[(Redis)]
  NATS[(NATS)]

  Roomserver --> App
  App --> MediaPort
  App --> TimelinePort
  MediaPort -->|MEDIA_SERVICE_MODE=local| MediaLocal
  MediaPort -->|MEDIA_SERVICE_MODE=rpc| MediaRPC
  TimelinePort -->|TIMELINE_SERVICE_MODE=local| TimelineLocal
  TimelinePort -->|TIMELINE_SERVICE_MODE=rpc| TimelineRPC
  MediaRPC --> MediaService
  TimelineRPC --> TimelineService
  MediaLocal -->|MEDIA_DATABASE_URL or DATABASE_URL| MediaPG
  TimelineLocal -->|TIMELINE_DATABASE_URL or DATABASE_URL| TimelinePG
  MediaService -->|MEDIA_DATABASE_URL fallback DATABASE_URL| MediaPG
  TimelineService -->|TIMELINE_DATABASE_URL fallback DATABASE_URL| TimelinePG
  TimelineService --> Kafka
  Roomserver --> Redis
  Roomserver --> NATS
```

## Database Ownership

Phase 7 keeps one PostgreSQL database and introduces logical ownership boundaries. Phase 8 makes those boundaries testable. Phase 9 gives `media` its own PostgreSQL database boundary when `MEDIA_DATABASE_URL` is configured. Phase 10 gives `timeline` its own PostgreSQL database boundary when `TIMELINE_DATABASE_URL` is configured. Table owners and cross-context access rules live in [Database Ownership](./database-ownership.md), with the CI source of truth in `server/internal/store/db_ownership.yaml`.

```mermaid
flowchart TD
  MainPG[(Main PostgreSQL database)]
  MediaPG[(Media PostgreSQL database)]
  TimelinePG[(Timeline PostgreSQL database)]
  Identity[identity owns users]
  Media[media owns media tables]
  RoomSession[room-session owns rooms and room_members]
  Progress[progress owns user_media_progress]
  Timeline[timeline owns room_timeline_outbox]
  Home[home-composition owns no tables]

  MainPG --> Identity
  MediaPG --> Media
  MainPG --> RoomSession
  MainPG --> Progress
  TimelinePG --> Timeline
  Home -->|read model/composition only| Identity
  Home -->|media port/RPC summaries| Media
  Home -->|read model/composition only| Progress
```

## Business Architecture

```mermaid
flowchart TD
  Login[Login and register]
  Home[Home summary and media catalog]
  Create[Create room]
  Join[Join room by code]
  WSJoin[WebSocket join_room]
  Presence[room_presence online snapshot]
  Control[Host playback controls]
  Authority[Authority decision]
  Timeline[Kafka room timeline]
  Derived[Derived business topics]
  Recovery[Authority recovery]
  Progress[Low-frequency progress]

  Login --> Home
  Home --> Create
  Home --> Join
  Create --> WSJoin
  Join --> WSJoin
  WSJoin --> Presence
  WSJoin --> Control
  Control --> Authority
  Authority --> Presence
  Authority --> Timeline
  Timeline --> Derived
  Timeline --> Recovery
  Home --> Progress
```

## Core Module View

```mermaid
flowchart LR
  Android[Android Client]
  HTTP[transport HTTP]
  WS[transport WebSocket]
  App[app assembly]
  Room[room.Manager]
  Redis[(Redis)]
  PG[(Main PostgreSQL)]
  MediaPG[(Media PostgreSQL)]
  TimelinePG[(Timeline PostgreSQL)]
  NATS[(NATS Core)]
  Outbox[room_timeline_outbox]
  OutboxWorker[cmd/outboxworker]
  Kafka[(Kafka)]
  Derived[cmd/derivedworker]
  Recovery[recovery service]
  Obs[observability]
  Metrics[Prometheus]
  Mediactl[mediactl]
  Storage[(Media storage)]
  Servicekit[servicekit/internalrpc]
  OTel[OpenTelemetry]
  MediaSvc[cmd/mediaservice]
  TimelineSvc[cmd/timelineservice]

  Android -->|HTTP bootstrap| HTTP
  Android -->|WebSocket envelopes| WS
  App --> HTTP
  App --> WS
  HTTP -->|room metadata and membership| PG
  HTTP -->|media port local mode| MediaPG
  HTTP -->|register local room mirror| Room
  HTTP -->|claim authority lease| Redis
  WS -->|local authoritative apply| Room
  WS -->|authority, active-device, requestId, rate-limit, presence| Redis
  WS -->|control request/reply| NATS
  WS -->|broadcast accepted envelopes| NATS
  WS -->|timeline result event| Outbox
  Outbox --> TimelinePG
  OutboxWorker -->|claim pending rows| TimelinePG
  OutboxWorker -->|canonical JSON v1| Kafka
  Derived -->|consume canonical| Kafka
  Derived -->|publish derived topics| Kafka
  Recovery -->|begin/complete authority epoch| Redis
  Recovery -->|room metadata| PG
  Recovery -->|replay canonical events| Kafka
  Recovery -->|merge pending/publishing rows| Outbox
  Recovery -->|register recovered state| Room
  Mediactl -->|ingest HLS| Storage
  HTTP -->|signed playback| Storage
  App -->|health/readiness/metrics| Obs
  App -->|service metadata and RPC adapters| Servicekit
  WS -->|control, presence, connection metrics| Obs
  OutboxWorker -->|worker metrics| Obs
  Derived -->|worker metrics| Obs
  Metrics -->|scrape| Obs
  Servicekit -->|optional ConnectRPC| MediaSvc
  Servicekit -->|optional ConnectRPC| TimelineSvc
  MediaSvc --> MediaPG
  TimelineSvc --> TimelinePG
  TimelineSvc --> Kafka
  OTel -->|traces| App
  OTel -->|traces| MediaSvc
  OTel -->|traces| TimelineSvc
```

## Runtime Boundaries

| Boundary | Owner | Phase 6 role |
| --- | --- | --- |
| Local WebSocket connection | `roomserver` process | Never leaves process memory. |
| Latest room snapshot | Redis `wt:room:state:{roomId}:v1` | Recovery-oriented cache, not authority. |
| Room authority lease | Redis `wt:room:authority:{roomId}:v1` | Identifies the instance allowed to mutate room playback state. |
| Active device lease | Redis `wt:room:active_device:{roomId}:{userId}:v1` | Guards one active client device per room user. |
| Control request idempotency | Redis `wt:room:control_request:{roomId}:{requestId}:v1` | Stores pending, accepted, and rejected request outcomes across instances. |
| Distributed seek rate limit | Redis `wt:room:control_rate:{roomId}:seek:v1` | Enforces seek min interval across authority takeovers and forwarded controls. |
| User-level presence | Redis `wt:room:presence:{roomId}:v1` | Runtime online occupancy by user; does not expose device, connection, or instance IDs. |
| Durable main business state | PostgreSQL | Users, rooms, members, and progress. |
| Media database boundary | `MEDIA_DATABASE_URL` PostgreSQL database | Optional Phase 9 database for media-owned tables; empty config falls back to the main database. |
| Timeline database boundary | `TIMELINE_DATABASE_URL` PostgreSQL database | Optional Phase 10 database for `room_timeline_outbox`; empty config falls back to the main database. |
| Reliable Kafka compensation | Timeline PostgreSQL outbox | Pending canonical timeline events. |
| Durable timeline | Kafka `wt.room.timeline.v1` | Accepted/rejected control and membership result log. |
| Derived topics | Kafka | `wt.room.control_result.v1`, `wt.room.membership.v1`. |
| Realtime internal routing | NATS Core | Broadcast fan-out and non-authority control request/reply. |
| Recovery replay | Kafka + timeline outbox | Rebuilds authority state after an expired authority lease. |
| Monitoring | `observability` + Prometheus | `/healthz`, `/readyz`, `/metrics`, and worker metric hooks. |
| Service metadata | `servicekit` | Request id, service name/version, deadline, internal auth, and trace metadata conventions. |
| Internal RPC | ConnectRPC over HTTP | Optional media/timeline RPC adapters; Android protocol remains unchanged. |
| Database ownership | PostgreSQL databases + docs | Main, optional media, and optional timeline databases keep owner rules and cross-context access checks. |
| Tracing | OpenTelemetry | Optional trace spans across HTTP, WebSocket control, Redis, NATS, Kafka, RPC, and PostgreSQL. |

## Control Flow

```mermaid
sequenceDiagram
  participant C as Android
  participant B as roomserver B
  participant R as Redis
  participant N as NATS Core
  participant A as roomserver A authority
  participant P as Timeline PostgreSQL
  participant K as Kafka

  C->>B: join_room(roomId,userId,deviceId)
  B->>R: acquire active-device lease
  B->>R: upsert user-level presence
  B-->>C: room_state
  B-->>C: room_presence
  C->>B: play(seq)
  B->>R: reserve requestId pending
  B->>R: read room authority
  alt B is not authority
    B->>N: request control to A
    N->>A: forwarded original envelope + connection context
    A->>R: validate active-device lease
    A->>R: reserve seek rate limit when type=seek
    A->>A: apply to room.Manager
    A->>P: insert room_timeline_outbox
    A->>R: finalize requestId accepted
    A->>N: publish accepted WebSocket envelope
    A-->>N: reply accepted envelope
    N-->>B: reply accepted envelope
    B-->>C: accepted envelope
  else B is authority
    B->>R: reserve seek rate limit when type=seek
    B->>B: apply to room.Manager
    B->>P: insert room_timeline_outbox
    B->>R: finalize requestId accepted
    B->>N: publish accepted WebSocket envelope
  end
  N-->>B: realtime broadcast to local clients
  P->>K: outboxworker publishes canonical event
  K->>K: derivedworker publishes domain topics
```

## Monitoring Flow

```mermaid
flowchart LR
  Roomserver[roomserver]
  Ready[/GET /readyz/]
  MetricsEndpoint[/GET /metrics/]
  WorkerMetrics[/worker METRICS_ADDR metrics/]
  Prom[Prometheus]
  Grafana[Grafana and alerts]
  WS[WebSocket handler]
  Recovery[recovery service]
  OutboxWorker[outboxworker]
  DerivedWorker[derivedworker]
  Obs[observability]

  Roomserver --> Ready
  Roomserver --> MetricsEndpoint
  WS -->|connections, controls, seek limits, presence| Obs
  Recovery -->|recovery attempts| Obs
  OutboxWorker -->|publish success/failure| Obs
  DerivedWorker -->|derived publish success/failure| Obs
  Obs --> MetricsEndpoint
  Obs --> WorkerMetrics
  Prom -->|scrape| MetricsEndpoint
  Prom -->|scrape| WorkerMetrics
  Prom --> Grafana
```

## Recovery Flow

```mermaid
sequenceDiagram
  participant B as roomserver B
  participant R as Redis
  participant P as Timeline PostgreSQL
  participant K as Kafka
  participant M as room.Manager

  B->>R: BeginRecovery(roomId)
  R-->>B: recovering lease epoch=N
  B->>P: load room metadata
  B->>K: replay wt.room.timeline.v1 for roomId
  B->>P: read pending/publishing outbox rows
  B->>M: RegisterRecoveredRoom(state)
  B->>R: backfill accepted requestId records
  B->>R: CompleteRecovery(epoch=N)
  R-->>B: active lease epoch=N
```

## Data Flows

Create room:

1. Android calls `POST /rooms`.
2. Room service validates the selected episode through the media port, either local `PostgresMediaStore` or `MediaInternalService`. In Phase 9 the local media store can use `MEDIA_DATABASE_URL`.
3. HTTP service writes room and host membership to PostgreSQL.
4. Runtime registers a local room mirror with media duration from the media port.
5. In `distributed_authority`, current instance tries to claim Redis authority.
6. Redis latest room snapshot is written best-effort.

Join room:

1. Android calls `POST /rooms/{roomCode}/join`.
2. PostgreSQL membership is created or reactivated.
3. Room service loads media detail through the media port for runtime bootstrap.
4. Runtime registers a local room mirror and tries authority claim.
5. WebSocket `join_room` follows.

WebSocket join:

1. Client sends `roomId`, `userId`, and required `deviceId`.
2. Server verifies token user and active PostgreSQL membership.
3. In `distributed_authority`, server acquires or refreshes Redis active-device lease.
4. In `distributed_authority`, server upserts Redis user-level presence.
5. Server joins the process-local connection table and sends `room_state`.
6. Server sends a `room_presence` snapshot to the joined client and broadcasts a fresh snapshot to other local and remote clients.

Local authoritative control:

1. Server validates identity, active-device lease, authority lease, requestId idempotency, seq, dedup, and rate limit.
2. `room.Manager` applies the state transition.
3. Server performs a final Redis authority epoch check.
4. Server writes timeline outbox before any accepted broadcast. In Phase 10 this uses `TIMELINE_DATABASE_URL` when configured.
5. Server finalizes Redis requestId as accepted and writes Redis latest snapshot.
6. Server broadcasts the accepted WebSocket envelope locally and through NATS.
7. If the local outbox or timeline RPC write fails in `distributed_authority`, the accepted envelope is not broadcast, accepted requestId idempotency is not written, state is rolled back, and the client receives `room timeline unavailable` or current `room_state`.

Distributed seek rate limiting:

1. `local_process` keeps process-local seek limiting.
2. `distributed_authority` reserves `wt:room:control_rate:{roomId}:seek:v1` before applying seek.
3. A rate-limited seek returns current `room_state`, finalizes the request as rejected, and does not broadcast an accepted seek.
4. If apply later fails, the matching Redis reservation token is released.

Non-authority control forwarding:

1. Ingress instance reserves Redis requestId and validates the source active-device lease.
2. Ingress reads Redis authority and authority epoch.
3. Ingress forwards the original envelope plus `instanceId`, `deviceId`, `connectionId`, and expected epoch over NATS request/reply.
4. Authority instance validates authority and active-device lease before apply and again before reply.
5. Accepted results are written to outbox or timeline RPC, finalized in Redis idempotency, and broadcast through NATS to all roomserver instances.
6. Ingress drops stale-epoch replies and returns `room authority unavailable` or the current snapshot instead of accepting late old-authority results.
7. If the authority timeline recorder fails, ingress receives `room timeline unavailable`; the authority state is rolled back and no accepted broadcast is emitted.

Active-device switch:

- `distributed_authority` externalizes active-device ownership to Redis.
- Same `deviceId` may refresh ownership with a newer `connectionId`.
- A different `deviceId` conflicts unless the existing lease expires or is released by matching `deviceId + connectionId`.

User-level presence:

1. `join_room` success upserts Redis presence and emits `room_presence`.
2. Heartbeat acks refresh presence at `PRESENCE_REFRESH_INTERVAL_MS`.
3. `leave_room`, disconnect, active-device lease loss, and device switch release only matching `deviceId + connectionId`.
4. Presence broadcasts expose only `roomId`, `onlineCount`, `members`, `reason`, and `serverTimeMs`.
5. Kafka does not store online presence.

Kafka outbox delivery:

1. Authority writes `room_timeline_outbox` after accepted/rejected controls and membership changes.
2. In Phase 10, this table is in the timeline database when `TIMELINE_DATABASE_URL` is configured; otherwise it uses the main database fallback.
3. `outboxworker` claims pending rows with `FOR UPDATE SKIP LOCKED`.
4. Successful Kafka publish marks rows `published`.
5. Failure increments attempts, stores `last_error`, and schedules `next_attempt_at`.

Derived topics:

1. `derivedworker` consumes `wt.room.timeline.v1`.
2. Control events publish to `wt.room.control_result.v1`.
3. Membership events publish to `wt.room.membership.v1`.

Progress:

1. Android calls `PUT /me/media-progress/{mediaItemId}`.
2. Progress service validates the episode through the media port before writing.
3. `PostgresProgressStore` writes only `user_media_progress` and user existence checks; it does not query media tables directly.

Home summary:

1. Android calls `GET /home/summary`.
2. `PostgresHomeStore` reads user profile and progress episode ids from the main database.
3. Home service calls `BatchGetEpisodeSummaries` through the media port to fill title and cover.
4. Missing media summaries are skipped, preserving the old inner-join behavior.

Realtime fan-out:

1. Accepted WebSocket envelopes publish to NATS Core broadcast subject.
2. Every roomserver subscribes directly to the subject, without queue groups.
3. Subscribers drop their own `instanceId`.
4. Remote events are delivered only to local WebSocket clients for that room and do not mutate local authority state.

Observability:

1. `/healthz` stays a lightweight liveness endpoint and keeps runtime headers.
2. `/readyz` reports configured dependency readiness for PostgreSQL, media PostgreSQL, timeline PostgreSQL, Redis, NATS, Kafka, outbox, and recovery.
3. `/metrics` exposes Prometheus metrics when `METRICS_ENABLED=true`.
4. `roomserver` serves metrics on the main HTTP server; `outboxworker` and `derivedworker` serve metrics when `METRICS_ADDR` is set.
5. Metrics include WebSocket connections, control accepted/rejected counts, seek rate-limit decisions, NATS room events, authority recovery attempts, presence online gauge, and worker publish results.

Authority recovery:

1. A join, room state request, control event, or low-frequency scanner discovers a missing or expired authority lease.
2. One instance enters Redis `status=recovering` and increments the authority `epoch`.
3. The instance loads room metadata from PostgreSQL.
4. The instance replays `room.control.accepted` events from `wt.room.timeline.v1`.
5. The instance merges same-room `pending` and `publishing` timeline outbox events by event id.
6. The instance registers recovered playback state in local `room.Manager`.
7. The instance backfills recent accepted Redis requestId records from recovered events.
8. Redis authority is completed as `status=active` for the new epoch.
9. NATS control forwarding and broadcast messages carry `authorityEpoch`; stale epoch messages are rejected or dropped.

## Service Foundation Flow

1. `internal/app` chooses local or RPC adapters for media and timeline based on `MEDIA_SERVICE_MODE` and `TIMELINE_SERVICE_MODE`.
2. Local adapters keep the current PostgreSQL/Kafka code path.
3. RPC adapters call `cmd/mediaservice` or `cmd/timelineservice` through ConnectRPC.
4. `servicekit` attaches request id, service name/version, deadline, internal auth, and trace metadata.
5. OpenTelemetry tracing is opt-in and does not change Android payloads.
6. `room-session -> media` and `progress -> media` now cross through the media port in both local and RPC mode.
7. `timeline` recorder failures, including timeline database failure, RPC timeout, or unavailable errors, are fail-closed for accepted controls.

## Phase 9 Media Database Flow

```mermaid
flowchart LR
  MainPG[(watch_together_main)]
  MediaPG[(watch_together_media)]
  MediaMigrations[server/media_migrations]
  MainMigrations[server/migrations]
  Sync[cmd/mediadbsync]
  Roomserver[roomserver]
  MediaService[mediaservice]
  MediaPort[media port]
  Home[home summary]

  MainMigrations --> MainPG
  MediaMigrations --> MediaPG
  Sync -->|copy and verify media tables| MediaPG
  MainPG -->|source shadow media rows| Sync
  Roomserver --> MediaPort
  MediaPort -->|local mode| MediaPG
  MediaPort -->|rpc mode| MediaService
  MediaService --> MediaPG
  Home -->|progress ids from main DB| MainPG
  Home -->|BatchGetEpisodeSummaries| MediaPort
```

`cmd/mediadbsync` copies `media_tags`, `media_seasons`, `media_episodes`, `media_season_tags`, and `media_episode_variants` with stable ids and timestamps. `--verify-only` checks row counts and deterministic content hashes.

## Phase 10 Timeline Database Flow

```mermaid
flowchart LR
  MainPG[(watch_together_main)]
  TimelinePG[(watch_together_timeline)]
  TimelineMigrations[server/timeline_migrations]
  MainMigrations[server/migrations]
  Roomserver[roomserver]
  TimelinePort[timeline port]
  TimelineService[timelineservice]
  OutboxWorker[outboxworker]
  Kafka[(Kafka wt.room.timeline.v1)]
  Recovery[recovery]

  MainMigrations --> MainPG
  TimelineMigrations --> TimelinePG
  Roomserver --> TimelinePort
  TimelinePort -->|local mode| TimelinePG
  TimelinePort -->|rpc mode| TimelineService
  TimelineService --> TimelinePG
  OutboxWorker -->|claim pending/publishing| TimelinePG
  OutboxWorker --> Kafka
  Recovery -->|Kafka replay| Kafka
  Recovery -->|unpublished gap| TimelinePort
```

Phase 10 does not migrate old `room_timeline_outbox` rows. The old main-database table remains fallback/shadow data. When `TIMELINE_DATABASE_URL` is configured but unavailable, timeline writes fail closed: accepted controls are not broadcast, accepted request idempotency is not finalized, and recovery must not complete a new authority epoch from an unavailable unpublished-outbox reader.

`pending` and `publishing` rows are recovery-gap state and must not be deleted by cleanup jobs. Published-row retention is a later operations decision.

## Not Phase 10 Goals

- Kafka is not the online broadcast final hop.
- Kafka is not the command ingress log.
- JetStream is not enabled.
- Device-level presence management and full multi-device UI are not complete.
- WebSocket connection objects never move between instances.
- A second PostgreSQL server/container is not required; the pilot uses a second database inside the existing PostgreSQL server/container.
- `room authority` is not extracted to a separate microservice.
- Kratos, go-zero, and go-kit are not adopted as the main framework.
