v0.2.4 新增官方 Gemini 后端，保留网页登录态账号池，并统一项目结构与 SQLite 会话存储。

- 新增官方 Gemini API Key / OAuth 后端，完整转发官方参数、嵌套配置、扩展字段、响应、错误和 SSE。
- 接入 Interactions 创建、查询、取消、删除，以及官方 agent、沙箱、声音、webhook、trigger、credential 管理操作。
- 支持官方 SDK、文件上传与续传；续传会话存入 SQLite，网关重启后可恢复。
- Cookie 会话改存 SQLite，修复自动刷新生命周期和并发保存问题；账号凭据替换或删除后使旧会话失效。
- 程序入口移至根目录 main.go，移除 cmd/web2api；所有测试集中到根目录 test，并纳入发布前验证。
- 部署不再创建或挂载 cookies 目录；提供 -import-cookie-cache 命令导入旧缓存，保留旧文件。
- 补充官方后端配置、SDK / REST、多模态、工具、后台任务和部署文档；逐项对照官方快照的 20 个路径、37 个操作、214 个类型和 722 个字段定义。

设置 WEB2API_GEMINI_API_KEY 可自动启用官方后端；客户端使用网关 Key，SDK Base URL 推荐 http://localhost:8800/gemini。详见 [官方后端接入](https://plunjoin.github.io/web2api/api/gemini-official) 和 [完整参数对照](https://plunjoin.github.io/web2api/api/gemini-schema)。

升级后继续使用原 SQLite 数据库和 AI Studio auth 数据。若需保留旧缓存中的最新 Cookie，请先停止旧服务，再执行一次：

```bash
./web2api -config config.yaml -import-cookie-cache /原来的Cookie缓存目录
```

容器镜像：ghcr.io/plunjoin/web2api:v0.2.4、ghcr.io/plunjoin/web2api:0.2.4、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：Go 全量测试与 vet、官方字段快照一致性、文档构建、Windows / Linux 编译和 Compose 配置检查。官方协议通过本地模拟服务验证，未使用真实 Google 计费账号逐模型验证。
