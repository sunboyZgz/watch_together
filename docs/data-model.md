# Data Model

The durable schema is SQL-first. Main database migrations live under `server/migrations/`; Phase 9 media database migrations live under `server/media_migrations/`; Phase 10 timeline database migrations live under `server/timeline_migrations/`. GORM models in `server/internal/model/models.go` mirror the active tables. Table ownership is enforced by `server/internal/store/db_ownership.yaml` plus architecture tests; see [Database Ownership](./database-ownership.md) for the owner map and split checklist.

When `MEDIA_DATABASE_URL` is empty, media tables continue to live in the main database for local fallback. When `MEDIA_DATABASE_URL` is set, media-owned tables are migrated and read from the independent media database. The old main-database media tables are kept as shadow/rollback data during the Phase 9 pilot.

When `TIMELINE_DATABASE_URL` is empty, `room_timeline_outbox` continues to live in the main database for local fallback. When `TIMELINE_DATABASE_URL` is set, timeline-owned outbox rows are migrated and written in the independent timeline database. The old main-database outbox table is kept as fallback/shadow data during the Phase 10 pilot; old history is not migrated.

## Primary Tables

### `users`

Stores account and profile data:

- `id`
- `account`
- `password_hash`
- `nickname`
- `avatar_seed`
- `avatar_url`
- `bio`
- timestamps

### `media_tags`

Stores searchable/filterable tag catalog entries:

- `slug`
- `name`
- `is_featured`
- `is_active`
- `sort_order`

### `media_seasons`

Stores show, season, collection, or title-level metadata:

- `slug`
- `title`
- `original_title`
- `description`
- `cover_url`
- `category`
- `production_team`
- `search_aliases`
- `season_number`
- `season_label`
- `sort_order`
- `status`

### `media_episodes`

Stores playable units:

- `season_id`
- `title`
- `subtitle`
- `description`
- `cover_url`
- `media_url`
- `duration_ms`
- `episode_number`
- `episode_label`
- `source_key`
- `source_hash`
- `sort_order`
- `status`

Current HTTP fields named `mediaItemId` point to `media_episodes.id`.

### `media_episode_variants`

Stores HLS rendition metadata:

- `media_episode_id`
- `variant_key`
- `label`
- `playlist_url`
- `width`
- `height`
- `bandwidth_bps`
- `codecs`
- `is_default`
- `sort_order`
- `segment_count`
- `average_segment_ms`

### `media_season_tags`

Join table from seasons to tags:

- `season_id`
- `media_tag_id`

### `rooms`

Stores room business records:

- `id`
- `room_code`
- `host_user_id`
- `media_episode_id`
- `status`
- `last_empty_at`
- `destroy_after`

Current room statuses are handled as `active`, `grace_period`, and `destroyed`.

Phase 9 removes the cross-database foreign key from `rooms.media_episode_id` to `media_episodes.id` in the main database. The column, indexes, and room behavior remain. Room create/join/detail do not query media tables from `PostgresRoomStore`; media detail is loaded through the media port or `MediaInternalService`.

### `room_members`

Stores room membership:

- `room_id`
- `user_id`
- `role`
- `joined_at`
- `left_at`
- `is_active`

Roles are currently `host` and `member`. A user can have at most one active membership row for the same room.

### `user_media_progress`

Stores low-frequency progress:

- `user_id`
- `media_episode_id`
- `last_position_seconds`
- `duration_seconds`
- `last_watched_at`
- `completed`
- `completion_source`

This table supports the Android home page's last-watched and continue-watching data. It does not drive real-time room sync.

Phase 9 removes the cross-database foreign key from `user_media_progress.media_episode_id` to `media_episodes.id` in the main database. Progress writes validate playable episodes through the media port before `PostgresProgressStore` writes this table.

### `room_timeline_outbox`

Stores reliable delivery work for Kafka room timeline result events:

- `topic`
- `event_id`
- `event_type`
- `room_id`
- `payload`
- `status`
- `attempts`
- `last_error`
- `next_attempt_at`
- `published_at`
- timestamps

Rows are written by the authority `roomserver` after accepted/rejected control decisions and membership events. `cmd/outboxworker` claims pending rows with `FOR UPDATE SKIP LOCKED`, publishes to Kafka, then marks rows as `published` or schedules retry. During Phase 5 authority recovery, same-room `pending` and `publishing` rows are merged after Kafka replay so already-decided events are not lost while asynchronous publishing catches up.

