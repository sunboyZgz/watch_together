# Android

`android/` 用于放置 Android 客户端工程。

当前已经落地的内容包括：

- `pages/` 目录下的真实业务页面入口
- `01 登录页` 以及抽离后的轻量登录弹窗组件
- `02 首页与加入房间` 的真实业务页面
- `02A 选择视频` 的真实业务页面与本地交互骨架
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
  当前也已经开始拆分为页面入口、页面壳层、调试壳层与播放器核心壳层
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
  当前已包含 `WatchTogetherApp`、`LoginPage`、`LoginDialog`、`HomePage` 与 `VideoSelectionPage`
- `config/`：统一读取 `BuildConfig` 并生成 Android 端可直接使用的 URL
- `sync/`：当前阶段的 Android 首个同步接入层，负责 create room、join room、房间会话控制、heartbeat ack、authority baseline 管理、drift correction、repeated join resync、控制事件出站、消息解码、`seq` 判断、倍速同步、ended-state 应用与播放器状态应用
- `sync/protocol/`：保留与 `INT-19` 协议草案一致的 Android 本地协议模型
- `ui/player/`：播放器页面入口、页面状态模型、页面壳层、开发联调壳层、Media3 适配器以及页面级同步事件映射入口
- `src/test/.../sync/` 与 `src/test/.../ui/player/`：同步核心与播放器页面边界的最小单元测试

当前实现使用的核心库：

- Player: AndroidX Media3 ExoPlayer
- Network: OkHttp

在 Phase 1 中，这里会继续承载 Android ↔ Android 同步观影 MVP 的主要客户端实现。

## Current App Entry

当前 `MainActivity` 通过 `WatchTogetherApp` 作为应用入口：

1. 先进入 `pages/login/LoginPage`
2. 点击 `登录` 后弹出 `LoginDialog`
3. 当前阶段确认账号后，进入 `pages/home/HomePage`
4. 点击 `创建放映室` 后，进入 `pages/video/VideoSelectionPage`
5. 在选片页选择影片后，底部固定栏可进入当前播放器放映室页面

这样做是为了让真实业务页面和已经稳定的播放器同步核心并行演进。
后续当个人中心、媒体检索接口与正式放映室页面落地后，会继续把入口推进到完整业务流。

## Video Selection Page

`pages/video/VideoSelectionPage.kt` 当前承接 `02A 选择视频` 的 Android UI：

- 顶部搜索框用于按影片标题、剧场版、作品名或制作信息检索
- 默认展示主标签，并通过右侧 `更多` 展开悬浮标签面板
- 悬浮标签面板不挤压后续影片列表布局
- 影片列表当前使用本地样例数据，后续会替换为 `INT-111` 的服务端检索接口
- 底部固定栏展示当前选中影片，并提供 `创建房间` 操作
- 小屏设备通过收紧 padding、缩短文案和两列卡片约束保持可读性

## Refactor Direction

当前 Android 播放器同步核心已经基本具备，下一步更适合进入“播放器核心与房间业务逻辑分离”的重构阶段。

当前推荐顺序：

1. 先抽离页面状态模型
2. 再抽离房间会话控制器
3. 最后拆分 `PlayerScreen` 为页面壳层与播放器核心壳层
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

- 已把页面壳层拆到独立的 `RoomPlayerPageShell`
- 已把播放器核心壳层拆到独立的 `PlayerCoreShell`
- `PlayerScreen` 现在更接近“状态 + 副作用 + 组合入口”
- 原本堆在 `PlayerScreen.kt` 中的大量 UI 区块已迁出为独立文件

当前已完成的第四步：

- 已新增 `RoomPlayerSyncEventHandler`
- 已将页面级同步事件映射与状态更新入口从 `PlayerScreen` 中继续收口
- 已新增 `RoomPlayerDebugShell`
- 已将开发联调面板从 `RoomPlayerPageShell` 中拆出
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

- `RoomPlayerPageShell` 更偏正式业务壳层
- `RoomPlayerDebugShell` 承接开发联调面板
