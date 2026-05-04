# Android 播放器使用说明

> 目标：说明当前 Android 播放器相关代码的层级关系、职责边界，以及后续应该从哪里接入或修改。

## 当前结论

`03 放映室` 是业务页面，不属于播放器组件本身。

当前边界应当这样理解：

- `pages/room/RoomTheaterPage.kt`：`03 放映室` 业务页面，负责房间码、成员、倍速、同步状态、开发联调入口和页面视觉层级
- `ui/player/PlayerCoreShell.kt`：播放器核心 UI 组件，负责播放器视口、媒体标题/进度展示、控制按钮排列
- `ui/player/PlayerScreen.kt`：当前同步播放器装配入口，负责把 room session、sync coordinator、player adapter 和业务页面串起来
- `sync/`：房间会话、WebSocket、协议解码、authority state、drift correction 等同步能力
- `ui/player/PlayerAdapter.kt` 与 `AndroidExoPlayerAdapter.kt`：本地播放器能力，不知道房间业务

这意味着后续如果要改 `03 放映室` 的 UI，优先改 `pages/room`；如果要改播放器组件本身，才改 `ui/player`。

## 当前文件分层

```text
android/app/src/main/java/com/example/watch_together/
├── pages/
│   ├── WatchTogetherApp.kt
│   ├── home/
│   ├── login/
│   ├── room/
│   │   └── RoomTheaterPage.kt
│   └── video/
├── sync/
│   ├── RoomSessionController.kt
│   ├── RoomHttpClient.kt
│   ├── RoomSyncCoordinator.kt
│   ├── RoomSyncState.kt
│   ├── RoomWebSocketClient.kt
│   ├── SyncMessageDecoder.kt
│   └── protocol/
└── ui/
    ├── player/
    │   ├── AndroidExoPlayerAdapter.kt
    │   ├── PlayerAdapter.kt
    │   ├── PlayerCoreShell.kt
    │   ├── PlayerEvent.kt
    │   ├── PlayerScreen.kt
    │   ├── RoomPlayerDebugShell.kt
    │   ├── RoomPlayerSyncEventHandler.kt
    │   └── RoomPlayerUiState.kt
    └── theme/
```

## 各层职责

### 1. `RoomTheaterPage`

[RoomTheaterPage.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/pages/room/RoomTheaterPage.kt) 是 `03 放映室` 的业务页面。

它负责：

- 放映室标题
- 房间码展示与复制
- 影片业务信息展示
- 房主、成员、倍速展示
- 轻量房间同步状态说明
- 开发联调入口的折叠展示
- 将业务页面文案和回调传给播放器组件

它不应该负责：

- WebSocket 消息解析
- drift correction 算法
- ExoPlayer 具体适配
- 直接调用 `RoomHttpClient` 或 `RoomWebSocketClient`

### 2. `PlayerCoreShell`

[PlayerCoreShell.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/PlayerCoreShell.kt) 是播放器核心 UI 组件。

它负责：

- 16:9 播放器视口
- 媒体标题与进度展示
- 播放/暂停单按钮
- seek 快捷按钮
- 倍速轻量入口
- 点击播放器后显示控制 overlay
- 一段时间无操作后自动隐藏控制 overlay
- 接收外部传入的按钮文案和回调

它不应该负责：

- 房间码
- host / viewer 判断
- create / join / rejoin
- `seq`
- heartbeat
- 房间成员展示
- 具体页面背景和放映室视觉氛围
- 媒体自动载入时机

如果一个需求必须让 `PlayerCoreShell` 直接知道 room、host、member、roomCode，通常说明边界走错了，应该先考虑放到 `RoomTheaterPage`。

## 当前视觉原则

`03 放映室` 页面应当优先突出播放器，而不是把同步能力做成按钮面板。

- 播放器保持 16:9，并尽量占满横向空间
- 播放、暂停、seek 和倍速选择属于播放器浮层，不常驻展示在页面卡片中
- 全屏属于播放器组件内部能力，不需要 `RoomTheaterPage` 直接管理
- 全屏按钮位于播放/暂停按钮左侧，点击后进入播放器全屏层，再次点击退出
- 播放/暂停使用轻量 icon 按钮，不使用文字按钮和重色块
- 接入 `master.m3u8` 后由 ExoPlayer 执行 ABR，浮层右上角使用轻量 pill 展示当前 `自动 · 720p / 1080p`
- 浮层控制栏左侧依次放置播放/暂停、`-10`、`+10`
- 浮层控制栏右侧放置倍速入口，点击后从右侧上方展开接近 B 站播放器气质的窄型深色倍速菜单
- 房主、成员、倍速、在线状态和 `seq` 使用轻量状态面板展示
- 开发联调入口默认折叠，避免影响正式页面观感
- 如果后续要补充更多同步说明，优先用日志、toast 或轻量状态，而不是新增大面积控制卡片

