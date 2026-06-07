# Database Ownership

Phase 7 keeps one PostgreSQL database and adds logical ownership boundaries. No physical database split happens in Phase 7. Phase 8 makes the boundary enforceable with the machine-readable registry at `server/internal/store/db_ownership.yaml` and architecture tests that scan store SQL and migrations.

## Ownership Rules

- A context owner may write its own tables.
- Other contexts must not write owner tables directly.
- Cross-context reads must go through a port/interface, RPC adapter, or a documented read model.
- New tables must declare an owner before their first migration is merged.
- Existing foreign keys remain in place until a later physical database split plan replaces them with service contracts or read models.
- The registry, not this prose table, is the CI source of truth. Documentation should explain owner intent; tests enforce the registry.

## Table Owners

| Context owner | Tables | Notes |
| --- | --- | --- |
| `identity` | `users` | Owns account, password hash, profile fields, and token identity source data. |
| `media` | `media_tags`, `media_seasons`, `media_episodes`, `media_episode_variants`, `media_season_tags` | Owns catalog, playback lookup metadata, tag relationships, and storage-facing media metadata. |
| `room-session` | `rooms`, `room_members` | Owns durable room business records, membership, room status, grace period, and host/member roles. Runtime authority still lives in `room.Manager` plus Redis leases. |
| `progress` | `user_media_progress` | Owns low-frequency personal watching progress. It does not drive realtime room sync. |
| `timeline` | `room_timeline_outbox` | Owns reliable Kafka delivery work, timeline event ids, publish status, retry state, and unpublished recovery gap closure. |
| `home-composition` | none | Reads identity/media/progress data through documented composition paths. It must not own writes to core tables. |

## Cross-Context Access Registry

The canonical registry lives in `server/internal/store/db_ownership.yaml`. Current registered reads are:

| Caller | Accessed owner | Current access | Phase 8 rule |
| --- | --- | --- | --- |
| `home-composition` | `identity`, `media`, `progress` | `PostgresHomeStore` composes home summary rows. | Allowed as a read model/composition query; no writes. |
| `room-session` | `media` | Room create/join/detail uses the media port for episode detail and playable metadata. | `PostgresRoomStore` no longer reads media tables directly; local/RPC media adapters are the boundary. |
| `room-session` | `identity` | Room create/join/detail validates and displays users. | Allowed while single PostgreSQL is retained; no direct writes to `users`. |
| `progress` | `media` | Progress writes validate playable episodes through the media port before upsert. | `PostgresProgressStore` no longer reads media tables directly; local/RPC media adapters are the boundary. |
| `progress` | `identity` | Progress writes validate user existence before upsert. | Allowed while single PostgreSQL is retained; no direct writes to `users`. |
| `media` playback delivery | `media` | Playback lookup reads `media_episodes` and variants. | Owned access. |
| `recovery` | `room-session`, `timeline` | Loads room metadata, Kafka events, and unpublished outbox rows. | Allowed through `RoomDetailStore`, `RoomEventReader`, and `PendingOutboxReader` ports. |
| `outboxworker` | `timeline` | Claims and updates `room_timeline_outbox`. | Owned access. |
| `derivedworker` | `timeline` | Consumes Kafka canonical topic and publishes derived topics. | Owned event-processing access. |

## Future Physical Split Checklist

Phase 8 known split blockers:

- `room-session -> media` direct SQL is closed in Phase 8. Room create/join/detail now use the media port, backed by local `PostgresMediaStore` or `MediaInternalService` RPC.
- `progress -> media` direct SQL is closed in Phase 8. Progress writes validate playable episodes through the media port before touching `user_media_progress`.
- `home-composition -> identity/media/progress`: `PostgresHomeStore` intentionally remains a read model query while PostgreSQL is single-database. A physical split needs a cached home read model or service composition layer.
- Existing foreign keys from `rooms` and `user_media_progress` to media tables remain in PostgreSQL until a later split removes cross-database FK assumptions.

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
