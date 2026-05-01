# Media Storage And Delivery

本文记录 `watch_together` 的媒体资源存储、HLS 分发和后续媒体入库工具方向。

## 背景

当前项目的测试资源主要放在仓库 `media/` 目录下，Android 通过后端返回的 `media_episodes.media_url` 或本地 `MEDIA_BASE_URL` 播放 HLS。

随着项目进入真实业务联调，媒体资源会逐步从本地样例迁移到云端对象存储和 CDN。这里需要提前确定三个边界：

- HLS 文件、封面图等大文件不进入 PostgreSQL。
- PostgreSQL 只保存媒体业务元数据和可播放 URL。
- Android 只消费后端 API 返回的 `mediaUrl / coverUrl`，不关心底层对象存储供应商。

## 阶段性选型

### 开发阶段

默认使用本地静态服务。

适用场景：

- 本地 Android 模拟器联调。
- 本地 server API 联调。
- 快速验证 HLS 分片和 `index.m3u8` 是否可播放。

当前推荐：

- 继续保留 `media/` 目录作为本地样例资源入口。
- 使用简单静态服务暴露 HLS，例如 `python3 -m http.server`、Go 静态服务或现有本地静态资源服务。
- Android 使用 `MEDIA_BASE_URL` 指向本地资源根地址。

### 开发后期 / S3 兼容预演

可引入 MinIO，但不作为第一默认路径。

适用场景：

- 需要提前验证 S3 SDK、bucket、object key、presign、上传流程。
- 需要模拟生产对象存储目录结构。
- 需要在本地跑媒体入库 CLI 的完整上传链路。

当前定位：

- MinIO 是“云存储兼容层预演”，不是必须依赖。
- 如果只是播放本地 HLS，静态服务更简单。
- 如果开始做 `mediactl ingest --upload`，MinIO 就很有价值。

### 公网测试 / 生产阶段

使用对象存储 + CDN。

候选：

- 阿里云 OSS
- 腾讯 COS
- 百度 BOS
- Cloudflare R2

当前建议：

- 国内公网测试或生产：优先选择离主要用户更近、备案和 CDN 接入更顺的云厂商，例如 OSS / COS / BOS。
- 海外测试、成本敏感或希望 S3 兼容简洁：优先评估 Cloudflare R2。
- 不在 Android 中固化供应商 URL，由后端返回最终 `mediaUrl / coverUrl`。

## Runtime Config Boundary

媒体存储配置只属于 Server / CLI，不属于 Android 运行时配置。

Android 的播放入口来自后端 API 返回的：

- `media.mediaUrl`
- `media.coverUrl`

Android 不应该直接读取对象存储 bucket、endpoint、access key、secret key，也不应该根据供应商拼接播放地址。

当前 Server / CLI 侧统一使用这些环境变量：

```text
MEDIA_STORAGE_DRIVER=local|minio|s3
MEDIA_LOCAL_ROOT=../media/tmp
MEDIA_PUBLIC_BASE_URL=http://127.0.0.1:9000/media/tmp
MEDIA_OBJECT_KEY_PREFIX=media
MEDIA_STORAGE_ENDPOINT=
MEDIA_STORAGE_BUCKET=
MEDIA_STORAGE_REGION=
MEDIA_STORAGE_ACCESS_KEY_ID=
MEDIA_STORAGE_SECRET_ACCESS_KEY=
MEDIA_STORAGE_FORCE_PATH_STYLE=true
FFMPEG_BIN=ffmpeg
FFPROBE_BIN=ffprobe
```

第一版只要求 `local` driver 能跑通。`minio` / `s3` driver 作为后续上传抽象预留。

### Driver 语义

| Driver | 用途 | 是否当前默认 | 说明 |
| -- | -- | -- | -- |
| `local` | 本地静态目录 | 是 | 适合 Android 模拟器和本机 server 联调。 |
| `minio` | 本地 S3-compatible 预演 | 否 | 适合验证 bucket、object key、上传和 public URL。 |
| `s3` | 云对象存储兼容层 | 否 | 后续可承接 OSS / COS / BOS / R2 的兼容接入。 |

