# web2api — 多引擎号池管理网关（Go）

> 📚 **在线文档**：[项目介绍与 v1 API 文档](https://plunjoin.github.io/web2api/)

文档站点使用 [VitePress](https://vitepress.dev/) 构建，推送 `main` 分支后由 GitHub Pages 自动部署。想在本地预览：

```bash
cd docs
npm ci
npm run dev
```

**号池管理平台**（账号池 → Key 分发 → 调度 → 用量统计），对外输出统一
OpenAI 兼容 API。路由只有一条原则——**谁声明谁接**：模型出现在哪个账号的目录里，
请求就交给那个引擎；没有任何账号声明，直接返回 `404 model_not_found`
（错误里写明当前号池缺哪个模型）。不猜、不兜底、不跨引擎乱试。
详见[号池调度](#号池调度)。

- **Gemini Web（gemini.google.com 网页号）**：`gemini.google.com` 网页版纯 Go 原生逆向
  （Cookie 会话 + `__Secure-1PSIDTS` 自动轮换续命）
- **AI Studio（aistudio.google.com 号）**：`aistudio.google.com` 纯 Go 原生逆向
  ——私有协议 + **WAA（BotGuard）纯 Go VM**（内嵌 goja JS 引擎执行官方解释器
  生成 proof，零浏览器依赖）。native 模式会把上游返回的 inline data 转成 data URI
  随 chat 响应带回，图片/音频类输出不需要额外端点；upstream 模式与第三方 Key 只能出文本
- **订阅引擎（GPT / Antigravity / Kiro / WorkBuddy）**：把订阅制 AI 编程工具的
  账号接入同一号池，每个引擎独立账号池与凭据管理：
  - **GPT（codex）**：OpenAI ChatGPT 订阅号（OAuth 授权或 `~/.codex/auth.json` 导入，
    Responses 协议，gpt-5.x / gpt-6 系列）
  - **Antigravity**：Google Cloud AI Companion 订阅号（Google OAuth 授权，
    v1internal 协议，Claude + Gemini 双模型通道，project_id 自动 onboard）
  - **Kiro**：AWS CodeWhisperer 订阅号（`~/.aws/sso/cache/kiro-auth-token.json`
    的 refresh_token，Claude 模型，refresh_token 旋转自动回写；上游风控较严建议少量自用）
  - **WorkBuddy**：腾讯 CodeBuddy/WorkBuddy SaaS 号（管理台扫码授权或粘贴
    accessToken，OpenAI 协议透传）
- **第三方 Key（custom）**：管理台填 OpenAI 兼容的 Base URL + API Key
  （可点「获取模型列表」），多 Key 轮询。**只承接目录里明确声明了该模型的请求**：
  没有账号声明就 `404 model_not_found`，没拉到模型目录的账号也不再对任意模型开放
  （需在管理台拉目录或手填 `models`）。客户端仍用网关 `sk-` Key 调
  `/v1/chat/completions`。余额查询对齐官方供应商 + OpenAI 兼容中转级联探测
  （`/v1/usage` → `/user/balance` → NewAPI `/api/user/self`）；管理台展示剩余额度，
  约每 10 分钟自动刷新，点「立即检查」也会刷新

**端点按模型用途分流**（`kind` 优先取上游目录的能力标记，其次按模型名推断）：
对话走 `/v1/chat/completions`，视频走 `/v1/videos`，音乐走 `/v1/music/generations`，
图片/语音/向量走 `/v1/images|audio|embeddings` 透传。音乐/图片/语音的透传端点由上游实现，
账号上游不提供该路径时会原样返回上游的 404。音乐和视频模型误发到 chat 端点、
又没有能出媒体的引擎接单时，返回 `400 unsupported_endpoint` 并提示正确入口。

只讲 Responses 协议的客户端（如 Codex）可以直连 `POST /v1/responses`：网关把
`input` / `instructions` / `tools` 转成内部 Chat 请求执行，流式返回
`response.created` / `output_text.delta` / `response.completed` 事件并带 usage。

function calling 在 chat 与 responses 两个端点都可用：`tools` 与 `tool_choice` 会
下发给支持工具调用的上游（custom / AI Studio / GPT），模型发起的调用以
`delta.tool_calls`（chat）或 `function_call` 输出项（responses）返回，历史
`function_call` / `function_call_output` 也会还原成 `assistant.tool_calls` 与
`role=tool` 消息。Gemini Web 逆向、Antigravity、Kiro、WorkBuddy 这类没有函数调用
通道的上游会忽略 `tools`，仍以纯文本回答；非 function 类型的内置工具
（`local_shell`、`web_search`）由客户端自行执行，网关不下发。

```
                  浏览器打开 http://localhost:8800/admin
                          │ 管理台（邮箱密码登录 → JWT）
                          ▼
              ┌─────── 号池管理平台 ───────┐
              │  号池：加号/删号/启停/检测    │  ← SQLite 持久化
              │  Key：创建/启停/删除         │
              │  用户：注册/余额/兑换码/流水  │
              │  用量：按 Key/引擎/模型 统计  │
              └────────────┬──────────────┘
                           │ 调度（轮询/粘性/冷却/双额度通道）
客户端 ── /v1 ──────────────▼
(Base URL + sk-xxx)   web2api.exe 单二进制
                          ├── Gemini Web：gemini.google.com（Cookie 号池）
                          ├── AI Studio：aistudio.google.com（WAA 号池）
                          ├── 订阅引擎：GPT / Antigravity / Kiro / WorkBuddy
                          └── 第三方 Key：custom（base_url + api_key）
```

## 启动

```powershell
cd F:\project\john\web2api
# 1. 启动 Redis，按需编辑 config.yaml（监听地址、引擎等）
# 2. 直接运行源码（main.go 位于根目录）
go run . -config config.yaml
#    或先编译再运行（Windows）
go build -o web2api.exe .
.\web2api.exe -config config.yaml
#    Linux/macOS 输出文件名可用 web2api
# go build -o web2api .
# ./web2api -config config.yaml
#    已有 web2api.exe 时也可以双击 scripts\start.bat
# 3. 打开管理台
#    http://localhost:8800/admin   （首次设置邮箱、密码、昵称及 Redis 连接，之后邮箱密码登录）
#    用户控制台在 http://localhost:8800/console（登录页 /login，开放注册后可用 /register）
```

## 管理台与用户控制台

前端是 React + shadcn/ui 单页应用（深色、紧凑的 Linear 风格，中文界面），与网关打包在同一个二进制里：
`/admin` 管理台、`/console` 用户控制台、`/login` 与 `/register` 共用一个页面入口。

| 管理台页面 | 能力 |
|---|---|
| 概览 | 上游账号可用数、24h 请求与计费 Token、用户数与活跃度、用户余额合计、30 天充值/消耗、未使用兑换码、14 天趋势、模型与 Key 消耗排行 |
| 用户 | 搜索/筛选、点击查看详情（Key、最近流水、7 天用量）、新建用户、调整余额（必填备注）、修改角色与用户倍率、重置密码、停用、删除 |
| 兑换码 | 批量生成（面值/数量/批次/有效期/备注）、生成后一键复制或下载 CSV、按状态/批次筛选与导出、停用/整批停用/删除未使用的码、查看兑换人 |
| 余额流水 | 全部用户的余额变动（变动前后余额），按用户/类型/时间/关键词筛选，入账与出账合计 |
| 号池账号 | **运行时加号/删号/启停（不重启即生效）**：Gemini 号填 Cookie；AI Studio 粘贴 `storage-state.json`；订阅引擎 OAuth/粘贴 token；**第三方 Key** 填 Base URL + API Key；实时状态与健康检测 |
| API 密钥 | 管理员 Key 与用户 Key，**Token 额度、Key 倍率、过期时间、模型白名单、每分钟请求上限**、清零已用额度、重新生成 |
| 用量 | 按 Key × 模型汇总、趋势图（计费 Token/总 Token/请求）、逐请求明细与扣费计算过程、对用户请求退款、CSV 导出 |
| 模型与定价 | 模型目录、默认倍率 `*`、逐模型倍率行内编辑与恢复默认；**按引擎分组折叠**展示，勾选或整组**批量上下架**（含「全部开放/全部下架」一键操作，不可用时下游 `/v1/models` 不可见）；**模型检查**与上下架 |
| 设置 | 开放注册、注册赠送、新用户默认倍率、每人 Key 上限、视频计费、不计量接口开关、站点名与公告、版本与一键升级 |
| 接口文档 | 网关 API 与管理 API 的 OpenAPI 3.1 浏览、搜索与导出 |

| 用户控制台页面 | 能力 |
|---|---|
| 概览 | 余额、今日/7 天消耗、14 天趋势、模型分布、最近请求、接入示例、公告 |
| API 密钥 | 创建（可设额度上限）、复制、改名、停用、重新生成、删除 |
| 用量明细 / 账单 | 每次请求的 Token 与扣费过程；余额流水（充值、扣费、退款、调整） |
| 兑换充值 | 输入兑换码即时到账，查看兑换记录 |
| 模型与价格 / 账号设置 | 可用模型与对自己生效的倍率；昵称与密码 |

全局 <kbd>Ctrl</kbd>/<kbd>⌘</kbd>+<kbd>K</kbd> 打开命令面板（跳转页面、新建用户/Key/兑换码），表格行可点击查看详情，行尾菜单收纳所有操作，危险操作有确认弹窗。
用户、余额、兑换码的完整说明见 [docs/guide/platform.md](docs/guide/platform.md)。

**账号状态实时回报**：Gemini Web Cookie 校验/失效/配额；AI Studio 登录态、冷却、模型资格、
额度 Tier 均由上游协议实时同步到管理台。

## 号池调度

- **Gemini Web**：多号轮询 + 配额/认证异常自动切换下一个号
- **AI Studio**：轮询（`round-robin`）或会话粘性（`account-sticky`），单号并发槽位，
  Playground+Build 双额度通道自动降级，模型资格与冷却由池管理
- **模型路由**：`routing.engine_a/b_models` 显式指定 > 订阅/custom 目录按模型 id 精确命中 >
  Gemini Web / AI Studio 实时目录精确命中（裸名模型归属拥有它的引擎，如 `gemini-argon` → Gemini Web）>
  多模态关键词（image/veo/lyria…）走 AI Studio > `default_engine` > `auto`（AI Studio 优先，
  不可用自动降级 Gemini Web）。除 `default_engine` 与 `auto` 外，命中即「专属」：引擎不可用时
  直接返回该引擎的具体错误，不会被换到别的引擎；AI Studio 收到目录外模型时也不再静默换用
  目录第一个模型
- **按声明接单（不猜、不兜底）**：第三方 Key 号池只把请求交给目录里明确声明了该模型的
  账号；没有账号声明就返回 `404 model_not_found`（错误里写明当前号池缺哪个模型），
  不会再「随便抓一个就绪号」顶替。没拉到模型目录的账号不再对任意模型开放，
  需在管理台获取模型列表或手填 `models`
- **多模态不跨引擎**：音乐/视频/生图/语音模型即使目录里暂时没有，也只交给 AI Studio，
  AI Studio 不可用时返回它自己的真实错误（账号与 Worker 状态），不会改投第三方 Key
- **按用途选入口**：音乐/视频模型发到 `/v1/chat/completions` 时先过用途闸门——
  能出媒体的引擎（目前只有 AI Studio native）接单，否则直接 `400 unsupported_endpoint`
  并提示改用 `/v1/music/generations` 或 `/v1/videos`，不把上游的原始 400 透给下游
- **就绪口径**：`/v1/models` 的 `available` 要求「至少一个可用账号的目录里有它」且
  「归属引擎能接单」；AI Studio 的 `Ready()` 还会看 WAA Worker 是否真的起得来，
  账号 ready 但工人为 0 时不再谎报可用

## REST 管理 API（供脚本/二次开发）

首次启动后打开 `/admin`，填写管理员邮箱、密码、昵称和 Redis 连接 URL。密码至少 8 个字符、最多 72 字节，昵称最多 64 个字符。Redis 地址支持 `redis://[user:password@]host:port/db` 和 `rediss://`（TLS）；例如 `redis://127.0.0.1:6379/0` 或 `redis://:password@127.0.0.1:6379/0`，特殊字符请 URL 编码。初始化会验证 Redis 读写权限，成功后只能初始化一次。

先通过 `POST /admin/api/auth/login` 使用邮箱密码获取 `access_token`，管理接口使用 `Authorization: Bearer <JWT>`。JWT 有效期 8 小时，Redis 保存会话；退出后令牌立即失效。Redis 不可用时管理请求返回 503，恢复后可重试。连续登录尝试超过 10 次会限制登录 15 分钟。旧 `server.admin_token` 和 `X-Admin-Token` 已停用。已有数据库升级后会保留号池、Key 和用量，首次访问管理台完成管理员初始化。

```
GET    /admin/api/setup                            查询是否已初始化（公开）
POST   /admin/api/setup                            {email,password,nickname,redis_url}（仅首次）
POST   /admin/api/auth/login                       {email,password} → access_token（公开）
POST   /admin/api/auth/logout                      撤销当前 JWT 会话
GET    /admin/api/auth/me                          管理员邮箱和昵称
GET    /admin/api/overview                          总览统计
GET    /admin/api/accounts                         号池列表（含实时状态）
POST   /admin/api/accounts                         {engine: a|b, storage_state, label?, email?, locale?, timezone?, proxy?}
POST   /admin/api/accounts/gemini                  兼容入口（自动转换为统一协议）
POST   /admin/api/accounts/aistudio                兼容入口（自动转换为统一协议）
PATCH  /admin/api/accounts/{id}                    {enabled: bool}
PUT    /admin/api/accounts/{id}/credentials        更新 Gemini Cookie {psid, psidts}
POST   /admin/api/accounts/{id}/check              触发健康检测
DELETE /admin/api/accounts/{id}                     删除号（出池+删库+删凭据）
GET    /admin/api/keys                             Key 列表（含额度、倍率、限制与 24h 用量）
POST   /admin/api/keys                             {name, token_limit?, multiplier?, expires_at?, allowed_models?, rpm_limit?} → 生成 sk- Key
PATCH  /admin/api/keys/{id}                        任意组合 {enabled, name, token_limit, multiplier, reset_usage, expires_at, allowed_models, rpm_limit}
POST   /admin/api/keys/{id}/regenerate             换发新密钥（旧密钥立即失效，设置保留）
DELETE /admin/api/keys/{id}
GET    /admin/api/usage?days=7&key_id&model        用量聚合（最大 90 天）：usage（旧口径）+ breakdown + totals
GET    /admin/api/usage/records?days&key_id&model&limit&offset   逐请求明细（每页最多 500）
GET    /admin/api/usage/timeseries?days&key_id&model             时间序列（days=1 按小时，否则按天）
GET    /admin/api/usage/export.csv?days&key_id&model             导出明细 CSV（Key 脱敏）
GET    /admin/api/multipliers                      模型倍率列表、默认倍率、可选模型、计费公式
PUT    /admin/api/multipliers                      {model, multiplier}（model 为 * 时设置默认倍率）
DELETE /admin/api/multipliers/{model}              删除单独倍率，回落到默认
GET    /admin/api/models                           模型目录（含生效倍率）
POST   /admin/api/models/batch                     批量上下架 {models, enabled}（去重，单次最多 500）
GET    /admin/api/version                          版本、提交号、Go 版本
GET    /admin/api/status                           引擎详细状态
GET    /admin/api/docs                             OpenAPI 3.1 JSON（可导入 Postman/Insomnia）
# 用户与余额（需要 admin 角色，完整说明见 docs/guide/platform.md）
GET/POST            /admin/api/users                     用户列表 / 新建
GET/PATCH/DELETE    /admin/api/users/{id}                详情 / 修改角色、启停、倍率 / 删除
POST                /admin/api/users/{id}/balance        {amount, note} 调整余额（写流水）
POST                /admin/api/users/{id}/password       重置密码
GET                 /admin/api/ledger                    余额流水
POST                /admin/api/usage/records/{id}/refund 对一次请求退款
GET/POST            /admin/api/redeem-codes              兑换码列表 / 批量生成
PATCH/DELETE        /admin/api/redeem-codes/{id}         启停 / 删除（仅未兑换）
GET                 /admin/api/redeem-codes/export.csv   导出
GET/PUT             /admin/api/settings                  平台设置（开放注册、赠送、默认倍率等）
```

`/admin/api/auth/login` 与 `/api/auth/login` 是同一个登录接口：root 管理员和平台用户都用邮箱密码登录。

### Token 额度与倍率

- 每次 `/v1/chat/completions` 请求记录一条明细：`prompt_tokens`、`completion_tokens`、`total_tokens`。
  AI Studio 和 upstream 引擎返回的真实用量直接采用；拿不到时（Gemini Web、流式响应等）按文本长度估算，
  明细中 `estimated: true`，管理台显示「估算」。
- 计费：`charged_tokens = ⌈total_tokens × 模型倍率 × Key 倍率 × 用户倍率⌉`。模型倍率按「精确模型 → `*` 默认 → 1」查找；
  Key 倍率即分组默认倍率（默认 1）；用户倍率只对用户自己创建的 Key 生效（管理员 Key 恒为 1）。失败请求不计费。
- 用户 Key 的扣费在写用量记录的同一个事务里从用户余额扣除，并写一条余额流水；余额 ≤ 0 后返回
  `402 {"error":{"type":"insufficient_quota","code":"insufficient_balance"}}`。管理员 Key 不涉及余额，行为不变。
- Key 的 `token_limit > 0` 时，`tokens_used ≥ token_limit` 的新请求返回
  `429 {"error":{"type":"insufficient_quota","code":"token_quota_exceeded",...}}`，并附带
  `X-Web2api-Token-Limit` / `X-Web2api-Tokens-Used` 响应头。扣减发生在请求完成后，所以越线的那次请求（以及并发请求）可能略超额度。
- 其他限制：过期 `401 key_expired`；模型不在白名单 `403 model_not_allowed`（聊天、视频、Gemini 原生路径中的 `models/{id}` 与 JSON 体 `model` 都会检查）；
  超过每分钟上限 `429 rpm_limit_exceeded`（含 `Retry-After`）。
- 只有 `/v1/chat/completions` 计量 Token；`/v1/videos`、多模态透传与 Gemini 原生路由受 Key 的启停、过期、额度、白名单与 RPM 约束，但不产生 Token 明细。

### 前端构建

前端源码在 `web/`（Vite + React 19 + TypeScript + Tailwind CSS v4 + shadcn/ui），`npm run build` 输出到
`internal/webui/dist/`，由 `//go:embed` 内嵌进二进制，不访问任何 CDN。**构建产物已提交到仓库，只运行 `go build` 即可，无需 Node。**
修改前端后：

```bash
cd web
npm ci
npm run dev        # 本地开发：http://localhost:5173/login，接口代理到 127.0.0.1:8800
npm run typecheck
npm run build      # 输出到 ../internal/webui/dist，然后提交 dist 的变化
```

Docker 镜像构建时会在 Node 阶段重新执行 `npm ci && npm run build`，再由 Go 阶段内嵌，所以镜像总是使用与源码一致的前端。
构建产物文件名带内容哈希（如 `index-DTrA9p-0.js`），网关对其返回 `immutable` 长缓存；入口页面 `no-store`，升级后不会混用旧资源。

管理台的「接口文档」页会读取同一份 OpenAPI 文档，也可以直接下载
`/admin/api/docs` 的 JSON 文件。管理接口统一返回 JSON，失败响应示例：

```json
{"error":"JWT 无效或已过期"}
```

### 调用示例

```bash
# 登录获取 JWT（将响应中的 access_token 设置为 ADMIN_JWT）
curl -X POST http://localhost:8800/admin/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"你的密码"}'

# 查看号池总览
curl http://localhost:8800/admin/api/overview \
  -H "Authorization: Bearer $ADMIN_JWT"

# 创建客户端 Key
curl -X POST http://localhost:8800/admin/api/keys \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"生产客户端"}'

# 拉取最近 30 天用量
curl "http://localhost:8800/admin/api/usage?days=30" \
  -H "Authorization: Bearer $ADMIN_JWT"

# 创建限额 Key：100 万计费 Token、0.5 倍、30 天后过期、只允许 gemini-3.5-* 模型、每分钟 60 次
curl -X POST http://localhost:8800/admin/api/keys \
  -H "Authorization: Bearer $ADMIN_JWT" -H "Content-Type: application/json" \
  -d "{\"name\":\"团队A\",\"token_limit\":1000000,\"multiplier\":0.5,\"expires_at\":$(( $(date +%s) + 30*86400 )),\"allowed_models\":[\"gemini-3.5-*\"],\"rpm_limit\":60}"

# 设置模型倍率（* 为默认倍率）
curl -X PUT http://localhost:8800/admin/api/multipliers \
  -H "Authorization: Bearer $ADMIN_JWT" -H "Content-Type: application/json" \
  -d '{"model":"gemini-3.1-pro-preview","multiplier":4}'

# 重置某个 Key 的已用额度
curl -X PATCH http://localhost:8800/admin/api/keys/1 \
  -H "Authorization: Bearer $ADMIN_JWT" -H "Content-Type: application/json" \
  -d '{"reset_usage":true}'
```

## 对外 API（客户端接入）

官方 Gemini 全参数调用：设置服务器环境变量 `WEB2API_GEMINI_API_KEY`，客户端将 Google SDK 的 Base URL 设为 `http://localhost:8800/gemini`，apiKey 填管理台创建的 `sk-` Key。Interactions、agent、沙箱、工具、后台任务、SSE 恢复及 Files 上传使用官方后端；详细配置和示例见[官方后端接入](docs/api/gemini-official.md)。原有网页登录态账号池继续通过下面的 OpenAI 兼容接口调用。

| 配置项 | 值 |
|---|---|
| Base URL | `http://localhost:8800/v1` |
| API Key | 管理台创建的任意 `sk-` Key |
| 文本模型 | `gemini-flash` / `gemini-pro`（Gemini Web），`/v1/models` 实时聚合所有引擎与账号目录 |

完整对外接口文档：`GET /v1/docs`（OpenAPI 3.1，可导入 Postman/Insomnia）。

Gemini 官方参数审计（2026-10-07）：[参数映射与实现遗漏](docs/api/gemini.md)、[官方全部字段对照](docs/api/gemini-schema.md)、[中文参考详解](docs/api/gemini-reference.md)。完整表由保存的官方 OpenAPI 自动生成，包含请求、响应、工具、agent、沙箱和 SSE 的深层字段。新增[官方 Gemini 后端](docs/api/gemini-official.md)，启用后官方 Interactions 参数和 SSE 完整转发；网页登录态聊天的字段限制单独标注。

生成接口的模式选择、参数、cURL 与响应读取示例见[视频生成](docs/api/videos.md)、[图片生成](docs/api/images.md)、[音频生成](docs/api/audio.md)。默认 native 模式中，图片和音频通过 `/v1/chat/completions` 返回媒体链接。图片尺寸是顶层 `image_size`（`4K`，K 大写），Omni 视频分辨率是顶层 `resolution`（`4k`，k 小写）。Veo 用 `POST /v1/videos` 的 `size`，`veo-3.1-fast-generate-preview` 的 4K 需要 `seconds: 8`。`/v1/images/generations`、`/v1/audio/speech` 与 `/v1/music/generations` 走透传，由上游实现。**音乐/视频模型不能走 `/v1/chat/completions`**（AI Studio native 除外，它能把媒体塞进 chat 响应），误发会返回 `400 unsupported_endpoint`。
端点：`POST /v1/chat/completions`（SSE 流式 + 非流式）、`GET /v1/models`、
`GET /health`、`GET /v1/accounts`。Veo 使用独立长任务接口：
`POST /v1/videos` 创建、`GET /v1/videos/{id}` 轮询、
`GET /v1/videos/{id}/content` 下载完成的视频；请求会走 AI Studio 的
`GenerateVideo` / `GetGenerateVideoOperation` 协议。多模态透传
（`/v1/images|videos|audio|music|files|embeddings`）按模型路由：请求体模型命中
custom（第三方 Key）目录时转发对应上游（native 模式即可用，如 Google
OpenAI 兼容端的 `gemini-embedding-*` 向量模型）；未命中时回退 AI Studio
upstream 模式透传。音乐模型（如 `lyria-3.5`）请走 `POST /v1/music/generations`。

### 一键导入 CC Switch（Codex / Claude Code）

管理台/控制台的 API 密钥页提供「导入 CC Switch」：生成 `ccswitch://` Deep Link
（CC Switch v4+），一键把该密钥导入为 Codex 或 Claude Code 供应商。配置要点：

- **模型列表随导入写入**：Deep Link 的 `config` 载荷里带 `modelCatalog`
  （取自 `GET /v1/codex/catalog`，已按该 Key 的白名单与上下架过滤），
  导入后 CC Switch 供应商的模型列表就是这些模型。`ccswitch://` 协议本身
  只有单数 `model` 参数，没有模型数组字段，列表只能这样带过去
- `wire_api = "chat"` / `apiFormat = openai_chat`：CC Switch 会自动开启本地路由，
  把 Codex 的 Responses、Claude Code 的 Anthropic Messages 在本地翻译成
  Chat Completions 再进网关
- **也可以直连，不走本地路由**：网关已提供 `POST /v1/responses`，Codex 直接填
  `base_url = "<网关>/v1"` + `wire_api = "responses"` 即可，链路里少一层转换。
  直连同样支持工具调用，Codex 的 shell / apply_patch 能正常跑；前提是该模型
  落到支持 function calling 的上游（custom、AI Studio、GPT）
- `model_catalog_url` 指向 `GET /v1/codex/catalog`（`Bearer` 或 `?key=` 鉴权）：
  Codex 从该 URL 动态拉取模型目录，号池变化后无需重改配置
- 目录条目与 `cc-switch-model-catalog.json` / 官方 `models_cache.json` 同构
  （`visibility` 枚举只有 `list/hide/none`，`support_verbosity` 等为必填字段）
- Claude Code 注意：网关没有 Anthropic `/v1/messages` 端点，不能把
  `ANTHROPIC_BASE_URL` 直接指向网关，必须经 CC Switch 本地路由转换

### 本机一键导入 AI Studio 登录态

用户可以在自己的电脑上打开可见浏览器完成登录，脚本会把登录态通过管理员 API
提交到后端；后端会写入 `auth/<邮箱>/storage-state.json` 并立即加入账号池。电脑上需要
Node.js 18+、npm 和 curl（建议使用 HTTPS 后端地址）：

```bash
curl -fsSL https://你的VPS地址:8800/tools/export-storage.sh -o /tmp/web2api-export.sh
sh /tmp/web2api-export.sh
```

脚本会提示后端地址、管理员 JWT 和 AI Studio 邮箱，随后打开本机 Chrome。完成登录后按
Enter，账号会自动提交到 `/admin/api/accounts`。脚本只在本机临时目录安装 Playwright，
结束后删除临时目录；不会把 JWT 放进 URL。Windows 请在 WSL 或 Git Bash 中运行。

### 管理台一键升级镜像

Docker 部署可启用 `docker-compose.upgrade.yml`，之后在管理台「系统升级」检查并下载最新镜像，一键替换容器。重启期间面板自动重连，启动失败时恢复旧容器，保留现有配置和数据卷。首次启用步骤及私有仓库配置见 [Docker 部署文档](docker/README.md#面板一键升级)。

## 账号从哪来

**统一账号协议**：所有新账号都提交 AI Studio 协议的 `storage-state.json` 到
`POST /admin/api/accounts`。`engine=b` 直接进入 AI Studio 号池；`engine=a` 会从同一份
状态中提取 `__Secure-1PSID` / `__Secure-1PSIDTS`，进入 Gemini 号池。旧的两个子路径仍可用，
会自动转换后走统一流程。

**AI Studio 号**：三选一——
1. 管理台粘贴 `storage-state.json` 内容（浏览器插件导出）；
2. 把已有的 `auth/<邮箱>/` 目录整个放到 web2api 目录下，启动时自动扫描入池并登记；
3. 配置 `auth_states` 指向已有的 auth 目录，或改用 `mode: upstream` 直接转发。

## 存储

SQLite（`data/web2api.db`，纯 Go 驱动无 cgo）：`accounts`（号池凭据与状态）、
`api_keys`（Key 分发）、`usage_log`（用量明细）、`cookie_sessions`（完整 Cookie 会话）、`gemini_upload_sessions`（官方断点上传会话）、`admin_settings`（管理员邮箱、昵称、bcrypt 密码哈希、Redis URL 和随机 JWT 签名密钥）。Redis 存储带过期时间的管理员登录会话与登录尝试计数，不保存业务数据。备份 SQLite 时请停止服务后拷贝 `data/` 目录；Redis 会话丢失时重新登录即可。Redis URL 可能含密码，数据库需要妥善保管；敏感目录 `data/`、`auth/` 勿提交 Git。

## 打包与部署

仓库提供跨平台发布脚本（位于 `scripts/`），默认一次生成 Windows（amd64/arm64）、Linux（amd64/arm64/armv7）和 macOS（Intel/Apple Silicon）共 7 个发布包。详细的产物结构、Windows/Linux/macOS/Docker 部署步骤见 [deploy/README.md](deploy/README.md)。

在仓库根目录执行：

```powershell
# Windows PowerShell：Windows 包为 zip，其余目标为 tar.gz
.\scripts\build.ps1 -Version v1.0.0
```

```bash
# Linux/macOS/WSL：所有目标生成 tar.gz
chmod +x scripts/build.sh
./scripts/build.sh v1.0.0
```

产物写入 `dist/`，并生成 `SHA256SUMS`。发布包只带示例配置（Linux 包另带 `/opt/web2api` 路径模板），不会带本机的数据库、Cookie 或 auth 凭据。

## 测试与构建

前端已构建并内嵌在程序中，运行时不需要访问 CDN，普通 Go 构建不需要 Node.js。修改 Go 或前端源码后，
先停止正在运行的旧程序，再执行下面的构建命令并重新启动。修改 `web/` 下的前端时，先执行
`npm ci --prefix web && npm run typecheck --prefix web && npm run build --prefix web` 更新 `internal/webui/dist`。

```powershell
go vet ./...
go build -o web2api.exe .
```

测试用例不随仓库分发，本地开发时自建 `test/` 目录跑 `go test ./...` 即可（该目录已在 `.gitignore` 中）。

## 目录结构

```
main.go                     入口（go run . / go build .）
internal/geminiapi/          官方 Gemini 后端（全参数 / SSE / 上传）
internal/store/              SQLite：账号 / Key / 用量 / 用户 / 余额流水 / 兑换码
internal/config/             YAML 配置 + 环境变量
internal/model/              OpenAI 协议数据结构
internal/engine/             引擎接口定义
internal/engine/geminiweb/   Gemini Web：gemini 网页协议（请求/流式/Cookie/模型）
internal/engine/upstream/    OpenAI 兼容上游（upstream 模式 AI Studio）
internal/provider/           多引擎装配 + 动态号池 + 调度路由（manager.go）
internal/gateway/            OpenAI 网关（动态 Key 鉴权 + 用量埋点）
internal/session/            Redis 会话通信（支持 ACL、数据库和 TLS）
internal/admin/              JWT 登录、管理 REST API（/admin/api/*）与用户 API（/api/*）
internal/webui/              内嵌前端（/admin、/console、/login、/register）
web/                         前端源码（React + shadcn/ui，构建到 internal/webui/dist）
internal/limiter/           令牌桶限流
internal/aistudio/          AI Studio 协议内核（AI Studio 私有协议 + 纯 Go Worker，MIT）
internal/aistudio/waa/      BotGuard（WAA）纯 Go VM，内嵌 goja 执行官方解释器
internal/aistudio/fingerprint/  账户浏览器指纹生成
```