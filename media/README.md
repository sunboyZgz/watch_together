# Media

`media/` 是本地开发、HLS 制作和 Android 联调用的媒体工作区。

当前目录需要和 `server/cmd/mediactl` 的推导规则保持一致：原始视频必须放在 `media/raw/{season-slug}/season-XX/episode-XX.ext` 下，CLI 会根据相对路径自动推导 `source_key / season_number / episode_number`。

## Current Layout

```text
media/
├─ raw/
│  └─ sample-show/
│     └─ season-01/
│        └─ episode-01.mp4
└─ tmp/
   ├─ media/
   │  └─ sample-show/
   │     └─ season-01/
   │        └─ episode-01/
   │           └─ hls/
   │              ├─ index.m3u8
   │              └─ segment_*.ts
   └─ sample_001/
      ├─ index.m3u8
      └─ segment_*.ts
```

说明：

- `media/raw/`：原始媒体库，作为 `mediactl --library-root`。
- `media/tmp/media/`：按 `source_key` 生成的 HLS 输出目录，建议后续主要使用这一类结构。
- `media/tmp/sample_001/`：早期固定样例输出目录，属于历史兼容测试产物，不再作为新 ingest 的推荐目录。

## Source Path Convention

推荐原始视频路径：

```text
media/raw/{season-slug}/season-XX/episode-XX.ext
```

当前样例：

```text
media/raw/sample-show/season-01/episode-01.mp4
```

`mediactl` 推导结果：

```text
source_key = sample-show/season-01/episode-01.mp4
season_slug = sample-show
season_number = 1
episode_number = 1
```

路径约束：

- `{season-slug}` 使用小写字母、数字和中划线，例如 `violet-evergarden`。
- `season-XX` 使用两位数字，例如 `season-01`。
- `episode-XX.ext` 使用两位数字，例如 `episode-09.mp4`。
- 路径组件只使用小写字母、数字、点、下划线和中划线。
- 不手动输入 `source_key`，由 `--library-root + --input` 自动推导。

## Ingest Example

从仓库根目录执行：

```bash
cd server

go run ./cmd/mediactl ingest \
  --library-root ../media/raw \
  --input ../media/raw/sample-show/season-01/episode-01.mp4 \
  --title "测试视频" \
  --subtitle "本地 2 倍速播放测试" \
  --category anime \
  --season-label "第 1 季" \
  --episode-label "第 01 集" \
  --tags test \
  --hls-segment-seconds 6 \
  --dry-run=false
```

默认 HLS 输出：

```text
media/tmp/media/sample-show/season-01/episode-01/hls/index.m3u8
```

## Local Access Convention

如果从仓库根目录启动一个本地静态文件服务，例如：

```bash
python3 -m http.server 9000
```

则当前规范输出的访问地址为：

```text
http://127.0.0.1:9000/media/tmp/media/sample-show/season-01/episode-01/hls/index.m3u8
```

Android 模拟器访问本机服务时使用：

```text
http://10.0.2.2:9000/media/tmp/media/sample-show/season-01/episode-01/hls/index.m3u8
```

如果使用 720p 诊断版本，则 Android 模拟器地址为：

```text
http://10.0.2.2:9000/media/tmp/media/sample-show/season-01/episode-01-720p/hls/index.m3u8
```

注意：`127.0.0.1` 在 Android 模拟器内代表模拟器自身，不代表宿主机 Mac。本机浏览器能打开 `127.0.0.1`，不等于 App 能打开。开发阶段如果直接写数据库，优先写 `10.0.2.2` 版本的 URL。

## Maintenance Rule

- `media/raw/` 和 `media/tmp/` 默认视为本地媒体工作区。
- 原始视频和生成的 HLS 不要求提交到远端仓库。
- 新增测试视频时优先放入 `media/raw/{season-slug}/season-XX/episode-XX.ext`。
- 后续如果接入云存储，`media/tmp/media/.../hls/` 中的内容会作为上传源。
