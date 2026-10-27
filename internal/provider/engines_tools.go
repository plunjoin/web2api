package provider

import (
	"fmt"
	"strings"

	"web2api/internal/engine"
	"web2api/internal/model"
)

// 编译期断言：支持 function calling 的引擎必须实现 engine.ToolEngine。
//
// 网关按接口断言选择路径（见 Manager.ChatStream），断言失败不会报错，
// 只会静默退化成「忽略 tools、仅回文本」——这种退化在人工测试里极难发现，
// 所以在这里让编译期就拦住。
var (
	_ engine.ToolEngine = (*CustomEngine)(nil)
	_ engine.ToolEngine = (*AIStudioEngine)(nil)
	_ engine.ToolEngine = (*NativeAIStudioEngine)(nil)
	_ engine.ToolEngine = (*CodexEngine)(nil)
	_ engine.ToolEngine = (*AntigravityEngine)(nil)
)

// stripChatTools 去掉 tools / tool_choice，并把 tool 历史折成普通文本。
// 用于上游拒收 Codex 自带工具 schema 时的纯文本降级重试（MiniMax / 部分 Gemini）。
func stripChatTools(req model.ChatRequest) model.ChatRequest {
	out := req
	out.Tools = nil
	out.ToolChoice = nil
	out.ParallelToolCalls = nil
	msgs := make([]model.ChatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch strings.ToLower(m.Role) {
		case "tool":
			text := m.Text()
			if name := strings.TrimSpace(m.Name); name != "" {
				text = "[" + name + "] " + text
			}
			msgs = append(msgs, model.ChatMessage{Role: "user", Content: text})
		case "assistant":
			msg := m
			msg.ToolCalls = nil
			if msg.Content == nil {
				msg.Content = ""
			}
			if strings.TrimSpace(msg.Text()) == "" && len(m.ToolCalls) > 0 {
				continue
			}
			msgs = append(msgs, msg)
		default:
			msgs = append(msgs, m)
		}
	}
	out.Messages = msgs
	return out
}

// isHTTPStatus 判断错误文案是否含「HTTP <code>」（上游 OpenAI 兼容客户端惯用格式）。
func isHTTPStatus(err error, code int) bool {
	if err == nil {
		return false
	}
	needle := strings.ToLower(fmt.Sprintf("http %d", code))
	return strings.Contains(strings.ToLower(err.Error()), needle)
}
