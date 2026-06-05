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

If `DATABASE_URL` is empty or PostgreSQL cannot be opened, database-backed HTTP endpoints return `503`. If `REDIS_ADDR` is empty, the Redis-backed room state cache is disabled.

`WS_CROSS_INSTANCE_BROADCAST_ENABLED` controls Phase 3 cross-instance WebSocket fan-out. It defaults to `false`. When set to `true`, `WS_EVENT_BUS` currently accepts only `nats_core`, and `roomserver` publishes local WebSocket broadcast envelopes to `NATS_SUBJECT_ROOM_BROADCAST`.

Each `roomserver` instance subscribes directly to the same NATS Core subject. Do not configure a NATS queue group for room broadcasts, because every instance needs the same broadcast message. JetStream is not enabled in this phase, so NATS is not a durable event log or playback authority. `ROOM_RUNTIME_MODE=local_process` and Redis `room_state` snapshot behavior remain unchanged.

If cross-instance broadcast is enabled but NATS cannot be opened, the server logs the connection failure and continues with local-process WebSocket behavior.

In `distributed_authority`, NATS is also used for internal control request/reply through `NATS_SUBJECT_ROOM_CONTROL`. Kafka is used only for durable room timeline result logging through PostgreSQL outbox workers. Online WebSocket fan-out still uses NATS Core and local connection tables.

See [distributed-architecture.md](./distributed-architecture.md) for the Phase 4 module map and data flows.

Worker examples:

```bash
cd server
APP_ENV=local go run ./cmd/outboxworker
APP_ENV=local go run ./cmd/derivedworker
```

Start the server:

```bash
cd server
APP_ENV=local go run ./cmd/roomserver
```

Health check:

```text
GET http://127.0.0.1:8080/healthz
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

## Production Compose

Production deployment uses `server/compose.prod.yaml`. Its intended boundary is:

- Only Nginx exposes HTTP to the public network.
- Go `roomserver`, workers, PostgreSQL, Redis, Kafka, NATS, and MinIO stay on the Docker network.
- NATS stays on the Docker network and is used for WebSocket fan-out and internal control forwarding.
- Kafka stays on the Docker network and stores durable timeline result events.
- Media delivery defaults to `MEDIA_DELIVERY_MODE=nginx_auth_request`.

See [server/deploy/README.md](../server/deploy/README.md) for deployment-specific commands and Nginx details.
