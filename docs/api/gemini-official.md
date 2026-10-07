# 官方 Gemini 后端接入

web2api 保留 Gemini 网页与 AI Studio 登录态账号池，同时提供独立的官方 Gemini API 后端。启用后，官方请求的全部参数、嵌套对象、联合类型和扩展字段直接交给 Google，响应、状态码、错误详情、媒体二进制和 SSE 事件保留原结构。无需把官方请求改成 OpenAI messages。

## 配置

服务器设置 Google API Key；客户端使用管理台创建的网关 sk- Key：

```yaml
gemini_api:
  enabled: true
  base_url: https://generativelanguage.googleapis.com
  api_key: 你的GoogleAPIKey
  access_token: ""
  proxy: ""
  timeout_seconds: 0
```

推荐用环境变量 `WEB2API_GEMINI_API_KEY`，设置后自动启用官方后端；可用 `WEB2API_GEMINI_BASE_URL` 和 `WEB2API_GEMINI_PROXY` 覆盖地址和代理。base_url 是 API 根地址，不应包含 /v1 或 /v1beta。默认总超时为 0，长后台 SSE 不因固定的聊天超时中断；可按部署需要设置秒数。客户端断开连接时，上游 HTTP 请求也会取消。

网页账号 Cookie 不能代替 Google API Key。官方调用使用关联项目的配额、计费和权限；模型实际支持哪些参数、agent / 服务等级的可用性和官方待确认字段，均以 Google 接受的请求为准。这里实现的是完整协议转发，未使用真实计费账号逐模型验证生成效果。

GCS 文件注册等功能需要 OAuth 时，配置 `access_token` 或 `WEB2API_GEMINI_ACCESS_TOKEN`。后端会以服务器的 OAuth Bearer token 调用 Google；该 token 的有效期和续期由部署方管理。API Key 和 OAuth token 可同时配置。客户端的 Authorization 永远用于网关鉴权，不会转成 Google 凭据。

## 路径和认证

| 路径 | 用途 |
| --- | --- |
| `/v1beta/interactions`、`/v1/interactions` 及其子路径 | 创建、查询、取消、删除；后台、工具回传、服务端多轮、续写和 SSE 恢复 |
| `/v1beta/agents`、`environments`、`voices`、`webhooks`、`triggers`、`credentials` 及子路径 | 官方快照中的所有资源管理操作；也提供 /v1 对应路由，版本实际可用性由 Google 校验 |
| `/v1beta/models`、`/v1beta/models/{model}:...` | 官方模型发现、生成等请求；不混入网页登录态的 OpenAI 模型目录 |
| `/v1beta/files`、`/v1beta/files/{id}`、`/v1beta/files:register` | 官方 Files 查询 / 删除 / 注册 |
| `/upload/v1beta/files` | 官方文件 resumable / multipart 上传 |
| `/upload/{version}/environments/{environment}/files/{path}` | 官方沙箱文件上传 |
| `/download/...` | 官方下载请求，按 Google 返回的路径调用 |
| `/gemini/{官方路径}` | 所有官方路径的统一前缀别名，如 /gemini/v1/files，避免与现有 OpenAI /v1/files、/v1/models 冲突 |
| `/gemini-upload/{token}` | 官方返回断点上传 URL 后，网关生成的续传入口；客户端原样使用返回 URL |
| `GET /v1/gemini-docs` | 官方全部资源 schema 的网关版 OpenAPI；无需鉴权 |

请求可用 `x-goog-api-key: sk-...`、`Authorization: Bearer sk-...` 或 `?key=sk-...`。这些值只在 SQLite 网关 Key 库校验，出站替换为服务器 Google 凭据。服务端保留 Content-Type、Accept、Api-Revision、X-Goog-Upload-* 等协议头和非认证查询参数。

`/v1/models`、`/v1/chat/completions`、`/v1/videos` 和现有专用透传路径继续使用其已定义的协议；官方 SDK 推荐根 Base URL `http://localhost:8800/gemini`，REST 也可直接调用 /v1beta 路径。网关未启用官方后端时，官方生成路径返回 503，提示配置后端；网关 Key 无效返回 401。

