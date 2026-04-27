# 服务端业务对象与 PostgreSQL 数据边界设计

> 目标：基于当前已经落地的房间服务实现，明确哪些对象属于业务主数据、哪些属于运行时同步对象，以及这些对象未来如何映射到 PostgreSQL 表结构。

## 当前结论

当前阶段已经确定：

- 数据库选择：`PostgreSQL`
- 本地 PostgreSQL 初始化方式：`server/compose.yaml + docker compose`
- 当前服务端同步主链路继续以**内存运行时对象**驱动
- 用户、房间、成员关系、媒体库等**业务主数据**进入 PostgreSQL
- authority timeline、websocket 连接、heartbeat 临时状态等**高频运行时状态**继续留在内存

这意味着接下来不是把当前 `Room / RoomManager / ClientConnection` 直接“硬落库”，而是要先建立一套稳定的数据边界。

---

## 1. 为什么不能直接把当前运行时对象映射成数据库表

当前服务端已经有这些核心运行时对象：

- [room.go](/Users/sunboy/Documents/my-projects/watch_together/server/internal/room/room.go)
- [manager.go](/Users/sunboy/Documents/my-projects/watch_together/server/internal/room/manager.go)
- [client.go](/Users/sunboy/Documents/my-projects/watch_together/server/internal/room/client.go)

它们现在负责的是：

- 房间内当前在线连接
- host transfer
- heartbeat timeout 后的断连清理
- authority timeline
- grace period 生命周期
- repeated join 的单有效连接收敛

这些状态有几个共同特点：

- 高频变化
- 强依赖当前内存时间线
- 直接服务于 WebSocket 实时同步
- 不适合现在就变成数据库里的“事实来源”

所以当前阶段更合理的做法是：

- **业务主数据**：持久化
- **运行时同步状态**：保留在内存

---

## 2. 业务对象分层

### 2.1 持久化业务对象

这些对象建议进入 PostgreSQL。

#### `User`

当前含义：

- 一个进入系统的用户身份

当前阶段用途：

- 作为 `Room.host_user_id` 的引用对象
- 作为 `RoomMember.user_id` 的引用对象
- 作为后续昵称、用户页、历史房间等功能的基础

建议字段：

- `id`
- `account`
- `password_hash`
- `nickname`
- `avatar_seed`
- `avatar_url`
- `bio`
- `created_at`
- `updated_at`

当前说明：

- 当前数据库模型需要先具备最小账号密码登录能力
- 密码字段不应保存明文，应保存 `password_hash`
- 后续匿名、OAuth、手机号等能力可以继续扩在这层之上
- 随着 `02 首页` 和后续个人中心页面推进，`User` 还会继续承载头像等资料信息

当前推荐头像与资料字段策略：

- `nickname`
  - 首页欢迎语直接使用
  - 个人中心也需要展示与编辑
- `avatar_seed`
  - 作为第一版默认头像来源
  - 即使没有上传头像，也能稳定生成一个可重复头像
- `avatar_url`
  - 作为后续远程头像能力的扩展字段
  - 第一版可以允许为空
- `bio`
  - 作为个人中心最轻量的资料扩展字段
  - 第一版可以允许为空

当前更推荐的第一版落地方式：

- 先同时保留 `avatar_seed` 与 `avatar_url`
- 页面展示逻辑优先：
  1. 有 `avatar_url` 时显示远程头像
  2. 否则使用 `avatar_seed` 生成默认头像

这样既能满足首页右上角头像和个人中心资料展示，又不会过早把实现绑死在文件上传方案上。

建议约束：

- `account`：唯一、非空
- `nickname`：非空
- `avatar_seed`：非空
- `avatar_url`：可空
- `bio`：可空

当前 migration 落地策略：

- `avatar_seed`：新增后统一回填，再设为非空
- `avatar_url`：可空，但若有值则不能为空白字符串
- `bio`：可空，但若有值则不能为空白字符串

### `User` 当前推荐设计

推荐字段草案：

