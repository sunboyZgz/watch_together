# Environment Config

> Current-stage environment parameter catalog for `watch_together`.

本文档定义当前阶段的环境配置参数与模块级配置策略，并区分哪些属于环境配置，哪些属于业务规则常量。

本文件当前聚焦：

- 参数清单
- 参数用途
- 参数归属
- 模块级配置承载方式
- 配置文件提交规则
- 环境命名与变量命名规则
- 环境参数与业务常量的边界

本文件当前不展开：

- 具体 `.env` 文件模板
- 生产密钥管理
- CI/CD 注入
- 完整多环境部署方案

---

## Decision Principle

只有会因运行环境变化而变化的值，才应放入环境配置。

产品规则、业务边界和阶段限制，优先保留为代码常量或文档规则，而不是环境变量。

---

## Environment Naming

当前阶段统一使用以下环境名称：

- `local`: 本地开发与联调
- `dev`: 后续共享开发环境
- `prod`: 正式运行环境

当前仓库主要围绕 `local` 阶段设计，`dev` 和 `prod` 先保留命名约定。

## Variable Naming

环境变量统一使用大写蛇形命名，例如：

- `APP_ENV`
- `SERVER_PORT`
- `API_BASE_URL`
- `MEDIA_DEFAULT_ID`

---

## Per-module Config Strategy

### Server

`server/` 使用 `.env.example` + `.env.local` + 运行时环境变量。

策略说明：

- `.env.example` 用于提交默认模板和字段说明。
- `.env.local` 用于本地开发机的实际值，不提交到仓库。
- 运行时环境变量可用于覆盖本地文件或后续部署环境中的配置。

### Windows

`windows/` 使用 `.env.example` + `.env.local`。

策略说明：

- `.env.example` 用于提交示例配置。
- `.env.local` 用于本地联调时的真实地址和调试开关，不提交到仓库。
- 当前阶段不引入更复杂的配置系统，先保持轻量。

### Android

`android/` 不单独引入 `.env` 文件，使用 Gradle / BuildConfig / `local.properties` 注入配置。

策略说明：

- 面向版本化的默认配置，应通过 Gradle 或 BuildConfig 注入。
- 本地开发机相关的私有配置，优先放在 `local.properties`。
- 当前阶段先定义接入方式，不强行对齐到 `.env` 体系。

---

## File Commit Rules

以下规则适用于当前仓库：

- `.env.example` 可以提交。
- `.env.local` 不可以提交。
- 其他本地环境文件，例如实际 `.env`，默认不提交。
- Android 的 `local.properties` 不提交。
- 模板文件应只包含示例值，不包含生产 secret 或个人私有地址。

当前规则已由根目录 `.gitignore` 配合约束。

---

## Template And Doc Locations

当前约定的文档和模板文件位置：

- `docs/environment-config.md`: 环境参数与配置策略统一说明
- `media/README.md`: 样例媒体资源目录与本地访问约定
- `server/.env.example`: Server 配置模板
- `windows/.env.example`: Windows 配置模板

后续如增加更多配置模板，应继续沿用“模块内模板 + 仓库级文档说明”的方式维护。

---

## Environment Parameters

### 1. Server Runtime

#### `APP_ENV`

- 用途：标识当前运行环境名称
- 示例：`local` / `dev` / `prod`
- 适用模块：Server、Windows、Android

#### `SERVER_HOST`

- 用途：指定服务监听地址
- 示例：`0.0.0.0`
- 适用模块：Server

#### `SERVER_PORT`

- 用途：指定服务监听端口
- 示例：`8080`
- 适用模块：Server

#### `LOG_LEVEL`

- 用途：指定日志级别
- 示例：`debug` / `info` / `warn` / `error`
- 适用模块：Server

#### `DATABASE_URL`

- 用途：指定服务端连接 PostgreSQL 的完整连接串
- 示例：`postgres://app:app@127.0.0.1:5432/anime_watch_dev?sslmode=disable`
- 适用模块：Server

#### `CORS_ALLOWED_ORIGIN` `(optional)`

- 用途：限制允许访问服务端的客户端来源
- 说明：主要服务于本地开发和后续桌面前端联调
- 适用模块：Server

