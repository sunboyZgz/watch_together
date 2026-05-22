# Server Deployment Notes

## 本地开发

默认的 `compose.yaml` 仍然优先服务本地开发，只启动基础设施：

```bash
docker compose up -d postgres redis minio minio-init
```

如果希望把 Go server 和 Nginx 也放进容器里联调，可以启用 `app` profile：

```bash
docker compose --profile app up -d --build
```

启用后入口是：

```text
http://127.0.0.1:8080
```

本地 compose 会继续把 PostgreSQL、Redis、MinIO API 和 MinIO Console 暴露到宿主机，方便调试。

## 生产编排

生产环境使用独立的 `compose.prod.yaml`：

```bash
cp .env.prod.example .env.prod
# 修改 .env.prod 中的密码、JWT secret、媒体签名 secret、域名等配置
docker compose --env-file .env.prod -f compose.prod.yaml up -d --build
```

生产 compose 的默认边界：

- 只暴露 Nginx HTTP 端口，默认 `80:80`。
- Go `roomserver` 只在 Docker 内网监听 `8080`。
- PostgreSQL、Redis、MinIO API、MinIO Console 不暴露到宿主机。
- 媒体分发默认使用 `MEDIA_DELIVERY_MODE=nginx_auth_request`。

## Nginx 与 MinIO 边界

`nginx_auth_request` 模式下，客户端先访问 Go 返回的短期 `/media/playback/...` 播放入口。Go 校验签名后设置 `wt_media_access` cookie，并跳转到 Nginx 的 `/watch-together-media/...` 媒体地址。

Nginx 对每个 HLS playlist / segment 请求调用 Go 的内部鉴权端点：

```text
/media/internal/auth
```

鉴权通过后，Nginx 再从 Docker 内网中的 MinIO 读取对象。因为 Nginx 不会自动生成 S3 签名，生产 compose 会让 MinIO bucket 在 Docker 内网可匿名下载，但 MinIO API 不暴露到公网，公网访问边界由 Nginx + Go 鉴权负责。

如果未来接入 CDN 或云厂商对象存储，可以把这个边界替换成 CDN signed URL / signed cookie，Go server 仍然只负责签发访问凭证，不承载视频字节流。