- `id uuid pk`
- `account text unique not null`
- `password_hash text not null`
- `nickname text not null`
- `avatar_seed text not null`
- `avatar_url text null`
- `bio text null`
- `created_at timestamptz not null`
- `updated_at timestamptz not null`

语义说明：

- `avatar_seed`
  - 当前第一版推荐作为默认头像来源
  - 适合本地生成或第三方 avatar service
- `avatar_url`
  - 预留给后续用户上传头像或远程头像能力
- `bio`
  - 用于支撑个人中心最轻量资料展示

这样一来，首页与个人中心在第一版就可以共享同一组基础资料字段，而不需要等完整账号系统成熟后再回头补。

## 4. 当前新增 migration 落地情况

`User` 资料字段已经落为独立 migration：

- `server/migrations/20260421110000_add_user_profile_fields.up.sql`
- `server/migrations/20260421110000_add_user_profile_fields.down.sql`

当前落地内容包括：

- `users.avatar_seed`
- `users.avatar_url`
- `users.bio`

当前落地规则包括：

- `avatar_seed` 统一回填后设为 `not null`
- `avatar_seed` 不允许空白字符串
- `avatar_url` 可空，但若存在则不允许空白字符串
- `bio` 可空，但若存在则不允许空白字符串

这意味着首页与个人中心第一版需要的最小用户资料字段，现在已经有了明确的数据库承载结构。
#### `MediaItem`

当前含义：

- 可被房间选择和播放的视频内容

当前阶段用途：

- 为 `02A 选择视频` 提供内容来源
- 为房间创建时的 `media_item_id` 提供合法引用

建议字段：

- `id`
- `title`
- `original_title`
- `subtitle`
- `description`
- `cover_url`
- `media_url`
- `category`
- `tags`
- `production_team`
- `search_aliases`
- `season_label`
- `episode_label`
- `duration_ms`
- `status`
- `created_at`
- `updated_at`

当前说明：

- `media_url` 先指向 HLS 入口
- `title`、`original_title`、`production_team`、`search_aliases` 共同服务 `02A 选择视频` 的搜索框
- `tags` 和 `category` 可以继续作为媒体展示字段存在
- `subtitle` 第一版继续用于承载轻量副标题，例如“治愈冒险”“剧场版”“特别篇”
- `season_label` 用于展示季、篇章或系列分组文案
- `episode_label` 用于展示当前集数或单集文案，例如“第 09 集”“OVA 01”
- 当前阶段不需要先引入复杂的 CMS 设计

### `MediaItem` 播放展示元数据结论

`03 放映室` 需要展示“影片名称 + 当前第几集 + 播放进度”。当前更推荐先保持 `media_items` 为单个可播放媒体条目，并在 `media_items` 上补充轻量播放展示字段，而不是立即引入独立 episode 表。

当前结论：

- 第一版继续使用 `media_items.title` 作为主标题
- 第一版继续使用 `media_items.subtitle` 作为内容副标题
- 新增 `media_items.season_label` 作为季、篇章或系列分组展示文案
- 新增 `media_items.episode_label` 作为当前集数或单集展示文案
- 播放进度仍由 WebSocket runtime state 的 `positionMs` 提供
- 总时长展示使用 `media_items.duration_ms`
- 当前不新增 `media_episodes` 独立表

这样做的原因：

- 当前 `02A 选择视频` 选中的对象本质上仍是“一个可播放媒体”
- `03 放映室` 需要更清晰的季/集展示字段，但还不需要跨集导航
- `user_media_progress` 当前也是按 `user_id + media_item_id` 记录进度
- 先补轻量字段能支撑当前 UI，同时避免 episode 表把选片、建房、进度和同步链路一起变复杂

后续触发 schema 升级的信号：

- 需要在同一部作品下展示多集列表
- 需要支持下一集自动跳转
- 需要区分系列、季、集、OVA、剧场版
- 需要按“番剧作品”聚合用户观看进度
- 需要对单集分别保存播放源、封面、时长或上线状态

