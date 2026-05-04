# Core Sync Technical Notes

`watch_together` 的核心价值不只是“能发出同步指令”，而是“在实际播放中长期保持可接受的同步体验”。

这份文档用于记录当前核心功能里的关键技术点，方便后续做：

- 性能优化
- 参数调优
- 问题排查
- 策略替换

当前重点聚焦 Phase 1 的 Android ↔ Android 同步主链路。

## Why This Matters

当前核心业务链路已经包括：

- create room
- join room
- room_state
- play / pause / seek
- playback rate sync
- ended-state authority
- heartbeat
- host transfer

这些能力解决了“能否进入同一房间并执行同一控制操作”。

但真正决定同步观感的是下面这些技术点：

- 服务端是否能及时识别静默断线
- 权威状态是否稳定
- 客户端是否会因为事件应用顺序产生回环
- 两端是否会随着播放逐步产生 drift

所以这些模块都属于核心功能中的核心技术点，而不是边缘优化项。

## Core Technical Points

### 1. Authority State

当前同步模型以服务端房间状态为权威来源。

关键点：

- 服务端维护 `room_state`
- 服务端维护 authority baseline 的服务端时间基线
- 服务端推进 `seq`
- 客户端只应用新于本地已知状态的事件
- 房主控制事件最终都要回到服务端确认和广播

当前服务端除了保存协议层可见的：

- `positionMs`
- `paused`
- `playbackRate`
- `seq`

还需要在运行时保存一个内部时间基线：

- `authorityUpdatedAtMs`

当房间播放完成后，权威状态还需要显式保存：

- `ended`

它不需要直接暴露进协议，但它决定了服务端在任意时刻如何推导“当前有效播放位置”。

#### Server-side Effective Position

这个时间基线最直接的用途，是在 `join_room`、host transfer 这类需要返回 `room_state` 的时刻，给出“此刻房间真正应该播放到哪里”，而不是“最后一次控制事件发生时播放到哪里”。

服务端算法思路：

1. 维护冻结基线
   每次收到权威控制事件时，更新：
   - `positionMs`
   - `paused`
   - `playbackRate`
   - `authorityUpdatedAtMs = serverNow`

2. 对外生成当前有效状态
   当需要返回 `room_state` 时：
   - 如果 `paused == true`
     - `effectivePositionMs = positionMs`
   - 如果 `paused == false`
     - `elapsedMs = max(0, nowMs - authorityUpdatedAtMs)`
     - `progressedMs = elapsedMs * playbackRate`
     - `effectivePositionMs = positionMs + progressedMs`

3. 作为新的 authority snapshot 返回给客户端
   这样 repeated join、host transfer 后收到的 `room_state` 就能代表“当前时刻的房间位置”，而不是过期快照。

#### Completed State Semantics

播放结束不是普通的暂停，它需要一个稳定的 completed state。

当前更合理的 authority 语义是：

- `ended == true`
- `paused == true`
- `positionMs` 冻结在完成位置
- `playbackRate` 保留当前权威倍率
- `seq` 因 ended 事件推进一次

服务端算法思路：

1. host 上报 `ended`
   - 服务端先按当前 authority timeline 结算出“此刻真实有效位置”
   - 再与 host 上报的 `positionMs` 取一个更接近结尾的冻结位置

2. 把房间收敛到 completed state
   - `ended = true`
   - `paused = true`
   - `positionMs = frozenPositionMs`
   - `authorityUpdatedAtMs = serverNow`
   - `seq++`

3. 对外广播 `ended`
   - 让其他成员停止继续播放
   - 让 repeated join / reconnect 后拿到稳定终态

4. seek 离开结尾时清除 ended
   - 一旦 host 主动 seek 到非结尾位置，completed state 不应继续保留
   - 这时房间回到正常的播放/暂停 authority 语义

优化方向：

- 保持 `seq` 语义稳定
- 避免房间状态与广播事件不一致
- 减少 stale event 误应用

### 2. Event Ordering

当前最小协议里，`room_state / play / pause / seek` 都依赖顺序正确应用。

关键点：

- 客户端收到旧 `seq` 事件时应忽略
- join 初始同步后才可把后续控制事件当作增量应用
- correction 不应再次制造新的控制事件

优化方向：

- 更明确地区分 authority baseline 与 local transient state
- 减少远端事件刚应用完又被本地逻辑覆盖的情况

### 3. Heartbeat And Liveness

heartbeat 是当前连接健康判断的基础。

