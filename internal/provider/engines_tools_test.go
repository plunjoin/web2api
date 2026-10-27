package provider

import (
	"encoding/json"
	"errors"
	"testing"

	"web2api/internal/model"
)

func TestStripChatToolsFlattensHistory(t *testing.T) {
	req := model.ChatRequest{
		Model: "MiniMax-M2",
		Tools: []model.Tool{{Type: "function", Function: &model.ToolFunc{Name: "exec_command"}}},
		ToolChoice: json.RawMessage(`"auto"`),
		Messages: []model.ChatMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: nil, ToolCalls: []model.ToolCall{{ID: "c1", Type: "function", Function: model.ToolFunction{Name: "exec_command", Arguments: "{}"}}}},
			{Role: "tool", Name: "exec_command", ToolCallID: "c1", Content: "ok"},
		},
	}
	out := stripChatTools(req)
	if len(out.Tools) != 0 || len(out.ToolChoice) != 0 {
		t.Fatalf("tools should be cleared: tools=%d choice=%s", len(out.Tools), out.ToolChoice)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("tool-only assistant should be dropped, tool folded to user: %#v", out.Messages)
	}
	if out.Messages[1].Role != "user" || out.Messages[1].Content != "[exec_command] ok" {
		t.Fatalf("tool fold=%#v", out.Messages[1])
	}
}

func TestIsHTTPStatus(t *testing.T) {
	if !isHTTPStatus(errors.New("上游返回 HTTP 400: bad tools"), 400) {
		t.Fatal("want true")
	}
	if isHTTPStatus(errors.New("上游返回 HTTP 429"), 400) {
		t.Fatal("429 is not 400")
	}
}
