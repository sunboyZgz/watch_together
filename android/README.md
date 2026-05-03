# Android

`android/` 用于放置 Android 客户端工程。

当前已经落地的内容包括：

- `pages/` 目录下的真实业务页面入口
- `01 登录页` 以及抽离后的轻量登录弹窗组件
- `02 首页与加入房间` 的真实业务页面
- `02A 选择视频` 的真实业务页面与本地交互骨架
- `03 放映室` 的真实业务页面壳层
- Kotlin + Compose Android 应用工程
- 基于 AndroidX Media3 ExoPlayer 的播放器适配层
- 播放器事件回调与调试面板
- 基于 `POST /rooms` + `/ws` 的 Android 首个 join-time initial state sync 实现
- 基于 WebSocket 的 `play / pause / seek` 最小控制同步实现
- 基于 WebSocket 的 `set_playback_rate` 最小控制同步实现
- 基于 WebSocket 的 `ended` completed-state 同步实现
- 基于 WebSocket 的应用层 heartbeat 接收与 `heartbeat_ack` 回包
- 基于 authority baseline 的最小 drift correction
- same-user repeated join 时的本地 resync flow
- Android 侧最小协议草案与同步状态模型

当前目录内的关键职责：

- `app/src/main/java/.../ui/player/`：播放器页面、播放器适配器、播放器事件与页面状态模型
  当前仅保留播放器核心壳层、调试壳层、页面状态模型和同步事件映射，不承载具体业务页面视觉
- `app/src/main/java/.../sync/`：建房、入房、协议解码、heartbeat ack 和 join 后初始状态应用
- `app/src/main/java/.../sync/protocol/`：Android 侧最小协议草案模型
- `app/src/main/java/.../config/`：Android 配置注入入口

## Current Structure

```text
android/
├── app/
│   └── src/main/java/com/example/watch_together/
│       ├── pages/
│       │   ├── WatchTogetherApp.kt
│       │   ├── home/
│       │   ├── login/
│       │   ├── room/
│       │   └── video/
│       ├── config/
│       │   └── AppConfig.kt
│       ├── sync/
│       │   ├── RoomSessionController.kt
│       │   ├── RoomHttpClient.kt
│       │   ├── RoomSyncCoordinator.kt
│       │   ├── RoomSyncState.kt
│       │   ├── RoomWebSocketClient.kt
│       │   ├── SyncMessageDecoder.kt
│       │   └── protocol/
│       ├── ui/
│       │   ├── player/
│       │   └── theme/
│       └── MainActivity.kt
│   └── src/test/java/com/example/watch_together/
├── gradle/
│   └── libs.versions.toml
└── README.md
```

各部分职责：

- `pages/`：真实业务页面入口与页面级组件
  当前已包含 `WatchTogetherApp`、`LoginPage`、`LoginDialog`、`HomePage`、`VideoSelectionPage` 与 `RoomTheaterPage`
- `config/`：统一读取 `BuildConfig` 并生成 Android 端可直接使用的 URL
- `sync/`：当前阶段的 Android 首个同步接入层，负责 create room、join room、房间会话控制、heartbeat ack、authority baseline 管理、drift correction、repeated join resync、控制事件出站、消息解码、`seq` 判断、倍速同步、ended-state 应用与播放器状态应用
- `sync/protocol/`：保留与 `INT-19` 协议草案一致的 Android 本地协议模型
- `ui/player/`：播放器核心壳层、页面状态模型、开发联调壳层、Media3 适配器以及页面级同步事件映射入口
- `src/test/.../sync/` 与 `src/test/.../ui/player/`：同步核心与播放器页面边界的最小单元测试

当前实现使用的核心库：

- Player: AndroidX Media3 ExoPlayer
- Network: OkHttp

在 Phase 1 中，这里会继续承载 Android ↔ Android 同步观影 MVP 的主要客户端实现。

## Current App Entry

当前 `MainActivity` 通过 `WatchTogetherApp` 作为应用入口：

