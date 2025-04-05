# crproxy

这是一个使用 Go 实现的轻量级 Docker 镜像代理服务，支持从 `docker.io`、`ghcr.io`、`gcr.io` 等多种镜像源代理拉取镜像元数据或 token。

## ✨ 功能特色

- ✅ 支持 DockerHub 镜像代理
- ✅ 多仓库路由映射（如 `ghcr.io`, `gcr.io`, `quay.io` 等）
- ✅ 支持重定向、支持跨域请求（CORS）
- ✅ 基于 Go `net/http`，并发性能强大

## 🛠 使用方法

```bash
go build -o crproxy main.go
