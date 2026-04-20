# Android 播放器使用说明

> 目标：说明当前 Android 播放器相关代码的层级关系、职责边界，以及后续应该从哪里接入或修改。

## 当前分层

当前应用入口已经不再直接从 `PlayerScreen` 开始，而是先经过 `pages/` 下的业务页面层。

当前播放器相关代码可以按 4 层来理解：

1. 业务页面入口层
2. 页面入口层
3. 页面壳层
4. 调试壳层
5. 播放器核心壳层与同步核心

对应文件如下：

- 业务页面入口层：
  [WatchTogetherApp.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/pages/WatchTogetherApp.kt),
  [LoginPage.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/pages/login/LoginPage.kt),
  [LoginDialog.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/pages/login/LoginDialog.kt)
- 页面入口层：
  [PlayerScreen.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/PlayerScreen.kt)
- 页面壳层：
  [RoomPlayerPageShell.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/RoomPlayerPageShell.kt)
- 调试壳层：
  [RoomPlayerDebugShell.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/RoomPlayerDebugShell.kt)
- 播放器核心壳层：
  [PlayerCoreShell.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/PlayerCoreShell.kt)
- 页面状态模型：
  [RoomPlayerUiState.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/RoomPlayerUiState.kt)
- 页面级同步事件映射：
  [RoomPlayerSyncEventHandler.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/RoomPlayerSyncEventHandler.kt)
- 房间会话入口：
  [RoomSessionController.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomSessionController.kt)
- 同步协调：
  [RoomSyncCoordinator.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomSyncCoordinator.kt)
- 播放器核心：
  [PlayerAdapter.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/PlayerAdapter.kt),
  [AndroidExoPlayerAdapter.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/AndroidExoPlayerAdapter.kt)

## 各层职责

### 1. `WatchTogetherApp` / `LoginPage` / `LoginDialog`

这一层是业务页面入口层，负责：

- 当前应用的页面切换入口
- `01 登录页` 的视觉与交互
- 把轻量登录弹窗从页面中抽成独立组件
- 在正式 `02 首页` 尚未落地前，把登录成功后的用户先带入现有播放器页面

这一层不应该直接承担同步算法，也不应该直接操作房间协议细节。

### 2. `PlayerScreen`

`PlayerScreen` 是当前页面的组合入口，主要负责：

- 持有 `RoomPlayerUiState`
- 组装 `RoomSessionController`、`RoomSyncCoordinator`、`PlayerAdapter`
- 绑定 WebSocket listener
- 在本地状态、远端同步状态和播放器之间做副作用协调
- 把最终需要的参数传给页面壳层

这一层应该继续保留：

- `remember`
- `LaunchedEffect`
- `DisposableEffect`
- authority state 应用
- drift correction 调度
- ended 上报

这一层不应该继续堆太多纯 UI 区块。

### 3. `RoomPlayerPageShell`

`RoomPlayerPageShell` 是页面壳层，负责把页面拼出来。

当前包含：

- 顶部状态区
- create/join/rejoin 操作区
- `PlayerCoreShell`
- 最新同步状态面板
- sync log 面板
- player event log 面板
- 配置提示区

如果后面要调整页面布局、信息顺序、调试面板位置，优先改这一层。

需要特别注意的是：

- `Sync log`
- `Player event log`
- 配置提示区

这些都更偏**开发联调能力**，不是正式生产页面的长期组成部分。

当前它们仍然保留在页面壳层里，主要是为了：

- 本地联调
- 同步问题排查
- 开发阶段快速验证 authority state 和播放器事件

后续进入正式产品化页面时，建议把它们收成：

- 仅开发构建可见
- 或单独的 debug 页面 / debug 开关

而不是继续作为常驻 UI 暴露给最终用户。

### 4. `RoomPlayerDebugShell`

`RoomPlayerDebugShell` 是开发联调壳层，负责承接当前这些调试区块：

- `Latest sync state`
- `Sync log`
- `Player event log`
- 配置提示区

这层的目标是把开发联调可视化能力从正式业务页面壳层里分开。

如果后面要：

- 增强调试可观测性
- 把调试面板移到单独 debug 页面
- 按 build variant 隐藏调试内容

优先改这一层。

### 5. `PlayerCoreShell`

