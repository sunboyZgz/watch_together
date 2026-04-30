# Server Migrations

这里存放 `server/` 的 SQL-first migration 文件。

当前约定：

- 所有 schema 变更都以 `.sql` 迁移文件为唯一权威来源
- 使用 `up/down` 成对文件
- 文件命名采用：`时间戳_语义化名称.up.sql`
- 对应回滚文件采用：`时间戳_语义化名称.down.sql`

示例：

```text
202604200001_create_users_table.up.sql
202604200001_create_users_table.down.sql
```

当前阶段本目录先承载 migration 基础设施。
真正的第一版 schema 已经写入这里，并通过后续 migration 逐步扩展业务字段。

当前与媒体播放展示相关的轻量字段 migration：

- `20260426104000_add_media_playback_display_fields.up.sql`
- `20260426104000_add_media_playback_display_fields.down.sql`

这组 migration 为 `media_items` 增加：

- `season_label`：季、篇章或系列分组展示文案
- `episode_label`：当前集数或单集展示文案

当前媒体内容模型已经进入 season/episode 过渡阶段。

新增两层媒体 schema migration：

- `20260429101000_add_media_season_episode_schema.up.sql`
- `20260429101000_add_media_season_episode_schema.down.sql`

这组 migration 增加：

- `media_seasons`：一季、篇章或作品容器，不额外引入 `media_series`
- `media_episodes`：真正可播放的一集或视频资源
- `media_season_tags`：season 与标签目录的关系表

兼容策略：

- 暂不删除 `media_items`
- 暂不立刻改动 `rooms.media_item_id` 与 `user_media_progress.media_item_id`
- migration 会把旧 `media_items` 安全 backfill 成一条 `media_seasons` 与一条 `media_episodes`
- `media_episodes.legacy_media_item_id` 用于后续 API 迁移期间关联旧数据

后续 `INT-147 / INT-148` 会分别处理服务端 API 和 Android 客户端对 episode-backed 模型的迁移。
