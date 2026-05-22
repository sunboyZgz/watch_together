# Watch Together 当前业务功能说明

> 更新时间：2026-05-18  
> 目的：记录仓库当前前后端已实现能力、部分实现能力和已确认但待实现的业务规则。旧业务文档可能仍有历史价值，但后续需求评审和重构应优先参考本文。

## 1. 产品定位

`watch_together` 是一个自托管同步观影系统。当前主线是先跑通 `Android ↔ Android` 多用户同步观影 MVP：用户登录、选片、创建房间、加入房间、播放 HLS 视频、由房主控制房间权威时间轴。

当前不优先做社交化扩展、复杂权限、微服务拆分或多区域部署。后端优先级是同步正确性、WebSocket 稳定性、房间状态一致性、并发广播性能、HLS 访问控制和媒体分发压力。

## 2. 用户与账号

当前能力：

- 已实现 `POST /auth/register`，创建账号、昵称和密码。
- 已实现 `POST /auth/login`，使用账号密码登录。
- 密码使用 `bcrypt` 哈希后写入 PostgreSQL。
- 账号写入前会做 trim 和 lowercase 归一化。
- 当前登录返回 JWT access token。
- Android 登录成功后在本地持久化 `AuthSession`，后续 HTTP 请求携带 `Authorization: Bearer <accessToken>`。

已确认的下一步需求：

- WebSocket 已要求携带登录 token。
- 后端不再信任 WebSocket payload 中的 `userId`，连接身份从 token 验证结果绑定。
- JWT 已作为 access token 工具落地。
- 后续 token 持久化、refresh token、登出和多端 session 管理另行设计。

## 3. 首页

当前能力：

- Android 登录后进入首页。
- 首页通过 `GET /home/summary` 加载当前用户摘要。
- 首页展示当前用户昵称、头像种子、上次观看和继续观看内容。
- 观看历史来自 `user_media_progress + media_episodes + media_seasons`。

后端边界：

- 首页是低频业务读接口，不承载实时播放状态。
- 首页进度来自用户观看进度表，不等于房间实时 `room_state`。

## 4. 媒体库与选片

当前能力：

- Android 选片页通过 `GET /media/tags` 获取 featured / active 标签。
- 通过 `GET /media/items?query=&tag=&limit=&cursor=` 检索 episode-backed 媒体。
- `GET /media/items` 需要登录态，因为响应包含可播放 `mediaUrl`；当前 `mediaUrl` 是短期签名播放入口，不直接暴露数据库中的永久 HLS URL。
- 媒体主模型是 `media_seasons + media_episodes + media_tags + media_season_tags`。
- `media_episodes.id` 是当前创建房间使用的媒体 ID。
- 旧的扁平媒体表已经不再作为目标模型。

媒体字段：

- `media_seasons` 保存标题、简介、制作团队、封面、季标签和搜索字段。
- `media_episodes` 保存集标题、集标签、HLS 播放 URL、时长、source_key 和 source_hash。
- `media_episode_variants` 保存 HLS variant playlist、分辨率、码率、codecs 和健康信息。

媒体来源演进边界：

- 当前已落地的是 `hosted_hls` 模式：服务端返回平台托管或外部 HLS 播放入口。
- 后续可扩展 `local_file / local_library` 模式：服务端只同步媒体指纹和播放时间轴，不负责分发用户本地视频文件。
- HLS signed URL / signed cookie 只针对平台托管或公网对象存储媒体；本地目录模式重点是媒体指纹匹配和客户端本地可用性检查。

## 5. 房间创建、加入与详情

当前能力：

- `POST /rooms` 创建房间。
- 创建房间会在 PostgreSQL 写入 `rooms` 和初始 `room_members`。
- 创建房间后会把持久化房间注册到内存 `room.Manager`，用于实时同步。
- `POST /rooms/{roomCode}/join` 支持通过房间码加入业务房间。
- `GET /rooms/{roomCode}` 提供放映室首屏业务数据，包括房间、媒体和成员。
- `GET /rooms/{roomCode}` 需要登录态，因为响应包含放映室媒体播放 URL。
- `GET /rooms/{roomCode}` 需要用户已经是该房间 active member；未加入者必须先走 `POST /rooms/{roomCode}/join`。
- WebSocket `join_room` 同样要求用户已经是 active member，不再作为首次加入业务关系的入口。
- Android 当前在进入放映室前会尽量加载 room detail，避免收到实时状态时缺少可播放 `mediaUrl`。
- Android WebSocket 握手已携带 `Authorization: Bearer <accessToken>`。

当前公开房间规则：