## SDK 接入

官方 SDK 无需更改请求字段，替换认证和 Base URL 即可。Interactions 请求字段仍是 snake_case，客户端初始化的 httpOptions 属于 SDK 自身配置。

```js
import { GoogleGenAI } from "@google/genai";

const ai = new GoogleGenAI({
  apiKey: "sk-你的网关Key",
  httpOptions: { baseUrl: "http://localhost:8800/gemini", apiVersion: "v1beta" }
});

const interaction = await ai.interactions.create({
  model: "gemini-3.8-flash",
  input: "解释这次 API 调用的步骤",
  generation_config: { max_output_tokens: 2048, thinking_level: "medium" },
  response_format: { type: "text", mime_type: "text/plain" }
});
console.log(interaction.output_text);
```

```python
from google import genai
from google.genai import types

client = genai.Client(
    api_key="sk-你的网关Key",
    http_options=types.HttpOptions(
        base_url="http://localhost:8800/gemini", api_version="v1beta"
    ),
)
interaction = client.interactions.create(
    model="gemini-3.8-flash",
    input="解释这次 API 调用的步骤",
    generation_config={"max_output_tokens": 2048, "thinking_level": "medium"},
)
print(interaction.output_text)
```

选择模型前可调用 `/v1beta/models` 或 SDK models.list，官方目录与网页账号目录不同。SDK 版本要求、模型族和示例见[中文详解](/api/gemini-reference)。

## REST、多模态及结构化输出

```bash
curl http://localhost:8800/v1beta/interactions \
  -H 'x-goog-api-key: sk-你的网关Key' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "gemini-3.8-flash",
    "input": [{"type":"text","text":"用 JSON 给出一个简短的状态说明"}],
    "generation_config": {"max_output_tokens": 1024},
    "response_format": {
      "type":"text", "mime_type":"application/json",
      "schema":{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}
    }
  }'
```

所有 image / audio / video / document 的 data / uri、annotations、Step[]、tools、tool_choice、generation_config、agent_config、response_format、environment、safety_settings、service_tier、labels 和 webhook_config 均以原始 JSON 转发。网关不先解码到缩减版 ChatRequest，因此不会忽略字段或改写 snake_case。新官方字段也能转发。

媒体内联时用裸标准 Base64；大文件把 JSON 写进文件后用 `-d @payload.json`。官方后端逐段传送请求体，未套用聊天的 8 MiB 上限；实际允许大小由 Google 和部署中的反向代理决定。response_format 配置可以是对象或数组；模型不支持的组合由 Google 返回原始错误。

## 多轮、函数工具和后台执行

多轮请求将 previous_interaction_id 设为上轮返回的官方 ID。system_instruction、tools、generation_config 等每轮按官方规则重传。函数结果以 input 中的 function_result step 回传，并使用对应 function_call.id 作为 call_id；原始 response.steps、thought signature 和引用不会被网关丢弃。无状态步骤历史也直接转发。

后台任务示例：

```bash
curl http://localhost:8800/v1beta/interactions \
  -H 'x-goog-api-key: sk-你的网关Key' \
  -H 'Content-Type: application/json' \
  -d '{"agent":"deep-research-preview-04-2026","input":"研究电池技术的发展","background":true,"store":true}'

curl 'http://localhost:8800/v1beta/interactions/真实ID?include_input=true' \
  -H 'x-goog-api-key: sk-你的网关Key'

curl -X POST http://localhost:8800/v1beta/interactions/真实ID/cancel \
  -H 'x-goog-api-key: sk-你的网关Key'

curl -X DELETE http://localhost:8800/v1beta/interactions/真实ID \
  -H 'x-goog-api-key: sk-你的网关Key'
```

后台执行和交互存储由 Google 管理；网关不伪造 completed 或把后台任务转成前台聊天。store=false、取消时机、保留期、续写 token、requires_action 等行为都按官方执行。webhook 由 Google 请求目标 URI，目标需能被 Google 访问；网关完整传送配置。

## SSE 和断线恢复