1. 先进入 `pages/login/LoginPage`
2. 点击 `登录` 后弹出 `LoginDialog`
3. 输入账号密码后调用 `POST /auth/login`
4. 登录成功后保存当前运行时 `AuthSession`，并进入 `pages/home/HomePage`
5. 点击 `创建放映室` 后，进入 `pages/video/VideoSelectionPage`
6. 在选片页选择影片后，底部固定栏可进入当前播放器放映室页面

这样做是为了让真实业务页面和已经稳定的播放器同步核心并行演进。
后续当个人中心、媒体检索接口与正式放映室页面落地后，会继续把入口推进到完整业务流。

当前登录接入状态：

- `auth/AuthHttpClient.kt` 负责调用 `POST /auth/login`
- `auth/AuthModels.kt` 保存 `AuthSession` 与最小用户信息
- 登录成功后首页优先使用服务端返回的 `nickname`
- 当前 access token 仍为后端约定的 `dev_<userId>` 占位 token
- token 暂时只保存在 Compose 运行时状态中，后续再接正式持久化/session 方案

当前首页接入状态：

- `pages/home/HomeSummaryClient.kt` 负责调用 `GET /home/summary`
- 请求使用登录后保存的 `Authorization: Bearer dev_<userId>` token
- `HomePage` 会用接口返回的 `user.nickname / avatarSeed` 更新欢迎语和头像缩写
- `HomePage` 会用 `lastWatched` 和 `continueWatching` 更新“上次观看”和“继续追番”
- `lastPositionSeconds / durationSeconds` 在 Android 侧展示为秒级 `mm:ss` 进度
- 当前 cover 图仍使用本地渐变占位，后续接入图片加载库后再消费 `coverUrl`
- 首页接口加载失败时保留页面可用，并展示轻量错误提示

## Video Selection Page

`pages/video/VideoSelectionPage.kt` 当前承接 `02A 选择视频` 的 Android UI：

- 顶部搜索框用于按影片标题、剧场版、作品名或制作信息检索
- 默认展示主标签，并通过右侧 `更多` 展开悬浮标签面板
- 悬浮标签面板不挤压后续影片列表布局
- `MediaCatalogClient` 会调用 `GET /media/tags` 获取默认标签和全部标签
- `MediaCatalogClient` 会调用 `GET /media/items` 获取默认片单、搜索结果和标签筛选结果
- `GET /media/items` 返回的 `items[].id` 当前语义是 `media_episodes.id`
- 搜索词或标签变化后，Android 会重新请求媒体列表
- 底部固定栏展示当前选中影片，并提供 `创建房间` 操作
- 当前创建房间入口拿到的是选中 episode id；HTTP 请求体字段仍暂时使用 `mediaItemId`
- 当前 cover 图仍使用本地渐变占位，后续接入图片加载库后再消费 `coverUrl`
- 小屏设备通过收紧 padding、缩短文案和两列卡片约束保持可读性

当前创建房间接入状态：

- 点击 `创建房间` 后，`VideoSelectionPage` 会把选中的 episode id 交给 `WatchTogetherApp`
- `WatchTogetherApp` 进入 `PlayerScreen` 时会传入 `accessToken / currentUserId / selectedEpisodeId`
- `PlayerScreen` 会自动调用 DB-backed `POST /rooms`
- 请求体暂时使用 `{ "mediaItemId": "<selected-episode-id>" }`
- 请求头使用 `Authorization: Bearer dev_<userId>`
- 创建成功后使用响应中的 `room.roomCode` 建立 WebSocket `join_room`
- 创建成功后使用响应中的 `media.title / media.episodeLabel / media.mediaUrl` 更新 `03 放映室`
- 播放器会优先使用 `media.mediaUrl` 载入选中的影片
- 如果后端返回的本地媒体 URL 使用 `127.0.0.1`、`localhost` 或 `0.0.0.0`，Android 会在本地环境中改写为 `MEDIA_BASE_URL` 的 host，例如默认的 `10.0.2.2`

当前放映室详情接入状态：