关键点：

- 服务端周期性发送 `heartbeat`
- 客户端立即返回 `heartbeat_ack`
- 超时连接会走现有 disconnect cleanup
- host transfer、reconnect status、room lifecycle 都依赖这层 liveness 判断

当前默认参数：

- heartbeat interval: `5s`
- heartbeat timeout: `15s`

后续优化方向：

- 是否需要区分 debug / release 参数
- timeout 是否过长或过短
- 心跳日志是否需要收敛
- 当房间只剩 host 自己时，如何向页面层提供明确提示

### 4. Room Lifecycle

房间生命周期不只是“能不能创建房间”，而是要明确房间什么时候继续保留、什么时候应该清理。

当前阶段更合理的生命周期规则已经收敛为：

- 只要房间还没有被销毁，就允许成员继续加入
- 不再单独维护“创建后无人加入 5 分钟自动销毁”的独立规则
- 当最后一个成员离开后，房间进入一个短暂 grace period
- 若 grace period 内有人重新加入，房间继续保留
- 若 grace period 到期仍为空房间，则服务端自动销毁

当前规则：

- empty-room grace period: `2m`

为什么这样更合适：

- 它把 `INT-51` 和 `INT-52` 收敛成一条更简单的规则
- 它与 heartbeat 超时后的静默断线清理更容易协同
- 它为 reconnect/rejoin 提供了短暂恢复窗口
- 它避免房间在最后一个人刚离开时被过早销毁

后续优化方向：

- grace period 是否需要区分 debug / release
- 房间清理事件是否需要对客户端可见
- 房间销毁和播放完成态是否要联动

### 5. Host Transfer

当前已实现 host 断线后的即时 host transfer。

关键点：

- host 断开后，从剩余成员中选出新 host
- 推进 `seq`
- 广播新的 `room_state`
- 新 host 后续继续发 `play / pause / seek`

后续优化方向：

- former host reconnect 后的角色处理
- repeated join / resync 后的身份稳定性
- host transfer 与 heartbeat timeout 的联动验证
- host transfer 后如何给新 host 提供明确提示

### 6. Drift Correction

drift correction 是当前最值得持续跟踪的核心技术点之一。

它解决的问题不是“能否同步发出控制事件”，而是：

- 两端在持续播放中是否会越播越偏
- 本地播放器是否能收敛到权威时间线附近

当前建议的最小实现方向：

- 仅在 `paused == false` 时检查 drift
- 客户端持有最近一次 authority baseline
- 周期性比较：
  - 本地播放器位置
  - 根据 authority baseline 外推出来的 expected position
- 轻微偏差使用 speed nudge 温和追平
- 严重偏差才 fallback 到本地 correction seek
- correction 只影响本地播放器，不回传 sync 事件

当前实现里的 authority baseline 至少包含：

- `positionMs`
- `paused`
- `playbackRate`
- `seq`
- `authorityAppliedAtMs`

当前建议的初始参数：

- correction interval: `1s`
- drift dead zone: `< 150ms`
- speed nudge range: `150ms <= abs(driftMs) < 2000ms`
- seek fallback threshold: `abs(driftMs) >= 2000ms`
- speed nudge delta: `authority playbackRate * 0.97 / 1.03`
- speed nudge duration: `1500ms`
- local ended guard: enabled
- local buffering guard: enabled

#### Current Algorithm

当前阶段采用的是“基于本地 wall-clock 的 baseline 外推”。

算法分 3 步：

1. 记录 authority baseline  
   当客户端应用 `room_state / play / pause / seek` 时，保存：
   - 当时的权威位置 `authority.positionMs`
   - 当时的暂停状态 `authority.paused`
   - 当时的倍速 `authority.playbackRate`
   - 本地应用该权威状态的时刻 `authorityAppliedAtMs`

2. 推导 expected position  
   每次做 drift check 时，根据当前时间 `nowMs` 估算“此刻理论上应该播放到哪里”：

   - 如果 `authority.paused == true`
     - `expectedPositionMs = authority.positionMs`
   - 如果 `authority.paused == false`
     - `elapsedMs = max(0, nowMs - authorityAppliedAtMs)`
     - `progressedMs = elapsedMs * authority.playbackRate`
     - `expectedPositionMs = authority.positionMs + progressedMs`

   也就是说，当前并不是依赖服务端持续推位置，而是本地从最近一次权威基线向前做时间外推。

