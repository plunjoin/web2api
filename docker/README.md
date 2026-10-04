# web2api Docker 部署

## 快速启动

```bash
# 请在仓库根目录执行以下命令。
# 1. 准备配置文件（容器内路径已配好，只需拷贝并按需修改）
cp docker/config.yaml ./config.yaml

# 2. （可选）自定义端口/Key/代理
cp .env.example .env
#   编辑 .env 修改 WEB2API_PORT、WEB2API_API_KEYS、WEB2API_PROXY

# 3. 构建并启动
docker compose up -d --build

# 4. 查看日志
docker compose logs -f web2api

# 5. 访问管理台
#    http://<服务器IP>:8800/admin
```

## 面板一键升级

首次部署包含升级功能的版本时，在仓库根目录启用升级配置：

```bash
docker compose -f docker-compose.yml -f docker-compose.upgrade.yml up -d --build
```

之后登录管理台，打开「系统升级」→「检查更新」→「一键升级」。检查时下载最新镜像，服务继续运行；升级时短暂停机，面板自动重连。新容器通过健康检查后删除旧容器；启动或健康检查失败时恢复旧容器。

新容器保留原端口映射、环境变量、配置文件、网络和数据卷。升级任务由独立临时容器执行，记录写入数据卷中的 `upgrade-state.json`。不会删除持久化卷；回滚恢复旧容器，不回退已经写入数据库的数据或版本迁移。

升级配置会挂载 `/var/run/docker.sock`，让应用具有宿主机 Docker 管理权限，因此默认关闭，应仅向可信管理员开放管理台。支持 Linux Docker 引擎（包括 Docker Desktop 的 Linux 容器），要求持久化的可写数据卷和 Docker healthcheck。不支持 `--rm`、共享其他容器网络或 `volumes_from` 部署。

默认更新 `ghcr.io/plunjoin/web2api:latest`；可以在 `.env` 中指定 `WEB2API_UPGRADE_IMAGE`。私有仓库还需配置 `WEB2API_UPGRADE_REGISTRY_USER` 和 `WEB2API_UPGRADE_REGISTRY_PASSWORD`（GHCR 使用具有 read:packages 权限的 Token）。这些凭据只用于镜像下载，不出现在管理 API 返回中。请保持 `.env` 私有。首次使用需确保目标 `latest` 已发布包含升级功能的版本。

以后执行 Compose 命令时也保留这两个 `-f` 参数，以保持升级配置。面板按钮只更新部署时指定的镜像；管理员接口不接受任意镜像地址或命令。

管理 API（均需管理员 JWT）：

```text
GET  /admin/api/upgrade        当前镜像与升级状态
POST /admin/api/upgrade/check  后台检查并下载最新镜像
POST /admin/api/upgrade        启动独立升级任务
```

## 文件结构

| 文件 | 用途 |
|------|------|
| `Dockerfile` | 多阶段构建：golang:1.27-alpine 编译 → alpine:3.20 运行 |
| `docker-compose.yml` | 编排：端口、环境变量、持久化卷 |
| `.env.example`（仓库根目录） | 环境变量样例（端口、Key、代理） |
| `docker/config.yaml` | 容器内配置模板（路径已指向 /app/*） |
| `.dockerignore` | 排除本地数据/产物，保持镜像精简 |

## 持久化卷

| 卷 | 容器路径 | 内容 |
|----|----------|------|
| `web2api-data` | `/app/data` | SQLite 库（号池账号、API Key、用量） |
| `web2api-auth` | `/app/auth` | 引擎B 账号凭据（AIStudio2API 兼容） |
| `web2api-cookies` | `/app/cookies` | 引擎A Cookie 自动轮换缓存 |

> 卷由 Docker 管理，`docker compose down` 不删除；`docker compose down -v` 才清除。

## 环境变量

环境变量优先级高于 config.yaml（见 `.env.example`）：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `WEB2API_PORT` | `8800` | 宿主机暴露端口 |
| `WEB2API_API_KEYS` | `sk-web2api` | 种子 Key（逗号分隔多个） |
| `WEB2API_DEFAULT_ENGINE` | `auto` | 路由：auto / a / b |
| `WEB2API_PROXY` | （空） | 代理地址（引擎A 访问 Google） |

## 常用操作

```bash
# 停止
docker compose down

# 重新构建（代码更新后）
docker compose up -d --build

# 查看实时日志
docker compose logs -f

# 进入容器
docker compose exec web2api sh

# 备份数据（SQLite + 凭据）
docker compose cp web2api:/app/data ./backup-data
docker compose cp web2api:/app/auth ./backup-auth
```

## 仅用 Dockerfile（不用 compose）

```bash
docker build -t web2api:latest .
docker run -d --name web2api \
  -p 8800:8800 \
  -v web2api-data:/app/data \
  -v web2api-auth:/app/auth \
  -v web2api-cookies:/app/cookies \
  -v ./config.yaml:/app/data/config.yaml:ro \
  -e TZ=Asia/Shanghai \
  --restart unless-stopped \
  web2api:latest
```

## 代理说明

容器内访问宿主机代理：
- **Docker Desktop**（Windows/Mac）：`http://host.docker.internal:7890`
- **Linux**：`http://<宿主机内网IP>:7890` 或加 `--network host`

引擎A 需要访问 `gemini.google.com`，引擎B 需要访问 `aistudio.google.com`，
部署在国内服务器时务必配置代理。

> AI生成
