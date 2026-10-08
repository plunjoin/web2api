# 图片生成

先确认引擎 B 的模式。默认 `native` 模式使用聊天接口；`/v1/images/generations` 需要 `upstream` 模式、开启透传，并且上游实现该接口。

| 模式 | 请求接口 | 结果位置 |
| --- | --- | --- |
| `native` | `POST /v1/chat/completions` | `choices[0].message.content` 中的媒体链接 |
| `upstream` + `passthrough: true` | `POST /v1/images/generations` | 上游响应，常见 `data[].url` 或 `data[].b64_json` |

## 原生模式：文本生成图片

先调用 `GET /v1/models`，确认图片模型 ID。以下模型仅为示例，需要出现在当前模型目录中，并且账户具备生成资格和可用额度。

```bash
curl http://localhost:8800/v1/chat/completions \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "gemini-3.1-flash-image",
    "messages": [{"role": "user", "content": "生成一张被浓雾笼罩的森林图片，气氛阴森"}],
    "stream": false
  }' \
  -o image-response.json
```

必填 `messages`，其中的用户文本描述图片；应显式指定图片 `model`。`stream: false` 返回完整聊天 JSON。输出结构如下，示例省略了实际 Base64 数据：

```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "\n![media](data:image/png;base64,<图片Base64>)\n"
    }
  }]
}
```

提取 Markdown 括号内的链接。若是 `data:` URI，逗号前是 MIME 等信息，逗号后是 Base64 数据，解码后保存；若是 URL，则读取对应媒体。响应可能同时包含文本和多张图片，需要逐一提取。

当前原生 HTTP 聊天适配器接收 `model`、`messages`、`stream`、`temperature`、`top_p`、`max_tokens`、`user`，以及图片尺寸 `image_size`。输出模态由模型目录自动选择。`size`、`n`、`image_config`、`response_modalities` 等额外字段不会设置图片参数；尺寸只用顶层 `image_size`，不能用 OpenAI 的 `size` 代替。原生消息只提取文本，当前不能通过 `image_url` 传入参考图做编辑。

这里的 `user` 仅被 HTTP 解析，未进入生成协议；采样参数也不保证每个私有 RPC 都会编码。

### 指定输出尺寸

`image_size` 放在聊天 JSON 顶层，取值 `512`、`1K`、`2K`、`4K`。K 必须大写，`4k` 会被拒绝。图片请求还需要足够大的 `max_tokens`：省略它时，本机对 `gemini-nano-banana-2.1` 的两次调用都是 HTTP 200 且正文为空；带 `max_tokens: 2048` 和 `image_size: "4K"` 的一次返回 HTTP 200、`image/jpeg`、5504×3072。

```json
{
  "model": "gemini-nano-banana-2.1",
  "messages": [{"role": "user", "content": "draw a simple red circle"}],
  "image_size": "4K",
  "max_tokens": 2048
}
```

## Gemini 官方图像参数对照

官方新 Interactions 请求使用 `response_format: {type: "image", aspect_ratio, image_size, mime_type, delivery}`，可与 text 分支组成数组。旧 `generation_config.image_config` 和 `response_modalities` 已废弃。官方 `response_format` 对象仍不能直接放进网关聊天 JSON。尺寸请用顶层 `image_size`。

| 官方参数 | 允许值 / 含义 | 网关现状 |
| --- | --- | --- |
| `aspect_ratio` | 1:1、2:3、3:2、3:4、4:3、4:5、5:4、9:16、16:9、21:9、1:8、8:1、1:4、4:1；具体模型取子集 | HTTP 未开放比例设置 |
| `image_size` | 512、1K、2K、4K，K 大写；具体模型取子集 | 聊天顶层 `image_size` 已接入。不是 `response_format.image_size`，也不能用 OpenAI `size` 代替。省略 `max_tokens` 时正文可能为空，2048 已出过图 |
| `mime_type` | 参考枚举仅 image/jpeg，指南的 image/png 输出示例存在冲突 | 按实际返回的 MIME 保存，不保证指定编码 |
| `delivery` | inline / uri | 不开放选择；把上游媒体转成 Markdown 链接 |
| `input` 的 image / video / document 块 | data 裸 base64 或 uri，按模型支持参考图 / 视频 / PDF | 原生聊天未映射媒体输入，不能据此启用编辑 |
| `generation_config.thinking_level` | 模型支持的思考等级 | HTTP 未开放控制；不同图像模型支持范围不同 |

Nano Banana 新 ID、各模型思考等级、参考图数量和视频生图限制见[中文参考详解](/api/gemini-reference)；全部字段和官方枚举见[完整对照](/api/gemini-schema)。新官方模型 ID 只有进入账号的实时模型目录后，才可能通过网关使用。

若使用 `stream: true`，媒体链接会通过 SSE 的 `choices[0].delta.content` 返回；先拼接分片，再提取完整链接。

## upstream 模式：图片专用接口

配置条件：

```yaml
engine_b:
  enabled: true
  mode: upstream
  base_url: http://127.0.0.1:2048
  api_key: 上游服务的Key
  passthrough: true
```

下面是常见 OpenAI 兼容协议示例。先替换模型占位值，确认上游支持相应字段；网关原样转发请求体，参数范围和默认值由上游定义。

```bash
curl http://localhost:8800/v1/images/generations \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "<上游图片模型ID>",
    "prompt": "被浓雾笼罩的森林",
    "n": 1,
    "size": "1024x1024",
    "response_format": "b64_json"
  }'
```

| 字段 | 常见用途 | 支持条件 |
| --- | --- | --- |
| `model` | 上游图片模型 ID | 由上游提供 |
| `prompt` | 图片描述 | 常见协议必填 |
| `n` | 生成数量 | 示例为 1，上游决定范围 |
| `size` | 图片尺寸 | 示例为 `1024x1024`，上游决定可选值 |
| `response_format` | `url` 或 `b64_json` | 仅上游支持时有效 |

常见响应为 `{"data":[{"b64_json":"..."}]}` 或 `{"data":[{"url":"https://..."}]}`。前者需解码后保存，后者使用返回 URL；实际响应格式以对应上游为准。

在 `native` 模式调用图片专用透传路径会返回 `502`。模型可见、聊天正常均不能保证图片生成有剩余额度。
