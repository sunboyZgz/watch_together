# watch_together

`watch_together` is a self-hosted synchronized HLS watching project. The current implemented product path is an Android app connected to a Go backend for login, media catalog browsing, room creation/joining, WebSocket playback sync, progress writes, and signed media playback.

## Current Scope

- Android client: Kotlin, Jetpack Compose, OkHttp, AndroidX Media3 ExoPlayer.
- Backend: Go, Gin, GORM, PostgreSQL, optional Redis room-state cache, `github.com/coder/websocket`.
- Media tooling: `mediactl` uses FFmpeg/FFprobe to build HLS, upload/store assets, and write episode-backed metadata.
- Deployment: local Docker Compose for PostgreSQL, Redis, and MinIO; production Compose with Nginx media proxying.

## Repository Structure

```text
watch_together/
|-- android/   Android client
|-- server/    Go backend, mediactl, migrations, Docker and deployment files
|-- docs/      current durable project documentation
|-- media/     local media workspace
|-- shared/    reserved shared protocol/schema area
|-- scripts/   repository helper scripts
|-- windows/   reserved Windows client area
```

## Start Reading

- [docs/README.md](./docs/README.md)
- [docs/overview.md](./docs/overview.md)
- [docs/setup-and-configuration.md](./docs/setup-and-configuration.md)
- [docs/backend-api-contract.md](./docs/backend-api-contract.md)
- [docs/websocket-protocol.md](./docs/websocket-protocol.md)
- [docs/media-operations.md](./docs/media-operations.md)
- [docs/android-client.md](./docs/android-client.md)
- [docs/data-model.md](./docs/data-model.md)

## Local Development

Start local infrastructure:

```bash
cd server
docker compose up -d postgres redis minio minio-init
make migration-up
```

Start the Go server:

```bash
cd server
APP_ENV=local go run ./cmd/roomserver
```

Health check:

```text
GET http://127.0.0.1:8080/healthz
```

Build or run Android from Android Studio, or use:

```bash
cd android
./gradlew installLocalDebug
```

For full configuration, media ingest, API, and sync details, use the docs linked above.

## TODO
eliminate the code relevant with local development mode 