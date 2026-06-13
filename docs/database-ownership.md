# Database Ownership

Phase 7 keeps one PostgreSQL database and adds logical ownership boundaries. Phase 8 makes the boundary enforceable with the machine-readable registry at `server/internal/store/db_ownership.yaml` and architecture tests that scan store SQL and migrations.

Phase 9 adds the first independent media database boundary. Phase 10 adds the timeline database boundary for `room_timeline_outbox`. Phase 11 makes timeline a default service-family boundary in compose: `roomserver` calls `cmd/timelineservice` over RPC, while `cmd/outboxworker` and `cmd/derivedworker` remain separate timeline-owned workers. Phase 16 makes local compose `app` use media, timeline, and authority RPC by default; Phase 17/18 adds identity RPC; Phase 19 adds room metadata and membership RPC through `cmd/roomservice`; Phase 20 adds progress/home composition RPC and `PROGRESS_DATABASE_URL`; Phase 21 adds the room-session database boundary through `ROOM_DATABASE_URL` and moves lifecycle/recovery metadata access behind `cmd/roomservice`. In these database phases, "split database" means a separate PostgreSQL database such as `anime_watch_media_dev`, `anime_watch_room_dev`, or `anime_watch_timeline_dev`, usually inside the same PostgreSQL server/container as the main database. It is not a requirement to deploy a second PostgreSQL software system.

When `MEDIA_DATABASE_URL` or `TIMELINE_DATABASE_URL` is empty, the matching owner keeps the previous single-database fallback. When either URL is configured, that owner uses the configured database. Timeline is fail-closed: if `TIMELINE_DATABASE_URL` is set but cannot be opened, timeline writes/readers must be unavailable rather than silently falling back to the main database.

## Ownership Rules

- A context owner may write its own tables.
- Other contexts must not write owner tables directly.
- Cross-context reads must go through a port/interface, RPC adapter, or a documented read model.
- New tables must declare an owner before their first migration is merged.
- Cross-database foreign keys are not valid. Phase 9 drops the main database FKs from `rooms.media_episode_id` and `user_media_progress.media_episode_id` to media tables while keeping the columns and indexes.
- The registry, not this prose table, is the CI source of truth. Documentation should explain owner intent; tests enforce the registry.

## Table Owners

| Context owner | Tables | Notes |
| --- | --- | --- |
| `identity` | `users` | Owns account, password hash, profile fields, and token identity source data. |
| `media` | `media_tags`, `media_seasons`, `media_episodes`, `media_episode_variants`, `media_season_tags` | Owns catalog, playback lookup metadata, tag relationships, and storage-facing media metadata. |
| `room-session` | `rooms`, `room_members` | Owns durable room business records, membership, room status, grace period, and host/member roles. Runtime authority still lives in `room.Manager` plus Redis leases. |
| `progress` | `user_media_progress` | Owns low-frequency personal watching progress. It does not drive realtime room sync. |
| `timeline` | `room_timeline_outbox` | Owns reliable Kafka delivery work, timeline event ids, publish status, retry state, and unpublished recovery gap closure. In Phase 11, compose defaults route roomserver timeline access through `TimelineInternalService`. |
| `home-composition` | none | Reads identity/progress from the main database and fills media labels through the media port or RPC. It must not own writes to core tables. |

## Cross-Context Access Registry

The canonical registry lives in `server/internal/store/db_ownership.yaml`. Current registered reads are:

