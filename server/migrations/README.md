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
真正的第一版 schema 将在后续任务中写入这里。
