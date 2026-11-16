v0.3.18 原生图片生成与多模态路由兜底：Gemini 生图模型改走原生 generateContent，兼容上游返回的 message.images；生图模型也可按路由组跨供应商失败切换。

- **Gemini 原生生图**：`chat/completions` 中的 Gemini 生图模型会优先调用 `generateContent`，支持文本、Data URI 图片输入与 `image_size` 参数，并把生成的图片转成 Markdown Data URI 返回。
- **OpenAI 兼容生图响应**：解析上游 `message.images` 中的 `image_url.url` 与 `b64_json` 图片，追加到回复文本中，下游仍只需按纯文本协议消费。
- **多模态路由组兜底**：生图模型不再固定只试第一个供应商；同名模型路由组中的首选供应商登录失效、额度不足或上游出错时，可继续切换到下一个供应商。

升级说明：拉取 `v0.3.18` 镜像或二进制后重启即可，无需数据库迁移；升级前建议照常备份数据库。已有供应商、模型目录和路由组配置保持不变。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.18、ghcr.io/plunjoin/web2api:0.3.18、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。
