# 聊天补全

`POST /v1/chat/completions` 兼容 OpenAI Chat Completions 请求。`model` 和 `messages` 必填，其余参数按上游模型支持情况传递。

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

响应包含 `id`、`object`、`created`、`model`、`choices` 和 `usage`，可以直接交给 OpenAI SDK 解析。

## SSE 流式请求

将 `stream` 设为 `true`，服务返回 `text/event-stream`：

```bash
curl -N http://localhost:8800/v1/chat/completions \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"写一首四句短诗"}],"stream":true}'
```

客户端按标准 SSE 读取 `data:` 事件，最后一个事件为 `data: [DONE]`。网络代理需要关闭响应缓冲，才能及时看到增量内容。

## 消息内容

请求可使用 `system`、`user`、`assistant` 和 `tool` 角色。当前原生适配将 system 作为系统提示词、assistant 作为历史回复，其他角色转为用户内容；工具调用并未完整映射。`content` 推荐使用字符串；结构化内容在原生模式中只提取 `type: text` 的文本段，图片和音频输入未映射。

原生模式生成图片或音频时，在此接口选择对应的图片/TTS/音频模型。结果通过 `choices[0].message.content` 中的 Markdown 媒体链接返回，详见[图片生成](/api/images)与[音频生成](/api/audio)。
