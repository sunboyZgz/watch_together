# Android 播放器重构分析

> 目标：把 Android 播放器整理成“播放器核心能力”和“房间业务 / 同步逻辑”分离的结构，为后续维护、测试和 UI 演进打基础。

## 背景

当前 Android 侧已经完成了播放器同步主链路的核心能力：

- `join` 后应用 `room_state`
- `play / pause / seek` 同步
- `set_playback_rate` 倍速同步
- repeated join / resync
- heartbeat
- host transfer 后的继续控制
- drift correction
- ended-state authority semantics

这些能力说明“同步核心是否成立”这个问题，当前已经基本解决。

接下来更需要处理的问题变成了：

- 播放器页面是否承担了过多职责
- 同步核心是否和业务页面耦合过深
- 后续做 UI、调试、测试、替换播放器时，是否需要反复穿透同一个大页面文件

因此现在适合进入播放器重构阶段。

## 当前问题

### 1. `PlayerScreen.kt` 职责过重

当前 [PlayerScreen.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/ui/player/PlayerScreen.kt) 同时承担了：

- 页面布局
- 房间 create / join 交互
- WebSocket listener 绑定
- authority state 应用
- drift correction 调度
- ended 上报
- sync log / player log 维护
- host / viewer 业务语义

这会带来几个直接问题：

- 文件过大，阅读和修改成本高
- UI 调整时容易误碰同步逻辑
- 同步逻辑问题排查时容易被页面细节干扰
- 后续如果要新增正式的 create room / join room 页面，会重复搬运状态和逻辑

### 2. “播放器能力”和“房间业务”耦合在同一层

当前播放器真正的核心能力其实已经比较清楚：

- `PlayerAdapter`
- `AndroidExoPlayerAdapter`
- `PlayerEvent`

但这些能力在页面里被直接和业务语义混合使用，例如：

- host 点击按钮后既更新 UI，又操作播放器，又发同步事件
- 收到远端事件后，页面层直接决定如何更新播放器和业务状态

这导致播放器层很难被当成一个“可复用核心组件”。

### 3. 同步核心已经存在，但边界还不够清楚

当前已经有一层较清晰的同步核心：

- [RoomSyncCoordinator.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomSyncCoordinator.kt)
- [RoomSyncState.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomSyncState.kt)
- [RoomWebSocketClient.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomWebSocketClient.kt)
- [SyncMessageDecoder.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/SyncMessageDecoder.kt)
- [RoomHttpClient.kt](/Users/sunboy/Documents/my-projects/watch_together/android/app/src/main/java/com/example/watch_together/sync/RoomHttpClient.kt)

但这层和页面层之间还缺少一个更明确的“房间会话 / 播放器会话”边界，导致：

- 页面直接感知过多同步细节
- UI 状态和同步状态纠缠
- 业务状态和 authority state 没有被明确区分

## 结论

从当前项目状态看，**和 Android 播放器直接相关的同步核心能力已经基本齐备，可以开始抽离播放器逻辑。**

也就是说，下一步不再是继续往 `PlayerScreen.kt` 上堆同步能力，而是要把现有能力拆成更清晰的层次。

## 推荐拆分目标

建议把 Android 播放器相关代码拆成 4 层。

### 1. 播放器核心层

职责：

- 本地媒体加载与播放控制
- 对外暴露统一播放器接口
- 对外暴露播放器事件

典型内容：

- `PlayerAdapter`
- `AndroidExoPlayerAdapter`
- `PlayerEvent`

这一层不应该知道：

- `roomId`
- `host / viewer`
- `seq`
- `heartbeat`
- `join_room`

它只应该知道“如何播放一段媒体”。

### 2. 同步协调层

职责：

- authority baseline
- `seq` 新旧判断
- 远端控制事件应用
- drift correction
- ended-state 应用

典型内容：

- `RoomSyncCoordinator`
- `RoomSyncState`

这一层可以操作播放器，但不应该关心页面长什么样，也不应该直接处理 create/join 按钮点击。

### 3. 房间会话层

职责：

- `POST /rooms`
- `/ws`
- join / repeated join / reconnect
- heartbeat ack
- 消息收发与 listener 生命周期

典型内容：

- `RoomHttpClient`
- `RoomWebSocketClient`
- `SyncMessageDecoder`

这一层负责“与房间服务说话”，但不应该直接决定 UI 怎么显示。

### 4. 页面业务层

职责：

- host / viewer 操作入口
- 页面状态展示
- debug panel
- create/join 输入
- 业务按钮和布局