`PlayerCoreShell` 是播放器核心壳层，负责播放器本体附近的 UI 组合。

当前包含：

- `PlayerViewport`
- `PlayerControls`

这里主要负责：

- 播放器视口怎么摆
- 控制栏怎么摆
- 倍速按钮怎么摆
- 本地控制和 sync 控制的按钮文案与交互

如果后面要改播放器样式、控制栏布局、按钮排布，优先改这一层。

### 6. `RoomPlayerUiState`

`RoomPlayerUiState` 是页面统一状态入口。

当前集中管理：

- `joinRoomInput`
- `activeUserId`
- `currentRoomId`
- `latestSyncState`
- `syncStatus`
- `player`
- `lastDriftCorrectionAtMs`
- `lastEndedReportedSeq`

如果后面要新增页面级状态，优先判断是否应该加到这里，而不是继续在 `PlayerScreen` 里散落新的 `mutableStateOf`。

### 7. `RoomPlayerSyncEventHandler`

`RoomPlayerSyncEventHandler` 是页面级同步事件映射入口。

当前负责：

- 把 `room_state / play / pause / seek / set_playback_rate / ended / heartbeat / error`
  映射成页面状态更新结果
- 统一 authority state 应用后的 UI 状态更新
- 统一生成页面需要展示的同步日志

这一层的作用是让 `PlayerScreen` 不再直接承载大量事件分支判断。

如果后面要调整：

- 同步事件如何更新页面状态
- 哪些日志要留在页面层
- 页面级同步结果如何组织

优先改这一层。

### 8. `RoomSessionController`

`RoomSessionController` 是房间会话入口层。

当前负责：

- `createRoom`
- `startSession`
- `closeSession`
- `sendPlay`
- `sendPause`
- `sendSeek`
- `sendPlaybackRate`
- `sendEnded`

如果后面要扩房间会话能力，优先从这一层扩，而不是让页面直接调用 `RoomHttpClient` 或 `RoomWebSocketClient`。

### 9. `RoomSyncCoordinator`

`RoomSyncCoordinator` 是同步协调层。

当前负责：

- 应用 `room_state`
- 应用 `play / pause / seek / set_playback_rate / ended`
- drift correction 判断和应用
- authority baseline 到本地播放器状态的映射

如果后面要修改同步算法、权威状态应用、drift correction 规则，优先改这一层。

### 10. `PlayerAdapter` / `AndroidExoPlayerAdapter`

这是播放器核心能力层。

当前负责：

- `load`
- `play`
- `pause`
- `seekTo`
- `setPlaybackSpeed`
- 获取本地播放状态
- 暴露播放器事件

这一层只应该关心“如何播放媒体”，不应该关心房间、host、seq、heartbeat。

## 当前组合关系

当前组合链路可以概括成：

`WatchTogetherApp`
-> `LoginPage`
-> `LoginDialog`
-> `PlayerScreen`
-> `RoomPlayerPageShell`
-> `RoomPlayerDebugShell`
-> `PlayerCoreShell`
-> `PlayerAdapter`

当前真正进入播放器后的链路是：

`PlayerScreen`
-> `RoomPlayerPageShell`
-> `RoomPlayerDebugShell`
-> `PlayerCoreShell`
-> `PlayerAdapter`

更准确地说，现在页面可视层已经分成两条壳层：

`PlayerScreen`
-> `RoomPlayerPageShell`
-> `PlayerCoreShell`
-> `PlayerAdapter`

而房间与同步链路是：

`PlayerScreen`
-> `RoomSessionController`
-> `RoomWebSocketClient / RoomHttpClient`

以及：

`PlayerScreen`
-> `RoomPlayerSyncEventHandler`
-> `RoomPlayerUiState`

以及：

`PlayerScreen`
-> `RoomSyncCoordinator`
-> `PlayerAdapter`

这意味着：

- 业务页面可以先独立推进，而不必先改播放器内部
- 页面壳层不直接做网络和同步算法
- 调试壳层不默认等同于正式页面结构
- 播放器核心壳层不直接做房间业务判断
- 页面入口层负责把“页面状态、房间会话、同步协调、播放器核心”串起来

## 当前使用方式

### 页面如何使用播放器

当前页面直接使用：

```kotlin
PlayerScreen()
```

`PlayerScreen` 会在内部：

