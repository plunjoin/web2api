# ============================================================
# web2api Dockerfile —— 多阶段构建
# 阶段1: node 构建 React 前端（web/ → internal/webui/dist）
# 阶段2: golang:1.27-alpine 编译（CGO_ENABLED=0，纯 Go 含 modernc.org/sqlite），前端随二进制内嵌
# 阶段3: alpine 运行（~15MB 基础镜像 + 证书 + 时区）
#
# 前端产物与平台无关，只在构建机架构上构建一次（$BUILDPLATFORM），多架构镜像共用。
# 仓库里也提交了一份构建好的 internal/webui/dist，所以不用 Docker 时 go build 不需要 Node。
# ============================================================

# ---------- 前端构建阶段 ----------
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS web

WORKDIR /src/web

# 先拷依赖清单，利用 Docker 层缓存
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund

COPY web/ ./
# vite 输出到 ../internal/webui/dist（即 /src/internal/webui/dist）
RUN npm run build

# ---------- Go 构建阶段 ----------
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

# 用本次构建出的前端替换仓库里提交的那份（先删除，避免旧的哈希文件残留进二进制）
RUN rm -rf ./internal/webui/dist
COPY --from=web /src/internal/webui/dist ./internal/webui/dist

# 交叉编译：纯 Go、无 CGO。目标平台取自 BuildKit 按 --platform 自动注入的
# TARGETOS / TARGETARCH / TARGETVARIANT（linux/arm/v7 → GOARCH=arm GOARM=7）。
# 注意这几个 ARG 不能写默认值：写了默认值会覆盖自动注入的值，多架构构建时
# arm64、arm/v7 镜像里会全部装进 amd64 的二进制。未使用 BuildKit 时回退到 linux/amd64。
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=dev
ENV CGO_ENABLED=0

RUN GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-amd64}" GOARM="${TARGETVARIANT#v}" \
    go build -ldflags="-s -w -X web2api/internal/version.Version=${VERSION}" -o /out/web2api . \
 && go version -m /out/web2api | grep -E "GOARCH|GOARM|GOOS"

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
