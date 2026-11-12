v0.3.13 WorkBuddy 真实模型目录与 deepseek 专属窗口：读取 `/v3/config` 展示账号实际可用的模型与免费标识，按默认推理档位下发请求，并单独跟踪 deepseek-v4.1-flash 的 token 窗口。

- **真实模型目录**：WorkBuddy 账号新增 `/v3/config` 读取，优先使用 cli agent 的真实模型列表，包含上下文/输出上限、图片能力、积分倍率与促销价格；读取失败时退回内置目录。配置按账号缓存并异步刷新，模型目录保存到 `model_catalog`。
- **免费标识**：根据 `credits` 与整日有效的 `modelPromotions` 推导免费模型，夜间限时优惠不会误标为免费；管理台模型页与供应商模型弹窗显示「免费」，同名模型跨供应商合并时保留免费态。
- **推理档位**：请求未显式指定 `reasoning_effort` 时，按模型在 config 中的默认档位下发；`max_tokens ≤ 1024` 的短答不开启思考，避免预算全花在推理上。流式响应会单独收集 reasoning 内容，短答只有思考没有正文时不再误报「上游没有返回任何内容」。
- **deepseek 专属窗口**：按账号记录 deepseek-v4.1-flash 的请求 token，额度卡片展示估算的 24 小时窗口（国服约 200M、国际服 100M）；429 拒绝会解析重置时刻并直接钉满窗口。
- **模型级停号**：deepseek 窗口被限流后仅暂停该账号上的这一模型直到重置，同账号其他模型不受影响；其他账号仍可继续承接请求。
- **额度包修复**：付费与免费额度包查询补齐 WorkBuddy 客户端要求的 `PackageCodes`，避免上游以缺少参数拒绝查询。
- **前端**：模型目录与供应商模型弹窗展示免费标识，重建内嵌前端资源。

升级说明：拉取 `v0.3.13` 镜像或二进制后重启即可。启动时会自动为 `model_catalog` 补充 `free` 列，无需手工迁移；旧模型快照在下一次目录刷新前保持原样。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.13、ghcr.io/plunjoin/web2api:0.3.13、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。