如果出现这些信号，再考虑引入：

- `media_series`：作品或系列层级
- `media_seasons`：季或篇章层级
- `media_episodes`：单集可播放条目

当前这部分已经落为独立 migration：

- `server/migrations/20260426104000_add_media_playback_display_fields.up.sql`
- `server/migrations/20260426104000_add_media_playback_display_fields.down.sql`

当前落地内容包括：

- `media_items.season_label`
- `media_items.episode_label`

当前落地规则包括：

- `season_label` 可空，但若存在则不允许空白字符串
- `episode_label` 可空，但若存在则不允许空白字符串

#### `MediaTag`（下一阶段候选）

当前含义：

- 服务端可稳定返回的媒体标签目录

当前阶段用途：

- 支撑 `02A 选择视频` 页面的默认 5 个标签
- 支撑 `更多` 中展开的全部标签列表
- 为后续标签排序、显隐和数量控制提供事实来源

建议字段：

- `id`
- `name`
- `slug`
- `sort_order`
- `is_featured`
- `is_active`
- `created_at`
- `updated_at`

当前说明：

- `is_featured = true` 可以直接支撑首屏默认主标签
- `sort_order` 用于保证标签稳定顺序
- `is_active` 用于后续下线标签而不破坏历史关联

#### `MediaItemTag`（下一阶段候选）

当前含义：

- 媒体内容与标签目录之间的关联关系

当前阶段用途：

- 根据标签筛选 `media_items`
- 保证标签查询不再只依赖 `media_items.tags jsonb`

建议字段：

- `media_item_id`
- `media_tag_id`
- `created_at`

当前说明：

- 如果后续进入更稳定的数据阶段，推荐用 `media_tags + media_item_tags` 逐步取代“只靠 `media_items.tags jsonb`”
- 这样更适合：
  - 默认主标签
  - 最多 20 个全部标签
  - 标签稳定排序
  - 标签显隐控制

#### `UserMediaProgress`（下一阶段高优先级候选）

当前含义：

- 用户针对某个媒体内容的观看进度

当前阶段用途：

- 支撑 `02 首页` 的“上次观看”
- 支撑 `02 首页` 的“继续追番”
- 为后续 resume playback 提供数据基础

建议字段：

- `id`
- `user_id`
- `media_item_id`
- `last_position_seconds`
- `duration_seconds`
- `last_watched_at`
- `completed`
- `completion_source`
- `updated_at`

当前说明：

- 当前第一版基础 schema 之外，已经新增独立 migration 负责这张表
- 它已经成为首页业务第一批真实依赖的数据模型
- 进度精度建议先到秒级，而不是毫秒级

建议补充约束：

- 同一 `user_id + media_item_id` 只保留一条当前进度记录
- `last_position_seconds >= 0`
- `duration_seconds > 0`
- `last_position_seconds <= duration_seconds`
- `completed = true` 时，`last_position_seconds` 应接近或等于 `duration_seconds`

建议索引：

- `unique(user_id, media_item_id)`
- `(user_id, last_watched_at desc)`：支撑“上次观看”
- `(user_id, completed, last_watched_at desc)`：支撑“继续追番”

`completion_source` 当前建议先收敛为文本枚举，例如：

- `ended`
- `manual_mark`
- `threshold_auto`

第一版也可以先不做这列，但如果希望后续区分“自然看完”和“手动标记已看完”，保留它会更稳。

## 3. 当前由 `02A 选择视频` 推动出的 schema 扩展建议

基于当前业务描述，`02A 选择视频` 已经明确推动出两类新的数据层需求：

### 3.1 `media_items` 的检索字段扩展

当前更推荐补充：

- `original_title`
- `production_team`
- `search_aliases`

建议语义：

- `original_title`
  - 用于日文/英文原始标题或别名展示
- `production_team`
  - 用于工作团队、制作公司或主要制作信息检索
- `search_aliases`
  - 用于保存额外可命中的别名数组

