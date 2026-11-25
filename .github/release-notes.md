v0.3.22 接入 Google Flow 图片 / 视频生成，移除 Kiro 引擎，并保留上一版的安全与可观测性修复。

- **Google Flow 原生引擎**：新增 `flow.google.com` 号池，支持 `Cookie` / storage-state 登录态、账号级代理与可选 `project_id`；图片覆盖 Nano Banana 2.1 / 2 Lite，视频覆盖 Veo 3.1 Lite、Fast、Quality 与 Omni 1.1 / 1.2 Flash，支持文生、图生、参考图与延长任务。
- **浏览器打码**：Flow 每次生成前通过本机 Chrome / Edge 获取 reCAPTCHA token，自动探测浏览器路径；支持 `browser_path`、`headless`、`warmup_seconds` 与出口代理配置。
- **统一视频调度**：视频网关按模型在 AI Studio 与 Flow 之间分派，统一长任务提交、轮询与文件下载；原有 AI Studio Veo 行为保持不变。
- **管理台**：账号页新增 Flow 账号添加、Cookie 更新、供应商分组与引擎标签，配置样例同步补充 Flow 段。
- **移除 Kiro**：删除 Kiro 引擎实现、账号入口、配置项、前端资源与文档引用。

升级说明：本版目录结构与配置项有新增；Kiro 引擎、前端入口及配置已移除，原 Kiro 配置项与已保存账号升级后不再生效。拉取 `v0.3.22` 最新镜像或重新下载发布包后重启即可，无需数据库迁移。Flow 每次生成都依赖 Chrome / Edge 获取 reCAPTCHA token；官方 Docker 镜像基于 Alpine，不含浏览器，容器内使用 Flow 需自行安装或挂载浏览器，并通过 `WEB2API_FLOW_BROWSER` 或 `engines.flow.browser_path` 指定可执行文件路径；无法提供浏览器的环境请设置 `engines.flow.enabled: false`，其他引擎不受影响。如已使用较早的 `v0.3.22` 构建，请重新拉取同名镜像或发布包。升级前建议照常备份数据库。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.22、ghcr.io/plunjoin/web2api:0.3.22、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。
