# Gemini Interactions 中文参考详解

> 整理自用户提供的 2026-10-07 中文参考手册。下文所有 Google URL、SDK 和 curl 示例用于直接调用官方 API；web2api 已提供官方后端，可替换 Base URL 和网关 Key 接入，见[官方后端接入](/api/gemini-official)。网关实际映射见 [Gemini 参数与实现差距](/api/gemini)，全部官方字段以 [官方字段完整对照](/api/gemini-schema) 和保存的官方快照为准。指南、模型限制与 SDK 专属结论沿用原手册注明的来源，本次未逐一独立验证。

> 适用范围：`POST https://generativelanguage.googleapis.com/v1beta/interactions`（JS：`ai.interactions.create(...)`，`@google/genai`；Python：`client.interactions.create(...)`，`google-genai`）。
>
> 资料抓取时间：2026-10-07（北京时间 UTC+8）。主要依据：ai.google.dev 官方 API 参考（`/api/interactions-api`，及其 Markdown 版 `/static/api/interactions.md.txt` 与 OpenAPI 规范 `/static/api/interactions.openapi.json`）、各功能指南页、模型页，以及 npm 上 `@google/genai@2.27.0` 的类型定义（`dist/genai.d.ts`）与实现代码。
>
> 写作原则：只记录文档中实际出现的字段、取值与模型 ID。文档没写清楚、或不同页面互相矛盾的地方，统一标成 **【待确认】**，并写出矛盾来源。每节末尾附来源 URL。

---

## 0. 协议使用要点

1. **字段名一律 snake_case。** 根据 `@google/genai@2.27.0` 的源码，`Interactions.create()` 会把请求对象原样 JSON 序列化（`encodeJSON("body", payload.body)`），不做 camelCase 到 snake_case 的转换。所以 `generation_config`、`thinking_level`、`response_format`、`image_size`、`previous_interaction_id` 等都必须写成 snake_case。（这和旧的 `ai.models.generateContent` 不一样。）
2. **2026 年 5 月的破坏性变更（Api-Revision `2026-05-20`）已经完全生效。** 旧 schema 于 2026-06-08 下线，`Api-Revision` 请求头从那天起被忽略。具体变化：
   - 响应里的 `outputs[]` 换成了 `steps[]`；
   - 新增顶层多态字段 `response_format`（单个对象或数组），每个条目带 `type: text | image | audio | video`；
   - 删除 `response_mime_type`，改为在 `response_format` 条目里写 `mime_type`；
   - `generation_config.image_config` 移到 `response_format`（`type: "image"` 条目中的 `aspect_ratio` / `image_size`）；
   - `response_modalities` 被 `response_format` 取代。
3. **`gemini-nano-banana-2.1` 是官方文档中的模型 ID**（Nano Banana 2.1，2026-10 发布，Stable）。官方示例用的都是不带前缀的写法 `"gemini-nano-banana-2.1"`。带 `models/` 前缀在 v1beta 是否被接受，**【待确认】**（详见 §4.3）。注意：v1beta API 参考里 `model` 的枚举还没有收录这个 ID（SDK 类型允许任意字符串），但模型页和图像生成指南都在使用它。
4. **`temperature`、`top_p`、`generation_config.image_config`、`response_mime_type`、`response_modalities`** 在当前渲染版 API 参考中已经不出现；在 SDK 2.27.0 类型里它们被标记为 `@deprecated`（"This will be removed in a future release"）；OpenAPI JSON 中还保留着。新代码不要再用。
5. **文件和媒体只能以 JSON 提交：** 要么在 `data` 放 base64 字符串，要么在 `uri` 放 Files API 的 `file.uri`、公开 https URL 或 YouTube URL。Interactions 没有给模型输入用的上传端点；上传走 Files API：`POST /upload/v1beta/files`（resumable）或 `ai.files.upload`。详见 §3.10。

来源：

- https://ai.google.dev/gemini-api/docs/interactions-breaking-changes-may-2026
- https://ai.google.dev/api/interactions-api
- https://www.npmjs.com/package/@google/genai （2.27.0，`dist/genai.d.ts`、`dist/index.mjs`）

---

## 1. 端点、认证与版本

### 1.1 认证

- 请求头：`x-goog-api-key: $GEMINI_API_KEY`。官方 API 参考的示例都用这个请求头。
- 部分指南示例用的是查询参数 `?key=$GEMINI_API_KEY`，例如破坏性变更指南和 Omni 指南。两种写法在文档中都出现过。
- 请求需带 `Content-Type: application/json`。流式请求可加 `Accept: text/event-stream`。SDK 在 `stream: true` 时会自动把 `Accept` 设为 `text/event-stream`（源码可见）。

### 1.2 Interactions 相关端点（v1beta）

| 方法 | URL | 说明 | 参数 |
|---|---|---|---|
| `POST` | `/v1beta/interactions` | CreateInteraction：创建（执行）一次交互 | 请求体见 §2 |
| `GET` | `/v1beta/interactions/{interactionsId}` | getInteractionById：获取单个交互的完整信息 | 查询参数：`include_input`（boolean，为 true 时响应包含输入）、`stream`（boolean，为 true 时以 SSE 推送事件）、`last_event_id`（string，从该事件之后恢复流，仅当 `stream=true` 时可用） |
| `POST` | `/v1beta/interactions/{interactionsId}/cancel` | cancelInteractionById：取消交互。**只对仍在运行的后台（background）交互有效** | 路径参数 `interactionsId` |
| `DELETE` | `/v1beta/interactions/{interactionsId}` | deleteInteraction：删除已存储的交互 | 返回空响应 |

SDK 对应方法（JS）：`ai.interactions.create(params)`、`ai.interactions.get(id, { include_input, stream, last_event_id })`、`ai.interactions.cancel(id)`、`ai.interactions.delete(id)`。

同一 API 下的其他资源，OpenAPI 中有路径，具体字段本手册不展开：

- `/v1beta/agents`（自定义 managed agent，`ai.agents.create`）
- `/v1beta/environments`（Antigravity 沙箱环境及文件上传）
- `/v1beta/voices`（TTS 声音库、Voice design / replication）
- `/v1beta/webhooks`（静态 webhook，含 `:ping`、`:rotateSigningSecret`）
- `/v1beta/triggers`（定时触发 agent）
- `/v1beta/credentials`

```bash
# 获取
curl -X GET "https://generativelanguage.googleapis.com/v1beta/interactions/$INTERACTION_ID" \
  -H "x-goog-api-key: $GEMINI_API_KEY"
# 断线后恢复流
curl -N -X GET "https://generativelanguage.googleapis.com/v1beta/interactions/$INTERACTION_ID?stream=true&last_event_id=$LAST_EVENT_ID" \
  -H "x-goog-api-key: $GEMINI_API_KEY"
# 取消（仅后台交互）
curl -X POST "https://generativelanguage.googleapis.com/v1beta/interactions/$INTERACTION_ID/cancel" \
  -H "x-goog-api-key: $GEMINI_API_KEY"
# 删除
curl -X DELETE "https://generativelanguage.googleapis.com/v1beta/interactions/$INTERACTION_ID" \
  -H "x-goog-api-key: $GEMINI_API_KEY"
```

### 1.3 API 版本：v1beta 与 v1

- 本手册以 **v1beta** 为准，功能最全。
- 官方另有稳定版 **v1**：`POST https://generativelanguage.googleapis.com/v1/interactions`。v1 的请求体是 v1beta 的子集：
  - 有 `model` / `agent`、`input`、`system_instruction`、`tools`、`response_format`、`stream`、`store`、`background`、`generation_config`、`agent_config`、`previous_interaction_id`；
  - v1 的 `generation_config` 只列出 `max_output_tokens`、`seed`、`stop_sequences`、`thinking_level`、`thinking_summaries`、`tool_choice`；
  - v1 的 `model` 枚举写法带前缀，如 `models/gemini-3.5-flash`，但 v1 文档自己的示例里仍写 `"gemini-3.6-flash"`。
- SDK 可以通过 `api_version` 参数切换版本（源码可见 `api_version` 字段）。

### 1.4 Api-Revision 请求头与 SDK 版本

| 日期 | 阶段 | SDK 用户 | REST 用户 |
|---|---|---|---|
| 2026-05-07 | Opt-in | 新 SDK 可用（Python ≥2.0.0，JS ≥2.0.0），自动启用新 schema | 加 `Api-Revision: 2026-05-20` 启用新 schema，默认仍为旧版 |
| 2026-05-26 | 默认切换 | 已升级则无需操作；1.x SDK 仍返回旧响应 | 新 schema 成为默认；可用 `Api-Revision: 2026-05-07` 临时退回旧版 |
| 2026-06-08 | 下线 | 1.x SDK 调用 Interactions 会失败 | 旧 schema 移除，`Api-Revision` 请求头被忽略 |

- Interactions 概览页写的 SDK 最低版本：Python `google-genai` ≥ 2.3.0，JS `@google/genai` ≥ 2.3.0。
- TTS 指南要求 Python `google-genai` ≥ 2.25.0，JS `@google/genai` ≥ 2.24.0。
- 撰写本文时 npm 最新版为 `@google/genai@2.27.0`（2026-10-03 02:24 北京时间发布，npm 显示为 2026-10-02T18:24Z）。
- 有些指南的 curl 示例里还带着 `-H "Api-Revision: 2026-05-20"`，例如后台执行页。按时间线，这个请求头现在已被忽略，带不带都行。

来源：

- https://ai.google.dev/api/interactions-api
- https://ai.google.dev/api/interactions-api-v1
- https://ai.google.dev/gemini-api/docs/interactions-breaking-changes-may-2026
- https://ai.google.dev/gemini-api/docs/interactions-overview
- https://ai.google.dev/gemini-api/docs/background-execution

---

## 2. 请求体：顶层字段完整表

请求体有两种模式，二选一：

- **ModelInteraction**：提供 `model`，调用普通模型。
- **AgentInteraction**：提供 `agent`，调用 Deep Research、Antigravity 等 agent。

API 参考把 `Interaction` 资源的输出字段（`id`、`status`、`created`、`updated`、`environment_id`、`steps`、`usage` 等）也列在了请求体 schema 里，并标注为 "Output only"。这些字段**不要在请求中发送**，本表单独标出。

"默认值"一列只记录文档明确写出的默认值，其余写"未注明"。

| 字段 | 类型 | 必填 | 适用模式 | 允许值 / 结构 | 默认值 | 说明 |
|---|---|---|---|---|---|---|
| `model` | string（ModelOption） | ModelInteraction 必填 | Model | 见 §4 模型 ID 表；SDK 类型为字面量联合加任意字符串 | — | 使用的模型 |
| `agent` | string（AgentOption） | AgentInteraction 必填 | Agent | 参考枚举：`deep-research-pro-preview-12-2025`、`deep-research-preview-04-2026`、`deep-research-max-preview-04-2026`、`antigravity-preview-05-2026`；也可填自定义 agent 的 id（见 §4.8） | — | 使用的 agent |
| `input` | string ｜ Content ｜ Content[] ｜ Step[] | 参考中标为 optional（v1 标为 required） | 两者 | 见 §3.1 | — | 本轮输入。纯文本可直接传字符串 |
| `system_instruction` | string | 否 | 两者 | 任意字符串 | 未注明 | 系统指令。属于"交互级"参数，`previous_interaction_id` **不会继承**，每轮都要重新传 |
| `generation_config` | GenerationConfig | 否 | Model | 见 §3.2 | — | 模型行为配置（思考、最大 token、TTS、转写、视频任务等）。也是交互级参数，每轮需重传 |
| `agent_config` | AntigravityAgentConfig ｜ CodeMenderAgentConfig ｜ DeepResearchAgentConfig ｜ DynamicAgentConfig | 否 | Agent | 见 §3.6 | — | agent 配置，用 `type` 区分 |
| `response_format` | ResponseFormat ｜ ResponseFormat[] | 否 | 两者 | 见 §3.3（`text`/`image`/`audio`/`video`） | 未注明（图像模型默认同时返回文字和图片；TTS 单次请求默认 wav；Lyria 默认 MP3；视频默认 720p / 16:9） | 输出格式。需要多种模态时传数组 |
| `tools` | Tool[] | 否 | 两者 | 见 §3.4 | 未注明（Deep Research / Antigravity 有默认工具集） | 可供模型调用的工具。交互级参数，每轮需重传 |
| `previous_interaction_id` | string | 否 | 两者 | 之前某次交互的 `id` | — | 服务端续接对话历史。只继承历史（输入与输出），不继承 `tools`、`system_instruction`、`generation_config` |
| `store` | boolean | 否 | 两者 | true / false | **true**（概览页说明默认存储） | 是否存储请求和响应。`store=false` 与 `background=true` 不兼容，也无法再被 `previous_interaction_id` 引用 |
| `stream` | boolean | 否 | 两者 | true / false | 未注明 | 是否以 SSE 流式返回，见 §5.6 |
| `background` | boolean | 否 | 两者 | true / false | 未注明 | 后台运行，立即返回 id，之后轮询或流式获取结果。Deep Research 必须为 true；后台模式需要 `store=true` |
| `service_tier` | string（ServiceTier） | 否 | 两者 | `flex`、`standard`、`priority`、`deferred` | `standard`（Flex 指南说明不填时走 standard） | 服务等级。Flex 价格为标准价 50%；Priority 贵 75–100%；`deferred` 文档无更多说明 **【待确认】** |
| `safety_settings` | SafetySetting[] | 否 | 两者 | 见 §3.8 | — | 安全设置。**矛盾：概览页"Limitations"写 "Custom safety settings are not supported in the Interactions API"，但 API 参考列出了此字段。【待确认】** |
| `environment` | EnvironmentConfig ｜ string | 否 | 两者（实际用于 Antigravity） | `"remote"`（新建沙箱）、已有环境 ID（如 `"env_abc123"`）、或 EnvironmentConfig 对象（§3.7） | — | Agent 沙箱环境 |
| `webhook_config` | WebhookConfig | 否 | 两者 | `{ uris?: string[], user_metadata?: object }` | — | 交互完成时回调到指定 URI（动态 webhook）。webhook 指南注明 `background: true` 是必需的 |
| `labels` | object（string→string） | 否 | 两者 | 键和值 ≤63 个 Unicode 字符，只能包含小写字母、数字、下划线、短横线（允许国际字符）；值可省略；键必须以字母开头 | — | 自定义元数据标签 |
| `continuation_token` | string（byte） | 否 | 两者 | 不透明 token | — | 长解码续写。输出端：状态为 `incomplete` 且可续写时返回；输入端：把最新的 token 原样传回 CreateInteraction 即可继续解码 |
| `id`、`status`、`created`、`updated`、`environment_id`、`steps`、`usage`、`errors` | — | 只读 | — | — | — | Output only，请求中不要填写，见 §5 |

只在 OpenAPI JSON 或 SDK 类型中出现、当前渲染版参考**不再列出**的顶层字段：

- `response_mime_type`：SDK 标为 @deprecated，已被 `response_format[].mime_type` 取代。
- `response_modalities`：SDK 标为 @deprecated，已被 `response_format` 取代；取值枚举 `text`/`image`/`audio`/`video`/`document`。
- `cached_content`（ModelInteraction，格式 `cachedContents/{id}`）：OpenAPI 中仍有，但概览页说 Interactions API **尚不支持**显式缓存。**【待确认/不要使用】**

来源：

- https://ai.google.dev/api/interactions-api （CreateInteraction / Request body）
- https://ai.google.dev/gemini-api/docs/interactions-overview
- https://ai.google.dev/gemini-api/docs/flex-inference
- https://ai.google.dev/gemini-api/docs/priority-inference
- https://ai.google.dev/gemini-api/docs/webhooks
- https://ai.google.dev/static/api/interactions.openapi.json

---
## 3. 嵌套对象详解

### 3.1 `input`：输入的四种写法

| 写法 | 示例 | 用途 |
|---|---|---|
| 字符串 | `"input": "讲个笑话"` | 纯文本单轮 |
| 单个 Content | `"input": {"type":"text","text":"..."}` | 单个多模态块 |
| Content 数组 | `"input": [{"type":"text","text":"描述这张图"},{"type":"image","uri":"https://...","mime_type":"image/png"}]` | 多模态混合 |
| Step 数组 | `"input": [{"type":"user_input","content":[...]},{"type":"model_output","content":[...]},{"type":"user_input","content":[...]}]` | 无状态（客户端自管历史）的多轮对话、回传函数结果等 |

**Content 类型**（每个块都要有 `type`）：

| `type` | 字段 | 允许值 |
|---|---|---|
| `text` | `text`（必填）、`annotations`（Annotation[]，见下表） | — |
| `image` | `data`（base64）或 `uri`、`mime_type`、`resolution` | `mime_type`：`image/png`、`image/jpeg`、`image/webp`、`image/heic`、`image/heif`、`image/gif`、`image/bmp`、`image/tiff`；`resolution`（MediaResolution）：`low`、`medium`、`high`、`ultra_high` |
| `audio` | `data` 或 `uri`、`mime_type`、`channels`、`sample_rate` | `mime_type`：`audio/wav`、`audio/mp3`、`audio/aiff`、`audio/aac`、`audio/ogg`、`audio/flac`、`audio/mpeg`、`audio/m4a`、`audio/l16`、`audio/opus`、`audio/alaw`、`audio/mulaw`、`audio/webm` |
| `document` | `data` 或 `uri`、`mime_type` | `application/pdf`、`text/csv` |
| `video` | `data` 或 `uri`、`mime_type`、`name`（用户自定义名，模型在回答中可引用）、`processing`、`resolution` | `mime_type`：`video/mp4`、`video/mpeg`、`video/mpg`、`video/mov`、`video/avi`、`video/x-flv`、`video/webm`、`video/wmv`、`video/3gpp`；`processing`：`"static"`、`"agentic"`（模型驱动的动态浏览），或对象 `{type:"static", fps, start_offset:"10.5s", end_offset:"30s"}` |

