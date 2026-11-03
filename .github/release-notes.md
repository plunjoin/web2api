v0.3.9 架构重构与能力升级：Responses / Chat 协议解耦、引擎目录规范化、模型快照管理与音乐模型双向互通。

- **音乐模型双向互通**：音乐模型（Lyria 等）支持 `/v1/chat/completions` 与 `/v1/music/generations` 双向调用。对话请求自动适配各底层引擎并以 Markdown Data URI 形式返回音频；专用音乐端点支持生成标准 JSON 或直接返回二进制 MP3/WAV。
- **Responses 与 Chat 协议解耦（Wire 分层）**：抽出 `internal/wire/chat` 与 `internal/wire/responses` 独立负责出入站协议编解码与流式装配，结合内部 IR 与工具清洗（非 function 工具在入口丢弃并记日志），彻底解决 Codex 与 CC Switch 工具兼容痛点。
- **引擎目录统一与规范化**：七大引擎实现统一迁入 `internal/engine/<name>`，引擎标识 a/b 规范命名为 `geminiweb` / `aistudio`，旧配置键与存量数据库记录无缝平滑向下兼容。
- **模型快照与管理端按供应商刷新**：数据库新增 `model_catalog` 快照表，管理台支持按单个供应商或全量主动刷新上游模型，新增供应商模型专页，支持可视化查看快照与一键全选上下架。
- **工具调用与备选路由强化**：请求携带 tools 时增加严格的通道校验，不支持工具的引擎明确报错 `tools_not_supported`；workbuddy 接入 ToolEngine；备选路由支持 custom 透明兜底。

升级说明：拉取 `v0.3.9` 镜像或二进制后重启即可。存量配置与数据库无需手动迁移。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.9、ghcr.io/plunjoin/web2api:0.3.9、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。