典型内容：

- `PlayerScreen`
- 后续的 create room / join room / room player screen 正式页面

这一层负责用户如何和系统交互，但不应该再承载大量同步算法细节。

## 推荐的目录方向

当前不一定要一次大搬家，但目录目标建议逐步靠近下面这种结构：

```text
android/app/src/main/java/com/example/watch_together/
├── player/
│   ├── core/
│   │   ├── PlayerAdapter.kt
│   │   ├── AndroidExoPlayerAdapter.kt
│   │   └── PlayerEvent.kt
│   ├── sync/
│   │   ├── RoomSyncCoordinator.kt
│   │   ├── RoomSyncState.kt
│   │   └── protocol/
│   └── session/
│       ├── RoomHttpClient.kt
│       ├── RoomWebSocketClient.kt
│       └── SyncMessageDecoder.kt
├── feature/
│   └── roomplayer/
│       ├── RoomPlayerScreen.kt
│       ├── RoomPlayerState.kt
│       └── RoomPlayerActions.kt
├── config/
└── MainActivity.kt
```

当前阶段不要求立刻完全迁到这个目录，但它可以作为重构方向参考。

## 推荐的第一阶段重构

第一阶段建议只做“边界收拢”，不要同时做 UI 改版。

当前更合理的执行顺序是：

1. 先抽页面状态模型
2. 再抽房间会话控制器
3. 最后再拆 `PlayerScreen` 为页面壳层与播放器核心壳层

原因：

- 如果先直接拆 `PlayerScreen`，页面里的状态和副作用仍然会一起搬过去，只是把大文件拆成多个小文件，并没有真正降低耦合
- 先把状态和会话流程收口，页面壳层的拆分才有稳定边界可依赖
- 这样后面的 UI 组件化才更像“换壳”，而不是把同步和业务逻辑一起复制到多个组件中

### 第一步：抽出页面状态容器

从 `PlayerScreen.kt` 中抽出一个页面状态模型，至少把这些状态先集中：

- 当前房间 ID
- 当前用户 ID
- host / viewer 身份
- sync status
- latest sync state
- logs

目标：

- 页面 UI 不再直接 scattered 地管理大量 `mutableStateOf`
- 页面状态有一个统一入口

当前这一步已经完成了第一轮落地：

- 已新增独立页面状态模型文件
- 已把 `SyncStatus` 从 `PlayerScreen.kt` 中抽出
- 已把播放器页面的主要运行时状态收拢到统一的 `RoomPlayerUiState`

当前仍暂时保留独立的 debug log 列表，没有在这一步一起引入更重的页面状态管理模式。

### 第二步：抽出房间会话控制器

把下面这类事情从页面里收出去：

- create room
- join as host
- join as viewer
- rejoin current user
- WebSocket listener 绑定和解绑
- heartbeat / ended / set_playback_rate 出站

可以先形成一个最小的 `RoomSessionController` 或类似对象。

目标：

- 页面只发“我要 create/join/rejoin”
- 控制器负责和 `RoomHttpClient + RoomWebSocketClient + RoomSyncCoordinator` 交互

### 第三步：页面只保留 UI 映射

重构完成后，页面层应尽量只做：

- 展示当前状态
- 把用户操作转成 controller action
- 显示日志和调试信息

而不是自己决定：

- 什么时候 `seekTo`
- 什么时候 `play`
- 什么时候发 `ended`
- 什么时候做 drift correction

对应任务顺序建议：

- `INT-93`：抽离房间播放器页面状态模型
- `INT-94`：抽离房间会话控制器
- `INT-95`：拆分 `PlayerScreen` 为页面壳层与播放器核心壳层
- `INT-96`：为重构后的播放器边界补测试与文档

## 推荐的第二阶段重构

在第一阶段边界稳定后，再做更细的拆分。

### 1. 拆出 Room Player Feature

把现在的 `PlayerScreen.kt` 继续拆成：

- `RoomPlayerScreen`
- `PlayerViewport`
- `PlaybackControls`
- `SyncStatusPanel`
- `JoinActionsPanel`
- `DebugLogPanel`

### 2. 让播放器核心更可替换

后续如果想做：

- 本地媒体播放测试
- mock player
- Windows 侧统一抽象参考

则播放器核心层应尽量只暴露稳定接口，不让页面直接依赖 Media3 细节。

### 3. 为测试创造稳定入口

当前很多逻辑测试虽然已经有了，但页面层仍然过大，不利于继续加测试。

