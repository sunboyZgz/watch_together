# Server

`server/` 用于放置房间服务与后端相关代码。

当前目录已经完成 `INT-14 bootstrap websocket room server` 的第一阶段骨架，主要包含：

- 可启动的 Go HTTP / WebSocket 服务入口
- `POST /auth/register` 的最小账号注册能力
- `POST /auth/login` 的最小账号密码登录能力
- `GET /home/summary` 的首页用户与观看进度聚合能力
- `GET /media/tags` 的媒体标签列表能力
- `GET /media/items` 的媒体搜索与标签筛选能力
- `POST /rooms` 的 DB-backed create room 能力
- `POST /rooms/{roomCode}/join` 的按房间码加入能力
- `GET /rooms/{roomCode}` 的放映室首屏业务数据能力
- `PUT /me/media-progress/{mediaItemId}` 的用户观看进度写入能力
- `/ws` WebSocket 接入路由与正式 join room 语义
- `play / pause / seek` 的最小控制事件处理与广播
- `set_playback_rate` 的最小控制事件处理与广播
- `ended` completed state 的最小权威语义与广播
- 应用层 heartbeat 与静默连接超时清理
- host disconnect 后的 immediate host transfer
- same-user repeated join 的单有效连接收敛
- 房间最后一个成员离开后的 grace-period lifecycle 规则
- 最小 `Room / ClientConnection / RoomManager` 内存结构
- 基于 `INT-19` 协议草案的最小消息解析层
- `join_room` 的已存在房间接入流程
- 基础断连清理逻辑

这里会成为全系统的房间协调与同步中心。

## Current Foundation

围绕 `INT-14 bootstrap websocket room server`，当前服务端基础选型已明确为：

- Language: Go
- HTTP server: Go standard library `net/http`
- WebSocket library: `github.com/coder/websocket`
- Message encoding: Go standard library `encoding/json`
- Room state storage: in-memory only

当前阶段先追求“最小可运行房间服务”，暂不引入 Gin / Echo / Fiber、Redis、数据库 ORM 或复杂配置中心。

## HTTP API Contract

当前后端业务接口进入定义阶段，统一契约记录在：

- [docs/backend-api-contract.md](../docs/backend-api-contract.md)

当前约定：

- HTTP API 负责用户、首页、媒体库、房间主数据和观看进度。
- WebSocket 继续负责播放同步、authority state、heartbeat 和实时房间状态。
- HTTP 成功响应统一使用 `data + meta` envelope。
- HTTP 错误响应统一使用 `error + meta` envelope。
- API 字段统一使用 `camelCase`。
- Android 与 Server 联调时优先以该文档对齐 DTO、错误码和调用顺序。

当前已落地的业务 HTTP API：

- `POST /auth/register`
- `POST /auth/login`
- `GET /home/summary`
- `GET /media/tags`
- `GET /media/items`
- `POST /rooms`
- `POST /rooms/{roomCode}/join`
- `GET /rooms/{roomCode}`
- `PUT /me/media-progress/{mediaItemId}`

注意：auth / home / media / room / progress endpoints 依赖 `DATABASE_URL` 和已执行的 PostgreSQL migration；如果未配置数据库，接口会返回 `503`。

## Media Storage Config

媒体资源存储与分发的详细约定记录在：

- [docs/media-storage-and-delivery.md](../docs/media-storage-and-delivery.md)

当前阶段的默认策略：

- 开发阶段默认使用 `local` storage driver。
- HLS 和封面文件输出到 `MEDIA_LOCAL_ROOT`。
- PostgreSQL 只保存 `media_url / cover_url / metadata`。
- Android 只消费 API 返回的 `mediaUrl / coverUrl`，不直接感知存储供应商。
- MinIO / S3-compatible 配置只作为后续 uploader 抽象预留。

当前 `roomserver` 和 `mediactl` 已统一接入配置加载层：

- 启动时会自动尝试读取 `server/.env`
- 然后按 `APP_ENV` 读取对应环境文件，例如 `APP_ENV=prod` 时读取 `server/.env.prod`
- 如果存在 `server/.env.local`，会覆盖 `.env`
- 如果存在 `server/.env.<APP_ENV>.local`，会覆盖对应环境文件
- 当前 shell 的环境变量仍然优先于本地文件
- `.env.example` 继续只作为模板，不直接参与运行时加载

关键环境变量：

