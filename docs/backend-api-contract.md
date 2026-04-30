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
- `INT-147` 后，观看进度展示数据来自 `users`、`user_media_progress`、`media_episodes` 与 `media_seasons`
- `media_items` 仅作为兼容期旧引用
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
      "mediaItemId": "episode_uuid",
      "title": "紫罗兰永恒花园",
      "coverUrl": "https://example.com/cover.jpg",
      "lastPositionSeconds": 564,
      "durationSeconds": 1458
    },
    "continueWatching": [
      {
        "mediaItemId": "episode_uuid_2",
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

Android 联调状态：

- `INT-127` 已接入 Android 首页
- 登录成功后，`WatchTogetherApp` 会把 `AuthSession.accessToken` 传给 `HomePage`
- `HomePage` 进入后调用 `GET /home/summary`
- 欢迎语使用 `user.nickname`
- 头像缩写优先使用 `user.avatarSeed`
- “上次观看”和“继续追番”使用 `lastWatched / continueWatching`
- Android 当前将进度展示为秒级 `mm:ss`
- Android 当前暂不加载远程 `coverUrl`，封面仍使用本地渐变占位
- 该接口失败不会阻断首页进入，只展示轻量错误提示

### Media

当前实现状态：

- `GET /media/tags` 已落地，对应 `INT-119`
- `GET /media/items` 已落地，对应 `INT-120`
- `INT-147` 后，`GET /media/items` 已迁移为 episode-backed 查询
- 标签目录仍来自 `media_tags`
- 媒体列表数据来自 `media_seasons / media_episodes / media_season_tags`
- `media_items / media_item_tags` 暂时保留为兼容期旧模型
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

Android 联调状态：

- `INT-128` 已接入 Android `02A 选择视频`
- Android 进入选片页后会调用 `GET /media/tags`
- 默认标签行优先使用 `featuredTags`
- 点击 `更多` 后使用 `allTags` 渲染悬浮标签面板
- Android 会保留一个本地 `全部` 伪标签，选中时不传 `tag`
- 标签接口失败时会显示轻量错误提示

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
- `query` 当前会命中 season 层的 `title / description / original_title / production_team / search_aliases`，以及 episode 层的 `title / subtitle / description / episode_label`。
- `tag` 使用 `media_tags.slug`，可为空。
- `limit` 默认 20，最大 50。
- `cursor` 对客户端不透明；Android 只需要原样带回下一页请求。

响应：

```json
{
  "data": {
    "items": [
      {
        "id": "episode_uuid",
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

Android 联调状态：

- `INT-128` 已接入 Android `02A 选择视频`
- Android 默认进入页面后会调用 `GET /media/items?limit=20`
- 搜索框变化后会用 `query` 重新请求媒体列表
- 标签变化后会用 `tag=<media_tags.slug>` 重新请求媒体列表
- Android 当前会展示接口返回的 `title` 和 `subtitle / episodeLabel / description` 中的首个可用描述
- Android 当前已把选中的 `mediaItemId` 传到创建房间入口回调，后续 `POST /rooms` 接入时直接使用
- `INT-147` 后，`items[].id` 是 `media_episodes.id`；HTTP 字段名暂时仍叫 `mediaItemId`，但语义已经是 episode-backed id
- Android 当前暂不加载远程 `coverUrl`，封面仍使用本地渐变占位

### Rooms

当前实现状态：

- `POST /rooms` 已调整为 DB-backed create room，对应 `INT-121`
- `POST /rooms/{roomCode}/join` 已落地，对应 `INT-122`
- `INT-147` 后，房间媒体业务主数据优先来自 PostgreSQL `rooms / room_members / media_episodes / media_seasons`
- `rooms.media_episode_id` 是后续 create/detail 的主要媒体引用
- `rooms.media_item_id` 暂时保留为兼容期旧引用
- 创建房间和加入房间都需要 `Authorization: Bearer dev_<userId>`
- `room.id` 是 PostgreSQL 中的 UUID 主键
- `room.roomCode` 是 6 位可分享房间码，也是当前 WebSocket `join_room.roomId` 使用的运行时房间 key
- HTTP 负责创建/加入业务关系，WebSocket 仍负责实时连接、初始 `room_state` 和播放同步

#### `POST /rooms`

用途：根据选中的媒体创建房间。

请求：

```http
POST /rooms
Authorization: Bearer dev_user_uuid
Content-Type: application/json
Accept: application/json
```

请求：

```json
{
  "mediaItemId": "episode_uuid"
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
      "mediaItemId": "episode_uuid",
      "status": "active"
    },
    "media": {
      "id": "episode_uuid",
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

- 创建成功后会写入 `rooms` 和 host 的 `room_members` 记录。
- 创建房间时 `mediaItemId` 当前传 `media_episodes.id`。
- 兼容期内如果客户端仍传旧 `media_items.id`，服务端会通过 `media_episodes.legacy_media_item_id` 解析到 episode。
- `rooms.media_episode_id` 会保存 episode id；`rooms.media_item_id` 仅在存在 legacy 对应时继续写入。
- 服务端会同时把 `roomCode` 注册到内存同步房间中，方便紧接着建立 WebSocket。
- Android 创建成功后进入 `03 放映室`，后续 WebSocket `join_room.payload.roomId` 当前传 `room.roomCode`。
- `roomState` 是新房间的初始运行时状态，用于首屏占位；真正实时状态仍以 WebSocket `room_state` 为准。
- Android `INT-129` 已接入该接口。
- Android 从 `02A 选择视频` 点击创建房间时，会使用当前选中的 episode-backed `mediaItemId` 请求该接口。
- Android 会使用响应中的 `media.title / media.episodeLabel / media.mediaUrl` 更新 `03 放映室` 与播放器载入地址。
- Android 不再使用默认样例媒体创建真实业务房间。

错误：

- `400 VALIDATION_ERROR`: `mediaItemId` 为空或请求体不是合法 JSON
- `401 UNAUTHORIZED`: 缺少 token、token 不是当前 dev token 形态，或用户不存在
- `404 NOT_FOUND`: 媒体不存在或不是 active
- `409 CONFLICT`: 短时间内无法生成唯一 6 位房间码
- `503 INTERNAL_ERROR`: 服务端未连接数据库，room service 不可用

#### `POST /rooms/{roomCode}/join`

用途：通过 6 位房间码加入房间业务关系。

请求：

```http
POST /rooms/A7K2M9/join
Authorization: Bearer dev_user_uuid
Accept: application/json
```

响应：

```json
{
  "data": {
    "room": {
      "id": "room_uuid",
      "roomCode": "A7K2M9",
      "hostUserId": "user_uuid",
      "mediaItemId": "episode_uuid",
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

说明：

- 如果用户已经是该房间 active member，接口保持幂等并返回现有成员身份。
- 如果用户之前离开过但房间仍存在，接口会恢复成员关系并将其作为 `member`。
- 该接口只处理业务成员关系，不替代 WebSocket `join_room`。
- Android 调用成功后仍需连接 `/ws` 并发送 `join_room`，其中 `roomId` 当前传 `room.roomCode`。

错误：

- `400 VALIDATION_ERROR`: 房间码格式不合法
- `401 UNAUTHORIZED`: 缺少 token、token 不是当前 dev token 形态，或用户不存在
- `404 NOT_FOUND`: 房间不存在、已销毁或不可加入
- `503 INTERNAL_ERROR`: 服务端未连接数据库，room service 不可用

#### `GET /rooms/{roomCode}`

用途：支撑 `03 放映室` 首屏业务数据。

当前实现状态：

- `GET /rooms/{roomCode}` 已落地，对应 `INT-123`
- 数据来自 PostgreSQL `rooms / room_members / users / media_episodes / media_seasons`
- `media_items` 仅作为兼容期桥接来源
- 响应只包含业务主数据，不包含实时 `positionMs / playbackRate / paused / ended / seq`
- 实时同步状态仍必须等待 WebSocket `room_state`

请求：

```http
GET /rooms/A7K2M9
Accept: application/json
```

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
      "id": "episode_uuid",
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

说明：

- Android 进入 `03 放映室` 时可以先调用该接口拿房间码、媒体信息和成员展示信息。
- 该接口不会返回 WebSocket 在线连接状态。
- 该接口不会替代 WebSocket `join_room`。
- 如果需要播放同步，客户端仍必须连接 `/ws` 并等待 `room_state`。
- Android `INT-130` 已接入该接口。
- Android 在进入或加入放映室时会调用 `GET /rooms/{roomCode}` 补齐业务首屏数据。
- Android 会先尽量加载 room detail，再启动 WebSocket `join_room`，避免 `room_state.mediaId` 到达时缺少真实 `media.mediaUrl`。
- Android 使用该接口返回的 `media.title / media.episodeLabel / media.mediaUrl` 展示和载入影片。
- WebSocket `room_state` 仍是 `positionMs / playbackRate / paused / ended / seq` 的实时权威。

错误：

- `400 VALIDATION_ERROR`: 房间码格式不合法
- `404 NOT_FOUND`: 房间不存在、已销毁或不可加入
- `503 INTERNAL_ERROR`: 服务端未连接数据库，room service 不可用

### Progress

#### `PUT /me/media-progress/{mediaItemId}`

用途：低频写入用户观看进度。

当前实现状态：

- `PUT /me/media-progress/{mediaItemId}` 已落地，对应 `INT-124`
- 写入 PostgreSQL `user_media_progress`
- `INT-147` 后优先写入 `user_media_progress.media_episode_id`
- `user_media_progress.media_item_id` 暂时保留为兼容期旧引用
- 使用 `Authorization: Bearer dev_<userId>` 识别当前用户
- 进度以秒级写入，用于首页 `lastWatched / continueWatching`
- 不用于实时播放同步，不替代 WebSocket authority state

请求：

```http
PUT /me/media-progress/episode_uuid
Authorization: Bearer dev_user_uuid
Content-Type: application/json
Accept: application/json
```

请求：

```json
{
  "lastPositionSeconds": 564,
  "durationSeconds": 1458,
  "completed": false,
  "completionSource": null
}
```

响应：

```json
{
  "data": {
    "mediaItemId": "episode_uuid",
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

说明：

- 普通播放进度 tick 不需要传 `completionSource`，传 `null` 或省略即可。
- 路径中的 `mediaItemId` 当前语义为 episode-backed id，即 `media_episodes.id`。
- 兼容期内如果传旧 `media_items.id`，服务端会通过 `media_episodes.legacy_media_item_id` 解析到 episode。
- `completionSource` 仅在明确完成语义时使用。
- 当前允许值：`ended`、`manual_mark`、`threshold_auto`。
- `lastPositionSeconds` 必须大于等于 0。
- `durationSeconds` 必须大于 0。
- `lastPositionSeconds` 必须小于等于 `durationSeconds`。
- 该接口建议低频调用，例如页面离开、暂停、播放结束、或间隔一段时间上报一次；不要按播放器帧或高频 tick 写入。
- Android `INT-131` 已接入该接口。
- Android 当前在暂停、播放结束、以及约 30 秒一次的低频 tick 上报进度。
- Android 上报秒级 `lastPositionSeconds / durationSeconds`，不上传毫秒。
- Android 播放结束时上报 `completed=true` 和 `completionSource=ended`。
- 该接口只服务业务观看历史，不参与 WebSocket 实时同步。

错误：

- `400 VALIDATION_ERROR`: 请求体非法、进度范围非法或 `completionSource` 不合法
- `401 UNAUTHORIZED`: 缺少 token、token 不是当前 dev token 形态，或用户不存在
- `404 NOT_FOUND`: 媒体不存在或不是 active
- `503 INTERNAL_ERROR`: 服务端未连接数据库，progress service 不可用

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

导入本地 mock 数据：

```bash
cd server
psql "$DATABASE_URL" -f seeds/dev_seed.sql
```

当前 seed 固定数据：

- host user: `00000000-0000-0000-0000-000000000001`
- viewer user: `00000000-0000-0000-0000-000000000002`
- test password: `secret`
- host dev token: `dev_00000000-0000-0000-0000-000000000001`
- viewer dev token: `dev_00000000-0000-0000-0000-000000000002`
- media item: `10000000-0000-0000-0000-000000000001`，`紫罗兰永恒花园`
- media item: `10000000-0000-0000-0000-000000000002`，`孤独摇滚!`
- media item: `10000000-0000-0000-0000-000000000003`，`葬送的芙莉莲`

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

### Room 本地联调示例

创建房间：

```bash
curl -s http://127.0.0.1:8080/rooms \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dev_<userId>' \
  -d '{"mediaItemId":"<mediaItemId>"}'
```

通过房间码加入：

```bash
curl -s -X POST http://127.0.0.1:8080/rooms/A7K2M9/join \
  -H 'Authorization: Bearer dev_<userId>'
```

获取放映室首屏数据：

```bash
curl -s http://127.0.0.1:8080/rooms/A7K2M9
```

Android 联调注意：

- 创建房间接口使用选片页返回的 `media.id` 作为 `mediaItemId`。
- 创建成功后，右上角展示 `room.roomCode`，而不是 `room.id`。
- 加入房间弹窗输入的是 6 位 `roomCode`。
- 当前 WebSocket `join_room.payload.roomId` 继续传 `room.roomCode`，这样能复用已经稳定的运行时同步链路。
- HTTP `room.id` 是数据库 UUID，只用于后续业务 API 和持久化关联，不直接作为当前 WebSocket join key。
- HTTP create/join 成功不代表已经进入实时同步；Android 仍需建立 `/ws` 并等待 `room_state`。
- `GET /rooms/{roomCode}` 只负责放映室首屏业务数据，不返回实时播放状态。

### Progress 本地联调示例

写入观看进度：

```bash
curl -s -X PUT http://127.0.0.1:8080/me/media-progress/10000000-0000-0000-0000-000000000001 \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dev_00000000-0000-0000-0000-000000000001' \
  -d '{"lastPositionSeconds":600,"durationSeconds":1458,"completed":false}'
```

播放结束时写入完成状态：

```bash
curl -s -X PUT http://127.0.0.1:8080/me/media-progress/10000000-0000-0000-0000-000000000001 \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dev_00000000-0000-0000-0000-000000000001' \
  -d '{"lastPositionSeconds":1458,"durationSeconds":1458,"completed":true,"completionSource":"ended"}'
```

Android 联调注意：

- 该接口是低频业务进度写入，只服务首页“上次观看”和“继续追番”。
- 不要用它驱动实时同步播放。
- 实时播放状态继续以 WebSocket authority state 为准。
- 普通进度上报不传 `completionSource`。
- 只有 ended、手动标记或阈值自动完成时才传 `completionSource`。

## 后续任务

- `INT-116`: 登录接口
- `INT-117`: 注册接口
- `INT-118`: 首页 summary 接口
- `INT-119`: 媒体标签接口，已落地
- `INT-120`: 媒体搜索接口，已落地
- `INT-121`: DB-backed create room 接口，已落地
- `INT-122`: room code join 接口，已落地
- `INT-123`: room detail 接口，已落地
- `INT-124`: 观看进度写入接口，已落地
- `INT-125`: API 文档持续维护，当前第一批接口已同步
