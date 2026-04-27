# 业务功能与数据需求映射

> 目标：记录“业务功能需要什么信息、这些信息从哪里来、是否需要落库或查询”，帮助后续随着 UI 和业务流程推进，持续更新 PostgreSQL schema 与服务端接口设计。

---

## 使用方式

这份文档不是页面视觉稿，也不是数据库 schema 文档。

它解决的是中间这层问题：

- 某个业务页面或交互到底需要哪些数据
- 这些数据是用户输入、服务端运行时状态，还是数据库主数据
- 页面提交后，服务端要检索哪些表
- 当前阶段哪些能力已经需要落库，哪些还可以先保留为运行时内存态

后续每当出现新的真实业务页面、表单或服务端能力时，都应该先补这里，再决定要不要新增 migration。

后端 HTTP API 的统一响应格式、错误格式和前后端联调规则见 [backend-api-contract.md](./backend-api-contract.md)。

---

## 当前数据来源分类

### 1. 用户输入

由页面表单直接收集，例如：

- 账号
- 密码
- 房间码
- 搜索关键词
- 标签筛选条件

### 2. 业务主数据

应进入 PostgreSQL，作为长期事实来源，例如：

- `users`
- `media_items`
- `rooms`
- `room_members`

### 3. 运行时同步状态

当前仍以内存为主，不作为第一版 schema 的主来源，例如：

- authority state
- websocket connection
- heartbeat 最近状态
- 当前在线成员连接

---

## 功能映射总览

| 业务功能 | 页面/入口 | 主要输入 | 主要读取 | 主要写入 | 当前建议 |
| --- | --- | --- | --- | --- | --- |
| 账号登录 | `01 登录页` 登录弹窗 | `account`、`password` | `users.account`、`users.password_hash` | 登录态暂不落库 | 需要数据库查询 |
| 账号注册 | 后续注册页 | `account`、`password`、`nickname` | `users.account` 唯一性校验 | `users` | 需要数据库写入 |
| 首页欢迎信息 | `02 首页` | 当前登录用户 | `users.nickname`、用户头像字段 | 无 | 需要数据库查询 |
| 首页上次观看 | `02 首页` | 当前登录用户 | 用户观看进度、媒体封面、媒体标题、上次观看秒级进度 | 用户观看进度 | 需要数据库读写 |
| 首页创建放映室 | `02 首页` | 当前登录用户 | `users`、默认媒体或选片结果 | `rooms`、`room_members` | 需要数据库写入 |
| 输入房间码加入 | `02 首页` | `room_code` | `rooms`、`room_members` | `room_members` | 需要数据库读写 |
| 继续追番列表 | `02 首页` | 当前登录用户 | 用户未看完内容、媒体封面、媒体标题、当前追番进度 | 用户观看进度 | 需要数据库读写 |
| 搜索视频 | `02A 选择视频` | 搜索词、标签 | `media_items` | 无 | 需要数据库查询 |
| 标签筛选视频 | `02A 选择视频` | 标签 | `media_items.tags`、`media_items.category` | 无 | 需要数据库查询 |
| 进入放映室 | `03 放映室` | `room_id` / `room_code` | `rooms`、`room_members`、`media_items`、运行时 `room_state` | 运行时连接态 | DB + 内存协作 |
| 播放同步 | `03 放映室` | host 控制操作 | room authority runtime state | room authority runtime state | 当前以内存为主 |
| 房主转移 | 生命周期 | 无直接表单输入 | `room_members` + 运行时在线状态 | 运行时 host 切换，必要时同步 `rooms.host_user_id` | 需要内存主导 |
| grace period 房间销毁 | 生命周期 | 无 | `rooms`、`room_members` | `rooms.status` | 需要业务表状态更新 |

---

## 1. 账号登录

### 当前入口

- `01 登录页`
- 登录弹窗使用：
  - `account`
  - `password`

### 页面需要的信息

- 用户输入账号
- 用户输入密码
- 登录失败时的错误反馈

