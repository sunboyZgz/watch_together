# Server Gin, GORM, Redis, And Sync Refactor Plan

> Purpose: phased implementation plan for refactoring the server without adding new business features.

## Design Direction

The server should be reorganized around clear infrastructure and runtime boundaries:

```text
Gin       -> HTTP routing, request binding, response writing, middleware
GORM      -> PostgreSQL models, repositories, transactions, migrations compatibility
Redis     -> cache, ephemeral runtime metadata, idempotency windows, future fan-out support
Realtime  -> authoritative room timeline and WebSocket runtime
Postgres  -> durable business data
```

The sync model should follow the local guide in `docs/sync`: synchronize the room timeline, not individual players.

## Data Responsibility Boundary

Use this boundary when deciding where state belongs.

### PostgreSQL

Durable business data:

```text
users
media_tags
media_seasons
media_episodes
media_episode_variants
rooms
room_members
user_media_progress
```

PostgreSQL remains the source of truth for account, catalog, room membership, media binding, and long-lived progress data.

### Redis

Ephemeral or cacheable data:

```text
media tag list cache
media search page cache, if query patterns justify it
home summary cache, with short TTL
active room presence summary
room latest timeline snapshot cache, optional
client control request id deduplication window
rate limit counters, optional
room event pub/sub or stream fan-out, future multi-instance phase
```

Redis must not silently become a second durable database. Any Redis key should have a clear TTL or a clear rebuild path.

### Go Process Memory

Process-local runtime objects:

```text
active websocket connections
connection write locks
current per-process room runtime
in-flight broadcast operations
heartbeat timers
```

Do not put WebSocket connection objects in Redis.

## Phase 0: Baseline And Safety Net

Goal: understand current behavior and preserve it during infrastructure changes.

Tasks:

```text
[ ] Run current server tests.
[ ] Note existing API paths and response shapes.
[ ] Note current WebSocket message types and payloads.
[ ] Confirm migrations represent the current PostgreSQL schema.
[ ] Avoid modifying .gitignore or unrelated client code.
```

Acceptance:

```text
Current tests pass before refactor work begins, or failures are documented.
No business feature behavior is intentionally changed.
```

## Phase 1: Introduce Gin Routing

Goal: replace `net/http` `ServeMux` routing with Gin while keeping current handlers behavior-compatible.

Suggested modules:

```text
server/internal/app/server.go
server/internal/transport/gin_response.go
server/internal/transport/*_gin_handler.go
```

Tasks:

```text
[ ] Add Gin dependency.
[ ] Build one `gin.Engine` in app assembly.
[ ] Keep current paths unchanged.
[ ] Preserve the existing API success/error envelope.
[ ] Keep `/healthz`.
[ ] Keep `/ws` upgrade route.
[ ] Add middleware only when needed: recovery, request id, logging, CORS if already required by clients.
```

Routes to preserve:

```text
POST /auth/login
POST /auth/register
GET  /home/summary
GET  /media/tags
GET  /media/items
POST /me/media-progress/
POST /rooms
POST /rooms/:roomCode/join
GET  /rooms/:roomCode
GET  /ws
GET  /healthz
```

Acceptance:

```text
All existing HTTP handler tests pass or are migrated to Gin test style.
Existing clients do not need path changes.
```

## Phase 2: Introduce GORM Models And Stores

Goal: replace handwritten PostgreSQL SQL repositories with GORM-backed repositories while matching current migrations.

Suggested modules:

```text
server/internal/db/postgres.go
server/internal/model/user.go
server/internal/model/media.go
server/internal/model/room.go
server/internal/model/progress.go
server/internal/store/*_gorm.go
```

Tasks:

```text
[ ] Add GORM PostgreSQL dependency.
[ ] Create one shared `*gorm.DB` during app assembly.
[ ] Define models that map to existing tables and column names.
[ ] Do not use AutoMigrate as the migration source of truth.
[ ] Keep SQL migrations in `server/migrations` as canonical schema changes.
[ ] Port auth store.
[ ] Port home store.
[ ] Port media store.
[ ] Port progress store.
[ ] Port room store and transactions.
```

Important model notes:

```text
Use explicit `TableName()` where names are not obvious.
Use UUID string fields carefully; match existing service DTOs.
Represent nullable DB columns with pointers or sql.Null* types at the model boundary.
Keep service-layer DTOs independent from GORM model structs.
```

Acceptance:

```text
All existing store and transport tests pass.
Create room, join room, room detail, media search, auth, home summary, and progress update keep their current API contracts.
```

## Phase 3: Add Redis Infrastructure

Goal: introduce Redis as an infrastructure capability with clear ownership, without making it required for every local development path unless configuration says so.

Suggested modules:

```text
server/internal/db/redis.go
server/internal/cache/cache.go
server/internal/cache/media_cache.go
server/internal/realtime/presence_store.go
server/internal/realtime/control_dedup_store.go
```

Configuration:

