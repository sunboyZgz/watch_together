# Mediactl Operations

本文是 `mediactl` CLI 的操作说明书，面向本地开发、媒体资源制作和后续生产前预演。

`mediactl` 目前位于：

```text
server/cmd/mediactl
```

当前已实现：

- `mediactl ingest` dry-run。
- 本地单码率 HLS 生成。
- 源视频时长探测。
- 基于媒体库目录自动推导 `source_key`。
- 基于源文件内容自动计算 `source_hash`。
- PostgreSQL season/episode 元数据 upsert，需要显式传 `--write-db`。

当前未实现：

- 上传到 MinIO / S3 / OSS / COS / BOS / R2。
- 多码率 HLS。
- 字幕、音轨、封面自动截图。

## 前置条件

### 1. 安装 ffmpeg

本地需要可执行的 `ffmpeg` 和 `ffprobe`。

macOS 可使用：

```bash
brew install ffmpeg
```

确认：

```bash
ffmpeg -version
ffprobe -version
```

### 2. 配置环境变量

默认配置记录在：

- `server/.env.example`
- `docs/environment-config.md`
- `docs/media-storage-and-delivery.md`

当前本地推荐：

```bash
export MEDIA_STORAGE_DRIVER=local
export MEDIA_LOCAL_ROOT=../media/tmp
export MEDIA_PUBLIC_BASE_URL=http://127.0.0.1:9000/media/tmp
export MEDIA_OBJECT_KEY_PREFIX=media
export FFMPEG_BIN=ffmpeg
export FFPROBE_BIN=ffprobe
export DATABASE_URL='postgres://app:app@127.0.0.1:5432/anime_watch_dev?sslmode=disable'
```

如果本机 `ffmpeg` 不在 PATH，可以显式指定：

```bash
export FFMPEG_BIN=/opt/homebrew/bin/ffmpeg
export FFPROBE_BIN=/opt/homebrew/bin/ffprobe
```

## 当前命令

### Dry-run

dry-run 不会写文件、不会上传、不会写数据库，适合先检查参数和配置。

```bash
cd server
go run ./cmd/mediactl ingest \
  --library-root ../media/raw \
  --input ../media/raw/violet-evergarden/season-01/episode-09.mp4 \
  --title "紫罗兰永恒花园" \
  --season-label "第 1 季" \
  --episode-label "第 09 集" \
  --tags healing,anime
```

输出会包含：

- 输入文件路径
- `sourceKey`
- `sourceHash`
- 从目录解析出的 `seasonSlug / seasonNumber / episodeNumber`
- 标题、季度、集数
- tags
- 当前 storage config
- 计划输出的 `hlsPlaylistPath`

### 生成本地 HLS

传入 `--dry-run=false` 后，CLI 会调用 `ffmpeg` 生成单码率 HLS。

```bash
cd server
go run ./cmd/mediactl ingest \
  --library-root ../media/raw \
  --input ../media/raw/violet-evergarden/season-01/episode-09.mp4 \
  --title "紫罗兰永恒花园" \
  --season-label "第 1 季" \
  --episode-label "第 09 集" \
  --tags healing,anime \
  --dry-run=false
```

默认输出目录：

```text
{MEDIA_LOCAL_ROOT}/{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExt}/hls/
```

例如：

```text
../media/tmp/media/violet-evergarden/season-01/episode-09/hls/
```

默认产物：

```text
index.m3u8
segment_00000.ts
segment_00001.ts
...
```

输出 summary 中会包含：

- `hlsPlaylistPath`
- `mediaUrl`
- `durationMs`

### 指定输出目录

如果只想临时测试，不想写到默认 `MEDIA_LOCAL_ROOT`，可以使用：

```bash
go run ./cmd/mediactl ingest \
  --input /path/to/source.mp4 \
  --title "测试视频" \
  --output-dir /tmp/watch-media/test_media/hls \
  --dry-run=false
```

`--output-dir` 适合本地试跑。正式入库流程建议不要使用它，让 CLI 根据 `source_key` 生成稳定目录。

### 写入 PostgreSQL

传入 `--write-db` 后，CLI 会在 HLS 生成成功后写入 PostgreSQL。

```bash
cd server
go run ./cmd/mediactl ingest \
  --library-root ../media/raw \
  --input ../media/raw/violet-evergarden/season-01/episode-09.mp4 \
  --title "紫罗兰永恒花园" \
  --subtitle "和搭子一起继续看" \
  --description "治愈系作品，适合夜晚慢慢看。" \
  --category anime \
  --original-title "Violet Evergarden" \
  --production-team "Kyoto Animation" \
  --search-aliases "紫罗兰,薇尔莉特,京阿尼" \
  --season-label "第 1 季" \
  --episode-label "第 09 集" \
  --tags healing,theatrical \
  --dry-run=false \
  --write-db
```

写入内容：

- upsert `media_seasons`
- upsert `media_episodes`
- upsert `media_tags`
- replace 当前 season 的 `media_season_tags`

`--write-db` 需要：

- `--dry-run=false`
- `DATABASE_URL` 或 `--database-url`

当前 `cover_url` 仍由后续上传阶段补齐；如果重复导入同一个 `source_key`，已有 `cover_url` 不会被空值覆盖。

## 参数说明