### 服务端需要做什么

1. 根据 `account` 查询 `users`
2. 读取 `password_hash`
3. 校验密码
4. 返回最小用户身份信息

### 数据库相关

需要读取：

- `users.account`
- `users.password_hash`
- `users.id`
- `users.nickname`

### 当前对 schema 的影响

这也是为什么 `users` 表已经需要：

- `account`
- `password_hash`
- `nickname`

说明：

- 登录框不需要昵称输入
- 昵称是注册阶段或后续资料编辑阶段的业务字段
- 但登录成功后，客户端或房间页可能仍需要展示昵称，所以 `users.nickname` 仍然是业务主数据

---

## 2. 账号注册

### 当前状态

UI 还未正式落地，但已经可以提前确定数据需求。

### 页面需要的信息

- `account`
- `password`
- `nickname`

### 服务端需要做什么

1. 检查 `account` 是否已存在
2. 生成 `password_hash`
3. 创建 `users` 记录

### 数据库相关

读取：

- `users.account`

写入：

- `users.account`
- `users.password_hash`
- `users.nickname`

### 当前对 schema 的影响

注册页一旦落地，`users.nickname` 就不再只是“以后也许会用”，而是首批真实业务字段。

---

## 3. 创建放映室

### 当前入口

- `02 首页`
- 后续会从“创建放映室”进入 `02A 选择视频`

### 页面需要的信息

- 当前登录用户是谁
- 当前选中的视频是什么

### 服务端需要做什么

1. 确认当前用户存在
2. 确认所选 `media_item` 存在
3. 创建 `rooms`
4. 创建当前用户的 `room_members`
5. 将该用户设为 host

### 数据库相关

读取：

- `users.id`
- `media_items.id`

写入：

- `rooms`
- `room_members`

### 当前对 schema 的影响

创建房间这件事，已经明确要求：

- `rooms.host_user_id`
- `rooms.media_item_id`
- `room_members.role`

不能只是运行时临时变量。

---

## 4. 02 首页与加入房间

### 当前页面目标

`02 首页与加入房间` 不只是两个按钮页，而是一个带有“用户信息 + 历史观看入口 + 创建/加入房间 + 继续追番”的首页。

根据当前业务描述，这一页至少包含：

1. 顶部欢迎语：`晚安, xx`
2. 右上角用户头像入口
3. “上次观看”卡片
4. `创建放映室`
5. `加入房间`
6. “继续追番”内容列表

这意味着它已经不是单纯依赖 `rooms` 表的页面，而是开始要求：

- 用户资料信息
- 用户观看历史/进度
- 媒体主数据

### 当前接口落地状态

`GET /home/summary` 已作为 `INT-118` 落地，当前用于一次性返回首页首屏需要的最小数据：

- `user.nickname`
- `user.avatarSeed`
- `user.avatarUrl`
- `lastWatched`
- `continueWatching`

当前读取来源：

- `users`
- `user_media_progress`
- `media_items`

当前约束：

- `lastWatched` 没有观看记录时返回 `null`
- `continueWatching` 当前返回最近 2 条 `completed = false` 的记录
- 登录态暂时使用 `Authorization: Bearer dev_<userId>` 占位 token

---

## 5. 02A 选择视频

### 当前页面目标

`02A 选择视频` 是从“创建放映室”进入的选片页，用来在真正创建房间前，先完成媒体搜索、标签筛选与选中确认。

根据当前业务描述，这一页至少包含：

1. 顶部搜索框
2. 默认展示的 5 个常用标签
3. 右侧 `更多` 入口
4. 悬浮展开的全部标签列表
5. 满足筛选条件的影片列表
6. 底部固定提示栏：
   - 已选中 xxx 影片
   - 创建房间

### 5.1 搜索框

#### 页面需要的信息

- 用户输入搜索词
- 搜索词可命中：
  - 影片标题
  - 别名/原始标题
  - 工作团队、制作公司等相关信息

#### 服务端需要做什么