媒体到底怎么提交（base64 规则、`uri` 接受哪些形式、Files API 上传端点、大小限制、各模型矩阵）见 **§3.10**。简单说：请求体只能是 JSON；文件要么以 base64 字符串放进 `data`，要么把 Files API 的 `file.uri`、公开 https URL 或 YouTube URL 放进 `uri`。

**Annotation 类型**（位于 `text` 块的 `annotations` 中，既可出现在输出里，也可出现在输入里，例如 TTS 的 `speech_metadata`）：

| `type` | 字段 |
|---|---|
| `url_citation` | `url`、`title`、`start_index`、`end_index` |
| `file_citation` | `document_uri`、`file_name`、`source`、`page_number`、`media_id`、`custom_metadata`、`start_index`、`end_index` |
| `place_citation` | `place_id`（`places/{id}`）、`name`、`url`、`review_snippets[]{review_id,title,url}`、`start_index`、`end_index` |
| `speech_metadata` | `speaker`、`style`（语音合成风格指令）、`start_index`、`end_index` |
| `word_info` | `text`、`start_offset`、`end_offset`、`speaker`（如 `spk_1`）、`start_index`、`end_index`（ASR 词级信息） |

`start_index` / `end_index` 按**字节**计数。

**可作为输入的 Step 类型**：

| `type` | 字段 | 说明 |
|---|---|---|
| `user_input` | `content: Content[]` | 用户输入 |
| `model_output` | `content: Content[]` | 回放模型以前的输出（无状态模式） |
| `thought` | `signature`、`summary` | 回放思考步骤。无状态模式下**必须原样回传**，包括 signature |
| `function_call` | `id`、`name`、`arguments` | 回放模型的函数调用 |
| `function_result` | `call_id`（必填，对应 `function_call.id`）、`name`、`result`（string，或对象，或 text/image Content 数组）、`is_error` | 回传函数执行结果 |
| 其他 `*_call` / `*_result` | 见 §5.3 | 回放内置工具调用 |

无状态多轮对话时，要把上一轮响应中的所有 steps 原样追加到 `input` 里，再加上新的 `user_input`。

来源：

- https://ai.google.dev/api/interactions-api （Content、Step）
- https://ai.google.dev/gemini-api/docs/interactions-overview
- https://ai.google.dev/gemini-api/docs/function-calling

### 3.2 `generation_config`（GenerationConfig）

只在 ModelInteraction 中使用。

| 字段 | 类型 | 允许值 / 结构 | 说明 |
|---|---|---|---|
| `max_output_tokens` | integer | — | 最大输出 token 数。思考指南注明该上限**包含思考 token**；达到上限时交互状态为 `incomplete` |
| `seed` | integer | — | 解码随机种子，用于复现 |
| `stop_sequences` | string[] | — | 遇到这些序列时停止输出 |
| `thinking_level` | enum | `minimal`、`low`、`medium`、`high` | 思考深度。各模型支持的取值和默认值见 §4.1 |
| `thinking_summaries` | enum | `auto`、`none` | 是否在 `thought` step 中返回思考摘要。思考指南说默认只返回最终输出 |
| `tool_choice` | enum 或 ToolChoiceConfig | 字符串：`auto`、`any`、`none`、`validated`；对象：`{ "allowed_tools": { "mode": "auto"\|"any"\|"none"\|"validated", "tools": ["fn_name", ...] } }` | 工具调用策略。函数调用指南说默认 `auto` |
| `speech_config` | SpeechConfig[] 或 SpeakerConfig | 数组：`[{ "voice": "Kore", "language": "en-US", "speaker": "..." }]`；对象：`{ "speakers": [ {speaker, voice, language}, ... ] }` | TTS 语音配置。单人用数组，多人用 `speakers`（最多 2 人）。TTS 指南的 REST 多人示例里还出现了 `"mode": "conversational"`，参考文档没有这个字段 **【待确认】** |
| `transcription_config` | TranscriptionConfig | `custom_vocabulary: string[]`；`language_codes: string[]`（BCP-47，省略则自动检测）；`mode`：`"verbatim"` ／ `"smart"`，或对象 `{ "type": "verbatim", "diarization_mode": "speaker", "timestamp_granularities": ["word"] }` ／ `{ "type": "smart" }` | 提供此字段即开启语音识别（ASR）。OpenAPI 中还有 `adaptation_phrases`、`language_hints`（已废弃），渲染版参考未列 |
| `video_config` | VideoConfig | `task`：`text_to_video`、`image_to_video`（1–2 张图，分别作首帧和可选尾帧）、`reference_to_video`、`edit`、`extend` | 视频生成任务模式。不填时由模型根据输入自动判断 |

**已废弃，不要再用：**

- `temperature`、`top_p`：SDK 2.27.0 类型里标为 `@deprecated`，渲染版参考不再列出。但文本生成指南还有 `generation_config: { temperature: 1.0 }` 的示例 **【待确认：服务端是否仍接受】**。
- `image_config`：已迁移到 `response_format`。Antigravity 指南明确说，对该 agent 设置 `temperature`、`top_p`、`top_k`、`stop_sequences`、`max_output_tokens` 会返回 400。

来源：

- https://ai.google.dev/api/interactions-api （GenerationConfig）
- https://ai.google.dev/gemini-api/docs/thinking
- https://ai.google.dev/gemini-api/docs/function-calling
- https://ai.google.dev/gemini-api/docs/speech-generation
- https://ai.google.dev/gemini-api/docs/text-generation

### 3.3 `response_format`（多态：单个对象或数组）

用 `type` 区分四种格式。需要多种输出时传数组，例如 `[{"type":"text"},{"type":"image"}]`。

**TextResponseFormat**

| 字段 | 类型 | 允许值 | 说明 |
|---|---|---|---|
| `type` | 必填 | `"text"` | — |
| `mime_type` | enum | `application/json`、`text/plain` | 要结构化 JSON 输出时设为 `application/json` |
| `schema` | object | JSON Schema | 输出需遵守的 schema |

**ImageResponseFormat**

| 字段 | 类型 | 允许值 | 说明 |
|---|---|---|---|
| `type` | 必填 | `"image"` | — |
| `aspect_ratio` | enum | `1:1`、`2:3`、`3:2`、`3:4`、`4:3`、`4:5`、`5:4`、`9:16`、`16:9`、`21:9`、`1:8`、`8:1`、`1:4`、`4:1` | 宽高比。哪些模型支持哪些比例见 §4.3 |
| `image_size` | enum | `512`、`1K`、`2K`、`4K` | 图像尺寸。**必须用大写 K**（指南写明小写会被拒绝）。默认 1K |
| `mime_type` | enum | 参考枚举和 SDK 类型（`ImageResponseFormatMimeType = "image/jpeg"`）都只有 `image/jpeg` | **【待确认】**：图像指南的 JS 示例里写过 `mime_type: 'image/png'`。由于 SDK 的 `ResponseFormat_2` 联合类型包含 `{[k:string]:any}`，写 png 不会触发 TS 报错，服务端是否接受未知。建议省略或用 `image/jpeg`。注意：**输入**图像块的 `mime_type` 可以用 png、jpeg、webp 等，见 §3.10.2 |
| `delivery` | enum | `inline`、`uri` | 以内联 base64 还是 URI 返回 |

**AudioResponseFormat**

| 字段 | 类型 | 允许值 | 说明 |
|---|---|---|---|
| `type` | 必填 | `"audio"` | — |
| `mime_type` | enum | `audio/mp3`、`audio/ogg_opus`、`audio/l16`、`audio/wav`、`audio/alaw`、`audio/mulaw` | TTS 单次请求默认 WAV，流式默认 l16（24kHz 单声道 PCM）；Lyria 默认 MP3 |
| `sample_rate` | integer | Hz | 采样率 |
| `bit_rate` | integer | bps | 只对压缩格式（MP3、Opus）有效 |
| `delivery` | enum | `inline`、`uri` | — |

**VideoResponseFormat**

| 字段 | 类型 | 允许值 | 说明 |
|---|---|---|---|
| `type` | 必填 | `"video"` | — |
| `aspect_ratio` | enum | `16:9`、`9:16` | 默认 16:9（Omni 指南） |
| `resolution` | enum | `360p`、`720p`、`1080p`、`4k` | 默认 720p（Omni 指南） |
| `duration` | string（google-duration，如 `"8s"`） | — | 视频时长；Omni 输出 3–10 秒 |
| `delivery` | enum | `inline`、`uri` | 超过 4MB 的视频建议用 `uri` |
| `gcs_uri` | string | — | 输出写入的 GCS 位置（参考注明适用于 Vertex） |

来源：

- https://ai.google.dev/api/interactions-api （ResponseFormat）
- https://ai.google.dev/gemini-api/docs/structured-output
- https://ai.google.dev/gemini-api/docs/image-generation
- https://ai.google.dev/gemini-api/docs/omni

### 3.4 `tools`（Tool[]）

每个工具对象都用 `type` 区分。

| `type` | 字段 | 说明 |
|---|---|---|
| `function` | `name`、`description`、`parameters`（JSON Schema） | 自定义函数。模型返回 `function_call` step，客户端执行后用 `function_result` 回传。**Python SDK 不支持自动函数调用**（概览页） |
| `google_search` | `search_types`: `web_search`（只返回文本结果）、`image_search`（返回图片字节）、`enterprise_web_search` | Google 搜索接地。图片搜索接地主要用于图像生成模型 |
| `code_execution` | 无 | 代码执行 |
| `url_context` | 无 | 读取提示中的 URL 内容 |
| `file_search` | `file_search_store_names: string[]`、`metadata_filter: string`、`top_k: integer` | 在 File Search Store 中做语义检索 |
| `google_maps` | `latitude`、`longitude`、`enable_widget`（是否返回 widget context token） | Google 地图接地 |
| `computer_use` | `environment`: `browser`、`mobile`、`desktop`；`disabled_safety_policies[]`: `financial_transactions`、`sensitive_data_modification`、`communication_tool`、`account_creation`、`data_modification`、`user_consent_management`、`legal_terms_and_agreements`；`enable_prompt_injection_detection: boolean`；`excluded_predefined_functions: string[]` | 电脑操作（预览） |
| `mcp_server` | `name`、`url`（如 `https://api.example.com/mcp`）、`headers`（鉴权头等）、`allowed_tools: [{ mode, tools: [...] }]` | 远程 MCP。只支持 Streamable HTTP；名称不要含 `-`。**矛盾：概览页写 "Gemini 3 does not support remote MCP, this is coming soon."，但函数调用指南有 gemini-3.8-flash 配合 MCP 的示例。【待确认】** |
| `retrieval` | `retrieval_types[]`: `rag_store`、`exa_ai_search`、`parallel_ai_search`；`rag_store_config{ rag_resources[{rag_corpus, rag_file_ids}], rag_retrieval_config{ top_k, filter{metadata_filter, vector_distance_threshold, vector_similarity_threshold}, hybrid_search{alpha}, ranking{...} } }`；`exa_ai_search_config{ api_key（必填）, custom_config }`；`parallel_ai_search_config{ api_key, custom_config }` | 外部检索。RAG Store 看起来是 Vertex 概念 **【待确认：Gemini API 下是否可用】** |

```json
"tools": [
  {"type": "google_search", "search_types": ["web_search", "image_search"]},
  {"type": "function", "name": "get_weather", "description": "获取天气",
   "parameters": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}}
]
```

来源：

- https://ai.google.dev/api/interactions-api （Tool）
- https://ai.google.dev/gemini-api/docs/function-calling
- https://ai.google.dev/gemini-api/docs/google-search
- https://ai.google.dev/gemini-api/docs/url-context
- https://ai.google.dev/gemini-api/docs/maps-grounding
- https://ai.google.dev/gemini-api/docs/interactions-overview

### 3.5 `tool_choice`

位置是 `generation_config.tool_choice`。

- 字符串：`auto`（默认，模型自行决定）、`any`（必须调用工具）、`none`（禁止调用）、`validated`（参考里只写了 "Validated tool choice"）。
- 对象（ToolChoiceConfig）：`{"allowed_tools": {"mode": "any", "tools": ["get_weather"]}}`，用于限定可以调用的工具。

来源：https://ai.google.dev/api/interactions-api （ToolChoiceConfig）、https://ai.google.dev/gemini-api/docs/function-calling

### 3.6 `agent_config`（按 `type` 区分）

| `type` | 字段 | 说明 |
|---|---|---|
| `deep-research` | `thinking_summaries`：`auto`/`none`（默认 none）；`visualization`：`auto`/`off`（默认 auto）；`collaborative_planning`：boolean（默认 false。为 true 时先返回研究计划，下一轮用户确认后才执行）；`enable_bigquery_tool`：boolean（只在 OpenAPI 中出现） | Deep Research agent |
| `antigravity` | `model`：推理所用模型（文档列出 `gemini-3.8-flash`（默认）、`gemini-3.7-flash`、`gemini-3.6-flash`、`gemini-3.5-flash`、`gemini-3.5-flash-lite`）；`max_total_tokens`：int64 字符串（只在 OpenAPI 中出现） | Antigravity managed agent |
| `code-mender` | `model`、`session_id`、`session_config{max_rounds}`、`find_request{mode: scan\|verify, description, finding_id, source_files[]}`、`fix_request{description, finding_id, source_files[]}` | CodeMender 安全 agent。只出现在 OpenAPI 中，没有指南，对应 agent ID 未列出 **【待确认】** |
| `dynamic` | 只有 `type` | 动态 / 自定义 agent。用法细节 **【待确认】** |

来源：

- https://ai.google.dev/api/interactions-api
- https://ai.google.dev/static/api/interactions.openapi.json
- https://ai.google.dev/gemini-api/docs/deep-research
- https://ai.google.dev/gemini-api/docs/antigravity-agent

### 3.7 `environment`（EnvironmentConfig ｜ string）

- 字符串写法：`"remote"` 表示新建远程沙箱；也可以填已有的环境 ID，例如 `"env_abc123"`。响应里的 `environment_id` 可在后续交互中复用。
- 对象写法：

| 字段 | 说明 |
|---|---|
| `type` | 必填，`"remote"` |
| `environment_id` | 可选。填写后会更新这个已有环境，而不是新建 |
| `sources[]` | 挂载源：`{type: gcs \| inline \| repository \| skill_registry, source（GCS 路径或 GitHub 路径）, target（在沙箱中的路径）, content（inline 内容）, encoding（如 base64）}` |
| `env` | 环境变量（object 或 string） |
| `network` | `"disabled"`，或 `{allowlist: [{domain: "github.com", transform: [{"Authorization": "Bearer ..."}]}, {domain: "*.googleapis.com"}]}`；省略则允许所有出站流量 |

来源：https://ai.google.dev/api/interactions-api （EnvironmentConfig、EnvironmentNetworkEgressAllowlist）、https://ai.google.dev/gemini-api/docs/antigravity-agent

### 3.8 `safety_settings`

结构为 `[{ "type": HarmCategory, "threshold": ..., "method": ... }]`。

- `type`：`hate_speech`、`dangerous_content`、`harassment`、`sexually_explicit`、`civic_integrity`（已废弃）、`image_hate`、`image_dangerous_content`、`image_harassment`、`image_sexually_explicit`、`jailbreak`
- `threshold`：`block_low_and_above`、`block_medium_and_above`、`block_only_high`、`block_none`、`off`
- `method`：`severity`、`probability`（参考写明默认 probability）

**【待确认】**：概览页 Limitations 写着 "Custom safety settings are not supported in the Interactions API"，与参考文档矛盾。

来源：https://ai.google.dev/api/interactions-api 、https://ai.google.dev/gemini-api/docs/interactions-overview

### 3.9 有状态、存储与后台运行的规则

- **默认会存储**（`store: true`）。保留期：付费层 55 天，免费层 1 天。可用 DELETE 端点提前删除。
- `previous_interaction_id` 只继承对话历史。`tools`、`system_instruction`、`generation_config` 等交互级参数**每轮都要重新传**。
- `store: false` 时不能用 `background: true`，之后也不能被 `previous_interaction_id` 引用。
- `background: true` 时立即返回 `id` 和 `status: in_progress`（或 `queued`）。之后用 GET 轮询，或用 `GET ?stream=true` 流式获取；运行中可用 `/cancel` 取消。
- `webhook_config` 要和 `background: true` 一起用。
- 概览页列出的暂不支持项：Batch API、Python SDK 自动函数调用、显式缓存（explicit caching）。

来源：

- https://ai.google.dev/gemini-api/docs/interactions-overview
- https://ai.google.dev/gemini-api/docs/background-execution
- https://ai.google.dev/gemini-api/docs/webhooks