当前这部分已经落为独立 migration：

- `server/migrations/20260421130000_add_media_search_fields.up.sql`
- `server/migrations/20260421130000_add_media_search_fields.down.sql`

当前落地内容包括：

- `media_items.original_title`
- `media_items.production_team`
- `media_items.search_aliases`

当前落地规则包括：

- `original_title` 可空，但若存在则不允许空白字符串
- `production_team` 可空，但若存在则不允许空白字符串
- `search_aliases` 为 `jsonb` 数组，默认值为 `[]`

当前落地索引包括：

- `idx_media_items_original_title`
- `idx_media_items_production_team`
- `idx_media_items_search_aliases_gin`

### 3.2 标签目录模型

当前更推荐新增：

- `media_tags`
- `media_item_tags`

这样可以稳定支撑：

- 默认主标签 5 个
- `更多` 中全部标签最多 20 个
- 标签排序、显隐和后续运营控制

当前这部分已经落为独立 migration：

- `server/migrations/20260421143000_add_media_tags.up.sql`
- `server/migrations/20260421143000_add_media_tags.down.sql`

当前落地内容包括：

- `media_tags`
- `media_item_tags`

当前落地规则包括：

- `media_tags.slug` 唯一，且必须为小写非空白字符串
- `media_tags.sort_order >= 0`
- `media_item_tags` 使用 `(media_item_id, media_tag_id)` 作为联合主键
- `media_item_tags` 通过外键与 `media_items`、`media_tags` 关联

当前落地索引包括：

- `idx_media_tags_active_sort`
- `idx_media_tags_featured_active_sort`
- `idx_media_item_tags_media_tag_id`

#### `Room`

当前含义：

- 一个放映室，是服务端的核心业务主实体

当前阶段用途：

- 记录房间主数据
- 作为成员关系和媒体选择的主引用对象
- 作为后续生命周期状态的业务事实来源

建议字段：

- `id`
- `room_code`
- `host_user_id`
- `media_item_id`
- `status`
- `created_at`
- `updated_at`
- `last_empty_at`
- `destroy_after`

当前说明：

- `status` 建议先支持：`active / grace_period / destroyed`
- `host_user_id` 是当前业务语义里的 host
- grace period 相关字段要能支撑“最后一个成员离开后保留 2 分钟”

#### `RoomMember`

当前含义：

- 用户和房间之间的成员关系

当前阶段用途：

- 表达谁属于这个房间
- 区分 host / member 角色
- 支撑 repeated join / reconnect 的业务恢复语义

建议字段：

- `id`
- `room_id`
- `user_id`
- `role`
- `joined_at`
- `left_at`
- `is_active`

当前说明：

- 当前角色先收敛为：`host / member`
- 这里记录的是业务成员关系，不是 websocket 连接
- repeated join 时应该优先恢复已有 active 成员，而不是不断新插入记录

---

### 2.2 运行时同步对象

这些对象当前继续以内存存在，不建议直接设计成第一版主表。

#### `RoomRuntimeState`

当前对应关系：

- `room.Room.state`
- `room.Room.authorityAt`

当前内容：

- `room_id`
- `seq`
- `position_ms`
- `paused`
- `ended`
- `playback_rate`
- `authority_updated_at`

当前说明：

- 这部分是 authority timeline
- 当前最适合继续留在内存
- 它服务的是实时同步，不是长期事实存储

#### `ClientConnection`

当前对应关系：

- `room.ClientConnection`

当前内容：

- `connection_id`
- `room_id`
- `user_id`
- `connected_at`
- `last_heartbeat_at`

当前说明：

- 这是 websocket 运行时连接
- 不能等价于业务成员关系
- 当前阶段不建议落为正式业务表

#### `RoomManagerLifecycleState`

当前对应关系：

- `room.Manager.rooms`
- `room.Manager.emptySince`

当前内容：

- 房间注册表
- 房间 empty grace period 计时

当前说明：