1. 接收关键词
2. 在媒体检索字段上进行模糊匹配
3. 返回满足条件的媒体列表

#### 数据库相关

当前会要求 `media_items` 不只是保存展示字段，还要具备可搜索字段。

建议至少支持：

- `media_items.title`
- `media_items.original_title`
- `media_items.production_team`
- `media_items.search_aliases`

当前这部分 schema 基础已经落地：

- `media_items.original_title`
- `media_items.production_team`
- `media_items.search_aliases`

当前接口落地状态：

- `GET /media/items` 已作为 `INT-120` 落地
- `query` 会检索 `title / subtitle / description / original_title / production_team / search_aliases`
- `tag` 使用 `media_tags.slug`
- `limit` 默认 20，最大 50
- `cursor` 由服务端返回，Android 不解析内容，只原样传回

### 5.2 标签筛选

#### 页面需要的信息

- 默认显示 5 个主标签
- 点击 `更多` 后显示服务器可返回的全部标签
- 当前阶段全部标签最多展示 20 个

#### 服务端需要做什么

1. 返回默认主标签列表
2. 返回全部标签列表（最多 20 个）
3. 根据选中标签筛选媒体内容

#### 数据库相关

这一点说明当前仅有 `media_items.tags jsonb` 还不够稳定。

如果希望支持：

- 标签的稳定顺序
- 主标签与全部标签的区分
- 未来标签数量控制
- 首页/选片页共用标签目录

更合理的方向是补一层标签目录模型：

- `media_tags`
- `media_item_tags`

当前这部分 schema 基础已经落地：

- `media_tags`
- `media_item_tags`

当前接口落地状态：

- `GET /media/tags` 已作为 `INT-119` 落地
- `featuredTags` 返回最多 5 个默认主标签
- `allTags` 返回最多 20 个可展开标签
- `featuredTags / allTags` 都只返回 `is_active = true` 的标签
- Android 默认标签行使用 `featuredTags`
- Android 点击 `更多` 后使用 `allTags`

### 5.3 影片列表

#### 页面需要的信息

- 当前筛选条件下的媒体列表
- 每条媒体至少展示：
  - cover
  - title
  - 简短描述

#### 服务端需要做什么

1. 在无筛选时返回默认推荐列表
2. 在有搜索词或标签时返回筛选结果
3. 保证返回字段足够支持卡片展示

#### 数据库相关

读取：

- `media_items.id`
- `media_items.title`
- `media_items.cover_url`
- `media_items.description`
- `media_items.category`
- `media_items.subtitle`
- `media_items.duration_ms`
- `media_items.season_label`
- `media_items.episode_label`
- `media_tags.slug`
- `media_tags.name`

以及后续的标签关系表

### 5.4 底部固定栏

#### 页面需要的信息

- 当前选中的媒体标题
- 创建房间按钮状态

#### 服务端需要做什么

- 当前阶段不需要额外查询
- 真正点击“创建房间”时，使用选中的 `media_item_id`

#### 数据库相关

仍然会落到：

- `rooms.media_item_id`
- `room_members`

### 当前对 schema 的影响

`02A 选择视频` 这页带来的 schema 更新需求，当前已经比较明确：

1. `media_items` 需要补充更适合检索的字段
2. 标签体系需要从“仅作为媒体字段”升级为“可被服务端稳定返回的标签目录”

当前更推荐的下一步方向：

- 扩展 `media_items`：
  - `original_title`
  - `production_team`
  - `search_aliases`
- 新增标签目录：
  - `media_tags`
  - `media_item_tags`

这样后面无论是：

- 搜索标题
- 搜索工作团队
- 默认 5 个标签
- `更多` 中展开全部标签

都会有稳定的数据来源，而不是临时从 `jsonb` 里拼。

### 4.1 顶部欢迎语

#### 页面需要的信息

- 登录用户昵称

#### 服务端需要做什么

1. 根据当前登录态识别用户
2. 读取该用户的昵称

