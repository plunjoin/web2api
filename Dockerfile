# ============================================================
# web2api Dockerfile —— 多阶段构建
# 阶段1: golang:1.27-alpine 编译（CGO_ENABLED=0，纯 Go 含 modernc.org/sqlite）
# 阶段2: alpine 运行（~15MB 基础镜像 + 证书 + 时区）
# ============================================================

# ---------- 构建阶段 ----------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

# 安装编译所需工具（git 用于 go mod 下载公开依赖）
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# 先拷依赖清单，利用 Docker 层缓存
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 拷贝源码
COPY main.go ./
COPY internal ./internal

# 交叉编译：纯 Go、无 CGO、Linux/amd64（可改 arm64 部署到 ARM 服务器）
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH}

RUN go build -ldflags="-s -w" -o /out/web2api .

# ---------- 运行阶段 ----------
FROM alpine:3.20

# 根证书（HTTPS 访问 Google 必需）+ 时区数据 + tzdata
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 拷贝二进制
COPY --from=builder /out/web2api /app/web2api

# 数据目录（SQLite 库含 Cookie 会话、引擎B auth 凭据）
# 由 docker-compose volumes 或 -v 持久化
RUN mkdir -p /app/data /app/auth

# 默认配置（容器内不可变区放一份模板，实际配置从挂载卷读）
COPY config.example.yaml /app/config.example.yaml

# 暴露网关端口
EXPOSE 8800

# 健康检查（/health 无需鉴权）
HEALTHCHECK --interval=30s --timeout=5s --start-period=120s --retries=3 \
  CMD wget -q --spider http://localhost:8800/health || exit 1

# 入口：默认读 /app/data/config.yaml（由 volume 挂载）
ENTRYPOINT ["/app/web2api"]
CMD ["-config", "/app/data/config.yaml"]
