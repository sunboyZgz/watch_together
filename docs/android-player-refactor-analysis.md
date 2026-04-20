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

另外，当前 Android 应用已经开始从“只有播放器页”过渡到“真实业务页面入口 + 播放器核心”的结构。
第一张正式业务页已经落地为：

- `pages/WatchTogetherApp`
- `pages/login/LoginPage`
- `pages/login/LoginDialog`

这意味着后续 `02 首页`、`02A 选择视频` 等真实业务页面，会优先在 `pages/` 层继续扩展，而不是继续堆进播放器内部。

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

当前这一步已经完成了第一轮落地：

- 已新增轻量 `RoomSessionController`
- 已把 `PlayerScreen` 中对 `RoomHttpClient` 和 `RoomWebSocketClient` 的直接依赖收敛到 controller
- 已把 create room、join/rejoin、会话关闭以及控制事件出站统一收口到会话控制器入口

当前仍然保留：

- 页面层对 `RoomSyncCoordinator` 的直接使用
- 页面层对 listener 具体内容的组织

这符合当前阶段“先把会话入口抽离，再继续做页面壳层拆分”的目标。

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

当前这一步已经完成了第一轮落地：

- 已把页面壳层拆到独立的 `RoomPlayerPageShell`
- 已把播放器核心壳层拆到独立的 `PlayerCoreShell`
- `PlayerScreen` 现在更接近“状态 + 副作用 + 组合入口”
- 原本堆在 `PlayerScreen.kt` 中的大量 UI 区块已经移出到独立文件

## 当前实际文件落点

经过前三步重构后，当前 Android 播放器相关文件已经可以按下面的方式理解：

```text
android/app/src/main/java/com/example/watch_together/
├── config/
│   └── AppConfig.kt
├── sync/
│   ├── RoomHttpClient.kt
│   ├── RoomSessionController.kt
│   ├── RoomSyncCoordinator.kt
│   ├── RoomSyncState.kt
│   ├── RoomWebSocketClient.kt
│   ├── SyncMessageDecoder.kt
│   └── protocol/
└── ui/player/
    ├── AndroidExoPlayerAdapter.kt
    ├── PlayerAdapter.kt
    ├── PlayerCoreShell.kt
    ├── PlayerEvent.kt
    ├── PlayerScreen.kt
    ├── RoomPlayerPageShell.kt
    └── RoomPlayerUiState.kt
```

其中：

- `PlayerScreen.kt`：页面组合入口，保留状态、副作用与页面装配
- `RoomPlayerPageShell.kt`：页面壳层，负责房间页面布局与调试区块组合
- `PlayerCoreShell.kt`：播放器核心壳层，负责视口与控制区的布局
- `RoomPlayerUiState.kt`：页面状态统一入口
- `RoomSessionController.kt`：房间会话入口
- `RoomSyncCoordinator.kt`：同步协调入口
- `AndroidExoPlayerAdapter.kt`：本地播放器核心适配层

这一步带来的直接收益：

- 页面壳层和播放器核心壳层有了更清晰的文件边界
- 后续如果继续改页面区块顺序、信息层级、调试面板位置，更多是在壳层文件里调整
- `PlayerScreen` 不再同时承担全部 UI 区块定义

## 推荐的第二阶段重构

当前已经可以进入第二阶段重构。

这里的判断依据不是“第一阶段已经被完全验证完”，而是：

- 当前页面状态入口已经存在
- 当前房间会话入口已经存在
- 页面壳层与播放器核心壳层已经拆开
- 当前代码已经具备继续拆薄 `PlayerScreen` 的基本边界

因此第二阶段的目标，不再是“先证明第一阶段绝对稳定”，而是继续把目前还残留在页面入口层的职责往外收，让后续业务开发时更容易定位问题、调整页面和替换实现。

第二阶段建议只聚焦两条主线：

1. 继续把 `PlayerScreen` 收薄，只保留更明确的页面入口职责
2. 把“开发联调 UI”和“正式业务 UI”进一步分开

不要在这一阶段同时做：

- UI 视觉重做
- 协议扩展
- 播放器内核替换
- Windows 端抽象统一

### 1. 继续抽薄 `PlayerScreen`

当前 `PlayerScreen.kt` 虽然已经不再承担全部 UI 区块定义，但仍然保留了较多页面级 listener 映射和副作用组织。

第二阶段更合理的方向是继续把下面这些东西往外收：

- `RoomWebSocketListener` 到页面状态的映射
- authority state 应用后的页面级状态更新
- 同步事件和页面状态之间的 reducer / translator
- ended 上报与 drift correction 的页面装配代码

目标不是把所有副作用都消灭，而是让 `PlayerScreen` 更接近：

- 创建依赖
- 持有顶层页面状态
- 组织页面组合

而不是继续承载大量“具体事件如何转换成页面状态”的细节。

当前这一步已经完成了第一轮落地：

- 已新增页面级同步事件映射入口 `RoomPlayerSyncEventHandler`
- 已把 `room_state / play / pause / seek / set_playback_rate / ended / heartbeat / error`
  到页面状态更新的映射从 `PlayerScreen` 中继续收口
- `PlayerScreen` 中的大量 listener 分支判断已明显减少
- `PlayerScreen` 现在更接近“页面入口 + 状态协调 + 壳层组合”

### 2. 拆出开发联调 UI 和正式业务 UI 的边界

当前 `RoomPlayerPageShell` 中仍然包含：

- `Latest sync state`
- `Sync log`
- `Player event log`
- 配置提示区

这些内容对开发联调很有价值，但不应继续和正式业务页面长期绑定在一起。

第二阶段应该开始把这两类页面意图区分开：

- 开发联调壳层
- 正式业务壳层

这不要求立刻把正式 UI 做完，但至少要把结构准备成：

- 调试区块可以整体拿掉
- 不影响播放器核心壳层
- 不影响会话与同步核心

当前这一步已经完成了第一轮落地：

- 已新增开发联调壳层 `RoomPlayerDebugShell`
- `Latest sync state`、`Sync log`、`Player event log`、配置提示区
  已从 `RoomPlayerPageShell` 中拆出
- `RoomPlayerPageShell` 更接近正式业务壳层
- 当前结构已经支持“业务壳层”和“调试壳层”分开演进

### 3. 为后续业务开发创造稳定入口

第二阶段完成后，后续业务开发更理想的状态应该是：

- 改正式页面时，更多改页面壳层
- 改同步算法时，更多改 `RoomSyncCoordinator`
- 改会话流程时，更多改 `RoomSessionController`
- 调试面板可以单独演进，不继续污染正式页面结构

## 推荐的第二阶段任务拆分

如果把第二阶段真正落成任务，建议拆成下面几项：

1. 抽离页面级同步事件映射与状态更新入口
2. 拆分开发联调壳层与正式业务壳层
3. 为第二阶段边界补最小测试与文档

这三项已经足够支撑第二阶段，不建议再一口气拆得更碎。

当前这一步也已经完成了第一轮落地：

- 已新增页面级同步事件映射相关最小单测
- 已同步更新 Android 目录说明
- 已同步更新播放器使用说明
- 已同步更新重构分析文档

## 第二阶段完成后的预期收益

如果第二阶段完成，直接收益会是：

- `PlayerScreen` 继续变薄，更接近真正的页面入口
- 页面层和同步层之间的映射关系更清晰
- 开发调试 UI 不再默认等于正式页面结构
- 后面做 create room / join room / room player 正式页面时，可以复用现有核心层，而不是继续背着当前调试页面整体前进

## 这一阶段之后再考虑的事情

第二阶段之后，再考虑下面这些会更自然：

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
