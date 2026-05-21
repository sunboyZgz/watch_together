# Backend Risk Hardening Requirements

> 更新时间：2026-05-18  
> 目的：在处理 Top 10 Backend Risks 之前，先把真实业务需求、当前实现状态、目标行为和成熟工具选型记录下来。后续每个风险进入实现前，都应先让需求在本文或对应 issue 中被审阅确认。

## 1. 当前最高优先级

后端加固优先级按以下顺序理解：

1. 多用户同步观影性能。
2. WebSocket 可靠性。
3. `room_state` 一致性。
4. 房主控制权正确性。
5. 断线、重连和主动离开处理。
6. HLS 访问压力和访问控制。
7. PostgreSQL / Redis / 进程内状态边界。

当前不把微服务拆分、Kubernetes、多区域部署、复杂社交系统作为第一优先级。

## 2. 已确认业务决策

### 2.1 WebSocket 必须鉴权

- WebSocket 必须携带登录 token。
- 后端不能信任 payload 中的 `userId`。
- 握手鉴权通过后，连接身份由 token 绑定。
- JWT 已作为正式 access token 的成熟工具落地。

### 2.2 公开房间加入规则

- 当前阶段，只要是平台已登录用户，就可以加入公开房间。
- 当前阶段不做私密房间密码。
- 后续私密房间可以增加首次加入密码验证。

### 2.3 不做自动 host transfer

- 业务需求已经改变：不再做自动房主转移。
- 房主断开后，房间时间轴暂停，`hostUserId` 置空，其他成员不能控制播放。
- 原房主在房间保留期内重连后恢复房主身份。
- 未来如果支持房主转移，必须是显式手动操作。

### 2.4 区分断线重连和主动离开

- 异常断线进入 5 分钟 `grace_period`。
- 如果用户 5 分钟内回来，应恢复房间。
- 主动离开房间应立即退出成员关系。
- 如果最后一个用户异常断开，房间也暂时保留。
- 如果 5 分钟后没人回来，房间清理。
- 只要房间还有一个在线或保留中的成员，就不走最终清理。

### 2.5 慢客户端不能拖垮房间

- 一个慢用户不能阻塞整个房间广播。
- 不默认立刻踢出慢用户。
- 优先合并或丢弃可恢复消息，例如最新 `room_state`。
- 客户端可以通过 `room_state.request` 恢复最新状态。
- 长期不可写、队列持续阻塞或心跳超时后，服务端可以关闭连接。

### 2.6 房间规模目标

- 企业级方向应能支持单房间 100+ 人同步观看。
- 因此不应使用高频全量播放状态广播推进时间轴。
- 服务端保留权威 timeline，客户端用 `serverTimeMs + velocity` 外推。

### 2.7 stale seq 从诊断进入拒绝

- 当前 `seq` 已作为控制请求的权威版本校验字段。
- 旧控制如果携带过期 `seq`，服务端必须拒绝推进时间轴，并返回最新 `room_state`。
- 控制请求里的客户端版本字段后续建议命名为 `expectedSeq`，和服务端广播 `seq` 区分。
- 迁移期可继续接受旧字段 `seq`，但语义已经是乐观版本前置条件。

### 2.8 requestId 去重

- 当前进程内 sharded bounded map 适合当前单实例阶段。
- Redis `SET NX EX` 去重只在多实例、客户端重试频率明显升高或跨进程房间权威设计明确后引入。
- 去重不能替代 `seq` 检查。

### 2.9 房间清理

- 房间只要还有一个在线或保留中的用户，就不进入最终清理。
- 最后一个用户异常断开时进入 `grace_period`。
- `grace_period` 为 5 分钟。
- 到期无人恢复则清理内存房间和 PostgreSQL 房间主数据。

### 2.10 HLS 必须做访问控制

- HLS 资源不能长期裸露为任意公开 URL。
- 非平台用户不能通过分享公开 URL 随意使用资源。
- Go 后端负责鉴权和签发短期访问凭证。
- HLS 文件字节由 Nginx、对象存储或 CDN 服务承载。

## 3. 成熟工具选型记录

