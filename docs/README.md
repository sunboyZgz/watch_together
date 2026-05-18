# watch_together Docs

`docs/` 目前保留为仓库内的文档入口与静态资源目录。

为避免仓库文档与 Linear 项目文档出现双份维护，项目级设计文档以 Linear 为准，这里只保留索引和本地资源。

## Canonical Docs In Linear

- [Project Overview](https://linear.app/interestings/document/01-project-overview-3e260fd6f4d3)
- [System Architecture](https://linear.app/interestings/document/02-system-architecture-3eb5b105f074)
- [Product Scope & Lifecycle](https://linear.app/interestings/document/03-product-scope-and-lifecycle-6089958d7517)
- [ADR / Tech Decisions](https://linear.app/interestings/document/09-adr-tech-decisions-cad8bee54188)

## Local Files

- [架构.png](./架构.png): 当前架构图静态资源
- [android-player-refactor-analysis.md](./android-player-refactor-analysis.md): Android 播放器业务与同步核心逻辑分离的重构分析
- [android-player-usage.md](./android-player-usage.md): 当前 Android 播放器的层级关系、职责边界和使用方式
- [current-business-overview.md](./current-business-overview.md): 当前前后端业务功能、最新业务决策和待实现能力说明；后续需求评审优先参考
- [business-feature-data-mapping.md](./business-feature-data-mapping.md): 业务功能、页面交互与数据库字段/查询需求之间的映射
- [backend-api-contract.md](./backend-api-contract.md): 后端 HTTP API 契约、统一响应格式与 Android/Server 联调规则
- [contributing.md](./contributing.md): 当前仓库的基础工作规约
- [core-sync-technical-notes.md](./core-sync-technical-notes.md): 核心同步功能中的关键技术点记录
- [database-relationship-uml.puml](./database-relationship-uml.puml): PostgreSQL 表关系 UML/ER 图与目标媒体主模型
- [environment-config.md](./environment-config.md): 当前阶段环境参数清单与归类
- [mediactl-operations.md](./mediactl-operations.md): `mediactl` CLI 的本地 HLS 生成、参数、验证和后续入库操作说明
- [media-storage-and-delivery.md](./media-storage-and-delivery.md): 媒体资源存储、HLS 分发、对象存储和后续入库 CLI 的技术选型
- [server-data-model-design.md](./server-data-model-design.md): 服务端业务对象、PostgreSQL 表设计与运行时数据边界
- [websocket-event-protocol.md](./websocket-event-protocol.md): Phase 1 最小 WebSocket 事件协议草案

## Maintenance Rule

- 项目目标、架构、产品边界和 ADR 统一在 Linear 更新。
- 仓库内的操作性规约与工程说明可以保留本地副本，例如贡献约定和环境配置说明。
- 跨端共享但已经进入工程实现阶段的协议草案，也可以保留本地副本以便联动开发。
- 仓库内 `docs/` 默认不再维护上述对应 Markdown 副本。
- 如果后续确实需要本地化文档，应先明确哪一侧是唯一权威来源。
