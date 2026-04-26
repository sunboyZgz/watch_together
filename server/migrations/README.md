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

当前仍不引入独立 `media_episodes` 表，等后续需要多集列表、下一集自动跳转或系列/季/集聚合时再扩展。
