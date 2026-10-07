# Gemini 参数与实现差距

本页对照 2026-10-07 的 Gemini Interactions 官方文档和当前 web2api 代码。官方端点是 `https://generativelanguage.googleapis.com/v1beta/interactions`；web2api 新增官方 Gemini API Key / OAuth 后端，启用后 `/v1/interactions` 和 `/v1beta/interactions` 完整转发官方请求、响应及 SSE。网页登录态的 `/v1/chat/completions` 继续使用 OpenAI 兼容协议，官方参数应放到 Interactions 入口中。

完整资料分为三个入口：本页说明每类能力的映射和遗漏；[官方字段完整对照](/api/gemini-schema)列出官方快照中所有操作和全部组件的字段、联合类型、枚举、必填标记和废弃标记；[中文参考详解](/api/gemini-reference)提供媒体上传、模型选择、函数调用、后台任务和 SDK / REST 示例。

## 对照依据和覆盖范围

| 依据 | 用途 | 核验方式 |
| --- | --- | --- |
| [官方 OpenAPI](https://ai.google.dev/static/api/interactions.openapi.json) | 端点、路径 / 查询参数、全部嵌套 schema、枚举、约束 | 本次直接下载并保存快照，自动递归生成字段对照 |
| [官方 Markdown](https://ai.google.dev/static/api/interactions.md.txt) | 官方字段描述、渲染版参考与 OpenAPI 的差异 | 本次直接下载并保存快照 |
| 用户提供的 `gemini-interactions-api-reference.md`（2026-10-07） | 中文解释、功能指南、模型限制、迁移注意事项 | 整理到中文参考详解；原文中待确认结论保留，指南 / SDK 专属结论没有逐一调用 Google 验证 |
| 当前仓库代码 | web2api 实际接收、编码和返回的内容 | 检查 HTTP 路由、ChatRequest、provider、私有协议编码器和响应转换 |

“全部字段”涵盖 **官方快照全部 20 个路径、37 个操作、214 个 schema**：包括 Interactions 创建 / 获取 / 取消 / 删除，以及 agents、voices、environments、webhooks、triggers、credentials 的独立管理操作、请求、响应、错误和 SSE 类型。快照未定义的 Files / models 独立 API 由通用官方路由转发，其常用调用流程在中文详解中说明。

对照表按 schema 分组，字段名保留官方 snake_case。同一字段在多个对象中出现时分别列出；JSON Schema、函数参数、标签和 HTTP 头等自由对象没有固定键列表，用字典类型说明。`type` 判别值和 `oneOf` 分支也纳入对照。

## 官方后端与网页登录态的支持范围

官方后端保留全部请求体字段、查询参数、输出结构和事件，不经过 ChatRequest 的缩减转换；Google 校验模型、参数和权限。创建、获取、取消、删除以及快照中的 agent、沙箱、声音、webhook、trigger、credential 资源均已接入，Files 上传和续传也已提供。配置、SDK、存储与测试说明见[官方后端接入](/api/gemini-official)。

**下文“未开放 / 尚未实现 / 内部具备”均专指网页登录态与 OpenAI 兼容聊天映射，不表示官方后端缺少这个字段。** 这些字段通过官方后端已可完整转发；实际模型可用性由 Google 决定。

## 网页登录态支持状态如何判断

| 状态 | 含义 |
| --- | --- |
| 已映射 / 部分映射 | 现有网关字段具备对应能力，但路径、消息结构、事件和返回值仍是 OpenAI 兼容协议 |
| 内部具备，HTTP 未开放 | AI Studio 私有协议层已有结构或编码器，但聊天入口没有把 JSON 字段传入；不能据此认为客户端可用 |
| 尚未实现 | 没有对应 HTTP 端点或完整处理流程 |
| 官方待确认 / 已废弃 | 官方资料本身存在冲突，或快照保留历史字段；收录用于对照，不承诺可用 |

聊天入口把请求反序列化为固定的 `ChatRequest`。未知顶层字段会被忽略，`Extra` 标为 `json:"-"` 且没有保存原始请求的逻辑。**这也影响 upstream 模式的聊天接口**：适配器重新组装请求，只发送 `model`、`messages`、`stream`、`temperature`、`top_p`、`max_tokens`；`user` 也没有发送到上游。只有专用媒体 / 文件透传路径保留原始请求体。

## 官方端点与网关端点

| 官方操作 | 官方参数 | 官方后端 / 网页登录态对照 |
| --- | --- | --- |
| `POST /v1beta/interactions` | ModelInteraction 或 AgentInteraction 请求体 | 官方后端完整转发；网页登录态只覆盖部分生成能力 |
| `GET /v1beta/interactions/{interactionsId}` | `interactionsId`、`include_input`、`stream`、`last_event_id` | 官方后端完整转发全部查询参数；网页聊天未映射 |
| `POST /v1beta/interactions/{interactionsId}/cancel` | `interactionsId`；只取消运行中的后台交互 | 官方后端提供取消接口；网页聊天未映射 |
| `DELETE /v1beta/interactions/{interactionsId}` | `interactionsId` | 官方后端提供删除接口，存储由 Google 管理；网页聊天未映射 |
| `POST /upload/v1beta/files` | 官方 Files API 上传流程 | 官方后端支持上传 / 续传，会话 URL 存 SQLite；`/v1/files/*` 仍是兼容上游透传 |

官方后端的网关入口支持 `x-goog-api-key: sk-...`、`Authorization: Bearer sk-...` 和 `?key=sk-...`；出站使用服务器 Google 凭据。网关 Key、Google API Key、网页登录态是不同凭据。官方后端转发 /v1 稳定版和 /v1beta；原有 /v1/chat/completions 的协议仍为 OpenAI 兼容。

## 请求顶层字段逐项映射

| 官方字段 | 官方用途 | 当前网关映射 / 遗漏 |
| --- | --- | --- |
| `model` | 普通模型 ID | 聊天 `model`，以 `GET /v1/models` 目录为准；官方列出的新模型不会自动加入账号可用目录 |
| `agent` | managed / 自定义 agent ID | 尚未实现；不能把 agent ID 当普通 `model` 等价调用 |
| `input` | string、Content、Content[]、Step[] | 使用 `messages`；native 仅提取字符串及 `type: text` 段，未映射官方媒体和步骤历史 |
| `system_instruction` | 交互级系统指令 | native 将 `messages` 中的 `system` 文本拼接为 System；网页引擎拼接到提示词 |
| `generation_config` | 思考、token、工具、TTS、转写、视频行为 | 未接收此对象；只有聊天 `max_tokens` 和旧采样参数进入 native Config |
| `agent_config` | agent 配置 | 尚未实现，所有四种分支均见后文 |
| `response_format` | 单个或多个 text / image / audio / video 输出配置 | 未映射；native 输出模态根据模型目录推导，不能用此字段指定比例、编码或 JSON Schema |
| `tools` | 函数及内置工具 | 内部有部分工具编码器，HTTP 未开放 |
| `previous_interaction_id` | 服务端续接历史 | 尚未实现；客户端需在 `messages` 中携带文本历史 |
| `store` | 是否存储官方交互 | 尚未实现；本地账号池 / Redis 会话不等同于官方交互存储 |
| `stream` | 官方 SSE | 网关同名字段控制兼容聊天 SSE；不提供 step 事件和 event_id |
| `background` | 立即返回交互 ID，后台执行 | 尚未实现；Veo 独立长任务不能等价替代 Interactions 后台执行 |
| `service_tier` | `standard` / `flex` / `priority` / `deferred` | 未开放；网页账号权益选择不等同于客户端可指定官方服务等级 |
| `safety_settings` | 内容安全阈值和方法 | 未开放自定义设置；GenerateContent 编码使用内部 observedSafetySettings，且官方资料存在支持范围冲突 |
| `environment` | 远程沙箱配置或环境 ID | 尚未实现 |
| `webhook_config` | 动态完成回调及用户元数据 | 尚未实现；网关不会接收完成回调 |
| `labels` | 自定义键值标签 | 尚未实现，网关 `user` 不等价于此字典 |
| `continuation_token` | 状态 incomplete 时继续长解码 | 尚未实现；与 previous_interaction_id 用途不同 |
| `cached_content` | 显式缓存引用，仅 OpenAPI 保留 | 未开放；附件指出概览页称尚不支持，待确认 |
| `response_mime_type` | 旧输出 MIME 字段 | HTTP 未映射；内部同名私有协议字段仍存在，官方 Interactions 已废弃 |
| `response_modalities` | 旧模态数组 | HTTP 未映射；内部根据模型设置旧私有协议模态，官方新请求改用 response_format |
| `id`、`created`、`updated`、`status`、`steps`、`usage`、`errors`、`environment_id` | 输出 / 诊断字段，具体所属 schema 见完整表 | 不应放到创建请求中；网关返回自己的聊天或 Veo 结构 |

## generation_config 全部参数

| 官方字段 | 类型 / 值 | 内部协议与 HTTP 状态 |
| --- | --- | --- |
| `max_output_tokens` | integer | HTTP `max_tokens` → Config.MaxOutputTokens；上限由模型目录校验，不能把思考与回答预算简单分开 |
| `seed` | integer | Config.Seed 已定义；HTTP 未映射 |
| `stop_sequences` | string[] | 内部支持部分 GenerateContent 的客户端停止匹配；CreateInteractionStream 明确拒绝；HTTP 未映射 stop 或 stop_sequences |
| `thinking_level` | `minimal` / `low` / `medium` / `high` | 内部用 ReasoningEffort / ThinkingBudget 选择思考等级；HTTP 两者均未映射，不能直接传 thinking_level |
| `thinking_summaries` | `auto` / `none` | 内部有 reasoning 事件，私有 Interaction 编码固定设置摘要项；HTTP 未提供控制开关，也不返回 thought 结构 |
| `tool_choice` | `auto` / `any` / `none` / `validated` 或 allowed_tools 对象 | 内部 Tools.ToolConfig.Mode 只接受 auto / none，未实现 any / validated / allowed_tools；HTTP 未映射 |
| `speech_config` | SpeechConfig[] 或 `{speakers: SpeechConfig[]}` | 内部 SpeechConfig 支持 VoiceName / Speakers / Mode；与官方 voice / language / speaker 形状不同，尤其没有 language 字段；HTTP 未映射 |
| `transcription_config` | 自定义词汇、语言、逐字 / 智能模式 | 内部支持 WordTimestamps / SpeakerLabels / CustomVocabulary / LanguageCodes / SmartTranscription；与官方 mode 多态结构不同；HTTP 没有音频输入或配置映射 |
| `video_config.task` | `text_to_video` / `image_to_video` / `reference_to_video` / `edit` / `extend` | 没有此官方对象；Veo 路由当前只支持文本生成视频 |
| `temperature`、`top_p` | number，OpenAPI 标记 deprecated | HTTP 同名字段传入 native Config 或兼容上游；某些私有 Interaction 路径不编码它们，不保证所有模型生效 |
| `image_config.aspect_ratio`、`image_config.image_size` | 旧图像配置，deprecated | 内部 ImageConfig 已定义，HTTP 未开放；官方新协议位置为 response_format 的 image 分支 |

官方转写配置当前还保留 `adaptation_phrases`、`language_hints`、顶层 `diarization_mode`、顶层 `timestamp_granularities` 四个废弃字段。新配置把后两项放进 `mode: {type:"verbatim", ...}`，智能模式用 `mode: "smart"` 或 `{type:"smart"}`；它们全部收录在字段完整表中。

## response_format 全部输出配置

| 分支 | 官方全部字段 | 当前网关情况 |
| --- | --- | --- |
| text | `type: "text"`、`mime_type`（text/plain、application/json）、`schema` | 内部有 ResponseMIMEType / ResponseSchema，但 HTTP 未映射；在提示词里要求 JSON 不等同于服务端 schema 校验 |
| image | `type: "image"`、`aspect_ratio`、`image_size`、`mime_type`、`delivery` | 内部有比例 / 尺寸结构但 HTTP 未开放；Markdown 输出不能保证指定格式、尺寸、交付方式 |
| audio | `type: "audio"`、`mime_type`、`sample_rate`、`bit_rate`、`delivery` | HTTP 未映射声音 / 编码 / 采样率 / 码率 / 交付方式；结果按实际 MIME 处理 |
| video | `type: "video"`、`aspect_ratio`、`resolution`、`duration`、`delivery`、`gcs_uri` | Veo 有比例、分辨率、秒数的相似能力；不能传此对象，不能据此启用 Omni 编辑 / 延长或 GCS 输出 |

图片完整宽高比为 `1:1`、`2:3`、`3:2`、`3:4`、`4:3`、`4:5`、`5:4`、`9:16`、`16:9`、`21:9`、`1:8`、`8:1`、`1:4`、`4:1`；尺寸为 `512`、`1K`、`2K`、`4K`，K 大写。字段枚举是协议范围，不代表每个图像模型支持全部值。图像输出 MIME 在参考中仅声明 image/jpeg，而部分指南使用 image/png，保留待确认状态。

音频输出枚举为 audio/mp3、audio/ogg_opus、audio/l16、audio/wav、audio/alaw、audio/mulaw；视频输出比例为 16:9 / 9:16，分辨率为 360p / 720p / 1080p / 4k。官方 duration 是带单位的字符串，如 `"8s"`；网关 Veo 的 seconds 为整数或整数文本，如 `8` / `"8"`。

官方 `delivery: "inline"` 表示 Content.data 中的裸 base64，`delivery: "uri"` 表示 URI。网关 native 则把媒体包装为 `![media](data:...)` 或 URL，并写入聊天 content；客户端不能套用官方 steps 提取逻辑。

## input、媒体和标注

| 官方对象 | 全部字段 / 分支 | 当前遗漏 |
| --- | --- | --- |
| TextContent | `type`、`text`、`annotations` | 文本可经 messages 映射；标注未映射 |
| ImageContent | `type`、`data`、`uri`、`mime_type`、`resolution` | 内部有 InlineData / ExternalMedia / File；HTTP messages 只取文本，图片编辑参考图未接入 |
| AudioContent | `type`、`data`、`uri`、`mime_type`、`channels`、`sample_rate` | HTTP 未接入音频；内部 Blob / ExternalMedia 不等于完整官方块 |
| VideoContent | `type`、`data`、`uri`、`mime_type`、`name`、`processing`、`resolution` | HTTP 未接入视频理解 / 视频编辑输入 |
| DocumentContent | `type`、`data`、`uri`、`mime_type` | HTTP 未接入 PDF / CSV；内部文件引用不等于官方 Files API URI |
| StaticMediaProcessing | `type: "static"`、`fps`、`start_offset`、`end_offset`；processing 也可为 static / agentic 字符串 | HTTP 未接入；偏移量参考用 duration 字符串，如 `"10.5s"` |
| Annotation | UrlCitation、FileCitation、PlaceCitation、SpeechAnnotation、WordInfo | 引用、语音元数据、词级时间戳没有以此结构返回 |
| Step[] | user_input、model_output、thought、函数 / 内置工具的 call / result | 不支持官方步骤历史回放和签名原样传回 |

Content 的 MIME 枚举、每一种 Annotation 的全部属性、各 Step 和增量事件的全部字段均见[字段完整对照](/api/gemini-schema)。例如 FileCitation 不只有文件名和索引，还包含 document_uri、custom_metadata、media_id、page_number、source；WordInfo 还包含 start_offset / end_offset / speaker。

官方 JSON 的 `data` 是裸 base64 字符串，不能直接放 Buffer 或带 `data:image/png;base64,` 的完整 URI。使用 Files API 时原样引用返回的 file.uri；本地路径和 file:// 不会被 Google 自动读取。输入上传大小、文件保存期、处理状态、YouTube 和各模型支持矩阵见[中文参考详解](/api/gemini-reference)。

网关聊天读取请求体最多 8 MiB；这与官方输入上限是两回事。官方不同指南给出 20MB / 100MB 的冲突口径，详解保留该差异，不能把这些值当成网关可上传额度。

## tools 全部工具分支

| 官方工具 | 主要配置（全部深层字段见完整表） | 内部协议 / 对外状态 |
| --- | --- | --- |
| function | `name`、`description`、`parameters` | 内部有函数声明、调用、结果与签名；HTTP 未开放，tool 消息当前按用户文本处理 |
| google_search | `search_types`：web_search / image_search / enterprise_web_search | 内部有 GoogleSearchOptions.WebSearch / ImageSearch / TimeRange；不等价于全部官方选项，HTTP 未开放 |
| code_execution | `type` | 内部有编码与 ExecutableCode / CodeExecutionResult 事件；HTTP 未开放 / 未返回结构化事件 |
| url_context | `type` | 内部有部分工具支持与 grounding 事件；HTTP 未开放 |
| google_maps | `latitude`、`longitude`、`enable_widget` | 内部工具支持不等于有完整位置 / widget 请求映射；HTTP 未开放 |
| file_search | `file_search_store_names`、`metadata_filter`、`top_k` | 未开放，不能把普通文件透传当成 File Search Store |
| computer_use | `environment`、`disabled_safety_policies`、`enable_prompt_injection_detection`、`excluded_predefined_functions` | 尚未实现；完整安全策略枚举见完整表 |
| mcp_server | `name`、`url`、`headers`、`allowed_tools` | 尚未实现；官方不同页面的 Gemini 3 支持说明有冲突 |
| retrieval | `retrieval_types`、`rag_store_config`、`vertex_ai_search_config`、`exa_ai_search_config`、`parallel_ai_search_config` | 尚未实现；Vertex 相关配置不能推断在 Gemini Developer API 可用 |

附件遗漏的 retrieval 类型 **vertex_ai_search** 已补入：`vertex_ai_search_config` 包含 `datastores` 和 `engine`。RAG 还包括 rag_resources 的 rag_corpus / rag_file_ids、rag_retrieval_config 的 top_k / filter / hybrid_search / ranking、废弃的 similarity_top_k / vector_distance_threshold。ranking 的判别字段是 `ranking_config: "rank_service"`，另有 model_name 和 rank_service.model_name，不能只写一个省略号对象。

函数调用完整流程要求模型输出 function_call，客户端执行函数，以相同 call_id 回传 function_result，并保留 thought signature / 全部步骤历史；当前网关没有实现这个闭环。完整表同时收录 Google Search、Maps、检索、MCP、代码执行、URL Context、processing 的所有 call / result 和 SSE delta 类型。

## agent、沙箱、安全和回调

| 对象 | 全部配置 / 子对象 | 当前状态 |
| --- | --- | --- |
| DeepResearchAgentConfig | `type`、`thinking_summaries`、`visualization`、`collaborative_planning`、仅 OpenAPI 的 `enable_bigquery_tool` | 尚未实现；官方 Deep Research 需要 background=true，不能用聊天 stream 代替 |
| AntigravityAgentConfig | `type`、`model`、仅 OpenAPI 的 `max_total_tokens` | 尚未实现；agent model 与顶层普通 model 用途不同 |
| CodeMenderAgentConfig | `type`、`model`、`session_id`、`session_config.max_rounds`、`find_request`、`fix_request` | 尚未实现；source_files 元素包含 content / path，find mode 为 scan / verify |
| DynamicAgentConfig | `type: "dynamic"` | 尚未实现；官方具体使用方法待确认 |
| EnvironmentConfig | `type: "remote"`、`environment_id`、`sources`、`env`、`network` | 尚未实现 |
| Source | `type`（gcs / inline / repository / skill_registry）、`source`、`target`、`content`、`encoding` | 尚未实现；附件示例内的沙箱文件内容是传给 Google agent 的数据 |
| EnvVar | `value`、`credential`；EnvironmentConfig.env 也可为字符串 | 尚未实现；这是附件未展开的服务器管理凭据引用 |
| EnvironmentNetworkEgressAllowlist / EgressRule | `allowlist[]`；元素含 `domain`、`transform`（字典或字典数组）、`credential`；network 也可为 disabled | 尚未实现；已补入凭据引用和两种头注入形状 |
| SafetySetting | `type`、`threshold`、`method` | HTTP 未开放；官方参考列字段但概览称不支持自定义安全设置，待确认 |
| WebhookConfig | `uris[]`、`user_metadata` | 尚未实现；官方动态回调需 background=true |
| labels | string → string | 未开放；键和值最长 63 个 Unicode 字符，键以字母开头，其余格式限制见详解 |

官方状态存储规则：store 默认 true；previous_interaction_id 续接历史，但 system_instruction、tools、generation_config 等每轮需要重传；store=false 不能配合 background=true；continuation_token 需原样回传。官方后端转发这些参数，并由 Google 执行对应存储生命周期；网页登录态聊天没有同等映射。

## 返回字段和 SSE 差异

| 官方结构 | web2api 实际情况 |
| --- | --- |
| Interaction.id / status / created / updated / environment_id / continuation_token / errors | 返回自己的 chatcmpl ID、Unix created 和固定兼容 finish_reason；没有完整交互状态；Veo 状态是另一套任务结构 |
| steps 中的 model_output.content | 聊天正文 / 媒体链接放到 choices[0].message.content，流式放到 choices[0].delta.content |
| thought.summary / signature | 内部能解码部分 reasoning 和签名事件，但 provider.generate 只消费文本、媒体、结束、错误；其他事件不会以官方对象传给客户端 |
| function_call / function_result 与内置工具结果 | 没有完整结构化返回与客户端工具执行闭环 |
| usage.total_input_tokens / total_output_tokens / total_thought_tokens / total_cached_tokens / total_tool_use_tokens / total_tokens | 内部有部分权威用量事件，但 HTTP 聊天响应不使用它们；非流式 usage 来自字符长度估算，不能当官方计费值 |
| usage.input_tokens_by_modality / output_tokens_by_modality / cached_tokens_by_modality / tool_use_tokens_by_modality / grounding_tool_count | 没有分模态用量 / 接地计费次数结构 |
| SDK output_text / output_image / output_audio / output_video | 属于 SDK 便捷属性，原始官方 REST 和网关聊天 JSON 中均没有这些属性 |
| event_type / event_id / step.index / delta / metadata.total_usage / step_usage | 网关使用 data: ChatCompletionChunk 与 [DONE]；没有步骤下标、官方增量对象、重放 ID 或断点恢复 |

官方状态包括 in_progress、requires_action、completed、failed、cancelled、incomplete、queued，以及已废弃的 budget_exceeded。不要把兼容聊天 finish_reason="stop" 解释为以上状态的完整映射。当前流式错误可能作为 delta 文本 `[Error] ...` 返回，此时 HTTP 流已经开始，不代表一次成功生成。

## 新旧协议和官方待确认项

2026 年 5 月变更后的官方结构是 steps 和带 type 的 response_format。旧 outputs、response_modalities、response_mime_type、generation_config.image_config 不能继续写成新官方示例。仓库内部的旧字段用于 AI Studio 私有协议，是否要改取决于实际 RPC，不能因为公开 API 改名就直接改私有 wire 编号。

| 差异 / 矛盾 | 本次文档处理 |
| --- | --- |
| OpenAPI 把输出字段放入 ModelInteraction.required，甚至 required 包含未列出的 steps | 完整表保留原始 required，同时明确创建请求不可据此发送输出字段 |
| generation_config 的 temperature / top_p / image_config 及旧输出字段仍在 OpenAPI | 保留并标注 deprecated，详解使用新 response_format |
| 转写顶层 diarization_mode / timestamp_granularities 仍保留且 deprecated | 补齐附件漏项，说明新 mode 对象位置 |
| 参考 model 枚举缺部分指南里的新 ID；v1 与 v1beta 的 models/ 前缀不一致 | 完整表列快照真实枚举；详解单列指南 ID；网关以实时目录为准 |
| 自定义 safety_settings、远程 MCP 支持范围、图像输出 image/png | 保留待确认，不把文档字段存在等同于模型支持 |
| SSE status_update 与状态专属事件的迁移说明冲突 | 收录快照全部事件；详解建议直接调用官方时兼容处理 |
| 内联媒体 20MB / 100MB、Files 配额、生成文件保存期 | 按资料来源保留差异，官方 Files 保存期与交互保存期分别说明 |
| speech_config.mode、deferred 服务等级、CodeMender / Dynamic 用法、processing 步骤 | 作为待确认项；schema 不存在的字段不计入官方字段统计 |

中文详解末尾保留更完整的待确认清单，包括 SDK 版本、模型特有输入限制、Files 下载、视频偏移类型和 URI 复用等细节。没有以账号实测或线上生成验证这些待确认能力。

## 本次补齐的文档遗漏

1. 增加官方四个 Interactions 操作的完整字段对照，包含深层引用、联合类型、枚举说明、路径 / 查询参数、响应、错误和 SSE。
2. 补齐 retrieval 的 Vertex AI Search 分支、RAG 排序与废弃字段、沙箱 EnvVar / EgressRule 凭据引用、转写废弃字段，以及引用标注与事件元数据。
3. 扩写现有聊天、图片、音频、视频、透传和总览页，说明哪些字段被忽略、哪些是私有协议能力、哪些示例用于直接调用 Google。
4. 修正文档中“聊天其余参数按上游传递”的描述：upstream 聊天也会丢弃未映射字段，专用路径透传才保留请求体。
5. 保存官方来源快照，并提供离线重新生成与一致性校验，方便后续文档升级时发现漏项。
6. 新增官方后端，完整转发全部参数、资源操作与 SSE；Cookie 会话和官方上传会话保存 SQLite，入口和测试目录已重构。

## 网页登录态后续实现缺口

| 优先级 | 需要实现的内容 | 完成标志 |
| --- | --- | --- |
| 第一批 | HTTP 请求扩展、文本 / 多模态消息转换、生成配置、response_format、未知参数校验 | 客户端传入的配置确实进入所选 RPC；不支持的字段返回明确错误 |
| 第二批 | 工具声明 / 调用 / 结果、thought signature、结构化 JSON、引用和权威 usage | 请求和输出都能保留结构，工具执行可完成后续轮次 |
| 第三批 | Interactions 路由、状态存储、background、查询 / 取消 / 删除、event_id 恢复、续写 token | 四个官方操作和生命周期可验收，完成 / 失败 / requires_action 均可区分 |
| 第四批 | agent、沙箱、MCP、检索、安全设置、服务等级、webhook、模型专属媒体流程 | 每个模型 / agent 的实际支持范围验证后再写入支持声明 |

以上缺口针对网页登录态的 OpenAI 兼容适配。官方后端已经完整转发这些能力；本次同时完成官方路由、凭据替换、SSE、Files 续传和 SQLite Cookie 会话，网页登录态对照中的限制仍按真实实现标注。

## 如何维护全量对照

保存的原始资料位于 `docs/public/reference/`。更新快照时同时记录来源、日期和校验值，再运行：

```bash
node scripts/audit-gemini-docs.mjs
node scripts/audit-gemini-docs.mjs --check
npm --prefix docs run build
```

生成器从全部官方操作入口递归追踪 $ref，并收录所有组件、展开内联对象、数组、字典、oneOf / anyOf / allOf；遇到无法解析的 schema 会失败。`--check` 验证字段页与保存快照完全一致。这个校验保证已保存官方 schema 的覆盖，不能自动验证模型实际可用性、指南专属限制或网关新增代码是否正确；这些变化仍需更新本页支持状态。

代码依据：`internal/gateway/server.go`、`internal/gateway/chat.go`、`internal/model/openai.go`、`internal/provider/engine_b_native.go`、`internal/provider/engine_a.go`、`internal/engine/upstream/openai.go`、`internal/aistudio2api/aistudio/types.go`、`generate.go`、`interaction.go`、`transcribe.go`、`tools.go`、`tool_validation.go`。
