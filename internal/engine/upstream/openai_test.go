package upstream

import (
	"encoding/json"
	"strings"
	"testing"

	"web2api/internal/model"
)

func TestChatPayloadToolChoiceNotBase64(t *testing.T) {
	o := New("t", "http://x", "", false, 0)
	req := model.ChatRequest{
		Model:      "m",
		Messages:   []model.ChatMessage{{Role: "user", Content: "hi"}},
		Tools:      []model.Tool{{Type: "function", Function: &model.ToolFunc{Name: "noop"}}},
		ToolChoice: json.RawMessage(` "auto" `),
	}
	data, err := json.Marshal(o.chatPayload(req, false))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tool_choice":"auto"`) {
		t.Fatalf("tool_choice must be raw JSON, got %s", data)
	}
}

func TestEndpointVersionedBase(t *testing.T) {
	cases := map[string]string{
		"https://api.example.com":                                 "https://api.example.com/v1/chat/completions",
		"https://api.example.com/v1/":                             "https://api.example.com/v1/chat/completions",
		"https://generativelanguage.googleapis.com/v1beta/openai": "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		"https://open.bigmodel.cn/api/paas/v4":                    "https://open.bigmodel.cn/api/paas/v4/chat/completions",
		"https://example.com/openai":                              "https://example.com/openai/v1/chat/completions",
	}
	for base, want := range cases {
		o := New("t", base, "", false, 0)
		if got := o.endpoint("/v1/chat/completions"); got != want {
			t.Errorf("base %q: got %q want %q", base, got, want)
		}
	}
}

func TestWithThoughtSignatures(t *testing.T) {
	sig := json.RawMessage(`{"google":{"thought_signature":"real-sig"}}`)
	model.RememberToolCallExtra("call_cached", sig)
	msgs := []model.ChatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call_cached", Type: "function"}}},
		{Role: "tool", ToolCallID: "call_cached", Content: "ok"},
		{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call_unknown", Type: "function"}, {ID: "call_2", Type: "function"}}},
	}
	out := withThoughtSignatures("gemini-3.5-flash", msgs)
	if string(out[1].ToolCalls[0].ExtraContent) != string(sig) {
		t.Fatalf("cached signature not restored: %s", out[1].ToolCalls[0].ExtraContent)
	}
	if string(out[3].ToolCalls[0].ExtraContent) != string(dummyThoughtSignature) {
		t.Fatalf("missing signature should fall back to dummy: %s", out[3].ToolCalls[0].ExtraContent)
	}
	if len(out[3].ToolCalls[1].ExtraContent) != 0 {
		t.Fatalf("only the first call of a turn needs a signature")
	}
	if len(msgs[3].ToolCalls[0].ExtraContent) != 0 {
		t.Fatalf("input messages must not be mutated")
	}
	if got := withThoughtSignatures("gpt-4o", msgs); len(got[3].ToolCalls[0].ExtraContent) != 0 {
		t.Fatalf("non-gemini models must be left untouched")
	}
}
