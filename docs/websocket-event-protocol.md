# WebSocket Event Protocol

> Minimal cross-platform WebSocket event protocol for Phase 1 sync.

本文档定义 `watch_together` 当前阶段最小可用的 WebSocket 事件协议，供 Android、Windows 和 Server 共用。

当前目标是先形成一套足够稳定的基础同步闭环，而不是一次覆盖完整的实时协作协议。

## Scope

当前协议只覆盖以下场景：

- 用户加入房间
- 服务端下发当前房间主状态
- 房主触发播放
- 房主触发暂停
- 房主触发拖动
- 服务端下发基础错误

当前协议不覆盖以下内容：

- 鉴权协议
- 复杂重连策略
- 错误码体系完善化
- 多房主和复杂权限
- 字幕、音轨、连麦扩展
- 版本协商机制

## Design Goals

- 使用跨平台语义，不绑定 Android 或 ExoPlayer 专属命名
- 保持消息外壳统一
- 统一关键字段命名和单位
- 明确消息方向
- 为 Phase 1 的 Android ↔ Android 实现提供最小闭环
- 为后续 Windows 端复用留出空间

## Encoding Decision

当前阶段的 WebSocket 传输层先使用 JSON 文本协议。

具体约定：

- WebSocket message format: JSON
- 使用 UTF-8 text frames
- 所有消息统一 `type + payload`
- `positionMs` 使用整数毫秒
- `seq` 使用服务端递增整数

当前不使用二进制编码协议，原因如下：

- `INT-19` 当前目标是先定义最小协议语义，而不是做传输优化
- 当前消息类型较少，且 payload 很小
- 当前跨端联调更需要可读性、可抓包性和实现一致性
- 当前性能关键路径更可能出现在状态抽象、广播逻辑和同步策略，而不是编码格式

## Future Encoding Strategy

后续可以保留引入二进制编码的可能性，但不应在当前阶段提前设计。

只有在以下条件逐步出现后，才值得认真评估：

- 房间人数明显上升
- 消息频率显著提高
- 状态对象明显变大
- profiling 已证明编码与解码成为瓶颈

即使后续切换编码方式，也应保持事件语义模型稳定，不改变 `play`、`pause`、`seek`、`room_state` 等事件本身的含义。

## Message Envelope

所有消息统一使用 `type + payload` 结构：

```json
{
  "type": "event_name",
  "payload": {}
}
```

约束说明：

- `type` 表示事件名称
- `payload` 表示事件数据
- 当前阶段不额外引入 `version`、`meta`、`traceId` 等扩展字段

## Common Field Semantics

### `roomId`

- 含义：房间 ID
- 类型：string

### `mediaId`

- 含义：当前房间正在使用的媒体 ID
- 类型：string

### `hostUserId`

- 含义：当前房主用户 ID
- 类型：string

### `userId`

- 含义：发送或触发当前操作的用户 ID
- 类型：string

### `positionMs`

- 含义：播放器当前播放位置
- 单位：毫秒
- 类型：number

### `playbackRate`

- 含义：当前播放倍率
- 类型：number
- 示例：`1.0`、`1.25`

### `paused`

- 含义：当前是否处于暂停状态
- 类型：boolean

### `seq`

- 含义：房间状态序列号
- 类型：number
- 约束：按房间内状态变更单调递增
- 用途：帮助客户端判断事件新旧顺序

### `message`

- 含义：错误或提示信息
- 类型：string

## Protocol Rules

- 所有消息统一使用 `type + payload`
- 播放位置统一使用 `positionMs`
- 协议字段使用跨平台语义，不使用 Android/ExoPlayer 专属命名
- 当前阶段只覆盖 `join_room`、`room_state`、`play`、`pause`、`seek`、`heartbeat`、`heartbeat_ack`、`error`
- `seq` 视为服务端权威顺序号；客户端上报事件时可带上本地已知 `seq`，服务端广播时应使用最新状态序号
- `heartbeat` 与 `heartbeat_ack` 只用于连接健康检测，不参与房间状态推进

## Control Sync Strategy

当前阶段的 `play / pause / seek` 同步建议统一按以下策略实现：

- 出站控制事件优先来自明确的用户操作，而不是直接把播放器底层所有事件都原样上报
- 服务端维护房间权威状态，并负责推进 `seq`
- 客户端收到服务端广播事件后，只有当事件 `seq` 新于本地已知状态时才应用
- 客户端在应用远端事件时，应避免再次把同一变化回传给服务端，防止回环

这意味着当前阶段更推荐：

- 本地用户点击 `Play` 才发送 `play`
- 本地用户点击 `Pause` 才发送 `pause`
- 本地用户明确完成一次 seek 操作后才发送 `seek`

而不是：

- 监听到任意播放器状态变化就立即上报
- 把远端事件应用到播放器后再次触发新的上报

## Server-side Event Rules

围绕 `play / pause / seek`，当前服务端建议遵循以下最小规则：

- 仅允许当前房主发起控制事件
- 房间不存在时返回 `error`
- 非房主发送控制事件时返回 `error`
- 服务端在接受控制事件后更新内存中的房间状态
- 服务端在更新状态后递增 `seq`
- 服务端广播的事件应携带最新 `seq`

当前阶段服务端无需引入复杂仲裁，只需要把“谁是权威、状态何时更新、`seq` 何时递增”这几个点保持稳定。

## Client-side Apply Rules

当前阶段客户端处理服务端广播事件时，建议统一遵循以下规则：