```text
MEDIA_STORAGE_DRIVER=local
MEDIA_LOCAL_ROOT=../media/tmp
MEDIA_PUBLIC_BASE_URL=http://127.0.0.1:9000/media/tmp
MEDIA_OBJECT_KEY_PREFIX=media
FFMPEG_BIN=ffmpeg
FFPROBE_BIN=ffprobe
```

推荐本地 object key 形态：

```text
media/{sourceKeyWithoutExt}/hls/master.m3u8
media/{sourceKeyWithoutExt}/cover/cover.jpg
```

## Local Config Files

当前推荐的本地配置方式：

1. 复制 `server/.env.example`
2. 生成你自己的 `server/.env.local`
3. 在 `server/` 目录运行 `mediactl` 或 `roomserver`

示例：

```bash
cd server
cp .env.example .env.local
```

然后按你当前机器环境修改：

- `DATABASE_URL`
- `MEDIA_STORAGE_DRIVER`
- `MEDIA_STORAGE_ENDPOINT`
- `MEDIA_STORAGE_BUCKET`
- `FFMPEG_BIN`
- `FFPROBE_BIN`

如果临时想覆盖某个值，直接在当前 shell `export` 即可，环境变量优先级高于 `.env.local`。

## Production Config Files

当前推荐的生产环境配置方式：

1. 参考 `server/.env.prod.example`
2. 在目标机器上创建 `server/.env.prod` 或 `server/.env.prod.local`
3. 启动前显式设置 `APP_ENV=prod`

示例：

```bash
cd server
cp .env.prod.example .env.prod
APP_ENV=prod go run ./cmd/roomserver
```

如果是本地 `mediactl` 上传到云上 MinIO，也可以显式指定：

```bash
cd server
APP_ENV=prod go run ./cmd/mediactl upload --library-root ../media/raw --input ../media/raw/sample-show/season-01/episode-01.mp4 --dry-run=false
```

## Current Structure