- 只要是平台已登录用户，都可以加入公开房间。
- 当前阶段不要求先做私密房间、邀请码或密码。
- 后续私密房间可以在首次加入时增加密码验证。

需要澄清的实现差异：

- 当前服务端仍存在 HTTP join 流程。
- 最新业务方向允许 WebSocket join 在登录 token 验证通过后成为实时加入入口，但是否仍强制先 HTTP join，需要在实现 WS 鉴权时再次确认。

## 6. 放映室与播放器

Android 当前能力：

- 放映室页面由 `RoomTheaterScreen / RoomTheaterPage` 承载。
- 播放器使用 AndroidX Media3 ExoPlayer。
- HLS 播放使用 Media3 `HlsMediaSource`。
- 已接入 `SimpleCache + CacheDataSource`，缓存 HLS playlist / segment。
- 支持播放、暂停、seek、倍速、清晰度选择、基础诊断日志和播放状态展示。
- Media3 原生控制器关闭，使用项目自定义控制层。

同步控制规则：

- 房主可以发送 `play / pause / seek / set_playback_rate / ended`。
- 普通成员不能推进房间权威时间轴。
- 普通成员本地误操作应被客户端忽略或被服务端拒绝。
- 客户端应根据 `room_state` 的 `positionMs + velocity + serverTimeMs` 计算当前权威播放位置。
- 房主和普通成员后续都可以增加“重新对齐到房间时间轴”的按钮，请求最新 `room_state` 并修正本地播放器。

## 7. 同步播放模型

当前后端模型：

- 权威模型是 `realtime.TimelineVector`。
- 权威运行时绑定是 `room.State`。
- 协议快照是 WebSocket `room_state`。
- Redis 中的 room_state 是 best-effort 最新快照缓存，不是权威来源。

`room.State` 当前包含：

- `roomId`
- `mediaId`
- `mediaDurationMs`
- `hostUserId`
- `paused`
- `ended`
- `positionMs`
- `velocity`
- `serverTimeMs`
- `playbackRate`
- `seq`
- `reason`

当前广播策略：

- 服务端只接受 `seq == current room_state.seq` 的控制事件；接受后更新权威 timeline，并递增 `seq`。
- 接受的控制事件会广播给房间内客户端。
- Android host 收到自己的控制确认后会更新本地 `latestRoomState.seq`，避免下一次控制继续沿用旧版本。
- `join_room`、房主断开、房主重连、显式 `room_state.request` 会下发 `room_state`。
- 当前不做高频周期性位置广播，客户端应基于服务器时间自行外推。

已确认的下一步需求：

- 建议后续把客户端控制请求中的 `seq` 迁移为更明确的 `expectedSeq`。
- 快速连续 seek 应按房主意图处理，但要避免广播风暴；可在客户端和服务端引入节流、合并或最后值优先策略。

## 8. WebSocket

当前能力：

- WebSocket 入口为 `/ws`。
- 使用 `github.com/coder/websocket`。
- 每个连接有独立写循环和出站队列。
- 出站队列有容量限制，`room_state` 支持 coalescing。
- 广播层有并发限制、超时、队列压力统计、慢客户端统计和超时关闭策略。
- 同一 `userId` 的房间活跃设备需要审批切换，未批准前旧设备仍是权威 active room device。
- `join_room` 命中已有活跃设备时，服务端先向旧设备发 `room_device.switch_request`，新设备先停在等待态。
- 服务端发送 `heartbeat`，客户端回 `heartbeat_ack`。
- 支持 `clock_sync.ping / clock_sync.pong`。
- 支持 `requestId` 控制事件去重，当前是进程内 sharded bounded map。

待实现或待加固：

- 慢客户端策略已进入实现：`room_state` 可合并，控制事件不合并；长期不可写、队列持续阻塞或心跳超时后关闭连接，客户端可通过 `room_state.request` 恢复。
- 需要补充面向 100+ 人房间的压测和指标。

## 9. 房主与控制权

最新业务规则：

- 不再做自动 host transfer。
- 房主身份来自创建房间的原始 owner。
- 房主断开后，房间时间轴暂停，`hostUserId` 置空，其他成员不能控制播放。
- 原房主在房间保留期间重连后，可以恢复房主身份。
- 房主控制必须来自当前 active room device，不能只凭 `userId` 通过。
- 同一账号允许多设备登录，但同步观影房间内只允许一个 active room device。
- 未来如果支持转移房主，必须是显式手动操作，不由断线自动触发。

当前实现状态：

- 服务端已实现房主断开后 `host_left`，暂停 timeline 并广播 `room_state`。
- 服务端已实现原 owner 重连后 `host_rejoin`。
- 历史 README 中出现的 “host transfer” 表述已过期。

