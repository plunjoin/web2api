package ir

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"web2api/internal/model"
)

func TestNormalizeChatDropsNonFunctionTools(t *testing.T) {
	req := model.ChatRequest{
		Model: "gemini-flash",
		Tools: []model.Tool{
			{Type: "web_search"},
			{Type: "function", Function: &model.ToolFunc{Name: "exec_command"}},
			{Type: "local_shell"},
		},
		ToolChoice: json.RawMessage(`"auto"`),
		Messages:   []model.ChatMessage{{Role: "user", Content: "hi"}},
	}
	var logs []string
	out := NormalizeChat(req, func(format string, args ...any) {
		logs = append(logs, strings.TrimSpace(fmt.Sprintf(format, args...)))
	})
	if len(out.Tools) != 1 || out.Tools[0].Function.Name != "exec_command" {
		t.Fatalf("only function tool should survive: %#v", out.Tools)
	}
	if out.Tools[0].Type != "function" {
		t.Fatalf("tool type should be normalized to function: %#v", out.Tools[0])
	}
	if len(logs) == 0 || !strings.Contains(logs[0], "web_search") || !strings.Contains(logs[0], "local_shell") {
		t.Fatalf("dropped tools should be logged: %v", logs)
	}
	if string(out.ToolChoice) != `"auto"` {
		t.Fatalf("tool_choice should stay when tools survive: %s", out.ToolChoice)
	}
}

func TestNormalizeChatClearsToolChoiceWhenAllToolsDropped(t *testing.T) {
	req := model.ChatRequest{
		Tools:      []model.Tool{{Type: "web_search"}},
		ToolChoice: json.RawMessage(`"auto"`),
		Messages:   []model.ChatMessage{{Role: "user", Content: "hi"}},
	}
	out := NormalizeChat(req, nil)
	if out.Tools != nil || out.ToolChoice != nil {
		t.Fatalf("tools/tool_choice should be cleared: tools=%v choice=%s", out.Tools, out.ToolChoice)
	}
}

func TestNormalizeChatFillsToolNameAndEmptyContent(t *testing.T) {
	req := model.ChatRequest{
		Messages: []model.ChatMessage{
			{Role: "user", Content: "run ls"},
			{Role: "assistant", Content: nil, ToolCalls: []model.ToolCall{
				{ID: "call_1", Type: "function", Function: model.ToolFunction{Name: "exec_command", Arguments: "{}"}},
			}},
			{Role: "tool", ToolCallID: "call_1", Content: "ok"}, // 无 name，Chat 客户端常见
		},
	}
	out := NormalizeChat(req, nil)
	if out.Messages[1].Content != "" {
		t.Fatalf("assistant content must be empty string: %#v", out.Messages[1].Content)
	}
	if out.Messages[2].Name != "exec_command" {
		t.Fatalf("tool name should be filled from call history: %#v", out.Messages[2])
	}
	data, _ := json.Marshal(out.Messages[1])
	if strings.Contains(string(data), `"content":null`) {
		t.Fatalf("content must not marshal as null: %s", data)
	}
}

func TestNormalizeChatIdempotent(t *testing.T) {
	req := model.ChatRequest{
		Messages: []model.ChatMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "", ToolCalls: []model.ToolCall{
				{ID: "c1", Type: "function", Function: model.ToolFunction{Name: "f", Arguments: "{}"}},
			}},
			{Role: "tool", Name: "f", ToolCallID: "c1", Content: "ok"},
		},
	}
	once := NormalizeChat(req, nil)
	if len(once.Tools) != 0 || once.ToolChoice != nil {
		t.Fatalf("unexpected tools: %v", once.Tools)
	}
	// 已满足不变量的消息切片应原样返回（不拷贝）。
	if &once.Messages[0] != &req.Messages[0] {
		t.Fatal("normalized messages should not be copied when already conformant")
	}
	// 幂等：再跑一遍结果不变。
	twice := NormalizeChat(once, nil)
	if len(twice.Messages) != len(once.Messages) || twice.Messages[2].Name != "f" {
		t.Fatalf("normalize must be idempotent: %#v", twice.Messages)
	}
}