```text
server/
├── cmd/
│   ├── mediactl/
│   │   └── main.go
│   └── roomserver/
│       └── main.go
├── compose.yaml
├── migrations/
│   └── README.md
├── scripts/
│   ├── migrate.sh
│   └── new_migration.sh
├── internal/
│   ├── app/
│   │   └── server.go
│   ├── auth/
│   │   └── service.go
│   ├── home/
│   │   └── service.go
│   ├── media/
│   │   └── service.go
│   ├── mediactl/
│   │   ├── ingest.go
│   │   └── ingest_test.go
│   ├── protocol/
│   │   ├── decode.go
│   │   ├── envelope.go
│   │   └── events.go
│   ├── progress/
│   │   └── service.go
│   ├── room/
│   │   ├── client.go
│   │   ├── manager.go
│   │   ├── manager_test.go
│   │   └── room.go
│   ├── roomapi/
│   │   └── service.go
│   ├── store/
│   │   ├── home_postgres.go
│   │   ├── media_postgres.go
│   │   ├── progress_postgres.go
│   │   ├── room_postgres.go
│   │   └── postgres.go
│   └── transport/
│       ├── api_response.go
│       ├── auth_http_handler.go
│       ├── auth_http_handler_test.go
│       ├── home_http_handler.go
│       ├── home_http_handler_test.go
│       ├── json.go
│       ├── media_http_handler.go
│       ├── media_http_handler_test.go
│       ├── progress_http_handler.go
│       ├── progress_http_handler_test.go
│       ├── room_http_handler.go
│       ├── room_http_handler_test.go
│       ├── websocket_handler.go
│       └── websocket_handler_test.go
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Directory Responsibilities

- `cmd/roomserver/`: 服务端启动入口
- `cmd/mediactl/`: 媒体维护 CLI 入口，当前支持 `ingest` dry-run、本地 HLS 生成、local / minio / s3-compatible uploader 和 PostgreSQL 写库
- `compose.yaml`: 本地 PostgreSQL 容器初始化入口
- `migrations/`: SQL-first migration 文件目录
- `scripts/`: migration 辅助脚本
- `internal/app/`: 应用组装层，负责配置和 HTTP server 初始化
- `internal/auth/`: 最小账号注册、登录、密码校验与 token 占位逻辑
- `internal/home/`: 首页 summary 业务聚合逻辑
- `internal/media/`: 媒体标签、媒体搜索与分页参数处理逻辑
- `internal/mediactl/`: 媒体入库 CLI 的参数解析、配置读取、dry-run 校验、HLS 生成、storage uploader 和写库逻辑
- `internal/protocol/`: 与 `INT-19` 对齐的最小协议结构，包含 create room 请求 / 响应和 WebSocket 事件模型
- `internal/progress/`: 用户媒体观看进度的低频写入业务逻辑
- `internal/room/`: 房间、连接、房间管理器，以及房间创建与控制状态更新的内存模型
- `internal/roomapi/`: 房间业务 API 的 create/join 服务层，负责 DB 主数据与运行时房间的边界
- `internal/store/`: PostgreSQL 读写入口，当前包含用户账号、首页 summary、媒体目录、房间业务与观看进度 store
- `internal/transport/`: auth、home、media、`POST /rooms`、`/ws`、join room 与 `play / pause / seek / set_playback_rate / ended` 的接入层和测试

当前关键文件对应关系：

- `cmd/roomserver/main.go`: 读取配置并启动服务
- `cmd/mediactl/main.go`: 媒体维护 CLI 进程入口
- `compose.yaml`: 启动本地 PostgreSQL 16 开发实例
- `migrations/README.md`: migration 命名规则与目录约定
- `scripts/new_migration.sh`: 创建新的 `up/down` SQL 迁移文件
- `scripts/migrate.sh`: 执行 `up/down/version/force` 迁移命令
- `Makefile`: 提供 `migration-create / migration-up / migration-down / migration-version` 入口
- `internal/app/server.go`: 注册 `/healthz`、`POST /auth/register`、`POST /auth/login`、`GET /home/summary`、`GET /media/tags`、`GET /media/items`、`POST /rooms`、`POST /rooms/{roomCode}/join`、`GET /rooms/{roomCode}`、`PUT /me/media-progress/{mediaItemId}` 和 `/ws`
- `internal/auth/service.go`: 注册、登录、bcrypt 密码哈希与 dev token 生成
- `internal/home/service.go`: 首页用户信息、上次观看和继续追番聚合逻辑
- `internal/media/service.go`: 媒体标签列表、搜索参数、分页 cursor 和结果裁剪逻辑
- `internal/mediactl/ingest.go`: `mediactl ingest` 参数解析、输入文件校验、dry-run summary、ffmpeg HLS 输出和写库主流程
- `internal/progress/service.go`: 用户媒体观看进度校验与低频写入逻辑
- `internal/roomapi/service.go`: 6 位房间码生成、DB-backed create room 和 join room by code 业务逻辑
- `internal/store/postgres.go`: PostgreSQL 连接与 `users` 读写
- `internal/store/home_postgres.go`: `users`、`user_media_progress`、`media_episodes` 与 `media_seasons` 的首页 summary 查询
- `internal/store/media_postgres.go`: `media_tags` 标签列表，以及 `media_seasons / media_episodes / media_season_tags` 的搜索和筛选查询
- `internal/store/progress_postgres.go`: `user_media_progress` 的 upsert 写入
- `internal/store/room_postgres.go`: `rooms`、`room_members`、`media_episodes` 与 `media_seasons` 的创建房间、加入和详情查询事务
- `internal/transport/auth_http_handler.go`: auth HTTP API 入口与统一 API envelope
- `internal/transport/home_http_handler.go`: `GET /home/summary` HTTP API 入口与 dev token 解析
- `internal/transport/media_http_handler.go`: `GET /media/tags`、`GET /media/items` HTTP API 入口和分页 envelope
- `internal/transport/progress_http_handler.go`: `PUT /me/media-progress/{mediaItemId}` HTTP API 入口
- `internal/protocol/events.go`: create room、join room、room_state、ended、heartbeat、error 等最小结构
- `internal/room/manager.go`: 房间创建、查询、唯一 `roomId` 生成和客户端清理
- `internal/transport/room_http_handler.go`: DB-backed create room 与 join by room code HTTP 入口
- `internal/transport/websocket_handler.go`: join room、heartbeat、host 校验、控制事件处理和广播
- `internal/room/room.go`: 房间成员、host 状态、authority timeline、ended completed state 和 repeated join 连接替换逻辑

## Current Validation

当前已完成的本地验证：

- `go test ./...`
- `go run ./cmd/mediactl ingest --library-root <root> --input <root>/<season>/season-01/episode-01.mp4 --title <title>` 输出 dry-run summary
- `go run ./cmd/mediactl ingest --library-root <root> --input <root>/<season>/season-01/episode-01.mp4 --title <title> --dry-run=false` 生成 HLS 并执行配置好的 storage uploader
- `POST /auth/register` 返回 `201 Created`、用户资料和 `dev_<userId>` access token
- `POST /auth/login` 校验 bcrypt 密码并返回统一 envelope
- 重复注册同一账号返回 `409 CONFLICT`
- 错误密码登录返回 `401 UNAUTHORIZED`
- `GET /home/summary` 返回当前用户、最近一次观看和最近 2 条未完成观看记录
- `GET /media/tags` 返回默认主标签和展开标签列表
- `GET /media/items` 支持 `query / tag / limit / cursor` 的媒体搜索与筛选
- `POST /rooms` 写入 DB 房间主数据、host 成员关系，并返回 `roomCode / media / roomState`
- `POST /rooms/{roomCode}/join` 根据 6 位房间码写入或恢复成员关系
- `GET /rooms/{roomCode}` 返回放映室首屏需要的 `room / media / members`
- `PUT /me/media-progress/{mediaItemId}` 低频写入用户媒体观看进度
- `go run ./cmd/roomserver`
- HTTP room API 返回统一 `data + meta` envelope
- `join_room` 仅允许加入已存在房间，不存在房间时返回 `error`
- host 发出的 `play / pause / seek` 会更新房间状态并广播
- host 发出的 `set_playback_rate` 会更新房间权威倍率并广播
- host 发出的 `ended` 会把房间收敛到稳定 completed state，并冻结当前位置
- `seek` 离开结尾时会清除 `ended`
- 非 host 发出的控制事件会返回 `error`
- 服务端会周期性发送 `heartbeat`，客户端需返回 `heartbeat_ack`
- 超时未 ack 的连接会进入现有断连清理流程
- host 断开连接后，剩余成员会收到新的 `room_state` 且 host 身份立即转移
- former host 在 host transfer 后重新 join room 时，会作为普通成员回到房间，不会隐式拿回 host 身份
- 同一 `userId` repeated join 同一房间时，新连接会替换旧连接并重新收到基于 authority timeline 结算后的最新 `room_state`
- repeated join / reconnect 在视频已播完时，会收到 `ended=true` 的稳定 `room_state`
- 最后一个成员离开后，房间不会立即销毁，而是进入 2 分钟 grace period；若期间有人重新加入则继续保留，否则自动销毁

其中 `POST /rooms`、`join_room`、`play / pause / seek / set_playback_rate / ended` 与 host transfer 的最小同步路径已通过基础测试验证。

## Mediactl

`mediactl` 是后续媒体资源制作、上传和入库的 CLI 入口。

当前已经支持本地多码率 HLS 生成和 PostgreSQL 媒体元数据写入；上传文件仍由后续任务补齐。

示例：

```bash
cd server
go run ./cmd/mediactl ingest \
  --library-root ../media/raw \
  --input ../media/raw/sample-show/season-01/episode-01.mp4 \
  --title "测试视频" \
  --season-label "第 1 季" \
  --episode-label "第 01 集" \
  --tags test,anime \
  --dry-run=false \
  --write-db