#### 数据库相关

读取：

- `users.id`
- `users.nickname`

### 4.2 右上角头像入口

#### 页面需要的信息

- 用户头像
- 至少要有一个可点击的个人中心入口

#### 服务端需要做什么

1. 返回用户可展示头像信息
2. 后续为个人中心页提供用户资料基础数据

#### 数据库相关

读取：

- `users.id`
- `users.nickname`
- `users.avatar_seed`
- `users.avatar_url`
- 未来的个人资料字段，例如：
  - `users.bio`

#### 当前对 schema 的影响

`users` 表需要继续扩成最小资料模型。  
当前推荐：

- `nickname`
- `avatar_seed`
- `avatar_url`
- `bio`

其中：

- 首页右上角头像优先读取 `avatar_url`
- 若为空，则用 `avatar_seed` 生成默认头像
- 个人中心第一版可直接复用这组字段

当前这组字段已经进入独立 migration 落地阶段：

- `users.avatar_seed`
- `users.avatar_url`
- `users.bio`

### 4.3 上次观看

#### 页面需要的信息

- 最近一次观看的媒体封面
- 最近一次观看的媒体名称
- 上次观看到的进度，精确到秒

#### 服务端需要做什么

1. 找到当前用户最近一次观看记录
2. 读取对应媒体信息
3. 返回：
   - `cover`
   - `title`
   - `last_position_seconds`

#### 数据库相关

当前第一版 schema 还缺少这一类持久化对象。  
这里最自然的新增方向是引入“用户观看进度”表，例如：

- `user_media_progress`

至少需要表达：

- `user_id`
- `media_item_id`
- `last_position_seconds`
- `duration_seconds`
- `last_watched_at`
- `updated_at`
- `completed`

当前建议：

- 进度以秒级存储
- 每个用户、每个媒体只保留一条当前进度记录
- 首页“上次观看”按 `last_watched_at desc` 取最近一条
- 这部分现在已经进入独立 migration 落地阶段

#### 当前对 schema 的影响

这是一个明确的 schema 缺口。  
如果没有“用户观看进度”这类表，就无法可靠支撑：

- 上次观看
- 继续追番
- 后续 resume playback

### 4.4 创建放映室

这一部分仍然沿用上面的“创建放映室”规则，但现在业务流程已经更清楚：

- `02 首页` 点击 `创建放映室`
- 跳到 `02A 选择视频`
- 用户选中视频后再真正创建房间

这意味着后续服务端接口更适合拆成：

1. 查询视频列表
2. 基于选中的 `media_item_id` 创建房间

当前接口落地状态：

- `POST /rooms` 已作为 `INT-121` 落地为 DB-backed create room
- 请求使用 `Authorization: Bearer dev_<userId>` 获取 host 用户
- 请求体使用 `mediaItemId`
- 服务端写入 `rooms`
- 服务端写入 host 的 `room_members`
- 响应同时返回 `room.id` 和 `room.roomCode`
- 当前 WebSocket 运行时仍以 `roomCode` 作为 `join_room.roomId`

当前数据边界：

- `rooms.id` 是数据库 UUID 主键
- `rooms.room_code` 是 6 位分享码，也是 Android 展示和输入的房间码
- `room_state` 仍属于运行时同步状态，HTTP create 只返回初始状态用于首屏衔接

### 4.5 加入房间

#### 页面需要的信息

- 输入 6 位房间码的弹窗

#### 服务端需要做什么

1. 根据 6 位 `room_code` 查询房间
2. 校验房间是否存在且可加入
3. 将当前用户加入该房间

#### 数据库相关

读取：

- `rooms.room_code`
- `rooms.status`

写入：

- `room_members`

说明：

- 当前 `rooms.room_code` 设计已经能支撑这项功能
- 弹窗本身不会新增 schema 需求

当前接口落地状态：

