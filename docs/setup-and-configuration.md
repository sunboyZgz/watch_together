# Setup And Configuration

## Local Infrastructure

Start the local PostgreSQL, Redis, NATS, Kafka, and MinIO services from the server directory:

```bash
cd server
docker compose up -d postgres redis nats kafka minio minio-init
```

Default local endpoints:

```text
PostgreSQL: 127.0.0.1:5432
Redis:      127.0.0.1:6380
NATS:       127.0.0.1:4222
Kafka:      127.0.0.1:9092
MinIO API:  127.0.0.1:9100
MinIO UI:   127.0.0.1:9101
```

Run the main database migration set with the server Makefile:

```bash
cd server
make migration-up
```

The main migration set intentionally has no business owner schema after Phase 27. It can still be applied to the main database for migration bookkeeping, but `users`, `rooms`, media tables, progress, and timeline outbox are created only by their owner migration directories.

Owner database migrations require their owner URLs:

```bash
cd server
docker compose --profile app up -d postgres identity-postgres-init room-postgres-init media-postgres-init progress-postgres-init timeline-postgres-init
IDENTITY_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_identity_dev?sslmode=disable make identity-migration-up
ROOM_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_room_dev?sslmode=disable make room-migration-up
MEDIA_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_media_dev?sslmode=disable make media-migration-up
PROGRESS_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_progress_dev?sslmode=disable make progress-migration-up
TIMELINE_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_timeline_dev?sslmode=disable make timeline-migration-up
```

The `*-postgres-init` services create separate databases inside the same local PostgreSQL container. They do not start additional PostgreSQL software systems.

## Server Configuration Loading

Both `roomserver` and `mediactl` load configuration from `server/` in this order:

1. Code defaults.
2. `server/.env`
3. `server/.env.local`
4. `server/.env.<APP_ENV>`
5. `server/.env.<APP_ENV>.local`
6. Current process environment variables.

Later file merges override earlier defaults, and process environment variables have the final say through Viper's env binding. If `APP_ENV` is not set, it defaults to `local`.

Create a local config file from the template:

```bash
cd server
cp .env.example .env.local
```

## Server Runtime Keys

Important `roomserver` keys:

```text
APP_ENV=local
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
LOG_LEVEL=debug
SERVER_INSTANCE_ID=
SERVER_EDGE_MODE=session_gateway
ROOM_RUNTIME_MODE=distributed_authority
DATABASE_URL=
IDENTITY_DATABASE_URL=
ROOM_DATABASE_URL=
MEDIA_DATABASE_URL=
PROGRESS_DATABASE_URL=
TIMELINE_POSTGRES_DB=anime_watch_timeline_dev
TIMELINE_DATABASE_URL=
DEBUG_SYNC=true
AUTH_JWT_SECRET=<secret>
AUTH_ACCESS_TOKEN_TTL_HOURS=24
REDIS_ADDR=127.0.0.1:6380
REDIS_PASSWORD=watch_together_redis_dev
REDIS_DB=0
REDIS_REQUIRED=true
WS_BROADCAST_CONCURRENCY_LIMIT=64
WS_BROADCAST_TIMEOUT_MS=5000
WS_BROADCAST_ENQUEUE_TIMEOUT_MS=3000
WS_CLIENT_OUTBOX_CAPACITY=64
WS_MAX_CONNECTIONS=0
ROOM_MAX_CLIENTS=0
WS_SEEK_MIN_INTERVAL_MS=250
CONTROL_IDEMPOTENCY_TTL_MS=600000
PRESENCE_LEASE_TTL_MS=45000
PRESENCE_REFRESH_INTERVAL_MS=15000
WS_CROSS_INSTANCE_BROADCAST_ENABLED=false
WS_EVENT_BUS=nats_core
NATS_URL=nats://127.0.0.1:4222
NATS_NAME=watch-together-roomserver
NATS_SUBJECT_ROOM_BROADCAST=wt.room.broadcast.v1
NATS_SUBJECT_ROOM_CONTROL=wt.room.control.v1
KAFKA_BROKERS=127.0.0.1:9092
KAFKA_CLIENT_ID=watch-together-roomserver
KAFKA_TOPIC_ROOM_TIMELINE=wt.room.timeline.v1
KAFKA_TOPIC_ROOM_CONTROL_RESULT=wt.room.control_result.v1
KAFKA_TOPIC_ROOM_MEMBERSHIP=wt.room.membership.v1
KAFKA_DERIVED_CONSUMER_GROUP_ID=watch-together-derived-workers
KAFKA_DERIVED_WORKER_POLL_INTERVAL_MS=1000
OUTBOX_WORKER_BATCH_SIZE=50
OUTBOX_WORKER_POLL_INTERVAL_MS=1000
AUTHORITY_RENEW_INTERVAL_MS=10000
AUTHORITY_TAKEOVER_SCAN_INTERVAL_MS=30000
AUTHORITY_RECOVERY_TIMEOUT_MS=5000
KAFKA_REPLAY_TIMEOUT_MS=1000
METRICS_ENABLED=true
METRICS_ADDR=
METRICS_PATH=/metrics
READINESS_PATH=/readyz
SERVICE_NAME=watch-together-roomserver
SERVICE_VERSION=dev
INTERNAL_RPC_ENABLED=false
INTERNAL_RPC_ADDR=:8090
INTERNAL_RPC_PATH_PREFIX=/internal.rpc
INTERNAL_RPC_TIMEOUT_MS=1000
INTERNAL_RPC_AUTH_TOKEN=
SERVICE_DISCOVERY_MODE=static
IDENTITY_SERVICE_MODE=rpc
IDENTITY_SERVICE_ADDR=http://127.0.0.1:8093
ROOM_SERVICE_MODE=rpc
ROOM_SERVICE_ADDR=http://127.0.0.1:8094
MEDIA_SERVICE_MODE=rpc
MEDIA_SERVICE_ADDR=http://127.0.0.1:8090
PROGRESS_SERVICE_MODE=rpc
PROGRESS_SERVICE_ADDR=http://127.0.0.1:8095
HOME_SERVICE_MODE=rpc
HOME_SERVICE_ADDR=http://127.0.0.1:8096
TIMELINE_SERVICE_MODE=rpc
TIMELINE_SERVICE_ADDR=http://127.0.0.1:8091
AUTHORITY_SERVICE_MODE=rpc
AUTHORITY_SERVICE_ADDR=http://127.0.0.1:8092
AUTHORITY_LEASE_INSTANCE_ID=roomauthorityservice-1
OTEL_TRACING_ENABLED=false
OTEL_SERVICE_NAME=watch-together-roomserver
OTEL_EXPORTER_OTLP_ENDPOINT=
OTEL_TRACE_SAMPLE_RATIO=0.1
MEDIA_DELIVERY_MODE=signed_redirect
MEDIA_PLAYBACK_SIGNING_SECRET=<secret>
MEDIA_PLAYBACK_URL_TTL_SECONDS=7200
MEDIA_PUBLIC_BASE_URL=http://127.0.0.1:9000/media/tmp
MEDIA_INTERNAL_BASE_URL=
MEDIA_STORAGE_ENDPOINT=
MEDIA_STORAGE_BUCKET=
MEDIA_STORAGE_REGION=
MEDIA_STORAGE_ACCESS_KEY_ID=
MEDIA_STORAGE_SECRET_ACCESS_KEY=
MEDIA_STORAGE_FORCE_PATH_STYLE=true
```

`SERVER_INSTANCE_ID` is optional metadata surfaced in startup logs and `/healthz` headers. Set it to a unique value per roomserver replica during multi-instance experiments.

