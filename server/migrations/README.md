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

历史迁移中曾经存在扁平的 `media_items` 模型，并通过后续迁移逐步过渡到 season/episode 模型。

当前目标模型已经收敛为：

- `media_seasons`：一季、篇章或作品容器，不额外引入 `media_series`
- `media_episodes`：真正可播放的一集或视频资源
- `media_tags`：媒体标签字典
- `media_season_tags`：season 与标签目录的关系表
- `rooms.media_episode_id`：房间选择的可播放 episode
- `user_media_progress.media_episode_id`：用户低频观看进度对应的 episode

关键迁移：

- `20260429101000_add_media_season_episode_schema.up.sql`
- `20260429101000_add_media_season_episode_schema.down.sql`
- `20260430103000_add_episode_refs_to_rooms_and_progress.up.sql`
- `20260430103000_add_episode_refs_to_rooms_and_progress.down.sql`
- `20260501100000_remove_legacy_media_items_schema.up.sql`
- `20260501100000_remove_legacy_media_items_schema.down.sql`

`20260501100000_remove_legacy_media_items_schema` 会删除旧的 `media_items / media_item_tags` 表，以及 `rooms.media_item_id`、`user_media_progress.media_item_id`、`media_episodes.legacy_media_item_id` 这些兼容字段。执行前会检查现有 `rooms` 与 `user_media_progress` 是否都已经具备 `media_episode_id`，避免静默丢失引用关系。
