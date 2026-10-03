# internal/aistudio2api 来源说明

本目录已合并自开源项目 **Mag1cFall/AIStudio2API**（MIT License，见本目录 LICENSE），
作为 web2api 主模块的一部分提供引擎B 的 AI Studio 逆向能力（纯 Go WAA 后端 + 私有协议）。

## 与上游的差异

- 仅保留运行时核心包：`core/{config, waa, camoufoxnative, aistudio}`（原 `internal/`），
  未包含其 web 管理界面、cmd 入口、setup、api 服务器等。
- 原始 import 路径已批量重写为 `web2api/internal/aistudio2api/...`，
  与主项目共用根 `go.mod`。
- **上游 bug 补丁（core/aistudio/accounts.go `AccountPool.Remove`）**：上游在 Remove
  成功路径漏掉复位 `account.exclusive = false`（各失败分支均有复位）。上游删号即弃
  对象所以不暴露；web2api 的号池管理会复用同一 Account 对象（停用/启用/重加），
  不补则对象再入池时永久报 `ErrAccountLeased`。已在成功出池的 `delete(p.byID, ...)`
  之后补一行复位，带 `[web2api 补丁]` 注释标记，其余逻辑未做任何修改。
  上游协议变更时，可对照上游仓库同步这几个包并保留该补丁。

## 相关能力

- `core/aistudio`：AI Studio 私有协议（GenerateContent 流式编码/解码、账号池、
  签名、模型目录、配额、上传、视频等）
- `core/waa`：Google BotGuard（WAA）纯 Go VM——内嵌 goja JS 引擎执行官方下发的
  解释器生成 proof，无需浏览器
- `core/camoufoxnative`：账号指纹、Worker 生命周期、原生（Camoufox）后端（本项目
  只使用其 go 后端与 Options/State 类型）
- `core/config`：协议参数与账号配置类型（单文件、零外部依赖）

> AI生成