- 若收到的 `seq` 小于或等于本地已知 `seq`，则忽略该事件
- `play`：先把本地基线位置对齐到事件 `positionMs`，再执行播放
- `pause`：先把本地基线位置对齐到事件 `positionMs`，再执行暂停
- `seek`：先执行 seek 到事件 `positionMs`，再根据当前房间暂停状态决定是否保持暂停
- 应用成功后更新本地 `RoomSyncState`

这几条规则的目标是先保证“控制语义一致 + 新旧顺序清楚”，而不是一次解决所有漂移修正问题。

## Event Definitions

### `join_room`

- 方向：client -> server
- 作用：请求加入指定房间

```json
{
  "type": "join_room",
  "payload": {
    "roomId": "room_001",
    "userId": "user_a"
  }
}
```

### `room_state`

- 方向：server -> client
- 作用：
- 新用户加入后同步当前房间主状态
- 房间状态基线广播
- 当前 Android 首个客户端实现会在 join 成功后把这条 `room_state` 视为本地权威基线，并立即应用到播放器
- 当房间处于播放中时，服务端返回的 `positionMs` 应代表“当前有效播放位置”，而不是最后一次控制事件被冻结的旧位置

```json
{
  "type": "room_state",
  "payload": {
    "roomId": "room_001",
    "mediaId": "sample_001",
    "hostUserId": "user_a",
    "paused": false,
    "positionMs": 125000,
    "playbackRate": 1.0,
    "seq": 3
  }
}
```

### `play`

- 方向：client -> server / server -> clients
- 作用：房主发起播放，服务端确认后广播
- 当前阶段建议仅由显式用户点击 `Play` 触发上报
- 服务端广播时应使用最新 `seq`

```json
{
  "type": "play",
  "payload": {
    "roomId": "room_001",
    "userId": "user_a",
    "positionMs": 125000,
    "seq": 4
  }
}
```

### `pause`

- 方向：client -> server / server -> clients
- 作用：房主发起暂停，服务端确认后广播
- 当前阶段建议仅由显式用户点击 `Pause` 触发上报
- 服务端广播时应使用最新 `seq`

```json
{
  "type": "pause",
  "payload": {
    "roomId": "room_001",
    "userId": "user_a",
    "positionMs": 130500,
    "seq": 5
  }
}
```

### `seek`

- 方向：client -> server / server -> clients
- 作用：房主发起拖动，服务端确认后广播
- 当前阶段建议在一次明确 seek 动作完成后再上报，而不是高频连续上报拖动过程
- 服务端广播时应使用最新 `seq`

```json
{
  "type": "seek",
  "payload": {
    "roomId": "room_001",
    "userId": "user_a",
    "positionMs": 210000,
    "seq": 6
  }
}
```

### `heartbeat`

- 方向：server -> client
- 作用：由服务端定时确认连接仍然存活
- 当前阶段使用应用层 heartbeat，而不是只依赖底层 WebSocket ping/pong
- `heartbeat` 不参与 `seq`

```json
{
  "type": "heartbeat",
  "payload": {
    "serverTimeMs": 1710000000000
  }
}
```

### `heartbeat_ack`

- 方向：client -> server
- 作用：客户端确认自己仍在线，并回传本次 heartbeat 的服务端时间戳
- `heartbeat_ack` 不参与 `seq`
- 当前阶段建议客户端收到 heartbeat 后立即回包

```json
{
  "type": "heartbeat_ack",
  "payload": {
    "serverTimeMs": 1710000000000,
    "clientTimeMs": 1710000000123
  }
}
```

## Heartbeat Rules

当前阶段 heartbeat 的最小语义如下：

- 服务端周期性向每个活跃 WebSocket 连接发送 `heartbeat`
- 客户端收到后尽快发送 `heartbeat_ack`
- 服务端仅把 heartbeat 视为连接健康信号，不把它写入 `room_state`
- 若服务端在超时窗口内未收到 ack，则关闭该连接并走现有 disconnect cleanup
- heartbeat 超时触发的 disconnect，应与显式断连走同一条 room cleanup / host transfer 流程

当前建议初始值：

- heartbeat interval: `5s`
- heartbeat timeout: `15s`

### `error`

- 方向：server -> client
- 作用：下发当前阶段最小错误信息

```json
{
  "type": "error",
  "payload": {
    "roomId": "room_001",
    "message": "room not found"
  }
}
```

## Direction Summary

| Event | Direction |
| --- | --- |
| `join_room` | client -> server |
| `room_state` | server -> client |
| `play` | client -> server, server -> clients |
| `pause` | client -> server, server -> clients |
| `seek` | client -> server, server -> clients |
| `error` | server -> client |

## Recommended Phase 1 Execution Order

围绕控制事件同步，当前最推荐的实现顺序是：

1. 先完成 `INT-20 play sync`
2. 再完成 `INT-21 pause sync`
3. 最后完成 `INT-22 seek sync`

原因：

- `play` 和 `pause` 的状态语义最简单，最适合先验证“客户端发事件 -> 服务端更新状态 -> 广播 -> 另一端应用”这条主链路
- `seek` 对本地播放器位置、状态收敛和回环控制更敏感，适合放在前两项稳定后再实现

## Current Implementation Guidance

- Android 侧应把播放器控制结果映射到本协议语义，而不是直接暴露播放器内部事件命名
- Server 侧应以房间状态为权威源，负责生成和推进 `seq`
- Windows 侧后续接入时，应直接复用相同事件名称和字段单位

## Follow-up

后续可能在此协议基础上继续扩展：

- `leave_room`
- `host_changed`
- `playback_rate_changed`
- 心跳与重连
- 更清晰的错误码结构
