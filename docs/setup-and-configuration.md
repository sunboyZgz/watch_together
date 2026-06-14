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

Run migrations with the server Makefile:

```bash
cd server
make migration-up
```

The migration tooling expects `DATABASE_URL` to point at the target database.

Phase 9 can run media-owned tables in an independent media database. The default local fallback leaves `MEDIA_DATABASE_URL` empty and keeps media tables in `DATABASE_URL`. To use the media database pilot in the local compose stack:

```bash
cd server
docker compose --profile services up -d postgres media-postgres-init
MEDIA_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_media_dev?sslmode=disable make media-migration-up
MEDIA_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_media_dev?sslmode=disable go run ./cmd/mediadbsync
```

`media-postgres-init` creates `anime_watch_media_dev` inside the same local PostgreSQL container. It does not start a second PostgreSQL server.

Phase 10 can run timeline-owned outbox rows in an independent timeline database. The default local fallback leaves `TIMELINE_DATABASE_URL` empty and keeps `room_timeline_outbox` in `DATABASE_URL`. To use the timeline database pilot in the local compose stack:

```bash
cd server
docker compose --profile services up -d postgres timeline-postgres-init
TIMELINE_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_timeline_dev?sslmode=disable make timeline-migration-up
```

`timeline-postgres-init` creates `anime_watch_timeline_dev` inside the same local PostgreSQL container. It does not start a second PostgreSQL server. Phase 10 starts the independent timeline database empty; old `room_timeline_outbox` rows are not migrated.

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
ROOM_RUNTIME_MODE=local_process
DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_dev?sslmode=disable
MEDIA_DATABASE_URL=
TIMELINE_POSTGRES_DB=anime_watch_timeline_dev
TIMELINE_DATABASE_URL=
DEBUG_SYNC=true
AUTH_JWT_SECRET=<secret>
AUTH_ACCESS_TOKEN_TTL_HOURS=24
REDIS_ADDR=127.0.0.1:6380
REDIS_PASSWORD=watch_together_redis_dev
REDIS_DB=0
REDIS_REQUIRED=false
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
KAFKA_BROKERS=
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
IDENTITY_SERVICE_MODE=local
IDENTITY_SERVICE_ADDR=
MEDIA_SERVICE_MODE=local
MEDIA_SERVICE_ADDR=
TIMELINE_SERVICE_MODE=rpc
TIMELINE_SERVICE_ADDR=http://127.0.0.1:8091
AUTHORITY_SERVICE_MODE=local
AUTHORITY_SERVICE_ADDR=
AUTHORITY_LEASE_INSTANCE_ID=
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

`ROOM_RUNTIME_MODE` supports `local_process` and `distributed_authority`. `local_process` keeps active room playback authority, WebSocket connection tables, control deduplication, and seek rate limiting in one Go process. `distributed_authority` requires `SERVER_INSTANCE_ID`, PostgreSQL, Redis, NATS, Kafka broker config, and `WS_CROSS_INSTANCE_BROADCAST_ENABLED=true`.

If `DATABASE_URL` is empty or PostgreSQL cannot be opened, database-backed HTTP endpoints return `503`. If `IDENTITY_DATABASE_URL` is set, local identity mode and `cmd/identityservice` use it for identity-owned tables and `/readyz` reports that dependency as `identity_postgres`; if it is empty, identity uses the main `DATABASE_URL`. In `IDENTITY_SERVICE_MODE=rpc`, `roomserver` does not connect to the identity database directly and rejects a non-empty `IDENTITY_DATABASE_URL`; compose app/prod paths pass the identity URL only to `cmd/identityservice`. Identity database migrations live under `server/identity_migrations`, and `cmd/identitydbsync` can copy/verify the main-database shadow table.

If `ROOM_DATABASE_URL` is set, local room mode and `cmd/roomservice` use it for `rooms` and `room_members`; if it is empty, room storage uses the main `DATABASE_URL`. In `ROOM_SERVICE_MODE=rpc`, `roomserver` does not connect to the room database directly and rejects a non-empty `ROOM_DATABASE_URL`; compose app/prod paths pass the room URL only to `cmd/roomservice`. Room database migrations live under `server/room_migrations`, and `cmd/roomdbsync` can copy/verify the main-database shadow tables.

If `MEDIA_DATABASE_URL` is set, local media mode and `cmd/mediaservice` use it for media-owned tables and `/readyz` reports that dependency as `media_postgres`; if it is empty, media uses the main `DATABASE_URL`. In `MEDIA_SERVICE_MODE=rpc`, `roomserver` does not connect to the media database directly.

