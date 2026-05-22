# Media Storage And Delivery

本文记录 `watch_together` 的媒体资源存储、HLS 分发和后续媒体入库工具方向。

## 背景

当前项目的测试资源主要放在仓库 `media/` 目录下。数据库中的 `media_episodes.media_url` 保存真实 HLS 地址；Android 通过后端返回的短期签名 `mediaUrl` 播放 HLS，本地联调时仍可通过 `MEDIA_BASE_URL` 做地址改写。

随着项目进入真实业务联调，媒体资源会逐步从本地样例迁移到云端对象存储和 CDN。这里需要提前确定三个边界：

- HLS 文件、封面图等大文件不进入 PostgreSQL。
- PostgreSQL 只保存媒体业务元数据和真实 HLS 地址或后续可签名资源身份。
- Android 只消费后端 API 返回的 `mediaUrl / coverUrl`，不关心底层对象存储供应商。

## 阶段性选型

### 开发阶段

默认使用本地静态服务。

适用场景：

- 本地 Android 模拟器联调。
- 本地 server API 联调。
- 快速验证 HLS master playlist、variant playlist 和 `.ts` 分片是否可播放。

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
- 如果开始做 `mediactl upload` 或 `mediactl ingest --stages=build-hls,upload`，MinIO 就很有价值。

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
当前公网前加固已经要求 `GET /media/items` 和 `GET /rooms/{roomCode}` 携带登录 token，避免匿名枚举播放 URL；API 返回的 `mediaUrl` 也已改为短期签名播放入口 `/media/playback/{episodeId}/master.m3u8?expires=...&sig=...`，不再直接暴露 `media_episodes.media_url`。
这还不是完整的对象存储保护；生产前仍需要让真实 HLS playlist / segment 字节层接入 signed URL / signed cookie / CDN 鉴权。
后续如果支持用户读取自己的本地目录，媒体保护边界会不同：本地目录模式不需要对象存储 signed URL，服务端只同步媒体指纹、展示名、时长和播放时间轴；客户端负责在自己的本地库中找到对应文件。

## 媒体分发模式配置方案

作为开源自托管项目，媒体分发不应该只有一种“最安全但部署最重”的方案。后续 Server 应明确支持多种 delivery mode，让部署者在安全性、部署复杂度和带宽成本之间做取舍。

建议配置项：

```text
MEDIA_DELIVERY_MODE=signed_redirect|minio_presign|nginx_auth_request
MEDIA_PLAYBACK_SIGNING_SECRET=...
MEDIA_PLAYBACK_URL_TTL_SECONDS=7200
MEDIA_PUBLIC_BASE_URL=...
MEDIA_INTERNAL_BASE_URL=...
```

| Mode | 适用场景 | 是否需要 Nginx | 安全性 | Go 是否转发视频字节 | 说明 |
| -- | -- | -- | -- | -- | -- |
| `signed_redirect` | 当前已实现的过渡方案 | 否 | 中低 | 否 | API 返回短期签名播放入口，Go 校验后 302 到真实 HLS URL。能避免 API 直接暴露永久 URL，但跳转后的真实 URL 仍可能被看到。 |
| `minio_presign` | 只有 MinIO、不想部署 Nginx，但希望 bucket 私有 | 否 | 中 | 否 | Go 读取并重写 HLS playlist，把 variant playlist 继续指向 Go 签名入口，把 segment 改成 MinIO presigned URL；Go 不转发视频分片。 |
| `nginx_auth_request` | 无 CDN 的公网部署、希望 MinIO 私有且 Go 不承载字节流 | 是 | 高 | 否 | 推荐的无 CDN 生产方案：客户端先访问 Go 签名播放入口，Go 下发访问 cookie/凭证后跳转到 Nginx；Nginx 对外承载 HLS 字节流，再反代到私有 MinIO。 |

当前代码状态：

- 已实现 `MEDIA_DELIVERY_MODE` 分支配置，支持 `signed_redirect / minio_presign / nginx_auth_request`。
- `signed_redirect` 已可直接使用。
- `minio_presign` 已接入 S3-compatible client：playlist 文本由 Go 读取和重写，segment URL 由 MinIO presign 承担。
- `nginx_auth_request` 已接入 Go 侧签发、播放入口跳转和 `/media/internal/auth` 校验入口；Nginx 模板位于 `server/deploy/nginx/media-auth-request.conf.example`。
- 不再提供 `public_direct` 作为正式模式；真实 HLS URL 直出安全性过低，不进入项目推荐配置。
- Android 播放器侧已安装 JVM/HttpURLConnection 默认 `CookieManager`，用于支持 `nginx_auth_request` 模式下 Go 播放入口返回的媒体访问 cookie。

