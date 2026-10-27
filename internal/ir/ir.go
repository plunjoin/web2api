// Package ir 定义网关内部协议（IR）。
//
// Wire 入站（/v1/responses、/v1/chat/completions）解析完成后统一经过
// NormalizeChat 规范化；之后路由层、引擎适配器拿到的都是满足下列不变量的
// ChatRequest。适配器只做「IR → 上游报文」的改写，不再各自猜测入站格式，
// 平台差异（工具协议、空 content、400 降级）全部下沉到适配器与能力表
// （见 provider.EngineCaps）。
//
// IR 不变量：
//   - tools 只含 type=function 且函数名非空；不明工具类型（web_search 等
//     服务端内置工具、local_shell 等 Chat 形态的内置工具）在入口丢弃并记
//     日志，不进入任何上游报文
//   - tool_choice 只在有工具存活时保留
//   - 带 tool_calls 的 assistant 消息 content 为 "" 而非 null：
//     encoding/json 会把 nil any 编成 null，MiniMax 等第三方上游直接 400
//   - role=tool 消息尽量补全 name：按 assistant 历史里的 call_id → 函数名
//     映射回填（Gemini functionResponse、MiniMax 都要求 name）
package ir

import (
	"strings"

	"web2api/internal/model"
)

// Logf 规范化过程的日志出口，签名与 log.Logger.Printf 一致；nil 时静默
// 丢弃（单测场景）。
type Logf func(format string, args ...any)

// NormalizeChat 把 Wire 入站解析出的请求规范化为 IR。幂等：已满足不变量
// 的请求原样返回（不拷贝消息切片）。
func NormalizeChat(req model.ChatRequest, logf Logf) model.ChatRequest {
	req.Tools = KeepFunctionTools(req.Tools, logf)
	if len(req.Tools) == 0 {
		req.ToolChoice = nil
	}
	req.Messages = normalizeMessages(req.Messages)
	return req
}

// KeepFunctionTools 只保留 function 工具，丢弃不明类型并记日志。
// Responses 入站的同名转换（gateway.toChatTools）产出已满足本不变量，
// 这里兜住 /v1/chat/completions 直发非 function 工具的客户端。
func KeepFunctionTools(tools []model.Tool, logf Logf) []model.Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]model.Tool, 0, len(tools))
	var dropped []string
	for _, tool := range tools {
		if tool.Function == nil || strings.TrimSpace(tool.Function.Name) == "" {
			typ := strings.TrimSpace(tool.Type)
			if typ == "" {
				typ = "(missing function)"
			}
			dropped = append(dropped, typ)
			continue
		}
		tool.Type = "function"
		out = append(out, tool)
	}
	if len(dropped) > 0 && logf != nil {
		logf("[ir] 丢弃非 function 工具: %s", strings.Join(dropped, ", "))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// normalizeMessages 保证消息级不变量：空 assistant content、tool 消息的
// name 回填。写时拷贝：没有任何消息需要改时不复制原切片。
func normalizeMessages(messages []model.ChatMessage) []model.ChatMessage {
	callNames := map[string]string{}
	for _, m := range messages {
		if !strings.EqualFold(m.Role, "assistant") {
			continue
		}
		for _, call := range m.ToolCalls {
			if name := strings.TrimSpace(call.Function.Name); name != "" {
				callNames[strings.TrimSpace(call.ID)] = name
			}
		}
	}
	var out []model.ChatMessage
	for i, m := range messages {
		var fixed model.ChatMessage
		switch {
		case strings.EqualFold(m.Role, "assistant") && m.Content == nil && len(m.ToolCalls) > 0:
			fixed = m
			fixed.Content = ""
		case strings.EqualFold(m.Role, "tool") && strings.TrimSpace(m.Name) == "":
			name := callNames[strings.TrimSpace(m.ToolCallID)]
			if name == "" {
				continue
			}
			fixed = m
			fixed.Name = name
		default:
			continue
		}
		if out == nil {
			out = append([]model.ChatMessage(nil), messages...)
		}
		out[i] = fixed
	}
	if out == nil {
		return messages
	}
	return out
}
