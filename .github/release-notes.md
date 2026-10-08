v0.2.5 为每个 API Key 增加 Token 额度与倍率，记录逐请求用量，并说明镜像升级拉取 403 的具体原因。管理台改为深色界面。

- 每个 Key 可设置 Token 额度、Key 倍率、过期时间、模型白名单和每分钟请求上限。额度用尽返回 429 insufficient_quota（token_quota_exceeded），过期返回 401 key_expired，模型不在白名单返回 403 model_not_allowed，超过每分钟上限返回 429 rpm_limit_exceeded。
- 计费 Token = ⌈总 Token × 模型倍率 × Key 倍率⌉。模型倍率按「精确模型 → * 默认 → 1」查找。聊天接口优先记录上游真实用量（AI Studio / upstream），拿不到时按文本长度估算并标记。
- 管理 API 增加用量明细、时间序列、CSV 导出、模型目录、倍率管理和 Key 重新生成。只有 /v1/chat/completions 记 Token；视频、透传和 Gemini 原生路由仍受 Key 限制，但不产生 Token 明细。
- 升级检查在拉取镜像返回 HTTP 403 时核对匿名访问和已配置凭据，并指出需要的设置：公开镜像 ghcr.io/plunjoin/web2api 匿名可拉取；失效凭据、错误或私有镜像名、以及宿主机 Docker 出站拦截会得到不同说明。v0.2.4 若仍报 403，需先手动 docker compose pull 一次，或清掉失效的仓库凭据。
- 管理台改为深色界面（预编译 Tailwind，不访问 CDN）。页面含总览、号池、API Key、用量、模型、设置和接口文档。
- 聊天与视频接口支持 4K：image_size 为 512/1K/2K/4K，Omni resolution 为 4k，Veo 使用 size 4k 且 seconds 为 8。

升级后继续使用原 SQLite 数据库。旧库会自动补上额度、倍率和用量明细字段，已有 Key 默认不限额、倍率为 1。

容器镜像：ghcr.io/plunjoin/web2api:v0.2.5、ghcr.io/plunjoin/web2api:0.2.5、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：Go 全量测试、额度与倍率单测、升级 403 诊断单测、官方字段快照一致性。4K 图片与视频已在本机用真实上游验证；官方协议仍通过本地模拟服务验证。
