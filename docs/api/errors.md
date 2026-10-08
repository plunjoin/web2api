# 错误与限流

## 状态码

| 状态码 | 含义 |
| --- | --- |
| `400` | 请求 JSON 或参数不符合接口要求 |
| `401` | API Key 缺失、无效或已停用；`code: key_expired` 表示 Key 已过期 |
| `402` | Key 所属用户余额不足（`type: insufficient_quota`，`code: insufficient_balance`，带 `X-Web2api-Balance`）。仅用户自己创建的 Key 会出现 |
| `403` | `code: model_not_allowed`：模型不在该 Key 的白名单内；`code: account_disabled`：Key 所属用户已被停用；`code: endpoint_not_allowed`：用户 Key 不能调用不计量的接口（视频未定价、Gemini 原生、透传） |
| `409` | 长任务尚未完成，暂时无法下载 |
| `429` | 超过限流窗口（`rate_limit_error`）、超过该 Key 每分钟请求上限（`code: rpm_limit_exceeded`，带 `Retry-After`），或 Token 额度已用尽（`type: insufficient_quota`，`code: token_quota_exceeded`） |
| `502` | 上游引擎调用失败 |
| `500` | 服务内部错误 |

错误响应统一包含 `error.message` 和 `error.type`，Key 相关的错误另有 `error.code`。客户端应根据状态码决定是否重试：`429 rate_limit_error` 可以退避后重试，`502` 适合切换请求或稍后重试，`400`、`401`、`403` 需要先修正请求或凭据。`429 insufficient_quota` 不会自行恢复，需要管理员提高 `token_limit` 或重置用量；`402 insufficient_balance` 需要在控制台兑换充值或由管理员调整余额。

```json
{
  "error": {
    "message": "API key token quota exhausted (Token 额度已用尽): used 1200 of 1000 charged tokens. Ask the administrator to raise token_limit or reset usage for this key.",
    "type": "insufficient_quota",
    "code": "token_quota_exceeded"
  }
}
```

额度错误同时返回 `X-Web2api-Token-Limit` 和 `X-Web2api-Tokens-Used` 响应头。

余额不足的响应：

```json
{
  "error": {
    "message": "Insufficient balance (余额不足): your account balance is 0 tokens. Redeem a code in the console or ask the administrator to top up.",
    "type": "insufficient_quota",
    "code": "insufficient_balance"
  }
}
```

用户 Key 的成功请求会带 `X-Web2api-Balance` 响应头，值为本次扣费后的余额。

## 限流

全局限流按客户端 Key 生效，参数由服务端配置文件控制。管理员还可以为单个 Key 设置每分钟请求上限（`rpm_limit`，按自然分钟计数）。收到 `429` 时建议使用指数退避，并避免同时重复提交同一长任务。

## 调度失败

当所有可用账号都在冷却、失效或没有目标模型资格时，请求可能返回 `502`。可以通过管理台状态页或 `GET /v1/accounts` 查看账号池是否有可用账号。
