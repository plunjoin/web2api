package custom

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/model"
)

func TestProviderScopeCandidatesAndPick(t *testing.T) {
	e := New(config.CustomConfig{}, nil)
	for _, base := range []string{"https://a.example/v1/", "https://a.example", "https://b.example"} {
		e.accounts = append(e.accounts, &customAccount{
			creds: Credentials{BaseURL: base}, enabled: true, ready: true,
			models: []model.ModelInfo{{ID: "shared", Available: true}},
		})
	}
	ctx := WithProviderScope(engine.WithProbeContext(context.Background()), " custom|https://a.example/v1/ ")
	usable, declared, total := e.candidatesCtx(ctx, "models/SHARED")
	if len(usable) != 2 || declared != 2 || total != 2 {
		t.Fatalf("scope candidates: usable=%d declared=%d total=%d", len(usable), declared, total)
	}
	for i := 0; i < 6; i++ {
		acc, err := e.pickCtx(ctx, "shared")
		if err != nil || acc == e.accounts[2] {
			t.Fatalf("pick outside scope: acc=%v err=%v", acc, err)
		}
	}
	for _, acc := range e.accounts[:2] {
		acc.enabled = false
	}
	if _, err := e.pickCtx(ctx, "shared"); err == nil {
		t.Fatal("unavailable provider fell back to another provider")
	}
	if _, err := e.pickCtx(WithProviderScope(ctx, "https://missing.example"), "shared"); err == nil {
		t.Fatal("missing provider must fail")
	}
	if _, err := e.pickCtx(ctx, "unknown"); !engine.IsModelNotFound(err) {
		t.Fatalf("undeclared model must fail: %v", err)
	}
	// 空 provider 和无 scope 的生产候选不受影响。
	plain := WithProviderScope(context.Background(), " ")
	usable, declared, total = e.candidatesCtx(plain, "shared")
	if len(usable) != 1 || usable[0] != e.accounts[2] || declared != 3 || total != 3 {
		t.Fatalf("default candidates changed: usable=%d declared=%d total=%d", len(usable), declared, total)
	}
}

// 管理台保存顺序后，候选账号严格按顺序排列（供应商之间与供应商内部一致）。
func TestAccountOrderDrivesCandidates(t *testing.T) {
	e := New(config.CustomConfig{}, nil)
	for _, acc := range []*customAccount{
		{id: "1", creds: Credentials{BaseURL: "https://a.example"}, enabled: true, ready: true, models: []model.ModelInfo{{ID: "shared", Available: true}}},
		{id: "2", creds: Credentials{BaseURL: "https://a.example"}, enabled: true, ready: true, models: []model.ModelInfo{{ID: "shared", Available: true}}},
		{id: "3", creds: Credentials{BaseURL: "https://b.example"}, enabled: true, ready: true, models: []model.ModelInfo{{ID: "shared", Available: true}}},
	} {
		e.accounts = append(e.accounts, acc)
	}
	e.SetAccountOrder([]string{"3", "1", "2"})
	for i := 0; i < 3; i++ {
		usable, _, _ := e.candidatesCtx(context.Background(), "shared")
		ids := make([]string, 0, len(usable))
		for _, acc := range usable {
			ids = append(ids, acc.id)
		}
		if len(ids) != 3 || ids[0] != "3" || ids[1] != "1" || ids[2] != "2" {
			t.Fatalf("candidates order = %v, want [3 1 2]", ids)
		}
	}
	// 清空顺序后恢复轮询：连续调用不应固定从同一个账号开始。
	e.SetAccountOrder(nil)
	first := map[string]int{}
	for i := 0; i < 6; i++ {
		usable, _, _ := e.candidatesCtx(context.Background(), "shared")
		first[usable[0].id]++
	}
	if len(first) < 2 {
		t.Fatalf("round-robin did not rotate: %v", first)
	}
}

func TestStripChatToolsFlattensHistory(t *testing.T) {
	req := model.ChatRequest{
		Model:      "MiniMax-M2",
		Tools:      []model.Tool{{Type: "function", Function: &model.ToolFunc{Name: "exec_command"}}},
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
