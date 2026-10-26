v0.3.4 按模型声明接单：不再乱兜底、不再跨引擎换模，并重整引擎B 协议内核目录。

- 第三方 Key 号池改为「按声明接单」：只有目录里明确声明了该模型的可用账号才会接单。原来存在两段兜底——模型没命中目录时随便抓一个就绪号、空目录账号对所有模型开放——下游请求 `lyria-3.5` 会被打到不相干的上游。现在没有任何账号声明时返回 `404 model_not_found`，错误里写明当前号池缺哪个模型；空目录账号需在管理台拉目录或手填 `models` 才会接单。
- 错误码对齐客户端语义：模型无人声明返回 `404 model_not_found`，端点与模型用途不匹配返回 `400 unsupported_endpoint`。此前一律 502，下游会误以为可以重试。业务错误（400/404）不再写入账号详情、不再换号重试。
- 禁止跨引擎兜底：`engine_a/b_models` 与 `routing.engine_models` 显式指定、以及多模态关键词（image/veo/lyria…）命中的模型，一律视为该引擎专属。目标引擎不可用时返回它自己的真实错误（如 `worker_ready=0`），不再改投第三方 Key；`alternate` 也不会切到没有声明该模型的号池。`routing.engine_models` 改为按引擎注册顺序遍历，避免 map 顺序随机导致同一模型在重启间落到不同引擎。
- 用途闸门与音乐入口：音乐/视频模型走 `/v1/chat/completions` 时，只有能出媒体的引擎（引擎B native）接单，否则直接返回 `400 unsupported_endpoint` 并提示正确入口，不再把上游那句 `Unhandled generated data mime type: audio/mpeg` 透给下游。新增 `POST /v1/music/generations` 透传入口。
- 就绪口径收紧：引擎B 的 `Ready()` 参考 WAA Worker 状态，账号 ready 但 Worker 全部启动失败时不再谎报可用；`/v1/models` 的 `available` 要求归属引擎能接单，且至少一个可用账号的目录里有该模型。管理台的模型探测要求「有引擎归属 + 目录声明 + 引擎可接单」，不再只查目录就算通过。
- 引擎B 协议内核目录重整：`internal/aistudio2api/aistudio` 迁到 `internal/aistudio`，`waa` 迁到 `internal/aistudio/waa`；只保留账户指纹能力到 `internal/aistudio/fingerprint`，删掉没有任何调用点的浏览器自动化栈（launcher / bidi / drive / download / cache / login 等 12 个文件）、`config` 包（仅一个函数被用到，已内联）以及隔离登录驱动、转写服务、WebSocket 通道。净减少约 5200 行，`github.com/gorilla/websocket` 依赖移除。
- 测试用例不再随仓库分发（`test/` 已加入 `.gitignore`），发布流水线的测试步骤改为 `go vet`。

升级说明：配置无需改动。两处行为变化需注意——① 模型无人声明时由 502 变为 `404 model_not_found`，客户端若把 502 当作可重试错误，需要同步调整；② 账户目录下的指纹文件由 `camoufox-fingerprint.json` 改名为 `fingerprint.json`，未迁移的账号会在首次启动 Worker 时重新生成一套指纹（可手工 `mv` 旧文件保留原指纹）。第三方 Key 账号若从未拉到模型目录也没手填 `models`，升级后不再接单，请在管理台补齐目录。

容器镜像：ghcr.io/plunjoin/web2api:v0.3.4、ghcr.io/plunjoin/web2api:0.3.4、ghcr.io/plunjoin/web2api:latest；支持 linux/amd64、linux/arm64、linux/arm/v7。

提供 Windows、Linux、macOS 共 7 个平台发布包和 SHA256SUMS。

验证：`go build ./...`、`go vet ./...` 与本地测试套件全部通过；覆盖严格选号（`lyria-3.5` 不再兜底）、空目录账号不接单、引擎B `Ready` 要求 Worker、多模态不跨引擎、`alternate` 跳过未声明号池、模型探测要求声明等用例；重构后的提交可独立编译。

> AI生成
