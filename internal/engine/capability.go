package engine

import "strings"

// EngineCaps 描述「引擎 × 模型族」对 function calling 与消息清洗的兼容
// 行为。适配器据此决定 IR → 上游报文的改写方式；路由层不查能力表——
// 引擎选择仍由模型目录与可用性决定。
//
// 首版能力表（完整背景见 docs/web2api-responses-adapter-plan.md §3）：
//
//	能力                        antigravity+Gemini  antigravity+其他  custom/MiniMax  b/codex(OpenAI 兼容)
//	透传 function tools         是                  否（先剥）        是              是
//	assistant content=null      不适用(Text 协议)   折文本时清洗      出站改 ""       出站改 ""
//	空 tool-only assistant      保留                去掉              按上游          按上游
//	tool 历史折成 user 文本      否                  是                仅 400 降级时   仅 400 降级时
//	带 tools 的 400 无工具重试  是                  —                 是              否
//
// assistant content=null → "" 的改写在 OpenAI 兼容出站客户端
// （engine/upstream.chatPayload）统一执行，故不设能力位。
// 失败用量记 Engine 是全局行为（Manager.Chat/ChatStream 出错仍返回带
// Engine 的 ChatResult），同样不设能力位。
type EngineCaps struct {
	// Tools 是否把 function tools / tool_choice 下发给上游。
	Tools bool
	// FlattenToolHistory 无函数调用通道：把 tool 历史折成 user 文本、
	// 丢弃只含 tool_calls 的空 assistant（Claude 清洗 / 纯文本引擎）。
	FlattenToolHistory bool
	// RetryNoToolsOn400 带 tools 的请求被上游 400 拒绝时，去掉 tools
	// 降级重试一次，保证纯文本对话仍可用。
	RetryNoToolsOn400 bool
}

// antigravityCaps / customCaps / openaiCompatCaps 是能力表首版的具名值，
// 适配器与测试都从这里取，避免字面量散落。
var (
	antigravityGeminiCaps = EngineCaps{Tools: true, RetryNoToolsOn400: true}
	antigravityPlainCaps  = EngineCaps{FlattenToolHistory: true}
	customCaps            = EngineCaps{Tools: true, RetryNoToolsOn400: true}
	openaiCompatCaps      = EngineCaps{Tools: true}
	plainTextCaps         = EngineCaps{FlattenToolHistory: true}
)

// CapsFor 查询能力表。engine 是引擎注册名（a/b/codex/antigravity/kiro/
// workbuddy/custom），未知引擎一律取保守值：不透传工具、折纯文本。
func CapsFor(engine, modelName string) EngineCaps {
	switch engine {
	case "antigravity":
		// Antigravity 目录里只有 Gemini 系模型有可用的 function calling
		// 通道；Claude 等模型不收 Gemini tools 协议，先剥成纯文本。
		if antigravityGeminiFamily(modelName) {
			return antigravityGeminiCaps
		}
		return antigravityPlainCaps
	case "custom":
		// MiniMax 等第三方 OpenAI 兼容上游能收 function tools，但 Codex
		// 自带的复杂 schema 常被整请求 400，需要无工具降级兜底。
		return customCaps
	case "b", "codex":
		// AI Studio / Codex 官方端点：协议标准，直接透传。
		return openaiCompatCaps
	default:
		// a / kiro / workbuddy 及未知引擎：没有函数调用通道。
		return plainTextCaps
	}
}

// antigravityGeminiFamily Antigravity 上的模型是否属于 Gemini 系。
func antigravityGeminiFamily(modelName string) bool {
	m := strings.ToLower(strings.TrimSpace(modelName))
	m = strings.TrimPrefix(m, "models/")
	return strings.Contains(m, "gemini")
}