`nginx_auth_request` 的部署约束：

- 推荐让 API 播放入口和 Nginx 媒体入口处于同一 host，或至少处于可共享 cookie 的同一父域名。
- 如果 API 是 `api.example.com`，媒体是 `media.example.net` 这类无关域名，Go 下发的 cookie 不会自动发送给媒体域名。
- 如果必须使用不同子域名，后续需要在服务端支持 cookie `Domain=.example.com` 或改为 Nginx query token / secure_link 方案。

`nginx_auth_request` 参考配置：

```text
server/deploy/nginx/media-auth-request.conf.example
```

对应服务端环境变量示例：

```text
MEDIA_DELIVERY_MODE=nginx_auth_request
MEDIA_PLAYBACK_SIGNING_SECRET=<long-random-secret>
MEDIA_PLAYBACK_URL_TTL_SECONDS=7200
MEDIA_PUBLIC_BASE_URL=https://watch.example.com/watch-together-media
MEDIA_STORAGE_BUCKET=watch-together-media
```

如果使用该模式，公网边界必须由 Nginx + Go 鉴权承载，MinIO API 不应直接暴露到公网。需要注意：Nginx 反代 MinIO 对象时不会自动生成 S3 签名，所以 `server/compose.prod.yaml` 采用“MinIO 仅在 Docker 内网匿名读取 + Nginx 对外 auth_request 鉴权”的最小生产方案。直接把带匿名下载的 MinIO 端口暴露到公网是不安全的。

### 媒体对象存储适配层

Server 侧已经把对象存储访问从 `transport` 层抽出到 `internal/mediaobj`：

```text
internal/mediaobj/
  store.go      # ObjectStore interface
  s3_store.go   # MinIO / S3-compatible implementation
```

核心接口：

```go
type ObjectStore interface {
    GetObject(ctx context.Context, key string) (io.ReadCloser, error)
    PresignGetObject(ctx context.Context, key string, ttl time.Duration) (string, error)
}
```

当前 `minio_presign` 只依赖 `ObjectStore`，不再直接依赖 AWS SDK。后续扩展阿里 OSS、腾讯 COS、Cloudflare R2、本地文件目录或其他流媒体资源服务器时，应新增对应 `ObjectStore` 实现，而不是把供应商 SDK 写进 `transport/media_delivery.go`。

职责边界：

- `mediaobj`: 负责对象读取和对象级签名。
- `transport/media_delivery`: 负责播放入口签名、playlist 重写、Nginx auth cookie 和 HTTP 响应。
- `store/media_postgres`: 负责读取媒体业务元数据和当前真实 HLS 地址。

无 CDN 推荐演进路线：

1. 当前阶段继续使用 `signed_redirect`，先保证 Android、房间、播放链路稳定。
2. 把 `media_episodes.media_url` 从公开 URL 迁移为对象 key / storage identity，例如 `media/show/season-01/episode-01/hls/master.m3u8`。
3. 如果部署者不想上 Nginx，使用 `minio_presign`，并在文档中明确它的 playlist 重写和 presigned segment 边界。
4. 如果部署者愿意做公网加固，提供 MinIO 私有 bucket + Nginx `auth_request` / `secure_link` + Go 签发访问凭证。

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

当前已经实现：

- `local` driver：本地稳定目录落盘
- `minio` / `s3` driver：S3-compatible 上传抽象

真实 OSS 厂商选型仍放在后续评估任务中，不在这里提前绑定。

### Driver 语义

| Driver | 用途 | 是否当前默认 | 说明 |
| -- | -- | -- | -- |
| `local` | 本地静态目录 | 是 | 适合 Android 模拟器和本机 server 联调。 |
| `minio` | 本地 S3-compatible 预演 | 否 | 已支持上传，可验证 bucket、object key、上传和 public URL。 |
| `s3` | 云对象存储兼容层 | 否 | 已支持 S3-compatible 上传抽象，后续承接 OSS / COS / BOS / R2 评估。 |

## 推荐资源结构

