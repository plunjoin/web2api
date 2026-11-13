# TeleAgent（TeleClaw）登录与签到接口

> 本文整理 TeleAgent（TeleClaw）桌面端的**登录**与**签到（积分福利社）**接口。
> 这些接口不在 web2api 代码库内，而是打包在 Electron 应用 `C:\Program Files\TeleClaw\TeleAgent\resources\app.asar` 中，本文内容为逆向该 asar 所得；字段名来自压缩后的代码，仅作参考，不代表上游公开协议。

## 1. 代码位置

| 位置 | 内容 |
|---|---|
| `app.asar:/dist/assets/index-CBVozzc-.js` | 渲染层主包：zustand store（`token`/`user`/`userPointData`/`dailySignData`/`taskList`）、API 常量 `ct`、请求封装 `Sj`/`Pn`/`nx`/`nMe` |
| `app.asar:/dist-electron/main-runtime-*.js` | 主进程：`login:open`/`login:close` IPC handler、登录窗口与回调解析、二维码登录 pairing 端点 |
| `app.asar:/dist-electron/preload.cjs` | `window.electronAPI` 桥接 |

## 2. 环境与基址

源码中通过 `XM()` 判断是否桌面端（`window.electronAPI.platform !== "web"`），据此切换基址：

| 变量 | 值 |
|---|---|
| `ul` / `go`（API 基址，`ct.BASE_URL`） | `https://agent.teleai.com.cn/superCowork/sapi` |
| `VZe`（登录入口） | `https://agent.teleai.com.cn/meter/teleagent-login/login` |
| `jU`（预发环境） | `https://agent-pre.teleai.com.cn` |
| `ZM`（Web 端路径前缀） | `/teleagent-space` |
| `Pie` / `LOGIN_PAGE`（登录回调页） | `${ul}/cowork/auth` |

- **桌面端**（`XM()` 为真）：`en = ""`，`ct.BASE_URL = ul`，接口用绝对域名。
- **Web 端**（`XM()` 为假）：`en = "/teleagent-space"`，`ct.BASE_URL = ""`，接口路径变为 `/teleagent-space/...`。

## 3. 登录流程

### 3.1 端到端流程

1. 渲染层 `toLogin()` 调用 `window.electronAPI.login.open({ errorMessage? })`。
2. 主进程 `login:open` handler → `bZt()` → 打开/复用 `BrowserWindow`（`TZt`/`IZt`），加载登录 URL。
3. 用户在登录窗口完成登录（可能涉及天翼账号，`login:close` 会清除 `open.e.189.cn` 的 cookie）。
4. 主进程监听 `did-navigate` / `did-navigate-in-page`：当 URL 的 pathname 包含 `/cowork/auth` 且带 `code` 查询参数时，解析出 `{ code, token, url }` 并 resolve。
5. `login:open` 返回给渲染层：

```json
{ "success": true, "code": "...", "token": "...", "url": "https://.../cowork/auth?code=...&token=..." }
```

取消或关闭窗口时返回 `{ "success": false, "cancelled": true }`。

6. 渲染层把 `token` 写入 store（`setToken`），置 `tokenVerified`，再调用 `USERINFO` / `AUTH_SESSION_CODE` 校验登录态。

### 3.2 登录相关 URL

| 用途 | URL |
|---|---|
| 登录入口 | `https://agent.teleai.com.cn/meter/teleagent-login/login` |
| 登录中间跳转 | `https://agent-pre.teleai.com.cn/meter/teleagent-login/middle?redirect_url=.../meter/teleagent/user/portal/rights` |
| 登录回调页 | `https://agent.teleai.com.cn/superCowork/sapi/cowork/auth?code=...&token=...` |
| AIS 登录 | `https://cs.teleai.com.cn/iam/api/aisLogin` |

### 3.3 登录后会话接口（`ct` 常量，带 `X-Token`）

| 常量 | 路径 | 方法 | 作用 |
|---|---|---|---|
| `AUTH_SESSION_CODE` | `${en}/api/v1/auth/session-code` | GET | 取会话 code |
| `AUTH_STR` | `${en}/cowork/auth_str` | GET | 取鉴权串 |
| `USERINFO` | `${en}/cowork/user_info` | GET | 用户信息 |
| `LOGOUT` | `${en}/cowork/logout` | GET | 登出 |

### 3.4 扫码登录（主进程 pairing 端点）

| 端点 | 作用 |
|---|---|
| `/api/v1/qrcode/generate` | 生成二维码 |
| `/api/v1/qrcode/scan` | 扫码 |
| `/api/v1/qrcode/status` | 轮询扫码结果 |

## 4. 鉴权请求头

每次 API 请求由 `Sj(method, path, opts)` 发起：

- URL：`/^https?:\/\//` 判定，相对路径拼 `${ct.BASE_URL}${path}`。
- 请求头 = `nx()` 返回的鉴权头 + `Content-Type: application/json`。
- 封装：`Pn(path) = Sj("GET", path)`（另有 PATCH 等变体）。

`nx()` 构造的请求头：

