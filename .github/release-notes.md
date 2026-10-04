统一账号协议和 Veo 视频长任务接口，默认优先使用 AI Studio 引擎。

- 新增统一账号入口 `POST /admin/api/accounts`，支持通过 storage-state 添加双引擎账号；兼容原有子路径。
- 新增 Veo 视频任务创建、状态查询和内容下载接口。
- 新增公开 OpenAPI 3.1 文档 `GET /v1/docs`。
- 新增本机 AI Studio 登录态导入工具 `/tools/export-storage.sh`。
- 修复 Waa/Create 的代理及响应解压处理。
- 补齐跨平台发布包所需的启动文件、部署模板，以及自动发布流程。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

容器镜像：`ghcr.io/plunjoin/web2api:v0.2.0`（也可使用 `latest`），支持 Linux amd64、arm64、arm/v7。

验证：`go test ./...`、`go build ./cmd/web2api`。
