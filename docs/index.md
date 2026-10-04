# web2api

## Gemini 双引擎号池，统一成 OpenAI 兼容 API

web2api 把 Gemini 网页端和 AI Studio 的账号池接入、调度、Key 分发和用量统计放在一个 Go 服务里。客户端只需要配置一个 Base URL 和一个 `sk-` Key，就能用熟悉的 OpenAI SDK 调用文本、流式聊天和 Veo 视频任务。

<div class="vp-hero-actions">
  <a class="vp-button vp-button-brand" href="./guide/quickstart">5 分钟接入</a>
  <a class="vp-button vp-button-alt" href="./api/overview">查看 v1 API</a>
</div>

## 你可以用它做什么

<div class="vp-feature-grid">
  <div class="vp-feature-card">
    <div class="vp-feature-icon">⚡</div>
    <h3>OpenAI 兼容</h3>
    <p>沿用 Chat Completions、Bearer Key 和 SSE 流式响应，已有 SDK 基本无需改造。</p>
  </div>
  <div class="vp-feature-card">
    <div class="vp-feature-icon">🔁</div>
    <h3>双引擎号池</h3>
    <p>Gemini 网页号与 AI Studio 号统一管理，自动轮询、冷却和故障切换。</p>
  </div>
  <div class="vp-feature-card">
    <div class="vp-feature-icon">🎛️</div>
    <h3>可视化管理</h3>
    <p>管理台负责账号、Key、模型、用量和系统状态，运行时加号无需重启。</p>
  </div>
</div>

## 三步开始

1. 启动服务并打开 `/admin`，完成管理员和 Redis 初始化。
2. 在管理台添加账号，创建一个客户端 `sk-` Key。
3. 将 SDK 的 `base_url` 指向 `http://你的地址:8800/v1`。

```python
from openai import OpenAI

client = OpenAI(
    api_key="sk-从管理台创建",
    base_url="http://localhost:8800/v1",
)

response = client.chat.completions.create(
    model="gemini-2.5-flash",
    messages=[{"role": "user", "content": "你好，介绍一下 web2api"}],
)
print(response.choices[0].message.content)
```

## 文档入口

| 内容 | 适合谁 | 入口 |
| --- | --- | --- |
| 项目介绍 | 想了解架构、引擎和数据流 | [项目介绍](/guide/introduction) |
| 快速开始 | 第一次部署和调用 | [快速开始](/guide/quickstart) |
| v1 接口 | 接入客户端或编写 SDK | [v1 API 总览](/api/overview) |
| OpenAPI JSON | 导入 Postman、Insomnia 或 Swagger UI | `/v1/docs` |
| 管理 API | 自动化管理账号和 Key | [README 管理 API](https://github.com/plunjoin/web2api#rest-管理-api供脚本二次开发) |

## 项目状态

- 服务端：Go 单体服务，SQLite 保存业务数据，Redis 保存管理会话。
- 协议：OpenAI Chat Completions 兼容，OpenAPI 3.1 描述对外 v1 接口。
- 部署：Windows、Linux、macOS 二进制，以及 Docker Compose。
- 开源协议：MIT。

<style>
.vp-hero-actions { display: flex; gap: 12px; flex-wrap: wrap; margin: 28px 0 42px; }
.vp-button { display: inline-flex; align-items: center; border-radius: 10px; padding: 10px 18px; font-weight: 600; text-decoration: none !important; transition: opacity .2s; }
.vp-button:hover { opacity: .82; }
.vp-button-brand { color: #fff !important; background: var(--vp-c-brand-1); }
.vp-button-alt { color: var(--vp-c-text-1) !important; background: var(--vp-c-bg-soft); border: 1px solid var(--vp-c-divider); }
.vp-feature-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; margin: 24px 0 42px; }
.vp-feature-card { border: 1px solid var(--vp-c-divider); border-radius: 14px; padding: 20px; background: var(--vp-c-bg-soft); }
.vp-feature-card h3 { margin: 8px 0; font-size: 16px; }
.vp-feature-card p { margin: 0; color: var(--vp-c-text-2); font-size: 14px; line-height: 1.7; }
.vp-feature-icon { font-size: 24px; }
@media (max-width: 640px) { .vp-feature-grid { grid-template-columns: 1fr; } }
</style>