| 领域 | 当前或推荐工具 | 状态 | 使用原因 | 备注 |
| --- | --- | --- | --- | --- |
| HTTP 路由 | `github.com/gin-gonic/gin` | 已使用 | 路由、middleware、恢复机制成熟 | 当前不需要替换 |
| WebSocket | `github.com/coder/websocket` | 已使用 | Go 原生风格、API 简洁、可控性强 | 保留独立读写循环和应用层心跳 |
| PostgreSQL ORM/连接 | GORM + `database/sql` pool | 已使用 | 当前业务 CRUD 和事务足够 | migrations 继续 SQL-first |
| PostgreSQL driver | pgx / gorm postgres driver | 已使用 | PostgreSQL 生态成熟 | 连接池参数由 `store.OpenPostgres` 管理 |
| Redis client | `github.com/redis/go-redis/v9` | 已使用 | Redis 官方推荐 Go 客户端之一 | room_state 明确 DB 0 |
| 密码哈希 | `golang.org/x/crypto/bcrypt` | 已使用 | 密码存储成熟方案 | 保留 |
| JWT | `github.com/golang-jwt/jwt/v5` | 已使用 | 正式 HTTP / WS token 生成和验证 | 当前签发 HS256 access token，不保留 legacy dev token |
| 指标 | Prometheus 风格 metrics | 待实现 | 广播延迟、队列深度、连接数、错误率可观测 | 可先用 `prometheus/client_golang` |
| 日志持久化 | Loki / OpenSearch / 云日志服务 | 待选型 | 应用日志不写业务数据库 | 本地先 stdout，部署用日志管道收集 |
| HLS 分发 | Nginx / 对象存储 / CDN | 已有方向 | Go 不承载大流量字节分发 | 后端签发短期 URL 或 cookie |
| S3 兼容存储 | AWS SDK Go v2 + MinIO/S3-compatible | 已使用 | 本地和云端统一抽象 | 后续评估 OSS / COS / BOS / R2 |
| 媒体处理 | FFmpeg / FFprobe | 已使用 | HLS 生成和媒体探测事实标准 | 由 `mediactl` 调用 |
| Android 播放器 | AndroidX Media3 ExoPlayer | 已使用 | HLS 播放和缓存成熟 | 客户端同步对齐基于服务端 timeline |

选型原则：

- 能使用成熟工具时优先使用成熟工具。
- 工具必须服务于具体风险：鉴权、广播、状态一致性、连接稳定性、媒体分发或可观测性。
- 不为了“企业级”名义提前引入暂时没有触发条件的复杂平台。

## 4. Top 10 风险需求清单

### R1 WebSocket 鉴权缺失

状态：已完成第一轮加固。

业务需求：

- WS 连接必须携带登录 token。
- 服务端从 token 获取 `userId`。
- `join_room` 和控制事件中的 `userId` 只能作为兼容字段，不作为信任来源。

当前状态：

- HTTP 登录返回 JWT access token。
- 服务端 token verifier 只接受 JWT access token。
- WS 握手要求 `Authorization: Bearer <accessToken>`。
- WS 连接身份来自 token 验证结果。

目标行为：

- 未携带 token 或 token 无效时拒绝 WS 握手。
- 连接身份在握手后绑定到 `ClientConnection`。
- `join_room`、`room_state.request` 和控制事件不再信任 payload `userId` 推进权限。
- 控制事件使用连接身份做房主校验。
- 控制事件还要求连接是当前 active room device，避免同一账号多设备同时推进房间状态。

实现前需确认：

- 已选择 `Authorization` header；Android OkHttp 已补齐 header。
- 不保留 legacy `dev_<userId>` 兼容。

优先级：P0。

### R2 公开房间加入和成员关系边界

业务需求：

- 当前公开房间允许任意登录用户加入。
- 私密房间密码后置。
- 需要避免重复成员、伪造用户加入和跨房间身份混乱。

当前状态：

- HTTP join 会写 PostgreSQL 成员关系。
- WS join 会把连接加入内存房间。
- 当前 WS join 是否必须先 HTTP join 需要重新确认。

目标行为：

- WS join 至少要求已登录用户和有效 roomCode。
- 如果保留 HTTP join，则 WS join 应校验用户已经是 active member。
- 如果允许 WS 首次加入，则 WS join 应通过服务层补齐成员关系或触发明确的加入流程。

实现前需确认：

- 最新阶段是“HTTP join 后 WS join”，还是“WS join 可直接加入公开房间并补齐成员”。

优先级：P1。

### R3 房主断开后的控制权

业务需求：

- 不做自动 host transfer。
- 房主断开后暂停时间轴，房主不可用。
- 原房主重连恢复房主身份。

当前状态：

- `room.Leave` 已实现 `host_left`。
- `JoinWithLimit` 已支持 owner reclaim。
- 代码字段已从泛化的 `HostChanged` 收敛为 `HostReclaimed`，避免误读为自动 host transfer。

目标行为：

