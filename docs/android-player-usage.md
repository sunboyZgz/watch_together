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