- 首页 `加入房间` 弹窗输入 6 位房间码后，会进入 `PlayerScreen`
- Android 会先调用 `POST /rooms/{roomCode}/join`，用当前登录用户写入或恢复业务成员关系
- HTTP join 成功后才会连接 WebSocket `/ws` 并发送 `join_room`
- `PlayerScreen` 会在进入或加入房间时调用 `GET /rooms/{roomCode}`
- `GET /rooms/{roomCode}` 返回的业务数据用于 `03 放映室` 首屏展示
- `media.title / media.episodeLabel / media.mediaUrl` 来自 room detail 或 create room response
- WebSocket `room_state` 仍是 `positionMs / playbackRate / paused / ended / seq` 的实时权威
- 加入 WebSocket 前会先尽量加载 room detail，避免收到 `room_state` 时缺少真实 `mediaUrl`
- 如果 room detail 加载失败，会保留同步日志提示，不阻断后续调试入口

当前观看进度上报状态：

- `ProgressHttpClient` 会调用 `PUT /me/media-progress/{mediaItemId}`
- 路径字段名仍叫 `mediaItemId`，但 Android 内部传入的是 episode id
- 上报使用登录后的 `Authorization: Bearer dev_<userId>` token
- 上报单位为秒，不上传毫秒
- 当前低频上报时机包括暂停、播放结束，以及每 30 秒一次的低频后台 tick
- `completed=true` 时会带上 `completionSource=ended`
- 该接口只更新首页 `lastWatched / continueWatching` 所需的业务进度，不参与 WebSocket 实时同步

## Room Player Page

`02A 选择视频` 点击 `创建房间` 后，会进入 `03 放映室` 对应的播放器页面。

这个页面当前由 `pages/room/RoomTheaterPage.kt` 承载，负责贴近 Figma 中的正式放映室结构：

- 顶部展示放映室标题和可复制的 6 位房间码
- 标题下方是尽量占满横向宽度、保持常用比例的视频播放器
- 播放器下方展示影片名称、当前集数和播放进度
- 再下方用轻量房间状态面板展示房主、成员、倍速、在线状态和 `seq`
- 播放、暂停、seek 和倍速选择只出现在播放器点击后的浮层中
- 底部保留折叠的开发联调入口，避免正式页面被调试控件压重

数据边界上，页面业务信息来自服务端主数据接口，例如 `rooms`、`room_members`、`users`、`media_seasons` 和 `media_episodes`。
实时播放状态继续来自 WebSocket 同步链路，例如 `positionMs`、`playbackRate`、`paused`、`ended` 和 `seq`。

当前架构边界：

- `pages/room/RoomTheaterPage.kt`：承载房间码、成员、倍速、同步状态、开发联调入口等 `03 放映室` 业务 UI
- `ui/player/PlayerCoreShell.kt`：只负责播放器视口、媒体标题/进度展示、播放/暂停单按钮、seek 快捷按钮和倍速轻量入口
- `ui/player/PlayerScreen.kt`：继续作为当前同步播放器的装配入口，负责把 room session、sync coordinator、player adapter 和业务页面串起来
- 后续如果要调整 `03 放映室` 的视觉，优先改 `pages/room`
- 后续如果要调整播放器组件能力，优先改 `ui/player`

当前播放器控制层规则：

