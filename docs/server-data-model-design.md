# 服务端业务对象与 PostgreSQL 数据边界设计

> 目标：明确当前服务端业务主数据、媒体模型和运行时同步状态之间的边界。当前数据库已经收敛为 episode-backed 媒体模型，不再使用旧的扁平 `media_items` 表。

## 当前结论

- 数据库选择：`PostgreSQL`
- 本地 PostgreSQL 初始化方式：`server/compose.yaml + docker compose`
- schema 变更方式：SQL-first migration，使用 `golang-migrate`
- 媒体主模型：`media_seasons + media_episodes`
- 标签关系：`media_tags + media_season_tags`
- 房间业务关系：`rooms + room_members`
- 观看历史：`user_media_progress`
- WebSocket 实时同步状态继续保留在服务端内存中

## 持久化业务主数据

### `users`

用途：

- 账号密码登录
- 首页欢迎语
- 头像与个人中心基础资料
- 房间房主和成员引用

核心字段：

- `id`
- `account`
- `password_hash`
- `nickname`
- `avatar_seed`
- `avatar_url`
- `bio`
- `created_at`
- `updated_at`

### `media_seasons`

用途：

- 表达一季、篇章、合集或作品容器
- 承载偏作品/季的信息
- 作为搜索和标签筛选的主要聚合对象

核心字段：

- `id`
- `slug`
- `title`
- `original_title`
- `description`
- `cover_url`
- `category`
- `production_team`
- `search_aliases`
- `season_number`
- `season_label`
- `sort_order`
- `status`
- `created_at`
- `updated_at`

说明：

- 当前不引入 `media_series`。
- 标签挂在 season 层，避免同一季下每一集重复维护标签。
- 搜索会命中 `title / description / original_title / production_team / search_aliases`。

### `media_episodes`

用途：

- 表达真正可播放的一集或一个视频资源
- 承载 HLS 地址、时长、集数和源文件身份
- 作为创建房间、房间详情和观看进度的媒体引用对象

核心字段：

- `id`
- `season_id`
- `title`
- `subtitle`
- `description`
- `cover_url`
- `media_url`
- `duration_ms`
- `episode_number`
- `episode_label`
- `source_key`
- `source_hash`
- `sort_order`
- `status`
- `created_at`
- `updated_at`

说明：

- `source_key` 由媒体库相对路径自动推导，例如 `violet-evergarden/season-01/episode-09.mp4`。
- `source_hash` 由源文件内容计算，用于识别源文件是否变化。
- `media_url` 指向 HLS `master.m3u8` 的可播放公开 URL。
- 一集可以拥有多个清晰度 variant，具体 variant URL 不重复塞进 `media_episodes`，而是进入 `media_episode_variants`。

### `media_episode_variants`

用途：

- 记录同一个 `media_episodes` 下的多码率播放资源
- 让 `mediactl` 生成 `720p / 1080p` 后可以把每个 variant 的 URL、分辨率、带宽写入数据库
- 支撑后续播放端按网络、设备能力、倍速播放压力选择更合适的清晰度

核心字段：

- `id`
- `media_episode_id`
- `variant_key`
- `label`
- `playlist_url`
- `width`
- `height`
- `bandwidth_bps`
- `codecs`
- `is_default`
- `sort_order`
- `created_at`
- `updated_at`

说明：

- `variant_key` 当前支持 `720p / 1080p`。
- 默认不生成 4K；大多数项目资源没有 4K，且同步播放器当前优先稳定 720p 以上体验。
- `media_episodes.media_url` 仍作为 Android 播放入口，指向 master playlist。
- `media_episode_variants.playlist_url` 指向具体 variant 的 `index.m3u8`。

### `media_tags`

用途：

- 选择视频页默认标签
- 点击“更多”后的展开标签列表
- 搜索筛选条件

核心字段：

- `id`
- `slug`
- `name`
- `sort_order`
- `is_featured`
- `is_active`
- `created_at`
- `updated_at`

### `media_season_tags`

用途：

- 建立 `media_seasons` 与 `media_tags` 的多对多关系
- 支撑 `GET /media/items?tag=<slug>` 标签筛选

核心字段：

- `season_id`
- `media_tag_id`
- `created_at`

### `rooms`

用途：

- 保存房间业务主数据
- 保存 6 位房间码
- 保存房主和当前选中的 episode
- 记录房间生命周期状态

核心字段：

- `id`
- `room_code`
- `host_user_id`
- `media_episode_id`
- `status`
- `last_empty_at`
- `destroy_after`
- `created_at`
- `updated_at`

说明：

- `room_code` 是用户分享和加入房间时使用的 6 位码。
- `media_episode_id` 是房间选中的可播放资源。
- WebSocket `join_room.payload.roomId` 当前仍使用 `room_code` 作为运行时房间 key。

### `room_members`

用途：

- 保存用户是否加入过某个房间
- 保存房间业务角色
- 支撑重复加入、离开后恢复成员关系等业务流程

核心字段：

- `id`
- `room_id`
- `user_id`
- `role`
- `joined_at`
- `left_at`
- `is_active`

说明：

- `room_members` 不等同于 WebSocket 在线连接。
- 在线连接、心跳、host transfer 的即时状态仍由内存房间模型维护。

### `user_media_progress`

用途：

- 保存用户对某个 episode 的低频观看进度
- 支撑首页“上次观看”和“继续追番”

核心字段：

- `id`
- `user_id`
- `media_episode_id`
- `last_position_seconds`
- `duration_seconds`
- `last_watched_at`
- `completed`
- `completion_source`
- `created_at`
- `updated_at`

说明：

- 进度以秒为单位，不保存毫秒。
- 该表不参与实时播放同步。
- WebSocket authority state 仍是实时 `positionMs / paused / playbackRate / ended / seq` 的权威来源。

## 当前目标关系

核心关系：

- `users (1) -> (n) rooms`
- `users (1) -> (n) room_members`
- `users (1) -> (n) user_media_progress`
- `rooms (1) -> (n) room_members`
- `media_seasons (1) -> (n) media_episodes`
- `media_episodes (1) -> (n) media_episode_variants`
- `media_episodes (1) -> (n) rooms`
- `media_episodes (1) -> (n) user_media_progress`
- `media_seasons (n) -> (n) media_tags`，通过 `media_season_tags`

可视化关系见：

- [database-relationship-uml.puml](./database-relationship-uml.puml)

## 不进入 PostgreSQL 的运行时状态

以下状态当前继续留在服务端内存中：

- WebSocket 连接对象
- 当前在线连接状态
- 当前房间 authority timeline
- `positionMs`
- `playbackRate`
- `paused`
- `ended`
- `seq`
- heartbeat 最近活跃时间
- host transfer 的即时决策
- drift correction 观测值

这些状态高频变化，直接服务实时同步，不适合作为 PostgreSQL 的第一阶段事实来源。

## 旧模型清理状态

旧的扁平媒体模型已经被 cleanup migration 清理：

- 删除 `media_items`
- 删除 `media_item_tags`
- 删除 `rooms.media_item_id`
- 删除 `user_media_progress.media_item_id`
- 删除 `media_episodes.legacy_media_item_id`

后续新增服务端 API、Android 对接和 `mediactl` 能力，都应以 `media_episodes.id` 作为可播放资源 id。
