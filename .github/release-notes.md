v0.3.7 修复 Codex 无法加载配置（wire_api=chat 已废弃）与 Antigravity「引擎未启用」。

- CC Switch / Codex 导入：`config.toml` 的 `wire_api` 从 `"chat"` 改为 `"responses"`，`requires_openai_auth` 改为 `true`（直连时 Codex 才会带 Bearer），Deep Link 的 `apiFormat` 对 Codex 改为 `openai_responses`。Codex 已硬拒绝 `wire_api = "chat"`（见 openai/codex#7782），旧导入配置会弹「无法加载 ChatGPT 配置」。网关已实现 `POST /v1/responses`，直连无需 CC Switch 本地路由做协议转换。Claude Code 仍用 `apiFormat = openai_chat` 走本地路由。
- 订阅引擎默认启用：`codex` / `antigravity` / `kiro` / `workbuddy` 与 `custom` 一样默认 `enabled: true`（`config.Default()`、`config.example.yaml`；deploy/docker 模板此前已启用）。管理台添加 Antigravity 等账号时不再因「引擎未启用」失败；空号池时引擎空转不接单，真正请求仍需可用账号。访问上游通常仍需配置 proxy。
- 文档：README「一键导入 CC Switch」章节同步为 responses 直连说明。

升级说明：若本地 `config.yaml` 仍显式写着 `engines.antigravity.enabled: false`（或其它订阅引擎），需改成 `true` 或删掉该字段后重启；新装/无该段配置的部署默认可用。重新在管理台「导入 CC Switch」生成配置，或把已有 `~/.codex/config.toml` 里的 `wire_api = "chat"` 改成 `"responses"` 后点重试。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.7、ghcr.io/plunjoin/web2api:0.3.7、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：`go vet ./...`、`go build ./...`、前端 `tsc -b --noEmit` 全部通过，`internal/webui/dist` 已同步重建；本地起服务验证 `/admin/api/engines` 中 antigravity.enabled=true、生成的导入 TOML 含 `wire_api = "responses"`，以及 `POST /v1/responses` 可达。
