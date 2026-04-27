# 后端 HTTP API 契约与前后端联调说明

> 对应 Linear `INT-115`。本文档定义当前阶段 Android 与 Server 进行 HTTP API 联调时必须共享的接口规则。WebSocket 同步协议仍以 [websocket-event-protocol.md](./websocket-event-protocol.md) 为准。

## 目标

当前阶段要先统一 HTTP API 的基础风格，而不是直接分散实现各个接口。

本文档先固定：

- API base URL 与版本策略
- 请求与响应 JSON 规则
- 成功响应 envelope
- 错误响应 envelope
- 分页参数
- 鉴权占位
- HTTP API 与 WebSocket 的职责边界
- 第一批后端业务接口清单

## 职责边界

### HTTP API 负责

- 账号登录与注册
- 当前用户资料
- 首页 summary 数据
- 媒体标签与媒体搜索
- 房间业务主数据
- 观看进度低频写入

### WebSocket 负责

- `room_state`
- `play`
- `pause`
- `seek`
- `set_playback_rate`
- `ended`
- `heartbeat`
- `seq`
- authority timeline
- 当前在线连接状态

### 不应通过 HTTP 高频写入的状态

- `positionMs`
- `playbackRate`
- `paused`
- `ended`
- `seq`
- 当前 WebSocket 在线连接
- heartbeat 最近时间
- drift correction 观测值

这些状态属于实时同步链路，当前继续以内存 authority state 为准。

## Base URL

本地开发默认：

```text
http://127.0.0.1:8080
```

Android 模拟器访问宿主机时通常使用：

```text
http://10.0.2.2:8080
```

客户端配置仍通过 `API_BASE_URL` / Android `BuildConfig` 注入，具体环境参数见 [environment-config.md](./environment-config.md)。

## API Version

第一版先不在路径中强制加入 `/v1`。

当前路径形态：

```text
/auth/login
/home/summary
/media/items
/rooms
```

如果后续出现破坏性变更，再统一迁移到：

```text
/v1/...
```

## Content Type

请求和响应统一使用 JSON：

```http
Content-Type: application/json
Accept: application/json
```

字段命名统一使用 `camelCase`，与 Android / WebSocket 协议保持一致。

## 成功响应 Envelope

所有 HTTP API 成功响应统一使用：

```json
{
  "data": {},
  "meta": {
    "requestId": "req_20260426_001"
  }
}
```

说明：

- `data` 是业务数据。
- `meta.requestId` 用于日志定位。
- 无分页时，`meta` 至少保留 `requestId`。

列表接口响应：

```json
{
  "data": {
    "items": []
  },
  "meta": {
    "requestId": "req_20260426_002",
    "page": {
      "limit": 20,
      "nextCursor": "cursor_next"
    }
  }
}
```

## 错误响应 Envelope

