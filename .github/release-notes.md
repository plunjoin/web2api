v0.3.17 WorkBuddy 双发行版供应商拆分：国服与国际服不再混池；停用账号会从模型目录和路由组中即时移除；后台为 WorkBuddy 与 WorkBuddy AI 提供独立添加入口。

- **WorkBuddy AI 独立供应商**：`workbuddy` 只承接国服和旧 CodeBuddy 账号，`workbuddy-ai` 只承接国际服账号。启动时会自动把国际服账号迁移到独立供应商；两个发行版各自维护账号池、模型目录与额度数据。同名模型（如 `glm-5.3`）可自动组成两个成员的路由组。
- **独立添加入口**：后台「添加账号」菜单区分「WorkBuddy（国服）」与「WorkBuddy AI（国际服）」，授权弹窗固定发行版，不再允许中途切换；粘贴 token 时会按声明站点写入对应供应商。
- **停用账号即时过滤**：第三方供应商停用最后一个支持某模型的账号后，该模型不再出现在下游模型列表和路由组；所有账号停用后，对应供应商也从管理台隐藏。重新启用账号会自动恢复展示与路由。
- **路由组健康度修正**：路由组账号就绪数只统计已启用账号；停用账号不再影响供应商优先级，避免切换到不可承接请求的来源。

升级说明：拉取 `v0.3.17` 镜像或二进制后重启即可，无需数据库迁移；升级前建议照常备份数据库。旧 WorkBuddy 国际服账号会自动迁移为 `workbuddy-ai` 供应商；国服与旧 CodeBuddy 账号保持不变。第三方 Key 的 `custom|<base_url>` 目录和已有路由组配置会继续沿用。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.17、ghcr.io/plunjoin/web2api:0.3.17、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。