### 2. Client Connection

#### `API_BASE_URL`

- 用途：客户端访问 HTTP API 的根地址
- 示例：`http://127.0.0.1:8080`
- 适用模块：Windows、Android

#### `WS_BASE_URL`

- 用途：客户端访问 WebSocket 服务的根地址
- 示例：`ws://127.0.0.1:8080/ws`
- 适用模块：Windows、Android

### 3. Media Access

#### `MEDIA_BASE_URL`

- 用途：指定 HLS 媒体资源根地址
- 示例：`http://127.0.0.1:9000/media/tmp`
- 说明：当前仓库样例资源按 `${MEDIA_BASE_URL}/${MEDIA_DEFAULT_ID}/index.m3u8` 约定访问
- 适用模块：Server、Windows、Android

#### `MEDIA_DEFAULT_ID` `(optional)`

- 用途：本地联调时提供默认媒体 ID
- 说明：方便调试，不属于运行必须项
- 示例：`sample_001`
- 适用模块：Windows、Android

#### `MEDIA_STORAGE_DRIVER`

- 用途：指定媒体入库 CLI 使用的存储后端
- 示例：`local` / `minio` / `s3`
- 默认：`local`
- 适用模块：Server / CLI

#### `MEDIA_LOCAL_ROOT`

- 用途：本地静态媒体资源根目录
- 示例：`../media/tmp`
- 说明：`local` driver 会把 HLS 和封面输出到该目录下
- 适用模块：Server / CLI

#### `MEDIA_PUBLIC_BASE_URL`

- 用途：媒体资源对客户端可访问的公开 URL 根地址
- 示例：`http://127.0.0.1:9000/media/tmp`
- 说明：CLI 写入 PostgreSQL 的 `media_url / cover_url` 会基于该值生成
- 适用模块：Server / CLI

#### `MEDIA_OBJECT_KEY_PREFIX`

- 用途：媒体资源 object key 前缀
- 示例：`media`
- 说明：推荐 key 形态为 `media/{mediaItemId}/hls/index.m3u8`
- 适用模块：Server / CLI

#### `MEDIA_STORAGE_ENDPOINT`

- 用途：MinIO 或 S3-compatible 对象存储 endpoint
- 示例：`http://127.0.0.1:9001`
- 适用模块：Server / CLI

#### `MEDIA_STORAGE_BUCKET`

- 用途：对象存储 bucket 名称
- 示例：`watch-together-media`
- 适用模块：Server / CLI

#### `MEDIA_STORAGE_REGION`

- 用途：对象存储 region
- 示例：`auto` / `oss-cn-hangzhou` / `ap-shanghai`
- 适用模块：Server / CLI

#### `MEDIA_STORAGE_ACCESS_KEY_ID`

- 用途：对象存储 access key id
- 提交规则：真实值不提交
- 适用模块：Server / CLI

#### `MEDIA_STORAGE_SECRET_ACCESS_KEY`

- 用途：对象存储 secret access key
- 提交规则：真实值不提交
- 适用模块：Server / CLI

#### `MEDIA_STORAGE_FORCE_PATH_STYLE`

- 用途：S3-compatible 客户端是否使用 path-style 访问
- 示例：`true` / `false`
- 说明：MinIO 本地开发通常需要 `true`
- 适用模块：Server / CLI

#### `FFMPEG_BIN`

- 用途：指定 `ffmpeg` 可执行文件路径
- 示例：`ffmpeg` / `/opt/homebrew/bin/ffmpeg`
- 适用模块：Server / CLI

#### `FFPROBE_BIN`

- 用途：指定 `ffprobe` 可执行文件路径，用于读取媒体源文件时长
- 示例：`ffprobe` / `/opt/homebrew/bin/ffprobe`
- 适用模块：Server / CLI

### 4. Debug

#### `DEBUG_SYNC`

- 用途：控制是否开启同步调试日志
- 示例：`true` / `false`
- 适用模块：Server、Windows、Android

---

## Parameter Ownership

### Server Uses

