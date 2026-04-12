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
- [contributing.md](./contributing.md): 当前仓库的基础工作规约
- [environment-config.md](./environment-config.md): 当前阶段环境参数清单与归类

## Maintenance Rule

- 项目目标、架构、产品边界和 ADR 统一在 Linear 更新。
- 仓库内的操作性规约与工程说明可以保留本地副本，例如贡献约定和环境配置说明。
- 仓库内 `docs/` 默认不再维护上述对应 Markdown 副本。
- 如果后续确实需要本地化文档，应先明确哪一侧是唯一权威来源。
