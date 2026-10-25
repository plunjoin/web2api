v0.3.3 模型路由与批量管理增强：修复多引擎场景下的模型路由缺陷，模型与定价页按引擎分组并支持批量上下架，多模态端点（embeddings 等）按 model 参数路由到第三方 Key 上游。

- 修复模型路由核心缺陷：引擎A/B 的实时目录此前不参与路由决策，目录内裸名模型（如引擎A 的 `gemini-argon`、引擎B 目录中的正式 ID）会全部落入 auto 兜底走 AI Studio，再被静默换成目录第一个模型，最终报「没有符合条件的 AI Studio 账户」。现路由顺序为：`routing.engine_a/b_models` 显式指定 > 订阅/custom 目录精确命中 > 引擎A/B 实时目录精确命中（`models/` 前缀宽松匹配）> 多模态关键词走引擎B > `auto` 兜底。目录命中的模型视为该引擎专属：引擎不可用时直接返回该引擎的具体错误，不做跨引擎切换。
- 移除引擎B 静默换模：请求模型不在 AI Studio 实时目录时原样透传，由引擎B 返回明确的目录错误（「AI Studio 实时目录中没有请求模型: X」）。旧行为会换用目录排序第一个模型，而排序首位可能是仅支持 Interactions API / 高权益门槛的模型，导致误报无可用账户。
- 多模态透传按模型路由：`/v1/embeddings` 等多模态端点此前只认引擎B 透传模式，custom 上游的 embedding 模型会报「未启用支持多模态透传的引擎」。现按请求 body 的 `model` 参数两级路由：命中 custom 目录 → 直接转发对应账号上游；未命中才回退引擎B。目录匹配与账号选择均容忍 `models/` 前缀（裸名/带前缀均可命中，聊天路由同样受益）；修复多 custom 账号裸名匹配失败时退化随机挑账号的问题（避免把 Gemini 模型发给 MiniMax）。
- 多模态端点缺 model 参数返回 OpenAI 风格 400：错误信息附示例 body，指引补齐必填参数，而非回退引擎B 后报误导性错误。
- 模型与定价页按引擎分组（Gemini 网页 / AI Studio / Codex / Antigravity / Kiro / WorkBuddy / 第三方 Key / 不在当前目录）：组头支持全选 / 全部开放 / 全部下架，分组可折叠，批量操作带确认弹窗。
- 模型批量上下架 API：新增 `POST /admin/api/models/batch`，去重后单事务写入，单次最多 500 个模型，保留各模型的 last_check 检查记录；store 新增 `SetModelsEnabled` 批量方法。
- 修复全新数据库首次启动问题：accounts 建表语句移除 `CHECK(engine IN ('a','b'))` 约束，首次启动即可直接添加订阅/custom 引擎账号，无需重启一次（既有数据库的自动迁移不受影响）。

升级说明：无需改配置；已有部署升级后引擎A/B 路由与换模行为按上述变化生效。使用第三方 Key 的 embedding 模型时，请求 `/v1/embeddings` 需带 `model` 参数（OpenAI 协议必填）。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.3、ghcr.io/plunjoin/web2api:0.3.3、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：Go 全量测试通过（新增引擎A/B 目录路由、请求模型原样透传、custom 模型路由等单元测试）；隔离实例实测 `gemini-argon` → 引擎A 正常返回、`models/gemini-4-argon` → custom 既有路由不变、未知模型返回明确目录错误；embeddings 带完整参数实测 200（3072 维向量），裸名/带 `models/` 前缀均可命中。

> AI生成