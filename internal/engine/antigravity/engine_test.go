package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"web2api/internal/model"
)

func TestToAntigravityRequestMapsToolsAndToolMessages(t *testing.T) {
	req := model.ChatRequest{
		Model: "gemini-3.8-flash",
		Tools: []model.Tool{{
			Type: "function",
			Function: &model.ToolFunc{
				Name:       "exec_command",
				Parameters: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}}}`),
			},
		}},
		ToolChoice: json.RawMessage(`"auto"`),
		Messages: []model.ChatMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", ToolCalls: []model.ToolCall{{
				ID: "call_1", Type: "function",
				Function:     model.ToolFunction{Name: "exec_command", Arguments: `{"cmd":"ls"}`},
				ExtraContent: json.RawMessage(`{"google":{"thought_signature":"sig123"}}`),
			}}},
			{Role: "tool", ToolCallID: "call_1", Name: "exec_command", Content: "ok"},
		},
	}
	out := toAntigravityRequest(req, "proj")
	if len(out.Tools) != 1 || out.Tools[0].Name != "exec_command" {
		t.Fatalf("tools=%+v", out.Tools)
	}
	if len(out.Messages) != 3 {
		t.Fatalf("messages=%d", len(out.Messages))
	}
	if len(out.Messages[1].ToolCalls) != 1 || out.Messages[1].ToolCalls[0].ThoughtSig != "sig123" {
		t.Fatalf("assistant tool call=%+v", out.Messages[1].ToolCalls)
	}
	if out.Messages[2].Role != "tool" || out.Messages[2].ToolCallID != "call_1" {
		t.Fatalf("tool msg=%+v", out.Messages[2])
	}
}

func TestToAntigravityRequestStripsToolsForClaude(t *testing.T) {
	req := model.ChatRequest{
		Model: "claude-sonnet-4-5",
		Tools: []model.Tool{{
			Type:     "function",
			Function: &model.ToolFunc{Name: "exec_command", Parameters: json.RawMessage(`{}`)},
		}},
		ToolChoice: json.RawMessage(`"auto"`),
		Messages: []model.ChatMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "c1", Type: "function", Function: model.ToolFunction{Name: "exec_command", Arguments: `{}`}}}},
			{Role: "tool", Name: "exec_command", ToolCallID: "c1", Content: "ok"},
		},
	}
	out := toAntigravityRequest(req, "proj")
	if len(out.Tools) != 0 {
		t.Fatalf("claude should not receive tools, got %d", len(out.Tools))
	}
	if len(out.ToolChoice) != 0 {
		t.Fatalf("claude should not receive tool_choice")
	}
	// 只含 tool_calls 的空 assistant 应被丢掉，tool 折成 user：user + user。
	if len(out.Messages) != 2 {
		t.Fatalf("want 2 messages after strip, got %#v", out.Messages)
	}
	for _, m := range out.Messages {
		if len(m.ToolCalls) != 0 {
			t.Fatalf("assistant tool_calls should be stripped for claude")
		}
		if strings.EqualFold(m.Role, "tool") {
			t.Fatalf("tool role should be flattened for claude")
		}
		if strings.EqualFold(m.Role, "assistant") {
			t.Fatalf("empty tool-only assistant should be dropped, got %#v", m)
		}
	}
	if out.Messages[1].Role != "user" || !strings.Contains(out.Messages[1].Text, "ok") {
		t.Fatalf("tool fold=%#v", out.Messages[1])
	}
}

func TestToAntigravityRequestKeepsToolsForGemini(t *testing.T) {
	req := model.ChatRequest{
		Model: "gemini-3-flash-preview",
		Tools: []model.Tool{{
			Type:     "function",
			Function: &model.ToolFunc{Name: "exec_command", Parameters: json.RawMessage(`{"type":"object"}`)},
		}},
		Messages: []model.ChatMessage{{Role: "user", Content: "hi"}},
	}
	out := toAntigravityRequest(req, "")
	if len(out.Tools) != 1 || out.Tools[0].Name != "exec_command" {
		t.Fatalf("gemini tools=%v", out.Tools)
	}
}
