# Media

`media/` 用于放置本地开发与联调所需的媒体样例资源。

当前阶段，这个目录主要服务于 `INT-8 prepare sample HLS asset`，为 Android 播放器验证和后续同步开发提供固定输入。

## Current Layout

```text
media/
├─ source/
│  └─ sample_001.mp4
└─ tmp/
   └─ sample_001/
      ├─ index.m3u8
      └─ segment_*.ts
```

## Sample Asset Convention

- 固定 `mediaId`: `sample_001`
- 原始测试视频：`media/source/sample_001.mp4`
- HLS 输出目录：`media/tmp/sample_001/`
- 播放清单入口：`media/tmp/sample_001/index.m3u8`

## Local Access Convention

如果从仓库根目录启动一个本地静态文件服务，例如：

```bash
python3 -m http.server 9000
```

则当前样例资源的本地访问地址为：

```text
http://127.0.0.1:9000/media/tmp/sample_001/index.m3u8
```

当前 Android 与 Windows 的示例配置也应基于这个路径约定：

- `MEDIA_BASE_URL=http://127.0.0.1:9000/media/tmp`
- `MEDIA_DEFAULT_ID=sample_001`

最终播放器侧拼接出的地址形式为：

```text
${MEDIA_BASE_URL}/${MEDIA_DEFAULT_ID}/index.m3u8
```

## Maintenance Rule

- 当前样例资源主要服务于本地开发与联调，不作为 GitHub 仓库内的正式版本化资产。
- `media/tmp/` 与 `media/source/` 默认视为本地媒体工作区，由根目录 `.gitignore` 忽略。
- 这些本地样例文件可以用于播放器验证与同步联调，但不要求提交到远端仓库。
- 如果后续需要一份可提交的长期样例资产，应单独约定固定目录，而不是继续复用 `media/tmp/`。
