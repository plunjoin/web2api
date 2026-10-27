package antigravity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildBodyIncludesFunctionDeclarations(t *testing.T) {
	body, err := buildBody(&Request{
		Model: "gemini-3.8-flash",
		Messages: []Message{
			{Role: "user", Text: "list /tmp"},
		},
		Tools: []Tool{{
			Name:        "exec_command",
			Description: "Run a shell command",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}`),
		}},
		ToolChoice: json.RawMessage(`"auto"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Request.Tools) == 0 {
		t.Fatal("expected tools on request body")
	}
	decls, ok := body.Request.Tools[0]["functionDeclarations"].([]map[string]any)
	if !ok || len(decls) != 1 {
		// json round-trip via map may store as []any
		raw, _ := json.Marshal(body.Request.Tools)
		if !strings.Contains(string(raw), `"exec_command"`) {
			t.Fatalf("functionDeclarations missing exec_command: %s", raw)
		}
	}
	cfg, _ := json.Marshal(body.Request.ToolConfig)
	if !strings.Contains(string(cfg), `"AUTO"`) && !strings.Contains(string(cfg), `"auto"`) {
		// mode is AUTO uppercase in our code
		if !strings.Contains(string(cfg), "AUTO") {
			t.Fatalf("toolConfig mode want AUTO, got %s", cfg)
		}
	}
}

func TestBuildBodyToolResultAsFunctionResponse(t *testing.T) {
	body, err := buildBody(&Request{
		Model: "gemini-3.8-flash",
		Messages: []Message{
			{Role: "user", Text: "read file"},
			{Role: "assistant", ToolCalls: []ToolCall{{
				ID: "call_1", Name: "exec_command", Arguments: `{"cmd":"cat a.txt"}`,
			}}},
			{Role: "tool", ToolCallID: "call_1", ToolName: "exec_command", Text: "hello"},
		},
		Tools: []Tool{{Name: "exec_command", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body.Request.Contents)
	if !strings.Contains(string(raw), `"functionCall"`) {
		t.Fatalf("missing functionCall in contents: %s", raw)
	}
	if !strings.Contains(string(raw), `"functionResponse"`) {
		t.Fatalf("missing functionResponse in contents: %s", raw)
	}
	if !strings.Contains(string(raw), "skip_thought_signature_validator") {
		t.Fatalf("expected dummy thought signature on historical functionCall: %s", raw)
	}
}

func TestBuildBodyMergesParallelFunctionResponses(t *testing.T) {
	body, err := buildBody(&Request{
		Model: "gemini-3.8-flash",
		Messages: []Message{
			{Role: "user", Text: "run two"},
			{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "c1", Name: "a", Arguments: `{}`},
				{ID: "c2", Name: "b", Arguments: `{}`},
			}},
			{Role: "tool", ToolCallID: "c1", ToolName: "a", Text: "one"},
			{Role: "tool", ToolCallID: "c2", ToolName: "b", Text: "two"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	userCount := 0
	for _, c := range body.Request.Contents {
		if c.Role != "user" {
			continue
		}
		userCount++
		if userCount == 2 && len(c.Parts) != 2 {
			t.Fatalf("parallel tool results should merge into one user with 2 parts, got %d parts", len(c.Parts))
		}
	}
	if userCount != 2 {
		t.Fatalf("want 2 user turns (prompt + merged tools), got %d", userCount)
	}
}

func TestParseSSEFunctionCallOnly(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"exec_command","args":{"cmd":"ls"}}}]}}]}}`,
		``,
		`data: {"response":{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}}`,
		``,
	}, "\n")
	res, err := parseSSE(strings.NewReader(sse), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ToolCalls) != 1 {
		t.Fatalf("tool calls=%d want 1", len(res.ToolCalls))
	}
	if res.ToolCalls[0].Name != "exec_command" {
		t.Fatalf("name=%q", res.ToolCalls[0].Name)
	}
	if !strings.Contains(res.ToolCalls[0].Arguments, `"cmd"`) {
		t.Fatalf("arguments=%s", res.ToolCalls[0].Arguments)
	}
	if res.Text != "" {
		t.Fatalf("unexpected text %q", res.Text)
	}
}

func TestIsBadRequest(t *testing.T) {
	if !IsBadRequest(&UpstreamError{Status: 400, Body: `{"error":{"message":"bad tools"}}`}) {
		t.Fatal("want true for 400")
	}
	if IsBadRequest(&UpstreamError{Status: 429, Body: "limit"}) {
		t.Fatal("429 should not be bad request")
	}
}
