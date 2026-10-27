v0.3.7 修复 Codex / CC Switch 全链路：Responses 直连、空号池订阅引擎抢路由、Gemini Key 工具调用与用量自查。

- CC Switch / Codex 导入：`config.toml` 的 `wire_api` 改为 `"responses"`，`requires_openai_auth` 改为 `true`，并补上 `model_provider = "custom"`（手动粘贴时不会把 Key 发到 api.openai.com）。Deep Link 对 Codex 使用 `apiFormat=openai_responses`（不再提示开路由）；Claude Code 仍用 `openai_chat` 走本地路由。一键导入同时带上用量查询脚本（`usageEnabled` + `usageScript`）。
- 订阅引擎默认启用：`codex` / `antigravity` / `kiro` / `workbuddy` 与 `custom` 一样默认 `enabled: true`。管理台加 Antigravity 等账号不再因「引擎未启用」失败。
- **空号池不再抢路由**：没有可用账号的订阅引擎不参与模型匹配，也不出现在 `/v1/models`（管理台仍可见）。避免 `gemini-2.5-flash` 被空的 Antigravity / WorkBuddy 抢走返回 502。
- **Codex 模型目录**：`/v1/codex/catalog` 去掉 Google 的 `models/` 前缀并按 `Kind==chat` 过滤；补全 `live` / `transcribe` / `deep-research` / `aqa` 等分类，非对话模型不再混进目录。
- **第三方 Key（Gemini）兼容**：`tool_choice` 原样 JSON 下发（修 base64 导致的 400）；base_url 已带版本段时不再补 `/v1`；Gemini 3 多轮工具调用自动回传 `thought_signature`。
- **用量自查**：新增 `GET /v1/usage`（Bearer / x-api-key），返回 Sub2API 形状的 remaining / used / isValid 等；CC Switch 卡片可直接测用量。注意 CC Switch 用量脚本仅允许 HTTPS 或 localhost。

升级说明：若本地 `config.yaml` 仍显式写着 `engines.antigravity.enabled: false`（或其它订阅引擎），需改成 `true` 或删掉该字段后重启。请在管理台重新「导入 CC Switch」生成配置；已有 `~/.codex/config.toml` 里的 `wire_api = "chat"` 请改成 `"responses"` 并补 `model_provider = "custom"`。图形版 CC Switch 导入后卡片模型列表可能仍为空（深链不写列表），在编辑页点「获取模型列表」即可；用量查询若未自动带上，可选手动粘贴面板「手动配置」里的脚本，或选 Sub2API 模板。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.7、ghcr.io/plunjoin/web2api:0.3.7、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：`go vet ./...`、`go test ./...`、前端重建通过；Gemini Key 经 custom 引擎，`codex exec` 对话 / 流式 / shell 工具调用跑通；`/v1/usage` 额度扣减正确。
