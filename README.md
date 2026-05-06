# watch_together

`watch_together` 是一个面向异地同步观影场景的跨平台项目。当前主线是先完成 `Android ↔ Android` 的同步放映 MVP：用户登录、选片、创建房间、加入房间、同步播放 HLS 视频。

## 项目结构

```text
watch_together/
├─ android/   Android 客户端
├─ server/    Go 后端与 mediactl CLI
├─ media/     本地测试媒体与 HLS 输出目录
├─ docs/      本地工程文档入口
├─ shared/    预留的跨端共享定义
├─ scripts/   仓库级辅助脚本
└─ windows/   Windows 客户端预留目录
```

更细的模块说明见：

- [android/README.md](./android/README.md)
- [server/README.md](./server/README.md)
- [docs/README.md](./docs/README.md)
- [media/README.md](./media/README.md)

## 已实现功能

- Android 登录页、首页、加入房间、选片页、放映室主页面
- Android 基于 Media3 ExoPlayer 的 HLS 播放、全屏、倍速、清晰度选择、缓存接入
- Android 基于 WebSocket 的播放、暂停、seek、倍速、ended 最小同步链路
- Go 后端账号注册、登录、首页摘要、媒体标签、媒体检索、创建房间、房间码加入、房间详情、观看进度写入
- Go 房间服务 `/ws`、房间状态广播、heartbeat、host transfer、重复 join 收敛
- PostgreSQL migration、seed、媒体 season / episode 主模型
- `mediactl` 支持 `plan / build-hls / upload / write-db / ingest`，可生成 HLS、上传到 local / MinIO、并写入数据库
- 本地 MinIO 联调链路与 Android 播放云端 HLS 资源

## 技术栈

- Android: Kotlin + Jetpack Compose + Media3 ExoPlayer
- Server: Go + `net/http` + `coder/websocket`
- Database: PostgreSQL
- Object Storage: Local / MinIO / S3-compatible
- Media Pipeline: FFmpeg / FFprobe

## 如何使用

### 1. 启动后端基础设施

```bash
cd server
docker compose up -d postgres minio minio-init
```

### 2. 配置本地环境

```bash
cd server
cp .env.example .env.local
```

按你的机器环境修改 `DATABASE_URL`、`MEDIA_STORAGE_DRIVER`、`MEDIA_STORAGE_ENDPOINT`、`MEDIA_STORAGE_BUCKET` 等配置。当前项目已支持自动加载 `server/.env` 和 `server/.env.local`。

### 3. 启动后端服务

```bash
cd server
APP_ENV=local go run ./cmd/roomserver
APP_ENV=prod go run ./cmd/roomserver
```

### 4. 生成并上传媒体资源

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

如果只想分阶段执行，也可以分别使用 `plan`、`build-hls`、`upload`、`write-db`。

### 5. 启动 Android 客户端

用 Android Studio 打开 `android/`，运行到模拟器或真机即可。当前推荐联调方式：

- 一个 Android 模拟器
- 一台真实 Android 设备

## 当前主线

- 完成云服务器部署第一版
- 用云上 PostgreSQL + MinIO + roomserver 跑通真实联调
- 继续优化播放器在高倍速和弱网场景下的稳定性

## 文档入口

- [后端接口契约](./docs/backend-api-contract.md)
- [播放器技术笔记](./docs/core-sync-technical-notes.md)
- [媒体存储与分发](./docs/media-storage-and-delivery.md)
- [mediactl 操作说明](./docs/mediactl-operations.md)
- [环境配置说明](./docs/environment-config.md)