- 当前有效文档、协议、测试和 UI 文案都不再表达“自动 host transfer”。
- 普通成员不能在 host unavailable 状态控制播放。
- 原房主重连后广播最新 `room_state`。

实现前需确认：

- 房主主动离开且仍有其他成员时，当前进入 owner unavailable；如需关闭房间，应另建显式 `close_room` 需求。

优先级：P0。

### R4 断线重连 grace period

业务需求：

- 异常断线进入 5 分钟 grace period。
- 主动离开立即退出成员关系。
- 最后一个用户异常断开也暂时保留房间。
- grace 到期无人回来才清理。

当前状态：

- 内存和 PostgreSQL 已有 grace period 机制。
- 当前默认值已改为 5 分钟。
- WebSocket 已新增 `leave_room` 主动离开协议。
- Android 正常关闭房间会先发送 `leave_room`；异常断线不会发送。

目标行为：

- 默认 grace period 改为 5 分钟。
- 增加 `leave_room` 或等价协议，区分主动离开和网络断开。
- 主动离开会将 PostgreSQL `room_members.is_active` 置为 false。
- 主动离开导致空房间时立即清理，不进入 grace period。
- 重连恢复时清除房间 grace 状态。

实现前需确认：

- 房主主动离开且仍有其他成员时，目前进入 owner unavailable，不自动转移 host。
- 后续如果需要“房主主动关闭房间”，应另建显式 `close_room` 语义。

优先级：P0。

### R5 慢客户端处理

业务需求：

- 慢客户端不能拖慢整个房间。
- 不默认立刻踢出慢客户端。
- 可恢复消息优先合并，客户端可主动拉取最新状态。

当前状态：

- 每个连接已有出站队列。
- 广播有并发限制、enqueue timeout 和 coalescing。
- `room_state` 可以合并，控制事件不合并。
- 当前 timeout 后可能关闭连接，并记录队列压力指标。

目标行为：

- 定义不同消息的可丢弃/不可丢弃级别。
- `room_state` 可 coalesce，控制事件保持逐条处理。
- 控制事件在过渡期仍保留 legacy event，但客户端可以靠 `room_state.request` 恢复。
- 记录慢客户端指标、队列压力和超时关闭结果，便于压测。

实现前需确认：

- 哪些消息必须可靠送达，哪些允许被最新状态覆盖。
- 关闭慢连接的时间阈值和 UI 重连体验。

优先级：P1。

### R6 100+ 人房间广播压力

业务需求：

- 企业级方向要能支撑 100+ 人单房间观看。
- 同步模型不能依赖高频状态广播。

当前状态：

- 服务端以控制事件和 `room_state` 快照为主。
- 广播层已有限流、超时和队列。

目标行为：

- 保持“控制事件驱动 + 客户端外推”。
- 不做每秒多次全房间状态广播。
- 增加 2 / 5 / 10 / 20 / 100 人房间压测。
- 关注广播耗时、队列深度、慢连接、goroutine 数和内存。

实现前需确认：

- 当前测试机和 CI 是否具备 100 WS 连接压测环境。
- 100 人是单房间目标，还是阶段性先验收 20 人再扩展。

优先级：P1。

### R7 stale seq 拒绝

业务需求：

- stale seq 不应继续推进权威时间轴。
- 房主快速重复控制时，服务端应能判断旧控制。

当前状态：

- 当前 client seq 已进入控制前置校验。
- 服务端 seq 是权威递增版本。

目标行为：

- 客户端控制请求携带 `expectedSeq`，当前协议阶段继续使用 `seq` 字段承载这个语义。
- 如果 `expectedSeq != currentSeq`，服务端拒绝并返回最新 `room_state`。
- 如果 `expectedSeq == currentSeq`，服务端接受并推进。
- 拒绝时不推进 timeline，不广播控制事件，不写入 requestId 去重表。

实现前需确认：

- 已决定不保留宽松旧客户端行为；`seq == 0` 只有在当前权威 seq 也为 0 时才可能通过。
- 拒绝时不单独发 error，直接向请求方返回最新 `room_state`，让客户端恢复。

优先级：P0。

### R8 控制事件频率和广播风暴

业务需求：

- 大多数情况下时间轴按 1 倍速自然推进。
- 同步观影不鼓励高频跳跃。
- 快速 seek 或重复按钮不应形成广播风暴。

当前状态：

- requestId 去重能处理重试重复。
- 不能合并不同 requestId 的快速 seek。

目标行为：

