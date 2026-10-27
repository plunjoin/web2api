package provider

import (
	"encoding/json"
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