- 这部分当前也更适合留在内存
- 后续如果要做服务重启恢复，再考虑是否需要部分持久化

---

## 3. PostgreSQL 持久化边界

### 当前建议落库

第一版建议持久化：

- `users`
- `media_items`
- `rooms`
- `room_members`

原因：

- 这些对象具备明确业务含义
- 需要稳定主键和关系约束
- 会直接被页面业务、房间恢复和媒体选择流程复用

### 下一轮高优先级候选

随着 `02 首页与加入房间` 的业务明确，下一轮最值得进入 PostgreSQL 的候选对象是：

- `user_media_progress`

原因：

- 首页已经明确需要“上次观看”
- 首页已经明确需要“继续追番”
- 这两块都要求用户维度的持久化观看进度
- 这条对象现在已经正式进入 migration 落地阶段

### `user_media_progress` 当前推荐设计

当前更推荐把它设计成“每个用户、每个媒体一条当前进度记录”，而不是事件流水表。

原因：

- 首页只需要当前状态，不需要完整回放历史
- `上次观看` 本质是最近一次更新的当前进度
- `继续追番` 本质是“未完成内容”的当前进度列表
- 这类读场景更适合状态表，而不是先上 event sourcing

推荐字段草案：

- `id uuid pk`
- `user_id uuid not null references users(id)`
- `media_item_id uuid not null references media_items(id)`
- `last_position_seconds integer not null`
- `duration_seconds integer not null`
- `last_watched_at timestamptz not null`
- `completed boolean not null default false`
- `completion_source text null`
- `created_at timestamptz not null`
- `updated_at timestamptz not null`

推荐唯一约束：

- `unique(user_id, media_item_id)`

推荐索引：

- `index on (user_id, last_watched_at desc)`
- `index on (user_id, completed, last_watched_at desc)`

语义说明：

- `last_position_seconds`
  - 首页展示只需要秒级
  - 后续放映室恢复播放时，也可以先基于秒级恢复
- `last_watched_at`
  - 用来判断“最近一次观看”
  - 也能给“继续追番”排序
- `completed`
  - 直接决定该条记录是否进入“继续追番”
- `completion_source`
  - 可选
  - 用来区分为什么被标记为完成

### 查询语义建议

#### 上次观看

查询逻辑建议：

- 从 `user_media_progress`
- 按 `last_watched_at desc`
- 取该用户最近一条
- join `media_items`

返回字段建议：

- `media_items.id`
- `media_items.title`
- `media_items.cover_url`
- `user_media_progress.last_position_seconds`
- `user_media_progress.duration_seconds`

#### 继续追番

查询逻辑建议：

- 从 `user_media_progress`
- 过滤 `completed = false`
- 按 `last_watched_at desc`
- 取前 2 条
- join `media_items`

返回字段建议：

- `media_items.id`
- `media_items.title`
- `media_items.cover_url`
- `user_media_progress.last_position_seconds`
- `user_media_progress.duration_seconds`

### Completed 判断建议

当前建议把“是否看完”先收成简单规则，不和播放器 ended 事件强耦合：

- 服务端持久化层直接使用 `completed boolean`
- 应用层可按以下方式更新它：
  - 收到明确的 ended / completed 语义时置为 `true`
  - 手动标记已看完时置为 `true`
  - 若后续 seek 到较早位置继续观看，可重新置回 `false`

这样首页查询逻辑会非常直接，也更便于后续继续演进。

## 5. 当前新增 migration 落地情况

`user_media_progress` 已经落为独立 migration：

- `server/migrations/20260421100000_add_user_media_progress.up.sql`
- `server/migrations/20260421100000_add_user_media_progress.down.sql`

当前落地内容包括：

- `user_id` / `media_item_id` 外键
- 秒级进度字段：
  - `last_position_seconds`
  - `duration_seconds`
- `last_watched_at`
- `completed`
- `completion_source`
- `created_at`
- `updated_at`
- `unique(user_id, media_item_id)`
- 首页查询所需索引：
  - `(user_id, last_watched_at desc)`
  - `(user_id, completed, last_watched_at desc)`

