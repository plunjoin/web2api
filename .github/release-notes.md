v0.3.24 统一模型展示名，并修复 Google Flow 项目页续期时 Cookie 的写回。

- **模型展示名**：模型目录、路由组、用量、计费、供应商模型与请求详情优先展示 `display_name`，真实模型 ID 保留为副标题；仅大小写或 `models/` 前缀不同的名称不再被误判为可读展示名。
- **可读默认名**：Antigravity、Codex 与自定义引擎的默认模型目录生成可读名称，避免全站只看到原始 ID。
- **Flow 凭据续期**：项目页引导响应逐跳合并 `Set-Cookie`，登录态仍有效时条件写回轮换后的 Cookie，避免续期结果只留在内存。
- **并发保护**：写回前校验库存 Cookie 与页面登录邮箱，手动同步或另一请求已更新时自动放弃，不覆盖较新的登录态。
- **项目绑定**：`project_id` 改为单独更新，不再整份覆盖凭据，也不会重置账号状态。
- **管理台资源**：内嵌前端资源已随模型展示名改动同步重建。

升级说明：本版无需数据库迁移，拉取 `v0.3.24` 镜像或重新下载发布包后重启即可。Flow 仍需 Chrome / Edge 获取 reCAPTCHA token，并继续使用原有的账号 Cookie 与代理配置；升级前建议照常备份数据库。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.24、ghcr.io/plunjoin/web2api:0.3.24、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。
