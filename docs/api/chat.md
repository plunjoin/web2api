# 聊天补全

`POST /v1/chat/completions` 使用 OpenAI Chat Completions 兼容请求。`messages` 必填且不能为空；建议显式指定 `model`，省略或空字符串时使用 `gemini-flash`。请求是否可执行取决于实际模型目录和账号状态。

此接口不接收 Gemini Interactions 原始请求。官方全部字段及映射见[Gemini 参数与差距](/api/gemini)和[官方字段完整对照](/api/gemini-schema)。

## 请求字段与实际传递范围

| 字段 | 类型 / 默认 | 引擎 B native | OpenAI 兼容 upstream |
| --- | --- | --- | --- |
| `model` | string；默认 `gemini-flash` | 模型 ID，按实时目录选择 RPC | 发送到上游 |
| `messages` | array；必填，至少一条 | 转为系统提示及文本历史，见后文 | 发送到上游，但消息只保留 role / content |
| `stream` | boolean；默认 false | 控制网关返回完整 JSON 或 SSE | 控制上游聊天流 |
| `temperature` | number；未填时由模型 / 协议默认配置决定 | 传入内部 Config；部分 Interaction RPC 不编码它 | 发送同名字段 |
| `top_p` | number；未填时使用默认配置 | 传入内部 Config；部分 Interaction RPC 不编码它 | 发送同名字段 |
| `max_tokens` | integer；未填时使用模型默认输出预算 | 转成 max_output_tokens，按模型范围校验 | 发送同名字段 |
| `user` | string | HTTP 可解析，但未用于生成请求或用户隔离 | 未发送到上游 |
| `image_size` | `512` / `1K` / `2K` / `4K`；K 必须大写 | 图片模型的输出边长。省略 `max_tokens` 时可能 HTTP 200 但没有图片，`2048` 已配合 `4K` 返回过图 | 未作为同名字段保证转发 |
| `resolution` | `360p` / `720p` / `1080p` / `4k`；`4k` 的 k 小写 | Omni 等视频聊天模型的输出分辨率，例如 `gemini-omni-1.1-flash` | 未作为同名字段保证转发 |

引擎 A 的原生网页模式把消息拼成文本提示词，不映射 temperature、top_p、max_tokens 或 user。引擎 A 的远程 upstream 模式使用上表的兼容上游传递规则。

除下表已列出的 `image_size` 和 `resolution` 外，未知顶层字段会被忽略。即使开启 `upstream` 和 `passthrough: true`，聊天入口仍重新组装以上固定字段，不能原样发送 tools、tool_choice、response_format、seed、stop、thinking_level、generation_config、previous_interaction_id、background 等扩展参数。专用媒体 / 文件路径的原样透传规则见[多模态透传](/api/passthrough)。聊天 JSON 读取上限为 8 MiB，不适合直接套用官方的大文件内联示例。

## 非流式请求

```bash
curl http://localhost:8800/v1/chat/completions \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [
      {"role": "system", "content": "回答简洁"},
      {"role": "user", "content": "什么是号池？"}
    ],
    "temperature": 0.7,
    "max_tokens": 512,
    "stream": false
  }'
```

响应包含 `id`、`object`、`created`、`model`、`choices` 和 `usage`，可以交给 OpenAI SDK 解析。正文只有字符串 content，媒体包装成 Markdown 链接。

`usage.prompt_tokens` 和 `completion_tokens` 根据输入 / 输出文本长度估算，`total_tokens` 为两者相加；这些数值不是 Google 返回的权威用量，不包含官方完整的思考、缓存、工具和分模态计费统计。`finish_reason` 当前固定为 stop，不能据此区分官方 incomplete、requires_action 等状态。

## SSE 流式请求

将 `stream` 设为 `true`，服务返回 `text/event-stream`：

```bash
curl -N http://localhost:8800/v1/chat/completions \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"写一首四句短诗"}],"stream":true}'
```

客户端按标准 SSE 读取 `data:` 事件，最后一个事件为 `data: [DONE]`。网络代理需要关闭响应缓冲，才能及时看到增量内容。

流中的分片使用 `choices[0].delta.content`。没有官方 event_type、event_id、step.index、thought signature 或 last_event_id 恢复机制。流开始后上游失败时，当前实现会输出 `[Error] ...` 文本并结束流，客户端应识别此情况；HTTP 200 只说明流已建立。流式分片没有权威 usage 汇总。

## 消息内容

请求可使用 `system`、`user`、`assistant` 和 `tool` 角色。当前原生适配将 system 作为系统提示词、assistant 作为历史回复，其他角色转为用户内容；工具调用并未完整映射。`content` 推荐使用字符串；结构化内容在原生模式中只提取 `type: text` 的文本段，图片、音频、视频和文档输入未映射。消息级 tool_calls、tool_call_id、name 等字段也没有进入 ChatMessage，不会因使用 upstream 聊天而自动保留。

多轮聊天由客户端在 messages 中携带历史，例如 user → assistant → user。官方 previous_interaction_id、Step[] 回放、thought signature 和 function_result 续接是另一种协议，当前没有对应入口。

原生模式生成图片、Omni 视频或音频时，在此接口选择对应模型。结果通过 `choices[0].message.content` 中的 Markdown 媒体链接返回，详见[图片生成](/api/images)与[音频生成](/api/audio)。

图片尺寸用顶层 `image_size`（`512`、`1K`、`2K`、`4K`，K 大写），并给出足够大的 `max_tokens`。Omni 视频分辨率用顶层 `resolution`（`360p`、`720p`、`1080p`、`4k`，k 小写），不要把 Veo 长任务的 `size` 用在这里。

```json
{
  "model": "gemini-omni-1.1-flash",
  "messages": [{"role": "user", "content": "a red circle moving once, 2 seconds"}],
  "resolution": "4k"
}
```

本机该请求返回 HTTP 200、`video/mp4`、h264、3840×2160，时长约 3 秒。