3. 判断 correction 类型  
   取：

   - `localPositionMs = player.currentPosition`
   - `driftMs = localPositionMs - expectedPositionMs`

   只有满足下面条件时，才允许做 correction：

   - 当前不是暂停态
   - 本地播放器还没有进入 ended 状态
   - 本地播放器不处于 buffering 状态
   - 已经拿到有效 authority baseline
   - 距离上次 authority baseline 应用已有一个最小检查窗口
   - 距离上一次 correction 已经超过 cooldown

   满足 guard 后按漂移大小分层：

   - `abs(driftMs) < 150ms`
     - 不做 correction
     - 这部分属于正常播放器误差，避免过度调节
   - `150ms <= abs(driftMs) < 2000ms`
     - 执行 speed nudge
     - 如果 `driftMs > 0`，说明本地跑得比 authority 快，临时降速到 `authorityRate * 0.97`
     - 如果 `driftMs < 0`，说明本地落后 authority，临时升速到 `authorityRate * 1.03`
     - 持续约 `1500ms` 后恢复 authority playbackRate
   - `abs(driftMs) >= 2000ms`
     - 执行 seek fallback
     - 直接 `seekTo(expectedPositionMs)`，避免长时间靠变速追不上

4. 处理 buffering 边界  
   如果本地播放器处于 `BUFFERING`：

   - 暂停本轮 drift correction
   - 不执行 speed nudge
   - 不执行 correction seek
   - 不丢弃最新 authority baseline
   - 等播放器恢复 `READY` 后继续按同一 authority baseline 做 drift 判断

   这样可以避免播放器已经在 rebuffer 或解码器 flush 时，又被 correction 打断，导致 `BUFFERING -> seek/变速 -> flush -> BUFFERING` 的连锁抖动。

5. 播放器缓冲策略  
   Android 端当前通过 `AndroidExoPlayerAdapter` 配置 `DefaultLoadControl`，使用 `minBuffer=30s / maxBuffer=90s / playbackStart=3.5s / rebufferStart=10s`。

   这不是同步协议的一部分，而是播放器本地的稳定性策略：2 倍速会更快消耗 HLS segment。当前除了记录原始 `bufferedAheadMs`，还会记录 `effectiveAheadMs = bufferedAheadMs / playbackSpeed` 和估算剩余 segment 数。

   2.0x 下还有额外的动态策略：

   - 起播前要求 `effectiveAheadMs >= 12s` 且估算剩余 segment 数至少为 `2`，不足时暂缓播放并输出 `high_speed_start_gate`。
   - `playbackSpeed >= 2.0x` 时，ABR 进入 `mobile_fast_720p`，最高限制到 720p。
   - 同一次播放中 rebuffer 次数达到 `2` 次后，继续保持 `mobile_fast_720p`，避免重新升到 1080p。
   - 2.0x 下 seek fallback 阈值从 `2000ms` 提高到 `3000ms`，减少高倍速下 seek 触发的 codec flush。

   后续判断是否继续优化时，优先观察 `WatchTogetherBuffer` 日志里的 `ahead / effectiveAhead / segments`：如果 `BUFFERING` 发生时 `effectiveAhead` 接近 0，说明是缓冲不足；如果 `effectiveAhead` 仍然很高但频繁 `BUFFERING`，则更可能是 HLS 封装、解码器或模拟器能力问题。

6. 处理 media ended 边界  
   如果本地播放器已经进入 ended：

   - 停止继续做 drift correction
   - 不再执行额外的 `seek + play`
   - 若已知媒体时长，则把 `expectedPositionMs` 截断到 `durationMs`

   这样可以避免视频结尾处因为 authority baseline 仍被视为“播放中”而产生：

   - play / pause 抖动
   - 结尾附近的反复 correction seek
   - sync log 被 drift correction 持续刷屏

#### Why This Works In Phase 1

这个算法的优点是：

- 实现简单
- 不需要服务端高频广播位置
- 足够解决“越播越偏”的明显问题
- 小漂移不再依赖硬 seek，观感更接近正常播放器
- 适合当前 2 人房间、小规模事件频率的阶段

它的限制也很明确：

- 使用的是本地 wall-clock 外推，不是精确时钟同步
- 大漂移仍然需要 seek fallback
- speed nudge 只能处理短时间小偏差，不能替代资源缓冲和解码性能优化
- 网络抖动、后台恢复、设备性能差异仍会影响误差大小

所以它是当前阶段“足够好”的最小方案，而不是最终极方案。

后续优化方向：

