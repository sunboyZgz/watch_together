# Data Model

The durable schema is SQL-first and lives under `server/migrations/`. GORM models in `server/internal/model/models.go` mirror the active tables.

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

Rows are written by the authority `roomserver` after accepted/rejected control decisions and membership events. `cmd/outboxworker` claims pending rows with `FOR UPDATE SKIP LOCKED`, publishes to Kafka, then marks rows as `published` or schedules retry.

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

The cache is written after HTTP room runtime bootstrap and WebSocket state transitions. It is readable through the cache layer for future recovery work, but Phase 2 still treats it as a latest-snapshot cache only. It does not store WebSocket connection objects, send queues, heartbeat state, control deduplication, seek rate limits, online presence, or active-device ownership.

In `distributed_authority`, Redis also stores:

```text
wt:room:authority:{roomId}:v1
wt:room:active_device:{roomId}:{userId}:v1
```

The authority value contains `instanceId` and `leaseUntilMs`. The active-device value contains `deviceId`, `instanceId`, `connectionId`, and `leaseUntilMs`.

Kafka stores JSON v1 room result events:

```text
wt.room.timeline.v1
wt.room.control_result.v1
wt.room.membership.v1
```

The canonical topic is the durable room timeline result log. Derived topics are produced by `cmd/derivedworker`.

## Startup And Cleanup

On server startup:

- PostgreSQL is opened if `DATABASE_URL` is configured.
- Redis is opened if `REDIS_ADDR` is configured.
- Existing active persistent rooms are marked `grace_period`.
- In-memory cleanup starts.
- Persistent cleanup destroys expired room records and deletes matching Redis `room_state`.

If PostgreSQL is unavailable, DB-backed HTTP endpoints return `503`, but the process can still start.