---
## 3.10 文件与媒体的输入输出（媒体输入方式：base64 / URI / Files API 上传）

> 本章统一回答：图像、视频、音频、PDF 等文件在 Interactions API 里**怎么提交**（URL？base64？二进制？），有没有**上传端点**，各模型能用哪些方式、有哪些**大小 / 数量 / 时长限制**，以及生成的图片、音频、视频**怎么取回和下载**。
>
> 所有结论都来自 ai.google.dev 官方文档、v1beta OpenAPI 和 `@google/genai@2.27.0` 的源码与类型定义。SDK 类型是在本地用 `tsc --strict` 实际编译检查过的。没有 API key，所以**没做任何线上服务端验证**。

### 3.10.1 一句话结论

- **Interactions 请求体只接受 JSON**。OpenAPI 里 `POST /v1beta/interactions` 只声明了 `application/json`，不接受 multipart 或裸二进制。
- 文件进入请求只有两种方式，都写在 Content 块（`{type:"image"|"video"|"audio"|"document", ...}`）里：
  1. **`data`**：base64 字符串，内联在 JSON 中；
  2. **`uri`**：一个 URI 字符串。可以是：
     - Files API 返回的 `file.uri`；
     - 公开的 https URL 或预签名 URL；
     - YouTube URL（仅视频理解和部分模型）；
     - GCS 文件需先通过 `files:register` 注册成 Files API 文件，再用其 `uri`。
- **Interactions API 本身没有给模型输入用的上传端点。** 上传要走独立的 **Files API**：`POST https://generativelanguage.googleapis.com/upload/v1beta/files`，使用 resumable 协议；SDK 方法是 `ai.files.upload`。
- Interactions OpenAPI 里唯一的上传路径是 `PUT /upload/v1beta/environments/{env}/files/{path}`。它把文件放进 **Antigravity 沙箱的文件系统**，不是模型输入。
- 后续轮次不需要重复上传：可以用 `previous_interaction_id` 引用上一轮生成的图片或视频；Files API 的文件在 48 小时内也可以反复用 `uri` 引用。

来源：

- https://ai.google.dev/static/api/interactions.openapi.json
- https://ai.google.dev/gemini-api/docs/file-input-methods
- https://ai.google.dev/gemini-api/docs/files
- https://ai.google.dev/api/files

### 3.10.2 媒体 Content 块的字段（输入）

| 块 `type` | 字段（schema 中只有 `type` 必填） | `mime_type` 允许值（OpenAPI / SDK 枚举） | 其他 |
|---|---|---|---|
| `image` | `data` ｜ `uri`、`mime_type`、`resolution` | `image/png`、`image/jpeg`、`image/webp`、`image/heic`、`image/heif`、`image/gif`、`image/bmp`、`image/tiff` | `resolution`：`low`/`medium`/`high`/`ultra_high`（按单个内容项设置，仅 Gemini 3） |
| `video` | `data` ｜ `uri`、`mime_type`、`name`、`processing`、`resolution` | `video/mp4`、`video/mpeg`、`video/mpg`、`video/mov`、`video/avi`、`video/x-flv`、`video/webm`、`video/wmv`、`video/3gpp` | `name`：自定义名称，模型可在回答中引用；`processing` 见 §3.10.6 |
| `audio` | `data` ｜ `uri`、`mime_type`、`channels`、`sample_rate` | `audio/wav`、`audio/mp3`、`audio/aiff`、`audio/aac`、`audio/ogg`、`audio/flac`、`audio/mpeg`、`audio/m4a`、`audio/l16`、`audio/opus`、`audio/alaw`、`audio/mulaw`、`audio/webm` | `channels`、`sample_rate`（通常用于裸 PCM 等格式，文档没有更多说明） |
| `document` | `data` ｜ `uri`、`mime_type` | `application/pdf`、`text/csv` | 文档处理指南说 TXT、Markdown、HTML、XML 等"技术上也能传"，但只按纯文本抽取；这些值不在枚举里 **【待确认】** |

**`data`（base64）的细节：**

- OpenAPI 中 `data` 是 `type: string, format: byte`。SDK 类型是 `data?: string`。
- 本地 tsc 检查：传 `Buffer` 会报错 `Type 'Buffer' is not assignable to type 'string'`。**必须自己转成 base64 字符串**，例如 `fs.readFileSync(p).toString("base64")`，或 `fs.readFileSync(p, {encoding: "base64"})`。
- **标准 base64 还是 URL-safe？** 官方示例一律用标准 base64：`base64 -w0`（Linux）、`base64 --input`（FreeBSD/macOS），以及 Node 的 `toString("base64")`。按 protobuf JSON 映射对 `bytes` 字段的规则，"标准或 URL-safe、带不带 padding 都接受"。但 Interactions 后端是否完全遵循这一规则，文档没写 **【待确认】**。**建议一律用标准 base64 并保留 padding。**
- **不要加 `data:image/png;base64,` 前缀。** 所有官方示例都是裸 base64。带前缀会怎样，文档没有说明 **【待确认：很可能报错】**。
- `base64` 命令默认每 76 字符换行，Linux 下必须加 `-w0`（或 `-w 0`），否则 JSON 里会出现换行。
- 大文件内联时不要用 `-d '...'` 拼进命令行（容易超过参数长度上限）。先写 `payload.json` 再用 `-d @payload.json`，官方示例也是这么做的。

**`mime_type` 是否必填：**

- schema 只把 `type` 列为 required。
- 官方 YouTube 示例省略了 `mime_type`，Omni 中用 Files API `uri` 的视频和图片示例也省略了。
- 但 file-input-methods 页的最佳实践是"**Always provide the correct MIME type**"。
- 内联 `data` 不带 `mime_type` 时能否被接受，文档没写 **【待确认】**。**建议每个块都带 `mime_type`。**
- 用 Files API 时，直接用 `file.mimeType`（SDK）或 `.file.mimeType`（REST 响应）即可。

**图像 MIME 不一致的结论：**

- **输入**（`image` Content 块）：`image/png`、`image/jpeg` 等都在 OpenAPI 和 SDK 枚举里，**都可以用**。
- **输出**（`response_format` 的 `ImageResponseFormat.mime_type`）：OpenAPI 和 SDK 都只声明了 `image/jpeg`（SDK 写作 `type ImageResponseFormatMimeType = "image/jpeg"`）。但官方图像指南的多轮编辑 JS 示例写了 `mime_type: "image/png"`。
- 由于 SDK 的 `ResponseFormat_2` 联合类型末尾有 `{[k: string]: any}`，`"image/png"` **不会**触发 TS 报错（本地已验证）。服务端是否接受 png 输出 **【待确认】**。
- 建议：输出格式**不填**，按响应块里实际的 `mime_type` 决定文件扩展名；需要指定时只用 `image/jpeg`。

**`uri` 接受哪些形式**（依据 file-input-methods 页和各指南示例）：

