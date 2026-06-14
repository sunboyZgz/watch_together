# Database Ownership

Phase 7 started with one PostgreSQL database and logical ownership boundaries. Phase 8 made the boundary enforceable with the machine-readable registry at `server/internal/store/db_ownership.yaml` and architecture tests that scan store SQL and migrations.

Phases 9-22 moved media, timeline, progress, room, and identity into independent PostgreSQL database boundaries while adding their RPC services. The independent media database was the first concrete database split and remains the catalog owner boundary. Phase 23 added the full-RPC multi-database smoke gate, Phase 26 moved public REST to `cmd/apigateway`, and Phase 27 removes the main-database shadow-table path. In the current architecture, "split database" means a separate PostgreSQL database such as `anime_watch_identity_dev`, `anime_watch_media_dev`, `anime_watch_room_dev`, `anime_watch_progress_dev`, or `anime_watch_timeline_dev`, usually inside the same PostgreSQL server/container as the main database. It is not a requirement to deploy a second PostgreSQL software system.

Owner database URLs are required by owner services: `IDENTITY_DATABASE_URL`, `ROOM_DATABASE_URL`, `MEDIA_DATABASE_URL`, `PROGRESS_DATABASE_URL`, and `TIMELINE_DATABASE_URL`. If an owner URL is missing or unavailable, the owner service is unavailable; it must not silently fall back to `DATABASE_URL`. The main database no longer creates or owns `users`, `rooms`, `room_members`, media tables, `user_media_progress`, or `room_timeline_outbox`.

## Ownership Rules

- A context owner may write its own tables.
- Other contexts must not write owner tables directly.
- Cross-context reads must go through a port/interface, RPC adapter, or a documented read model.
- New tables must declare an owner before their first migration is merged.
- Cross-database foreign keys are not valid. Owner databases keep cross-context ids as columns and enforce existence through service calls, not SQL FKs.
- The registry, not this prose table, is the CI source of truth. Documentation should explain owner intent; tests enforce the registry.

## Table Owners

| Context owner | Tables | Notes |
| --- | --- | --- |
| `identity` | `users` | Owns account, password hash, profile fields, and token identity source data. |
| `media` | `media_tags`, `media_seasons`, `media_episodes`, `media_episode_variants`, `media_season_tags` | Owns catalog, playback lookup metadata, tag relationships, and storage-facing media metadata. |
| `room-session` | `rooms`, `room_members` | Owns durable room business records, membership, room status, grace period, and host/member roles. Runtime authority still lives in `room.Manager` plus Redis leases. |
| `progress` | `user_media_progress` | Owns low-frequency personal watching progress. It does not drive realtime room sync. |
| `timeline` | `room_timeline_outbox` | Owns reliable Kafka delivery work, timeline event ids, publish status, retry state, and unpublished recovery gap closure. In Phase 11, compose defaults route roomserver timeline access through `TimelineInternalService`. |
| `home-composition` | none | Composes identity, progress, and media through ports/RPC. It must not own writes or direct SQL reads to core tables. |

## Cross-Context Access Registry

The canonical registry lives in `server/internal/store/db_ownership.yaml`. Current cross-context access is through service ports/RPC:

| Caller | Accessed owner | Current access | Current rule |
| --- | --- | --- | --- |
| `home-composition` | `identity`, `progress`, `media` | Home summary calls identity, progress, and media ports/RPC. | No direct SQL reads. Missing media summaries are skipped to keep the old inner-join behavior. |
| `room-session` | `media` | Room create/join/detail uses the media port for episode detail and playable metadata. | `PostgresRoomStore` no longer reads media tables directly; local/RPC media adapters are the boundary. |
| `room-session` | `identity` | Room create/join/detail validates users and enriches member profiles through identity port/RPC. | No direct SQL reads of `users` from `PostgresRoomStore`. |
| `progress` | `media` | Progress writes validate playable episodes through the media port before upsert. | `PostgresProgressStore` no longer reads media tables directly; local/RPC media adapters are the boundary. |
| `progress` | `identity` | Progress writes validate user existence before upsert. | Uses identity port/RPC; no direct SQL reads of `users`. |
| `media` playback delivery | `media` | Playback lookup reads `media_episodes` and variants. | Owned access. |
| `recovery` | `room-session`, `timeline` | Loads room metadata and asks timeline for recovery-ready events. | Allowed through `RoomDetailStore` and timeline `RecoveryReader` ports. |
| `outboxworker` | `timeline` | Claims and updates `room_timeline_outbox` from the timeline database. | Timeline-owned worker access. |
| `derivedworker` | `timeline` | Consumes Kafka canonical topic and publishes derived topics. | Timeline-owned projection worker access. |

## Phase 27 Owner Database Boundary

Each owner table is created only by its owner migration directory:

- `server/identity_migrations` creates `users`.
- `server/room_migrations` creates `rooms` and `room_members`.
- `server/media_migrations` creates media catalog tables.
- `server/progress_migrations` creates `user_media_progress`.
- `server/timeline_migrations` creates `room_timeline_outbox`.
- `server/migrations` remains for main-database infrastructure bookkeeping and must not create or reference owner tables.

`cmd/identityservice`, `cmd/roomservice`, `cmd/mediaservice`, `cmd/progressservice`, `cmd/timelineservice`, and `cmd/outboxworker` require their owner database URL. `cmd/apigateway`, the default `roomserver` session gateway, and `cmd/roomauthorityservice` do not receive `DATABASE_URL` or direct owner database URLs in compose.

