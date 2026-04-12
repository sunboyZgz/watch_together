# Contributing

> Basic working conventions for this repository.

这是一份面向当前个人项目阶段的最小规约，用来约束仓库结构、分支命名和提交习惯，避免项目在早期快速演进时变得混乱。

---

## 1. Repository Structure

本仓库采用 monorepo 组织方式，当前包含以下顶层模块：

- `android/` — Android client built with Kotlin
- `windows/` — Windows client built with Tauri
- `server/` — backend service built with Go
- `docs/` — project documents and technical specifications
- `shared/` — shared protocol drafts, schemas, and examples
- `scripts/` — repository-level helper scripts

规则：

- 新增代码或文档时，优先放入已有职责明确的目录。
- 不要在根目录随意堆放临时文件、实验代码或未分类脚本。
- 如果需要新增顶层目录，应先明确其长期职责。

---

## 2. Fixed Tech Stack

当前项目的主技术栈固定如下：

- Android: Kotlin
- Windows: Tauri
- Server: Go

规则：

- 不要为以上模块引入替代性的主栈实现。
- 如果需要变更主技术栈，必须先记录 ADR，再调整仓库结构或实现计划。
- 共用协议和数据结构应尽量保持实现无关，不要把某一端的框架选择泄漏成全局约束。

---

## 3. Repository Strategy

当前仓库使用 monorepo 组织方式，但暂不引入专门的 monorepo 项目管理工具。

当前选择：

- 用目录边界保持模块清晰。
- 用简单脚本和约定支撑早期开发。
- 暂不引入 Nx、Turborepo、Bazel、Rush 等额外工具。

规则：

- 只有在跨模块构建、任务编排、缓存或共享包管理复杂度明显上升后，才评估是否引入 monorepo 工具。
- 在此之前，优先保持结构简单、可读和可维护。

---

## 4. Branch Naming Convention

建议使用以下分支命名格式：

- `feat/<short-description>`
- `fix/<short-description>`
- `infra/<short-description>`
- `docs/<short-description>`
- `refactor/<short-description>`

示例：

- `feat/android-room-ui`
- `fix/seek-sync-loop`
- `infra/init-repo-structure`
- `docs/project-overview`

规则：

- 使用小写字母。
- 单词之间使用连字符 `-`。
- 名称保持简短且可读。
- 一个分支尽量只对应一个明确目标或一个 issue。

---

## 5. Commit Message Convention

建议采用简单的 conventional 风格：

- `feat: ...`
- `fix: ...`
- `infra: ...`
- `docs: ...`
- `refactor: ...`
- `chore: ...`

示例：

- `feat: add android room join page`
- `fix: resolve repeated seek sync issue`
- `infra: initialize repository structure`
- `docs: add product scope document`

规则：

- 标题行保持简短。
- 使用现在时。
- 描述做了什么，不在标题里展开过多背景。

---

## 6. Contribution Scope

当前阶段更适合以下工作方式：

- 保持改动小且聚焦。
- 一条 issue 对应一个明确目标。
- 结构性变更发生时，同步补文档。
- 优先先写清规则，再进入实现。

避免：

- 在一个提交里混入不相关改动。
- 未记录决策就更改既定技术栈。
- 在基线尚未稳定前提前引入重型工具链。

---

## 7. Notes

这份文档当前主要服务于个人开发时的自我约束，而不是正式团队流程。

后续如果项目复杂度上升，可以继续补充：

- code style
- testing strategy
- release process
- review workflow
