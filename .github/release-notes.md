v0.3.5 新增 Codex 模型目录端点与 CC Switch 一键导入：密钥直接变成 Codex / Claude Code 的供应商配置。

- 新增 `GET /v1/codex/catalog`：按该 Key 的白名单与模型上下架过滤后输出可用对话模型，字段与 `cc-switch-model-catalog.json` / Codex `models_cache.json` 同构（`visibility`、`support_verbosity`、`truncation_policy` 等必填字段齐全，缺失会导致 Codex 启动时解析失败）。鉴权支持 `Authorization: Bearer` 与查询参数 `?key=`，目录拉取不计 RPM、不写用量。
- 管理台与控制台的 API 密钥页新增「导入 CC Switch」：实时拉取该密钥可用的对话模型供选择，生成 `ccswitch://v1/import` Deep Link（需 CC Switch v4+），一键导入为 Codex 或 Claude Code 供应商；Deep Link 打不开时展开「手动配置」，提供可复制的 `config.toml`、Base URL 与导入链接。密钥无可用对话模型（白名单限制或全部下架）时直接提示，避免导入一个用不了的配置。
- Codex 侧用 `wire_api = "chat"` + `model_catalog_url` 指向新端点：网关只实现 Chat Completions，协议差异由 CC Switch 本地路由转换；模型目录改为动态拉取，号池加号、模型上下架或白名单调整后无需重新导入、无需手改配置。
- Claude Code 侧用 `apiFormat = openai_chat` 走 CC Switch 本地路由：网关没有 Anthropic `/v1/messages` 端点，`ANTHROPIC_BASE_URL` 不能直接指向网关，导入时由 CC Switch 把 Anthropic Messages 翻成 Chat Completions 再进网关。
- 文档：README 新增「一键导入 CC Switch（Codex / Claude Code）」章节，API 文档补 `/v1/codex/catalog` 条目，根路径端点列表同步。

升级说明：无需改动配置，纯新增端点与前端入口，既有 Key 的额度、白名单与用量统计口径不变；`/v1/codex/catalog` 只读目录，不消耗额度。另修正一处版本漂移：`internal/version/VERSION` 此前停留在 `v0.3.2`，容器镜像构建未传 `VERSION` 时会回退到该文件，导致镜像内自报版本偏低，本次同步为 `v0.3.5`。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.5、ghcr.io/plunjoin/web2api:0.3.5、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：`go vet ./...`、`go build ./...`、前端 `tsc -b --noEmit` 全部通过，`internal/webui/dist` 已同步重建且入口引用的资源齐全。

> AI生成