- 客户端只在明确用户操作时发送控制事件。
- seek 建议在拖动结束后发送，而不是拖动中高频发送。
- 服务端可按房间对 seek 做短窗口最后值优先，需谨慎评估观感。
- 所有控制事件统一写入权威 timeline 并递增 seq。

实现前需确认：

- Android 当前 seek 发送时机是否已经是拖动结束。
- 是否需要第一版服务端 seek 节流，还是先压测观察。

优先级：P2。

### R9 房间生命周期和孤儿房间

业务需求：

- 有人在线或保留中时不最终清理。
- 全部用户离开或 grace 过期后清理。
- 服务重启后不能留下永久 active 的孤儿房间。

当前状态：

- 启动时会把 active rooms backfill 到 grace_period。
- cleanup loop 会清理过期房间。
- 当前 grace 默认值已调整为 5 分钟。

目标行为：

- 进程内和 PostgreSQL 生命周期一致。
- 服务重启后 active 房间进入 grace，而不是永久 active。
- 清理时 Redis room_state 也应删除或让 TTL 过期。

实现前需确认：

- 清理房间是否保留历史记录。目前实现是删除 `rooms`，是否满足后续审计/历史需求需确认。

优先级：P1。

### R10 HLS 访问控制和分发压力

业务需求：

- 非平台用户不能通过公开 URL 看资源。
- Go 不应长期承载 HLS 大流量分片。
- 后端负责鉴权并签发短期访问凭证。

当前状态：

- 本地和对象存储 URL 主要是可直接播放 URL。
- HLS 分发安全仍待实现。

目标行为：

- 小规模阶段：后端校验用户和媒体权限，签发短期 signed URL。
- 公网阶段：Nginx / 对象存储 / CDN 服务文件字节。
- CDN 或对象存储缓存 playlist / segment。
- `media_url` 可以从永久 public URL 迁移为内部 object key 或可签名资源标识。

实现前需确认：

- 第一版使用对象存储 presign、Nginx secure_link 还是 CDN token auth。
- 房间媒体访问是否仅要求登录，还是要求已加入房间。

优先级：P1。

## 5. 建议实施顺序

第一批必须先做：

1. WebSocket token 鉴权和连接身份绑定。已完成第一轮加固。
2. stale seq 拒绝和拒绝后的 `room_state` 恢复。已完成第一轮加固。
3. 断线、重连、主动离开和 5 分钟 grace period。已完成第一轮加固。
4. host transfer 旧语义清理，确保只保留 owner reclaim。已完成第一轮加固。

第二批稳定性增强：

1. 慢客户端策略文档化和指标化。已完成第一轮加固。
2. 100+ 人房间压测脚本。已完成第一版。
3. 房间生命周期清理和 Redis cache 删除策略。已完成首轮：空房间销毁后同步删除 `room_state` cache；grace_period 期间保留。
4. 控制事件频率压测和 seek 节流评估。

第三批公网前必须做：

1. HLS signed URL / signed cookie。
2. 媒体 URL 从 public URL 向可授权资源标识演进。
3. CDN / 对象存储缓存策略。
4. 基础 metrics、日志持久化和告警。

## 6. 每个风险实现前的需求评审模板

```text
风险编号：
业务目标：
当前实现：
目标行为：
协议变化：
数据变化：
Redis / PostgreSQL / 内存边界：
客户端影响：
测试验收：
需要使用的成熟工具：
不做的范围：
```

实现前必须明确“不做的范围”，避免一个风险修复演变成过大的架构重写。

## 7. 当前明确不先做

| 项目 | 暂不优先原因 | 触发条件 | 当前应保留的设计空间 |
| --- | --- | --- | --- |
| 自动 host transfer | 最新业务已取消 | 未来需要多人协作管理房间 | 保留显式手动转移的可能 |
| 私密房间密码 | 当前公开房间足够联调 | 出现真实私密观影需求 | join 流程预留校验点 |
| 多实例房间调度 | 当前单实例权威更简单可靠 | 单实例连接数或可用性成为瓶颈 | 不把 Redis 当 timeline authority |
| Redis 跨实例 dedup | 当前单实例 requestId 足够 | 多实例 WS 或高重试频率 | requestId key 设计已预留 |
| 消息队列 | 当前没有跨服务事件流 | 聊天、弹幕、审计、异步任务扩大 | 控制历史不要只放 Redis |
| Kubernetes | 当前部署复杂度不值得 | 多服务、多实例、灰度和自动扩缩容 | 配置、健康检查、无状态边界先做好 |
| 大型后台管理 | 不影响核心同步正确性 | 媒体库运营和用户管理成规模 | 后端 API 保持清晰权限边界 |