The legacy import tools `cmd/identitydbsync`, `cmd/roomdbsync`, `cmd/mediadbsync`, and `cmd/progressdbsync` are explicit import utilities. They require `--source-database-url` for the source and no longer default to reading from `DATABASE_URL`.

## Phase 10 Timeline Database Boundary

Phase 10 moved the timeline owner from a purely logical table boundary to a database boundary:

- Timeline-owned migrations live under `server/timeline_migrations`.
- `cmd/timelineservice` and `cmd/outboxworker` require `TIMELINE_DATABASE_URL`.
- `TIMELINE_SERVICE_MODE=rpc` keeps `roomserver` away from the timeline database; it calls `cmd/timelineservice` instead. Phase 11 makes this the compose default.
- `/readyz` reports the owner database as `timeline_postgres` in timeline-owned processes.

## Phase 11 Timeline Service-Family Boundary

Phase 11 keeps timeline as a service family rather than a single process:

- `cmd/timelineservice` is the internal RPC API for recording timeline results and reading recovery-ready room events.
- `cmd/outboxworker` remains a separate timeline-owned worker for Kafka publish retry and mark-published state.
- `cmd/derivedworker` remains a separate timeline-owned projection worker for derived topics.
- Compose app/prod paths default `roomserver` to `TIMELINE_SERVICE_MODE=rpc` and do not pass the timeline-owned database URL to roomserver.
- `TIMELINE_SERVICE_MODE=local` is still supported as an explicit compatibility path, using `ROOMSERVER_TIMELINE_DATABASE_URL` in compose when direct timeline DB access is required.

## Phase 12 Timeline Result Ownership

Phase 12 moves canonical result event semantics into the timeline owner:

- `roomserver` sends typed control and membership results; it no longer constructs complete `TimelineEvent` values on the default path.
- `cmd/timelineservice` generates event ids, event version, occurrence time, payload JSON, and outbox rows.
- Recovery uses one timeline-owned feed that merges Kafka canonical events with same-room unpublished outbox gaps.
- Kafka remains a result log, not a command ingress log, and `room authority` remains in `roomserver`.

## Phase 21 Room Service and Room Database Boundary

Phase 19 makes room metadata and membership a serviceized business slice. Phase 21 gives that slice its own database boundary and moves lifecycle/recovery metadata behind the same service:

- `cmd/roomservice` exposes `RoomInternalService` for create, join, leave, detail, active-member checks, runtime bootstrap, recoverable room listing, grace-period updates, activation, destroy, startup backfill, and expired-room cleanup.
- Local compose `app` and production compose default `roomserver` to `ROOM_SERVICE_MODE=rpc`.
- `roomserver` still owns WebSocket connections, local room runtime state, and client-visible envelopes.
- Room-owned migrations live under `server/room_migrations`.
- `cmd/roomservice` requires `ROOM_DATABASE_URL`.
- `ROOM_SERVICE_MODE=rpc` keeps `roomserver` away from the room database; compose app/prod pass `ROOM_DATABASE_URL` only to `cmd/roomservice`.
- `cmd/roomservice` calls identity RPC for user validation/profile enrichment and media RPC for episode details, so `PostgresRoomStore` only reads/writes `rooms` and `room_members`.
- `cmd/roomauthorityservice` reads room runtime bootstrap and recoverable-room metadata through room RPC instead of PostgreSQL.
- `cmd/roomdbsync` is a legacy explicit import tool and requires `--source-database-url`.

## Phase 22 Identity Database Boundary and Home Composition Cleanup

Phase 22 gives identity its own database boundary and removes the last home SQL read model:

- Identity-owned migrations live under `server/identity_migrations`.
- `cmd/identityservice` requires `IDENTITY_DATABASE_URL`.
- Local compose `app` and production compose pass `IDENTITY_DATABASE_URL` only to `cmd/identityservice`; `roomserver` stays on identity RPC and does not receive the identity database URL.
- `cmd/identitydbsync` is a legacy explicit import tool and requires `--source-database-url`.
- Service calls enforce user validity; owner database schemas do not use cross-database SQL FKs.
- `home-composition` now reads identity/profile, recent progress, and media summaries only through ports/RPC. The `PostgresHomeStore` read model is removed.

Operational rules:

- `pending` and `publishing` rows are part of the authority recovery gap and must not be deleted by cleanup tasks.
- Published row retention can be introduced later, but Phase 10 only documents that boundary and does not auto-delete history.
- Backing up the timeline database protects outbox retry state and recovery gap closure. Restoring it does not restore users, rooms, progress, or media metadata.

## Current Split Checklist

- `room-session -> media` direct SQL is closed in Phase 8. Room create/join/detail now use the media port, backed by local `PostgresMediaStore` or `MediaInternalService` RPC.
- `progress -> media` direct SQL is closed in Phase 8. Progress writes validate playable episodes through the media port before touching `user_media_progress`.
- `home-composition -> identity/progress/media` direct SQL is closed in Phase 22. Home summary now requests every dependency through service ports/RPC.
- Timeline owns its independent outbox schema and compose-default RPC path. Later phases can add richer timeline service APIs or projection management, but `roomserver` should not regain default direct timeline DB ownership.
- Identity, room, media, progress, and timeline own their independent schemas in compose app/prod.
- Phase 27 smoke verifies owner DB writes and verifies the main database owner tables do not exist.

Before adding another durable context:

1. Replace direct cross-context writes with service calls or events.
2. Replace cross-context reads with RPC queries, cached projections, or read models.
3. Move migrations, backups, readiness checks, and restore runbooks to the owning service.
4. Remove cross-database transaction assumptions.
5. Add a business smoke that proves the Android-facing path writes only to the owning database.