这意味着首页“上次观看”和“继续追番”现在已经有了明确的数据库承载结构。

### 当前不建议落库

第一版不建议持久化：

- websocket 连接
- heartbeat 临时状态
- authority timeline 的实时推进值
- drift correction 的本地/远端观测值

原因：

- 这些状态变化频率高
- 生命周期短
- 当前主要服务实时同步
- 过早落库会让同步主链路复杂化

---

## 4. 推荐的表关系

### 4.1 主关系

- `users (1) -> (n) room_members`
- `rooms (1) -> (n) room_members`
- `media_items (1) -> (n) rooms`
- `users (1) -> (n) rooms(host_user_id)`

### 4.2 含义解释

- `Room` 表达放映室本身
- `RoomMember` 表达某个用户是否在这个放映室里
- `Room.host_user_id` 表达当前业务 host
- `Room.media_item_id` 表达当前房间选中了哪部视频

### 4.3 关键约束

建议第一版至少明确这些业务约束：

- 一个房间同一时刻只有一个 `host_user_id`
- 一个用户在同一房间内最多保留一个 active 成员关系
- repeated join 优先恢复已有 active 成员关系
- former host 在 host transfer 后重连时回到普通成员

### 4.4 `03 放映室` 页面数据边界

`03 放映室` 页面会同时使用业务主数据和运行时同步状态。

业务主数据来自 PostgreSQL：

- `rooms.room_code`：右上角 6 位房间码，Android 后续可点击复制
- `rooms.media_item_id`：当前房间选中的影片
- `rooms.host_user_id`：当前业务房主
- `room_members`：当前房间成员关系
- `users.nickname / avatar_seed / avatar_url`：房主和成员展示信息
- `media_items.title / subtitle / season_label / episode_label / media_url / duration_ms`：影片标题、播放展示文案、播放地址和总时长

运行时同步状态继续来自 WebSocket authority state：

- `positionMs`
- `seq`
- `playbackRate`
- `paused`
- `ended`
- 在线连接、heartbeat 和 host transfer 结果

当前 schema 对第一版 `03 放映室` 已经基本够用，不需要为了播放器实时状态新增表。

当前影片播放展示模型：

- 第一版使用 `media_items.season_label` 和 `media_items.episode_label` 展示季/集信息
- 当前不新增 `media_episodes` 独立表
- 如果后续需要支持系列、季、集、下一集自动跳转，再评估独立 episode 表

当前接口落地状态：

- `POST /rooms` 已使用 PostgreSQL 写入 `rooms` 和 host 的 `room_members`
- `POST /rooms/{roomCode}/join` 已使用 PostgreSQL 查询 `rooms.room_code` 并写入或恢复 `room_members`
- HTTP API 响应中的 `room.id` 是数据库 UUID
- HTTP API 响应中的 `room.roomCode` 是 6 位分享码
- 当前 WebSocket `join_room.payload.roomId` 仍使用 `room.roomCode`，避免把实时同步链路和数据库 UUID 强耦合
- 后续如果要让 WebSocket 改用数据库 UUID，需要单独做协议迁移任务，而不是在业务 API 中隐式切换

---

## 5. 第一版表草案

### `users`

建议字段：

- `id uuid primary key`
- `account text not null unique`
- `password_hash text not null`
- `nickname text not null`
- `created_at timestamptz not null`
- `updated_at timestamptz not null`

### `media_items`

建议字段：

- `id uuid primary key`
- `title text not null`
- `subtitle text null`
- `season_label text null`
- `episode_label text null`
- `description text null`
- `cover_url text null`
- `media_url text not null`
- `category text null`
- `tags jsonb not null default '[]'`
- `duration_ms bigint null`
- `status text not null`
- `created_at timestamptz not null`
- `updated_at timestamptz not null`

说明：