- `APP_ENV`
- `SERVER_HOST`
- `SERVER_PORT`
- `LOG_LEVEL`
- `DATABASE_URL`
- `CORS_ALLOWED_ORIGIN` `(optional)`
- `MEDIA_BASE_URL`
- `MEDIA_STORAGE_DRIVER`
- `MEDIA_LOCAL_ROOT`
- `MEDIA_PUBLIC_BASE_URL`
- `MEDIA_OBJECT_KEY_PREFIX`
- `MEDIA_STORAGE_ENDPOINT`
- `MEDIA_STORAGE_BUCKET`
- `MEDIA_STORAGE_REGION`
- `MEDIA_STORAGE_ACCESS_KEY_ID`
- `MEDIA_STORAGE_SECRET_ACCESS_KEY`
- `MEDIA_STORAGE_FORCE_PATH_STYLE`
- `FFMPEG_BIN`
- `FFPROBE_BIN`
- `DEBUG_SYNC`

### Windows Uses

- `APP_ENV`
- `API_BASE_URL`
- `WS_BASE_URL`
- `MEDIA_BASE_URL`
- `DEBUG_SYNC`
- `MEDIA_DEFAULT_ID` `(optional)`

### Android Uses

- `APP_ENV`
- `API_BASE_URL`
- `WS_BASE_URL`
- `MEDIA_BASE_URL`
- `DEBUG_SYNC`
- `MEDIA_DEFAULT_ID` `(optional)`

---

## Business Constants

以下内容当前建议保留为代码常量或文档规则，而不是环境配置：

- 房间码长度
- 房间人数上限
- 房间空闲销毁时长

### Reasoning

- 它们属于产品规则
- 它们不体现运行环境差异
- 当前阶段不需要通过环境切换来改变

---

## Local Development Environment Record

### Android Development Environment

当前机器已确认的 Android 开发环境如下：

- Android Studio: installed
- Android SDK Location: `/Users/sunboy/Library/Android/sdk`
- Installed SDK Platform:
  - Android 16.0 (`Baklava`)
  - API Level: `36.1`
  - Revision: `1`
- `adb`:
  - Android Debug Bridge version `1.0.41`
  - Version `37.0.0-14910828`
  - Path: `/Users/sunboy/Library/Android/sdk/platform-tools/adb`
- Gradle user home: `/Users/sunboy/.gradle`
- Android Studio Gradle JVM criteria:
  - Version: `21`
  - Vendor: `Any vendor`
- Terminal Java:
  - OpenJDK `22`

### Notes For Android Runtime

- 当前记录反映的是本机已安装环境，不等同于未来项目最终锁定的工程基线。
- Android Studio 的 Gradle JVM criteria 当前为 `21`，终端 Java 当前为 OpenJDK `22`，后续初始化 Android 工程时应确认项目实际要求的 JDK 版本并保持一致。

### Go Development Environment

当前机器已确认的 Go 开发环境如下：

- Go version: `go1.26.2`
- OS / Arch: `darwin/arm64`

### JavaScript / Node.js Environment

当前机器已确认的 JavaScript / Node.js 开发环境如下：

- Node.js version: `v20.12.2`
- npm version: `10.8.1`

### Rust Development Environment

当前机器已确认的 Rust 开发环境如下：

- rustc version: `1.91.1`
- cargo version: `1.91.1`
- Default host: `aarch64-apple-darwin`
- rustup home: `/Users/sunboy/.rustup`
- Active toolchain: `stable-aarch64-apple-darwin`
- Installed targets:
  - `aarch64-apple-darwin`

### Notes For Go / Node.js / Rust Runtime

- 当前记录用于描述本机已安装工具链状态，便于后续初始化 `server/` 和 `windows/` 工程时对照环境。
- Node.js 和 Rust 共同构成当前 Windows/Tauri 方向的本地开发基础。
- Go 版本与系统架构已记录，后续初始化 Go 服务工程时应以工程实际要求为准，再决定是否需要进一步锁版本。

---

## Notes

- 当前阶段只保留最小必要配置，避免过度设计。
- 后续如引入数据库、Redis、对象存储、鉴权密钥等，再新增对应环境参数。
- 具体配置承载方式，例如 `.env.example`、`.env.local`、Gradle 注入规则，应在后续配置策略任务中补充。