## 推荐资源结构

对象存储中的资源建议按媒体 ID 组织：

```text
media/
└── {mediaItemId}/
    ├── hls/
    │   ├── master.m3u8
    │   ├── 720p/
    │   │   ├── index.m3u8
    │   │   └── segment_00001.ts
    │   └── 1080p/
    │       ├── index.m3u8
    │       └── segment_00001.ts
    └── cover.jpg
```

第一版可以只生成单码率 HLS：

```text
media/
└── {mediaItemId}/
    ├── hls/
    │   ├── index.m3u8
    │   └── segment_00001.ts
    └── cover/
        └── cover.jpg
```

后续如果要支持多清晰度，再引入 master playlist。

## Object Key 规范

object key 必须稳定、可预测，并且不包含用户本地文件名。

第一版约定：

```text
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/hls/index.m3u8
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/hls/segment_00001.ts
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/cover/cover.jpg
```

其中：

- `MEDIA_OBJECT_KEY_PREFIX` 默认是 `media`。
- `{sourceKeyWithoutExt}` 由媒体库相对路径自动推导，例如 `violet-evergarden/season-01/episode-09`。
- `media_episodes.media_url` 指向 `hls/index.m3u8` 的公开 URL。
- `media_episodes.cover_url` 或 `media_seasons.cover_url` 指向封面 URL。

本地静态服务下的 URL 示例：

```text
http://127.0.0.1:9000/media/tmp/media/violet-evergarden/season-01/episode-09/hls/index.m3u8
http://127.0.0.1:9000/media/tmp/media/violet-evergarden/season-01/episode-09/cover/cover.jpg
```

MinIO / S3-compatible 下的 URL 示例：

```text
https://cdn.example.com/media/violet-evergarden/season-01/episode-09/hls/index.m3u8
https://cdn.example.com/media/violet-evergarden/season-01/episode-09/cover/cover.jpg
```

这里的 `cdn.example.com` 可以是 CDN 域名，也可以是对象存储 public endpoint。

## 数据库存储边界

PostgreSQL 保存业务元数据：

- `media_seasons.title`
- `media_seasons.description`
- `media_seasons.cover_url`
- `media_seasons.season_label`
- `media_episodes.title`
- `media_episodes.subtitle`
- `media_episodes.media_url`
- `media_episodes.duration_ms`
- `media_episodes.episode_label`
- `media_episodes.source_key`
- `media_episodes.source_hash`
- `media_tags`
- `media_season_tags`

PostgreSQL 不保存：

- HLS 分片内容
- `m3u8` 文件内容
- 视频二进制
- 封面图片二进制
- 对象存储上传临时文件

## HLS 制作规范

媒体源文件应通过 `ffmpeg` 转成 HLS。

第一版建议：

- HLS segment 时长：4 到 6 秒。
- 输出 `index.m3u8`。
- 输出 `.ts` 或 `.m4s` 分片，第一版优先 `.ts`，兼容性更直接。
- 保留源视频时长，写入 `media_episodes.duration_ms`。
- 生成或接收封面图，写入 `media_episodes.cover_url` 或 `media_seasons.cover_url`。

示例方向：

```bash
ffmpeg -i input.mp4 \
  -c:v h264 \
  -c:a aac \
  -hls_time 6 \
  -hls_playlist_type vod \
  -hls_segment_filename "segment_%05d.ts" \
  index.m3u8
```

当前 `mediactl ingest --dry-run=false` 会固化第一版单码率 HLS 生成参数，避免手工命令分散。

第一版实际命令语义：

- 使用 `ffmpeg` 生成 VOD HLS。
- 默认 segment 时长为 6 秒，可通过 `--hls-segment-seconds` 在 4 到 6 秒之间调整。
- 输出 `index.m3u8`。
- 输出 `segment_%05d.ts`。
- 使用 `ffprobe` 读取源视频时长，后续写入 `media_episodes.duration_ms`。
- 自动根据 `--library-root + --input` 推导 `source_key`。
- 自动根据源文件内容计算 `source_hash`。
- 不负责上传，上传由 `INT-141` 补齐。
- 传入 `--write-db` 后可写入 PostgreSQL，当前由 `INT-140` 落地。