对象存储中的资源建议按 `source_key` 去掉扩展名后的稳定路径组织。当前默认生成多码率 HLS，但不默认生成 4K。项目当前资源大多不是 4K，而且 Android 同步播放器的首要目标是稳定 720p 以上播放体验。

```text
media/
└── {sourceKeyWithoutExt}/
    ├── hls/
    │   ├── master.m3u8
    │   ├── 720p-fast/
    │   │   ├── index.m3u8
    │   │   └── segment_00001.ts
    │   ├── 720p-high/
    │   │   ├── index.m3u8
    │   │   └── segment_00001.ts
    │   └── 1080p/
    │       ├── index.m3u8
    │       └── segment_00001.ts
    └── cover/
        └── cover.jpg
```

说明：

- `master.m3u8` 是 Android 播放入口。
- `720p-fast/index.m3u8` 是默认稳定 variant，面向移动端和 1.5x / 2.0x 高倍速同步播放。
- `720p-high/index.m3u8` 是 720p 画质优先 variant，供 buffer 健康时 ABR 升档。
- `1080p/index.m3u8` 仅在源视频高度足够时生成。
- 4K 不进入默认 ladder，后续只有在明确有 4K 资源和播放端性能验证后再追加。

## Object Key 规范

object key 必须稳定、可预测，并且不包含用户本地文件名。

当前约定：

```text
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/hls/master.m3u8
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/hls/720p-fast/index.m3u8
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/hls/720p-fast/segment_00001.ts
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/hls/720p-high/index.m3u8
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/hls/720p-high/segment_00001.ts
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/hls/1080p/index.m3u8
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/hls/1080p/segment_00001.ts
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/cover/cover.jpg
```

其中：

- `MEDIA_OBJECT_KEY_PREFIX` 默认是 `media`。
- `{sourceKeyWithoutExt}` 由媒体库相对路径自动推导，例如 `sample-show/season-01/episode-01`。
- `media_episodes.media_url` 当前指向真实 `hls/master.m3u8` 地址；REST API 返回给 Android 的 `mediaUrl` 会先包装成短期签名播放入口。
- `media_episode_variants` 记录同一个 episode 下的 `720p-fast / 720p-high / 1080p` variant playlist URL、分辨率、带宽和 segment 健康信息。
- `media_episodes.cover_url` 或 `media_seasons.cover_url` 指向封面 URL。

本地静态服务下的 URL 示例：

```text
http://127.0.0.1:9000/media/tmp/media/sample-show/season-01/episode-01/hls/master.m3u8
http://127.0.0.1:9000/media/tmp/media/sample-show/season-01/episode-01/cover/cover.jpg
```

MinIO / S3-compatible 下的 URL 示例：

```text
https://cdn.example.com/media/sample-show/season-01/episode-01/hls/master.m3u8
https://cdn.example.com/media/sample-show/season-01/episode-01/cover/cover.jpg
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
- `media_episode_variants.variant_key`
- `media_episode_variants.playlist_url`
- `media_episode_variants.width`
- `media_episode_variants.height`
- `media_episode_variants.bandwidth_bps`
- `media_episode_variants.codecs`
- `media_tags`
- `media_season_tags`

PostgreSQL 不保存：

- HLS 分片内容
- `m3u8` 文件内容
- 视频二进制
- 封面图片二进制
- 对象存储上传临时文件

Android 本地播放器 cache 也不属于 PostgreSQL 或服务端资源存储：

- Android 使用 Media3 `SimpleCache` 缓存已下载的 HLS playlist / segment。
- cache 目录位于应用 `cacheDir/watch_together_media_cache`，当前上限 `512MB`，由 LRU 淘汰。
- 该 cache 主要解决 seek、rejoin、重复进入房间、短时间重播和后续 ahead prefetch 时的重复请求复用。
- 该 cache 不是离线下载，不保证长期存在，也不应该作为业务数据来源。

Android ahead prefetch 基于同一个 cache 工作：

- 播放器会解析 VOD HLS playlist，并根据当前播放位置估算当前 segment。
- 在 1.5x、2.0x、rebuffer 较多或 effective buffer 偏低时，提前把未来有限 segment 写入 Media3 cache。
- 第一版只预取当前将要播放的 variant，不做多 variant 大规模预取。
- 预取窗口是性能策略，不会写入 PostgreSQL，也不会改变服务端 HLS 文件结构。

## HLS 制作规范

媒体源文件应通过 `ffmpeg` 转成 HLS。

当前建议：

- HLS segment 时长：4 到 6 秒。
- 输出 `master.m3u8`。
- 默认输出 `720p-fast / 720p-high / 1080p` variant，其中 `1080p` 会在源视频高度不足时自动跳过。
- 源视频高度低于 720p 时直接失败，因为 720p 是当前产品最低清晰度基线。
- 输出 `.ts` 分片，第一版优先 `.ts`，兼容性更直接。
- 保留源视频时长，写入 `media_episodes.duration_ms`。
- 生成或接收封面图，写入 `media_episodes.cover_url` 或 `media_seasons.cover_url`。
- 生成后通过 `ffprobe` 校验每个 variant 的实际 `width / height`，不符合目标清晰度则失败。

手工调试时可以先理解为每个 variant 独立执行一次 ffmpeg，再由 `mediactl` 写出 `master.m3u8`。实际生产命令以 `mediactl ingest` 为准，不建议长期手写分散命令。

```bash
ffmpeg -i input.mp4 \
  -vf scale=-2:720 \
  -c:v libx264 \
  -preset veryfast \
  -crf 24 \
  -profile:v main \
  -level 3.1 \
  -pix_fmt yuv420p \
  -force_key_frames "expr:gte(t,n_forced*6)" \
  -sc_threshold 0 \
  -tune fastdecode \
  -bf 0 \
  -refs 1 \
  -maxrate 2200k \
  -bufsize 4400k \
  -c:a aac \
  -b:a 128k \
  -hls_time 6 \
  -hls_playlist_type vod \
  -hls_flags independent_segments \
  -hls_segment_filename "segment_%05d.ts" \
  720p/index.m3u8
