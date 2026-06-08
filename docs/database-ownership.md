# Database Ownership

Phase 7 keeps one PostgreSQL database and adds logical ownership boundaries. Phase 8 makes the boundary enforceable with the machine-readable registry at `server/internal/store/db_ownership.yaml` and architecture tests that scan store SQL and migrations.

Phase 9 adds the first independent media database boundary. This means a separate PostgreSQL database such as `watch_together_media` or `anime_watch_media_dev`, selected by `MEDIA_DATABASE_URL`, usually inside the same PostgreSQL server/container as the main database. It is not a requirement to deploy a second PostgreSQL software system. When `MEDIA_DATABASE_URL` is empty, the server keeps the previous single-database fallback.

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
| `timeline` | `room_timeline_outbox` | Owns reliable Kafka delivery work, timeline event ids, publish status, retry state, and unpublished recovery gap closure. |
| `home-composition` | none | Reads identity/progress from the main database and fills media labels through the media port or RPC. It must not own writes to core tables. |

## Cross-Context Access Registry

The canonical registry lives in `server/internal/store/db_ownership.yaml`. Current registered reads are:

| Caller | Accessed owner | Current access | Phase 9 rule |
| --- | --- | --- | --- |
| `home-composition` | `identity`, `progress` | `PostgresHomeStore` reads user profile and progress episode ids from the main database. | Allowed as a main-database read model; no writes. |
| `home-composition` | `media` | Home summary calls the media port/RPC `BatchGetEpisodeSummaries`. | No direct SQL reads of media tables. Missing summaries are skipped to keep the old inner-join behavior. |
| `room-session` | `media` | Room create/join/detail uses the media port for episode detail and playable metadata. | `PostgresRoomStore` no longer reads media tables directly; local/RPC media adapters are the boundary. |
| `room-session` | `identity` | Room create/join/detail validates and displays users. | Allowed while single PostgreSQL is retained; no direct writes to `users`. |
| `progress` | `media` | Progress writes validate playable episodes through the media port before upsert. | `PostgresProgressStore` no longer reads media tables directly; local/RPC media adapters are the boundary. |
| `progress` | `identity` | Progress writes validate user existence before upsert. | Allowed while single PostgreSQL is retained; no direct writes to `users`. |
| `media` playback delivery | `media` | Playback lookup reads `media_episodes` and variants. | Owned access. |
| `recovery` | `room-session`, `timeline` | Loads room metadata, Kafka events, and unpublished outbox rows. | Allowed through `RoomDetailStore`, `RoomEventReader`, and `PendingOutboxReader` ports. |
| `outboxworker` | `timeline` | Claims and updates `room_timeline_outbox`. | Owned access. |
| `derivedworker` | `timeline` | Consumes Kafka canonical topic and publishes derived topics. | Owned event-processing access. |

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

## Future Split Checklist

Remaining blockers after the Phase 9 media database pilot:

- `room-session -> media` direct SQL is closed in Phase 8. Room create/join/detail now use the media port, backed by local `PostgresMediaStore` or `MediaInternalService` RPC.
- `progress -> media` direct SQL is closed in Phase 8. Progress writes validate playable episodes through the media port before touching `user_media_progress`.
- `home-composition -> media` direct SQL is closed in Phase 9. Home summary now reads progress ids from the main database and requests media summaries through the media port.
- The main database still retains old media tables for rollback/shadow validation. A later cleanup can remove them after sync confidence is high.
- Remaining non-media contexts still share the main database. Splitting identity, progress, room-session, or timeline still needs separate read-model and transaction-boundary work.

Before moving a context to its own database:

1. Replace direct cross-context writes with service calls or events.
2. Replace cross-context reads with RPC queries, cached projections, or read models.
3. Backfill the target database and verify row counts.
4. Run dual-read or shadow-read checks before switching production traffic.
5. Move migrations, backups, readiness checks, and restore runbooks to the owning service.
6. Remove cross-database transaction assumptions.

Suggested order:

1. Split `media` first because it is catalog/storage oriented and mostly independent from room authority.
2. Split `timeline` after outbox and recovery RPC paths are stable.
3. Split `identity` and `progress` after home composition has a read model or service composition path.
4. Split `room-session` last; it is closest to realtime authority, recovery, and membership correctness.