If `TIMELINE_DATABASE_URL` is set, local timeline mode, `cmd/timelineservice`, and `cmd/outboxworker` use it for `room_timeline_outbox` and `/readyz` reports that dependency as `timeline_postgres`; if it is empty, timeline uses the main `DATABASE_URL`. In `TIMELINE_SERVICE_MODE=rpc`, `roomserver` does not connect to the timeline database directly and rejects a non-empty `TIMELINE_DATABASE_URL`; compose app/prod paths default to this RPC mode and leave the roomserver timeline DB URL empty. Set `TIMELINE_SERVICE_MODE=local` plus `ROOMSERVER_TIMELINE_DATABASE_URL` only for explicit compose rollback. Timeline is fail-closed: if the configured timeline RPC or local timeline database is unavailable, accepted distributed controls must not fall back to the main database or broadcast an accepted result.

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
- `IDENTITY_SERVICE_MODE`, `ROOM_SERVICE_MODE`, `MEDIA_SERVICE_MODE`, `PROGRESS_SERVICE_MODE`, `HOME_SERVICE_MODE`, `TIMELINE_SERVICE_MODE`, and `AUTHORITY_SERVICE_MODE` accept `local` or `rpc`. `local` keeps current in-process adapters. `rpc` calls the matching `*_SERVICE_ADDR`.
- `AUTHORITY_SERVICE_MODE=rpc` also requires `AUTHORITY_LEASE_INSTANCE_ID`, which is the Redis authority lease owner claimed during HTTP room bootstrap for the authority service. In `roomserver`, authority RPC must run with `ROOM_RUNTIME_MODE=distributed_authority`.
- `OTEL_TRACING_ENABLED` turns on OpenTelemetry tracing. `OTEL_EXPORTER_OTLP_ENDPOINT` points at the internal OTLP collector, and `OTEL_TRACE_SAMPLE_RATIO` must be between `0` and `1`.
- Phase 9 keeps the old single-database fallback but can put media-owned tables in an independent PostgreSQL database through `MEDIA_DATABASE_URL`. Phase 10 does the same for timeline-owned outbox rows through `TIMELINE_DATABASE_URL`. Phase 11 makes timeline RPC the compose default while preserving `local` as an explicit single-process rollback path, Phase 12 moves result timeline ownership into `cmd/timelineservice`, Phase 13 documents the authority boundary, Phase 14 adds a non-default `roomauthorityservice` RPC pilot under `rpc-pilot`, Phase 15 hardens that pilot with dynamic readiness, metrics, stable failure tests, and lease identity naming, Phase 16 makes local compose `app` default to media/timeline/authority RPC, Phase 17/18 adds identity RPC to the same local app baseline, Phase 22 gives identity an independent database by default in compose, and Phase 25 makes production compose default to authority RPC through `authority.Engine`. Table owners and future split rules are documented in [Database Ownership](./database-ownership.md) and enforced from `server/internal/store/db_ownership.yaml`.

See [distributed-architecture.md](./distributed-architecture.md) for the current module map, business flows, and monitoring data flow.

Worker examples:

```bash
cd server
APP_ENV=local go run ./cmd/outboxworker
APP_ENV=local go run ./cmd/derivedworker
APP_ENV=local INTERNAL_RPC_ADDR=:8093 go run ./cmd/identityservice
APP_ENV=local INTERNAL_RPC_ADDR=:8094 IDENTITY_SERVICE_MODE=local MEDIA_SERVICE_MODE=local go run ./cmd/roomservice
APP_ENV=local INTERNAL_RPC_ADDR=:8090 go run ./cmd/mediaservice
APP_ENV=local INTERNAL_RPC_ADDR=:8091 go run ./cmd/timelineservice
```

Identity database helpers:

```bash
cd server
IDENTITY_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_identity_dev?sslmode=disable make identity-migration-up
go run ./cmd/identitydbsync --dry-run
go run ./cmd/identitydbsync --verify-only
```

Room database helpers:

```bash
cd server
ROOM_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_room_dev?sslmode=disable make room-migration-up
go run ./cmd/roomdbsync --dry-run
go run ./cmd/roomdbsync --verify-only
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

Run the Phase 23 verification loop on Windows. This is the recommended full-RPC, multi-database baseline gate:

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

Media database sync:

```bash
cd server
go run ./cmd/mediadbsync --dry-run
go run ./cmd/mediadbsync --verify-only
go run ./cmd/mediadbsync --batch-size 200
```

The command reads from `DATABASE_URL` by default and writes/verifies `MEDIA_DATABASE_URL`. It syncs `media_tags`, `media_seasons`, `media_episodes`, `media_season_tags`, and `media_episode_variants` in dependency order using upsert.

Timeline database migrations:

```bash
cd server
TIMELINE_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_timeline_dev?sslmode=disable make timeline-migration-up
TIMELINE_DATABASE_URL=postgres://app:app@127.0.0.1:5432/anime_watch_timeline_dev?sslmode=disable make timeline-migration-version
```

There is no timeline history sync command in Phase 10. The independent timeline database starts from an empty `room_timeline_outbox` table, and the old main-database outbox remains fallback/shadow data.

Run the recommended local serviceized app stack through compose:

```bash
cd server
docker compose --profile app up -d --build
```

The `app` profile starts `roomserver`, `identityservice`, `roomservice`, `mediaservice`, `progressservice`, `homecompositionservice`, `timelineservice`, `roomauthorityservice`, the timeline workers, and the local infrastructure. `roomserver` uses compose-only override variables so `.env` files copied for bare `go run` do not silently change the compose default:

```text
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
http://127.0.0.1:8080/readyz  roomserver through nginx
http://127.0.0.1:8090/readyz  mediaservice
http://127.0.0.1:8091/readyz  timelineservice
http://127.0.0.1:8092/readyz  roomauthorityservice
http://127.0.0.1:8093/readyz  identityservice
http://127.0.0.1:8094/readyz  roomservice
http://127.0.0.1:8095/readyz  progressservice
http://127.0.0.1:8096/readyz  homecompositionservice
```

Explicit compose rollback:

```bash
cd server
ROOMSERVER_RUNTIME_MODE=local_process \
ROOMSERVER_IDENTITY_SERVICE_MODE=local \
ROOMSERVER_ROOM_SERVICE_MODE=local \
ROOMSERVER_MEDIA_SERVICE_MODE=local \
ROOMSERVER_TIMELINE_SERVICE_MODE=local \
ROOMSERVER_AUTHORITY_SERVICE_MODE=local \
docker compose --profile app up -d --build
```

Existing main-database media data is not copied into the media database automatically. Use `go run ./cmd/mediadbsync`, or use the Phase 16 smoke script which seeds a deterministic local episode into `anime_watch_media_dev`.

Phase 23's smoke script is the preferred end-to-end local validation. It applies main, identity, room, media, progress, and timeline migrations, writes users/rooms/progress/timeline data through the public Android-facing routes, and asserts those writes landed in the owning databases rather than the main shadow tables. Phase 25 wraps that baseline with authority engine checks for accepted controls, duplicate idempotency, timeline failure rollback, recovery feed failure, and production RPC/default plus rollback compose wiring.

Run the optional RPC pilot stack through compose for observability, load, and service experiments:

```bash
cd server
MEDIA_SERVICE_MODE=rpc \
TIMELINE_SERVICE_MODE=rpc \
OTEL_TRACING_ENABLED=true \
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4318 \
docker compose --profile rpc-pilot up -d --build
```

`rpc-pilot` starts `roomserver-rpc-pilot`, `identityservice`, `roomservice`, `mediaservice`, `progressservice`, `homecompositionservice`, `timelineservice`, `roomauthorityservice`, an OTLP collector, and the owner database init jobs. In this profile `roomserver-rpc-pilot` sets `ROOM_RUNTIME_MODE=distributed_authority`, `AUTHORITY_SERVICE_MODE=rpc`, `AUTHORITY_SERVICE_ADDR=http://roomauthorityservice:8090`, and `AUTHORITY_LEASE_INSTANCE_ID=roomauthorityservice-1`. `identityservice` uses `IDENTITY_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_identity_dev?sslmode=disable` in compose. `roomservice` calls identity and media through RPC and uses `ROOM_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_room_dev?sslmode=disable`. `mediaservice` uses `MEDIA_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_media_dev?sslmode=disable` in compose. `progressservice` uses `PROGRESS_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_progress_dev?sslmode=disable`, and `homecompositionservice` composes identity, progress, and media only through RPC. `timelineservice` and `outboxworker` use `TIMELINE_DATABASE_URL=postgres://app:app@postgres:5432/anime_watch_timeline_dev?sslmode=disable`. The normal local compose `app` path uses the same identity, room, media, progress, home, timeline, and authority RPC boundaries, while `rpc-pilot` remains useful for observability and load experiments. The code still supports local service modes for explicit rollback or bare single-process debugging.

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

`roomserver` readiness actively probes identity, room, media, progress, home, timeline, and authority RPC service readiness in RPC mode. `identityservice`, `roomservice`, `mediaservice`, `progressservice`, and `timelineservice` readiness ping their selected PostgreSQL database at request time. `roomservice`, `progressservice`, and `homecompositionservice` also check their configured RPC dependencies. `timelineservice` reports Kafka reader state as `ok`, `disabled`, or `unavailable` depending on broker configuration and reader initialization. `roomauthorityservice` readiness checks Redis, NATS, room RPC, timeline RPC, and internal RPC availability.

Start the server:

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
- `roomauthorityservice` runs by default and owns authority decisions through `authority.Engine`; `roomserver` remains the Android-facing HTTP/WebSocket edge and keeps local WebSocket connection tables.
- Authority rollback is explicit: set `AUTHORITY_SERVICE_MODE=local`, `ROOM_RUNTIME_MODE=local_process`, and `WS_CROSS_INSTANCE_BROADCAST_ENABLED=false`. No Android HTTP/WebSocket payload changes are involved.

See [server/deploy/README.md](../server/deploy/README.md) for deployment-specific commands and Nginx details.
