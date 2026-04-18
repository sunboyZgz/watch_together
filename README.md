# watch_together

`watch_together` 是一个用于异地同步观看云端 HLS 视频的跨平台应用。

当前目标是先交付 Android ↔ Android 的同步观影 MVP，后续再扩展到 Windows ↔ Android 的跨平台同步。

## Project Goal

项目当前聚焦解决以下问题：

- 两个用户进入同一个房间
- 观看同一个云端视频资源
- 在房间内同步播放、暂停、拖动和倍速
- 保持较好的同步体验
- 尽量降低使用门槛

## Current Status

当前仓库还处于工程初始化阶段，重点是把 monorepo 结构、模块边界和基础说明先搭起来。

当前阶段规划：

- Phase 1: Android ↔ Android 同步观影 MVP
- Phase 2: Windows ↔ Android 跨平台同步

当前产品边界：

- 支持管理员预注册用户
- 支持登录后使用
- 支持云端 HLS 媒体播放
- 不支持匿名使用
- 不支持用户导入任意 URL

## Tech Stack

当前已确认的技术选型：

- Android: Kotlin
- Windows: Tauri
- Server: Go

这个技术栈是当前仓库结构和后续任务拆解的基础假设。

当前已明确的核心实现库：

- Android player: AndroidX Media3 ExoPlayer
- Android networking: OkHttp
- Server HTTP: Go standard library `net/http`
- Server WebSocket: `github.com/coder/websocket`
- Server message encoding: Go standard library `encoding/json`

这些库当前属于项目关键实现基础。若后续发生替换或重大升级，应同步更新对应 issue 和仓库文档。

## Repository Strategy

当前仓库采用 monorepo 组织方式，把 Android、Windows、Server、shared 和 scripts 放在同一个代码仓库中统一管理。

当前阶段暂不引入额外的 monorepo 项目管理工具，先保持目录结构清晰、依赖关系直接，等跨模块构建、任务编排或共享代码管理复杂度明显上升后，再评估是否引入专门工具。

## Repository Structure

```text
watch_together/
├─ android/
├─ media/
├─ windows/
├─ server/
├─ docs/
├─ shared/
├─ scripts/
├─ LICENSE
└─ README.md
```

各目录职责：

- `android/`: Android 客户端工程，当前已包含播放器适配层、协议草案、join-time initial state sync、heartbeat ack、最小 drift correction、repeated join resync 和最小控制事件同步能力
- `media/`: 本地开发和联调使用的样例媒体资源
- `windows/`: Windows 客户端工程与跨平台同步能力
- `server/`: 基于 Go 的房间服务骨架，当前已包含 `POST /rooms`、`/ws`、协议解析、应用层 heartbeat、最小房间管理结构和 `play / pause / seek` 控制广播
- `docs/`: 仓库内文档入口和静态资源
- `shared/`: 跨端共享定义，例如协议、Schema、常量
- `scripts/`: 开发、维护和初始化辅助脚本

当前已经落地的关键子结构：

```text
android/app/src/main/java/com/example/watch_together/
├─ config/
├─ sync/
│  └─ protocol/
├─ ui/
│  ├─ player/
│  └─ theme/
└─ MainActivity.kt

server/
├─ cmd/roomserver/
└─ internal/
   ├─ app/
   ├─ protocol/
   ├─ room/
   └─ transport/
```

子结构职责速览：

- `android/.../config/`: Android 配置注入与运行时地址拼装
- `android/.../sync/`: 建房、入房、消息解码、heartbeat ack、本地同步状态和 join 后初始状态应用
- `android/.../sync/protocol/`: Android 侧对齐 `INT-19` 的协议草案模型
- `android/.../ui/player/`: 播放器页面、适配器和播放器事件
- `server/cmd/roomserver/`: Go 服务启动入口
- `server/internal/app/`: HTTP server 与路由组装
- `server/internal/protocol/`: 服务端协议外壳、事件模型和解码
- `server/internal/room/`: 房间、连接和房间管理器
- `server/internal/transport/`: `POST /rooms`、`/ws` 以及对应测试

## Documentation

项目级设计文档当前以 Linear 为准，仓库内 `docs/` 主要保留入口与本地资源。

主要文档入口：

- [docs/README.md](./docs/README.md)
- [media/README.md](./media/README.md)
- [WebSocket Event Protocol](./docs/websocket-event-protocol.md)
- [Project Overview](https://linear.app/interestings/document/01-project-overview-3e260fd6f4d3)
- [System Architecture](https://linear.app/interestings/document/02-system-architecture-3eb5b105f074)
- [Product Scope & Lifecycle](https://linear.app/interestings/document/03-product-scope-and-lifecycle-6089958d7517)
- [ADR / Tech Decisions](https://linear.app/interestings/document/09-adr-tech-decisions-cad8bee54188)

## Near-term Focus

接下来优先推进：

- Android 客户端工程初始化
- 房间服务基础能力搭建
- WebSocket 同步协议和共享结构沉淀
- Phase 1 所需的最小可运行链路
