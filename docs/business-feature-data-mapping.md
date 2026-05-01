# 业务功能与数据映射

> 目标：把 Android 页面业务、后端 API、PostgreSQL 表和内存同步状态之间的关系整理清楚，避免 UI 开发和 schema 设计脱节。

## 当前数据模型边界

当前业务主数据使用 episode-backed 媒体模型：

- 用户资料：`users`
- 首页观看历史：`user_media_progress + media_episodes + media_seasons`
- 选择视频：`media_seasons + media_episodes + media_tags + media_season_tags`
- 创建/加入房间：`rooms + room_members`
- 放映室业务首屏：`rooms + room_members + users + media_episodes + media_seasons`
- 实时同步：服务端内存房间状态 + WebSocket

旧的扁平媒体表已经从目标模型中移除，后续功能不再围绕旧表设计。

## 01 登录页

业务功能：

- 用户输入账号和密码。
- 登录成功后进入 `02 首页与加入房间`。
- 登录失败展示轻量错误提示。

数据来源：

- `POST /auth/login`
- `users.account`
- `users.password_hash`
- `users.nickname`
- `users.avatar_seed`
- `users.avatar_url`

说明：

- 登录框不输入昵称。
- 昵称属于注册或个人资料维护阶段。

## 02 首页与加入房间

业务功能：

- 顶部展示 `晚安, xx`，其中 `xx` 来自当前用户昵称。
- 右上角头像后续进入个人中心。
- `上次观看` 展示最近一次观看的标题、封面和秒级进度。
- `继续追番` 展示最近未完成的两个 episode。
- 点击 `创建放映室` 进入 `02A 选择视频`。
- 点击 `加入房间` 弹出房间码输入框。

数据来源：

- `GET /home/summary`
- `users`
- `user_media_progress`
- `media_episodes`
- `media_seasons`

关键字段：

- `users.nickname`
- `users.avatar_seed`
- `users.avatar_url`
- `user_media_progress.last_position_seconds`
- `user_media_progress.duration_seconds`
- `user_media_progress.last_watched_at`
- `media_episodes.id`
- `media_episodes.title`
- `media_episodes.cover_url`
- `media_episodes.episode_label`
- `media_seasons.title`
- `media_seasons.cover_url`
- `media_seasons.season_label`

## 02A 选择视频

业务功能：

- 由首页点击 `创建放映室` 进入。
- 顶部搜索框按标题、制作团队、搜索别名、简介等信息检索。
- 默认展示最多 5 个 featured 标签。
- 点击 `更多` 展开悬浮标签列表，最多展示 20 个 active 标签。
- 选中标签后筛选满足条件的 episode 列表。
- 底部固定操作栏展示已选中影片，并提供 `创建房间`。
- 点击 `创建房间` 后进入 `03 放映室`。

数据来源：

- `GET /media/tags`
- `GET /media/items?query=&tag=&limit=&cursor=`
- `POST /rooms`

关键表：

- `media_tags`
- `media_season_tags`
- `media_seasons`
- `media_episodes`
- `rooms`
- `room_members`

关键字段：

- `media_tags.slug`
- `media_tags.name`
- `media_tags.is_featured`
- `media_tags.is_active`
- `media_seasons.title`
- `media_seasons.original_title`
- `media_seasons.production_team`
- `media_seasons.search_aliases`
- `media_seasons.cover_url`
- `media_episodes.id`
- `media_episodes.title`
- `media_episodes.subtitle`
- `media_episodes.description`
- `media_episodes.media_url`
- `media_episodes.duration_ms`
- `media_episodes.episode_label`

说明：

- `GET /media/items` 返回的 `items[].id` 是 `media_episodes.id`。
- `POST /rooms` 请求体字段名暂时仍为 `mediaItemId`，但值必须是 `media_episodes.id`。

## 03 放映室

业务功能：

- 由选择视频页创建房间后进入，也可以由加入房间流程进入。
- 右上角展示 6 位房间码，后续可点击复制。
- 顶部展示放映室名称和连接状态。
- 视频播放器占满横向宽度并保持常用比例。
- 播放器下方展示影片名称、当前集数和播放进度。
- 展示房主、成员和当前播放倍速。
- 播放、暂停、seek、倍速选择等同步控制通过播放器浮层触发。
- 底部展示同步播放器的功能说明和当前状态。

业务主数据来源：

- `GET /rooms/{roomCode}`
- `rooms`
- `room_members`
- `users`
- `media_episodes`
- `media_seasons`

实时数据来源：

- WebSocket `/ws`
- `join_room`
- `room_state`
- `play`
- `pause`
- `seek`
- `set_playback_rate`
- `ended`
- `heartbeat`

关键字段：

- `rooms.room_code`
- `rooms.host_user_id`
- `rooms.media_episode_id`
- `room_members.user_id`
- `room_members.role`
- `users.nickname`
- `users.avatar_seed`
- `users.avatar_url`
- `media_episodes.id`
- `media_episodes.title`
- `media_episodes.media_url`
- `media_episodes.duration_ms`
- `media_episodes.episode_label`
- `media_seasons.title`
- `media_seasons.season_label`

运行时同步字段：

- `positionMs`
- `playbackRate`
- `paused`
- `ended`
- `seq`

说明：

- PostgreSQL 不保存实时播放 authority state。
- `GET /rooms/{roomCode}` 只负责业务首屏。
- 播放进度、倍速、暂停/播放状态以 WebSocket `room_state` 为准。

## 观看进度

业务功能：

- Android 在暂停、播放结束、离开页面或约 30 秒低频 tick 时上报进度。
- 首页使用该数据展示上次观看和继续追番。

数据来源：

- `PUT /me/media-progress/{mediaItemId}`
- `user_media_progress`

关键字段：

- `user_media_progress.user_id`
- `user_media_progress.media_episode_id`
- `user_media_progress.last_position_seconds`
- `user_media_progress.duration_seconds`
- `user_media_progress.completed`
- `user_media_progress.completion_source`
- `user_media_progress.last_watched_at`

说明：

- 路径参数 `{mediaItemId}` 当前语义是 `media_episodes.id`。
- 进度单位为秒，不保存毫秒。
- 该接口不参与实时同步，不替代 WebSocket authority state。

## mediactl 导入链路

业务功能：

- 将本地源视频转为 HLS。
- 根据媒体库目录结构推导 `source_key`。
- 根据源文件内容计算 `source_hash`。
- 写入 season、episode 和 tag 关系。

数据写入：

- `media_seasons`
- `media_episodes`
- `media_tags`
- `media_season_tags`

关键字段：

- `media_seasons.slug`
- `media_seasons.title`
- `media_seasons.search_aliases`
- `media_episodes.source_key`
- `media_episodes.source_hash`
- `media_episodes.media_url`
- `media_episodes.duration_ms`

说明：

- `source_key` 不由用户手动输入。
- `source_hash` 不由用户手动输入。
- 两者都由 CLI 基于规范目录和源文件自动生成。
