# Data Model

The durable schema is SQL-first. After Phase 27, `server/migrations/` is intentionally a main-database migration set with no business owner tables. Identity database migrations live under `server/identity_migrations/`; media database migrations live under `server/media_migrations/`; timeline database migrations live under `server/timeline_migrations/`; progress database migrations live under `server/progress_migrations/`; room database migrations live under `server/room_migrations/`. GORM models in `server/internal/model/models.go` mirror the active owner tables. Table ownership is enforced by `server/internal/store/db_ownership.yaml` plus architecture tests; see [Database Ownership](./database-ownership.md) for the owner map and split checklist.

`IDENTITY_DATABASE_URL`, `ROOM_DATABASE_URL`, `MEDIA_DATABASE_URL`, `PROGRESS_DATABASE_URL`, and `TIMELINE_DATABASE_URL` are required by their owning services in the full-RPC local and production paths. They no longer fall back to `DATABASE_URL`. The main database does not create `users`, `rooms`, `room_members`, media tables, `user_media_progress`, or `room_timeline_outbox`; those tables exist only in their owner databases.

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

The room database keeps `media_episode_id` as an id column without a cross-database foreign key to media tables. Room create/join/detail do not query media tables from `PostgresRoomStore`; media detail is loaded through the media port or `MediaInternalService`.

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

The progress database keeps `media_episode_id` as an id column without a cross-database foreign key to media tables. Progress writes validate playable episodes through the media port before `PostgresProgressStore` writes this table.

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

Rows are written after accepted/rejected control decisions and membership events. In Phase 12 default paths, `roomserver` sends typed result fields and `cmd/timelineservice` generates the canonical event id, event version, server occurrence time, payload JSON, and outbox row. Explicit local rollback can still use the same timeline-owned builder in process. `cmd/outboxworker` claims pending rows with `FOR UPDATE SKIP LOCKED`, publishes to Kafka, then marks rows as `published` or schedules retry. During authority recovery, `cmd/timelineservice` exposes one recovery feed that merges Kafka canonical events with same-room `pending` and `publishing` rows so already-decided events are not lost while asynchronous publishing catches up.

This table lives in the timeline database selected by `TIMELINE_DATABASE_URL`. In Phase 12 and later, compose defaults route roomserver access through typed result RPCs on `cmd/timelineservice`, while `cmd/outboxworker` and `cmd/derivedworker` remain timeline-owned workers. Kafka remains a result log, not command ingress. If timeline RPC or explicit local timeline storage is unavailable, timeline writes fail closed and accepted distributed controls are not broadcast.

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

The cache is written after HTTP room runtime bootstrap, WebSocket state transitions, and completed authority recovery. It can serve quick read-only snapshots, but it is not the recovery source of truth; Phase 12 rebuilds writable authority from the timeline-owned recovery feed, which combines Kafka timeline events plus unpublished PostgreSQL outbox rows. It does not store WebSocket connection objects, send queues, heartbeat state, seek rate limits, online presence, or active-device ownership.

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

On startup in the full-RPC path:

- `cmd/identityservice`, `cmd/roomservice`, `cmd/mediaservice`, `cmd/progressservice`, `cmd/timelineservice`, and `cmd/outboxworker` open their owner database URL and fail closed if it is missing or unavailable.
- `cmd/apigateway` and the default `roomserver` session gateway do not open `DATABASE_URL` or any owner database URL.
- Redis is opened if `REDIS_ADDR` is configured.
- Room lifecycle startup work, grace-period transitions, and expired-room cleanup go through room RPC.
- Timeline recovery and outbox gap reads go through timeline RPC.

If an owner database is unavailable, the owning service is not ready and callers receive the existing service-unavailable envelope through RPC/HTTP boundaries.