| URI 形式 | 状态 | 说明 |
|---|---|---|
| Files API 文件 URI（`file.uri`，即 `https://generativelanguage.googleapis.com/v1beta/files/{id}` 一类） | ✅ 文档化 | 单文件 ≤2GB，项目总量 20GB，保存 48 小时。Files API 示例从未打印过 `uri` 的确切字面形式，**直接原样使用返回值，不要自己拼** |
| 公开 https URL / 预签名 URL（AWS、Azure、GCS 等） | ✅ 文档化 | 每个请求最多 100MB；不能需要登录或付费墙；会做安全检查（不通过时为 `URL_RETRIEVAL_STATUS_UNSAFE`）；**不支持 Gemini 2.0 系列**。外链支持的类型清单：图像 bmp/jpeg/png/webp，视频 mp4/mpeg/quicktime/avi/x-flv/mpg/webm/wmv/3gpp，以及 PDF、JSON 和多种 text/* |
| YouTube URL（`https://www.youtube.com/watch?v=...`） | ✅ 视频理解；✅ Nano Banana 2.1 / 3.1 Flash / 3.1 Flash Lite 的视频生图；❌ **Omni 明确不支持** | 只能用公开视频（不能是私有或不公开视频）；免费层每天最多 8 小时；Gemini 2.5 及以后每个请求最多 10 个视频；预览功能 |
| `gs://bucket/object` | ⚠️ 需先调用 `POST /v1beta/files:register` 注册，再用返回 File 的 `uri` | 需要 OAuth 加 Storage Object Viewer 权限，**不能只用 API key**。注册不复制文件，一次注册最长 30 天有效。直接在 Content 的 `uri` 里写 `gs://` 能否用 **【待确认】** |
| `data:` URL、本地路径、`file://` | ❌ 无文档 | 不要使用 |

来源：

- https://ai.google.dev/static/api/interactions.openapi.json （ImageContent / VideoContent / AudioContent / DocumentContent）
- https://ai.google.dev/gemini-api/docs/file-input-methods
- https://ai.google.dev/gemini-api/docs/image-understanding
- https://ai.google.dev/gemini-api/docs/video-understanding
- https://ai.google.dev/gemini-api/docs/image-generation
- https://protobuf.dev/programming-guides/json/
- `@google/genai@2.27.0` `dist/genai.d.ts`（`ImageContent`、`ImageResponseFormatMimeType`、`ResponseFormat_2`）

### 3.10.3 大小与数量限制（汇总）

| 项目 | 限制 | 来源 / 备注 |
|---|---|---|
| 内联（`data`）总请求大小 | **file-input-methods 页：每请求 100MB（PDF 50MB）**；**图像、音频、视频理解页：总请求 20MB**（含提示词、系统指令和所有内联字节） | **两处矛盾 【待确认】**。稳妥做法：总请求超过 20MB 就改用 Files API。Files API 页写的是"总请求超过 100MB（PDF 50MB）时**必须**用 Files API" |
| 外部 URL | 每请求 100MB | file-input-methods |
| Files API 单文件 / 项目总量 / 保存期 | 2GB / 20GB / 48 小时（到期自动删除） | files、api/files。视频理解页写 File API "20GB (paid) / 2GB (free)"，与"每文件 2GB"的写法不一致 **【待确认】** |
| GCS 注册 | 单文件 2GB，无总量上限 | file-input-methods |
| 单请求图像数（理解类模型） | 最多 3,600 张 | image-understanding |
| PDF | ≤50MB 或 ≤1000 页（内联和 Files API 都适用）；每页约 258 token；大页缩到 3072×3072，小页放大到 768×768 | document-processing |
| 音频（理解） | 最长 9.5 小时 / 每个提示；32 token/秒；下采样到 16Kbps；多声道合并为单声道 | audio。media-resolution 页给 Gemini 3 的数值是 25 token/秒，两处数值不同 |
| 音频（gemini-3.5-transcribe） | 单次 ≤1 小时；开启说话人分离或词级时间戳时 ≤30 分钟；custom_vocabulary ≤1000 条 | transcribe、模型页 |
| 视频（理解） | 1M 上下文模型：低分辨率约 3 小时，高分辨率约 1 小时；静态模式 1 FPS；每请求最多 10 个视频（2.5 及以后） | video-understanding |
| Nano Banana 参考图 | 3.1 Flash Lite Image：最多 14 张物体图；2.1 / 3.1 Flash Image：10 张物体 + 4 张角色；3 Pro Image：6 张物体 + 5 张角色 + 3 张风格参考（合计 14） | image-generation 的"Use up to 14 reference images"表。同页 Limitations 写的是"3-pro-image 5 张高保真、共 14 张；2.5-flash-image 建议 ≤3 张"，口径不同 |
| Nano Banana 视频输入 | 只有 2.1、3.1 Flash Image、3.1 Flash Lite Image 支持；不支持音频输入 | image-generation |
| Omni 输入视频 | 用于编辑或延长的上传视频 ≤10 秒（多轮延长模型自己生成的视频不受此限）；视频参考最多 3 段、每段 ≤3 秒，音频被忽略；不支持音频参考；不支持 YouTube；不支持跨多个视频推理 | omni |
| Omni 输出 | 3–10 秒，24 FPS；超过 4MB（720p 以上时）建议 `delivery:"uri"` | omni、模型页 |
| Lyria 3.5 | 文本加**最多 10 张图像**；输入 131,072 token | music-generation、模型页 |
| Antigravity | 只接受 text、image，且图像**必须内联 base64**；沙箱 inline 源每文件 1MB、总计 2MB；git 源 500MB；GCS 源 2GB | antigravity-agent、agent-environment |

### 3.10.4 各模型支持的提交方式（矩阵）

✅ = 文档有示例或明确说明；◐ = 通用规则（file-input-methods 说"新方法适用于所有端点，包括 Interactions"）推断可用，但该模型页没有示例 **【待确认】**；❌ = 文档明确不支持或不接受该模态。

| 模型 | 可接受的输入模态 | 内联 base64 `data` | Files API `uri` | 外部 https URL | YouTube | 备注 |
|---|---|---|---|---|---|---|
| 3.x Flash / Flash-Lite / 3.1 Pro 等文本模型 | 文本、图像、视频、音频、PDF | ✅ | ✅ | ✅（2.0 系列除外） | ✅（视频） | 视频可用 `processing`；图像和视频可用 `resolution` |
| `gemini-3.5-transcribe` | 音频 | ◐ | ✅（官方示例） | ◐ | ❌（不是音频） | — |
| `gemini-nano-banana-2.1` / `gemini-3.1-flash-image` / `gemini-3.1-flash-lite-image` | 文本、图像、视频、PDF（不接受音频） | ✅（图像） | ✅（视频，按指南；图像 ◐） | ◐ | ✅（视频生图） | — |
| `gemini-3-pro-image` | 图像、文本 | ✅ | ◐ | ◐ | ❌（不接受视频输入） | — |
| TTS（3.8 Flash / Flash-Lite） | 仅文本 | — | — | — | — | — |
| `lyria-3.5` / `lyria-3-clip-preview` | 文本、图像（≤10 张） | ✅（图像） | ◐ | ◐ | ❌ | **不接受视频或音频输入**，没有"视频配乐"能力，见 §3.10.9 |
| `gemini-omni-1.1-flash` | 文本、图像、视频（≤10 秒） | ✅（图像、视频） | ✅（图像、视频，官方推荐用于视频） | ◐ | ❌（明确不支持） | 不支持音频参考 |
| Deep Research | 文本、图像、文档 | ◐ | ◐ | ✅（图像、PDF URL 示例） | ◐ | — |
| Antigravity | 文本、图像 | ✅（**必须**） | ❌ | ❌ | ❌ | 其他文件用 `environment.sources` 或沙箱上传（§3.10.10） |

### 3.10.5 Files API：上传、查询、删除、下载

**端点**（Files API 是独立资源，不属于 Interactions）：

| 操作 | 方法与 URL | 说明 |
|---|---|---|
| 上传（media.upload） | `POST https://generativelanguage.googleapis.com/upload/v1beta/files` | resumable 协议，两步：start → upload,finalize |
| 只建元数据 | `POST https://generativelanguage.googleapis.com/v1beta/files` | 参考中列为"metadata-only requests" |
| 注册 GCS | `POST https://generativelanguage.googleapis.com/v1beta/files:register`，请求体 `{"uris":["gs://..."]}` | 需要 OAuth（`Authorization: Bearer` 加 `x-goog-user-project`） |
| 获取 | `GET https://generativelanguage.googleapis.com/v1beta/files/{id}` | 返回 File |
| 列表 | `GET https://generativelanguage.googleapis.com/v1beta/files?pageSize=&pageToken=` | 返回 `{files:[...], nextPageToken}` |
| 删除 | `DELETE https://generativelanguage.googleapis.com/v1beta/files/{id}` | 返回空对象 |
| 下载 | `GET https://generativelanguage.googleapis.com/v1beta/files/{id}:download?alt=media` | **只能下载模型生成的文件**（例如 Omni 视频）；"you can't download user-uploaded files"。需要 API key（官方用 `?key=`；SDK 走 `x-goog-api-key` 请求头） |

**resumable 上传协议**（官方 curl，原样整理）：

1. **第 1 步（start）**：POST 到 `/upload/v1beta/files`。
   - 请求头：`X-Goog-Upload-Protocol: resumable`、`X-Goog-Upload-Command: start`、`X-Goog-Upload-Header-Content-Length: <字节数>`、`X-Goog-Upload-Header-Content-Type: <MIME>`、`Content-Type: application/json`。
   - 请求体：`{"file":{"display_name":"..."}}`。
   - 从**响应头** `x-goog-upload-url` 取出上传地址。
2. **第 2 步（upload, finalize）**：向该地址 POST 原始字节（`--data-binary @file`）。
   - 请求头：`Content-Length: <字节数>`、`X-Goog-Upload-Offset: 0`、`X-Goog-Upload-Command: upload, finalize`。
   - 响应 JSON 为 `{"file": File}`。

SDK 源码（`uploadFile` / `fetchUploadUrl`）用的是同样这组请求头，有文件名时还会加 `X-Goog-Upload-File-Name`。

**File 资源字段**（REST 为 camelCase，SDK 类型同名）：

| 字段 | 说明 |
|---|---|
| `name` | 形如 `files/abc-123`。ID 最多 40 个字符，只能是小写字母、数字和 `-` |
| `displayName` | ≤512 字符 |
| `mimeType`、`sizeBytes`、`createTime`、`updateTime`、`sha256Hash` | — |
| `expirationTime` | 计划删除的时间 |
| `uri` | **放进 Interactions Content 的 `uri` 字段的就是它** |
| `downloadUri` | 下载地址 |
| `state` | `STATE_UNSPECIFIED` / `PROCESSING` / `ACTIVE` / `FAILED` |
| `source` | `UPLOADED` / `GENERATED` / `REGISTERED` |
| `error` | 处理失败时的错误 |
| `videoMetadata.videoDuration` | 视频时长 |

**`state` 与等待处理完成：**

- `PROCESSING` 表示"还不能用于推理"。视频理解和 Omni 的官方示例都会轮询到 `ACTIVE` 后再使用。
- 图像和 PDF 的示例没有轮询。是否也会经历 PROCESSING 状态，文档没写 **【待确认】**。稳妥做法是统一轮询。

**SDK 用法要点**（本地 tsc 验证）：

- `ai.files.upload({ file: "路径" 或 Blob, config: { mimeType, displayName, name } })`。
  - 配置字段是 **camelCase 的 `mimeType`**。
  - 很多官方 JS 示例写成 `config: { mime_type: ... }`，tsc 会报错 `'mime_type' does not exist in type 'UploadFileConfig'. Did you mean 'mimeType'?`。
  - 运行时这个 `mime_type` 会被忽略，SDK 改为按扩展名推断；推断不出时抛出 `Can not determine mimeType`。
- 返回值是 File 对象：`file.uri`、`file.mimeType`、`file.name`、`file.state`。
- 文档处理页的 JS 示例写了 `uploadedFile.mime_type`，但 SDK 的 File 类型字段是 `mimeType`，**那样写会得到 undefined**。
- `file.state` 是字符串枚举，应写 `f.state === "ACTIVE"`。Omni 指南写成 `fInfo.state.name === 'ACTIVE'`，与 SDK 类型不符（像是 Python 写法）。
- 注意区分：Files API 用 camelCase，Interactions 请求体用 snake_case。即 `{ type:"image", uri: file.uri, mime_type: file.mimeType }`。

```bash
# ===== Files API：resumable 上传，然后在 Interactions 中引用 =====
FILE=cat.png
MIME=$(file -b --mime-type "$FILE")
BYTES=$(wc -c < "$FILE")

curl -s "https://generativelanguage.googleapis.com/upload/v1beta/files" \
  -D hdr.tmp \
  -H "x-goog-api-key: $GEMINI_API_KEY" \
  -H "X-Goog-Upload-Protocol: resumable" \
  -H "X-Goog-Upload-Command: start" \
  -H "X-Goog-Upload-Header-Content-Length: $BYTES" \
  -H "X-Goog-Upload-Header-Content-Type: $MIME" \
  -H "Content-Type: application/json" \
  -d '{"file": {"display_name": "cat"}}' > /dev/null
UPLOAD_URL=$(grep -i "x-goog-upload-url: " hdr.tmp | cut -d" " -f2 | tr -d "\r"); rm hdr.tmp

curl -s "$UPLOAD_URL" \
  -H "Content-Length: $BYTES" \
  -H "X-Goog-Upload-Offset: 0" \
  -H "X-Goog-Upload-Command: upload, finalize" \
  --data-binary "@$FILE" > file_info.json
FILE_URI=$(jq -r '.file.uri' file_info.json)
FILE_NAME=$(jq -r '.file.name' file_info.json)   # files/xxxx

# 可选：等待 ACTIVE（视频必须等）
until [ "$(curl -s "https://generativelanguage.googleapis.com/v1beta/$FILE_NAME" -H "x-goog-api-key: $GEMINI_API_KEY" | jq -r .state)" = "ACTIVE" ]; do sleep 5; done

curl -s -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-nano-banana-2.1",
    "input": [
      {"type": "image", "uri": "'"$FILE_URI"'", "mime_type": "'"$MIME"'"},
      {"type": "text", "text": "把这只猫改成水彩画风格"}
    ],
    "response_format": {"type": "image"}
  }' > resp.json

# 列表 / 删除
curl -s "https://generativelanguage.googleapis.com/v1beta/files?pageSize=10" -H "x-goog-api-key: $GEMINI_API_KEY"
curl -s -X DELETE "https://generativelanguage.googleapis.com/v1beta/$FILE_NAME" -H "x-goog-api-key: $GEMINI_API_KEY"
```

```js
import { GoogleGenAI } from "@google/genai";
import * as fs from "node:fs";
const ai = new GoogleGenAI({}); // GEMINI_API_KEY

// 上传（注意：config 用 camelCase 的 mimeType）
let f = await ai.files.upload({ file: "cat.png", config: { mimeType: "image/png", displayName: "cat" } });
while (f.state === "PROCESSING") {               // 字符串枚举
  await new Promise(r => setTimeout(r, 5000));
  f = await ai.files.get({ name: f.name });
}
if (f.state === "FAILED") throw new Error("file processing failed");

// 在 Interactions 中引用（请求体用 snake_case 的 mime_type）
const r = await ai.interactions.create({
  model: "gemini-nano-banana-2.1",
  input: [
    { type: "image", uri: f.uri, mime_type: f.mimeType },
    { type: "text", text: "把这只猫改成水彩画风格" },
  ],
  response_format: { type: "image" },
});
if (r.output_image?.data) fs.writeFileSync("out.jpg", Buffer.from(r.output_image.data, "base64"));

// 列表 / 删除
for await (const file of await ai.files.list({ config: { pageSize: 10 } })) console.log(file.name, file.expirationTime);
await ai.files.delete({ name: f.name });
```

来源：

- https://ai.google.dev/gemini-api/docs/files
- https://ai.google.dev/api/files
- https://ai.google.dev/gemini-api/docs/file-input-methods
- https://ai.google.dev/gemini-api/docs/document-processing
- https://ai.google.dev/gemini-api/docs/omni
- `@google/genai@2.27.0`：`dist/node/index.mjs`（`uploadFile`、`fetchUploadUrl`、`downloadFile`）、`dist/genai.d.ts`（`UploadFileConfig`、`File`、`FileState`）

### 3.10.6 视频输入（理解、生图、生视频通用）

- **提交方式**：
  - 内联 `data` + `mime_type`：适合较小文件，理解页建议总请求 <20MB、时长 <1 分钟；
  - Files API `uri`：官方推荐，适合 100MB 以上、10 分钟以上或需要复用的文件，要等 `ACTIVE`；
  - GCS 注册后的 `uri`；
  - YouTube URL：只限公开视频，Omni 不支持。
- **`processing`**（理解类）：
  - `"static"`：默认，1 FPS，所有模型都支持。
  - `"agentic"`：模型自己在时间线上浏览，只支持 3.8 / 3.7 / 3.6 Flash 和 3.5 Flash-Lite。
  - 对象 `{type:"static", fps, start_offset, end_offset}`。
- **偏移量的写法有矛盾：**
  - 参考和 SDK 类型都是**字符串**，格式为"十进制秒数加 `s`"，如 `"10.5s"`。
  - 视频理解指南的示例写的是数字 `start_offset: 1200`。
  - 本地 tsc 检查：数字不符合 SDK 类型 `string`。**建议写 `"1200s"`。**
  - 数字形式服务端是否接受 **【待确认】**。
- **`resolution`**：
  - 可选 `low` / `medium` / `high`，与 `processing` 相互独立。
  - Gemini 3 下视频帧 token：默认和 low 都是 70，high 是 280。
  - 静态模式下另一组口径：low 每帧 66 token，其他档每帧 258 token，约 100 或 300 token/秒。
- **`name`**：给视频块起名，模型可在回答中引用。
- **Prompt 位置**：
  - 视频理解页：单视频时，把文本放在视频**之后**；
  - 图像理解页：单图时，把文本放在图像**之前**；
  - Files 页的提示策略：单图或单视频时，把图像或视频放在**前面**。
  - 三处说法不一致，只影响效果，不影响请求是否合法。

```bash
# 内联小视频
B64=$(base64 -w0 clip.mp4)
cat > payload.json <<JSON
{"model":"gemini-3.8-flash","input":[
  {"type":"video","data":"$B64","mime_type":"video/mp4","processing":{"type":"static","fps":2,"start_offset":"0s","end_offset":"8s"},"resolution":"high"},
  {"type":"text","text":"逐秒描述画面变化"}]}
JSON
curl -s -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" -d @payload.json

# YouTube（理解）
curl -s -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" \
  -d '{"model":"gemini-3.8-flash","input":[{"type":"video","uri":"https://www.youtube.com/watch?v=9hE5-98ZeCg"},{"type":"text","text":"三句话总结"}]}'
```
```js
const v = await ai.files.upload({ file: "talk.mp4", config: { mimeType: "video/mp4" } });
// …轮询到 ACTIVE（见 §3.10.5）…
const r = await ai.interactions.create({
  model: "gemini-3.8-flash",
  input: [
    { type: "video", uri: v.uri, mime_type: v.mimeType, name: "talk", processing: "agentic" },
    { type: "text", text: "找出演讲者提到预算的时间点（MM:SS）" },
  ],
  stream: false,
});
```

来源：

- https://ai.google.dev/gemini-api/docs/video-understanding
- https://ai.google.dev/gemini-api/docs/interactions/media-resolution
- https://ai.google.dev/api/interactions-api （MediaProcessing）

### 3.10.7 音频输入

- **格式**：`audio/wav`、`mp3`、`aiff`、`aac`、`ogg`、`flac`、`mpeg`、`m4a`、`l16`、`opus`、`alaw`、`mulaw`、`webm`。
- **提交方式**：内联 `data`（总请求 ≤20MB），或 Files API `uri`（超过 20MB 时推荐）。
- **转写**：用 `gemini-3.5-transcribe` 加 `generation_config.transcription_config`（见 §4.2）；单次 ≤1 小时，开启说话人分离或时间戳时 ≤30 分钟。
- `channels`、`sample_rate` 字段的用法（例如传 `audio/l16` 裸 PCM 时是否必须给出）文档没有说明 **【待确认】**。

```bash
B64=$(base64 -w0 note.mp3)
cat > payload.json <<JSON
{"model":"gemini-3.8-flash","input":[{"type":"text","text":"总结这段录音"},{"type":"audio","data":"$B64","mime_type":"audio/mp3"}]}
JSON
curl -s -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" -d @payload.json
```
```js
const a = await ai.files.upload({ file: "meeting.m4a", config: { mimeType: "audio/m4a" } });
const r = await ai.interactions.create({
  model: "gemini-3.5-transcribe",
  input: [{ type: "audio", uri: a.uri, mime_type: a.mimeType }],
  generation_config: { transcription_config: { mode: { type: "verbatim", diarization_mode: "speaker", timestamp_granularities: ["word"] } } },
});
```

来源：

- https://ai.google.dev/gemini-api/docs/audio
- https://ai.google.dev/gemini-api/docs/transcribe
- https://ai.google.dev/gemini-api/docs/models/gemini-3.5-transcribe

### 3.10.8 PDF / 文档输入

- **块类型**：`{type:"document", data|uri, mime_type:"application/pdf"}`。枚举里还有 `text/csv`。
- **限制**：每个 PDF ≤50MB 或 ≤1000 页，内联和 Files API 都适用。多个 PDF 可以放进同一个请求，只要总量不超过上下文窗口。
- **提交方式**：内联 base64、Files API `uri`、公开 https URL 都有官方示例（file-input-methods、document-processing、deep-research）。

```bash
B64=$(base64 -w0 report.pdf)
cat > payload.json <<JSON
{"model":"gemini-3.8-flash","input":[{"type":"text","text":"概括要点"},{"type":"document","data":"$B64","mime_type":"application/pdf"}]}
JSON
curl -s -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" -d @payload.json
```
```js
const r = await ai.interactions.create({
  model: "gemini-3.8-flash",
  input: [
    { type: "text", text: "概括要点" },
    { type: "document", uri: "https://arxiv.org/pdf/1706.03762", mime_type: "application/pdf" }, // 公开 URL
  ],
});
```

来源：

- https://ai.google.dev/gemini-api/docs/document-processing
- https://ai.google.dev/gemini-api/docs/file-input-methods

### 3.10.9 Lyria：图生音乐；"视频配乐"未文档化

- `lyria-3.5` 和 `lyria-3-clip-preview` 的模型页和音乐指南都写明：**输入只有文本和图像**，可以带最多 **10 张图**。官方示例用的是内联 base64 图像。
- **没有任何官方文档描述"视频输入生成音乐"**：模型页、音乐指南、Lyria 提示指南和 2026-10-07 当天的线上页面都核对过。所以"Lyria 3.5 video-to-music"在 Interactions API 中**没有文档依据**，【待确认】是否存在。
- 可行的替代做法（以下均为推断，非官方能力）：
  - 自己从视频中抽几张关键帧（≤10 张）作为 `image` 输入，并在文字里写明节奏和时长；
  - 或者先用理解类模型分析视频，得到文字描述，再交给 Lyria。
- **输出**：
  - 默认 MP3，44.1kHz 立体声，以内联 base64 的 `audio` 块返回；同时可能有歌词或结构 `text` 块，可能交错出现。
  - Lyria 3.5 可以通过 `response_format` 请求 WAV，但官方示例只写了 `{type:"audio"}`，没有给出具体 `mime_type`（推测为 `audio/wav`）**【待确认】**。
  - Lyria 是否支持 `delivery:"uri"` **【待确认】**。

```bash
B64=$(base64 -w0 sunset.jpg)
cat > payload.json <<JSON
{"model":"lyria-3.5","input":[
  {"type":"text","text":"根据这张图的氛围写一首 2 分钟的氛围电子乐，纯音乐"},
  {"type":"image","mime_type":"image/jpeg","data":"$B64"}]}
JSON
curl -s -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" -d @payload.json \
 | jq -r '.steps[] | select(.type=="model_output") | .content[] | select(.type=="audio") | .data' | head -n1 | base64 -d > music.mp3
```
```js
const img = fs.readFileSync("sunset.jpg").toString("base64");
const r = await ai.interactions.create({
  model: "lyria-3.5",
  input: [{ type: "text", text: "根据这张图的氛围写一首氛围电子乐，纯音乐" }, { type: "image", mime_type: "image/jpeg", data: img }],
});
for (const s of r.steps) if (s.type === "model_output") for (const c of s.content ?? []) {
  if (c.type === "audio") fs.writeFileSync("music.mp3", Buffer.from(c.data, "base64")); // 扩展名按 c.mime_type
  if (c.type === "text") console.log(c.text);
}
```

来源：

- https://ai.google.dev/gemini-api/docs/music-generation
- https://ai.google.dev/gemini-api/docs/models/lyria-3.5
- https://ai.google.dev/gemini-api/docs/models/lyria-3-clip-preview
- https://ai.google.dev/gemini-api/docs/lyria-prompt-guide

### 3.10.10 Agent 的文件输入

- **Deep Research**：官方示例通过 https URL 传入图像和 PDF：`{type:"image", uri:"https://...jpg", mime_type:"image/jpeg"}`、`{type:"document", uri:"https://arxiv.org/pdf/...", mime_type:"application/pdf"}`。也可以用 `file_search` 工具检索已上传的语料。
- **Antigravity**：
  - 只接受 `text` 和 `image`，图像**必须是内联 base64 的 `data`**；
  - 其他文件放进沙箱，有两种方式：
    - 用 `environment.sources`，类型为 `inline`（≤1MB/文件，总计 ≤2MB）、`repository`（≤500MB）、`gcs`（≤2GB）、`skill_registry`；
    - 对已有环境调用沙箱上传：`PUT https://generativelanguage.googleapis.com/upload/v1beta/environments/{env_id}/files/{path}`，请求体为原始字节，`Content-Type` 设为文件类型。**必须带 `/upload/` 前缀，否则返回 400。** 上传 `.tar` / `.tar.gz` 时可加 `extract=true` 解包；SDK 方法是 `client.environments.files.upload(...)`。
  - 取回沙箱文件：`GET /v1beta/environments/{env_id}/files/{path}?alt=media`；对目录请求时返回 tar 包。

```bash
curl -X PUT "https://generativelanguage.googleapis.com/upload/v1beta/environments/$ENV_ID/files/workspace/data/input.csv" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: text/csv" --data-binary @input.csv
curl -L "https://generativelanguage.googleapis.com/v1beta/environments/$ENV_ID/files/workspace/report.pdf?alt=media" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -o report.pdf
```

来源：

- https://ai.google.dev/gemini-api/docs/deep-research
- https://ai.google.dev/gemini-api/docs/antigravity-agent
- https://ai.google.dev/gemini-api/docs/agent-environment

### 3.10.11 输出文件：inline 与 uri、下载、保存期

| 输出 | 默认交付方式 | `delivery:"uri"` | 下载与保存 |
|---|---|---|---|
| 图像（Nano Banana） | `model_output` 中的 `{type:"image", data(base64), mime_type}`；思考过程中的中间图在 `thought.summary` 里 | schema 允许 `inline`/`uri`，但**图像指南没有 uri 示例**，返回的 URI 形式和下载方式 **【待确认】** | 用 `Buffer.from(data,"base64")` 写文件，扩展名按 `mime_type` |
| TTS 音频 | 单次请求为内联 `audio/wav`（RIFF，16bit，单声道，24kHz）；流式为 `audio/l16` 裸 PCM | schema 允许，但 **TTS 指南没有 uri 示例** **【待确认】** | 指南列出的输出格式只有 wav、l16、mulaw、alaw，`sample_rate` 可设 24000、16000、8000。参考枚举里的 `audio/mp3`、`audio/ogg_opus` 能否用于 TTS **【待确认】** |
| Lyria 音乐 | 内联 MP3 base64 | **【待确认】** | 同上 |
| Omni 视频 | 内联 `{type:"video", mime_type:"video/mp4", data}` | ✅ 有文档：返回 `uri: "https://generativelanguage.googleapis.com/v1beta/files/...:download?alt=media"`。超过 4MB 时推荐 | 先轮询 `GET /v1beta/files/{id}` 直到 `state=ACTIVE`，再 `GET .../files/{id}:download?alt=media`（**需要 API key**）。SDK：`ai.files.download({ file: interaction.output_video, downloadPath })` |

**Omni 用 uri 交付的已知坑：**

1. 之后用 `GET /v1beta/interactions/{id}` 取回时，视频**总是以内联 base64 返回**；`uri` 只保证出现在创建响应或 SSE 中。
2. 官方 REST 示例用 `jq -r '.output_video.uri'` 从原始 JSON 取 uri，但 `output_video` 是 **SDK 专有**的便捷属性，原始 REST 响应里没有。应改为从 `steps[].content[]` 中取 `type=="video"` 的 `uri`。
3. 官方示例中 `cut -d'/' -f2` 和正则 `/files\/([a-zA-Z0-9]+)/` 都无法正确提取包含 `-` 的文件 ID（File ID 允许 `-`）。SDK 的 `tFileName` 也用 `/[a-z0-9]+/` 截取 ID，遇到含 `-` 的 ID 理论上同样会截断 **【待确认】**。下面的示例改用 `sed` 提取 `files/` 和 `:` 之间的整段。
4. 生成文件（`source=GENERATED`）的保存期：Files API 统一写的是 48 小时，生成的视频是否同样适用，文档没单独说明。**以 `expirationTime` 字段为准【待确认】。**

**其他保存与复用规则：**

- 交互本身（含内联的输出媒体）在 `store=true` 时保存：付费层 55 天（可设为 7/14/28/55 天），免费层 1 天。
- Omni 指南建议，追求速度时可设 `store=false`，但这样就**不能**再用 `previous_interaction_id` 继续编辑。

```bash
# Omni：请求 uri 交付并下载
RESP=$(curl -s -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" \
  -d '{"model":"gemini-omni-1.1-flash","input":"海边日落延时摄影","response_format":{"type":"video","delivery":"uri","resolution":"1080p"}}')
VURI=$(echo "$RESP" | jq -r '.steps[] | select(.type=="model_output") | .content[] | select(.type=="video") | .uri' | head -n1)
FID=$(echo "$VURI" | sed -E 's#.*files/([^:?/]+).*#\1#')
until [ "$(curl -s "https://generativelanguage.googleapis.com/v1beta/files/$FID" -H "x-goog-api-key: $GEMINI_API_KEY" | jq -r .state)" = "ACTIVE" ]; do sleep 5; done
curl -L "https://generativelanguage.googleapis.com/v1beta/files/$FID:download?alt=media" -H "x-goog-api-key: $GEMINI_API_KEY" -o out.mp4
```
```js
const r = await ai.interactions.create({
  model: "gemini-omni-1.1-flash", input: "海边日落延时摄影",
  response_format: { type: "video", delivery: "uri", resolution: "1080p" },
});
const v = r.output_video;                                   // {type:'video', uri, mime_type}
const id = v.uri.match(/files\/([^:?/]+)/)[1];              // 允许包含 '-'
for (;;) { const f = await ai.files.get({ name: `files/${id}` }); if (f.state === "ACTIVE") break; if (f.state === "FAILED") throw new Error("failed"); await new Promise(s => setTimeout(s, 5000)); }
await ai.files.download({ file: `files/${id}`, downloadPath: "out.mp4" }); // 传 name 字符串，避开 uri 解析
```

来源：

- https://ai.google.dev/gemini-api/docs/omni
- https://ai.google.dev/gemini-api/docs/speech-generation
- https://ai.google.dev/gemini-api/docs/music-generation
- https://ai.google.dev/gemini-api/docs/image-generation
- https://ai.google.dev/gemini-api/docs/files
- https://ai.google.dev/gemini-api/docs/interactions-overview
- `@google/genai@2.27.0`：`tFileName`、`downloadFile`

### 3.10.12 跨轮复用媒体（不必重复上传）

- **有状态**（`previous_interaction_id`）：
  - 图像指南的多轮编辑示例：第二轮只传文字指令和 `previous_interaction_id`，模型就会修改上一轮生成的图。
  - Omni 指南说明，用 `previous_interaction_id` "无需重新上传上一段视频"即可继续编辑或延长。延长模型自己生成的视频不受 10 秒限制，也可以生成对白。
  - 用户上传的输入图像是否也会随历史保留（第三轮仍能引用第一轮上传的原图），机制上应该如此，因为历史包含输入，但文档没有专门说明 **【待确认】**。
- **无状态**：把上一轮响应的 `steps` 原样放回 `input`，其中包括带 base64 `data` 的 `model_output` 图像或视频块和 `thought` 签名。这样请求体会变大，可能触发 §3.10.3 的内联大小上限。
- **Files API**：同一个 `file.uri` 在 48 小时内可以在任意多个请求中引用。
- 把**生成的**视频的 uri（GENERATED 文件）直接作为下一次请求的输入 `uri`，文档没有示例 **【待确认】**。官方推荐的做法是 `previous_interaction_id`。

来源：

- https://ai.google.dev/gemini-api/docs/image-generation （Multi-turn image editing）
- https://ai.google.dev/gemini-api/docs/omni （Stateful video editing、Video extension）
- https://ai.google.dev/gemini-api/docs/interactions-overview

---

## 4. 按模型家族分节

### 4.0 模型 ID 总表与思考等级

**v1beta API 参考中 `model` 字段的枚举**（原样列出）：

```
gemini-2.5-flash, gemini-2.5-pro, gemini-2.5-flash-lite, gemma-4-26b-a4b-it, gemma-4-31b-it,
gemini-flash-latest, gemini-flash-lite-latest, gemini-pro-latest, gemini-2.5-flash-image,
gemini-3-flash-preview, gemini-3.1-pro-preview, gemini-3.1-pro-preview-customtools, gemini-3.1-flash-lite,
gemini-3-pro-image, nano-banana-pro-preview, gemini-3.1-flash-image, gemini-3.1-flash-tts-preview,
gemini-3.5-flash, gemini-3.6-flash, gemini-3.7-flash, gemini-3.8-flash, gemini-3.8-flash-tts,
gemini-3.8-flash-lite-tts, lyria-3-clip-preview, lyria-3-pro-preview, lyria-3.5,
gemini-robotics-er-1.6-preview, gemini-robotics-er-2-preview, gemini-omni-1.1-flash, gemini-omni-flash-preview
```

有些 ID 不在参考枚举里，但模型页或指南在 Interactions API 示例中实际使用了：

- `gemini-nano-banana-2.1`
- `gemini-3.1-flash-lite-image`
- `gemini-3.5-flash-lite`
- `gemini-3.5-transcribe`

SDK 类型是 `Model = 字面量联合 | (string & {})`，任意字符串都能通过类型检查。

**关于 `models/` 前缀：**

- v1beta 参考的枚举和所有 v1beta 示例都用裸 ID，例如 `gemini-3.8-flash`。
- v1 参考的枚举写成 `models/gemini-3.5-flash` 的形式，但 v1 自己的示例仍写裸 ID。
- **建议在 v1beta 一律用裸 ID。v1beta 是否接受 `models/` 前缀，【待确认】。**

**思考等级**（`generation_config.thinking_level`）。依据为思考指南的表格、模型页和图像指南：

| 模型 | 默认 | 支持的取值 | 备注 |
|---|---|---|---|
| `gemini-3.8-flash` | medium | low / medium / high | 模型页：`minimal` 不支持，会报错 |
| `gemini-3.7-flash` | medium | low / medium / high | 同上 |
| `gemini-3.6-flash` | medium | minimal / low / medium / high | — |
| `gemini-3.5-flash` | medium | minimal / low / medium / high | — |
| `gemini-3.5-flash-lite` | minimal | minimal / low / medium / high | — |
| `gemini-3.1-pro-preview` | high | low / medium / high | — |
| `gemini-3-flash-preview` | high | minimal / low / medium / high | — |
| `gemini-2.5-pro`、`gemini-2.5-flash` | 开启（On） | low / medium / high | — |
| `gemini-2.5-flash-lite` | 关闭（Off） | low / medium / high | — |
| `gemini-robotics-er-2-preview` | high | minimal / low / medium / high | — |
| `gemini-nano-banana-2.1` | medium | minimal / medium / high | 图像指南 |
| `gemini-3.1-flash-image` | minimal | minimal / high | 图像指南 |
| `gemini-3.1-flash-lite-image` | minimal | minimal / high | 图像指南 |
| `gemini-3-pro-image` | 始终开启，无法关闭 | **【待确认】**（图像指南只说 Gemini 3 图像模型的思考无法关闭，没写可选等级） | — |
| `gemini-3.1-flash-lite` | **【待确认】** | 不在思考表中；模型页说支持 Thinking，示例用了 `high` | — |
| TTS / Lyria / Transcribe | — | 模型页标为 Thinking Not supported | — |

补充说明：

- 思考指南的表格里还有 `gemini-3-pro-preview`，但模型页显示该模型已下线。
- 思考 token 计入 `max_output_tokens`，并在 `usage.total_thought_tokens` 中单独统计。
- `thinking_summaries: "auto"` 会在 `thought` step 的 `summary` 中返回摘要。

来源：

- https://ai.google.dev/api/interactions-api
- https://ai.google.dev/api/interactions-api-v1
- https://ai.google.dev/gemini-api/docs/interactions/thinking
- https://ai.google.dev/gemini-api/docs/models
- https://ai.google.dev/gemini-api/docs/image-generation

---

### 4.1 文本 / 多模态理解模型

| 模型 ID | 状态 | 输入 → 输出 | Token 上限（输入/输出） | 备注 |
|---|---|---|---|---|
| `gemini-3.8-flash` | Stable（2026-09） | 文本、图像、视频、音频、PDF → 文本 | 1,048,576 / 65,536 | 当前旗舰 Flash。不支持 thinking `minimal` |
| `gemini-3.7-flash` | Stable | 同上 | 同上 | 不支持 `minimal` |
| `gemini-3.6-flash` | Stable | 同上 | 同上 | — |
| `gemini-3.5-flash` | Stable | 同上 | 同上 | — |
| `gemini-3.5-flash-lite` | Stable（2026-07） | 同上 | 同上 | 不在参考枚举中 |
| `gemini-3.1-flash-lite` | Stable（2026-05） | 同上 | 同上 | 不支持 Computer Use |
| `gemini-3.1-pro-preview` / `-customtools` | Preview | 同上 | 同上 | File Search 仅 AI Studio 可用 |
| `gemini-3-flash-preview` | Preview | 同上 | 同上 | — |
| `gemini-2.5-pro` / `gemini-2.5-flash` / `gemini-2.5-flash-lite` | 旧版 | — | — | 本手册未逐一核对模型页 |
| `gemini-flash-latest` / `gemini-flash-lite-latest` / `gemini-pro-latest` | 别名 | — | — | 指向最新版本 |
| `gemma-4-26b-a4b-it`、`gemma-4-31b-it`、`gemini-robotics-er-*` | — | — | — | 只在参考枚举中出现，本手册未展开 |

**适用参数（3.x Flash 系列）：**

- `system_instruction`
- `generation_config`：`max_output_tokens`、`seed`、`stop_sequences`、`thinking_level`、`thinking_summaries`、`tool_choice`
- 结构化输出：`response_format: {type:"text", mime_type:"application/json", schema:{...}}`
- 所有工具：`function`、`google_search`、`code_execution`、`url_context`、`file_search`、`google_maps`、`computer_use`（Preview，3.5 及以上，3.1-flash-lite 不支持）、`mcp_server`
- `service_tier: flex | priority`：上表的 3.x 文本模型页均标为支持
- 输入 Content 的 `resolution`（媒体分辨率），以及 video 的 `processing`

**curl：**
```bash
curl -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.8-flash",
    "system_instruction": "用简体中文回答。",
    "input": "用三句话解释量子纠缠。",
    "generation_config": { "thinking_level": "low", "max_output_tokens": 2048 }
  }'
```

**JS：**
```js
import { GoogleGenAI } from "@google/genai";
const ai = new GoogleGenAI({}); // 读取环境变量 GEMINI_API_KEY

const interaction = await ai.interactions.create({
  model: "gemini-3.8-flash",
  system_instruction: "用简体中文回答。",
  input: "用三句话解释量子纠缠。",
  generation_config: { thinking_level: "low", max_output_tokens: 2048 },
});
console.log(interaction.output_text);

// 多轮（有状态）：只需传上一轮 id；system_instruction / tools / generation_config 需要重传
const next = await ai.interactions.create({
  model: "gemini-3.8-flash",
  previous_interaction_id: interaction.id,
  system_instruction: "用简体中文回答。",
  input: "再举一个生活中的类比。",
});
```

**结构化输出（JSON）：**
```js
const r = await ai.interactions.create({
  model: "gemini-3.8-flash",
  input: "列出 3 种水果及其颜色",
  response_format: {
    type: "text",
    mime_type: "application/json",
    schema: {
      type: "array",
      items: {
        type: "object",
        properties: { name: { type: "string" }, color: { type: "string" } },
        required: ["name", "color"],
      },
    },
  },
});
const data = JSON.parse(r.output_text);
```

**函数调用（JS 需手动循环；Python SDK 也不支持自动函数调用）：**
```js
const tools = [{ type: "function", name: "get_weather", description: "查询城市天气",
  parameters: { type: "object", properties: { city: { type: "string" } }, required: ["city"] } }];

const first = await ai.interactions.create({ model: "gemini-3.8-flash", input: "北京天气如何？", tools });
const call = first.steps.find(s => s.type === "function_call");
if (call) {
  const result = { temp_c: 21, sky: "晴" }; // 你自己执行函数
  const second = await ai.interactions.create({
    model: "gemini-3.8-flash",
    previous_interaction_id: first.id,
    tools, // 必须重传
    input: [{ type: "function_result", call_id: call.id, name: call.name, result }],
  });
  console.log(second.output_text);
}
```

来源：

- https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash （及其他各模型页）
- https://ai.google.dev/gemini-api/docs/text-generation
- https://ai.google.dev/gemini-api/docs/structured-output
- https://ai.google.dev/gemini-api/docs/function-calling
- https://ai.google.dev/gemini-api/docs/interactions/thinking

---

### 4.2 （补充）语音转写：`gemini-3.5-transcribe`

- 输入：音频，单次最长 1 小时；开启说话人分离或词级时间戳时最长 30 分钟。
- 输出：文本，以及 `word_info` 标注。
- 用 `generation_config.transcription_config` 配置：
  - `custom_vocabulary`：最多 1,000 条
  - `language_codes`
  - `mode`：`verbatim` / `smart`，或带 `diarization_mode` / `timestamp_granularities` 的对象
- 不支持思考、函数调用、Flex、Priority、Batch。
- 实时转写用 Live API 的 `gemini-3.5-transcribe-live`，不走 Interactions。

```bash
curl -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.5-transcribe",
    "input": [{ "type": "audio", "uri": "YOUR_FILE_URI", "mime_type": "audio/mp3" }],
    "generation_config": { "transcription_config": {
      "mode": { "type": "verbatim", "diarization_mode": "speaker", "timestamp_granularities": ["word"] } } }
  }'
```
```js
const file = await ai.files.upload({ file: "sample.mp3", config: { mimeType: "audio/mp3" } }); // Files API 本身用 camelCase
const r = await ai.interactions.create({
  model: "gemini-3.5-transcribe",
  input: [{ type: "audio", uri: file.uri, mime_type: file.mimeType }],
  generation_config: { transcription_config: { mode: "smart", language_codes: ["zh-CN"] } },
});
console.log(r.output_text);
```

注：官方转写示例中 Files API 写成 `config: { mime_type: "audio/mp3" }`。**这是错的。** SDK 的 `UploadFileConfig` 只有 camelCase 的 `mimeType`（本地 tsc 报错 `'mime_type' does not exist ... Did you mean 'mimeType'?`）。运行时 `mime_type` 会被忽略，SDK 改为按扩展名推断。见 §3.10.5。

来源：

- https://ai.google.dev/gemini-api/docs/transcribe
- https://ai.google.dev/gemini-api/docs/models/gemini-3.5-transcribe

---

### 4.3 图像生成（Nano Banana 系列）

| 模型 ID | 别名 | 输入 → 输出 | Token（输入/输出） | 尺寸 | 搜索接地 | 思考等级 |
|---|---|---|---|---|---|---|
| `gemini-nano-banana-2.1` | Nano Banana 2.1（Stable，2026-10） | 文本、图像、视频、PDF → 图像+文本 | 131,072 / 32,768 | 1K（默认）/ 2K / 4K | 支持（web + image search） | minimal / **medium（默认）** / high |
| `gemini-3.1-flash-image` | Nano Banana 2 | 文本、图像、视频、PDF → 图像+文本 | 131,072 / 32,768 | 512 / 1K / 2K / 4K | 支持 | minimal（默认）/ high |
| `gemini-3.1-flash-lite-image` | Nano Banana 2 Lite | 文本、图像、视频、PDF → 图像+文本 | 65,536 / 4,096 | **只支持 1K** | **不支持** | minimal（默认）/ high |
| `gemini-3-pro-image` | Nano Banana Pro | 图像、文本 → 图像+文本 | 65,536 / 32,768 | 1K / 2K / 4K | 支持 | 始终开启（等级 **【待确认】**） |
| `nano-banana-pro-preview` | — | — | — | — | — | 只在参考枚举中出现，**【待确认】** |
| `gemini-2.5-flash-image` | Nano Banana | — | — | 只有单一分辨率（如 1:1 = 1024x1024） | — | — |

**这一系列的共同规则：**

- 不支持：函数调用、结构化输出、URL context、Code execution、Maps。
- Flex / Priority 均为 **Not supported**，所以不要设置 `service_tier: "flex"` 或 `"priority"`。
- **默认同时返回文字和图片。** 只想要图片时，设置 `response_format: {type:"image", ...}`。两者都要时传数组 `[{type:"text"},{type:"image"}]`。
- **`aspect_ratio`：**
  - Nano Banana 2.1 和 3.1 Flash Image 支持全部 14 种比例：`1:1`、`1:4`、`1:8`、`2:3`、`3:2`、`3:4`、`4:1`、`4:3`、`4:5`、`5:4`、`8:1`、`9:16`、`16:9`、`21:9`。
  - 图像指南里标题为 "3.1 Pro Image" 的表格只列了 10 种，没有 1:4、4:1、1:8、8:1。（这个标题疑似指 `gemini-3-pro-image`，**【待确认】**）
  - 3.1-flash-lite-image 支持哪些比例，文档没写 **【待确认】**。
  - 不指定时：有输入图则匹配输入图尺寸，否则输出 1:1。
- **`image_size`：**
  - 必须用大写 K，例如 `"4K"`；小写 `"4k"` 会被拒绝。
  - `"512"` 只有 3.1 Flash Image 支持。
  - Nano Banana 2.1 在 4K、1:1 时输出 4096x4096，消耗 2520 输出 token；16:9 为 5504x3072。
- **思考：** Gemini 3 系列图像模型默认开启思考且无法在 API 中关闭。思考过程最多产生两张中间图（放在 `thought.summary` 里），最后一张即最终图。思考 token 默认计费。
- **参考图**（依据图像指南的"Use up to 14 reference images"表）：
  - 3.1 Flash Lite Image：14 张物体图；
  - 2.1 / 3.1 Flash Image：10 张物体 + 4 张角色；
  - 3 Pro Image：6 张物体 + 5 张角色 + 3 张风格参考。
  - 同页 Limitations 的口径是"3-pro-image 5 张高保真、共 14 张；2.5-flash-image 建议 ≤3 张"。
- **视频输入：** 只有 2.1、3.1 Flash、3.1 Flash Lite 支持。所有图像模型都不支持音频输入。
- **搜索接地：**
  - 用 `tools: [{type:"google_search", search_types:["web_search","image_search"]}]` 开启。
  - 用了 Image Search 时，**必须展示** `google_search_result` step 里的 `search_suggestions`。
  - 2.1 / 3.1 Flash 的搜索接地目前**不会使用网络上真实人物的图片**。
- 所有生成的图片都带 SynthID 水印。

#### 常见代码问题

```js
// 原始代码（有问题）
ai.interactions.create({
  model: 'models/gemini-nano-banana-2.1',
  input,
  tools: [{ type: 'google_search' }],
  generation_config: { max_output_tokens, thinkingLevel, imageConfig: { imageSize: '4K' } },
  response_modalities: ['image'],
});
```

| 位置 | 问题 | 改法 |
|---|---|---|
| `model: 'models/gemini-nano-banana-2.1'` | 文档中所有 v1beta 示例都用裸 ID；带前缀能否用 **【待确认】** | `model: 'gemini-nano-banana-2.1'` |
| `thinkingLevel` | SDK 不做 camelCase 转换，这个键会原样发给服务端，**不是有效字段** | `thinking_level`，取值 `'minimal' \| 'medium' \| 'high'`（2.1 不支持 `low`） |
| `imageConfig: { imageSize }` | camelCase，而且 `image_config` 本身已在 2026-05 迁移到 `response_format` | `response_format: { type: 'image', image_size: '4K' }` |
| `response_modalities: ['image']` | 已被 `response_format` 取代，SDK 标为 deprecated | 由上面的 `response_format` 一并表达。想要文字加图片时传数组 |
| `tools: [{ type: 'google_search' }]` | 写法有效。不写 `search_types` 时的默认行为文档没有明确说明 **【待确认】**；想用图片搜索需要显式指定 | 可选：`search_types: ['web_search', 'image_search']` |
| `max_output_tokens` | 有效，snake_case 正确。注意它包含思考 token，图像 token 也计入输出（4K 约 2520/张），设得太小会导致 `incomplete` | 保留，或不设置 |

#### 正确写法（JS）

```js
import { GoogleGenAI } from "@google/genai";
import * as fs from "node:fs";

const ai = new GoogleGenAI({ apiKey: process.env.GEMINI_API_KEY });

const interaction = await ai.interactions.create({
  model: "gemini-nano-banana-2.1",
  input, // string，或 [{type:'text',text:'...'},{type:'image',data:b64,mime_type:'image/png'}]
  tools: [{ type: "google_search", search_types: ["web_search", "image_search"] }],
  generation_config: {
    thinking_level: "high",          // 2.1：minimal | medium(默认) | high
    max_output_tokens: 32768,        // 可选；模型输出上限为 32,768
  },
  response_format: {                 // 只要图片；要图文并茂改成数组 [{type:'text'},{type:'image', image_size:'4K'}]
    type: "image",
    image_size: "4K",                // 必须大写 K
    aspect_ratio: "16:9",            // 可选
  },
});

if (interaction.status !== "completed") console.warn(interaction.status, interaction.errors);
const img = interaction.output_image; // SDK 便捷属性：最后一个 image 块 { type, data(base64), mime_type }
if (img?.data) fs.writeFileSync("out.png", Buffer.from(img.data, "base64")); // 扩展名请按 img.mime_type 决定

// 用了 image_search 时，按要求展示搜索建议：
const gs = interaction.steps.find(s => s.type === "google_search_result");
console.log(gs?.result?.map(r => r.search_suggestions));
```

#### 等价的 curl

```bash
curl -s -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-nano-banana-2.1",
    "input": "一张赛博朋克风格的上海外滩夜景海报",
    "tools": [{"type": "google_search", "search_types": ["web_search", "image_search"]}],
    "generation_config": {"thinking_level": "high"},
    "response_format": {"type": "image", "image_size": "4K", "aspect_ratio": "16:9"}
  }' | jq -r '.steps[] | select(.type=="model_output") | .content[] | select(.type=="image") | .data' | head -n1 | base64 -d > out.img
```

#### 图像编辑（图生图）与多张参考图：输入写法详解

- **原图怎么提交**：
  - 放一个 `{type:"image", data:<标准 base64>, mime_type:"image/png"|"image/jpeg"|...}` 块。官方编辑示例就是这样写的。
  - 也可以用 Files API 的 `uri`，或公开 https URL。这两种是通用规则，Nano Banana 指南里没有示例 ◐ **【待确认】**。
  - **不接受**裸二进制或 multipart。详见 §3.10。
- **多张参考图**：
  - 在 `input` 数组里依次放多个 `image` 块，再加一个 `text` 块写指令。
  - **没有** `role`、`name`、`index` 之类的字段用来标记参考图用途（`image` 块的字段只有 `data`、`uri`、`mime_type`、`resolution`），**只能在文字里按顺序描述**，例如"第一张是人物，第二张是服装"。
  - 图像指南的"组合多图"示例就是这种写法。Omni 有 `<IMAGE_REF_N>` 标签机制，但**图像指南没有为 Nano Banana 说明这个机制**，它在 Nano Banana 上是否生效 **【待确认】**。
- **数量上限**：
  - 3.1 Flash Lite Image：14 张物体图；
  - 2.1 / 3.1 Flash Image：10 张物体 + 4 张角色；
  - 3 Pro Image：6 张物体 + 5 张角色 + 3 张风格参考；
  - 详见 §3.10.3。超出上限的行为文档没写。
- **大小**：内联时注意总请求的上限（20MB 还是 100MB，文档有矛盾，见 §3.10.3）。多张大图建议先用 Files API 上传。
- **视频作参考**（只限 2.1、3.1 Flash、3.1 Flash Lite）：用 `{type:"video", uri:"https://www.youtube.com/watch?v=..."}`，或 Files API 上传后的 `uri`。
- **多轮编辑**：
  - 第二轮及以后只需传 `previous_interaction_id` 和新的文字指令，不必重传图片。
  - `response_format` 和 `tools` 每轮都要重传。

```bash
# 两张参考图（内联 base64）+ 指令
P=$(base64 -w0 person.jpg); D=$(base64 -w0 dress.png)
cat > payload.json <<JSON
{"model":"gemini-nano-banana-2.1",
 "input":[
  {"type":"image","mime_type":"image/jpeg","data":"$P"},
  {"type":"image","mime_type":"image/png","data":"$D"},
  {"type":"text","text":"让第一张图中的人物穿上第二张图里的裙子，保持人物面部和背景不变"}],
 "response_format":{"type":"image","image_size":"2K","aspect_ratio":"3:4"}}
JSON
curl -s -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" -d @payload.json \
 | jq -r '.steps[] | select(.type=="model_output") | .content[] | select(.type=="image") | .data' | tail -n1 | base64 -d > edited.jpg
```
```js
const b64 = p => fs.readFileSync(p).toString("base64");
const first = await ai.interactions.create({
  model: "gemini-nano-banana-2.1",
  input: [
    { type: "image", mime_type: "image/jpeg", data: b64("person.jpg") },
    { type: "image", mime_type: "image/png", data: b64("dress.png") },
    { type: "text", text: "让第一张图中的人物穿上第二张图里的裙子，保持人物面部和背景不变" },
  ],
  response_format: { type: "image", image_size: "2K", aspect_ratio: "3:4" },
});
fs.writeFileSync("edited.jpg", Buffer.from(first.output_image.data, "base64"));

// 第二轮：基于上一轮结果继续改，无需重传图片
const second = await ai.interactions.create({
  model: "gemini-nano-banana-2.1",
  previous_interaction_id: first.id,
  input: "把背景换成黄昏的海边，其他不变",
  response_format: { type: "image", image_size: "2K", aspect_ratio: "3:4" },
});
```

来源：

- https://ai.google.dev/gemini-api/docs/image-generation （Image editing、Use up to 14 reference images、Multi-turn image editing、Video-to-image generation）
- https://ai.google.dev/gemini-api/docs/image-understanding


来源：

- https://ai.google.dev/gemini-api/docs/image-generation
- https://ai.google.dev/gemini-api/docs/models/gemini-nano-banana-2.1
- https://ai.google.dev/gemini-api/docs/models/gemini-3.1-flash-image
- https://ai.google.dev/gemini-api/docs/models/gemini-3.1-flash-lite-image
- https://ai.google.dev/gemini-api/docs/models/gemini-3-pro-image
- https://ai.google.dev/gemini-api/docs/models

---

### 4.4 语音合成（TTS）

| 模型 ID | 说明 | 输入 → 输出 | Token（输入/输出） |
|---|---|---|---|
| `gemini-3.8-flash-tts` | 旗舰 TTS，130 种语言 | 文本 → 音频 | 8,192 / 16,384 |
| `gemini-3.8-flash-lite-tts` | 低成本、高吞吐，101 种语言 | 文本 → 音频 | 本手册未核对 |
| `gemini-3.1-flash-tts-preview` | 旧版预览，建议迁移到 3.8 | 文本 → 音频 | — |

（模型页上的 `gemini-2.5-flash-preview-tts`、`gemini-2.5-pro-preview-tts` 不在 Interactions 参考枚举里，**【待确认】**能否用于 Interactions。）

**能力与限制：**

- 不支持思考、工具、结构化输出。
- 3.8 两个 TTS 模型的模型页标为支持 Flex 和 Priority。
- SDK 最低版本：JS ≥ 2.24.0，Python ≥ 2.25.0。

**必需参数：**

- `response_format: {type:"audio"}`。可选 `mime_type`、`sample_rate`、`bit_rate`、`delivery`。
- 输出格式默认：单次请求为 `audio/wav`，流式为 l16（24kHz 单声道 PCM）。

**声音配置**（`generation_config.speech_config`）：

- 单人：`[{ "voice": "Kore" }]`
- 多人（最多 2 人）：`{ "speakers": [{ "speaker":"Joe", "voice":"Puck" }, { "speaker":"Jane", "voice":"Kore" }] }`。官方 REST 示例里多出一个 `"mode":"conversational"`，参考文档没有这个字段 **【待确认】**。
- 声音来源：
  - 30 个预置声音，如 Zephyr、Puck、Kore、Charon 等；
  - 扩展声音库（`GET /v1beta/voices`）；
  - Voice design 或 Voice replication 得到的 `voice_...` ID，也可以用无状态的 `voicekey_...`。

**输入写法：**

- 用 `user_input` step，在 text 块里放**逐字稿**。模型会把 `text` 当作要念的原文，不会把其中的指令当成指令。
- 整段的风格（情绪、语速、音量等）写在 `annotations: [{type:"speech_metadata", style:"...", speaker:"..."}]` 里。
- 瞬时事件用尖括号标签插在逐字稿里，例如 `<short pause>`、`<sigh>`、`<laugh>`、`<cough>`。

```bash
curl -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.8-flash-tts",
    "input": [{ "type": "user_input", "content": [{
      "type": "text", "text": "祝你今天过得愉快！",
      "annotations": [{ "type": "speech_metadata", "style": "cheerful and friendly" }] }] }],
    "response_format": { "type": "audio" },
    "generation_config": { "speech_config": [ { "voice": "Kore" } ] }
  }' | jq -r '.steps[] | select(.type=="model_output") | .content[] | select(.type=="audio") | .data' | base64 -d > out.wav
```
```js
const r = await ai.interactions.create({
  model: "gemini-3.8-flash-tts",
  input: [{ type: "user_input", content: [{ type: "text", text: "祝你今天过得愉快！",
    annotations: [{ type: "speech_metadata", style: "cheerful and friendly" }] }] }],
  response_format: { type: "audio" },              // 默认 audio/wav；可加 mime_type:'audio/mp3', sample_rate:24000
  generation_config: { speech_config: [{ voice: "Kore" }] },
});
fs.writeFileSync("out.wav", Buffer.from(r.output_audio.data, "base64"));
```

来源：

- https://ai.google.dev/gemini-api/docs/speech-generation
- https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-tts
- https://ai.google.dev/gemini-api/docs/voice-design
- https://ai.google.dev/gemini-api/docs/models

---

### 4.5 音乐生成（Lyria）

| 模型 ID | 用途 | 时长 | 输出 |
|---|---|---|---|
| `lyria-3-clip-preview` | 短片段、循环、预览 | 固定 30 秒 | MP3 和文本（歌词/结构） |
| `lyria-3.5` | 完整歌曲（主歌、副歌、桥段） | 几分钟，可通过提示词控制 | MP3（默认），可选 WAV；44.1kHz 立体声 |
| `lyria-3-pro-preview` | 上一代完整歌曲模型 | — | — |

- 输入：文本和图像（最多 10 张图）。`lyria-3.5` 输入上限 131,072 token。**官方文档没有"视频输入生成音乐"的能力**，见 §3.10.9。
- 只能单轮生成，不支持多轮迭代编辑。
- 不支持思考、工具、Flex、Priority、Batch。
- 生成的音频都带 SynthID 音频水印。
- **如何选择 WAV：** 官方示例只写了 `response_format: {type:"audio"}`，没有给出具体的 `mime_type` 值。按参考枚举推测应为 `audio/wav`，**【待确认】**。
- 响应可能是歌词文本块和音频块交错排列，复杂情况下要遍历 `steps`，不能只靠便捷属性。

```bash
curl -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" \
  -d '{ "model": "lyria-3-clip-preview", "input": "A short instrumental acoustic guitar piece." }'
```
```js
const r = await ai.interactions.create({ model: "lyria-3.5", input: "一段优美的钢琴旋律" });
if (r.output_audio) fs.writeFileSync("music.mp3", Buffer.from(r.output_audio.data, "base64"));
console.log(r.output_text); // 歌词 / 结构
```

来源：

- https://ai.google.dev/gemini-api/docs/music-generation
- https://ai.google.dev/gemini-api/docs/models/lyria-3.5
- https://ai.google.dev/gemini-api/docs/models/lyria-3-clip-preview
- https://ai.google.dev/gemini-api/docs/models/lyria-3-pro-preview

---

### 4.6 视频生成（Gemini Omni Flash）

| 模型 ID | 状态 | 输入 → 输出 |
|---|---|---|
| `gemini-omni-1.1-flash` | Stable | 文本、图像、视频 → 视频（3–10 秒，24 FPS，自带音频） |
| `gemini-omni-flash-preview` | Preview | 同上 |

- 上下文窗口 1,048,576 token。
- 用于编辑或延长的输入视频不能超过 10 秒。由模型生成的视频在多轮对话中继续延长，不受此限。
- 视频参考：最多 3 段，每段不超过 3 秒，参考视频中的音频会被忽略。

**参数：**

- `response_format: {type:"video", aspect_ratio:"16:9"(默认)|"9:16", resolution:"360p"|"720p"(默认)|"1080p"|"4k", duration, delivery:"inline"|"uri"}`
  - 1080p 和 4k 是放大（upscaled）输出。
  - 视频超过 4MB 时建议用 `delivery:"uri"`，再通过 Files API 轮询文件状态到 `ACTIVE` 后下载。
  - 已知问题：之后用 GET 取回交互时，视频总是以 inline base64 返回，`uri` 字段只保证出现在创建响应或 SSE 中。
- `generation_config.video_config.task`：`text_to_video`、`image_to_video`、`reference_to_video`、`edit`、`extend`。不填时自动判断。
- 用 `previous_interaction_id` 做多轮对话式编辑。

**不支持：**

- `system_instruction`、`temperature`、`top_p`、stop sequences、负面提示（可以把要避免的内容直接写进提示词）
- 多视频推理、语音编辑
- 视频延长只能往结尾追加
- 在 EEA、瑞士、英国，不能上传或编辑含未成年人的图像

**文档中的不一致：** Omni 指南有一个 JS 示例写的是 camelCase 的 `generationConfig: { videoConfig: { task } }`，而同页的 REST 示例是 `generation_config.video_config`。由于 SDK 不做键名转换，**请使用 snake_case**。

```bash
curl -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-omni-1.1-flash",
    "input": "无人机航拍日出时的雪山，镜头缓慢推进",
    "response_format": { "type": "video", "aspect_ratio": "16:9", "resolution": "1080p" }
  }'
```
```js
const r = await ai.interactions.create({
  model: "gemini-omni-1.1-flash",
  input: [
    { type: "image", data: base64Image, mime_type: "image/jpeg" },
    { type: "text", text: "让画面中的鱼跃出水面，写实风格" },
  ],
  generation_config: { video_config: { task: "image_to_video" } },
  response_format: { type: "video", aspect_ratio: "9:16" },
});
if (r.output_video?.data) fs.writeFileSync("out.mp4", Buffer.from(r.output_video.data, "base64"));
```

#### 图生视频 / 首尾帧 / 参考图 / 编辑与延长：输入写法详解

- **图像怎么提交**：
  - 官方示例用内联 base64：`{type:"image", data, mime_type:"image/jpeg"}`。
  - 也可以用 Files API 的 `uri`：官方"Extending with reference media"示例里有 `{type:"image", uri: characterImg.uri}`，没带 `mime_type`。
  - 不接受 multipart 或裸二进制，也**不支持 YouTube**。
- **首帧 / 尾帧怎么指定**：
  - 不存在 `first_frame`、`last_frame` 之类的字段。
  - 按官方说法，"在 `input` 列表中提供两张图"，**第一张作首帧，第二张作尾帧**。`video_config.task:"image_to_video"` 的参考说明也写着"第一张为起始帧，可选的第二张为结束帧"。
  - 要更明确地指定角色，可以在提示词里写 **标签**：
    - `<FIRST_FRAME>`、`<LAST_FRAME>`。`<LAST_FRAME>` 必须与 `<FIRST_FRAME>` 一起用。
    - 声明式写法：`[# Sources <FIRST_FRAME>@Image1 <LAST_FRAME>@Image2]`。`@Image1`、`@Image2` 按 `input` 中图像块的出现顺序编号。
    - `<FIRST_FRAME>@Image1 <LAST_FRAME>@Image1` 表示首尾同帧，生成循环视频。
- **参考图（主体 / 风格参考）**：
  - 提供多张图，并在提示词中用 `<IMAGE_REF_0>`、`<IMAGE_REF_1>`… 引用，编号从 0 开始；也可以用 `[# References <IMAGE_REF_0>@Image1]` 声明。
  - 官方提示示例用到了 6 张参考图。参考图张数上限文档没写 **【待确认】**。
  - 可以设置 `generation_config.video_config.task:"reference_to_video"`。官方建议优先靠提示词，因为设置 task 会增加严格约束。
  - 视频参考：`<VIDEO_REF_N>`，最多 3 段，每段 ≤3 秒，参考视频里的音频会被忽略。
  - **不支持音频参考。**
- **编辑或延长你自己的视频**：
  - 推荐用 Files API 上传：`ai.files.upload` → 轮询到 `ACTIVE` → `{type:"video", uri: file.uri}`。
  - 也可以内联 `{type:"video", mime_type:"video/mp4", data}`。官方说可以，但视频通常较大，推荐 Files API。
  - 上传的视频必须 **≤10 秒**。
  - EEA、瑞士、英国不能编辑或延长**上传的**视频。
  - 延长只能往结尾追加，生成 3–10 秒的续段。
  - 上传的视频里有人在说话时，延长不能再加对白。
  - 延长**模型生成的**视频请用 `previous_interaction_id`：不受 10 秒限制，可以生成对白，所有地区可用。
  - 延长时可以再加一张参考图，例如 `<IMAGE_REF_0>` 引入新角色。
- **输出**：
  - 默认内联 base64，`mime_type: video/mp4`。
  - 超过 4MB 时用 `response_format:{type:"video", delivery:"uri"}`，再按 §3.10.11 轮询 Files API 并用 `:download?alt=media` 下载（需要 API key）。

```bash
# 首尾帧插值（内联 base64）
F=$(base64 -w0 first.jpg); L=$(base64 -w0 last.jpg)
cat > payload.json <<JSON
{"model":"gemini-omni-1.1-flash",
 "input":[
  {"type":"image","data":"$F","mime_type":"image/jpeg"},
  {"type":"image","data":"$L","mime_type":"image/jpeg"},
  {"type":"text","text":"[# Sources <FIRST_FRAME>@Image1 <LAST_FRAME>@Image2] 从清晨的绿色森林平滑过渡到星空下的雪林。Use Image1 as the starting frame and Image2 as the final frame."}],
 "generation_config":{"video_config":{"task":"image_to_video"}},
 "response_format":{"type":"video","aspect_ratio":"16:9","resolution":"720p"}}
JSON
curl -s -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" -d @payload.json \
 | jq -r '.steps[] | select(.type=="model_output") | .content[] | select(.type=="video") | .data' | head -n1 | base64 -d > interp.mp4
```
```js
// 编辑自己的视频：Files API 上传 → 等待 ACTIVE → 引用（再加一张参考图）
let vf = await ai.files.upload({ file: "my_clip.mp4", config: { mimeType: "video/mp4" } });   // ≤10 秒
let img = await ai.files.upload({ file: "character.png", config: { mimeType: "image/png" } });
while (vf.state === "PROCESSING" || img.state === "PROCESSING") {
  await new Promise(r => setTimeout(r, 10000));
  vf = await ai.files.get({ name: vf.name }); img = await ai.files.get({ name: img.name });
}
const r = await ai.interactions.create({
  model: "gemini-omni-1.1-flash",
  input: [
    { type: "video", uri: vf.uri, mime_type: vf.mimeType },
    { type: "image", uri: img.uri, mime_type: img.mimeType },
    { type: "text", text: "Extend this video: have the character shown in <IMAGE_REF_0> enter the scene and wave." },
  ],
  response_format: { type: "video", delivery: "uri" },
});
// 下载见 §3.10.11
```

来源：

- https://ai.google.dev/gemini-api/docs/omni （Image to video、First and last frame interpolation、Subject reference、Tasks parameter、Edit your own videos、Retrieving videos with an URI、Video extension、Limitations、Using tags in prompts）
- https://ai.google.dev/api/interactions-api （VideoConfig.task）

（模型页还列出了 Veo 3.1 / Veo 3.1 Lite，但它们不在 Interactions 参考的 `model` 枚举中。能否通过 Interactions 调用 **【待确认】**，本手册不覆盖。）

来源：

- https://ai.google.dev/gemini-api/docs/omni
- https://ai.google.dev/gemini-api/docs/models/gemini-omni-flash

---

### 4.7 Deep Research Agent

| agent ID | 说明 |
|---|---|
| `deep-research-preview-04-2026` | 速度和效率优先，适合流式推送给前端 |
| `deep-research-max-preview-04-2026` | 追求最全面的资料收集和综合 |
| `deep-research-pro-preview-12-2025` | 只在参考枚举中出现（旧版），**【待确认】** |

**规则：**

- 必须设置 `background: true`，并且需要存储（不能用 `store: false`）。
- 最长运行 60 分钟。
- 通过 GET 轮询 `status`；或在创建时设置 `stream: true` + `background: true`，断线后用 `GET ?stream=true&last_event_id=` 恢复。

**`agent_config`：**
```
{ type: "deep-research", thinking_summaries: "auto"|"none"(默认), visualization: "auto"(默认)|"off", collaborative_planning: false(默认) }
```

**工具：**

- 默认带 `google_search`、`url_context`、`code_execution`。
- 可以追加 `mcp_server`、`file_search`。
- 不支持自定义函数调用和结构化输出。

```bash
curl -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" \
  -d '{ "agent": "deep-research-preview-04-2026", "input": "研究 Google TPU 的发展史。", "background": true,
        "agent_config": { "type": "deep-research", "thinking_summaries": "auto" } }'
# 然后轮询：GET /v1beta/interactions/{id}
```
```js
const job = await ai.interactions.create({
  agent: "deep-research-preview-04-2026",
  input: "研究 Google TPU 的发展史。",
  background: true,
});
while (true) {
  const r = await ai.interactions.get(job.id);
  if (r.status === "completed") { console.log(r.output_text); break; }
  if (["failed", "cancelled", "incomplete"].includes(r.status)) { console.log(r.status, r.errors); break; }
  await new Promise(res => setTimeout(res, 10_000));
}
```

来源：

- https://ai.google.dev/gemini-api/docs/deep-research
- https://ai.google.dev/gemini-api/docs/background-execution
- https://ai.google.dev/gemini-api/docs/models/deep-research-preview-04-2026

---

### 4.8 Antigravity Agent

| agent ID | 出处 |
|---|---|
| `antigravity-preview-09-2026` | Antigravity 指南、模型页、概览页的示例都用这个 ID |
| `antigravity-preview-05-2026` | API 参考的 `agent` 枚举里**只有**这个 ID |

两处不一致，**【待确认】**。建议使用指南里的 `09-2026`。

- 这是一个通用的托管 agent，运行在 Google 托管的 Linux 沙箱中，可以执行 Bash / Python / Node、管理文件、浏览网页。
- `environment` 写 `"remote"`、填环境 ID，或传 EnvironmentConfig（见 §3.7）。
- `agent_config: { type: "antigravity", model: "gemini-3.8-flash"(默认) | "gemini-3.7-flash" | "gemini-3.6-flash" | "gemini-3.5-flash" | "gemini-3.5-flash-lite" }`
- 设置 `temperature`、`top_p`、`top_k`、`stop_sequences`、`max_output_tokens` 会返回 **400**。
- 只接受文本和图像输入，不接受音频、视频、文档。
- 可以接远程 MCP，但只支持 Streamable HTTP；server `name` 必须全小写字母和数字，出现大写会得到 400。
- 任务耗时较长。官方 JS 示例给请求设了 `{ timeout: 300000 }`，即 5 分钟。也可以用 `background: true`。

```bash
curl -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" \
  -d '{ "agent": "antigravity-preview-09-2026", "environment": "remote",
        "input": "读取 Hacker News，总结前 10 条并保存为 PDF。" }'
```
```js
const r = await ai.interactions.create({
  agent: "antigravity-preview-09-2026",
  environment: "remote",
  input: "读取 Hacker News，总结前 10 条并保存为 PDF。",
  agent_config: { type: "antigravity", model: "gemini-3.8-flash" },
}, { timeout: 300000 });
console.log(r.output_text, r.environment_id); // 后续可用 environment: r.environment_id 复用沙箱
```

**其他 agent：**

- OpenAPI 中还有 `code-mender` 类型的 agent_config（CodeMender 安全扫描与修复）和 `dynamic` 类型，前者对应的 agent ID 没有列出。
- 可以通过 `/v1beta/agents` 创建自定义 agent。
- 这些都超出本手册范围，**【待确认】**。

来源：

- https://ai.google.dev/gemini-api/docs/antigravity-agent
- https://ai.google.dev/gemini-api/docs/models/antigravity-preview-09-2026
- https://ai.google.dev/api/interactions-api

---
## 5. 响应结构

### 5.1 Interaction 对象

POST 创建和 GET 获取返回的都是 Interaction 对象。

| 字段 | 说明 |
|---|---|
| `id` | 交互 ID。示例中形如 `v1_...`，后台模式下立即返回 |
| `status` | `in_progress`、`requires_action`（等你回传 function_result）、`completed`、`failed`、`cancelled`、`incomplete`（例如达到 `max_output_tokens`，此时可能附带 `continuation_token`）、`queued`、`budget_exceeded`（已废弃） |
| `steps[]` | 时间线。**POST 只返回本轮的输出步骤；GET 返回完整时间线，包括开头的 `user_input` step** |
| `usage` | token 用量，见 §5.5 |
| `created` / `updated` | ISO 8601 UTC 时间（`YYYY-MM-DDThh:mm:ssZ`）。SDK 会把它们规范化为日期对象 |
| `model` / `agent`、`service_tier`、`previous_interaction_id`、`environment_id`、`continuation_token`、`errors[]{code, message}` | 回显字段和诊断信息 |
| `object: "interaction"` | 出现在示例 JSON 中；参考的字段列表里没有 |

GET 加上 `include_input=true` 时，响应会包含 `input`。

### 5.2 便捷属性（SDK 专有，REST 中没有）

依据 `@google/genai@2.27.0` 的源码：

| 属性 | 取值逻辑 |
|---|---|
| `output_text` | 从最后一个 `user_input` 之后的 `model_output` steps 中，**由后往前取连续的一段 text 块，拼接起来**。也就是"最后一次模型文本输出" |
| `output_image` / `output_audio` / `output_video` | 同一范围内**最后一个**对应类型的 Content 块，形如 `{type, data(base64) 或 uri, mime_type}` |

官方说明：遇到交错的复杂输出（例如图文混排的教程、歌词和音频交错），便捷属性取不全，**需要自己遍历 `steps`**。

### 5.3 Step 类型（响应中可能出现的）

| `type` | 主要字段 |
|---|---|
| `user_input` | `content[]` |
| `model_output` | `content[]`（text / image / audio / video / document 块；text 可带 `annotations`）；`error`（已废弃） |
| `thought` | `signature`（回放时必须原样传回）、`summary[]`（text / image 块；开启 `thinking_summaries:"auto"` 才有文字摘要，图像模型会在这里放中间图） |
| `function_call` | `id`、`name`、`arguments` |
| `function_result` | `call_id`、`name`、`result`、`is_error` |
| `google_search_call` / `google_search_result` | call：`id`、`arguments`、`search_type`、`signature`；result：`call_id`、`result[]{search_suggestions}`、`is_error`、`signature` |
| `code_execution_call` / `code_execution_result` | call：`id`、`arguments`、`signature`；result：`call_id`、`result`、`is_error`、`signature` |
| `url_context_call` / `url_context_result` | 同上结构 |
| `google_maps_call` / `google_maps_result` | 同上结构，result 没有 `is_error` |
| `file_search_call` / `file_search_result` | `id` / `call_id`、`signature` |
| `mcp_server_tool_call` / `mcp_server_tool_result` | `id` / `call_id`、`name`、`server_name`、`arguments` / `result` |
| `retrieval_call` / `retrieval_result` | `id`、`arguments`、`retrieval_type` / `call_id`、`is_error` |
| `processing_call` / `processing_result` | `id` / `call_id`、`signature`。文档没有用途说明 **【待确认】** |

另外两点：

- Omni 指南的示例 JSON 里，`thought` step 写成了 `{"type":"thought","content":[{"type":"thought","text":"..."}]}`，与参考中的 `summary` 结构不一致。请以参考为准，**【待确认】**。
- 迁移指南的部分 REST 示例里出现了 step 级的 `status: "done"/"waiting"`，参考中没有这个字段 **【待确认】**。

### 5.4 提取各类输出（REST / JS 通用写法）

```js
const outs = interaction.steps.filter(s => s.type === "model_output").flatMap(s => s.content ?? []);

// 文本（含引用标注）
const text = outs.filter(c => c.type === "text").map(c => c.text).join("");
const citations = outs.flatMap(c => c.annotations ?? []).filter(a => a.type === "url_citation");

// 图片 / 音频 / 视频：按实际 MIME 保存，URI 交付时读取 uri
const extensions = { "image/png": "png", "image/jpeg": "jpg", "image/webp": "webp",
  "audio/wav": "wav", "audio/mp3": "mp3", "audio/l16": "pcm", "video/mp4": "mp4" };
let mediaIndex = 0;
for (const c of outs) {
  if (!["image", "audio", "video"].includes(c.type)) continue;
  if (c.data) {
    const ext = extensions[c.mime_type] ?? "bin";
    fs.writeFileSync(`media-${mediaIndex++}.${ext}`, Buffer.from(c.data, "base64"));
  } else if (c.uri) {
    console.log("媒体 URI：", c.uri); // 按对应 Files API / 交付指南下载
  }
}

// 函数调用
const calls = interaction.steps.filter(s => s.type === "function_call"); // status 为 requires_action

// 思考摘要
const thoughts = interaction.steps.filter(s => s.type === "thought").flatMap(s => s.summary ?? []);
```

```bash
# jq：取全部文本
jq -r '[.steps[] | select(.type=="model_output") | .content[] | select(.type=="text") | .text] | join("")'
# jq：取第一张图片并解码
jq -r '.steps[] | select(.type=="model_output") | .content[] | select(.type=="image") | .data' | head -n1 | base64 -d > out.img
```

注意：流式 TTS 返回的是裸 PCM（l16，24kHz 单声道），需要自己加 WAV 头才能直接播放。单次请求默认返回 `audio/wav`。

### 5.5 `usage` 字段

| 字段 | 说明 |
|---|---|
| `total_input_tokens` | 提示（上下文）token |
| `total_output_tokens` | 所有生成内容的 token |
| `total_thought_tokens` | 思考 token |
| `total_cached_tokens` | 缓存命中的 token |
| `total_tool_use_tokens` | 工具调用提示中的 token |
| `total_tokens` | 总计（提示 + 响应 + 其他内部 token） |
| `input_tokens_by_modality[]`、`output_tokens_by_modality[]`、`cached_tokens_by_modality[]`、`tool_use_tokens_by_modality[]` | 按模态细分：`{ modality: text\|image\|audio\|video\|document, tokens }` |
| `grounding_tool_count[]` | `{ type: google_search\|google_maps\|retrieval, count }`，接地工具的调用次数，用于计费 |

### 5.6 流式（SSE）

- 创建时设 `stream: true`，SDK 返回一个异步迭代器。REST 端会收到 `text/event-stream`。
- 也可以对已有交互调用 `GET /interactions/{id}?stream=true`。
- 每个事件都带 `event_type` 和 `event_id`。断线后用 `last_event_id` 恢复。流的最后会有 `event: done` / `data: [DONE]`。

| `event_type` | 负载 |
|---|---|
| `interaction.created` | `interaction`（id、status、model 等） |
| `interaction.status_update` | `interaction_id`、`status` |
| `step.start` | `index`、`step`（给出 step 的 type；函数调用时这里带函数名） |
| `step.delta` | `index`、`delta`（见下） |
| `step.stop` | `index`、`usage`、`step_usage` |
| `interaction.completed` | 最终的 `interaction`，含 `usage` |
| `error` | 错误信息 |

`delta.type` 的可能取值：

- 内容类：`text`、`image`、`audio`、`video`、`document`
- 思考类：`thought_summary`、`thought_signature`
- 函数参数增量：`arguments_delta`
- 引用标注：`text_annotation_delta`
- 各类工具的 call / result：`google_search_call`、`code_execution_result` 等

**矛盾：** 破坏性变更指南称 `interaction.status_update` 已被 `interaction.in_progress`、`interaction.requires_action` 等事件取代，但当前 API 参考和流式指南的示例仍然使用 `interaction.status_update`。**【待确认】** 建议两种都处理。

```js
const stream = await ai.interactions.create({ model: "gemini-3.8-flash", input: "从 1 数到 25", stream: true });
let lastEventId;
for await (const ev of stream) {
  lastEventId = ev.event_id ?? lastEventId;
  if (ev.event_type === "step.delta" && ev.delta.type === "text") process.stdout.write(ev.delta.text);
  if (ev.event_type === "interaction.completed") console.log("\n", ev.interaction.usage);
}
```
```bash
curl -N -X POST "https://generativelanguage.googleapis.com/v1beta/interactions" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" -H "Accept: text/event-stream" \
  -d '{"model":"gemini-3.8-flash","input":"从 1 数到 25","stream":true}'
```

来源：

- https://ai.google.dev/api/interactions-api （Interaction、Step、InteractionSseStreamEvent、Usage）
- https://ai.google.dev/gemini-api/docs/interactions-overview
- https://ai.google.dev/gemini-api/docs/interactions/streaming
- https://ai.google.dev/gemini-api/docs/background-execution
- `@google/genai@2.27.0`：`dist/index.mjs`（便捷属性实现）

---

## 6. 2026 年 5 月破坏性变更：新旧对照表

| 旧（legacy，2026-06-08 已移除） | 新（steps schema，Api-Revision 2026-05-20，现为唯一版本） | 备注 |
|---|---|---|
| 响应 `outputs[]`（只含模型生成的内容） | 响应 `steps[]`（带 `type` 的结构化步骤） | POST 只返回输出步骤，GET 返回完整时间线 |
| `interaction.outputs[-1].text` | `interaction.output_text`（SDK），或遍历 `steps` 中的 `model_output.content` | — |
| 遍历 `outputs` 找 `type=="function_call"` | 遍历 `steps` 找 `type=="function_call"` | 内置工具改为 `google_search_call` / `google_search_result` 等成对出现的 step |
| 无状态历史：把 `outputs` 放回 `input` | 把上轮的 `steps` 放进 `input`，再追加一个新的 `user_input` step | thought 的 signature 要原样保留 |
| `response_mime_type: "application/json"` + `response_format: <JSON Schema>` | `response_format: { type: "text", mime_type: "application/json", schema: <JSON Schema> }` | `response_mime_type` 已删除 |
| `generation_config.image_config: { aspect_ratio, image_size }` | `response_format: { type: "image", aspect_ratio, image_size, mime_type? }` | 图像配置移到顶层 |
| `response_modalities: ["audio"]` | `response_format: { type: "audio" }` | TTS |
| `response_modalities: ["image"]` | `response_format: { type: "image" }` | 只要图片 |
| `response_modalities: ["text","image"]` | `response_format: [{ type: "text" }, { type: "image" }]` | 多模态时传数组 |
| SSE `interaction.start` | `interaction.created` | — |
| SSE `content.start` / `content.delta` / `content.stop` | `step.start` / `step.delta` / `step.stop` | — |
| SSE `interaction.complete` | `interaction.completed` | — |
| SSE `interaction.status_update` | 指南称改为 `interaction.in_progress`、`interaction.requires_action` 等 | **【待确认】**：参考仍列 `status_update` |
| 顶层 `role: "model"`（旧响应示例中有） | 新 schema 不再出现 | — |
| SDK 1.x | Python / JS SDK ≥ 2.0.0（概览页现在要求 ≥ 2.3.0） | 1.x 调用 Interactions 会失败 |
| REST 请求头 `Api-Revision: 2026-05-20`（opt-in）/ `2026-05-07`（opt-out） | 2026-06-08 起该请求头被忽略 | 新功能只在 steps schema 上发布 |
| `generation_config.temperature` / `top_p` | SDK 2.27.0 标为 deprecated，渲染版参考不再列出 | 迁移指南说 generation_config 用于"temperature、top_p、thinking 等"模型行为参数，彼此矛盾 **【待确认】** |

迁移指南中有几处 REST 示例用的是 `v1beta2` 路径，与 API 参考的 `v1beta` 不一致。请以参考为准，**【待确认】**。

来源：

- https://ai.google.dev/gemini-api/docs/interactions-breaking-changes-may-2026
- https://ai.google.dev/gemini-api/docs/migrate-to-interactions

---

## 7. 待确认清单（汇总）

1. `model` 带 `models/` 前缀（例如 `models/gemini-nano-banana-2.1`）在 v1beta 是否被接受。v1beta 文档全部用裸 ID；v1 参考的枚举带前缀。
2. `gemini-nano-banana-2.1`、`gemini-3.1-flash-lite-image`、`gemini-3.5-flash-lite`、`gemini-3.5-transcribe` 有模型页和指南示例，但不在 v1beta 参考的 `model` 枚举中。`nano-banana-pro-preview` 只在枚举中出现。
3. Antigravity 的 agent ID：指南用 `antigravity-preview-09-2026`，参考枚举只有 `antigravity-preview-05-2026`。
4. `safety_settings`：参考中有这个字段，但概览页写"Interactions API 不支持自定义安全设置"。
5. `ImageResponseFormat.mime_type`：参考枚举只有 `image/jpeg`，指南示例里出现过 `image/png`。
6. `temperature`、`top_p`：SDK 标为 deprecated，参考不再列出，但文本生成指南仍有示例。服务端是否仍然接受未知。
7. SSE 事件：`interaction.status_update` 与 `interaction.in_progress` / `interaction.requires_action` 的说法互相矛盾。
8. 远程 MCP：概览页说"Gemini 3 does not support remote MCP, this is coming soon"，但函数调用指南有 3.8 Flash 配合 MCP 的示例。
9. `speech_config.mode: "conversational"`：TTS 的 REST 示例中有，参考中没有。
10. Lyria 3.5 选择 WAV 输出时的具体 `mime_type` 值（推测为 `audio/wav`）。
11. `gemini-3-pro-image` 可选的 `thinking_level`；`gemini-3.1-flash-lite` 的默认思考等级；`gemini-3.1-flash-lite-image` 支持哪些宽高比；图像指南的 "3.1 Pro Image" 表格具体对应哪个模型。
12. `google_search` 不写 `search_types` 时的默认行为。
13. `service_tier: "deferred"` 的含义；`code-mender` / `dynamic` 类型的 agent_config 怎么用；`processing_call` / `processing_result` 的用途；`retrieval.rag_store` 在 Gemini API（非 Vertex）下能否使用。
14. OpenAPI 中仍保留的 `cached_content`、`response_mime_type`、`response_modalities`、`transcription_config.adaptation_phrases` / `language_hints`、`agent_config.enable_bigquery_tool` / `max_total_tokens`：在渲染版参考中不出现或已废弃。
15. 迁移指南 REST 示例里的 `v1beta2` 路径和 step 级 `status: "done"/"waiting"`；Omni 示例里 `thought.content` 的结构。
16. SDK 最低版本：概览页写 ≥ 2.3.0，破坏性变更指南写 ≥ 2.0.0，TTS 指南写 JS ≥ 2.24.0。建议直接用最新版（npm 上为 2.27.0）。
17. TTS 的 2.5 系列（`gemini-2.5-*-preview-tts`）、Veo 3.1 能否通过 Interactions 调用。
18. 内联请求大小上限：file-input-methods 页写 100MB（PDF 50MB），图像、音频、视频理解页写 20MB。
19. `data` 的 base64：是否接受 URL-safe 或不带 padding（按 protobuf 规则应接受），带 `data:...;base64,` 前缀会怎样。内联 `data` 不带 `mime_type` 能否被接受。
20. 直接在 `uri` 写 `gs://` 能否用（文档要求先 `files:register`）。Nano Banana、Lyria、Transcribe 使用 Files API `uri` 或外部 URL 的方式（通用规则说支持，但没有该模型的示例）。
21. 图像和 PDF 上传后是否也要等 `state=ACTIVE`。Files API 配额：视频理解页写"20GB (paid) / 2GB (free)"，与"每文件 2GB、每项目 20GB"的写法不一致。
22. Lyria 3.5 的"视频配乐"：文档未提及。WAV 输出的 `mime_type` 值、Lyria 是否支持 `delivery:"uri"`。
23. 图像和音频输出用 `delivery:"uri"` 时返回的 URI 形式和下载方式（只有 Omni 视频有文档）。生成文件的保存期（以 `expirationTime` 为准）。TTS 能否输出 `audio/mp3` 或 `audio/ogg_opus`。
24. 视频 `processing.start_offset` / `end_offset`：参考和 SDK 写字符串 `"10.5s"`，视频理解指南示例写数字 `1200`。
25. Omni 的参考图张数上限；Nano Banana 是否支持 `<IMAGE_REF_N>` 一类的提示标签；`audio` 块的 `channels` / `sample_rate` 何时必填。
26. SDK `tFileName` 用 `/[a-z0-9]+/` 从 https URI 截取文件 ID，遇到含 `-` 的 ID 可能被截断。建议给 `files.get` / `files.download` 传 `files/{id}` 字符串。
27. 用户上传的输入媒体在多轮对话（`previous_interaction_id`）中能保留多久；能否把 GENERATED 文件的 uri 直接作为下一次请求的输入。
28. 文档中的代码错误（不是待确认项，提醒注意）：
    - JS 示例 `config:{mime_type}` 应为 `mimeType`；
    - `uploadedFile.mime_type` 应为 `mimeType`；
    - Omni 示例 `fInfo.state.name` 应为 `fInfo.state`；
    - REST 示例用 `jq '.output_video.uri'`，但原始 JSON 中没有 `output_video`；
    - file-input-methods / files 页的 REST 示例还在用旧的 `.outputs[]`。

---

## 8. 主要来源

- API 参考（v1beta）：https://ai.google.dev/api/interactions-api
  - Markdown 版：https://ai.google.dev/static/api/interactions.md.txt
  - OpenAPI 版：https://ai.google.dev/static/api/interactions.openapi.json
- API 参考（v1）：https://ai.google.dev/api/interactions-api-v1
- 概览：https://ai.google.dev/gemini-api/docs/interactions-overview
- 破坏性变更（2026-05）：https://ai.google.dev/gemini-api/docs/interactions-breaking-changes-may-2026
- 迁移指南：https://ai.google.dev/gemini-api/docs/migrate-to-interactions
- 思考：https://ai.google.dev/gemini-api/docs/interactions/thinking
- 模型总览：https://ai.google.dev/gemini-api/docs/models
- 图像生成：https://ai.google.dev/gemini-api/docs/image-generation
- 语音合成：https://ai.google.dev/gemini-api/docs/speech-generation
- 音乐生成：https://ai.google.dev/gemini-api/docs/music-generation
- 视频（Omni）：https://ai.google.dev/gemini-api/docs/omni
- 转写：https://ai.google.dev/gemini-api/docs/transcribe
- Deep Research：https://ai.google.dev/gemini-api/docs/deep-research
- Antigravity：https://ai.google.dev/gemini-api/docs/antigravity-agent
- 结构化输出：https://ai.google.dev/gemini-api/docs/structured-output
- 函数调用：https://ai.google.dev/gemini-api/docs/function-calling
- 流式：https://ai.google.dev/gemini-api/docs/interactions/streaming
- 后台执行：https://ai.google.dev/gemini-api/docs/background-execution
- Webhooks：https://ai.google.dev/gemini-api/docs/webhooks
- Flex：https://ai.google.dev/gemini-api/docs/flex-inference
- Priority：https://ai.google.dev/gemini-api/docs/priority-inference
- 文件输入方式：https://ai.google.dev/gemini-api/docs/file-input-methods
- Files API 指南：https://ai.google.dev/gemini-api/docs/files
- Files API 参考：https://ai.google.dev/api/files
- 图像理解：https://ai.google.dev/gemini-api/docs/image-understanding
- 视频理解：https://ai.google.dev/gemini-api/docs/video-understanding
- 音频理解：https://ai.google.dev/gemini-api/docs/audio
- 文档处理：https://ai.google.dev/gemini-api/docs/document-processing
- 媒体分辨率：https://ai.google.dev/gemini-api/docs/interactions/media-resolution
- Agent 环境：https://ai.google.dev/gemini-api/docs/agent-environment
- Lyria 提示指南：https://ai.google.dev/gemini-api/docs/lyria-prompt-guide
- protobuf JSON 映射（bytes 的 base64 规则）：https://protobuf.dev/programming-guides/json/
- SDK：https://www.npmjs.com/package/@google/genai （v2.27.0）


## 9. 本次官方结构核对补充

本次直接下载官方 OpenAPI 和 Markdown 后，下列字段补充到原手册。完整对象、所有枚举及字段必填 / 废弃标记见 [官方字段完整对照](/api/gemini-schema)。

| 原章节 | 补充内容 |
| --- | --- |
| §3.4 retrieval | retrieval_types 还包括 vertex_ai_search；vertex_ai_search_config 包含 datastores（string[]）、engine（string） |
| §3.4 RAG | rag_store_config 保留已废弃的 similarity_top_k、vector_distance_threshold；rag_retrieval_config.ranking 有 ranking_config: rank_service 判别字段、model_name 和 rank_service.model_name |
| §3.7 env | 字典值是 EnvVar 对象，包含 value 或 credential，不能把所有值都写成普通字符串；env 本身也允许字符串分支 |
| §3.7 network.allowlist | EgressRule 包含 credential；transform 允许单个头字典或头字典数组 |
| §3.2 transcription_config | 顶层 diarization_mode、timestamp_granularities 也已废弃；新字段使用 mode 的 verbatim 对象分支；adaptation_phrases、language_hints 同样保留 deprecated 标记 |
| §3.1 FileCitation | 还有 custom_metadata、document_uri、media_id、page_number、source；索引按字节而非 JavaScript 字符位置计量 |
| §5.6 SSE | StepDelta 可带 metadata，metadata.total_usage 是累计用量；StepStop 的 step_usage 与 usage 都需按 schema 处理 |
| §2 required | OpenAPI 的 ModelInteraction.required 混入输出字段，并包含未在 properties 中声明的 steps；不要据此发送 created/id/status/updated/steps |

上述字段在 web2api 官方 Interactions 后端完整转发；网页登录态 HTTP 聊天入口没有对应的官方对象映射。中文说明中的字段存在与某个模型、服务版本实际支持是两种结论。