```bash
curl -N http://localhost:8800/v1beta/interactions \
  -H 'x-goog-api-key: sk-你的网关Key' \
  -H 'Content-Type: application/json' \
  -H 'Accept: text/event-stream' \
  -d '{"model":"gemini-3.8-flash","input":"从 1 数到 25","stream":true}'

curl -N 'http://localhost:8800/v1beta/interactions/真实ID?stream=true&last_event_id=最后收到的事件ID' \
  -H 'x-goog-api-key: sk-你的网关Key'
```

保留 event、id、data、event_type、event_id、step.index、delta、metadata、usage、step_usage 和结束标记；不转换成 choices[].delta。网关收到增量即刷新到客户端。恢复查询参数直接发送 Google，因此不依赖某个网关进程内的事件缓存。stream 中途失败时按 HTTP / SSE 原始连接状态处理，不在官方流中注入 OpenAI 错误文本。

## 文件上传、续传和下载

SDK files.upload 可以使用同一 Base URL。REST 上传遵循官方 resumable 流程：

1. 向 `/upload/v1beta/files` 发起 start，请求头使用 X-Goog-Upload-Protocol、X-Goog-Upload-Command、X-Goog-Upload-Header-Content-Length 和 X-Goog-Upload-Header-Content-Type。
2. 读取响应 X-Goog-Upload-URL。网关将官方上传 URL 保存到 SQLite，并返回本服务的 `/gemini-upload/{token}` URL。
3. 原样使用返回 URL 发送二进制，保留 X-Goog-Upload-Offset、X-Goog-Upload-Command（upload / query / finalize / cancel 等）。续传仍需网关 Key。该 URL 在网关重启后仍可使用，记录保留 48 小时；上游会话是否有效由 Google 决定。
4. 最终响应中的 file.uri 保持 Google 原值。把它放到 Interactions input.uri，供 Google 读取；不要改成网关地址。

Files 查询 / 删除与下载需要访问 Google API 路径时，可将相同路径请求到网关 /gemini 前缀下。返回的公共或签名媒体 URL 可以直接下载；响应 URI 本身不改写，以保证模型复用媒体的语义。不能把输出 URI 的所有域名都机械替换为网关。

上游 4xx / 5xx、Retry-After、错误 details 和二进制正文原样保留。仅网关鉴权失败、后端未配置、网络失败或本地上传会话失效时生成网关错误。下载前检查状态码和 Content-Type，避免把错误 JSON 保存为媒体。

## SQLite 和目录调整

入口为根目录 main.go，运行 `go run .` 或 `go build .`。所有测试统一在 test；`go test ./...` 会运行新增集成测试和集中保存的原有私有包测试。

Gemini Cookie 原始账号凭据仍在 accounts 表，轮换后的完整 Cookie 集合存到 cookie_sessions；替换凭据清理旧会话，删除账号级联删除会话，旧刷新请求无法重新写回已替换或删除的账号。客户端重启从 SQLite 恢复轮换值。运行和部署不再创建或挂载 cookies 目录；AI Studio 的 auth 目录仍用于其现有私有协议。

升级时如果需要旧缓存中最新的 Cookie，先停止旧服务，并保留原 SQLite 数据库，然后执行一次：

```bash
go run . -config config.yaml -import-cookie-cache /原来的Cookie缓存目录
```

该命令只导入 SQLite 已登记账号匹配的缓存文件，完成后退出，不删除旧文件。新服务不再使用该目录。SQLite 的 gemini_upload_sessions 表保存官方续传会话 URL；备份应包含数据库及其 WAL 状态，推荐停止服务后备份 data 目录。

## 验证和完整参数文档

已使用本地模拟 Google 服务验证完整多模态 / 工具 / agent / 沙箱参数、未知扩展字段、所有快照操作的 v1 / v1beta 路由、原始响应与错误、SSE 即时输出及取消传播、超过聊天上限的二进制续传、SQLite 会话恢复和失效、OpenAPI 字段与引用完整性。验证不消耗 Google 生成额度。

全部参数及官方资料冲突见[字段完整对照](/api/gemini-schema)和[中文参考详解](/api/gemini-reference)。本地 HTTP 测试证明网关协议完整性；实际 Google 账号、模型或预览服务的可用性需要部署后的官方请求验证。
