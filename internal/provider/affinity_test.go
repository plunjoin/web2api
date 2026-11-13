package provider

import (
	"context"
	"errors"
	"testing"

	"web2api/internal/engine"
	"web2api/internal/model"
)

// affinityEngine 记录调用顺序，并在成功时上报实际接单账号。
type affinityEngine struct {
	fakeEngine
	account string
	calls   *[]string
}

func (a *affinityEngine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	*a.calls = append(*a.calls, a.name)
	if a.chatErr != nil {
		return nil, a.chatErr
	}
	if a.account != "" {
		engine.NoteServedAccount(ctx, a.account)
	}
	return &model.ChatResult{Engine: a.name, Text: a.name}, nil
}

func stickyRequest(user string) model.ChatRequest {
	return model.ChatRequest{
		Model: "gpt-5",
		Messages: []model.ChatMessage{
			{Role: "system", Content: "you are helpful"},
			{Role: "user", Content: user},
		},
	}
}

// 客户端每轮重发完整历史时，会话键必须保持不变；换会话或显式会话头才变。
func TestConversationKeyStableAcrossTurns(t *testing.T) {
	ctx := context.Background()
	first := stickyRequest("hello")
	later := stickyRequest("hello")
	later.Messages = append(later.Messages,
		model.ChatMessage{Role: "assistant", Content: "hi"},
		model.ChatMessage{Role: "user", Content: "next turn"},
	)
	key := conversationKey(ctx, first)
	if key == "" {
		t.Fatal("conversation key should not be empty")
	}
	if got := conversationKey(ctx, later); got != key {
		t.Fatalf("later turn key = %q, want %q", got, key)
	}
	if got := conversationKey(ctx, stickyRequest("another conversation")); got == key {
		t.Fatal("different first user message must produce a different key")
	}

	hinted := WithSessionHint(ctx, "sess-123")
	if got := conversationKey(hinted, first); got == key {
		t.Fatal("explicit session hint should override the message hash")
	}
	if got := conversationKey(hinted, later); got != conversationKey(hinted, first) {
		t.Fatal("explicit session hint must stay stable across turns")
	}
	if got := conversationKey(ctx, model.ChatRequest{Model: "gpt-5"}); got != "" {
		t.Fatalf("no user message should disable affinity, got %q", got)
	}
}

// 会话粘性把上次接单账号的供应商提到顺序最前，失败仍可切换。
func TestAffinityKeepsPreferredSupplierFirst(t *testing.T) {
	var calls []string
	codex := &affinityEngine{fakeEngine: fakeEngine{name: "codex", ready: true, models: []string{"gpt-5"}}, account: "20", calls: &calls}
	custom := &affinityEngine{fakeEngine: fakeEngine{name: "custom", ready: true, models: []string{"gpt-5"}}, account: "10", calls: &calls}
	m := newTestManager(codex, custom)
	// 正常顺序 codex 在前；粘性映射指向 custom 的账号。
	m.routeIDs = []string{"20", "10"}
	m.routeSupplier = map[string]int{"codex": 0, "custom": 1}
	m.routeEngine = map[string]string{"20": "codex", "10": "custom"}

	ctx := context.Background()
	req := stickyRequest("sticky")
	m.affinity.store(conversationKey(ctx, req), "10")

	if _, err := m.Chat(ctx, req); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(calls) != 1 || calls[0] != "custom" {
		t.Fatalf("sticky account's supplier should go first, got %v", calls)
	}
}

// 粘性账号失败时照常换下家，并把映射刷新到实际接单账号。
func TestAffinityRefreshesToServedAccountAfterFailover(t *testing.T) {
	var calls []string
	codex := &affinityEngine{fakeEngine: fakeEngine{name: "codex", ready: true, models: []string{"gpt-5"}}, account: "20", calls: &calls}
	custom := &affinityEngine{fakeEngine: fakeEngine{name: "custom", ready: true, models: []string{"gpt-5"}}, account: "10", calls: &calls}
	custom.chatErr = errors.New("上游返回 HTTP 500")
	m := newTestManager(codex, custom)
	m.routeIDs = []string{"20", "10"}
	m.routeSupplier = map[string]int{"codex": 0, "custom": 1}
	m.routeEngine = map[string]string{"20": "codex", "10": "custom"}

	ctx := context.Background()
	req := stickyRequest("failover")
	key := conversationKey(ctx, req)
	m.affinity.store(key, "10")

	if _, err := m.Chat(ctx, req); err != nil {
		t.Fatalf("chat should fail over to codex: %v", err)
	}
	if len(calls) != 2 || calls[0] != "custom" || calls[1] != "codex" {
		t.Fatalf("want custom then codex, got %v", calls)
	}
	if got := m.affinity.lookup(key); got != "20" {
		t.Fatalf("affinity should refresh to served account 20, got %q", got)
	}
}

// 关闭粘性后完全按保存的路由顺序选号。
func TestAffinityOffUsesConfiguredOrder(t *testing.T) {
	var calls []string
	codex := &affinityEngine{fakeEngine: fakeEngine{name: "codex", ready: true, models: []string{"gpt-5"}}, account: "20", calls: &calls}
	custom := &affinityEngine{fakeEngine: fakeEngine{name: "custom", ready: true, models: []string{"gpt-5"}}, account: "10", calls: &calls}
	m := newTestManager(codex, custom)
	m.routeIDs = []string{"20", "10"}
	m.routeSupplier = map[string]int{"codex": 0, "custom": 1}
	m.routeEngine = map[string]string{"20": "codex", "10": "custom"}
	if err := m.SetAffinityMode(AffinityOff); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	req := stickyRequest("off")
	m.affinity.store(conversationKey(ctx, req), "10")

	if _, err := m.Chat(ctx, req); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(calls) != 1 || calls[0] != "codex" {
		t.Fatalf("affinity off must follow configured order, got %v", calls)
	}
}

// 粘性设错模式必须被拒绝，而不是悄悄退回默认值。
func TestSetAffinityModeRejectsUnknown(t *testing.T) {
	m := newTestManager()
	if err := m.SetAffinityMode("strict"); err == nil {
		t.Fatal("unsupported mode should be rejected")
	}
}
