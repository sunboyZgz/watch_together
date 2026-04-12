# Environment Config

> Current-stage environment parameter catalog for `watch_together`.

本文档只定义当前阶段真正需要的环境配置参数，并区分哪些属于环境配置，哪些属于业务规则常量。

本文件当前聚焦：

- 参数清单
- 参数用途
- 参数归属
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
- 示例：`http://127.0.0.1:9000/media`
- 适用模块：Server、Windows、Android

#### `MEDIA_DEFAULT_ID` `(optional)`

- 用途：本地联调时提供默认媒体 ID
- 说明：方便调试，不属于运行必须项
- 示例：`sample_001`
- 适用模块：Windows、Android

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
- `CORS_ALLOWED_ORIGIN` `(optional)`
- `MEDIA_BASE_URL`
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

## Notes

- 当前阶段只保留最小必要配置，避免过度设计。
- 后续如引入数据库、Redis、对象存储、鉴权密钥等，再新增对应环境参数。
- 具体配置承载方式，例如 `.env.example`、`.env.local`、Gradle 注入规则，应在后续配置策略任务中补充。