`ROOM_RUNTIME_MODE` supports `local_process` and `distributed_authority`, but the recommended local and production path is `distributed_authority` through compose. `distributed_authority` requires `SERVER_INSTANCE_ID`, Redis, NATS, Kafka broker config, and `WS_CROSS_INSTANCE_BROADCAST_ENABLED=true`; it no longer requires a main `DATABASE_URL` when room, timeline, and authority dependencies are RPC-backed.

After Phase 27, owner service URLs are required by owner services and are not fallback knobs. `cmd/identityservice` requires `IDENTITY_DATABASE_URL`, `cmd/roomservice` requires `ROOM_DATABASE_URL`, `cmd/mediaservice` requires `MEDIA_DATABASE_URL`, `cmd/progressservice` requires `PROGRESS_DATABASE_URL`, and `cmd/timelineservice` plus `cmd/outboxworker` require `TIMELINE_DATABASE_URL`. `apigateway`, the default `roomserver` session gateway, and `roomauthorityservice` do not receive `DATABASE_URL` or owner database URLs in compose. They call RPC services instead.

If an owner database is unavailable, the owning service returns `not_ready` and callers receive the existing service-unavailable envelope. Accepted distributed controls must not fall back to a main database; timeline unavailable means no accepted broadcast and no accepted idempotency finalization.

If `REDIS_ADDR` is empty, the Redis-backed room state cache is disabled.

`WS_CROSS_INSTANCE_BROADCAST_ENABLED` controls Phase 3 cross-instance WebSocket fan-out. It defaults to `false`. When set to `true`, `WS_EVENT_BUS` currently accepts only `nats_core`, and `roomserver` publishes local WebSocket broadcast envelopes to `NATS_SUBJECT_ROOM_BROADCAST`.

Each `roomserver` instance subscribes directly to the same NATS Core subject. Do not configure a NATS queue group for room broadcasts, because every instance needs the same broadcast message. JetStream is not enabled in this phase, so NATS is not a durable event log or playback authority. `ROOM_RUNTIME_MODE=local_process` and Redis `room_state` snapshot behavior remain unchanged.

If cross-instance broadcast is enabled but NATS cannot be opened, the server logs the connection failure and continues with local-process WebSocket behavior.

In `distributed_authority`, NATS is also used for internal control request/reply through `NATS_SUBJECT_ROOM_CONTROL`. Kafka is used for durable room timeline result logging through PostgreSQL outbox workers and Phase 5 authority recovery replay. Online WebSocket fan-out still uses NATS Core and local connection tables.

Authority recovery settings:

- `AUTHORITY_RENEW_INTERVAL_MS` controls how often the current instance renews active authority leases for rooms it owns.
- `AUTHORITY_TAKEOVER_SCAN_INTERVAL_MS` controls the low-frequency background scan for active or grace-period rooms whose authority lease is missing or expired.
- `AUTHORITY_RECOVERY_TIMEOUT_MS` bounds the whole recovery attempt.
- `KAFKA_REPLAY_TIMEOUT_MS` bounds canonical timeline replay for one room.

When a Redis authority lease expires, Phase 5 recovery can claim `status=recovering` with a higher epoch, replay `wt.room.timeline.v1`, merge same-room `pending` and `publishing` outbox rows, register recovered playback state locally, then complete a new active authority lease. Kafka remains a result log, not a command ingress path; Redis `room_state` remains a latest snapshot cache.

Distributed control hardening settings:

- `CONTROL_IDEMPOTENCY_TTL_MS` controls how long Redis keeps recent control `requestId` outcomes. Duplicate accepted requests return the same accepted envelope; duplicate pending requests return `room authority processing`.
- `PRESENCE_LEASE_TTL_MS` controls how long a Redis user-level presence entry remains valid without refresh.
- `PRESENCE_REFRESH_INTERVAL_MS` controls how often heartbeat acks refresh presence while the WebSocket is healthy.

Presence is runtime state only. It is stored in Redis, broadcast as user-level `room_presence` snapshots, and is not PostgreSQL membership or a Kafka timeline event.

Distributed seek rate limiting:

