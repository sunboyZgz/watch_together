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
- `media_episodes.media_url` 指向 `hls/master.m3u8` 的公开 URL。
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
