# 模型与状态

## `GET /v1/models`

返回当前账号池聚合出的模型目录：

```bash
curl http://localhost:8800/v1/models \
  -H 'Authorization: Bearer sk-xxxxxxxx'
```

响应示例：

```json
{
  "object": "list",
  "data": [
    {
      "id": "gemini-2.5-flash",
      "object": "model",
      "display_name": "Gemini 2.5 Flash",
      "available": true,
      "engine": "a"
    }
  ]
}
```

`available` 表示当前至少有一个可用账号，`engine` 为 `a`（Gemini 网页）或 `b`（AI Studio）。模型目录会在账号状态变化后刷新。

## `GET /health`

健康检查不需要 Key：

```json
{
  "status": "ok",
  "uptime": "2h31m",
  "engine_a": true,
  "engine_b": true
}
```

## `GET /v1/accounts`

需要 Key，返回引擎和账号池的汇总状态，适合运维面板查看。它不会返回账号 Cookie 或登录凭据。
