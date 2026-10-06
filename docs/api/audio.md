# 音频生成

| 用途 | 模式与接口 | 输出 |
| --- | --- | --- |
| 原生 TTS/音频模型生成 | 引擎 B `native`，`POST /v1/chat/completions` | 聊天 JSON 中的 Markdown 媒体链接 |
| 文本转语音专用接口 | 引擎 B `upstream` + `passthrough: true`，`POST /v1/audio/speech` | 上游音频二进制，格式由上游决定 |

`/v1/audio/speech` 是文本转语音接口。音乐生成、实时语音和音频转写是不同能力，不能默认套用同一请求。

## 原生模式：请求音频模型

先用 `GET /v1/models` 选择实际可用的 TTS/音频模型。以下 ID 仅为示例，需出现在当前目录中；底层根据模型目录选择 `AUDIO` 输出，具体模型是否接受该请求由上游决定。

```bash
curl http://localhost:8800/v1/chat/completions \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "gemini-2.5-flash-preview-tts",
    "messages": [{"role": "user", "content": "请用平静的中文声音朗读：欢迎来到雾中的森林。"}],
    "stream": false
  }' \
  -o audio-response.json
```

输出位于 `choices[0].message.content`，例如：

```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "\n![media](data:audio/pcm;rate=24000;base64,<音频Base64>)\n"
    }
  }]
}
```

实际 MIME 和媒体格式由上游返回，也可能是媒体 URL。提取 `data:` URI 后，按第一个逗号分隔元信息与 Base64 数据，再解码保存。解码不会转换格式：`audio/pcm` 是原始采样数据，需要确认采样率、声道数和采样格式后封装成 WAV 或转换，不能只改文件名为 MP3。`audio/wav` 等带容器格式可按实际类型保存。

当前原生聊天适配器未映射 `voice`、`speech_config`、`response_format`、`speed`、`response_modalities`，传入这些字段不会设置声音或输出编码；`input_audio` 等音频输入也未映射。如需指定声音及输出格式，使用支持这些参数的上游专用接口。选择原生音乐模型时，使用该模型支持的文本描述，结果同样从媒体链接读取；是否支持以模型和上游为准。

`stream: true` 时先拼接 SSE 的 `choices[0].delta.content`，再提取媒体链接；这不是独立的实时音频会话协议。

## upstream 模式：文本转语音

需要 `engine_b.enabled: true`、`engine_b.mode: upstream`、`engine_b.passthrough: true`，且配置的上游实现 `/v1/audio/speech`。

下面是常见 OpenAI 兼容协议示例。替换模型和声音占位值，确认上游支持输出格式后使用。

```bash
curl http://localhost:8800/v1/audio/speech \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "<上游语音模型ID>",
    "input": "欢迎来到雾中的森林。",
    "voice": "<上游声音名称>",
    "response_format": "mp3"
  }' \
  -o speech.mp3
```

| 字段 | 常见用途 | 支持条件 |
| --- | --- | --- |
| `model` | 上游语音模型 ID | 由上游提供 |
| `input` | 需要朗读的文本 | 常见协议必填 |
| `voice` | 声音名称 | 由上游提供，不能通用套用 |
| `response_format` | 输出编码，如 `mp3`、`wav`、`pcm` | 上游决定支持范围和默认值 |
| `speed` | 语速，如 `1` | 仅上游支持时有效 |

网关原样转发请求体并保留上游状态码、响应头和正文。成功响应通常为音频二进制，可直接保存；失败可能是 JSON 错误，下载后应检查 HTTP 状态与 `Content-Type`，避免把错误正文当作音频播放。

在 `native` 模式调用 `/v1/audio/speech` 会返回 `502`；若已使用 upstream 透传但请求失败，则检查对应上游的端点、模型、声音参数和额度。
