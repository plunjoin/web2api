v0.3.20 品牌设置：站点名称会同步到浏览器标题，新增可配置站点 Logo；Logo 统一用于侧边栏、登录/注册页和 favicon。

- **动态标题**：保存站点名称后，管理台、控制台、登录/注册页的浏览器标签页立即更新；服务端输出入口 HTML 时也会写入站点名，首次加载不再固定显示 `web2api`。
- **站点 Logo**：管理台「系统 → 设置 → 平台设置」新增 Logo 配置，支持 `http(s)` 图片地址或直接上传 PNG/JPG/WebP/GIF/ICO（≤ 100 KB）；留空回退到内置 W 图标。
- **统一品牌**：Logo 同时用于侧边栏、登录/注册页和浏览器图标；图片加载失败时自动回退，不影响页面使用。
- **接口**：`/api/public/config` 与 `PUT /admin/api/settings` 新增 `site_logo`，原有 `site_name` 行为不变。

升级说明：拉取 `v0.3.20` 镜像或二进制后重启即可，无需数据库迁移；原有站点名、公告与其他设置保持不变。升级前建议照常备份数据库。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.20、ghcr.io/plunjoin/web2api:0.3.20、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。