| Header | 来源 |
|---|---|
| `X-Token` | `sn.getState().token`（登录后写入的 token） |
| `X-Timestamp` | 当前 Unix 秒 |
| `X-Nonce` | `pl()` 生成的随机 nonce（时间戳 + `crypto.getRandomValues` 的 UUID 样式） |
| `X-App-Version` | 应用版本 |
| `X-OS-Type` | 操作系统类型 |
| `X-SuperAgent-Device-Id` | 设备 ID（回退 MAC 地址） |
| `X-Channel-Id` | 渠道 ID |
| `X-Runtime-Env` | 运行时环境 |

`nMe()`：对**同源**请求（`eMe()` 比较 origin，或相对路径）额外加一个 `teleagent` 头，其值为 `rMe()` 返回的部署环境字符串（`JRe` 只是 trim，无加密签名）。

## 5. 签到 / 积分福利社接口

UI 文案为「积分福利社 / Star Welfare Club」：签到即「每日领取积分」；另有「首登即赠积分礼」。全部为 **GET** 请求。

### 5.1 端点（`ct` 常量）

| 常量 | 路径 | 作用 |
|---|---|---|
| `ACTIVE_CHECK_DAILY_TASK` | `${en}/user/portal/companyStore/checkTask` | **签到领取**（拼 `?type=daily_login`） |
| `NEW_ACTIVE_DAYTASK` | `${en}/user/portal/companyStore/v2/checkDayTask` | 查询每日任务状态 |
| `ACTIVE_DAYTASK` | `${en}/user/portal/companyStore/checkDayTask` | 旧版查询 |
| `ACTIVE_USERCENTER` | `${en}/user/portal/companyStore/userCenter` | 用户中心 / 积分余额（`userPointData`） |
| `ACTIVE_CAMPAIGN_TYPE` | `${en}/user/portal/companyStore/getCampaignType` | 活动类型 |

桌面端完整 URL：

```
GET https://agent.teleai.com.cn/superCowork/sapi/user/portal/companyStore/checkTask?type=daily_login
GET https://agent.teleai.com.cn/superCowork/sapi/user/portal/companyStore/v2/checkDayTask
GET https://agent.teleai.com.cn/superCowork/sapi/user/portal/companyStore/userCenter
```

### 5.2 签到动作

```js
await Pn(ct.ACTIVE_CHECK_DAILY_TASK + "?type=daily_login"); // 签到领取
await Pn(ct.NEW_ACTIVE_DAYTASK);                            // 刷新每日任务状态
```

### 5.3 响应数据结构（`checkDayTask` 响应）

| 字段 | 含义 |
|---|---|
| `daily_task.claimed_today` | 今日是否已领 |
| `daily_task.daily_claimable_points` | 今日可领积分 |
| `daily_task.total_claim_days` | 本期累计领取天数 |
| `daily_task.button_text` | 领取按钮文案 |
| `activity.visible` / `daily_task.visible` | 是否展示 |
| `task_code` | 任务类型：`first_login`（首登）、`monthly_basic`（月度基础）、`daily_login`（每日登录/签到） |

「首登赠礼」复用同一 `checkTask` 接口，按 `task_code === "first_login" || "monthly_basic"` 过滤。

### 5.4 福利社页面（跳转）

```
https://agent-pre.teleai.com.cn/meter/teleagent/user/portal/star-welfare
```

## 附录 A：web2api 的登录接口

代码位于 `internal/admin/auth.go`、`internal/admin/admin.go`。

| 端点 | 方法 | 说明 |
|---|---|---|
| `/api/auth/register` | POST | 平台用户注册 |
| `/api/auth/login` | POST | 平台用户 / 管理员登录（同一 `handleLogin`） |
| `/admin/api/auth/login` | POST | 管理员登录 |
| `/api/auth/logout` | POST | 登出 |
| `/api/auth/me` | GET | 当前登录主体 |
| `/admin/api/oauth/{engine}/authorize` `/exchange` `/poll` | POST | 号池账号的 OAuth 授权登录 |

`handleLogin` 逻辑要点：

- 入参 `{ email, password }`；校验密码 ≤72 字节、邮箱 ≤254 字符。
- 管理员邮箱命中的走管理员分支：Redis `EVAL`（INCR+EXPIRE）限流，10 次 / 15 分钟 → `429`。
- 非管理员邮箱走平台用户分支（按邮箱 + 来源 IP 计数，避免一个用户输错锁住所有人）。
- 密码用 `bcrypt.CompareHashAndPassword` 校验。
- 成功签发 **HS256 JWT**，claims：

| 字段 | 含义 |
|---|---|
| `iss` | `web2api` |
| `aud` | `web2api-admin` |
| `sub` | 根管理员 `"1"`，平台用户 `"u:<id>"` |
| `jti` | 随机会话 ID |
| `iat` / `nbf` / `exp` | 签发 / 生效 / 过期（`sessionLifetime = 8h`） |
| `ver` | 平台用户 `token_version`（改密/停用/改角色后旧登录态失效） |

- 会话写入 Redis：`SET web2api:<sha256(jwtSecret)[:8]>:session:<jti> <sub> EX <8h>`。
- 返回：

```json
{ "access_token": "<jwt>", "token_type": "Bearer", "expires_in": 28800, "user": { ... } }
```

- 鉴权：请求头 `Authorization: Bearer <jwt>`，`verifySession` 校验 JWT 签名/claims + Redis 会话是否存在且 subject 一致。
