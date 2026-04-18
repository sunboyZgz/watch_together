# Server

`server/` 用于放置房间服务与后端相关代码。

当前目录已经完成 `INT-14 bootstrap websocket room server` 的第一阶段骨架，主要包含：

- 可启动的 Go HTTP / WebSocket 服务入口
- `POST /rooms` 的最小 create room 能力
- `/ws` WebSocket 接入路由与正式 join room 语义
- `play / pause / seek` 的最小控制事件处理与广播
- 应用层 heartbeat 与静默连接超时清理
- host disconnect 后的 immediate host transfer
- same-user repeated join 的单有效连接收敛
- 最小 `Room / ClientConnection / RoomManager` 内存结构
- 基于 `INT-19` 协议草案的最小消息解析层
- `join_room` 的已存在房间接入流程
- 基础断连清理逻辑

这里会成为全系统的房间协调与同步中心。

## Current Foundation

围绕 `INT-14 bootstrap websocket room server`，当前服务端基础选型已明确为：

- Language: Go
- HTTP server: Go standard library `net/http`
- WebSocket library: `github.com/coder/websocket`
- Message encoding: Go standard library `encoding/json`
- Room state storage: in-memory only

当前阶段先追求“最小可运行房间服务”，暂不引入 Gin / Echo / Fiber、Redis、数据库 ORM 或复杂配置中心。

## Current Structure

```text
server/
├── cmd/
│   └── roomserver/
│       └── main.go
├── internal/
│   ├── app/
│   │   └── server.go
│   ├── protocol/
│   │   ├── decode.go
│   │   ├── envelope.go
│   │   └── events.go
│   ├── room/
│   │   ├── client.go
│   │   ├── manager.go
│   │   ├── manager_test.go
│   │   └── room.go
│   └── transport/
│       ├── json.go
│       ├── room_http_handler.go
│       ├── room_http_handler_test.go
│       ├── websocket_handler.go
│       └── websocket_handler_test.go
├── go.mod
├── go.sum
└── README.md
```

## Directory Responsibilities

- `cmd/roomserver/`: 服务端启动入口
- `internal/app/`: 应用组装层，负责配置和 HTTP server 初始化
- `internal/protocol/`: 与 `INT-19` 对齐的最小协议结构，包含 create room 请求 / 响应和 WebSocket 事件模型
- `internal/room/`: 房间、连接、房间管理器，以及房间创建与控制状态更新的内存模型
- `internal/transport/`: `POST /rooms`、`/ws`、join room 与 `play / pause / seek` 的接入层和测试

当前关键文件对应关系：

- `cmd/roomserver/main.go`: 读取配置并启动服务
- `internal/app/server.go`: 注册 `/healthz`、`POST /rooms` 和 `/ws`
- `internal/protocol/events.go`: create room、join room、room_state、heartbeat、error 等最小结构
- `internal/room/manager.go`: 房间创建、查询、唯一 `roomId` 生成和客户端清理
- `internal/transport/room_http_handler.go`: create room HTTP 入口
- `internal/transport/websocket_handler.go`: join room、heartbeat、host 校验、控制事件处理和广播
- `internal/room/room.go`: 房间成员、host 状态和 repeated join 连接替换逻辑

## Current Validation

当前已完成的本地验证：

- `go test ./...`
- `go run ./cmd/roomserver`
- `POST /rooms` 返回 `201 Created` 与初始 `room_state`
- `join_room` 仅允许加入已存在房间，不存在房间时返回 `error`
- host 发出的 `play / pause / seek` 会更新房间状态并广播
- 非 host 发出的控制事件会返回 `error`
- 服务端会周期性发送 `heartbeat`，客户端需返回 `heartbeat_ack`
- 超时未 ack 的连接会进入现有断连清理流程
- host 断开连接后，剩余成员会收到新的 `room_state` 且 host 身份立即转移
- 同一 `userId` repeated join 同一房间时，新连接会替换旧连接并重新收到最新 `room_state`

其中 `POST /rooms`、`join_room`、`play / pause / seek` 与 host transfer 的最小同步路径已通过基础测试验证。
