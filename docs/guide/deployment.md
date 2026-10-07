# 部署方式

## Docker Compose（推荐）

```bash
docker compose up -d
docker compose logs -f web2api
```

持久化以下目录或卷：

| 路径 | 内容 |
| --- | --- |
| `data/` | SQLite 数据库，保存账号、Cookie 会话、官方上传续传会话、Key、用量和管理员配置 |
| `auth/` | AI Studio 登录态 |

生产环境建议在反向代理后启用 HTTPS，并且只对可信网络开放 `/admin`。

## 二进制运行

下载对应平台的发布包，准备 `config.yaml`，然后执行：

```bash
./web2api -config config.yaml
```

Windows 可以运行 `web2api.exe -config config.yaml`。监听地址、上游模式、模型路由和限流参数都可以在配置文件中调整。

## 健康检查

`GET /health` 不需要 Key，适合容器健康检查和负载均衡探针。它会返回服务状态、运行时间和两个引擎的可用状态。
