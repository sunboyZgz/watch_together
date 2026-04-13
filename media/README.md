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

- `MEDIA_BASE_URL=http://127.0.0.1:9000/media/hls`
- `MEDIA_DEFAULT_ID=sample_001`

最终播放器侧拼接出的地址形式为：

```text
${MEDIA_BASE_URL}/${MEDIA_DEFAULT_ID}/index.m3u8
```

## Maintenance Rule

- 当前仓库保留一份稳定、固定、可重复引用的 HLS 样例资源。
- 这些样例文件属于项目开发资产，可以提交到仓库。
- 后续临时转码目录、缓存和中间产物不应直接堆在当前样例目录里，应放到 `media/tmp/` 或其他忽略目录。
