# syntax=docker/dockerfile:1
# DeepWiki(Go版) 多阶段构建（Go 1.22+；纯静态二进制，CGO 关闭——全部依赖均为纯 Go 客户端）。

# ---- 构建阶段 ----
FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/deepwiki ./cmd/server

# ---- 运行阶段 ----
FROM alpine:3.20
# git ≥ 2.30（硬约束 R5：git CLI 为部署前置依赖；验收红线：容器内 git --version 可用）
# ca-certificates：HTTPS 克隆与 provider API 调用所需根证书
RUN apk add --no-cache git ca-certificates \
    && adduser -D -u 10001 deepwiki
WORKDIR /app
COPY --from=build /out/deepwiki /app/deepwiki
COPY configs/ /app/configs/
COPY migrations/ /app/migrations/
RUN mkdir -p /app/data/repos && chown -R deepwiki:deepwiki /app/data
USER deepwiki
EXPOSE 8080
# 密钥与基础设施凭据经环境变量注入（DEEPWIKI_*，见 .env.example），禁止写入镜像。
ENTRYPOINT ["/app/deepwiki", "-config", "configs/config.yaml"]