重构后更容易补的测试类型包括：

- 房间会话控制器单测
- 页面状态迁移测试
- UI 不同业务状态下的渲染测试

## 这次重构暂时不要做什么

为了避免一次重构过重，下面这些建议暂时不要混进来：

- 不要同时改 UI 设计
- 不要同时改协议
- 不要同时重写 drift correction 算法
- 不要同时替换 ExoPlayer 适配层
- 不要同时为 Windows 端抽共享 Kotlin 模块

重构的重点是：**先把边界梳理清楚，而不是借机全面改造。**

## 当前最值得保留的稳定边界

下面这些代码当前已经比较接近稳定核心，建议尽量保留对外语义，只做组织层面的重构：

- `PlayerAdapter`
- `PlayerEvent`
- `RoomSyncState`
- `RoomSyncCoordinator`
- `RoomWebSocketClient`
- `SyncMessageDecoder`

真正应该首先被拆薄的，是页面层和页面里混合的业务流程。

## 推荐后续任务

如果要把这份分析真正落成任务，最适合拆成下面几个方向：

1. 抽离 Android 房间播放器页面状态模型
2. 抽离 Android 房间会话控制器
3. 将 `PlayerScreen` 拆成业务页面层与播放器核心层
4. 为抽离后的房间会话与页面状态补测试

## 组件化后的样式自由度与边界

如果这次重构做得足够干净，Android 播放器后续在“样式调整”和“页面组合”上的体验，会更接近前端里 Vue / React 的组件化开发方式。

但这里要先明确一个前提：

- **可自由调整样式的，主要是播放器外层的布局壳与业务页面层**
- **不是把底层播放器本身变成一个完全无约束的纯 Compose 组件**

### 重构后可以获得的自由度

当播放器被拆成“核心能力 + 外层布局壳”后，后续比较容易自由调整的部分包括：

- 播放器区域在页面里的层级结构
- 播放器 viewport 是否满宽
- 播放器容器的比例与外边距
- 控制栏的摆放方式
- 房间状态区、同步状态区、调试信息区的位置
- 不同页面对同一个播放器核心的不同包装方式

这意味着后面可以形成一种更接近前端组件组合的开发方式：

- 同一个 `PlayerCore`
- 外面套不同的 `PlayerLayout`
- 再由不同页面决定：
  - 是极简播放器页
  - 还是带房间信息的观影页
  - 还是带调试面板的开发联调页

### 为什么不能完全等同于 Vue / React 组件

Android 播放器底层仍然依赖：

- `PlayerView`
- `AndroidView`
- Media3 / ExoPlayer 的原生播放容器

所以它和纯声明式前端组件有一个现实差异：

- 外层布局和业务组合可以非常灵活
- 但底层播放器渲染容器本身仍然是一个原生 view

这意味着：

- 可以高度自由地调整“外层壳”
- 但不适合把最底层播放器视图拆成完全任意的 Compose 原子片段

### 最适合追求的目标

这次重构后，最值得追求的不是“播放器的一切都能随意重排”，而是下面这个目标：

1. 播放器核心能力稳定独立
2. 同步核心逻辑不依赖具体页面布局
3. 页面层可以自由调整样式、区块顺序和信息层级
4. UI 调整不需要反复碰播放与同步核心

如果做到这一点，后面调样式时，绝大部分时候你改的是：

- 布局结构
- 外层容器
- 控制区位置
- 状态区样式

而不是去反复改：

- authority state
- drift correction
- ended-state semantics
- 播放器事件与同步事件的映射

### 当前最合理的边界

因此，后续最推荐维持的边界是：

- `PlayerCore`
  只负责本地播放与播放事件

- `SyncCore`
  只负责 authority state、seq、drift correction、ended、远端事件应用

- `RoomSession`
  只负责 create/join/ws/rejoin/heartbeat

- `Feature UI`
  只负责页面样式、信息层级、按钮布局和业务交互入口

这样做之后，Android 播放器虽然不会完全等同于 Vue / React 的纯前端组件，但已经足够实现“通过组件层级自由调整页面外观，而不破坏播放和同步核心”的目标。

## 当前结论

当前阶段最合理的判断是：

- **同步核心：基本完成**
- **业务页面：仍然偏重**
- **下一步重点：不是继续堆同步功能，而是开始做播放器与业务逻辑分离**

因此，播放器重构已经具备启动条件，而且现在做这件事比继续把更多业务逻辑堆进 `PlayerScreen.kt` 更划算。
