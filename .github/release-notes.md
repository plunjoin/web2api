v0.3.2 新增订阅引擎与第三方 Key 接入：把 Codex、Antigravity、Kiro、WorkBuddy 四类订阅制 AI 编程工具账号，以及任意 OpenAI 兼容供应商的第三方 Key，接入同一号池，对外输出统一 OpenAI 兼容 API。每个引擎独立账号池与凭据管理，历史引擎A（Gemini 网页）/引擎B（AI Studio）行为不变。

- 新增四个订阅引擎，均在管理台添加账号：
  - Codex（OpenAI ChatGPT 订阅号）：OAuth 授权或粘贴 `~/.codex/auth.json` 的 refresh_token；Responses 协议，gpt-5.x / gpt-6 系列模型；access_token 运行时自动刷新，401 自动重试。
  - Antigravity（Google Cloud AI Companion 订阅号）：Google OAuth 授权或粘贴 refresh_token；Claude + Gemini 双模型通道；project_id 自动 onboard；账号状态对齐官方 IDE "View Usage"，展示权益档位与周窗口 / 5h 窗口用量。
  - Kiro（AWS CodeWhisperer 订阅号）：粘贴 Kiro IDE 登录态（`~/.aws/sso/cache/kiro-auth-token.json`）的 refresh_token，旋转式 token 自动回写数据库；识别上游封禁（suspended）并进入长冷却。上游风控较严，建议少量账号自用。
  - WorkBuddy（腾讯 CodeBuddy）：管理台扫码设备授权或粘贴 accessToken；OpenAI 协议透传。
- 新增 custom 第三方 Key 引擎（默认启用）：管理台填写 OpenAI 兼容 Base URL + API Key 即接入，多 Key 轮询，添加前可一键探测上游模型列表；余额查询按供应商识别（DeepSeek / OpenRouter / SiliconFlow / StepFun 等）并对 OpenAI 兼容中转级联探测（/v1/usage → /user/balance → NewAPI /api/user/self），管理台展示剩余额度并约每 10 分钟自动刷新。
- 模型上下架与可用性检查：管理台可对任一模型上下架（下架后对下游 /v1/models 不可见、调用返回 404）；单个/批量「检查」按用途真实探测（对话走 chat/completions、生图走 images/generations、向量走 embeddings，视频/音乐/语音仅查目录避免昂贵生成），默认按结果自动同步上下架；模型分类补充 omni→视频、dall-e→图像。
- 模型路由：auto 模式下订阅/custom 引擎按模型 id 目录精确命中，专属模型引擎不可用时不跨引擎切换（避免误导性错误）；`routing.engine_models` 可按引擎显式指定（如 `codex: ["gpt-*"]`）；`routing.default_engine` 支持 codex / antigravity / kiro / workbuddy / custom。引擎B 兜底时若请求模型不在其实时目录，自动改用目录第一个可用模型。
- 管理台：添加账号菜单扩展为七引擎；OAuth 授权链接生成、授权码回填换 token、扫码轮询三类加号流程；订阅/custom 引擎凭据支持在线更新（token 轮换）；账号状态展示用量/余额（quota）；概览与 /health 展示全部引擎就绪状态。
- 管理端 API：新增 `GET /admin/api/engines`、`POST /admin/api/custom/models`（第三方模型预览）、`PATCH /admin/api/models/{model}`（上下架）、`POST /admin/api/models/check`（可用性检查）与 OAuth 三端点（authorize / exchange / poll）；OpenAPI 规范同步。
- 数据库：accounts 表自动迁移，解除 `CHECK(engine IN ('a','b'))` 约束；新增 model_settings 表（模型上下架与检查记录）。均保留全部数据，启动时自动完成，无需手工干预。

升级说明：无需改配置；订阅引擎默认关闭，在 config.yaml 的 `engines` 段启用，或直接在管理台添加账号即可使用；custom 引擎默认启用，管理台直接填 Base URL + API Key。访问 chatgpt.com / googleapis.com 需要代理时，配置各引擎的 `proxy`。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.2、ghcr.io/plunjoin/web2api:0.3.2、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：Go 全量测试通过（新增供应商余额识别、Base URL 归一化、模型设置持久化、custom 路由与目录命中等单元测试）；各批次提交可独立编译（upstream 客户端、provider 接入两批以干净 worktree 抽查编译通过）；antigravity 与 kiro 引擎已实测真实推理连通性（流式与非流式出字正常）。

> AI生成