所有错误响应统一使用：

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "account is required",
    "details": {
      "field": "account"
    }
  },
  "meta": {
    "requestId": "req_20260426_003"
  }
}
```

### 第一版错误码

| Code | HTTP Status | 使用场景 |
| --- | --- | --- |
| `VALIDATION_ERROR` | `400` | 请求参数缺失或格式错误 |
| `UNAUTHORIZED` | `401` | 未登录或登录态无效 |
| `FORBIDDEN` | `403` | 已登录但无权限 |
| `NOT_FOUND` | `404` | 用户、媒体、房间不存在 |
| `CONFLICT` | `409` | 账号已存在、房间状态冲突 |
| `INTERNAL_ERROR` | `500` | 服务端未知错误 |

## 鉴权占位

第一版接口先约定 `Authorization` header，但具体 token 机制可以在 `INT-116` / `INT-117` 实现时收敛。

推荐形态：

```http
Authorization: Bearer <accessToken>
```

登录接口响应先预留：

```json
{
  "data": {
    "user": {
      "id": "user_uuid",
      "account": "xingye",
      "nickname": "Xingye",
      "avatarSeed": "xingye"
    },
    "accessToken": "dev_token_placeholder"
  },
  "meta": {
    "requestId": "req_20260426_004"
  }
}
```

当前阶段如果暂不实现真实 JWT，也不要让 Android 直接依赖“无 token 登录”的最终形态。

## 分页规则

列表接口统一使用 cursor 风格。

请求参数：

```text
limit=20
cursor=opaque_cursor
```

响应：

```json
{
  "meta": {
    "page": {
      "limit": 20,
      "nextCursor": "opaque_cursor_or_null"
    }
  }
}
```

当前第一版约定：

- 默认 `limit = 20`
- 最大 `limit = 50`
- `cursor` 对客户端不透明

## 第一批接口清单

### Auth

当前实现状态：

- `POST /auth/register` 已落地，对应 `INT-117`
- `POST /auth/login` 已落地，对应 `INT-116`
- 密码使用 `bcrypt` 写入 `users.password_hash`
- 账号写入前会做 trim + lowercase 归一化
- 当前 `accessToken` 为 `dev_<userId>` 占位 token，后续再替换为正式 token/session 机制
- 启动服务端前必须配置 `DATABASE_URL`，否则 auth endpoints 会返回 `503`

#### `POST /auth/login`

用途：账号密码登录。

请求：

```json
{
  "account": "xingye",
  "password": "password"
}
```

响应：

```json
{
  "data": {
    "user": {
      "id": "user_uuid",
      "account": "xingye",
      "nickname": "Xingye",
      "avatarSeed": "xingye",
      "avatarUrl": null
    },
    "accessToken": "dev_token_placeholder"
  },
  "meta": {
    "requestId": "req_001"
  }
}
```

错误：

- `400 VALIDATION_ERROR`: `account` 或 `password` 为空，或请求体不是合法 JSON
- `401 UNAUTHORIZED`: 账号不存在或密码错误
- `503 INTERNAL_ERROR`: 服务端未连接数据库，auth service 不可用

#### `POST /auth/register`

用途：账号注册。

请求：

```json
{
  "account": "xingye",
  "password": "password",
  "nickname": "Xingye"
}
```

响应同登录。

错误：

- `400 VALIDATION_ERROR`: `account`、`password` 或 `nickname` 为空，或请求体不是合法 JSON
- `409 CONFLICT`: `account` 已存在
- `503 INTERNAL_ERROR`: 服务端未连接数据库，auth service 不可用

### Me

#### `GET /me`

用途：获取当前登录用户资料。

响应：

```json
{
  "data": {
    "user": {
      "id": "user_uuid",
      "account": "xingye",
      "nickname": "Xingye",
      "avatarSeed": "xingye",
      "avatarUrl": null,
      "bio": null
    }
  },
  "meta": {
    "requestId": "req_002"
  }
}
```

### Home

当前实现状态：

- `GET /home/summary` 已落地，对应 `INT-118`
- 使用 `Authorization: Bearer dev_<userId>` 读取当前用户
- 数据来自 `users` 与 `user_media_progress` join `media_items`
- `lastWatched` 没有记录时返回 `null`
- `continueWatching` 当前取最近 2 条未完成记录
- 启动服务端前必须配置 `DATABASE_URL`，否则 home endpoints 会返回 `503`

#### `GET /home/summary`

用途：支撑 `02 首页与加入房间`。

请求：

```http
GET /home/summary
Authorization: Bearer dev_user_uuid
Accept: application/json
```

响应：

```json
{
  "data": {
    "user": {
      "nickname": "Xingye",
      "avatarSeed": "xingye",
      "avatarUrl": null
    },
    "lastWatched": {
      "mediaItemId": "media_uuid",
      "title": "紫罗兰永恒花园",
      "coverUrl": "https://example.com/cover.jpg",
      "lastPositionSeconds": 564,
      "durationSeconds": 1458
    },
    "continueWatching": [
      {
        "mediaItemId": "media_uuid_2",
        "title": "孤独摇滚!",
        "coverUrl": "https://example.com/cover2.jpg",
        "lastPositionSeconds": 120,
        "durationSeconds": 1440
      }
    ]
  },
  "meta": {
    "requestId": "req_003"
  }
}
```

错误：

- `401 UNAUTHORIZED`: 缺少 token、token 不是当前 dev token 形态，或用户不存在
- `503 INTERNAL_ERROR`: 服务端未连接数据库，home service 不可用

### Media

当前实现状态：

- `GET /media/tags` 已落地，对应 `INT-119`
- `GET /media/items` 已落地，对应 `INT-120`
- 数据来自 `media_items`、`media_tags`、`media_item_tags`
- `featuredTags` 当前返回最多 5 个 `is_featured = true` 且 `is_active = true` 的标签
- `allTags` 当前返回最多 20 个 `is_active = true` 的标签
- `GET /media/items` 支持 `query`、`tag`、`limit`、`cursor`
- 当前 cursor 为服务端不透明字符串，第一版内部使用 offset 表达
- 启动服务端前必须配置 `DATABASE_URL`，否则 media endpoints 会返回 `503`

#### `GET /media/tags`

用途：支撑 `02A 选择视频` 标签筛选。

请求：

```http
GET /media/tags
Accept: application/json
```

响应：

```json
{
  "data": {
    "featuredTags": [
      {
        "id": "tag_uuid",
        "slug": "healing",
        "name": "治愈"
      }
    ],
    "allTags": []
  },
  "meta": {
    "requestId": "req_004"
  }
}
```

约束：

- `featuredTags` 用于默认展示，建议最多 5 个。
- `allTags` 用于点击 `更多` 后的浮层，当前最多返回 20 个。
- Android 默认标签行优先使用 `featuredTags`。
- Android 点击 `更多` 后使用 `allTags` 渲染悬浮标签面板。

错误：

- `503 INTERNAL_ERROR`: 服务端未连接数据库，media service 不可用

#### `GET /media/items`

用途：媒体搜索与标签筛选。

查询参数：

```text
query=紫罗兰
tag=healing
limit=20
cursor=...
```

说明：

- `query` 可为空；为空时返回默认媒体列表。
- `query` 当前会命中 `title / subtitle / description / original_title / production_team / search_aliases`。
- `tag` 使用 `media_tags.slug`，可为空。
- `limit` 默认 20，最大 50。
- `cursor` 对客户端不透明；Android 只需要原样带回下一页请求。

响应：

```json
{
  "data": {
    "items": [
      {
        "id": "media_uuid",
        "title": "紫罗兰永恒花园",
        "subtitle": "治愈冒险",
        "description": "适合慢慢看",
        "coverUrl": "https://example.com/cover.jpg",
        "durationMs": 1458000,
        "seasonLabel": "第 1 季",
        "episodeLabel": "第 09 集",
        "tags": [
          {
            "slug": "healing",
            "name": "治愈"
          }
        ]
      }
    ]
  },
  "meta": {
    "requestId": "req_005",
    "page": {
      "limit": 20,
      "nextCursor": null
    }
  }
}
```

错误：

- `400 VALIDATION_ERROR`: `limit` 或 `cursor` 格式不合法
- `503 INTERNAL_ERROR`: 服务端未连接数据库，media service 不可用

### Rooms

#### `POST /rooms`

用途：根据选中的媒体创建房间。

请求：

```json
{
  "mediaItemId": "media_uuid"
}
```

响应：

```json
{
  "data": {
    "room": {
      "id": "room_uuid",
      "roomCode": "A7K2M9",
      "hostUserId": "user_uuid",
      "mediaItemId": "media_uuid",
      "status": "active"
    },
    "media": {
      "id": "media_uuid",
      "title": "紫罗兰永恒花园",
      "mediaUrl": "https://example.com/index.m3u8",
      "durationMs": 1458000,
      "seasonLabel": "第 1 季",
      "episodeLabel": "第 09 集"
    },
    "roomState": {
      "paused": true,
      "positionMs": 0,
      "playbackRate": 1.0,
      "ended": false,
      "seq": 1
    }
  },
  "meta": {
    "requestId": "req_006"
  }
}
```

说明：

- HTTP 创建房间负责业务主数据。
- WebSocket `join_room` 仍负责实时连接进入房间。

#### `POST /rooms/{roomCode}/join`

用途：通过 6 位房间码加入房间业务关系。

响应：

```json
{
  "data": {
    "room": {
      "id": "room_uuid",
      "roomCode": "A7K2M9",
      "hostUserId": "user_uuid",
      "mediaItemId": "media_uuid",
      "status": "active"
    },
    "member": {
      "userId": "user_uuid_2",
      "role": "member"
    }
  },
  "meta": {
    "requestId": "req_007"
  }
}
```

#### `GET /rooms/{roomCode}`

用途：支撑 `03 放映室` 首屏业务数据。

响应：

```json
{
  "data": {
    "room": {
      "id": "room_uuid",
      "roomCode": "A7K2M9",
      "hostUserId": "user_uuid",
      "status": "active"
    },
    "media": {
      "id": "media_uuid",
      "title": "紫罗兰永恒花园",
      "subtitle": "和搭子一起继续看到第 09 集",
      "mediaUrl": "https://example.com/index.m3u8",
      "durationMs": 1458000,
      "seasonLabel": "第 1 季",
      "episodeLabel": "第 09 集"
    },
    "members": [
      {
        "userId": "user_uuid",
        "nickname": "Xingye",
        "avatarSeed": "xingye",
        "avatarUrl": null,
        "role": "host"
      }
    ]
  },
  "meta": {
    "requestId": "req_008"
  }
}
```

### Progress

#### `PUT /me/media-progress/{mediaItemId}`

用途：低频写入用户观看进度。

请求：

```json
{
  "lastPositionSeconds": 564,
  "durationSeconds": 1458,
  "completed": false,
  "completionSource": "player_tick"
}
```

响应：

```json
{
  "data": {
    "mediaItemId": "media_uuid",
    "lastPositionSeconds": 564,
    "durationSeconds": 1458,
    "completed": false,
    "lastWatchedAt": "2026-04-26T12:00:00Z"
  },
  "meta": {
    "requestId": "req_009"
  }
}
```

## 前后端联调规则

### Server 侧

- 所有新 HTTP handler 必须返回统一 envelope。
- 所有错误必须返回统一错误 envelope。
- handler 不直接返回内部 DB model。
- API 字段使用 `camelCase`。
- WebSocket authority state 不应直接作为 DB 高频写入来源。

### Android 侧

- 统一通过 `API_BASE_URL` 访问 HTTP API。
- Android DTO 字段应与本文档保持一致。
- HTTP API 只负责业务主数据，不替代 WebSocket 同步事件。
- 登录成功后保存 `accessToken` 的位置后续由 Android API 接入任务确定。

### 联调顺序

1. `GET /healthz`
2. `POST /auth/register`
3. `POST /auth/login`
4. `GET /home/summary`
5. `GET /media/tags`
6. `GET /media/items`
7. `POST /rooms`
8. `POST /rooms/{roomCode}/join`
9. WebSocket `/ws` + `join_room`
10. `GET /rooms/{roomCode}`
11. `PUT /me/media-progress/{mediaItemId}`

### Auth 本地联调示例

启动 PostgreSQL 并执行 migration：

```bash
cd server
docker compose up -d
export DATABASE_URL='postgres://app:app@127.0.0.1:5432/anime_watch_dev?sslmode=disable'
make migration-up
go run ./cmd/roomserver
```

注册：

```bash
curl -s http://127.0.0.1:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"account":"xingye","password":"secret","nickname":"Xingye"}'
```

登录：

```bash
curl -s http://127.0.0.1:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"account":"xingye","password":"secret"}'
```

### Media 本地联调示例

获取标签：

```bash
curl -s http://127.0.0.1:8080/media/tags
```

默认媒体列表：

```bash
curl -s 'http://127.0.0.1:8080/media/items?limit=20'
```

按搜索词检索：

```bash
curl -s 'http://127.0.0.1:8080/media/items?query=紫罗兰&limit=20'
```

按标签筛选：

```bash
curl -s 'http://127.0.0.1:8080/media/items?tag=healing&limit=20'
```

分页：

```bash
curl -s 'http://127.0.0.1:8080/media/items?limit=20&cursor=<nextCursor>'
```

Android 联调注意：

- `featuredTags` 用于默认显示的一行标签。
- `allTags` 用于点击 `更多` 后的悬浮标签列表。
- `tag` 参数传 `slug`，不要传中文 `name`。
- `nextCursor` 不为空时表示还有下一页；客户端不解析 cursor 内容，只原样传回。
- 当前搜索仍是 PostgreSQL 第一版实现，先满足基础模糊搜索和标签筛选；如果后续体验不够，再评估 Meilisearch / OpenSearch。

## 后续任务

- `INT-116`: 登录接口
- `INT-117`: 注册接口
- `INT-118`: 首页 summary 接口
- `INT-119`: 媒体标签接口，已落地
- `INT-120`: 媒体搜索接口，已落地
- `INT-121`: DB-backed create room 接口
- `INT-122`: room code join 接口
- `INT-123`: room detail 接口
- `INT-124`: 观看进度写入接口
- `INT-125`: API 文档持续维护
