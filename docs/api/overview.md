# v1 API 总览

对外接口统一位于 `/v1`。除健康检查和文档地址外，接口都使用客户端 API Key。

| 配置 | 值 |
| --- | --- |
| Base URL | `http://你的地址:8800/v1` |
| 认证 | `Authorization: Bearer sk-...` |
| 文档 JSON | `GET /v1/docs` |
| 协议版本 | OpenAPI 3.1 |

## 接口分组

| 分组 | 接口 | 说明 |
| --- | --- | --- |
| 基础 | `GET /health` | 服务与引擎健康状态，无需 Key |
| 基础 | `GET /v1/models` | 聚合可用模型 |
| 基础 | `GET /v1/accounts` | 查看引擎和账号池状态 |
| 聊天 | `POST /v1/chat/completions` | OpenAI 兼容聊天，支持 SSE |
| 视频 | `POST /v1/videos` | native 模式创建 Veo 视频长任务 |
| 视频 | `GET /v1/videos/{id}` | 查询视频任务状态 |
| 视频 | `GET /v1/videos/{id}/content` | 下载已完成视频 |
| 图片 | `POST /v1/chat/completions` | native 模式选择图片模型，返回 Markdown 媒体链接 |
| 音频 | `POST /v1/chat/completions` | native 模式选择 TTS/音频模型，返回 Markdown 媒体链接 |
| 图片 | `POST /v1/images/generations` | upstream + passthrough 模式，需上游支持图片接口 |
| 音频 | `POST /v1/audio/speech` | upstream + passthrough 模式，需上游支持文本转语音接口 |
| 透传 | `POST /v1/files/*` | upstream 模式文件透传 |
| 透传 | `POST /v1/embeddings/*` | upstream 模式 Embedding 透传 |

## 获取完整 OpenAPI

服务运行后直接访问：

```bash
curl http://localhost:8800/v1/docs > openapi.json
```

把 `openapi.json` 导入 Postman、Insomnia、Swagger UI 或其他 OpenAPI 工具即可生成交互式接口页面。文档 JSON 与服务端代码一起生成，字段和响应会随版本同步。

文档内包含生成接口的参数说明、请求与响应示例和 cURL 命令。按用途阅读：[视频生成](/api/videos)、[图片生成](/api/images)、[音频生成](/api/audio)。原生模式是默认配置；图片和语音专用透传端点需要启用 `engine_b.mode: upstream`、`passthrough: true`，参数及响应以实际配置的上游为准。

## 响应约定

聊天、视频创建与查询使用 JSON；流式聊天使用 `text/event-stream`；视频下载返回 MP4，语音透传通常返回音频二进制。网关生成的错误结构为下例；透传错误保留上游结构，`code` 也可能省略：

```json
{
  "error": {
    "message": "Invalid API key",
    "type": "authentication_error",
    "code": null
  }
}
```

具体状态码和字段请查看对应接口页面。
