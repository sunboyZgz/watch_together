# Android

`android/` 用于放置 Android 客户端工程。

当前已经落地的内容包括：

- Kotlin + Compose Android 应用工程
- 基于 AndroidX Media3 ExoPlayer 的播放器适配层
- 播放器事件回调与调试面板
- 基于 `POST /rooms` + `/ws` 的 Android 首个 join-time initial state sync 实现
- Android 侧最小协议草案与同步状态模型

当前目录内的关键职责：

- `app/src/main/java/.../ui/player/`：播放器页面、播放器适配器、播放器事件
- `app/src/main/java/.../sync/`：建房、入房、协议解码和 join 后初始状态应用
- `app/src/main/java/.../sync/protocol/`：Android 侧最小协议草案模型
- `app/src/main/java/.../config/`：Android 配置注入入口

## Current Structure

```text
android/
├── app/
│   └── src/main/java/com/example/watch_together/
│       ├── config/
│       │   └── AppConfig.kt
│       ├── sync/
│       │   ├── RoomHttpClient.kt
│       │   ├── RoomSyncCoordinator.kt
│       │   ├── RoomSyncState.kt
│       │   ├── RoomWebSocketClient.kt
│       │   ├── SyncMessageDecoder.kt
│       │   └── protocol/
│       ├── ui/
│       │   ├── player/
│       │   └── theme/
│       └── MainActivity.kt
│   └── src/test/java/com/example/watch_together/sync/
├── gradle/
│   └── libs.versions.toml
└── README.md
```

各部分职责：

- `config/`：统一读取 `BuildConfig` 并生成 Android 端可直接使用的 URL
- `sync/`：当前阶段的 Android 首个同步接入层，负责 create room、join room、消息解码与状态应用
- `sync/protocol/`：保留与 `INT-19` 协议草案一致的 Android 本地协议模型
- `ui/player/`：播放器页面、Media3 适配器、播放器事件和调试面板
- `src/test/.../sync/`：协议解码和 join-time state application 的最小单元测试

当前实现使用的核心库：

- Player: AndroidX Media3 ExoPlayer
- Network: OkHttp

在 Phase 1 中，这里会继续承载 Android ↔ Android 同步观影 MVP 的主要客户端实现。