### 3. `PlayerScreen`

[PlayerScreen.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/PlayerScreen.kt) 当前仍是同步播放器的装配入口。

它负责：

- 创建并持有 `PlayerAdapter`
- 创建并持有 `RoomSessionController`
- 创建并持有 `RoomSyncCoordinator`
- 持有 `RoomPlayerUiState`
- 绑定 WebSocket listener
- 调度 drift correction
- 上报 ended
- 在进入房间后自动载入当前媒体
- 计算当前按钮应该走本地播放还是同步事件
- 将最终状态和回调传给 `RoomTheaterPage`

它不应该继续承载大块业务 UI。

后续如果继续重构，可以把它改名或迁移为更贴切的 `RoomPlayerScreen` / `RoomPlayerRoute`，但当前先保持入口稳定。

### 4. `RoomPlayerUiState`

[RoomPlayerUiState.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/RoomPlayerUiState.kt) 是当前同步播放器页面状态模型。

当前集中管理：

- `joinRoomInput`
- `activeUserId`
- `currentRoomId`
- `latestSyncState`
- `syncStatus`
- `player`
- `lastDriftCorrectionAtMs`
- `lastEndedReportedSeq`

如果后续新增页面级状态，应先判断它是业务页面状态、同步状态还是播放器状态，不要无脑塞进播放器组件。

### 5. `RoomPlayerSyncEventHandler`

[RoomPlayerSyncEventHandler.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/RoomPlayerSyncEventHandler.kt) 负责把同步事件映射成页面状态更新。

它处理：

- `room_state`
- `play`
- `pause`
- `seek`
- `set_playback_rate`
- `ended`
- `heartbeat`
- `error`

如果要改“收到某个同步事件后页面状态如何变化”，优先改这里。

### 6. `RoomSessionController`

[RoomSessionController.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomSessionController.kt) 是房间会话入口。

它负责：

- `createRoom`
- `startSession`
- `closeSession`
- `sendPlay`
- `sendPause`
- `sendSeek`
- `sendPlaybackRate`
- `sendEnded`

页面不应该直接调用 `RoomHttpClient` 或 `RoomWebSocketClient`。

### 7. `RoomSyncCoordinator`

[RoomSyncCoordinator.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomSyncCoordinator.kt) 是同步协调层。

它负责：

- 应用 authority state
- 应用远端控制事件
- drift correction 判断和执行
- ended-state 应用

当前 drift correction 采用 `speed-nudge + seek fallback`：

- `abs(driftMs) < 150ms`：视为正常误差，不做 correction。
- `150ms <= abs(driftMs) < 2000ms`：临时把播放器倍率调整为 authority 倍率的 `0.97x / 1.03x`，约 `1500ms` 后恢复。
- `abs(driftMs) >= 2000ms`：执行 `seekTo(expectedPositionMs)` 作为兜底。
- 本地 `BUFFERING / ENDED` 或 authority 已暂停、已结束时不会做 correction。

如果要改同步算法、drift 阈值或 authority baseline 外推规则，优先改这里。

### 8. `PlayerAdapter` / `AndroidExoPlayerAdapter`

播放器核心能力层只负责“如何播放媒体”。

它负责：

- `load`
- `play`
- `pause`
- `seekTo`
- `setPlaybackSpeed`
- 获取当前播放状态
- 暴露播放器事件

它不应该知道房间、host、viewer、seq、heartbeat。

当前 `AndroidExoPlayerAdapter` 还负责本地 ABR 播放策略：