In Phase 10, this table can live in the independent timeline database selected by `TIMELINE_DATABASE_URL`. `cmd/timelineservice`, `cmd/outboxworker`, and `roomserver` local timeline mode prefer that database when configured. If `TIMELINE_DATABASE_URL` is set but unavailable, timeline writes fail closed and accepted distributed controls are not broadcast.

`pending` and `publishing` rows are part of the recovery gap and must not be deleted by cleanup jobs. Published row retention can be added later, but Phase 10 does not auto-delete outbox history.

## Runtime State

The in-process `room.Manager` owns active WebSocket room state:

- joined clients
- host authority
- active device per user
- pause/play/seek/rate/ended state
- `seq`
- grace-period cleanup

When configured, Redis stores latest room snapshots in a `room_state` cache. In `local_process`, Redis is not the source of truth for playback authority; the in-process room manager is.

The room snapshot cache uses keys shaped as:

```text
wt:room:state:{roomCode}:v1
```

The cached value is the WebSocket `room_state` payload: room code, media ID, optional media duration, host user ID, paused/ended flags, position, velocity, server time, reason, playback rate, and `seq`. The default TTL is currently 10 minutes.

The cache is written after HTTP room runtime bootstrap, WebSocket state transitions, and completed authority recovery. It can serve quick read-only snapshots, but it is not the recovery source of truth; Phase 5 rebuilds writable authority from Kafka timeline events plus unpublished PostgreSQL outbox rows. It does not store WebSocket connection objects, send queues, heartbeat state, seek rate limits, online presence, or active-device ownership.

In `distributed_authority`, Redis also stores:

```text
wt:room:authority:{roomId}:v1
wt:room:active_device:{roomId}:{userId}:v1
wt:room:control_request:{roomId}:{requestId}:v1
wt:room:control_rate:{roomId}:seek:v1
wt:room:presence:{roomId}:v1
```

The authority value contains `instanceId`, `epoch`, `status`, and `leaseUntilMs`. `status=active` means the instance can apply room controls for the current epoch. `status=recovering` fences a takeover attempt while one instance replays Kafka and restores local room state. Same-instance renewals keep the epoch; successful takeover after lease expiry increments it. The active-device value contains `deviceId`, `instanceId`, `connectionId`, and `leaseUntilMs`.

The control request value contains `roomId`, `requestId`, `status=pending|accepted|rejected`, `authorityEpoch`, `seq`, accepted WebSocket envelope, rejection error, and `leaseUntilMs`. It is the runtime idempotency layer for `distributed_authority`; recovered accepted events from Kafka/outbox backfill recent request records after authority takeover.

The control rate value is a short-lived seek reservation. It lets `distributed_authority` enforce `WS_SEEK_MIN_INTERVAL_MS` across forwarded controls and authority recovery. It is runtime state only and is not part of the durable playback timeline.

The presence value is a per-room user-level online registry. Internally it may keep `deviceId`, `connectionId`, `instanceId`, role, `lastSeenMs`, and `leaseUntilMs`, but client-facing `room_presence` snapshots expose only user-level fields. Presence is not PostgreSQL membership, not Kafka timeline data, and not a durable audit record.

Kafka stores JSON v1 room result events:

```text
wt.room.timeline.v1
wt.room.control_result.v1
wt.room.membership.v1
```

The canonical topic is the durable room timeline result log and Phase 5 authority recovery source. Recovery applies only `room.control.accepted` events to rebuild playback state. Rejected control events and membership events remain audit and retry context; they do not change recovered playback state. Derived topics are produced by `cmd/derivedworker`.

## Startup And Cleanup

On server startup:

- PostgreSQL is opened if `DATABASE_URL` is configured.
- The media database is opened when `MEDIA_DATABASE_URL` is configured and the process needs a local media store.
- The timeline database is opened when `TIMELINE_DATABASE_URL` is configured and the process needs a local timeline recorder or outbox worker.
- Redis is opened if `REDIS_ADDR` is configured.
- Existing active persistent rooms are marked `grace_period`.
- In-memory cleanup starts.
- Persistent cleanup destroys expired room records and deletes matching Redis `room_state`.

If PostgreSQL is unavailable, DB-backed HTTP endpoints return `503`, but the process can still start.
