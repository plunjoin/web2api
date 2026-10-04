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

支持 `system`、`user`、`assistant` 和 `tool` 角色。`content` 可以是字符串，也可以按 OpenAI 多模态格式传递结构化内容；需要引擎 B 的图片能力时，优先使用[多模态透传](/api/passthrough)。
