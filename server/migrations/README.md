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
- `media_episode_variants`：同一个 episode 下的多码率 HLS variant，例如 `720p / 1080p`
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
- `20260503110000_add_media_episode_variants.up.sql`
- `20260503110000_add_media_episode_variants.down.sql`

`20260501100000_remove_legacy_media_items_schema` 会删除旧的 `media_items / media_item_tags` 表，以及 `rooms.media_item_id`、`user_media_progress.media_item_id`、`media_episodes.legacy_media_item_id` 这些兼容字段。执行前会检查现有 `rooms` 与 `user_media_progress` 是否都已经具备 `media_episode_id`，避免静默丢失引用关系。

## 脚本使用

`server/scripts/` 下目前有两个 migration 相关脚本：

- `new_migration.sh`：生成新的 `up/down` SQL migration 文件。
- `migrate.sh`：调用 `golang-migrate` 执行、回滚或查看 migration 状态。

建议所有命令都在 `server/` 目录下执行，这样路径和文档示例保持一致。

### 创建新 migration

使用：

```bash
cd server
./scripts/new_migration.sh add_example_table
```

脚本会在 `server/migrations/` 下生成两个文件：

```text
YYYYMMDDHHMMSS_add_example_table.up.sql
YYYYMMDDHHMMSS_add_example_table.down.sql
```

约定：

- 参数使用 `snake_case`，例如 `add_media_episode_variants`。
- `.up.sql` 写正向变更。
- `.down.sql` 写对应回滚。
- 创建后必须手动补充 SQL，脚本只生成占位模板。

### 执行 migration

前置条件：

- 本机已安装 `golang-migrate` CLI。
- 已设置 `DATABASE_URL`。
- PostgreSQL 服务已启动。

安装示例：

```bash
brew install golang-migrate
```

本地 Docker PostgreSQL 示例：

```bash
cd server
docker compose up -d
export DATABASE_URL='postgres://app:app@127.0.0.1:5432/anime_watch_dev?sslmode=disable'
```

执行所有未应用的 migration：

```bash
cd server
./scripts/migrate.sh up
```

查看当前 migration 版本：

```bash
cd server
./scripts/migrate.sh version
```

回滚最近 `n` 个 migration：

```bash
cd server
./scripts/migrate.sh down 1
```

注意：`down` 会真实回滚数据库结构和数据变更。执行前要确认当前数据库可以安全回滚，尤其不要在未备份的共享环境里随意执行。

### 修复 dirty migration 状态

如果 migration 执行过程中失败，`golang-migrate` 可能会把数据库标记为 dirty。此时 `up/down` 会拒绝继续执行，需要先人工确认数据库状态，再使用 `force` 修复版本标记。

示例：

```bash
cd server
./scripts/migrate.sh version
./scripts/migrate.sh force 20260503110000
```

重要边界：

- `force` 不会执行 SQL。
- `force` 只修改 migration 版本标记。
- 只有在你确认数据库结构已经和目标版本一致，或者已经手动修复到可继续迁移的状态后，才应该使用。
- 常规开发流程不要用 `force` 代替 `down` 或 `up`。

### 推荐工作流

新增 schema 时：

```bash
cd server
./scripts/new_migration.sh add_some_schema
```

然后：

- 编辑生成的 `.up.sql` 和 `.down.sql`。
- 检查 SQL 是否包含必要约束、索引和回滚逻辑。
- 本地执行 `./scripts/migrate.sh up`。
- 必要时执行 `./scripts/migrate.sh down 1` 验证回滚。
- 再执行 `./scripts/migrate.sh up` 确认正向迁移可重复恢复。