- correction cooldown
- 阈值调优
- 是否需要轻量 playback speed 微调
- 不同设备性能下的 correction 观感差异

### 7. Playback Rate Sync

`playbackRate` 是 authority state 的一部分，不能只留在本地播放器控件里。

关键点：

- host 修改倍速后，服务端要把新的 `playbackRate` 写回权威房间状态
- 服务端更新倍速前，应先结算当前有效 `positionMs`，保证 authority timeline 连续
- 服务端推进 `seq` 并广播 `set_playback_rate`
- 客户端接收后，需要同时应用 settle 后的位置和新的倍率
- repeated join / resync 收到的 `room_state.playbackRate` 必须反映当前权威倍率

算法思路：

1. host 发出倍速变更事件
2. 服务端先根据 authority timeline 结算当前有效位置
3. 再写入新的 `playbackRate`
4. 刷新 authority 时间基线
5. 推进 `seq`
6. 广播 `set_playback_rate`
7. 客户端应用新倍率后，继续沿更新后的 baseline 做 drift correction

### 8. Ended-state Authority

`ended` 是房间权威状态的一部分，而不只是本地播放器事件。

关键点：

- host 到达媒体结尾后，需要由服务端确认并广播 `ended`
- `ended` 不是简单复用 `pause`
- repeated join / reconnect 收到的 `room_state` 必须能表达 completed state
- drift correction 在 authority state 已经 `ended` 时必须停止

算法思路：

1. host 进入播放器 ended
2. Android 端上报 `ended(positionMs, seq)`
3. 服务端先结算 authority timeline 的当前有效位置
4. 服务端冻结 `positionMs`，写入 `ended=true`
5. 服务端广播 `ended`
6. 客户端收到后：
   - seek 到冻结位置
   - pause 本地播放器
   - 更新本地 authority baseline
   - 停止继续做 drift correction

这样可以解决两个关键问题：

- 视频播完后房间状态不再继续被当作“播放中”
- repeated join / reconnect 不会再收到一个继续向前外推的伪播放状态

### 9. Loop Prevention

当前同步实现必须避免回环。

典型风险：

- 收到远端 `seek` 后本地应用一次 seek
- 本地播放器事件又被当成新的本地控制事件发回服务端

当前原则：

- 远端事件应用与本地主动控制要分开
- correction 不回传
- join 初始同步不回传

后续优化方向：

- 明确记录 `isApplyingRemoteEvent`
- 明确记录 `lastDriftCorrectionAtMs`
- 更清楚地区分 user action 和 passive state application

## Performance-sensitive Areas

后续如果要针对性做性能优化，优先观察这些区域：

- WebSocket 消息处理与主线程切换
- 播放器状态采样频率
- authority baseline 到本地应用的延迟
- drift check 频率与阈值
- correction 触发次数
- correction 后再次偏离的速度
- heartbeat 周期与 timeout
- 房间状态广播后的客户端应用耗时

## Suggested Metrics To Observe Later

后续可以逐步补充这些观测指标：

- heartbeat timeout 次数
- host transfer 触发次数
- correction 触发次数
- average drift before correction
- average drift after correction

### 7. Player Telemetry And Rebuffer Diagnosis

播放器优化必须先能观测，再谈调参。当前 Android 端已经把播放、缓冲、ABR 和同步 correction 的关键信息拆成三个 Logcat 标签：

- `WatchTogetherBuffer`
  记录播放状态、当前位置、buffered position、buffered ahead、buffer percentage、倍速和当前 variant。
- `WatchTogetherABR`
  记录 master playlist 中可用的 video variants，以及 ExoPlayer 当前选择的分辨率、码率和 codecs。
- `WatchTogetherTelemetry`
  记录 buffering / rebuffer session 的开始和结束、rebuffer 次数、累计 rebuffer 时长，以及 correction 类型计数。

当前 telemetry 指标：

- `playbackState`
- `currentPosition`
- `bufferedPosition`
- `bufferedAheadMs`
- `playbackSpeed`
- `videoVariant`
- `currentMediaUrl`
- `rebufferCount`
- `totalRebufferDurationMs`
- `lastRebufferDurationMs`
- `driftCorrectionCount`
- `seekCorrectionCount`
- `speedNudgeCorrectionCount`
- `lastCorrectionReason`
- `lastCorrectionDriftMs`

#### Rebuffer Session

不是所有 `BUFFERING` 都算卡顿。初次加载进入 `BUFFERING` 是正常启动流程，只有播放已经开始后再次进入 `BUFFERING` 才计为 rebuffer。