| 参数 | 必填 | 当前作用 |
| -- | -- | -- |
| `--input` | 是 | 源视频文件路径，必须存在。 |
| `--library-root` | 是 | 媒体库根目录，用于从 `--input` 自动推导 `source_key`。 |
| `--title` | 是 | 媒体标题。 |
| `--media-id` | 否 | 兼容期 legacy override；常规生产流程不要传。 |
| `--subtitle` | 否 | 媒体副标题。 |
| `--description` | 否 | 媒体简介。 |
| `--category` | 否 | 媒体分类，例如 `anime`。 |
| `--original-title` | 否 | 原始标题。 |
| `--production-team` | 否 | 制作团队或工作室。 |
| `--search-aliases` | 否 | 逗号分隔搜索别名，写入 `media_seasons.search_aliases`。 |
| `--season-label` | 否 | 季度展示文本。 |
| `--episode-label` | 否 | 集数展示文本。 |
| `--tags` | 否 | 逗号分隔标签；写库时会进入 `media_tags / media_season_tags`。 |
| `--cover` | 否 | 封面文件路径，当前只校验存在，后续由 `INT-141` 上传。 |
| `--output-dir` | 否 | 覆盖 HLS 输出目录，适合临时测试。 |
| `--hls-segment-seconds` | 否 | HLS 分片时长，允许 4 到 6 秒，默认 6。 |
| `--upload` | 否 | 当前只记录上传意图，后续由 `INT-141` 实现。 |
| `--write-db` | 否 | HLS 成功生成后写入 PostgreSQL。 |
| `--database-url` | 否 | 覆盖 `DATABASE_URL`。 |
| `--dry-run` | 否 | 默认 `true`。设为 `false` 时执行本地 HLS 生成。 |

## HLS 生成规则

当前规则：

- 单码率 HLS。
- VOD playlist。
- 输出 `index.m3u8`。
- 输出 `.ts` 分片。
- 分片命名：`segment_%05d.ts`。
- 默认分片时长：6 秒。
- 允许分片时长：4 到 6 秒。
- 使用 `ffprobe` 读取 `durationMs`。

当前不做：

- 多码率 master playlist。
- DRM。
- 字幕处理。
- 多音轨处理。
- 自动封面截图。
- 视频画质参数精细调优。

## 本地播放验证

如果 HLS 输出到了：

```text
media/tmp/media/media_uuid/hls/index.m3u8
```

可以在仓库根目录启动静态服务：

```bash
python3 -m http.server 9000
```

本机访问地址：

```text
http://127.0.0.1:9000/media/tmp/media/media_uuid/hls/index.m3u8
```

Android 模拟器访问宿主机时通常使用：

```text
http://10.0.2.2:9000/media/tmp/media/media_uuid/hls/index.m3u8
```

## 与数据库的关系

当前 `mediactl` 支持通过 `--write-db` 写 PostgreSQL。

写入字段包括：

- `media_seasons.slug`
- `media_seasons.title`
- `media_seasons.description`
- `media_seasons.category`
- `media_seasons.original_title`
- `media_seasons.production_team`
- `media_seasons.search_aliases`
- `media_seasons.season_number`
- `media_seasons.season_label`
- `media_episodes.title`
- `media_episodes.subtitle`
- `media_episodes.description`
- `media_episodes.media_url`
- `media_episodes.duration_ms`
- `media_episodes.episode_number`
- `media_episodes.episode_label`
- `media_episodes.source_key`
- `media_episodes.source_hash`
- `media_tags`
- `media_season_tags`

当前 `cover_url` 仍等待 `INT-141` 上传阶段补齐。

## 与上传的关系

当前 `mediactl` 不上传文件。

后续 `INT-141` 会补：

- local uploader
- MinIO / S3-compatible uploader
- public URL 生成
- cover 文件复制或上传

当前 `--upload` 只表达“后续希望走上传链路”，不会产生副作用。

## 常见问题

### `--library-root` 与目录规范

常规生产流程不手动输入 `source_key / source_hash`：

- `source_key` 由 `--input` 相对 `--library-root` 的路径推导
- `source_hash` 由源文件内容 SHA-256 自动计算

推荐目录结构：

```text
media/raw/
└── violet-evergarden/
    └── season-01/
        └── episode-09.mp4
```

示例：

```bash
go run ./cmd/mediactl ingest \
  --library-root ../media/raw \
  --input ../media/raw/violet-evergarden/season-01/episode-09.mp4 \
  --title "紫罗兰永恒花园"
```

推导结果：

```text
source_key = violet-evergarden/season-01/episode-09.mp4
season_slug = violet-evergarden
season_number = 1
episode_number = 9
source_hash = sha256:<file-content-hash>
```

如果 `--input` 不在 `--library-root` 内，CLI 会拒绝执行。

路径规范：

- 每个路径片段只允许小写英文字母、数字、点、短横线或下划线。
- 推荐 season 目录使用 `season-01` 这类格式。
- 推荐 episode 文件名使用 `episode-09.mp4` 这类格式。
- 不建议在媒体库目录和文件名里使用中文、空格或大写字母；这些信息应进入 `title / original-title / search-aliases` 等元数据字段。

### 找不到 ffmpeg 或 ffprobe

检查：

```bash
which ffmpeg
which ffprobe
```

或者设置：

```bash
export FFMPEG_BIN=/absolute/path/to/ffmpeg
export FFPROBE_BIN=/absolute/path/to/ffprobe
```

### Android 播放不了本地 HLS

优先检查：

- 静态服务是否启动。
- Android 使用的是 `10.0.2.2` 而不是 `127.0.0.1`。
- `index.m3u8` 和 `.ts` 分片是否都能通过浏览器访问。
- PostgreSQL 中 `media_episodes.media_url` 是否指向 `index.m3u8`。

## 后续任务

- `INT-141`: 增加 local / MinIO 上传抽象。
- `INT-144`: 验证 HLS ingest 到 Android 播放完整链路。
