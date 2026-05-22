# watch_together

`watch_together` 是一个自托管的同步观影项目，目标是让多个用户在不同设备上进入同一个房间，围绕同一部 HLS 视频进行播放、暂停、seek、倍速和播放进度同步。

当前主线是先完成 `Android -> Android` 的稳定同步观影 MVP，再逐步扩展到更完整的媒体库、部署方案和更大房间规模。

## 当前状态

项目已经进入“可联调的原型 / MVP 加固阶段”：

- Android 客户端已经具备登录、首页、选片、创建房间、加入房间、放映室和 HLS 播放能力。
- Go 服务端已经具备账号注册登录、JWT 鉴权、媒体列表、房间创建、房间加入、房间详情、观看进度写入和 WebSocket 同步能力。
- WebSocket 同步模型以服务端 timeline 为权威，host 可以 `play / pause / seek / set_playback_rate / ended` 推进房间时间轴。
- 房间当前状态可以写入 Redis `room_state` cache，PostgreSQL 负责用户、媒体、房间、成员和观看进度等持久化数据。
- 服务端已经加入慢客户端隔离、广播并发限制、control 去重、stale seq 拒绝、房间生命周期 grace period、同一账号单房间活跃设备限制等后端风险加固。
- 媒体分发支持 `signed_redirect / minio_presign / nginx_auth_request` 三种模式，其中生产 compose 默认使用 `nginx_auth_request`。

当前不再设计自动 host transfer。房主断线后，房间可以进入宽限期；房主是否重新进入、房间是否继续存在，由当前业务规则和房间生命周期处理。

## 项目结构

```text
watch_together/
├── android/   Android 客户端
├── server/    Go 后端、mediactl CLI、Docker / Compose 部署配置
├── media/     本地测试媒体与 HLS 输出目录
├── docs/      项目业务、同步模型、接口、部署和重构文档
├── shared/    预留的跨端共享定义
├── scripts/   仓库级辅助脚本
└── windows/   Windows 客户端预留目录
```

推荐先阅读：

- [docs/current-business-overview.md](./docs/current-business-overview.md)
- [docs/backend-api-contract.md](./docs/backend-api-contract.md)
- [docs/websocket-event-protocol.md](./docs/websocket-event-protocol.md)
- [docs/media-storage-and-delivery.md](./docs/media-storage-and-delivery.md)
- [server/README.md](./server/README.md)
- [server/deploy/README.md](./server/deploy/README.md)

## 技术栈

- Android: Kotlin, Jetpack Compose, Media3 ExoPlayer
- Server: Go, Gin, `net/http`, `github.com/coder/websocket`
- Database: PostgreSQL, GORM
- Cache / Runtime State: Redis
- Object Storage: Local, MinIO, S3-compatible storage
- Media Pipeline: FFmpeg, FFprobe, `server/cmd/mediactl`
- Reverse Proxy: Nginx

## 本地开发

### 1. 启动基础设施

默认 `server/compose.yaml` 面向本地开发，会暴露 PostgreSQL、Redis、MinIO API 和 MinIO Console，方便调试。

```bash
cd server
docker compose up -d postgres redis minio minio-init
```

默认端口：

```text
PostgreSQL: 127.0.0.1:5432
Redis:      127.0.0.1:6380
MinIO API:  127.0.0.1:9100
MinIO UI:   127.0.0.1:9101
```

### 2. 配置本地环境

```bash
cd server
cp .env.example .env.local
```

按本机环境调整 `DATABASE_URL`、`REDIS_ADDR`、`MEDIA_STORAGE_DRIVER`、`MEDIA_STORAGE_ENDPOINT`、`MEDIA_PUBLIC_BASE_URL` 等配置。

服务端配置加载顺序是：

1. 当前 shell 环境变量
2. `server/.env.<APP_ENV>.local`
3. `server/.env.<APP_ENV>`
4. `server/.env.local`
5. `server/.env`
6. 代码默认值