- `WS_SEEK_MIN_INTERVAL_MS` still controls the minimum interval between accepted seek controls.
- `local_process` uses process-local throttling.
- `distributed_authority` uses Redis `wt:room:control_rate:{roomId}:seek:v1`, so forwarded controls and recovered authority instances share the same limit.

Observability settings:

- `METRICS_ENABLED` controls whether `METRICS_PATH` is registered.
- `METRICS_ADDR` is optional for worker processes. Leave it empty for `roomserver`, which exposes metrics on the main HTTP server; set it for workers such as `:9091`.
- `METRICS_PATH` defaults to `/metrics` and exposes Prometheus metrics.
- `READINESS_PATH` defaults to `/readyz` and reports dependency readiness as JSON.
- `/healthz` remains lightweight liveness and keeps the runtime headers.

Service foundation settings:

- `SERVICE_NAME` and `SERVICE_VERSION` identify the current process in logs, internal RPC metadata, and traces.
- `INTERNAL_RPC_*` configures optional ConnectRPC service endpoints. `INTERNAL_RPC_AUTH_TOKEN` protects internal calls when configured and is required for production internal RPC.
- `SERVICE_DISCOVERY_MODE=static` is the only supported discovery mode in the current serviceized path.
- `IDENTITY_SERVICE_MODE`, `ROOM_SERVICE_MODE`, `MEDIA_SERVICE_MODE`, `PROGRESS_SERVICE_MODE`, `HOME_SERVICE_MODE`, `TIMELINE_SERVICE_MODE`, and `AUTHORITY_SERVICE_MODE` accept `local` or `rpc`. The default compose path uses `rpc`. `local` is a compatibility path and still requires the relevant owner database URL when durable state is touched.
- `AUTHORITY_SERVICE_MODE=rpc` also requires `AUTHORITY_LEASE_INSTANCE_ID`, which is the Redis authority lease owner claimed during HTTP room bootstrap for the authority service. In `roomserver`, authority RPC must run with `ROOM_RUNTIME_MODE=distributed_authority`.
- `OTEL_TRACING_ENABLED` turns on OpenTelemetry tracing. `OTEL_EXPORTER_OTLP_ENDPOINT` points at the internal OTLP collector, and `OTEL_TRACE_SAMPLE_RATIO` must be between `0` and `1`.
- Phase 27 makes the local compose and production paths RPC-only for business services and removes main-database shadow tables. Table owners and split rules are documented in [Database Ownership](./database-ownership.md) and enforced from `server/internal/store/db_ownership.yaml`.

See [distributed-architecture.md](./distributed-architecture.md) for the current module map, business flows, and monitoring data flow.

Worker examples:

```bash
cd server
TIMELINE_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_timeline_dev?sslmode=disable APP_ENV=local go run ./cmd/outboxworker
APP_ENV=local go run ./cmd/derivedworker
IDENTITY_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_identity_dev?sslmode=disable APP_ENV=local INTERNAL_RPC_ADDR=:8093 go run ./cmd/identityservice
ROOM_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_room_dev?sslmode=disable APP_ENV=local INTERNAL_RPC_ADDR=:8094 IDENTITY_SERVICE_MODE=rpc IDENTITY_SERVICE_ADDR=http://127.0.0.1:8093 MEDIA_SERVICE_MODE=rpc MEDIA_SERVICE_ADDR=http://127.0.0.1:8090 go run ./cmd/roomservice
MEDIA_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_media_dev?sslmode=disable APP_ENV=local INTERNAL_RPC_ADDR=:8090 go run ./cmd/mediaservice
TIMELINE_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_timeline_dev?sslmode=disable APP_ENV=local INTERNAL_RPC_ADDR=:8091 go run ./cmd/timelineservice
```

Identity database helpers:

```bash
cd server
IDENTITY_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_identity_dev?sslmode=disable make identity-migration-up
go run ./cmd/identitydbsync --source-database-url postgres://legacy:legacy@127.0.0.1:5432/legacy_main?sslmode=disable --target-database-url postgres://app:app@127.0.0.1:5432/anime_watch_identity_dev?sslmode=disable --dry-run
go run ./cmd/identitydbsync --source-database-url postgres://legacy:legacy@127.0.0.1:5432/legacy_main?sslmode=disable --target-database-url postgres://app:app@127.0.0.1:5432/anime_watch_identity_dev?sslmode=disable --verify-only
```

Room database helpers:

```bash
cd server
ROOM_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_room_dev?sslmode=disable make room-migration-up
go run ./cmd/roomdbsync --source-database-url postgres://legacy:legacy@127.0.0.1:5432/legacy_main?sslmode=disable --target-database-url postgres://app:app@127.0.0.1:5432/anime_watch_room_dev?sslmode=disable --dry-run
go run ./cmd/roomdbsync --source-database-url postgres://legacy:legacy@127.0.0.1:5432/legacy_main?sslmode=disable --target-database-url postgres://app:app@127.0.0.1:5432/anime_watch_room_dev?sslmode=disable --verify-only
```

Generate and lint internal RPC contracts:

```bash
cd server
make proto-lint
make proto-generate
```

Run the Phase 8 verification loop on Windows:

```powershell
cd server
.\scripts\verify_phase8.ps1
```

The script runs `buf lint`, `buf generate`, checks generated code for drift, runs focused Go targets, runs `go test ./...`, and finishes with `git diff --check`.

Run the Phase 9 verification loop on Windows:

```powershell
cd server
.\scripts\verify_phase9.ps1
```

Run the Phase 10 verification loop on Windows:

```powershell
cd server
.\scripts\verify_phase10.ps1
```

Run the Phase 11 verification loop on Windows:

```powershell
cd server
.\scripts\verify_phase11.ps1
```

Run the Phase 16 verification loop on Windows:

```powershell
cd server
.\scripts\verify_phase16.ps1
```

Run the Phase 23 verification loop on Windows for the historical full-RPC, multi-database baseline:

```powershell
cd server
.\scripts\verify_phase23.ps1
```

Add `-RunSmoke` to start the local full-RPC compose app path, apply all owner database migrations, seed deterministic media, and prove HTTP, WebSocket control, authority RPC, timeline RPC, and owner database writes:

```powershell
cd server
.\scripts\verify_phase23.ps1 -RunSmoke -ResetVolumes -DownAfterRun
```

Run the Phase 25 verification loop after authority RPC changes. The default run keeps the Phase 23 baseline and adds authority engine tests plus prod RPC/default and local rollback compose validation:

```powershell
cd server
.\scripts\verify_phase25.ps1
```

Add `-RunSmoke` to run the full-RPC multi-database E2E smoke after the authority engine and compose checks:

```powershell
cd server
.\scripts\verify_phase25.ps1 -RunSmoke -ResetVolumes -DownAfterRun
```

Run the Phase 26 verification loop after gateway/session edge changes. The default run keeps the Phase 23 baseline and adds API gateway/session gateway route and compose guards:

```powershell
cd server
.\scripts\verify_phase26.ps1
```

Add `-RunSmoke` to run the full-RPC smoke through nginx public REST -> `apigateway` and `/ws` -> `roomserver`:

```powershell
cd server
.\scripts\verify_phase26.ps1 -RunSmoke -ResetVolumes -DownAfterRun
```

Run the Phase 27 verification loop after removing main-database shadow tables. The default run includes Phase 26 verification plus RPC-only database boundary guards:

```powershell
cd server
.\scripts\verify_phase27.ps1
```

Add `-RunSmoke` to run the full-RPC smoke and assert that main-database owner tables do not exist:

```powershell
cd server
.\scripts\verify_phase27.ps1 -RunSmoke -ResetVolumes -DownAfterRun
```

Legacy media database import:

```bash
cd server
go run ./cmd/mediadbsync --source-database-url postgres://legacy:legacy@127.0.0.1:5432/legacy_main?sslmode=disable --target-database-url postgres://app:app@127.0.0.1:5432/anime_watch_media_dev?sslmode=disable --dry-run
go run ./cmd/mediadbsync --source-database-url postgres://legacy:legacy@127.0.0.1:5432/legacy_main?sslmode=disable --target-database-url postgres://app:app@127.0.0.1:5432/anime_watch_media_dev?sslmode=disable --verify-only
go run ./cmd/mediadbsync --source-database-url postgres://legacy:legacy@127.0.0.1:5432/legacy_main?sslmode=disable --target-database-url postgres://app:app@127.0.0.1:5432/anime_watch_media_dev?sslmode=disable --batch-size 200
```

The command requires an explicit legacy source URL and writes/verifies the target media database. It syncs `media_tags`, `media_seasons`, `media_episodes`, `media_season_tags`, and `media_episode_variants` in dependency order using upsert.

Timeline database migrations:

```bash
cd server
TIMELINE_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_timeline_dev?sslmode=disable make timeline-migration-up
TIMELINE_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_timeline_dev?sslmode=disable make timeline-migration-version
```

There is no default timeline history sync command. A clean timeline database starts from an empty `room_timeline_outbox` table unless you explicitly import legacy data outside the normal Phase 27 path.

Run the recommended local serviceized app stack through compose:

```bash
cd server
docker compose --profile app up -d --build
```

The `app` profile starts `apigateway`, `roomserver`, `identityservice`, `roomservice`, `mediaservice`, `progressservice`, `homecompositionservice`, `timelineservice`, `roomauthorityservice`, the timeline workers, and the local infrastructure. Nginx routes public REST to `apigateway` and `/ws` to `roomserver`. `roomserver` uses compose-only override variables so `.env` files copied for bare `go run` do not silently change the compose default:

```text
ROOMSERVER_EDGE_MODE=session_gateway
ROOMSERVER_RUNTIME_MODE=distributed_authority
ROOMSERVER_IDENTITY_SERVICE_MODE=rpc
ROOMSERVER_ROOM_SERVICE_MODE=rpc
ROOMSERVER_MEDIA_SERVICE_MODE=rpc
ROOMSERVER_PROGRESS_SERVICE_MODE=rpc
ROOMSERVER_HOME_SERVICE_MODE=rpc
ROOMSERVER_TIMELINE_SERVICE_MODE=rpc
ROOMSERVER_AUTHORITY_SERVICE_MODE=rpc
ROOMSERVER_AUTHORITY_LEASE_INSTANCE_ID=roomauthorityservice-1
```

Local service readiness endpoints:

```text
http://127.0.0.1:8080/readyz  public nginx -> apigateway
http://127.0.0.1:8097/readyz  apigateway
http://127.0.0.1:8098/readyz  roomserver session gateway
http://127.0.0.1:8090/readyz  mediaservice
http://127.0.0.1:8091/readyz  timelineservice
http://127.0.0.1:8092/readyz  roomauthorityservice
http://127.0.0.1:8093/readyz  identityservice
http://127.0.0.1:8094/readyz  roomservice
http://127.0.0.1:8095/readyz  progressservice
http://127.0.0.1:8096/readyz  homecompositionservice
```

Explicit compose compatibility mode still uses owner databases; it is not a main-database fallback:

```bash
cd server
ROOMSERVER_RUNTIME_MODE=local_process \
ROOMSERVER_IDENTITY_SERVICE_MODE=local \
ROOMSERVER_ROOM_SERVICE_MODE=local \
ROOMSERVER_MEDIA_SERVICE_MODE=local \
ROOMSERVER_TIMELINE_SERVICE_MODE=local \
ROOMSERVER_AUTHORITY_SERVICE_MODE=local \
ROOMSERVER_IDENTITY_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_identity_dev?sslmode=disable \
ROOMSERVER_ROOM_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_room_dev?sslmode=disable \
ROOMSERVER_MEDIA_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_media_dev?sslmode=disable \
ROOMSERVER_TIMELINE_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_timeline_dev?sslmode=disable \
docker compose --profile app up -d --build
```