## 10. 断线、重连与离开

最新业务规则：

- 已区分异常断线和主动离开房间。
- 异常断线进入 `grace_period`，宽限期 5 分钟。
- 如果用户在 5 分钟内回来，应恢复房间。
- 如果 5 分钟后没有任何人回来，房间清理。
- 只要房间还有一个在线或保留中的成员，就不应走最终清理。
- 如果最后一个用户是异常断开，房间也应暂时保留，进入 grace period。

当前实现状态：

- 内存 `room.Manager` 已有空房间 grace period 和 cleanup loop，默认宽限期已按最新规则调整为 5 分钟。
- PostgreSQL `rooms` 已有 `active / grace_period` 生命周期更新和过期清理。
- WebSocket 已新增 `leave_room` 主动离开协议；Android 正常关闭房间会先发送 `leave_room`，异常断线不会发送。
- 主动离开会将 PostgreSQL `room_members.is_active` 置为 false；如果房间因此为空，内存房间会立即销毁，不进入 grace period。
- 异常断线仍走连接清理；如果房间为空，则进入 5 分钟 grace period，期间重连会恢复房间。

## 11. HLS 媒体访问

当前能力：

- 本地开发可使用静态服务暴露 `media/` 下 HLS。
- `mediactl` 支持 `plan / build-hls / upload / write-db / ingest`。
- 支持 local / MinIO / S3-compatible 上传抽象。
- PostgreSQL 只保存媒体元数据和 URL，不保存视频二进制、m3u8 内容或 ts 分片。
- Android 只消费后端返回的 `mediaUrl / coverUrl`，不关心对象存储供应商。
- `mediaUrl` 已改为 `/media/playback/{episodeId}/master.m3u8?expires=...&sig=...` 短期签名播放入口；服务端校验后跳转到真实 HLS 地址。

已确认的安全方向：

- HLS 不能长期裸露为任意公开 URL。
- 不能让非平台用户通过分享公开 URL 随意使用媒体资源。
- Go 后端应负责鉴权并签发短期 signed URL 或 signed cookie。
- HLS 字节流应由 Nginx、对象存储或 CDN 分发，而不是 Go 业务服务长期承载。

当前阶段建议：

- 本地开发继续允许真实 HLS 地址由本地静态服务承载。
- 公网测试前补齐 playlist / segment 字节层访问授权。
- 优先评估对象存储 presign、CDN token auth 或 Nginx `secure_link`。

## 12. 观看进度

当前能力：

- Android 在暂停、播放结束、离开页面或低频 tick 时调用 `PUT /me/media-progress/{mediaItemId}`。
- 进度单位为秒，写入 `user_media_progress`。
- 首页用该数据展示上次观看和继续观看。

边界：

- 观看进度是用户个人业务数据。
- 观看进度不参与房间实时同步。
- 观看进度不能替代 WebSocket authority timeline。

## 13. 数据边界

| 数据 | 当前存储 | 说明 |
| --- | --- | --- |
| 用户、密码哈希、昵称 | PostgreSQL | 必须持久化 |
| 媒体 season / episode / tag | PostgreSQL | 必须持久化 |
| HLS 文件与封面二进制 | 本地文件 / 对象存储 / CDN | 不进入 PostgreSQL |
| 房间主数据 | PostgreSQL | 创建、加入、详情、生命周期 |
| 房间在线连接 | 进程内内存 | WebSocket runtime 状态 |
| 权威播放时间轴 | 进程内 `room.State` | 当前单实例权威 |
| 最新 `room_state` 快照 | Redis DB 0 best-effort cache | 不是权威 |
| 控制事件 requestId 去重 | 进程内 sharded map | 多实例前够用 |
| 观看进度 | PostgreSQL | 低频个人业务数据 |

## 14. 已确认但待实现的近期能力

- 控制请求 `seq` 字段后续改名为 `expectedSeq`。
- 持久化成员列表和在线成员列表的 UI 展示边界。
- 慢客户端策略压测。
- HLS 短期授权访问。
- 面向 100+ 人房间的广播、心跳、重连和 goroutine 泄漏测试。

## 15. 暂不优先的能力

- 自动房主转移。
- 私密房间密码。
- 聊天和弹幕。
- 手动房主转移。
- 多实例 WebSocket 房间调度。
- 消息队列。
- Kubernetes。
- 大型后台管理系统。

这些能力不是被否定，而是应在同步正确性、房间生命周期、WebSocket 可靠性和 HLS 鉴权稳定后再进入主线。
