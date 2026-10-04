# 项目介绍

web2api 是一个自托管的 Gemini 双引擎网关。它把账号池、请求调度、API Key 和用量统计组合成一个服务，对外提供熟悉的 OpenAI 兼容接口。

## 解决的问题

直接对接网页端或 AI Studio 协议时，客户端需要自己处理登录态、多个账号切换、配额冷却和不同响应格式。web2api 将这些工作放到服务端：客户端只面向稳定的 `/v1` 协议，账号变化不会影响业务代码。

## 运行结构

```text
客户端 / OpenAI SDK
          │  Bearer sk-xxx
          ▼
      web2api 网关
      ├─ Key 校验与限流
      ├─ 模型路由与账号调度
      ├─ 用量记录
      ├─ Gemini 网页引擎（引擎 A）
      └─ AI Studio 引擎（引擎 B）
          │
      管理台 / SQLite / Redis
```

## 两个引擎

| 引擎 | 账号来源 | 适合场景 |
| --- | --- | --- |
| A：Gemini 网页 | Gemini 网页 Cookie 会话 | 文本对话和兼容的轻量请求 |
| B：AI Studio | AI Studio `storage-state.json` | 多模态、Veo、更多模型能力 |

模型路由由 `routing.engine_a_models`、`routing.engine_b_models` 和 `auto` 策略共同决定。多模态和视频请求会优先使用引擎 B；账号不可用时，支持的请求会自动切换到其他账号。

## 数据和安全边界

- SQLite 保存账号、API Key 和用量明细。
- Redis 只保存管理员登录会话和登录尝试计数。
- 客户端 Key 使用 `Authorization: Bearer <key>`，管理台使用独立的管理员 JWT。
- `auth/`、`cookies/` 和 `data/` 包含敏感信息，应使用独立备份策略，不要提交到 Git。

## 代码入口

| 目录 | 作用 |
| --- | --- |
| `internal/gateway` | 对外 `/v1` 网关、认证、限流和 OpenAPI 文档 |
| `internal/provider` | 双引擎调度与模型路由 |
| `internal/store` | SQLite 数据访问 |
| `internal/admin` | 管理 REST API 和管理员认证 |
| `internal/webui` | `/admin` 管理台 |

## 适合的部署方式

个人或小团队可以直接运行发布包；长期运行推荐 Docker Compose，并将 `data/`、`auth/` 和 `cookies/` 挂载到持久化卷。详细步骤见[部署方式](/guide/deployment)。
