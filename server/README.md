# Server

`server/` 用于放置房间服务与后端相关代码。

当前目录已经完成 `INT-14 bootstrap websocket room server` 的第一阶段骨架，主要包含：

- 可启动的 Go HTTP / WebSocket 服务入口
- `POST /rooms` 的最小 create room 能力
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

## Current Structure

```text
server/
├── cmd/
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
│   ├── protocol/
│   │   ├── decode.go
│   │   ├── envelope.go
│   │   └── events.go
│   ├── room/
│   │   ├── client.go
│   │   ├── manager.go
│   │   ├── manager_test.go
│   │   └── room.go
│   └── transport/
│       ├── json.go
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
- `compose.yaml`: 本地 PostgreSQL 容器初始化入口
- `migrations/`: SQL-first migration 文件目录
- `scripts/`: migration 辅助脚本
- `internal/app/`: 应用组装层，负责配置和 HTTP server 初始化
- `internal/protocol/`: 与 `INT-19` 对齐的最小协议结构，包含 create room 请求 / 响应和 WebSocket 事件模型
- `internal/room/`: 房间、连接、房间管理器，以及房间创建与控制状态更新的内存模型
- `internal/transport/`: `POST /rooms`、`/ws`、join room 与 `play / pause / seek / set_playback_rate / ended` 的接入层和测试

当前关键文件对应关系：

- `cmd/roomserver/main.go`: 读取配置并启动服务
- `compose.yaml`: 启动本地 PostgreSQL 16 开发实例
- `migrations/README.md`: migration 命名规则与目录约定
- `scripts/new_migration.sh`: 创建新的 `up/down` SQL 迁移文件
- `scripts/migrate.sh`: 执行 `up/down/version/force` 迁移命令
- `Makefile`: 提供 `migration-create / migration-up / migration-down / migration-version` 入口
- `internal/app/server.go`: 注册 `/healthz`、`POST /rooms` 和 `/ws`
- `internal/protocol/events.go`: create room、join room、room_state、ended、heartbeat、error 等最小结构
- `internal/room/manager.go`: 房间创建、查询、唯一 `roomId` 生成和客户端清理
- `internal/transport/room_http_handler.go`: create room HTTP 入口
- `internal/transport/websocket_handler.go`: join room、heartbeat、host 校验、控制事件处理和广播
- `internal/room/room.go`: 房间成员、host 状态、authority timeline、ended completed state 和 repeated join 连接替换逻辑

## Current Validation

当前已完成的本地验证：

- `go test ./...`
- `go run ./cmd/roomserver`
- `POST /rooms` 返回 `201 Created` 与初始 `room_state`
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

当前本地 PostgreSQL 通过 `docker compose` 启动。

#### Start Local PostgreSQL

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

当前包含的主表：

- `users`
- `media_items`
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

当前包含的关键约束：

- `rooms.room_code` 唯一且固定为 6 位
- `rooms.status` 当前收敛为 `active / grace_period / destroyed`
- `room_members.role` 当前收敛为 `host / member`
- 同一房间同一用户最多保留一个 active 成员关系

当前包含的第一版索引：

- `media_items.category / status`
- `media_items.tags` GIN 索引
- `rooms.host_user_id / media_item_id / status / destroy_after`
- `room_members.room_id / user_id`
- active 成员关系相关索引
- `user_media_progress(user_id, media_item_id)` 唯一约束
- `user_media_progress(user_id, last_watched_at desc)`
- `user_media_progress(user_id, completed, last_watched_at desc)`