- ABR 是 Adaptive Bitrate Streaming，即播放器从 `master.m3u8` 读取多个 variant，并根据网络、缓冲、设备解码能力和播放状态自动选择当前清晰度。
- 当前 track selector 设置最低视频尺寸为 `1280x720`，避免正常体验降到 720p 以下。
- 当前依赖 Media3 HLS 自适应能力在 `720p / 1080p` 之间切换。
- 当 `playbackSpeed >= 2.0x` 或本次播放 rebuffer 次数达到 `2` 次后，播放器会切到 `mobile_fast_720p` 策略，限制 ABR 最高到 `1280x720`，避免在高倍速或设备压力较大时继续升到 1080p。
- 当前 `DefaultLoadControl` 使用 `minBuffer=30s / maxBuffer=90s / playbackStart=3.5s / rebufferStart=10s`，更偏向 VOD 稳定播放。
- `2.0x` 下点击播放前会检查 `bufferedAheadMs >= 12s`，不足时暂缓起播并输出 `high_speed_start_gate` telemetry。
- 当前通过 Logcat `WatchTogetherABR` 输出可用 variant 和当前 variant。
- 当前通过播放器浮层右上角的轻量 pill 展示当前自动清晰度，例如 `自动 · 720p`。

## 当前组合关系

当前进入 `03 放映室` 后的组合链路是：

```text
WatchTogetherApp
-> PlayerScreen
-> RoomTheaterPage
-> PlayerCoreShell
-> PlayerAdapter
```

同步链路是：

```text
PlayerScreen
-> RoomSessionController
-> RoomHttpClient / RoomWebSocketClient
```

房间码加入链路是：

```text
HomePage join dialog
-> WatchTogetherApp 保存 roomCode 并进入 PlayerScreen
-> PlayerScreen 调用 POST /rooms/{roomCode}/join
-> RoomSessionController 启动 WebSocket join_room
-> RoomSyncCoordinator 应用 room_state
```

这条顺序不能跳过 HTTP join。HTTP join 负责确认房间存在、写入或恢复当前用户的业务成员关系，并让服务端准备好后续 WebSocket 运行时房间。

同步状态应用链路是：

```text
PlayerScreen
-> RoomPlayerSyncEventHandler
-> RoomPlayerUiState
```

播放器同步算法链路是：

```text
PlayerScreen
-> RoomSyncCoordinator
-> PlayerAdapter
```

## 如何修改

### 修改 `03 放映室` 页面

优先修改：

[RoomTheaterPage.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/pages/room/RoomTheaterPage.kt)

适合放在这里的改动：

- 房间码位置
- 页面背景
- 房主/成员展示
- 页面底部状态说明
- 开发联调入口
- 业务卡片布局

### 修改播放器组件

优先修改：

[PlayerCoreShell.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/PlayerCoreShell.kt)

适合放在这里的改动：

- 播放器视口比例
- 媒体标题和进度展示组件
- 播放/暂停单按钮
- seek 快捷按钮
- 倍速入口
- 播放器组件内部视觉样式

不适合放在这里的改动：

- 房间码
- 成员列表
- host / viewer 业务判断
- join / create room
- WebSocket 文案
- 媒体自动载入时机

当前播放器控制层规则：

