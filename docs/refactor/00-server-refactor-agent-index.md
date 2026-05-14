# Server Refactor Agent Index

> Purpose: entry point for agents working on the server Gin, GORM, Redis, and synchronization model refactor.

This folder is the working guide for the `refactor/server-sync-model` branch.

The target is not to add new product features. The target is to make the current server easier to maintain, move HTTP and database infrastructure to Gin and GORM, introduce Redis for appropriate runtime and cache data, and replace the old playback state model with an authoritative server timeline model.

## Read Order

For any server refactor agent, read these files first:

```text
1. docs/refactor/00-server-refactor-agent-index.md
2. docs/refactor/01-server-gin-gorm-redis-sync-plan.md
3. docs/refactor/02-server-sync-model-agent-brief.md
4. docs/refactor/03-server-sync-model-phase4-closeout.md
5. docs/refactor/04-server-clock-sync-phase5.md
6. docs/sync/00-index.md
7. docs/sync/07-agent-brief-checklists-and-anti-patterns.md
```

When changing timeline structs, transitions, or WebSocket behavior, also read:

```text
docs/sync/02-timeline-state-and-query-model.md
docs/sync/03-control-events-and-state-transitions.md
docs/sync/04-authority-consistency-and-versioning.md
docs/sync/05-clock-sync-and-client-contract.md
```

When changing runtime boundaries, buffering, or future sync extensions, also read:

```text
docs/sync/06-runtime-boundaries-buffering-and-extensions.md
```

Only open the original W3C Timing Object report when the local `docs/sync` files do not answer the design question:

```text
https://www.w3.org/community/reports/webtiming/CG-FINAL-timingobject-20241203/
```

## Current Server Areas To Understand

Before making code changes, inspect these current modules:

```text
server/cmd/roomserver/main.go
server/internal/app/server.go
server/internal/transport
server/internal/room
server/internal/protocol
server/internal/store
server/internal/roomapi
server/internal/auth
server/internal/media
server/internal/home
server/internal/progress
server/migrations
```

The current server already has:

```text
HTTP APIs for auth, home, media, room, and progress
WebSocket room sync
in-memory room manager
PostgreSQL persistence through database/sql and pgx
room lifecycle cleanup
host-only playback controls
heartbeat
```

Do not remove these behaviors while refactoring infrastructure.

## Refactor Goals

The refactor has four main outcomes:

```text
Gin owns HTTP routing and request/response binding.
GORM owns PostgreSQL access through typed models and repositories.
Redis is available for cache, ephemeral runtime metadata, and future multi-instance coordination.
Room playback sync is modeled as an authoritative server timeline vector.
```

The timeline vector is:

```text
positionMs
velocity
serverTimeMs
seq
reason
```

Clients send control intent. The server creates the authoritative vector.

## Non-goals

Do not add unrelated business features during this refactor:

```text
no new media product features
no new room social features
no subtitle or danmaku scheduling system
no complex distributed lock architecture unless the phase explicitly asks for it
no continuous current-position broadcast loop
no client-player-as-authority behavior
```

## Branch And Commit Guidance

Recommended branch:

```text
refactor/server-sync-model
```

Keep changes staged by phase when possible:

```text
phase 1: dependencies and app wiring
phase 2: GORM models and stores
phase 3: Redis client and cache boundary
phase 4: timeline model and WebSocket protocol
phase 5: tests and compatibility cleanup
```

Each phase should keep the server buildable.
