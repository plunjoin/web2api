# 项目介绍

web2api 是一个自托管的 AI 号池网关。它把账号池、请求调度、API Key 和用量统计组合成一个服务，对外提供熟悉的 OpenAI 兼容接口。

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
      ├─ 引擎 A：Gemini 网页号
      ├─ 引擎 B：AI Studio 号
      ├─ 订阅引擎：codex / antigravity / kiro / workbuddy
      └─ 第三方 Key：custom（base_url + api_key）
          │
      管理台 / SQLite / Redis
```

## 引擎与账号来源

| 引擎 | 账号来源 | 适合场景 |
| --- | --- | --- |
| A：Gemini 网页 | Gemini 网页 Cookie 会话 | 文本对话和兼容的轻量请求 |
| B：AI Studio | AI Studio `storage-state.json` | 多模态、Veo、更多模型能力 |
| 订阅引擎 | codex / antigravity / kiro / workbuddy 的授权凭据 | 复用订阅制额度 |
| custom | 任意 OpenAI 兼容上游的 Base URL + API Key | 接入第三方供应商 |

路由按「**谁声明谁接**」：显式配置（`routing.engine_a/b_models`、`routing.engine_models`）
优先，其次按订阅/custom 目录与引擎实时目录精确命中。命中即专属——引擎不可用时返回该引擎
的真实错误，不会换到别的引擎；多模态关键词（image / veo / lyria…）只交给引擎 B；
没有任何引擎声明该模型时返回 `404 model_not_found`。音乐和视频模型不能走
`/v1/chat/completions`（引擎 B native 除外），请使用 `/v1/music/generations` 与 `/v1/videos`。

## 数据和安全边界

- SQLite 保存账号、API Key 和用量明细。
- Redis 只保存管理员登录会话和登录尝试计数。
- 客户端 Key 使用 `Authorization: Bearer <key>`，管理台使用独立的管理员 JWT。
- `auth/` 和 `data/` 包含敏感信息，应使用独立备份策略，不要提交到 Git。

## 代码入口

| 目录 | 作用 |
| --- | --- |
| `internal/gateway` | 对外 `/v1` 网关、认证、限流和 OpenAPI 文档 |
| `internal/provider` | 多引擎装配、调度与模型路由 |
| `internal/store` | SQLite 数据访问 |
| `internal/admin` | 管理 REST API 和管理员认证 |
| `internal/webui` | `/admin` 管理台 |

## 适合的部署方式

个人或小团队可以直接运行发布包；长期运行推荐 Docker Compose，并将 `data/` 和 `auth/` 挂载到持久化卷。详细步骤见[部署方式](/guide/deployment)。