| Caller | Accessed owner | Current access | Current rule |
| --- | --- | --- | --- |
| `home-composition` | `identity`, `progress` | `PostgresHomeStore` reads user profile and progress episode ids from the main database. | Allowed as a main-database read model; no writes. |
| `home-composition` | `media` | Home summary calls the media port/RPC `BatchGetEpisodeSummaries`. | No direct SQL reads of media tables. Missing summaries are skipped to keep the old inner-join behavior. |
| `room-session` | `media` | Room create/join/detail uses the media port for episode detail and playable metadata. | `PostgresRoomStore` no longer reads media tables directly; local/RPC media adapters are the boundary. |
| `room-session` | `identity` | Room create/join/detail validates users and enriches member profiles through identity port/RPC. | No direct SQL reads of `users` from `PostgresRoomStore`. |
| `progress` | `media` | Progress writes validate playable episodes through the media port before upsert. | `PostgresProgressStore` no longer reads media tables directly; local/RPC media adapters are the boundary. |
| `progress` | `identity` | Progress writes validate user existence before upsert. | Allowed while single PostgreSQL is retained; no direct writes to `users`. |
| `media` playback delivery | `media` | Playback lookup reads `media_episodes` and variants. | Owned access. |
| `recovery` | `room-session`, `timeline` | Loads room metadata and asks timeline for recovery-ready events. | Allowed through `RoomDetailStore` and timeline `RecoveryReader` ports. |
| `outboxworker` | `timeline` | Claims and updates `room_timeline_outbox` from the timeline database. | Timeline-owned worker access. |
| `derivedworker` | `timeline` | Consumes Kafka canonical topic and publishes derived topics. | Timeline-owned projection worker access. |

## Phase 9 Media Database Boundary

Phase 9 moves the media owner from a purely logical table boundary to a database boundary:

- Media-owned migrations live under `server/media_migrations`.
- Main application migrations still live under `server/migrations`.
- `cmd/mediaservice` prefers `MEDIA_DATABASE_URL` and falls back to `DATABASE_URL`.
- `roomserver` local media mode prefers `MEDIA_DATABASE_URL` and falls back to `DATABASE_URL`.
- `MEDIA_SERVICE_MODE=rpc` keeps `roomserver` away from the media database; it calls `cmd/mediaservice` instead.
- `/readyz` reports the main database as `postgres` and the optional media database as `media_postgres`.
- `cmd/mediadbsync` copies media-owned rows from the old main database tables into the media database and can run `--verify-only` content checks.

The old main-database media tables are intentionally kept as shadow/rollback data for this phase. They should not receive new non-media-owner access.

## Phase 10 Timeline Database Boundary

Phase 10 moves the timeline owner from a purely logical table boundary to an optional database boundary:

- Timeline-owned migrations live under `server/timeline_migrations`.
- Main application migrations still live under `server/migrations`.
- `cmd/timelineservice` prefers `TIMELINE_DATABASE_URL` and falls back to `DATABASE_URL` only when the timeline URL is empty.
- `cmd/outboxworker` prefers `TIMELINE_DATABASE_URL` and falls back to `DATABASE_URL` only when the timeline URL is empty.
- `roomserver` local timeline mode opens `TIMELINE_DATABASE_URL` when configured; if that connection fails, the timeline recorder is unavailable and accepted distributed controls fail closed.
- `TIMELINE_SERVICE_MODE=rpc` keeps `roomserver` away from the timeline database; it calls `cmd/timelineservice` instead. Phase 11 makes this the compose default.
- `/readyz` reports the main database as `postgres` and the optional timeline database as `timeline_postgres`.

The old main-database `room_timeline_outbox` table is intentionally kept as fallback/shadow data. Development-stage Phase 10 does not migrate old outbox history into the timeline database; new timeline databases start empty after running `server/timeline_migrations`.

## Phase 11 Timeline Service-Family Boundary

Phase 11 keeps timeline as a service family rather than a single process:

- `cmd/timelineservice` is the internal RPC API for recording timeline results and reading recovery-ready room events.
- `cmd/outboxworker` remains a separate timeline-owned worker for Kafka publish retry and mark-published state.
- `cmd/derivedworker` remains a separate timeline-owned projection worker for derived topics.
- Compose app/prod paths default `roomserver` to `TIMELINE_SERVICE_MODE=rpc` and do not pass the timeline-owned database URL to roomserver.
- `TIMELINE_SERVICE_MODE=local` is still supported as an explicit rollback path, using `ROOMSERVER_TIMELINE_DATABASE_URL` in compose when direct timeline DB access is required.

## Phase 12 Timeline Result Ownership

Phase 12 moves canonical result event semantics into the timeline owner:

- `roomserver` sends typed control and membership results; it no longer constructs complete `TimelineEvent` values on the default path.
- `cmd/timelineservice` generates event ids, event version, occurrence time, payload JSON, and outbox rows.
- Recovery uses one timeline-owned feed that merges Kafka canonical events with same-room unpublished outbox gaps.
- Kafka remains a result log, not a command ingress log, and `room authority` remains in `roomserver`.

## Phase 21 Room Service and Room Database Boundary

Phase 19 makes room metadata and membership a serviceized business slice. Phase 21 gives that slice its own optional database boundary and moves lifecycle/recovery metadata behind the same service:

- `cmd/roomservice` exposes `RoomInternalService` for create, join, leave, detail, active-member checks, runtime bootstrap, recoverable room listing, grace-period updates, activation, destroy, startup backfill, and expired-room cleanup.
- Local compose `app` and production compose default `roomserver` to `ROOM_SERVICE_MODE=rpc`.
- `roomserver` still owns WebSocket connections, local room runtime state, and client-visible envelopes.
- Room-owned migrations live under `server/room_migrations`.
- `cmd/roomservice` prefers `ROOM_DATABASE_URL` and falls back to `DATABASE_URL` only when the room URL is empty.
- `ROOM_SERVICE_MODE=rpc` keeps `roomserver` away from the room database; compose app/prod pass `ROOM_DATABASE_URL` only to `cmd/roomservice`.
- `cmd/roomservice` calls identity RPC for user validation/profile enrichment and media RPC for episode details, so `PostgresRoomStore` only reads/writes `rooms` and `room_members`.
- `cmd/roomauthorityservice` reads room runtime bootstrap and recoverable-room metadata through room RPC instead of PostgreSQL.
- `cmd/roomdbsync` copies `rooms` and `room_members` from the main database shadow tables to the room database and supports `--verify-only`.

Operational rules:

- `pending` and `publishing` rows are part of the authority recovery gap and must not be deleted by cleanup tasks.
- Published row retention can be introduced later, but Phase 10 only documents that boundary and does not auto-delete history.
- Backing up the timeline database protects outbox retry state and recovery gap closure. Restoring it does not restore users, rooms, progress, or media metadata.

## Future Split Checklist

Remaining blockers after the Phase 21 service and database pilots:

- `room-session -> media` direct SQL is closed in Phase 8. Room create/join/detail now use the media port, backed by local `PostgresMediaStore` or `MediaInternalService` RPC.
- `progress -> media` direct SQL is closed in Phase 8. Progress writes validate playable episodes through the media port before touching `user_media_progress`.
- `home-composition -> media` direct SQL is closed in Phase 9. Home summary now reads progress ids from the main database and requests media summaries through the media port.
- The main database still retains old media tables for rollback/shadow validation. A later cleanup can remove them after sync confidence is high.
- Timeline owns its independent outbox schema and compose-default RPC path. Later phases can add richer timeline service APIs or projection management, but `roomserver` should not regain default direct timeline DB ownership.
- Identity still uses the main database by default. Splitting it still needs token/profile operational planning.
- Main-database shadow tables remain for media, progress, room, and timeline fallback. Later cleanup can remove them after sync confidence is high.

Before moving a context to its own database:

1. Replace direct cross-context writes with service calls or events.
2. Replace cross-context reads with RPC queries, cached projections, or read models.
3. Backfill the target database and verify row counts.
4. Run dual-read or shadow-read checks before switching production traffic.
5. Move migrations, backups, readiness checks, and restore runbooks to the owning service.
6. Remove cross-database transaction assumptions.

Suggested order:

1. Split `media` first because it is catalog/storage oriented and mostly independent from room authority.
2. Keep hardening `timeline` as a service family before extracting more contexts. Phase 11 establishes service-owned RPC defaults and worker responsibility boundaries.
3. Split `identity` after room/progress/home have proven their RPC dependency behavior.
4. Consider an independent API gateway only after service boundaries are stable; `roomserver` can continue as the Android-facing BFF/session gateway meanwhile.