- `POST /rooms/{roomCode}/join` 已作为 `INT-122` 落地
- 请求使用 `Authorization: Bearer dev_<userId>` 获取当前用户
- 服务端根据 `roomCode` 查询可加入房间
- 用户已有 active member 时保持幂等
- 用户曾离开但房间仍存在时恢复成员关系
- join 成功后 Android 仍需要连接 WebSocket `/ws` 并发送 `join_room`

### 4.6 继续追番

#### 页面需要的信息

- 两个“还没看完”的内容
- 每个内容至少需要：
  - 封面
  - 标题
  - 当前观看进度

#### 服务端需要做什么

1. 找到该用户最近仍未看完的内容
2. 按最近观看时间排序
3. 返回前 2 个条目

#### 数据库相关

读取：

- `user_media_progress`
- `media_items`

#### 当前对 schema 的影响

这再次说明，单独的 `rooms` 和 `media_items` 还不够。  
必须引入用户维度的观看进度数据，才能支撑首页的“继续追番”模块。

当前建议：

- `继续追番` 按 `completed = false` 过滤
- 按 `last_watched_at desc` 取前 2 条
- 返回给首页时 join `media_items.cover_url / title`
- 当前数据库承载对象明确为 `user_media_progress`

### 当前结论

`02 首页与加入房间` 带来的 schema 影响，当前最值得记录的是两类：

1. `users` 未来需要头像相关字段
2. 需要新增“用户观看进度”这一类表

其中，“用户观看进度”当前已经可以进一步收敛为：

- 表名建议：`user_media_progress`
- 每个用户每个媒体一条记录
- 用秒级进度支撑首页展示
- 用 `completed` 直接驱动“继续追番”筛选

其中，`users` 当前也已经可以进一步收敛为：

- `nickname`
- `avatar_seed`
- `avatar_url`
- `bio`

这组字段已经足够支撑：

- 首页欢迎语
- 首页右上角头像入口
- 第一版个人中心资料展示

因此这页功能会推动下一轮数据库设计，但更适合在：

- 个人中心稿件明确后
- `02A 选择视频`
- 以及房间创建/进入流程接口一起梳理后

再统一出新的 migration。

---

## 5. 输入房间码加入

### 当前入口

- `02 首页`

### 页面需要的信息

- `room_code`

### 服务端需要做什么

1. 根据 `room_code` 查询房间
2. 校验房间是否仍可加入
3. 查找是否已有该用户 active 成员关系
4. 没有则新增，有则恢复/复用

### 数据库相关

读取：

- `rooms.room_code`
- `rooms.status`
- `room_members.room_id`
- `room_members.user_id`
- `room_members.is_active`

写入：

- `room_members`

### 当前对 schema 的影响

这也是为什么：

- `rooms.room_code` 需要唯一
- `room_members` 需要表达 active/inactive

当前 HTTP / WebSocket 分工：

- HTTP `POST /rooms/{roomCode}/join` 负责业务成员关系
- WebSocket `join_room` 负责实时连接进入、收到 `room_state`、参与播放同步
- 当前 WebSocket `join_room.payload.roomId` 使用 6 位 `roomCode`

---

## 6. 选择视频、搜索与标签筛选

### 当前入口

- `02A 选择视频`

### 页面需要的信息

- 视频标题
- 副标题或简介
- 封面
- 标签
- 分类
- 时长

### 交互需要的信息

- 搜索词
- 当前标签
- 列表排序结果

### 服务端需要做什么

1. 按搜索词查询 `media_items.title / subtitle / description`
2. 按标签或分类筛选
3. 返回适合卡片展示的列表字段

### 数据库相关

读取：

- `media_items.title`
- `media_items.subtitle`
- `media_items.description`
- `media_items.cover_url`
- `media_items.media_url`
- `media_items.category`
- `media_items.tags`
- `media_items.duration_ms`
- `media_items.status`

### 当前对 schema 的影响

这部分已经明确说明：

- `media_items` 不能只保存一个 `media_url`
- 它必须能支撑列表页面、搜索和标签筛选

---

## 7. 放映室与同步核心

