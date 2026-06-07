# Overview

`watch_together` is a self-hosted synchronized HLS watching application. The implemented path is an Android client talking to a Go backend that manages login, media catalog data, room membership, playback sync, progress writes, and media playback entry points.

## Current User Flow

1. A user logs in or registers through the Android app.
2. The app loads the home summary and media catalog from the backend.
3. A host selects an episode and creates a room.
4. Other users join with the 6-character room code.
5. Clients open `/ws`, send `join_room`, receive `room_state`, and follow the host's playback timeline.
6. The host can broadcast `play`, `pause`, `seek`, `set_playback_rate`, and `ended`.
7. Android writes low-frequency viewing progress through HTTP; progress is separate from real-time sync.

## Top-Level Modules

```text
watch_together/
|-- android/   Android Kotlin app using Jetpack Compose, OkHttp, and Media3 ExoPlayer
|-- server/    Go backend, mediactl CLI, migrations, Docker Compose, and deployment files
|-- docs/      current durable project documentation
|-- media/     local media workspace for raw and generated assets
|-- shared/    reserved cross-client/shared protocol area
|-- scripts/   repository-level helper scripts
|-- windows/   reserved Windows client area
```

The Windows and shared directories exist, but the implemented product path is Android plus the Go server.

## Backend Architecture

The backend starts from `server/cmd/roomserver/main.go`, loads config with `server/internal/config`, and assembles the HTTP server in `server/internal/app/server.go`.

Main backend boundaries:

- `internal/app`: server assembly, route registration, database/Redis wiring, lifecycle cleanup loops.
- `internal/auth`: account registration, login, bcrypt password hashing, JWT token issue/verify.
- `internal/home`: home summary aggregation.
- `internal/media`: media tag/catalog/playback lookup logic.
- `internal/progress`: user viewing progress writes.
- `internal/roomapi`: DB-backed room creation, joining, detail lookup, and membership changes.
- `internal/room`: in-memory runtime room state, clients, host authority, lifecycle, reconnect, and device-switch logic.
- `internal/transport`: HTTP handlers, WebSocket handler, response envelopes, media playback delivery.
- `internal/store`: PostgreSQL persistence.
- `internal/cache`: Redis room snapshots, authority leases, active-device ownership, idempotency, presence, and distributed rate limiting.
- `internal/eventbus`: NATS Core broadcast and control request/reply.
- `internal/timeline`: Kafka timeline events, outbox dispatch, and derived-topic dispatch.
- `internal/recovery`: distributed authority takeover and Kafka replay recovery.
- `internal/observability`: readiness snapshots and Prometheus metrics.
- `internal/servicekit`: service identity, request metadata, deadline, and internal auth conventions.
- `internal/internalrpc`: ConnectRPC helper layer for optional internal service adapters.
- `internal/rpcgen/v1`: generated typed internal RPC contracts for media and timeline.
- `internal/telemetry`: OpenTelemetry tracing setup.
- `internal/mediactl`: media ingestion CLI implementation.

## Runtime State

PostgreSQL is the durable store for users, media metadata, rooms, room members, and user progress. The in-process room manager owns live WebSocket connection objects. Redis stores runtime coordination state when configured: latest snapshots, authority leases, active-device leases, request idempotency, presence, and distributed seek rate limiting. Kafka stores the durable room timeline result log, and NATS Core handles realtime cross-instance fan-out and control forwarding.

On startup, the server marks previously active persisted rooms as `grace_period`, starts an in-memory cleanup loop, and, when PostgreSQL is available, starts persistent room cleanup for expired rooms.

The default room runtime mode is still `local_process`. `distributed_authority` enables Redis authority leases, NATS forwarding, Kafka timeline logging, authority recovery, distributed seek rate limiting, and Prometheus/readiness observability. Phase 8 keeps one PostgreSQL database but adds generated typed internal RPC contracts, verifiable media/timeline RPC pilots, OpenTelemetry collector support, and CI-tested logical database ownership rules. See [Runtime Boundaries](./runtime-boundaries.md), [Distributed Architecture](./distributed-architecture.md), and [Database Ownership](./database-ownership.md) for the current architecture.

## Media Model

The current media model is episode-backed:

- `media_seasons` describes a show, season, collection, or title container.
- `media_episodes` describes playable units.
- `media_episode_variants` describes generated HLS renditions.
- `media_tags` and `media_season_tags` support catalog filtering.

Some API fields still use the name `mediaItemId` for compatibility. In the current implementation, `mediaItemId` means `media_episodes.id`.

## Implemented External Dependencies

- Go, Gin, GORM, PostgreSQL, Redis, `github.com/coder/websocket`.
- Android Kotlin, Jetpack Compose, OkHttp, AndroidX Media3 ExoPlayer.
- FFmpeg and FFprobe for HLS generation.
- Local filesystem, MinIO, or S3-compatible storage for generated media assets.
- Nginx for production media proxying with `auth_request`.