## CLI-first 入库工具

当前更推荐先做 CLI，而不是直接做后台管理。

原因：

- CLI 更适合批量处理本地视频资源。
- CLI 更容易串联 `ffmpeg -> 上传 -> 写 PostgreSQL`。
- CLI 可复用在本地、测试环境和后续 CI/运维脚本。
- 管理后台可以在入库流程稳定后再做。

建议工具名：

```text
server/cmd/mediactl
```

建议命令：

```bash
mediactl ingest \
  --library-root ../media/raw \
  --input ../media/raw/violet-evergarden/season-01/episode-09.mp4 \
  --title "紫罗兰永恒花园" \
  --season-label "第 1 季" \
  --episode-label "第 09 集" \
  --tags healing,anime \
  --dry-run=false \
  --upload
```

第一版职责：

- 校验输入视频存在。
- 校验媒体库根目录存在。
- 根据规范目录结构自动推导 `source_key / season_number / episode_number`。
- 根据源文件内容自动计算 `source_hash`。
- 读取媒体存储相关环境变量。
- 输出 dry-run summary。
- 在 `--dry-run=false` 时调用 `ffmpeg` 输出单码率 HLS。
- 在 `--dry-run=false` 时调用 `ffprobe` 读取源视频时长。
- 在 `--dry-run=false --write-db` 时写入或更新 `media_seasons / media_episodes`，并写入 `media_tags / media_season_tags`。

后续任务继续补齐：

- `INT-141`: 上传 HLS 和封面到目标存储。

## 配置建议

服务端或 CLI 环境变量统一如下：

```text
MEDIA_STORAGE_DRIVER=local|minio|s3
MEDIA_LOCAL_ROOT=../media/tmp
MEDIA_PUBLIC_BASE_URL=http://127.0.0.1:9000/media/tmp
MEDIA_OBJECT_KEY_PREFIX=media
MEDIA_STORAGE_ENDPOINT=
MEDIA_STORAGE_BUCKET=
MEDIA_STORAGE_REGION=
MEDIA_STORAGE_ACCESS_KEY_ID=
MEDIA_STORAGE_SECRET_ACCESS_KEY=
MEDIA_STORAGE_FORCE_PATH_STYLE=true
FFMPEG_BIN=ffmpeg
FFPROBE_BIN=ffprobe
```

Android 侧不直接使用这些变量。Android 继续通过 API 获取 `mediaUrl`。

### 本地静态服务示例

如果 `MEDIA_LOCAL_ROOT=../media/tmp`，并且 object key 是：

```text
media/{mediaItemId}/hls/index.m3u8
```

那么本地文件路径应为：

```text
media/tmp/media/{mediaItemId}/hls/index.m3u8
```

本地静态服务可以这样启动：

```bash
python3 -m http.server 9000
```

然后 `MEDIA_PUBLIC_BASE_URL` 可设为：

```text
http://127.0.0.1:9000/media/tmp
```

Android 模拟器如果直接访问宿主机，应使用：

```text
http://10.0.2.2:9000/media/tmp
```

## 当前决策

- 开发阶段继续使用本地静态服务作为默认媒体分发方式。
- MinIO 作为后续 S3 兼容上传链路预演，不强制立即引入。
- 公网测试和生产使用对象存储 + CDN。
- 国内部署优先评估 OSS / COS / BOS。
- 海外或中立公网测试优先评估 Cloudflare R2。
- 先做 `mediactl` CLI，再考虑后台管理页面。

## 后续任务方向

- 定义云端 object key 规范。
- 新增 `mediactl` CLI 工程入口。
- 新增 ffmpeg HLS 制作命令封装。
- 新增 local/minio/cloud storage uploader 抽象。
- 新增媒体元数据写入 PostgreSQL 能力。
- 补充 dev seed 与 ingest 的关系。
- 后续再规划媒体管理后台。
