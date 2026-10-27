v0.3.8 修复 Gemini / Codex 工具调用：Antigravity 透传 function calling，`/v1/responses` 不再丢掉非 function 内置工具。

- **Antigravity function calling**：实现 `ToolEngine` / `ChatStreamSink`；请求下发 `functionDeclarations`，SSE 解析 `functionCall`，多轮回填 `functionResponse`（含 `thought_signature`）。此前 tools 被静默丢弃，模型只能用纯文本说「没有挂载本地命令行」。
- **`/v1/responses` 工具映射**：`local_shell` / `apply_patch` 降级为同名 `function`；`namespace` 拍平为内部 functions；`web_search` 仍忽略。老 Codex 客户端不再因 `type != function` 丢工具。
- **单测**：覆盖 Antigravity `buildBody` / `parseSSE` / `toAntigravityRequest`，以及 gateway `toChatTools`。

升级说明：拉取 `v0.3.8` 镜像或二进制后重启即可。用 Antigravity Gemini 跑 Codex / CC Switch 时，请确认账号可用且引擎已启用；官方 Gemini Key（custom）路径同样受益于 responses 工具映射。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.8、ghcr.io/plunjoin/web2api:0.3.8、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：`go test ./internal/upstream/antigravity ./internal/gateway ./internal/provider` 通过；真 Codex CLI + 官方 Gemini Key（`gemini-3-flash-preview`）`exec_command` 读文件跑通；`local_shell` 经网关后模型发出 `function_call`。真实 Antigravity 订阅号端到端建议合并后本地补测。
