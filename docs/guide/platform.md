# 用户、余额与兑换码

web2api 可以作为对外运营的 API 平台使用：用户自助注册（或由管理员创建），在控制台创建自己的 API Key，调用按 Token 从账户余额扣费，余额通过兑换码或管理员调整充值。管理员自己的 Key（在管理台「API 密钥」创建，不属于任何用户）行为与旧版本完全一致，不扣任何余额。

## 入口

| 地址 | 用途 |
| --- | --- |
| `/login` | 统一登录页。初始化时设置的管理员（root）和平台用户都从这里登录 |
| `/register` | 自助注册，只有管理员在「设置」里开启「开放注册」后可用（默认关闭） |
| `/console` | 用户控制台：概览、API 密钥、用量明细、账单、兑换充值、模型与价格、账号设置 |
| `/admin` | 管理台：概览、用户、兑换码、余额流水、号池账号、API 密钥、用量、模型与定价、设置、接口文档 |

角色分为 `admin` 与 `user`。初始化时创建的 root 管理员只进入管理台；被设为 `admin` 角色的平台用户可以同时使用控制台和管理台。

## 计费

```
扣费 Token = ⌈总 Token × 模型倍率 × Key 倍率 × 用户倍率⌉
```

- 模型倍率在「模型与定价」设置（`*` 为默认倍率）；Key 倍率在 Key 上设置；用户倍率在用户详情里设置，新用户的默认值在「设置」里调整。
- 扣费和用量记录在同一个数据库事务里完成，同时写一条余额流水（`kind = usage`），记录变动前后余额。
- 余额 ≤ 0 时，该用户的所有 Key 返回 `402 insufficient_balance`。跨过 0 的那一次请求会照常完成并扣成负数（请求完成后才知道实际 Token），之后的请求被拒绝。
- Key 的 `token_limit`、过期时间、模型白名单、RPM 限制对用户 Key 同样生效。
- 用户 Key 默认不能调用无法按 Token 计费的接口（Gemini 原生接口、多模态透传）；在「设置」里打开「允许用户密钥使用不计量接口」才放行。
- 用户 Key 调用 `/v1/videos` 时按 `秒数 × 视频每秒计费 Token` 扣费；该值为 0（默认）时用户 Key 不能生成视频。

## 余额流水

每一次余额变动都是一条不可修改的流水，类型包括：

| kind | 含义 |
| --- | --- |
| `signup_bonus` | 注册赠送 |
| `redeem` | 兑换码充值 |
| `adjust` | 管理员调整（必须填写备注，余额不能被调成负数） |
| `usage` | 请求扣费 |
| `refund` | 管理员对某条请求退款（每条请求只能退一次） |

每条流水都带 `balance_before` / `balance_after`，任意用户的流水首尾相接、合计等于当前余额。

## 兑换码

- 管理员在「兑换码」页批量生成（每批 1–1000 个），格式 `W2A-XXXX-XXXX-XXXX-XXXX`（不含易混淆的 0/O/1/I/L），可设置面值、批次名、有效天数和备注。
- 生成后可全部复制或下载 CSV，也可以按批次/状态导出。
- 每个码只能兑换一次；并发兑换同一个码只有一个请求成功，其余返回 `409`。
- 未使用的码可以停用、整批停用或删除；已兑换的码作为充值凭据保留，不能删除，列表里能看到兑换人和兑换时间。
- 用户输入时大小写、空格和短横线位置都不敏感。连续兑换失败 20 次会被限制 15 分钟。

## 安全设计

- 开放注册默认关闭，升级不会让已有部署突然对外开放。
- 用户登录按「邮箱」和「来源 IP」分别限流；root 管理员沿用原有的全局登录限流。
- 修改密码、重置密码、停用账号、修改角色都会让该用户的所有登录立即失效。
- 用户只能看到和操作自己的 Key、用量和流水；错误信息中的邮箱会被隐去。

## 接口

用户接口（`Authorization: Bearer <登录 JWT>`）：

```
GET    /api/public/config                 站点名、是否开放注册、版本（公开）
POST   /api/auth/register                 {email, password, nickname?} → access_token
POST   /api/auth/login                    {email, password} → access_token（root 管理员同样可用）
POST   /api/auth/logout
GET    /api/auth/me
GET    /api/user/overview                 余额、今日/7 天消耗、14 天趋势、最近请求
GET    /api/user/keys                     我的 Key 与每人上限
POST   /api/user/keys                     {name, token_limit?}
PATCH  /api/user/keys/{id}                {name?, enabled?, token_limit?}
DELETE /api/user/keys/{id}
POST   /api/user/keys/{id}/regenerate
GET    /api/user/usage/records?days&key_id&model&limit&offset
GET    /api/user/usage/timeseries?days&key_id
GET    /api/user/ledger?kind&days&limit&offset
GET    /api/user/redeem                   兑换记录（兑换码部分隐藏）
POST   /api/user/redeem                   {code}
PATCH  /api/user/profile                  {nickname}
POST   /api/user/password                 {old_password, new_password} → 新的 access_token
GET    /api/user/models                   可用模型与对我生效的倍率
```

管理接口（需要 admin 角色）：

```
GET    /admin/api/users?q&role&status&limit&offset
POST   /admin/api/users                   {email, password, nickname?, role?, balance?, multiplier?, note?}
GET    /admin/api/users/{id}              用户详情、Key、最近流水、7 天用量
PATCH  /admin/api/users/{id}              {nickname?, role?, enabled?, multiplier?, note?}
DELETE /admin/api/users/{id}              删除用户及其 Key、流水（用量记录保留）
POST   /admin/api/users/{id}/password     {password}
POST   /admin/api/users/{id}/balance      {amount, note}（amount 可为负，note 必填）
GET    /admin/api/ledger?user_id&kind&q&days&limit&offset
POST   /admin/api/usage/records/{id}/refund   {note?}
GET    /admin/api/redeem-codes?status&batch&q&user_id&limit&offset
POST   /admin/api/redeem-codes            {amount, count, batch?, note?, expires_at?}
GET    /admin/api/redeem-codes/export.csv?status&batch&q
PATCH  /admin/api/redeem-codes/{id}       {enabled}
DELETE /admin/api/redeem-codes/{id}       仅限未兑换的码
POST   /admin/api/redeem-codes/batches/{batch}/disable
GET    /admin/api/settings
PUT    /admin/api/settings                {registration_open?, signup_bonus?, default_user_multiplier?, max_keys_per_user?,
                                           site_name?, announcement?, video_tokens_per_second?, user_unmetered_routes?}
```
