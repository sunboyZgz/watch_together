# Server

`server/` contains the Go backend, PostgreSQL migrations, Docker Compose files, deployment assets, and the `mediactl` CLI.

## Entry Points

- `cmd/roomserver/main.go`: loads runtime config and starts the HTTP/WebSocket server.
- `cmd/mediactl/main.go`: loads media tool config and runs media maintenance commands.
- `cmd/mediaservice/main.go`: optional internal media RPC service for the serviceization pilot.
- `cmd/timelineservice/main.go`: optional internal timeline RPC service for the serviceization pilot.

## Main Internal Packages

- `internal/app`: server assembly, route registration, DB/Redis wiring, cleanup loops.
- `internal/auth`: registration, login, password hashing, JWT tokens.
- `internal/home`: home summary data.
- `internal/media`: media catalog and playback lookup.
- `internal/progress`: user media progress writes.
- `internal/roomapi`: DB-backed room creation, joining, detail lookup, and leave persistence.
- `internal/room`: in-memory active room state and client lifecycle.
- `internal/transport`: HTTP handlers, WebSocket handler, API envelopes, media delivery.
- `internal/store`: PostgreSQL stores.
- `internal/cache`: Redis room-state cache.
- `internal/rpcgen/v1`: generated typed ConnectRPC contracts.
- `internal/mediactl`: HLS generation, storage upload, and metadata upsert logic.

## Local Commands

Start infrastructure:

```bash
docker compose up -d postgres redis minio minio-init
```

Run migrations:

```bash
make migration-up
```

Start the server:

```bash
APP_ENV=local go run ./cmd/roomserver
```

Optional Phase 1 runtime metadata:

```bash
SERVER_INSTANCE_ID=local-roomserver-1 ROOM_RUNTIME_MODE=local_process APP_ENV=local go run ./cmd/roomserver
```

`ROOM_RUNTIME_MODE=local_process` is the only implemented room runtime mode. `/healthz` exposes it through `X-Watch-Together-Room-Runtime` and exposes `SERVER_INSTANCE_ID` when configured.

Run tests:

```bash
go test ./...
```

Regenerate internal RPC contracts:

```bash
make proto-lint
make proto-generate
```

Run mediactl:

```bash
go run ./cmd/mediactl plan --library-root ../media/raw --input ../media/raw/sample-show/season-01/episode-01.mp4 --title "Sample Episode"
```

## Detailed Docs

- [Setup and configuration](../docs/setup-and-configuration.md)
- [Backend API contract](../docs/backend-api-contract.md)
- [WebSocket protocol](../docs/websocket-protocol.md)
- [Runtime boundaries](../docs/runtime-boundaries.md)
- [Database ownership](../docs/database-ownership.md)
- [Media operations](../docs/media-operations.md)
- [Data model](../docs/data-model.md)
- [Deployment notes](./deploy/README.md)
- [Migration notes](./migrations/README.md)