```text
REDIS_ADDR
REDIS_USERNAME
REDIS_PASSWORD
REDIS_DB
REDIS_TLS_ENABLED
REDIS_REQUIRED
```

Tasks:

```text
[ ] Add go-redis dependency.
[ ] Load Redis config from environment.
[ ] Create a Redis client during app assembly when configured.
[ ] Health-check Redis separately from PostgreSQL.
[ ] Make Redis optional unless `REDIS_REQUIRED=true`.
[ ] Add small cache wrapper interfaces instead of passing Redis everywhere.
[ ] Add TTL conventions.
[ ] Add tests for key construction and fallback behavior.
```

Initial Redis use cases:

```text
media tags cache with short TTL
optional home summary cache with short TTL
control request id deduplication window, if client request ids are added
presence summary cache, if a view needs fast online counts
latest timeline vector cache, optional debug/reconnect optimization
```

Redis key suggestions:

```text
wt:cache:media:tags:v1
wt:cache:home:summary:{userId}:v1
wt:room:{roomCode}:presence
wt:room:{roomCode}:timeline:latest
wt:room:{roomCode}:control_req:{requestId}
```

Acceptance:

```text
Server starts without Redis when Redis is optional.
Server fails fast when Redis is required but unavailable.
Redis keys have TTLs or rebuild paths.
No durable business data exists only in Redis.
```

## Phase 4: Build The Authoritative Timeline Model

Goal: replace the old sync state semantics with the server timeline vector model.

Suggested modules:

```text
server/internal/realtime/timeline_state.go
server/internal/realtime/timeline_transition.go
server/internal/realtime/clock.go
server/internal/realtime/room_runtime.go
server/internal/realtime/room_hub.go
server/internal/realtime/sync_protocol.go
```

Core state:

```text
roomCode
mediaId
positionMs
velocity
serverTimeMs
seq
reason
mediaDurationMs optional
hostUserId
ended derived or explicit view field
```

Transition rules:

```text
play: derive current position, set velocity > 0, increment seq
pause: derive current position, set velocity = 0, increment seq
seek: validate target, preserve velocity unless request explicitly says otherwise, increment seq
rate_change: derive current position, validate rate, set velocity, increment seq
media_change: reset position to 0, velocity = 0, increment seq
ended: clamp to duration or reported end policy, velocity = 0, increment seq
```

Compatibility rule:

```text
The server may still expose `paused` and `playbackRate` while Android is migrating.
They must be derived from the timeline vector:
paused = velocity == 0
playbackRate = velocity
```

Acceptance:

```text
Server authority is the only source of timeline vectors.
Client position is treated as hint or diagnostics, not truth.
Late join receives latest vector.
Reconnect receives latest vector.
Every accepted transition increments seq.
Every accepted transition broadcasts to all connected clients, including requester.
```

## Phase 5: Add Clock Sync Message Support

Goal: allow clients to estimate server time and correctly apply `serverTimeMs`.

WebSocket messages:

```text
clock_sync.ping
clock_sync.pong
```

Ping payload:

```json
{
  "clientSendMonoMs": 100000
}
```

Pong payload:

```json
{
  "serverTimeMs": 1710000000000,
  "clientSendMonoMs": 100000
}
```

Tasks:

```text
[ ] Add protocol constants and payload DTOs.
[ ] Reply quickly without DB work.
[ ] Use centralized server clock.
[ ] Keep heartbeat separate from clock sync.
```

Acceptance:

```text
Clients can request server time over WebSocket.
Every room_state includes serverTimeMs.
Control transitions use server-generated timestamps.
```

## Phase 6: Tests And Cleanup

Goal: prove the refactor preserved business behavior and improved sync semantics.

Required tests:

```text
[ ] timeline current-position derivation
[ ] play transition
[ ] pause transition after elapsed playback
[ ] seek transition preserving velocity
[ ] rate-change transition
[ ] stale or unauthorized control rejection
[ ] late join receives latest vector
[ ] clock sync pong shape
[ ] Redis optional startup
[ ] Gin HTTP route compatibility
[ ] GORM repository behavior
```

Cleanup tasks:

```text
[ ] Remove old independent `paused` truth where possible.
[ ] Keep derived compatibility fields only at transport boundary.
[ ] Remove unused database/sql store code after GORM replacement is complete.
[ ] Update docs/backend-api-contract.md only if external API contract changes.
[ ] Update docs/websocket-event-protocol.md when protocol migration is finalized.
```

## Phase Dependencies

Recommended order:

```text
Phase 0 -> Phase 1 -> Phase 2 -> Phase 3 -> Phase 4 -> Phase 5 -> Phase 6
```

Allowed parallel work:

```text
GORM model mapping can be explored while Gin route migration is happening.
Redis infrastructure can be introduced before the first Redis-backed feature.
Timeline model tests can be written before WebSocket handlers are migrated.
```

Avoid parallel edits to the same files:

```text
server/internal/app/server.go
server/internal/transport/websocket_handler.go
server/internal/room/room.go
server/internal/room/manager.go
```