- 正式 UI 不展示 `载入` 按钮
- 创建或加入房间后，由 `PlayerScreen` 自动载入当前媒体
- 播放和暂停合并成同一个按钮，根据当前播放状态切换
- host 点击播放/暂停时，页面装配层映射为同步事件
- viewer 入房后默认跟随房主，播放按钮不可用，避免误导为主控
- 放映室页面层负责把播放器状态映射为展示文案：`IDLE` 是等待载入，`BUFFERING` 是缓冲中，`ENDED` 是已结束，`READY + isPlaying` 是正在播放，`READY + !isPlaying` 再结合 host/viewer 语义显示为已暂停或跟随同步中
- 播放、暂停、seek 和倍速都必须等到底层播放器进入 `Player.STATE_READY` 后才可用；资源未加载、仍在缓冲或已经 ended 时，不应触发本地控制或同步事件
- `AndroidExoPlayerAdapter` 使用自定义 `DefaultLoadControl`，当前缓冲策略为 `minBuffer=30s / maxBuffer=90s / playbackStart=3.5s / rebufferStart=10s`，用于降低 2 倍速播放时短 HLS segment 被快速消耗导致的频繁 rebuffer
- `AndroidExoPlayerAdapter` 使用 `DefaultTrackSelector` 接入 ABR，当前最低视频尺寸约束为 720p，避免 master playlist 中意外出现低清晰度时被常规选择
- `2.0x` 或 rebuffer 过多时会启用 `mobile_fast_720p` 策略，把 ABR 最高限制到 720p
- 当前播放 variant 会通过 Logcat `WatchTogetherABR` 输出，播放器 overlay 右上角也会显示 `自动 · 当前清晰度`
- HLS 播放通过 Media3 `SimpleCache + CacheDataSource + HlsMediaSource` 接入本地缓存，缓存目录为 Android `cacheDir/watch_together_media_cache`，当前上限为 `512MB`
- 本地 HLS cache 主要解决 seek、rejoin、重复进入房间、短时间重播和后续 ahead prefetch 时对已下载 playlist / segment 的重复读取问题；它不是离线下载，也不替代服务端存储或 CDN
- cache 命中、cache ignored 和 HLS cache 载入会通过 Logcat `WatchTogetherCache` 输出；播放器 `release()` 不会删除缓存，缓存由 LRU 策略和系统 cache 目录生命周期管理
- HLS ahead prefetch 基于同一个 `CacheDataSource` 工作：播放器会解析 VOD HLS playlist，根据当前播放位置估算 segment index，并在 1.5x、2.0x、rebuffer 较多或 effective buffer 偏低时提前缓存后续有限 segment
- 当前预取窗口：`2.0x` 预取约 10 个后续 segment，`1.5x` 预取约 5 个后续 segment，rebuffer 较多时预取约 8 个，低 effective buffer 时预取约 3 个；预取不是整集下载
- 预取日志使用 `WatchTogetherPrefetch`，会记录 selected variant、segment prefetch start/done、segment URL 和 cache space before/after，需要和 `WatchTogetherCache / WatchTogetherBuffer / WatchTogetherTelemetry` 一起看
- 进度条支持拖拽 seek preview：拖动中只更新 UI 预览，松手后才提交一次 seek；host 提交同步 seek，viewer 第一版只做本地 seek，不向房间广播
- 2 倍速及以上调试时，`PlayerScreen` 会通过 Logcat `WatchTogetherBuffer` 低频写入 buffer debug 日志，格式包含 `state / pos / buffered / ahead / effectiveAhead / estimatedSegmentsAhead / percent / speed`
- 播放器 telemetry 会通过 Logcat `WatchTogetherTelemetry` 输出 rebuffer start/end、rebuffer 次数、累计 rebuffer 时长和 correction 类型计数
- 高倍速起播门槛使用 `effectiveAheadMs = bufferedAheadMs / playbackSpeed` 和估算 segment 数，而不是只看原始 `bufferedAheadMs`
- 2 倍速下 seek fallback 阈值提高到 `3000ms`，优先让 speed-nudge 吸收中小漂移，减少 codec flush
- drift correction 日志会区分 `speed_nudge / speed_nudge_restore / drift_seek`，用于判断同步层是在温和追平还是发生了硬 seek
- 初始载入的 `BUFFERING` 会标记为 `initial_buffer`，播放开始后的再次 `BUFFERING` 才计为 `rebuffer`
- 全屏按钮位于播放/暂停按钮左侧，并由 `PlayerCoreShell` 内部管理全屏显示/退出
- 倍速默认显示 `倍速 + 当前倍率`，点击后在右侧上方展开窄型深色倍率列表，当前倍率用粉色文字和小点标记
- 播放/暂停、seek、倍速入口显示在播放器画面 overlay 上，排列为左侧全屏、播放/暂停、`-10`、`+10`，右侧倍速
- 点击播放器画面时显示 overlay，连续无操作后自动隐藏
- Media3 原生控制器关闭，避免与自定义控制层重叠

### 修改同步行为

优先修改：

- [RoomSyncCoordinator.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomSyncCoordinator.kt)
- [RoomPlayerSyncEventHandler.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/RoomPlayerSyncEventHandler.kt)

### 修改会话流程

优先修改：

[RoomSessionController.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomSessionController.kt)

## 边界规则

- 不要把 `03 放映室` 的业务 UI 放进 `ui/player`
- 不要让 `PlayerCoreShell` 直接知道 `roomId / roomCode / hostUserId / member`
- 不要让 `RoomTheaterPage` 直接处理 WebSocket 细节
- 不要让页面直接依赖 `RoomHttpClient / RoomWebSocketClient`
- 不要把调试面板当成正式页面结构
- 不要把播放器视口样式调整和同步算法改动混在一起

这条边界比目录名字更重要：播放器组件负责“播放”，放映室页面负责“业务语境”，同步层负责“房间状态一致”。 
