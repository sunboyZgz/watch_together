# Database Ownership

Phase 7 keeps one PostgreSQL database and adds logical ownership boundaries. No physical database split happens in Phase 7.

## Ownership Rules

- A context owner may write its own tables.
- Other contexts must not write owner tables directly.
- Cross-context reads must go through a port/interface, RPC adapter, or a documented read model.
- New tables must declare an owner before their first migration is merged.
- Existing foreign keys remain in place until a later physical database split plan replaces them with service contracts or read models.

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

| Caller | Accessed owner | Current access | Phase 7 rule |
| --- | --- | --- | --- |
| `home-composition` | `identity`, `media`, `progress` | `PostgresHomeStore` composes home summary rows. | Allowed as a read model/composition query; no writes. |
| `room-session` | `media` | Room create/detail references `media_episodes.id`. | Allowed through room store while single PostgreSQL is retained; future split should call `MediaInternalService` or store a room media snapshot. |
| `media` playback delivery | `media` | Playback lookup reads `media_episodes` and variants. | Owned access. |
| `recovery` | `room-session`, `timeline` | Loads room metadata, Kafka events, and unpublished outbox rows. | Allowed through `RoomDetailStore`, `RoomEventReader`, and `PendingOutboxReader` ports. |
| `outboxworker` | `timeline` | Claims and updates `room_timeline_outbox`. | Owned access. |
| `derivedworker` | `timeline` | Consumes Kafka canonical topic and publishes derived topics. | Owned event-processing access. |

## Future Physical Split Checklist

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