### 当前入口

- `03 放映室`
- 从 `02A 选择视频` 点击 `创建房间` 后进入
- 也可以从 `02 首页` 输入房间码加入后进入

### 页面需要的信息

- 当前房间标题与房间码
- 当前媒体标题
- 当前集数或播放展示文案
- 当前播放进度
- 当前 host 与成员信息
- 当前播放倍速
- 当前同步状态与控制状态
- 当前在线成员状态

### 7.1 房间码

#### 页面需要的信息

- 右上角展示 6 位房间码
- 后续支持点击复制到剪贴板

#### 服务端需要做什么

1. 创建房间时生成 `room_code`
2. 进入房间页时返回当前房间的 `room_code`

#### 数据库相关

读取：

- `rooms.room_code`

当前对 schema 的影响：

- `rooms.room_code` 已经存在，且应保持 6 位唯一约束
- 点击复制属于 Android 交互，不新增 schema

当前接口落地状态：

- `GET /rooms/{roomCode}` 已作为 `INT-123` 落地
- 该接口返回 `room / media / members`
- 该接口只负责 `03 放映室` 首屏业务数据
- 运行时同步状态仍来自 WebSocket `room_state`

### 7.2 播放器主区域

#### 页面需要的信息

- 播放器应占满手机横向宽度
- 播放地址来自当前房间选中的媒体

#### 服务端需要做什么

1. 根据房间读取当前 `media_item_id`
2. 返回对应媒体的播放地址

#### 数据库相关

读取：

- `rooms.room_code`
- `rooms.host_user_id`
- `rooms.media_item_id`
- `rooms.status`
- `room_members.user_id`
- `room_members.role`
- `room_members.is_active`
- `users.nickname`
- `users.avatar_seed`
- `users.avatar_url`
- `media_items.title`
- `media_items.subtitle`
- `media_items.media_url`
- `media_items.duration_ms`
- `media_items.season_label`
- `media_items.episode_label`

### 7.3 观看进度持久化

`03 放映室` 的实时播放同步由 WebSocket 负责，但首页的“上次观看”和“继续追番”需要低频持久化用户进度。

当前接口落地状态：

- `PUT /me/media-progress/{mediaItemId}` 已作为 `INT-124` 落地
- 写入 `user_media_progress`
- 使用当前登录用户和媒体 ID 作为唯一业务维度
- 秒级进度用于业务展示，不用于实时同步

当前写入字段：

- `last_position_seconds`
- `duration_seconds`
- `completed`
- `completion_source`
- `last_watched_at`

当前约束：

- 普通播放进度不需要 `completion_source`
- 完成语义才传 `ended / manual_mark / threshold_auto`
- 不要把播放器高频 tick 直接写入数据库

读取：

- `rooms.media_item_id`
- `media_items.media_url`

当前对 schema 的影响：

- `media_items.media_url` 已经能支撑第一版播放地址
- 播放器宽度与比例属于 Android UI，不新增 schema

### 7.3 影片名称、集数与播放进度

#### 页面需要的信息

- 影片名称
- 当前是第几集
- 当前播放进度

#### 服务端需要做什么

1. 返回当前媒体标题
2. 返回用于展示的集数或播放展示文案
3. 运行时同步层继续提供当前播放位置

#### 数据库相关

读取：

- `media_items.title`
- `media_items.subtitle`
- `media_items.season_label`
- `media_items.episode_label`
- `media_items.duration_ms`

运行时读取：

- room authority state 中的 `positionMs`
- room authority state 中的 `paused`
- room authority state 中的 `ended`

当前对 schema 的影响：

- 第一版使用 `media_items.season_label` 承载季、篇章或系列分组文案
- 第一版使用 `media_items.episode_label` 承载“第 09 集”这类展示文案
- 当前不新增独立 episode 表
- 如果后续要稳定支持多集列表或下一集自动跳转，再评估 episode model
- 当前播放进度仍属于运行时同步状态，不建议直接落为 `rooms` 表字段

