# 认证与 Key

## 客户端 API

除 `/health` 和 `/v1/docs` 外，客户端接口使用管理台生成的 `sk-` Key：

```http
Authorization: Bearer sk-xxxxxxxx
```

也可以用 curl：

```bash
curl http://localhost:8800/v1/models \
  -H 'Authorization: Bearer sk-xxxxxxxx'
```

Key 可以在管理台启用、停用和删除，变更会即时生效。服务端会按 Key 记录用量并执行限流。

## 管理 API

管理 API 使用独立的邮箱密码登录流程：

1. `POST /admin/api/auth/login` 获取 `access_token`。
2. 后续请求使用 `Authorization: Bearer <JWT>`。
3. `POST /admin/api/auth/logout` 撤销当前会话。

管理 JWT 默认有效期为 8 小时，登录会话保存在 Redis。管理 API 的 OpenAPI 文档需要登录后访问 `/admin/api/docs`。

## 无鉴权模式

当数据库中没有任何客户端 Key 且配置中没有种子 Key 时，服务会进入无鉴权模式，方便首次本地调试。生产部署完成初始化后，请在管理台创建 Key，并限制服务的网络访问范围。