- `subtitle` 用于媒体副标题或简介式补充文案
- `season_label` 用于季、篇章或系列分组展示
- `episode_label` 用于当前集数或单集展示
- `tags` 已逐步从 `jsonb` 过渡到 `media_tags + media_item_tags` 关系模型

### `rooms`

建议字段：

- `id uuid primary key`
- `room_code text unique not null`
- `host_user_id uuid not null references users(id)`
- `media_item_id uuid not null references media_items(id)`
- `status text not null`
- `created_at timestamptz not null`
- `updated_at timestamptz not null`
- `last_empty_at timestamptz null`
- `destroy_after timestamptz null`

说明：

- `destroy_after` 可以直接支持 grace period 清理逻辑
- 当前阶段不急着引入更多房间统计字段

### `room_members`

建议字段：

- `id uuid primary key`
- `room_id uuid not null references rooms(id)`
- `user_id uuid not null references users(id)`
- `role text not null`
- `joined_at timestamptz not null`
- `left_at timestamptz null`
- `is_active boolean not null default true`

说明：

- `left_at` 和 `is_active` 一起表达历史成员关系与当前成员关系
- 未来可以加部分唯一索引，确保同一房间同一用户最多一个 active 记录

当前这一点已经在第一版 migration 中实现为部分唯一索引：

- `uniq_room_members_active_user_per_room`

---

## 6. 当前第一版 schema 落地情况

当前第一版 schema 已经落为 migration 文件：

- `server/migrations/20260420113000_create_initial_schema.up.sql`
- `server/migrations/20260420113000_create_initial_schema.down.sql`
- `server/migrations/20260420123000_add_account_fields_to_users.up.sql`
- `server/migrations/20260420123000_add_account_fields_to_users.down.sql`

当前已经实现的内容包括：

- `users / media_items / rooms / room_members` 四张主表
- `users.account / users.password_hash` 登录字段扩展
- 主要外键关系
- 房间码唯一约束
- `room_members` active 成员关系的部分唯一索引
- `media_items.tags` 的 GIN 索引

这意味着当前数据设计文档已经不再只是草案，而是和首个 schema migration 对齐。

---

## 7. 当前不进入第一版表的设计

下面这些内容，当前明确不进入第一版 schema：

- `room_runtime_state`
- `client_connections`
- `heartbeat_events`
- `drift_correction_history`
- `room_events`

理由：

- 现在还没有强到需要做事件溯源
- 运行时同步主链路已经能在内存里工作
- 当前优先目标是建立主数据边界，而不是提前把所有运行时痕迹落库

---

## 8. 推荐任务拆分

基于这份设计，下一步更适合拆成以下几类任务：

### A. 设计与边界

- 明确 PostgreSQL 为当前服务端数据库
- 固化持久化对象与运行时对象边界
- 固化第一版表关系与关键约束

### B. Schema 与 Migration

- 在 `server/` 内引入 SQL-first migration 机制
- 建立 `users / media_items / rooms / room_members` 第一版 schema
- 准备本地开发数据库初始化流程

当前其中第一步已经具备基础设施：

- `server/compose.yaml`
- `server/migrations/`
- `server/scripts/new_migration.sh`
- `server/scripts/migrate.sh`
- `server/Makefile`

当前本地 PostgreSQL 初始化也已经有统一入口：

- `cd server && docker compose up -d`
- 默认数据库：`anime_watch_dev`
- 默认连接串：`postgres://app:app@127.0.0.1:5432/anime_watch_dev?sslmode=disable`

接下来可以直接进入 schema 与本地 PostgreSQL 初始化的实现阶段

### C. 服务端接入

- 先接入媒体库查询
- 再接入建房时的数据库读写
- 房间运行时同步逻辑继续保留内存实现

---

## 9. 当前建议

当前最合理的推进顺序是：

1. 先固化这份数据边界设计
2. 再补一组 Linear 任务
3. 再做 `server/` 下的第一版 PostgreSQL schema 和 migration 机制

这样可以避免后面一边改 schema、一边还在讨论“哪些该落库、哪些不该落库”。
