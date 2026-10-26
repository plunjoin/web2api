# 多模态透传

多模态透传按两级路由：

1. **按模型路由（优先）**：请求体 `model` 命中订阅/custom 引擎目录时，转发到拥有该模型的第三方 Key 上游（OpenAI 兼容）。原生模式下即可使用，例如把 Google AI Studio 的 API Key 作为 custom 账号添加后，`/v1/embeddings` 可直接调用 `gemini-embedding-*`。目录匹配对 `models/` 前缀宽松（Google 目录 id 带前缀，裸名同样命中）。
2. **引擎 B upstream 模式回退**：模型未命中目录时，在 `mode: upstream` 且 `passthrough: true` 时透传到配置的 OpenAI 兼容上游。对应上游必须实现这些端点：

| 路径 | 用途 |
| --- | --- |
| `POST /v1/images/generations` | 图片生成，详见[图片生成](/api/images) |
| `POST /v1/images/*` | 其他上游图片端点 |
| `/v1/videos/*` | 未被标准视频路由占用的上游视频路径 |
| `POST /v1/audio/speech` | 文本转语音，详见[音频生成](/api/audio) |
| `POST /v1/audio/*` | 其他上游音频端点 |
| `POST /v1/music/generations` | 音乐生成（Lyria 等），转发给拥有该模型的账号上游 |
| `POST /v1/files/*` | 文件上传和处理 |
| `POST /v1/embeddings/*` | Embedding 请求 |

音乐模型**不要**发到 `/v1/chat/completions`：OpenAI 兼容的 chat 端点只能返回文本，上游会以 `Unhandled generated data mime type: audio/mpeg` 之类的 400 拒绝。网关现在会在发出请求前拦下这类用途不匹配的调用，返回 `400 unsupported_endpoint` 并提示正确端点；只有引擎 B `native` 模式（能把 inline data 转成 data URI 放进 chat 响应）才允许音乐/图片模型走 chat。

透传请求仍然使用 `Authorization: Bearer sk-...`。JSON 请求使用 `Content-Type: application/json`，二进制内容使用 `application/octet-stream`。路径和请求体字段由对应上游协议决定，使用前请确认配置中的 `mode: upstream`（按模型路由不受此限制）。

## 透传范围与 Gemini 官方参数

上述专用路径保留原始请求体；`/v1/chat/completions` 会重新组装固定聊天字段，即使启用 passthrough 也不会原样转发 tools、response_format、generation_config 等额外字段。消息级工具元数据也会丢失。聊天可传字段见[聊天补全](/api/chat)。

新增[官方 Gemini 后端](/api/gemini-official)，独立配置 gemini_api 后可使用 `/v1/interactions`、`/v1beta/interactions`、`/upload/v1beta/files`。该后端保留官方协议、处理 SDK 鉴权和断点上传，并直接刷新官方 SSE。直接把 Google 官方地址设为 OpenAI upstream base_url 不能完成这种转换；需要上游自行提供兼容端点。

原生协议内部的上传 / Drive 引用能力不代表网关已实现官方 Files API。官方 data / uri 输入、Files 生命周期、交互存储、工具和后台回调的参数说明见[Gemini 参数与差距](/api/gemini)及[中文参考详解](/api/gemini-reference)。

`POST /v1/videos`、`GET /v1/videos/{id}`、`GET /v1/videos/{id}/content` 固定使用原生 Veo 适配，在 upstream 模式下不会透传，调用会返回未启用原生协议的错误。

默认 native 模式的图片/音频生成使用 `/v1/chat/completions`；图片/音频专用透传路径在该模式下且模型未命中 custom 目录时返回 `502`。透传只替换为服务端配置的上游认证，保留请求方法、路径、Content-Type 和请求体，以及上游响应状态、响应头和正文；字段支持范围及默认值由上游决定。

> AI生成