```

当前命令会完成：

- 校验 `--input` 文件存在
- 校验 `--library-root` 目录存在
- 自动推导 `source_key`
- 自动计算 `source_hash`
- 校验 `--cover` 文件存在，如果传入
- 校验 `--title` 不为空
- 读取媒体存储环境变量
- 默认输出 dry-run summary
- `--dry-run=false` 时调用 `ffmpeg` 生成 `master.m3u8`、`720p/index.m3u8`、必要时的 `1080p/index.m3u8` 和 `.ts` 分片
- `--dry-run=false` 时调用 `ffprobe` 读取源视频时长
- `--dry-run=false` 时调用 `ffprobe` 校验每个 variant 的实际分辨率
- `--dry-run=false --write-db` 时写入 `media_seasons / media_episodes / media_episode_variants / media_tags / media_season_tags`

后续任务会继续补充：

- `INT-141`: 接入 local / MinIO 上传抽象

## SQL-first Migration

当前 `server/` 已引入 SQL-first migration 基础设施。

当前约定：

- schema 变更统一以 SQL migration 文件作为唯一权威来源
- migration 文件存放在 `server/migrations/`
- migration 文件采用 `up/down` 成对命名
- 当前使用 `golang-migrate` CLI 作为执行工具

### Install

本地可使用：

```bash
brew install golang-migrate
```

### Create Migration

```bash
cd server
make migration-create name=create_users_table
```

### Run Migration

当前本地 PostgreSQL 和 MinIO 都通过 `docker compose` 启动。

#### Start Local PostgreSQL And MinIO

```bash
cd server
docker compose up -d
```

当前默认配置：

- image: `postgres:16`
- host: `127.0.0.1`
- port: `5432`
- user: `app`
- password: `app`
- database: `anime_watch_dev`
- MinIO API: `http://127.0.0.1:9100`
- MinIO Console: `http://127.0.0.1:9101`
- MinIO root user: `minioadmin`
- MinIO root password: `minioadmin`
- 自动初始化 bucket: `watch-together-media`