当前判断：

1. 如果播放器首次载入资源时进入 `BUFFERING`
   - 记录为 `initial_buffer`
   - 不增加 `rebufferCount`

2. 如果播放器已经 `READY`、已有播放位置、已有 buffer 或正在播放后进入 `BUFFERING`
   - 记录为 `rebuffer`
   - `rebufferCount += 1`
   - 记录 `activeRebufferStartedAtMs`

3. 离开 `BUFFERING` 时
   - 计算本次 rebuffer duration
   - 累加 `totalRebufferDurationMs`
   - 更新 `lastRebufferDurationMs`

#### Diagnosis Flow

排查 1.5x / 2.0x 卡顿时，优先按这个顺序看：

1. 看 `WatchTogetherABR`
   - 如果当前 variant 是 `1080p`，而设备或模拟器解码吃力，可以优先怀疑 ABR 选档过高。
   - 如果 2.0x 下日志出现 `playback strategy=mobile_fast_720p`，说明播放器已经进入稳定优先策略。
   - 如果已经是 `720p` 仍频繁 rebuffer，继续看 buffer。

2. 看 `WatchTogetherBuffer`
   - 如果 `BUFFERING` 发生时 `effectiveAhead` 接近 `0ms` 或 `segments` 很少，更像下载、静态服务吞吐、segment 生成或 buffer 策略不足。
   - 如果 `ahead` 仍然很高但进入 `BUFFERING`，更像解码器、模拟器能力、HLS 封装或 codec 参数问题。

3. 看 `WatchTogetherTelemetry`
   - `rebuffer start/end` 可以确认每次卡顿是否真的进入 rebuffer。
   - `rebufferCount` 和 `totalRebufferDurationMs` 用来比较优化前后是否真的变好。

4. 看 correction 日志
   - 如果主要出现 `correction type=speed_nudge`，说明漂移较小，播放器正在用临时变速温和追平。
   - 如果 rebuffer 附近频繁出现 `correction type=drift_seek`，说明漂移已经超过 seek fallback 阈值，同步 correction 可能在打断播放器。
   - 如果出现 `correction type=speed_nudge_restore`，说明临时变速已经恢复到 authority playbackRate。
   - 如果 correction 很少但 rebuffer 很多，说明更可能是播放资源、ABR 或解码能力问题。

5. 结合播放倍率
   - 1.0x 稳定、1.5x 稳定、2.0x 不稳定，通常说明资源和设备在高倍率下触达了吞吐或解码上限。
   - 1.0x 都不稳定，优先检查 HLS 文件、静态服务、URL 和 ExoPlayer 错误。

#### 2.0x Acceptance Notes

2.0x 是当前播放器压力最大的常规倍率，验收时需要区分模拟器和真机：

- Android 模拟器：
  - 允许偶发短 rebuffer，尤其是 CPU 核心数较低、宿主机负载较高、或 1080p 被选中时。
  - 重点看优化后是否进入 `mobile_fast_720p`，以及 `rebufferCount / totalRebufferDurationMs` 是否明显下降。
  - 如果 720p 下仍持续 rebuffer，优先检查模拟器 CPU 核心数、硬件加速和源文件编码参数。
- Android 真机：
  - 720p + 2.0x 应作为最低目标体验。
  - 稳定网络、本地静态服务或对象存储正常的情况下，不应出现连续 rebuffer。
  - 如果真机仍频繁 rebuffer，需要优先回看 HLS segment、编码 profile、码率、关键帧间隔和 ABR variant。

当前 2.0x 测试建议同时记录：

- 当前 variant：`WatchTogetherABR`
- buffer ahead：`WatchTogetherBuffer`
- rebuffer 次数与时长：`WatchTogetherTelemetry`
- correction 类型：`speed_nudge / drift_seek`
- 播放设备：模拟器配置或真机型号
- ignored stale event 次数
- repeated join / reconnect 后的 resync 成功率

## Current Conclusion

在当前项目阶段，下面三项最值得作为“核心功能中的核心技术点”持续跟踪：

1. authority state + event ordering
2. heartbeat + lifecycle cleanup
3. drift correction
4. playback rate sync

其中 `drift correction` 和 `playback rate sync` 都很重要：

- `drift correction` 决定“同步控制已经成功后，观看体验是否还能继续保持同步”
- `playback rate sync` 决定两端是否会因为不同倍速而再次持续拉开时间线
