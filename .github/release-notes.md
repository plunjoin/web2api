v0.3.2 新增订阅引擎：把 Codex、Antigravity、Kiro、WorkBuddy 四类订阅制 AI 编程工具账号接入同一号池，对外输出统一 OpenAI 兼容 API。每个引擎独立账号池与凭据管理，历史引擎A（Gemini 网页）/引擎B（AI Studio）行为不变。

- 新增四个订阅引擎，均在管理台添加账号：
  - Codex（OpenAI ChatGPT 订阅号）：OAuth 授权或粘贴 `~/.codex/auth.json` 的 refresh_token；Responses 协议，gpt-5.x / gpt-6 系列模型；access_token 运行时自动刷新，401 自动重试。
  - Antigravity（Google Cloud AI Companion 订阅号）：Google OAuth 授权或粘贴 refresh_token；Claude + Gemini 双模型通道；project_id 自动 onboard；账号状态对齐官方 IDE "View Usage"，展示权益档位与周窗口 / 5h 窗口用量。
  - Kiro（AWS CodeWhisperer 订阅号）：粘贴 Kiro IDE 登录态（`~/.aws/sso/cache/kiro-auth-token.json`）的 refresh_token，旋转式 token 自动回写数据库；识别上游封禁（suspended）并进入长冷却。上游风控较严，建议少量账号自用。
  - WorkBuddy（腾讯 CodeBuddy）：管理台扫码设备授权或粘贴 accessToken；OpenAI 协议透传。
- 模型路由：auto 模式按模型名自动命中订阅引擎；`routing.engine_models` 可按引擎显式指定（如 `codex: ["gpt-*"]`）；`routing.default_engine` 支持 codex / antigravity / kiro / workbuddy。
- 管理台：添加账号菜单扩展为六引擎；OAuth 授权链接生成、授权码回填换 token、扫码轮询三类加号流程；订阅引擎凭据支持在线更新（token 轮换）；账号状态展示用量/余额（quota）；概览与 /health 展示全部引擎就绪状态。
- 管理端 API：新增 `GET /admin/api/engines`（引擎元数据）与 OAuth 三端点（authorize / exchange / poll），账号凭据更新接口按引擎分流；OpenAPI 规范同步。
- 数据库：accounts 表自动迁移，解除 `CHECK(engine IN ('a','b'))` 约束（保留全部数据，启动时自动完成，无需手工干预）。

升级说明：无需改配置；新引擎默认关闭，在 config.yaml 的 `engines` 段启用，或直接在管理台添加账号即可使用。访问 chatgpt.com / googleapis.com 需要代理时，配置各引擎的 `proxy`。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.2、ghcr.io/plunjoin/web2api:0.3.2、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：Go 全量测试通过；各批次提交可独立编译（upstream 客户端、provider 接入两批以干净 worktree 抽查编译通过）；antigravity 与 kiro 引擎已实测真实推理连通性（流式与非流式出字正常）。

> AI生成