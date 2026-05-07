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

`server/` 使用 `.env.example` / `.env.prod.example` + `.env` / `.env.local` / `.env.prod` / `.env.prod.local` + 运行时环境变量。

策略说明：

- `.env.example` 与 `.env.prod.example` 用于提交模板和字段说明。
- `.env.local` 用于本地开发机的实际值，不提交到仓库。
- `.env.prod` 用于生产环境基础值，不提交到仓库。
- `.env.prod.local` 用于生产机上的本地覆盖值，不提交到仓库。
- 运行时环境变量可用于覆盖本地文件或后续部署环境中的配置。

### Windows

`windows/` 使用 `.env.example` + `.env.local`。

策略说明：

- `.env.example` 用于提交示例配置。
- `.env.local` 用于本地联调时的真实地址和调试开关，不提交到仓库。
- 当前阶段不引入更复杂的配置系统，先保持轻量。

### Android

`android/` 不单独引入 `.env` 文件，使用 Gradle flavor + BuildConfig + `local.properties` 注入配置。

策略说明：

- 面向版本化的默认配置，应通过 Gradle 或 BuildConfig 注入。
- 本地开发机相关的私有配置，优先放在 `local.properties`。
- 当前阶段通过 `local` / `prod` 两个 product flavor 区分环境。

---

## File Commit Rules

以下规则适用于当前仓库：

- `.env.example` 和 `.env.*.example` 可以提交。
- `.env.local` 不可以提交。
- `.env.prod` 和 `.env.prod.local` 不可以提交。
- 其他本地环境文件，例如实际 `.env`，默认不提交。
- Android 的 `local.properties` 不提交。
- 模板文件应只包含示例值，不包含生产 secret 或个人私有地址。

当前规则已由根目录 `.gitignore` 配合约束。

---

## Template And Doc Locations

当前约定的文档和模板文件位置：

- `docs/environment-config.md`: 环境参数与配置策略统一说明
- `media/README.md`: 样例媒体资源目录与本地访问约定
- `server/.env.example`: Server 本地配置模板
- `server/.env.prod.example`: Server 生产配置模板
- `windows/.env.example`: Windows 配置模板

后续如增加更多配置模板，应继续沿用“模块内模板 + 仓库级文档说明”的方式维护。

---

## INT-170 Design

### Goal

`INT-170` 的目标不是“让程序直接读取 `.env.example`”，而是为 `server` 和 `mediactl` 引入一层统一、可测试、可扩展的配置加载器。

`.env.example` 继续只承担模板和说明职责，真实运行值来自：

- `server/.env.local` 或 `server/.env`
- 当前 shell 的环境变量
- 后续可选的 `config.local.yaml / config.prod.yaml`

### Why Not Read `.env.example` Directly

`.env.example` 的职责是：

- 告诉开发者需要哪些字段
- 给出安全的示例值
- 作为团队共享模板提交到仓库

它不应该承担：

- 真实本地运行配置
- 密钥存储
- 当前机器上真正的 endpoint / bucket / database URL

如果程序直接把 `.env.example` 当真实配置源，会让“模板”和“运行状态”混在一起，后续很难判断到底哪些值只是示例，哪些值正在生效。

### Scope

`INT-170` 第一阶段只整理配置加载层，不改业务逻辑。

优先覆盖：

- `server/cmd/mediactl`
- `server/internal/mediactl`
- `server/internal/app`

第一阶段不处理：

- Android 配置注入
- Windows 端配置系统统一
- 生产密钥管理
- CI/CD 注入

### Proposed Loader Shape

建议新增一个轻量配置模块，例如：

```text
server/internal/config/
  loader.go
  mediactl.go
  server.go
```

建议分成两层：

1. 原始加载层

- 负责用 `viper` 读取文件和环境变量
- 负责字段默认值
- 负责把配置反序列化到结构体

2. 领域配置层

- `MediactlConfig`
- `ServerRuntimeConfig`
- `StorageConfig`

业务代码只依赖结构体，不直接依赖 `viper` 全局状态。

### Recommended Load Order

`mediactl` 和 `roomserver` 当前统一采用以下优先级：

1. 命令行显式参数
2. 运行时环境变量
3. `server/.env.<APP_ENV>.local`
4. `server/.env.<APP_ENV>`
5. `server/.env.local`
6. `server/.env`
7. 代码默认值

说明：

- CLI flag 仍然优先级最高，例如 `--database-url`
- 环境变量始终可以覆盖本地文件
- `.env.example` 不参与运行时加载

### Mediactl First-phase Config Contract

`mediactl` 第一阶段建议只抽出这些配置：

- `DATABASE_URL`
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

建议对应结构：

```text
MediactlConfig
  DatabaseURL
  Storage StorageConfig
  Tools MediaToolConfig
```

其中：

- `StorageConfig` 继续承载 local / minio / s3 配置
- `MediaToolConfig` 只承载 `ffmpeg / ffprobe`

### Implementation Strategy

建议按三步落地：

1. 先新增配置加载层，但保留旧接口

- 保留 `LoadStorageConfig(...)`
- 内部逐步改成读取 `MediactlConfig`
- 让现有测试先继续通过

2. 再把 `mediactl.Run(...)` 从 `os.Getenv` 迁移到配置对象

- `main.go` 负责创建 loader
- `Run(...)` 改成接收配置或配置 provider

3. 最后再整理 `server/internal/app`

- 把 `DATABASE_URL`、`SERVER_PORT` 等 server 运行时配置统一接入

### Testing Strategy

为了不破坏当前测试友好性，`INT-170` 不建议让测试直接依赖真实 `viper` 全局状态。

建议：

- 保留可注入的 config provider
- 单测优先喂结构体或 map-backed provider
- 只给 loader 本身补少量读取 `.env.local` / 环境变量优先级测试

换句话说：

- 业务测试不依赖 `viper`
- 只有配置加载测试依赖 `viper`

### Success Criteria

`INT-170` 完成后，应满足：

- 本地运行 `mediactl` 时不需要每次手动 `export`
- `.env.example` 继续只是模板，不直接参与运行时
- `mediactl` 和 `roomserver` 使用统一配置加载方式
- 业务逻辑不直接依赖 `viper` 全局对象
- 现有 `mediactl` 单测不因配置系统而变脆弱

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