#### Set Database URL

先设置数据库连接：

```bash
export DATABASE_URL='postgres://app:app@127.0.0.1:5432/anime_watch_dev?sslmode=disable'
```

#### Run Migration

然后执行：

```bash
cd server
make migration-up
```

### Notes

当前这一阶段只完成了 migration 基础设施引入。

当前已经同时明确了本地 PostgreSQL 初始化方式：

- 使用 `server/compose.yaml`
- 使用 Docker Compose 管理本地 PostgreSQL 16 容器
- migration 执行依赖 `DATABASE_URL`

还未完成的内容包括：

- 服务端最小数据库读写接入

## First Schema

当前第一版业务主数据 schema 已经落为首个 migration：

- `migrations/20260420113000_create_initial_schema.up.sql`
- `migrations/20260420113000_create_initial_schema.down.sql`
- `migrations/20260420123000_add_account_fields_to_users.up.sql`
- `migrations/20260420123000_add_account_fields_to_users.down.sql`
- `migrations/20260421100000_add_user_media_progress.up.sql`
- `migrations/20260421100000_add_user_media_progress.down.sql`
- `migrations/20260421110000_add_user_profile_fields.up.sql`
- `migrations/20260421110000_add_user_profile_fields.down.sql`
- `migrations/20260421130000_add_media_search_fields.up.sql`
- `migrations/20260421130000_add_media_search_fields.down.sql`
- `migrations/20260421143000_add_media_tags.up.sql`
- `migrations/20260421143000_add_media_tags.down.sql`
- `migrations/20260429101000_add_media_season_episode_schema.up.sql`
- `migrations/20260429101000_add_media_season_episode_schema.down.sql`
- `migrations/20260430103000_add_episode_refs_to_rooms_and_progress.up.sql`
- `migrations/20260430103000_add_episode_refs_to_rooms_and_progress.down.sql`
- `migrations/20260501100000_remove_legacy_media_items_schema.up.sql`
- `migrations/20260501100000_remove_legacy_media_items_schema.down.sql`

当前包含的主表：

- `users`
- `media_seasons`
- `media_episodes`
- `media_episode_variants`
- `media_tags`
- `media_season_tags`
- `rooms`
- `room_members`
- `user_media_progress`

当前 `users` 额外包含最小账号登录字段：

- `account`
- `password_hash`
- `avatar_seed`
- `avatar_url`
- `bio`

当前 `user_media_progress` 额外承载：

- `last_position_seconds`
- `duration_seconds`
- `last_watched_at`
- `completed`
- `completion_source`

当前媒体内容模型已经收敛为 episode-backed 结构：

- `media_seasons` 表达一季、篇章、合集或作品容器
- `media_episodes` 表达真正可播放的一集或视频资源
- `media_episode_variants` 表达同一 episode 下的多码率 HLS variant，例如 `720p / 1080p`
- `media_season_tags` 表达 season 与标签目录的关系
- 服务端媒体列表、创建房间、房间详情、首页 summary 和观看进度均使用 `media_episodes / media_seasons`
- HTTP 字段名 `mediaItemId` 暂时保留，但语义已经是 `media_episodes.id`

当前标签目录与标签关联由以下表承载：

- `media_tags`
- `media_season_tags`

当前包含的关键约束：

- `rooms.room_code` 唯一且固定为 6 位
- `rooms.status` 当前收敛为 `active / grace_period / destroyed`
- `room_members.role` 当前收敛为 `host / member`
- 同一房间同一用户最多保留一个 active 成员关系

当前包含的第一版索引：

- `media_tags.is_active / sort_order`
- `media_tags.is_featured / is_active / sort_order`
- `media_season_tags.media_tag_id`
- `media_seasons.status / sort_order / category / original_title / production_team / search_aliases`
- `media_episodes.season_id / sort_order / status / source_hash`
- `rooms.host_user_id / media_episode_id / status / destroy_after`
- `room_members.room_id / user_id`
- active 成员关系相关索引
- `user_media_progress(user_id, media_episode_id)` 唯一约束
- `user_media_progress(user_id, last_watched_at desc)`
- `user_media_progress(user_id, completed, last_watched_at desc)`
