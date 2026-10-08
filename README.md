# web2api — Gemini 双引擎号池管理网关（Go）

> 📚 **在线文档**：[项目介绍与 v1 API 文档](https://plunjoin.github.io/web2api/)

文档站点使用 [VitePress](https://vitepress.dev/) 构建，推送 `main` 分支后由 GitHub Pages 自动部署。想在本地预览：

```bash
cd docs
npm ci
npm run dev
```

把 Google 两个免费入口做成一个**号池管理平台**（架构参考 sub2api：账号池 →
Key 分发 → 调度 → 用量统计），对外输出统一 OpenAI 兼容 API：

- **引擎A（Gemini 网页号）**：`gemini.google.com` 网页版纯 Go 原生逆向
  （Cookie 会话 + `__Secure-1PSIDTS` 自动轮换续命）
- **引擎B（AI Studio 号）**：`aistudio.google.com` 纯 Go 原生逆向
  ——私有协议 + **WAA（BotGuard）纯 Go VM**（内嵌 goja JS 引擎执行官方解释器
  生成 proof，零浏览器依赖），协议内核已合并自
  [Mag1cFall/AIStudio2API](https://github.com/Mag1cFall/AIStudio2API)（MIT）

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
                          ├── 引擎A：gemini.google.com（Cookie 号池）
                          └── 引擎B：aistudio.google.com（WAA 号池）
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
#    已有 web2api.exe 时也可以双击 start.bat
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
| 号池账号 | **运行时加号/删号/启停（不重启即生效）**：Gemini 号填 `__Secure-1PSID`/`__Secure-1PSIDTS`；AI Studio 号粘贴 `storage-state.json`；实时状态、换 Cookie、健康检测 |
| API 密钥 | 管理员 Key 与用户 Key，**Token 额度、Key 倍率、过期时间、模型白名单、每分钟请求上限**、清零已用额度、重新生成 |
| 用量 | 按 Key × 模型汇总、趋势图（计费 Token/总 Token/请求）、逐请求明细与扣费计算过程、对用户请求退款、CSV 导出 |
| 模型与定价 | 模型目录、默认倍率 `*`、逐模型倍率行内编辑与恢复默认 |
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

**账号状态实时回报**：引擎A Cookie 校验/失效/配额；引擎B 登录态、冷却、模型资格、
额度 Tier 均由上游协议实时同步到管理台。

## 号池调度

- **引擎A**：多号轮询 + 配额/认证异常自动切换下一个号
- **引擎B**：轮询（`round-robin`）或会话粘性（`account-sticky`），单号并发槽位，
  Playground+Build 双额度通道自动降级，模型资格与冷却由池管理
- **模型路由**：`routing.engine_a/b_models` 显式指定 > 多模态关键词（image/veo/lyria…）走
  引擎B > `auto`（引擎B 优先，不可用自动降级引擎A）

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
  引擎B（AI Studio）和 upstream 引擎返回的真实用量直接采用；拿不到时（引擎A、流式响应等）按文本长度估算，
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
| 文本模型 | `gemini-flash` / `gemini-pro`（引擎A），`/v1/models` 实时聚合双引擎 |

完整对外接口文档：`GET /v1/docs`（OpenAPI 3.1，可导入 Postman/Insomnia）。

Gemini 官方参数审计（2026-10-07）：[参数映射与实现遗漏](docs/api/gemini.md)、[官方全部字段对照](docs/api/gemini-schema.md)、[中文参考详解](docs/api/gemini-reference.md)。完整表由保存的官方 OpenAPI 自动生成，包含请求、响应、工具、agent、沙箱和 SSE 的深层字段。新增[官方 Gemini 后端](docs/api/gemini-official.md)，启用后官方 Interactions 参数和 SSE 完整转发；网页登录态聊天的字段限制单独标注。

生成接口的模式选择、参数、cURL 与响应读取示例见[视频生成](docs/api/videos.md)、[图片生成](docs/api/images.md)、[音频生成](docs/api/audio.md)。默认 native 模式中，图片和音频通过 `/v1/chat/completions` 返回媒体链接。图片尺寸是顶层 `image_size`（`4K`，K 大写），Omni 视频分辨率是顶层 `resolution`（`4k`，k 小写）。Veo 用 `POST /v1/videos` 的 `size`，`veo-3.1-fast-generate-preview` 的 4K 需要 `seconds: 8`。`/v1/images/generations` 与 `/v1/audio/speech` 需要 upstream 模式开启透传，并由上游实现。
端点：`POST /v1/chat/completions`（SSE 流式 + 非流式）、`GET /v1/models`、
`GET /health`、`GET /v1/accounts`。Veo 使用独立长任务接口：
`POST /v1/videos` 创建、`GET /v1/videos/{id}` 轮询、
`GET /v1/videos/{id}/content` 下载完成的视频；请求会走引擎B的
`GenerateVideo` / `GetGenerateVideoOperation` 协议。upstream 模式引擎B 另支持
`/v1/images|videos|audio|files` 透传。

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

**统一账号协议**：所有新账号都提交引擎B协议的 `storage-state.json` 到
`POST /admin/api/accounts`。`engine=b` 直接进入 AI Studio 号池；`engine=a` 会从同一份
状态中提取 `__Secure-1PSID` / `__Secure-1PSIDTS`，进入 Gemini 号池。旧的两个子路径仍可用，
会自动转换后走统一流程。

**AI Studio 号（引擎B）**：三选一——
1. 管理台粘贴 `storage-state.json` 内容（浏览器插件导出或 AIStudio2API 生成）；
2. 跑一次 AIStudio2API 的 `start.bat setup` 导入 Chrome 登录态，把生成的
   `auth/` 目录整个放到 web2api 目录（自动扫描入池并登记）；
3. 已部署 AIStudio2API 的，配置 `auth_states` 指向其 auth 目录，或改用
   `mode: upstream` 直接转发。

## 存储

SQLite（`data/web2api.db`，纯 Go 驱动无 cgo）：`accounts`（号池凭据与状态）、
`api_keys`（Key 分发）、`usage_log`（用量明细）、`cookie_sessions`（完整 Cookie 会话）、`gemini_upload_sessions`（官方断点上传会话）、`admin_settings`（管理员邮箱、昵称、bcrypt 密码哈希、Redis URL 和随机 JWT 签名密钥）。Redis 存储带过期时间的管理员登录会话与登录尝试计数，不保存业务数据。备份 SQLite 时请停止服务后拷贝 `data/` 目录；Redis 会话丢失时重新登录即可。Redis URL 可能含密码，数据库需要妥善保管；敏感目录 `data/`、`auth/` 勿提交 Git。

## 打包与部署

仓库提供跨平台发布脚本，默认一次生成 Windows（amd64/arm64）、Linux（amd64/arm64/armv7）和 macOS（Intel/Apple Silicon）共 7 个发布包。详细的产物结构、Windows/Linux/macOS/Docker 部署步骤见 [deploy/README.md](deploy/README.md)。

在仓库根目录执行：

```powershell
# Windows PowerShell：Windows 包为 zip，其余目标为 tar.gz
.\build.ps1 -Version v1.0.0
```

```bash
# Linux/macOS/WSL：所有目标生成 tar.gz
chmod +x build.sh
./build.sh v1.0.0
```

产物写入 `dist/`，并生成 `SHA256SUMS`。发布包只带示例配置（Linux 包另带 `/opt/web2api` 路径模板），不会带本机的数据库、Cookie 或 auth 凭据。

## 测试与构建

前端已构建并内嵌在程序中，运行时不需要访问 CDN，普通 Go 构建不需要 Node.js。修改 Go 或前端源码后，
先停止正在运行的旧程序，再执行下面的构建命令并重新启动。修改 `web/` 下的前端时，先执行
`npm ci --prefix web && npm run typecheck --prefix web && npm run build --prefix web` 更新 `internal/webui/dist`。

```powershell
go test ./...     # 存储层 + 协议解析 + 网关/管理台集成 + 动态号池装配
go vet ./...
go build -o web2api.exe .
```

## 目录结构

```
main.go                     入口（go run . / go build .）
test/                       全部测试与测试服务（go test ./...）
internal/geminiapi/          官方 Gemini 后端（全参数 / SSE / 上传）
internal/store/              SQLite：账号 / Key / 用量 / 用户 / 余额流水 / 兑换码
internal/config/             YAML 配置 + 环境变量
internal/model/              OpenAI 协议数据结构
internal/engine/             引擎接口定义
internal/engine/geminiweb/   引擎A：gemini 网页协议（请求/流式/Cookie/模型）
internal/engine/upstream/    OpenAI 兼容上游（upstream 模式引擎B）
internal/provider/           双引擎装配 + 动态号池 + 调度路由（manager.go）
internal/gateway/            OpenAI 网关（动态 Key 鉴权 + 用量埋点）
internal/session/            Redis 会话通信（支持 ACL、数据库和 TLS）
internal/admin/              JWT 登录、管理 REST API（/admin/api/*）与用户 API（/api/*）
internal/webui/              内嵌前端（/admin、/console、/login、/register）
web/                         前端源码（React + shadcn/ui，构建到 internal/webui/dist）
internal/limiter/           令牌桶限流
internal/aistudio2api/      引擎B 协议内核（已合并进主模块，MIT）
```
