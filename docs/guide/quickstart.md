# 快速开始

## 1. 启动服务

### Docker Compose

```bash
git clone https://github.com/plunjoin/web2api.git
cd web2api
docker compose up -d
```

### 本地运行

```bash
go run ./cmd/web2api -config config.yaml
```

服务默认监听 `http://localhost:8800`。第一次访问 `http://localhost:8800/admin`，设置管理员邮箱、密码、昵称和 Redis 连接地址。

## 2. 添加账号并创建 Key

在管理台的「号池管理」添加 Gemini 或 AI Studio 账号。然后进入「Key 管理」创建客户端 Key。Key 只在创建时完整显示，请立即保存。

没有创建任何 Key 时，开发环境会进入无鉴权模式；生产环境应始终创建并启用 Key。

## 3. 验证 API

```bash
export WEB2API_URL=http://localhost:8800
export WEB2API_KEY=sk-你的Key

curl "$WEB2API_URL/health"
curl "$WEB2API_URL/v1/models" \
  -H "Authorization: Bearer $WEB2API_KEY"

curl "$WEB2API_URL/v1/chat/completions" \
  -H "Authorization: Bearer $WEB2API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [{"role": "user", "content": "用一句话介绍 web2api"}]
  }'
```

## 4. 接入现有 OpenAI SDK

只需替换 `base_url` 和 `api_key`。接口路径已经包含 `/v1`，不要重复添加一次。

```javascript
import OpenAI from 'openai'

const client = new OpenAI({
  apiKey: process.env.WEB2API_KEY,
  baseURL: 'http://localhost:8800/v1',
})

const result = await client.chat.completions.create({
  model: 'gemini-2.5-flash',
  messages: [{ role: 'user', content: 'Hello' }],
})
console.log(result.choices[0].message.content)
```

## 下一步

- 了解鉴权规则：[认证与 Key](/guide/authentication)
- 查看可用模型：[模型与状态](/api/models)
- 接入流式输出：[聊天补全](/api/chat)
- 导入完整 OpenAPI：访问服务的 `/v1/docs`