- 正式 UI 不再展示 `载入 / 同步播放 / 同步暂停` 这类开发态按钮
- 创建或加入房间后，当前媒体由装配层自动载入
- 播放和暂停合并为同一个按钮
- host 点击播放/暂停时，底层映射为同步事件
- viewer 入房后默认跟随房主，不显示成主控交互
- 放映室媒体状态会区分 `等待载入 / 缓冲中 / 正在播放 / 已暂停 / 跟随同步中 / 已结束`，避免把播放器 rebuffer 误显示成同步等待
- 播放器只有进入 `Player.STATE_READY` 后才允许播放/暂停、seek 和倍速操作；`IDLE / BUFFERING / ENDED` 状态下控制层会保持禁用，避免资源未成功加载时仍发出本地播放或同步事件
- ExoPlayer 使用自定义 `DefaultLoadControl`，当前缓冲策略为 `minBuffer=30s / maxBuffer=90s / playbackStart=2.5s / rebufferStart=5s`，优先保障 2 倍速播放时有更充足的可播放缓存
- ExoPlayer 使用 `DefaultTrackSelector` 接入 `master.m3u8` 的 ABR，当前最低视频尺寸约束为 720p，并通过 Logcat `WatchTogetherABR` 输出当前 variant
- 2 倍速播放时会通过 Logcat `WatchTogetherBuffer` 输出低频 buffer debug 日志，包含 `state / pos / buffered / ahead / percent / speed`，用于判断是否真的发生 rebuffer
- 播放器 telemetry 会通过 Logcat `WatchTogetherTelemetry` 输出 rebuffer start/end、rebuffer 次数、累计 rebuffer 时长和 correction 类型计数，用于区分下载不足、解码压力和同步 seek 干预
- drift correction 已升级为 `speed-nudge + seek fallback`：小漂移优先用临时变速追平，只有大漂移才执行硬 seek
- 全屏按钮位于播放/暂停按钮左侧，点击后进入播放器全屏层，再次点击退出
- 播放/暂停使用轻量 icon 按钮，位于浮层控制栏最左侧
- `-10` 和 `+10` 跟随播放/暂停按钮靠左排列
- 倍速默认展示 `倍速 + 当前倍率`，固定在浮层控制栏右侧，点击后从右侧上方展开窄型深色倍率列表
- 当前清晰度使用播放器浮层右上角的轻量 pill 展示，例如 `自动 · 720p`，避免把清晰度信息做成干扰播放的大面板
- 播放、暂停、seek 和倍速入口显示在播放器画面 overlay 上
- 点击播放器画面时显示控制层，无操作一段时间后自动隐藏
- Media3 原生控制器关闭，避免和自定义控制层重复

## Refactor Direction

当前 Android 播放器同步核心已经基本具备，下一步更适合进入“播放器核心与房间业务逻辑分离”的重构阶段。

当前推荐顺序：

1. 先抽离页面状态模型
2. 再抽离房间会话控制器
3. 最后拆分 `PlayerScreen` 为业务页面壳层与播放器核心壳层
4. 再补测试与文档收口

这样可以避免过早直接拆页面 UI，导致状态和副作用只是被分散复制，而没有真正降低耦合。

当前已完成的第一步：

- 已抽离 `RoomPlayerUiState`
- 已把 `SyncStatus` 从 `PlayerScreen.kt` 中抽出
- 已将页面主要运行时状态收拢到统一状态入口

当前已完成的第二步：

- 已新增 `RoomSessionController`
- 已把 create room / join / rejoin / close session / 控制事件出站 收口到统一会话入口
- `PlayerScreen` 不再直接依赖 `RoomHttpClient` 与 `RoomWebSocketClient`

当前已完成的第三步：

- 已把 `03 放映室` 页面壳层放到 `pages/room/RoomTheaterPage`
- 已把播放器核心壳层拆到独立的 `PlayerCoreShell`
- `PlayerScreen` 现在更接近“状态 + 副作用 + 组合入口”
- 原本堆在 `PlayerScreen.kt` 中的大量 UI 区块已迁出到业务页面与播放器组件

当前已完成的第四步：

- 已新增 `RoomPlayerSyncEventHandler`
- 已将页面级同步事件映射与状态更新入口从 `PlayerScreen` 中继续收口
- 已新增 `RoomPlayerDebugShell`
- 已将 `03 放映室` 业务页面移出 `ui/player`，落到 `pages/room/RoomTheaterPage`
- `PlayerScreen` 现在更接近“页面入口 + 状态协调 + 壳层组合”

## Debug Boundary

当前页面中可见的：

- sync log
- player event log
- 最新同步状态面板
- 配置提示区

都主要用于开发联调和问题排查。

后续进入正式产品化 UI 时，建议：

- 仅保留用户真正需要的状态提示
- 把更详细的同步与播放器信息收进日志或单独 debug 入口
- 不把这些调试面板继续当成正式页面结构的一部分

当前这条边界已经开始落实：

- `RoomTheaterPage` 承接正式业务壳层
- `RoomPlayerDebugShell` 承接开发联调面板
