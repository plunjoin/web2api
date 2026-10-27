# web2api Responses 分层适配方案

> 用「一条 Responses 入口 + 内部协议 + 按引擎适配」替代共享路径上的反复打补丁，避免 Codex / Claude / Gemini / MiniMax 再被同一处转换拖垮。

页面版（可在 Grok Bot 里继续改）：见对话中的 Page「web2api Responses 分层适配方案」。

---

## 1. 问题与目标

### 反复翻车的根因

Codex（CC Switch）走 `/v1/responses` 时，先被收成一份共享 `ChatRequest`，再丢给 Antigravity / Custom / OpenAI 兼容上游。平台差异（非 function 工具、content=null、tool 回填 name、空 tool-only turn）有的堵在共享转换，有的堵在引擎里，补丁互相打架。

### 目标

目标不是「每个平台各做一套 `/v1/responses`」，而是：

- 一条公开入口
- 一份内部协议（IR）
- 多个上游适配器

新平台只加适配器与能力行，不再改 Responses 主路径。

### 这样做

- 入口统一：Codex / 面板 / 第三方都打 `/v1/responses` 或 `/v1/chat/completions`
- 差异下沉：工具协议、空 content、400 降级留在引擎适配器
- 可测：能力表 + 单测/mock，回归可复现

### 明确不做

- 不为每个上游复制一套 HTTP Responses handler
- 不在共享 `toChatTools` 里按模型 if/else 堆特例
- 不静默吞掉工具失败（退化要可观测、可记用量引擎）

---

## 2. 目标架构

```
客户端 (Codex / CC Switch / SDK)
    ↓
Wire 入站 (/v1/responses · /v1/chat)
    ↓  只保留 function
内部 IR (规范化消息 + function tools)
    ↓
路由 (选引擎 · 查能力表 · 记 Engine)
    ├─ Antigravity 适配 (Gemini tools · Claude 清洗)
    ├─ OpenAI 兼容适配 (content 规范化 · thought)
    └─ Custom / MiniMax (400 无工具重试)
    ↓
Wire 出站 (编回 Responses / Chat · 用量)
```

Wire 层只做格式互转与「不明工具类型」策略（丢弃或折成文本并打日志）。能力表决定这次请求能不能带 tools、要不要空 content、失败是否无工具重试、是否回填 `call_id→name`。适配器独占上游报文与降级，路由层统一写 `Engine`。

---

## 3. 能力表（首版）

按引擎+模型族扩展；未知项默认保守（不透传危险能力）。

| 能力 | Antigravity Gemini | Antigravity Claude | Custom / MiniMax | OpenAI 兼容 |
| --- | --- | --- | --- | --- |
| 透传 function tools | 是 | 否（先剥） | 是，400 可降级 | 是 |
| 非 function 工具入站 | 入口丢弃 | 入口丢弃 | 入口丢弃 | 入口丢弃 |
| assistant content=null | 按上游 | 清洗 | 改成 `""` | 改成 `""` |
| 空 tool-only assistant | 保留 | 去掉 | 按上游 | 按上游 |
| function_call_output 补 name | 需要 | 需要 | 需要 | 按上游 |
| 带 tools 的 400 重试 | 无工具再试 | 无工具再试 | 无工具再试 | 可选 |
| 失败用量写 Engine | 是 | 是 | 是 | 是 |

---

## 4. 落地分阶段

### 阶段 0：冻结线上，收口现有补丁

不发版。把本机未提交改动收进独立分支：Responses 规范化、Custom 400 降级、失败记 Engine。用 mock 固定 Codex Responses + tools 的回归用例。

### 阶段 1：抽出 IR 与能力表

新增内部类型（规范化 messages/tools）与 Capability 查询（引擎+模型）。`toChatTools` / `parseResponsesInput` 只产出 IR；不明工具类型在此统一处理并打日志。

### 阶段 2：按引擎拆适配器

Antigravity / OpenAI-compat / Custom 各自 IR→上游、结果→Chat。把现在散落的 `stripChatTools`、`normalizeAssistantContent`、Claude 清洗、400 重试挪进对应适配器，共享路由不再 if 模型名。

### 阶段 3：契约测试矩阵

每个适配器至少覆盖：纯文本、function tools、Codex `local_shell` 入站被剥、上游 400 降级、失败用量含 Engine。CI 跑 gateway/provider/upstream 包。

### 阶段 4：再发版

你确认后再合并、打 tag（建议 v0.3.9）、通知 @bllii 用真号本地测 Claude/Gemini/MiniMax + CC Switch Responses。

### 进度（as of 2026-10-28）

- [x] 阶段 0 补丁收进 `refactor/responses-adapter` 分支（95a9a6a，未合并）
- [x] IR + 能力表代码落地（`internal/ir.NormalizeChat` + `provider.CapsFor`，617c8bd）
- [x] 三引擎适配器边界清晰、共享路由无模型特例（能力表驱动；local_shell/apply_patch 入站从「降级成 function」改为「丢弃并记日志」，见 §5.2）
- [x] 契约测试矩阵进 CI（`.github/workflows/test.yml`，go test ./...）
- [x] 引擎目录统一（同分支 refactor/engine-packages）：实现收进 `internal/engine/<name>/`，
  引擎标识 a/b 改名 geminiweb/aistudio（config 旧键与存量 DB 行读时兼容）
- [ ] 发版与 bllii 真号验收

---

## 5. 验收与边界

1. CC Switch `wire_api=responses` 带 tools：Claude / Gemini / MiniMax 不再因共享转换或错误透传整请求 4xx/5xx
2. 入站 `local_shell` / `apply_patch` 等非 function 工具不进入上游报文；行为可测、可日志
3. 失败用量明细始终能显示引擎来源（不再是「—」）
4. 新增上游时只需：能力表一行 + 适配器实现 + 契约测试，无需改 `handleResponses` 主流程

### 和「每平台一份 Responses」的关系

公开路由仍然只有一份。平台兼容写在适配器里，最后都汇入同一条 `/v1/responses`（及 Chat）出站编码。这样既按平台隔离，又不会复制 N 套 HTTP 与鉴权/用量逻辑。

### 建议下一动作

按阶段 0→1 开重构分支 PR（不合并不发版），把现有未提交补丁作为阶段 0 收进去。