`.env.example` 和 `.env.prod.example` 只作为模板，不参与运行时加载。

### 3. 启动 Go 服务

```bash
cd server
APP_ENV=local go run ./cmd/roomserver
```

健康检查：

```text
GET http://127.0.0.1:8080/healthz
```

### 4. 可选：本地也使用容器化 Nginx + Go server

如果希望本地联调也走 Nginx 反向代理，可以启用 `app` profile：

```bash
cd server
docker compose --profile app up -d --build
```

入口：

```text
http://127.0.0.1:8080
```

### 5. 启动 Android

用 Android Studio 打开 `android/`，运行到模拟器或真机。

Android 端通过后端 API 获取 `mediaUrl`，不直接拼接 MinIO、bucket、access key 或对象存储 URL。

## 生产部署

生产环境建议使用独立的 [server/compose.prod.yaml](./server/compose.prod.yaml)。它的默认边界是：

- 只暴露 Nginx HTTP 端口。
- Go `roomserver` 只在 Docker 内网监听 `8080`。
- PostgreSQL、Redis、MinIO API、MinIO Console 不暴露到宿主机。
- 媒体分发默认使用 `MEDIA_DELIVERY_MODE=nginx_auth_request`。
- Go 负责签发播放入口和媒体访问 cookie，Nginx 负责公网反代和 HLS 字节流，MinIO 只在 Docker 内网被访问。

启动示例：

```bash
cd server
cp .env.prod.example .env.prod
# 修改 .env.prod 中的密码、JWT secret、媒体签名 secret、域名等配置
docker compose --env-file .env.prod -f compose.prod.yaml up -d --build
```

生产环境不要直接暴露 MinIO API 到公网。当前生产 compose 中的 MinIO bucket 会在 Docker 内网允许匿名读取，这是为了让 Nginx 能够反代对象字节；公网访问仍然由 Nginx `auth_request` + Go `/media/internal/auth` 进行鉴权。

更多说明见 [server/deploy/README.md](./server/deploy/README.md)。

## 媒体入库

`mediactl` 用于把本地视频转成 HLS、上传到目标存储，并写入 PostgreSQL 媒体元数据。

示例：

```bash
cd server
go run ./cmd/mediactl ingest \
  --stages=build-hls,upload,write-db \
  --library-root ../media/raw \
  --input ../media/raw/sample-show/season-01/episode-01.mp4 \
  --title "测试视频" \
  --season-label "第 1 季" \
  --episode-label "第 01 集" \
  --tags test,anime \
  --dry-run=false \
  --write-db
```

详细说明见 [docs/mediactl-operations.md](./docs/mediactl-operations.md) 和 [docs/media-storage-and-delivery.md](./docs/media-storage-and-delivery.md)。

## 核心文档

- [当前业务总览](./docs/current-business-overview.md)
- [后端 API 契约](./docs/backend-api-contract.md)
- [WebSocket 事件协议](./docs/websocket-event-protocol.md)
- [同步模型技术笔记](./docs/core-sync-technical-notes.md)
- [服务端数据模型设计](./docs/server-data-model-design.md)
- [媒体存储与分发](./docs/media-storage-and-delivery.md)
- [环境配置说明](./docs/environment-config.md)
- [后端风险加固需求](./docs/refactor/11-backend-risk-hardening-requirements.md)
- [压测方案](./docs/refactor/12-server-load-testing.md)

## 当前优先级

接下来仍然优先打磨核心后端能力：

1. 多用户同步观影性能与一致性。
2. WebSocket 长连接可靠性。
3. 房间生命周期、断线重连和主动离开语义。
4. room_state、PostgreSQL、Redis、进程内状态的边界稳定性。
5. HLS 媒体访问压力与无 CDN 部署方案。
6. Android 与服务端协议联调体验。

大型微服务拆分、复杂权限系统、推荐系统、完整后台管理和多区域部署都不是当前第一优先级。