Existing legacy main-database media data is not copied into the media database automatically. Use `go run ./cmd/mediadbsync --source-database-url ...` for an explicit import, or use the smoke script which seeds a deterministic local episode into `anime_watch_media_dev`.

Phase 27's verification script is the preferred end-to-end local validation. It applies main and owner migrations, writes users/rooms/progress/timeline data through the public Android-facing routes, asserts those writes landed in the owning databases, and asserts the main database owner tables are absent. The smoke runs through `apigateway + roomserver(ws)`; the script starts infrastructure, init jobs, migrations, and services in separate steps and prints compose diagnostics on failure.

Run the optional RPC pilot stack through compose for observability, load, and service experiments:

```bash
cd server
MEDIA_SERVICE_MODE=rpc \
TIMELINE_SERVICE_MODE=rpc \
OTEL_TRACING_ENABLED=true \
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4318 \
docker compose --profile rpc-pilot up -d --build
```

`rpc-pilot` starts `roomserver-rpc-pilot`, `identityservice`, `roomservice`, `mediaservice`, `progressservice`, `homecompositionservice`, `timelineservice`, `roomauthorityservice`, an OTLP collector, and the owner database init jobs. In this profile `roomserver-rpc-pilot` sets `ROOM_RUNTIME_MODE=distributed_authority`, `AUTHORITY_SERVICE_MODE=rpc`, `AUTHORITY_SERVICE_ADDR=http://roomauthorityservice:8090`, and `AUTHORITY_LEASE_INSTANCE_ID=roomauthorityservice-1`. `identityservice` uses `IDENTITY_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_identity_dev?sslmode=disable` in compose. `roomservice` calls identity and media through RPC and uses `ROOM_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_room_dev?sslmode=disable`. `mediaservice` uses `MEDIA_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_media_dev?sslmode=disable` in compose. `progressservice` uses `PROGRESS_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_progress_dev?sslmode=disable`, and `homecompositionservice` composes identity, progress, and media only through RPC. `timelineservice` and `outboxworker` use `TIMELINE_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_timeline_dev?sslmode=disable`. The normal local compose `app` path uses the same identity, room, media, progress, home, timeline, and authority RPC boundaries, while `rpc-pilot` remains useful for observability and load experiments. Local service modes are compatibility paths and still require owner DB URLs; they do not use a single main business database.

Local app RPC smoke checks:

```bash
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8093/readyz
curl http://127.0.0.1:8094/readyz
curl http://127.0.0.1:8090/readyz
curl http://127.0.0.1:8091/readyz
curl http://127.0.0.1:8092/readyz
curl -H "Authorization: Bearer <token>" "http://127.0.0.1:8080/media/tags"
curl -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"mediaItemId":"<episode-id>"}' http://127.0.0.1:8080/rooms
```

`apigateway` readiness actively probes identity, room, media, progress, home, and authority RPC. `roomserver` readiness actively probes identity, room, media, progress, home, timeline, and authority RPC service readiness in RPC mode. `identityservice`, `roomservice`, `mediaservice`, `progressservice`, and `timelineservice` readiness ping their selected PostgreSQL database at request time. `roomservice`, `progressservice`, and `homecompositionservice` also check their configured RPC dependencies. `timelineservice` reports Kafka reader state as `ok`, `disabled`, or `unavailable` depending on broker configuration and reader initialization. `roomauthorityservice` readiness checks Redis, NATS, room RPC, timeline RPC, and internal RPC availability.

For production-like local debugging, start the compose app profile. Bare process debugging requires starting the RPC services separately and providing the same service addresses used by compose. Starting only `cmd/roomserver` is a compatibility path for WebSocket/session code, not the recommended full application path:

```bash
cd server
APP_ENV=local go run ./cmd/roomserver
```

Health check:

```text
GET http://127.0.0.1:8080/healthz
GET http://127.0.0.1:8080/readyz
GET http://127.0.0.1:8080/metrics
```

The response body remains `ok`. Health responses include `X-Watch-Together-Room-Runtime` and include `X-Watch-Together-Instance-ID` when `SERVER_INSTANCE_ID` is configured.

## Mediactl Configuration

`mediactl` uses the same file loading mechanism. Important keys:

```text
DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_dev?sslmode=disable
MEDIA_STORAGE_DRIVER=local
MEDIA_LOCAL_ROOT=../media/tmp
MEDIA_PUBLIC_BASE_URL=http://127.0.0.1:9000/media/tmp
MEDIA_OBJECT_KEY_PREFIX=media
MEDIA_STORAGE_ENDPOINT=
MEDIA_STORAGE_BUCKET=
MEDIA_STORAGE_REGION=
MEDIA_STORAGE_ACCESS_KEY_ID=
MEDIA_STORAGE_SECRET_ACCESS_KEY=
MEDIA_STORAGE_FORCE_PATH_STYLE=true
FFMPEG_BIN=ffmpeg
FFPROBE_BIN=ffprobe
```

`MEDIA_STORAGE_DRIVER` supports `local`, `minio`, and `s3`.

## Android Configuration

Android config is compiled into `BuildConfig` from `android/app/build.gradle.kts`. Values can be overridden in `android/local.properties` or Gradle properties.

Current flavors:

- `local`: defaults to emulator loopback with `10.0.2.2`.
- `prod`: defaults to the configured public server address in Gradle.

Common overrides in `android/local.properties`:

```properties
LOCAL_API_BASE_URL=http://10.0.2.2:8080
LOCAL_WS_BASE_URL=ws://10.0.2.2:8080/ws
LOCAL_MEDIA_BASE_URL=http://10.0.2.2:9000/media/tmp
LOCAL_REWRITE_LOOPBACK_MEDIA_URLS=true
PROD_API_BASE_URL=https://example.com
PROD_WS_BASE_URL=wss://example.com/ws
PROD_REWRITE_LOOPBACK_MEDIA_URLS=false
```

Build examples:

```bash
cd android
./gradlew installLocalDebug
./gradlew assembleProdRelease
```

On Windows, when Java is available through Android Studio but not on `PATH`, run unit tests with:

```powershell
cd android
$env:JAVA_HOME='C:\Program Files\Android\Android Studio\jbr'
./gradlew testLocalDebugUnitTest
```

## Production Compose

Production deployment uses `server/compose.prod.yaml`. Its intended boundary is:

- Only Nginx exposes HTTP to the public network.
- Go `roomserver`, workers, PostgreSQL, Redis, Kafka, NATS, and MinIO stay on the Docker network.
- NATS stays on the Docker network and is used for WebSocket fan-out and internal control forwarding.
- Kafka stays on the Docker network and stores durable timeline result events.
- `/metrics` should be scraped from an internal network or blocked at the public reverse proxy.
- Media delivery defaults to `MEDIA_DELIVERY_MODE=nginx_auth_request`.
- Production compose defaults identity, room, media, progress, home, timeline, and authority to RPC services.
- `apigateway` runs by default and owns Android-facing REST/BFF routing.
- `roomauthorityservice` runs by default and owns authority decisions through `authority.Engine`; `roomserver` remains the WebSocket/session gateway and keeps local WebSocket connection tables.
- Authority compatibility mode is explicit: set `AUTHORITY_SERVICE_MODE=local`, `ROOM_RUNTIME_MODE=local_process`, `WS_CROSS_INSTANCE_BROADCAST_ENABLED=false`, and the required owner DB URLs for any local durable adapters. No Android HTTP/WebSocket payload changes are involved.

See [server/deploy/README.md](../server/deploy/README.md) for deployment-specific commands and Nginx details.