- 创建播放器适配器
- 创建房间会话控制器
- 创建同步协调器
- 管理页面状态
- 组合出完整页面

也就是说，当前对外的使用入口仍然是 `PlayerScreen`。

## 调试面板与生产环境边界

当前播放器页面里能看到的：

- `Latest sync state`
- `Sync log`
- `Player event log`
- 配置提示区

本质上都属于**开发环境可观测性工具**。

它们当前存在的目的，是帮助开发阶段快速确认：

- 当前 room_state 是什么
- 最近一次同步事件有没有正确应用
- drift correction 是否在触发
- ended / heartbeat / set_playback_rate 是否正常工作

这不等于正式产品页面必须保留这些面板。

更合理的长期方向是：

- 生产环境页面只保留用户需要理解的少量状态
  例如连接状态、房间身份、必要的错误提示
- 更详细的同步细节写入日志系统
- 调试面板只在开发构建、内部调试模式或专门的 debug 页面中可见

所以后续如果要做正式 UI：

- 不要把 `SyncDebugPanel`、`PlayerEventDebugPanel` 当成正式页面结构的一部分
- 它们应被视为“开发阶段临时可视化工具”

### 如何修改页面布局

如果你只是想：

- 调整顶部信息区
- 调整 join/create 按钮顺序
- 增删状态展示块

优先修改：
[RoomPlayerPageShell.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/RoomPlayerPageShell.kt)

如果是正式产品页面的布局调整，也要顺手评估：

- 哪些状态仍然应该可见
- 哪些调试面板应该移出正式页面

### 如何修改调试面板

如果你想：

- 调整 `Latest sync state`
- 调整 `Sync log`
- 调整 `Player event log`
- 调整配置提示区

优先修改：
[RoomPlayerDebugShell.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/RoomPlayerDebugShell.kt)

### 如何修改播放器样式

如果你想：

- 调整播放器视口比例
- 调整控制区结构
- 调整按钮样式或按钮排布
- 弱化/增强播放器周边视觉样式

优先修改：
[PlayerCoreShell.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/PlayerCoreShell.kt)

### 如何修改同步行为

如果你想：

- 调整 `seq` 判断
- 调整 authority state 应用
- 调整 drift correction 阈值
- 调整 ended-state 处理

优先修改：
[RoomSyncCoordinator.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomSyncCoordinator.kt)

如果你改的是“同步事件如何映射成页面状态和日志”，优先修改：
[RoomPlayerSyncEventHandler.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/RoomPlayerSyncEventHandler.kt)

### 如何修改会话或入房流程

如果你想：

- 改 create room
- 改 join / rejoin
- 改 WebSocket 会话生命周期
- 改控制事件出站逻辑

优先修改：
[RoomSessionController.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomSessionController.kt)

## 修改边界建议

后面继续演进时，尽量遵守这几个边界：

- 不要把大量 UI 区块重新塞回 `PlayerScreen`
- 不要把调试面板重新塞回正式页面壳层
- 不要让页面层重新直接依赖 `RoomHttpClient` / `RoomWebSocketClient`
- 不要把房间业务判断写进 `PlayerAdapter`
- 不要把播放器视口样式调整和同步算法改动混在一个改动里

更简单地说：

- 改布局：优先改壳层
- 改调试 UI：优先改 `RoomPlayerDebugShell`
- 改状态：优先改 `RoomPlayerUiState`
- 改页面级同步映射：优先改 `RoomPlayerSyncEventHandler`
- 改房间流程：优先改 `RoomSessionController`
- 改同步算法：优先改 `RoomSyncCoordinator`
- 改播放器能力：优先改 `PlayerAdapter`

## 后续适合继续拆分的方向

当前重构已经把结构拉开了第一层边界，但后面还可以继续往下收：

- 把 `PlayerCoreShell` 的控制项抽成更小的 UI 组件
- 如果后面 UI 继续演进，再把 `PlayerScreen` 改名成更贴业务语义的 `RoomPlayerScreen`

当前阶段不需要急着继续拆到很细，先保持：

- 页面壳层
- 调试壳层
- 播放器核心壳层
- 页面状态模型
- 页面级同步事件映射入口
- 房间会话入口
- 同步协调层
- 播放器核心层

这 8 个边界稳定即可。
