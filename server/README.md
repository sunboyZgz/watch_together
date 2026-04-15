# Server

`server/` 用于放置房间服务与后端相关代码。

当前目录作为服务端占位，后续会逐步补入：

- 最小鉴权逻辑
- 房间创建与加入逻辑
- 房主转移与生命周期管理
- 播放同步服务
- 存储接入与配置

这里会成为全系统的房间协调与同步中心。

## Current Foundation

围绕 `INT-14 bootstrap websocket room server`，当前服务端基础选型已明确为：

- Language: Go
- HTTP server: Go standard library `net/http`
- WebSocket library: `github.com/coder/websocket`
- Message encoding: Go standard library `encoding/json`
- Room state storage: in-memory only

当前阶段先追求“最小可运行房间服务”，暂不引入 Gin / Echo / Fiber、Redis、数据库 ORM 或复杂配置中心。
