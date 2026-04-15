# Server

`server/` 用于放置房间服务与后端相关代码。

当前目录已经完成 `INT-14 bootstrap websocket room server` 的第一阶段骨架，主要包含：

- 可启动的 Go HTTP / WebSocket 服务入口
- `POST /rooms` 的最小 create room 能力
- `/ws` WebSocket 接入路由与正式 join room 语义
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
- `internal/room/`: 房间、连接、房间管理器，以及房间创建逻辑的内存模型
- `internal/transport/`: `POST /rooms` 和 `/ws` 的接入层与最小 HTTP / WebSocket 处理

## Current Validation

当前已完成的本地验证：

- `go test ./...`
- `go run ./cmd/roomserver`
- `POST /rooms` 返回 `201 Created` 与初始 `room_state`
- `join_room` 仅允许加入已存在房间，不存在房间时返回 `error`

其中 `POST /rooms` 和 `/ws` 的最小 `create room -> join_room -> room_state` 路径已通过基础测试验证。
