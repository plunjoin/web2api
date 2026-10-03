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