```

注意：HLS 分片应尽量和关键帧对齐。只设置 `-hls_time 6` 不一定能得到稳定 6 秒分片，如果源视频关键帧间隔不规则，playlist 可能出现 9 到 10 秒长分片和 1 秒左右短分片。2 倍速播放会放大这种不稳定，Android 模拟器更容易进入 rebuffer。

当前 `mediactl ingest --dry-run=false` 会固化多码率 HLS 生成参数，避免手工命令分散。

第一版实际命令语义：

- 使用 `ffmpeg` 生成 VOD HLS。
- 默认 segment 时长为 6 秒，可通过 `--hls-segment-seconds` 在 4 到 6 秒之间调整。
- 输出 `master.m3u8`。
- 输出 `720p-fast/index.m3u8`、`720p-high/index.m3u8` 和必要时的 `1080p/index.m3u8`。
- 输出各 variant 目录下的 `segment_%05d.ts`。
- 使用 `ffprobe` 读取源视频时长，后续写入 `media_episodes.duration_ms`。
- 使用 `ffprobe` 读取源视频和生成结果的分辨率，确认 variant 清晰度符合 `720p / 1080p` 要求。
- 解析生成后的 HLS playlist，记录 segment 数量和平均 segment 时长；如果分片结构明显异常则失败。
- 自动根据 `--library-root + --input` 推导 `source_key`。
- 自动根据源文件内容计算 `source_hash`。
- 不负责上传，上传由 `INT-141` 补齐。
- 传入 `--write-db` 后可写入 PostgreSQL，包含 `media_seasons / media_episodes / media_episode_variants / media_tags / media_season_tags`。

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
  --input ../media/raw/sample-show/season-01/episode-01.mp4 \
  --title "测试视频" \
  --season-label "第 1 季" \
  --episode-label "第 01 集" \
  --tags test,anime \
  --dry-run=false
```

第一版职责：

- 校验输入视频存在。
- 校验媒体库根目录存在。
- 根据规范目录结构自动推导 `source_key / season_number / episode_number`。
- 根据源文件内容自动计算 `source_hash`。
- 读取媒体存储相关环境变量。
- 输出 dry-run summary。
- 在 `--dry-run=false` 时调用 `ffmpeg` 输出多码率 HLS 和 master playlist。
- 在 `--dry-run=false` 时调用 `ffprobe` 读取源视频时长。
- 在 `--dry-run=false --write-db` 时写入或更新 `media_seasons / media_episodes / media_episode_variants`，并写入 `media_tags / media_season_tags`。

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
media/{sourceKeyWithoutExt}/hls/master.m3u8
```

那么本地文件路径应为：

```text
media/tmp/media/{sourceKeyWithoutExt}/hls/master.m3u8
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