### 7.4 房主、成员与倍速

#### 页面需要的信息

- 当前房主昵称与头像
- 当前成员昵称与头像
- 当前播放倍速

#### 服务端需要做什么

1. 根据 `room_members` 查询当前房间成员
2. 关联 `users` 返回昵称与头像字段
3. 返回当前 host 信息
4. 通过运行时 `room_state.playbackRate` 表达当前倍速

#### 数据库相关

读取：

- `room_members.room_id`
- `room_members.user_id`
- `room_members.role`
- `room_members.is_active`
- `users.nickname`
- `users.avatar_seed`
- `users.avatar_url`

运行时读取：

- `room_state.hostUserId`
- `room_state.playbackRate`

当前对 schema 的影响：

- `users`、`room_members` 当前字段已经能支撑第一版展示
- 播放倍速继续由 authority runtime state 维护，不新增持久化字段

### 7.5 同步控制与在线状态

#### 页面需要的信息

- 同步状态提示
- 播放/暂停/seek 等控制
- 当前是否双端在线
- 房主/成员身份提示
- 重连与自动续播提示

#### 服务端需要做什么

分成两层：

1. 业务主数据读取
   - 查询 `rooms`
   - 查询 `room_members`
   - 查询 `media_items`
   - 查询 `users`

2. 运行时同步
   - 维护 authority state
   - 维护在线连接
   - 维护 heartbeat
   - 维护 host transfer
   - 广播 `room_state`

#### 数据库相关

需要读取：

- `rooms`
- `room_members`
- `media_items`
- `users`

当前不建议直接落库：

- 当前播放位置
- 当前 seq
- 当前 heartbeat 最近时间
- websocket 在线连接
- 当前是否双端在线

### 当前结论

`03 放映室` 这页需要的是“业务主数据 + 运行时同步状态”的组合视图。

当前已经足够支撑第一版的 schema：

- `rooms.room_code`
- `rooms.media_item_id`
- `rooms.host_user_id`
- `room_members`
- `users.nickname / avatar_seed / avatar_url`
- `media_items.title / subtitle / season_label / episode_label / media_url / duration_ms`

需要后续继续评估的 schema：

- 当出现多集列表、下一集自动跳转、系列/季/集聚合时，再评估独立 episode model

当前仍应保留在运行时的状态：

- `positionMs`
- `seq`
- `playbackRate`
- `paused`
- `ended`
- 在线连接与 heartbeat 状态

---

## 8. 生命周期相关功能

### 当前已经明确的规则

- 房间最后一个成员离开后进入 `2 分钟 grace period`
- grace period 内重新加入则取消销毁
- grace period 到期且仍为空则销毁

### 服务端需要做什么

1. 基于运行时在线情况识别“最后一个成员离开”
2. 更新 `rooms.status`
3. 记录 `last_empty_at / destroy_after`

### 数据库相关

需要写入：

- `rooms.status`
- `rooms.last_empty_at`
- `rooms.destroy_after`

---

## 当前最值得继续推动的 schema 方向

接下来如果继续根据真实业务页面推进 schema，优先级建议是：

1. 完善 `users`
   - 围绕登录、注册、昵称与头像
2. 新增用户观看进度模型
   - 围绕 `02 首页` 的“上次观看”和“继续追番”
3. 完善 `media_items`
   - 围绕选片页搜索、标签筛选与首页推荐内容
4. 将 create/join room 的 DB 读写路径落地
   - 围绕 `rooms` 和 `room_members`
5. 再决定哪些生命周期字段需要从运行时同步到业务表

---

## 更新规则

后续如果出现以下情况，应优先更新这份文档：

- 新增真实业务页面
- 新增表单字段
- 新增数据库查询需求
- 发现已有字段无法支撑页面交互
- 准备增加新的 migration

更新顺序建议：

1. 先更新本文件
2. 再更新 [server-data-model-design.md](./server-data-model-design.md)
3. 最后再新增 migration 或服务端实现
