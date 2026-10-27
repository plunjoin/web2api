package provider

import (
	"context"
	"io"
	"testing"

	"web2api/internal/engine"
	"web2api/internal/model"
)

type fakeEngine struct {
	name      string
	ready     bool
	models    []string
	chatErr   error
	streamErr error
}

func (f *fakeEngine) Name() string     { return f.name }
func (f *fakeEngine) Describe() string { return f.name }
func (f *fakeEngine) Ready() bool      { return f.ready }
func (f *fakeEngine) ListModels() []model.ModelInfo {
	out := make([]model.ModelInfo, 0, len(f.models))
	for _, id := range f.models {
		out = append(out, model.ModelInfo{ID: id, Engine: f.name, Available: f.ready, Kind: "chat"})
	}
	return out
}
func (f *fakeEngine) Chat(context.Context, model.ChatRequest) (*model.ChatResult, error) {
	if f.chatErr != nil {
		return nil, f.chatErr
	}
	return nil, nil
}
func (f *fakeEngine) ChatStream(context.Context, model.ChatRequest, model.ChatStreamFunc) (*model.ChatResult, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	return nil, nil
}
func (f *fakeEngine) Passthrough() bool { return false }
func (f *fakeEngine) ServePassthrough(context.Context, string, string, io.Reader, string, engine.PassthroughWriter) error {
	return nil
}
func (f *fakeEngine) Status() map[string]any { return nil }

func newTestManager(engines ...*fakeEngine) *Manager {
	m := &Manager{engines: map[string]engine.Engine{}, defaultEngine: "auto"}
	for _, e := range engines {
		m.engines[e.name] = e
		m.engineOrder = append(m.engineOrder, e.name)
	}
	return m
}

// 号池为空的订阅引擎（静态目录里有 gemini-2.5-flash）不能抢走 custom 的同名模型。
func TestEmptySubscriptionEngineDoesNotClaimModel(t *testing.T) {
	m := newTestManager(
		&fakeEngine{name: "antigravity", ready: false, models: []string{"gemini-2.5-flash", "claude-sonnet-4-5"}},
		&fakeEngine{name: "workbuddy", ready: false, models: []string{"gemini-2.5-flash"}},
		&fakeEngine{name: "custom", ready: true, models: []string{"models/gemini-2.5-flash"}},
	)
	eng, owned := m.pickEngine("gemini-2.5-flash")
	if eng == nil || eng.Name() != "custom" || !owned {
		t.Fatalf("gemini-2.5-flash routed to %v (owned=%v), want custom", eng, owned)
	}
}

func TestReadySubscriptionEngineStillClaimsModel(t *testing.T) {
	m := newTestManager(
		&fakeEngine{name: "antigravity", ready: true, models: []string{"gemini-2.5-flash"}},
		&fakeEngine{name: "custom", ready: true, models: []string{"models/gemini-2.5-flash"}},
	)
	if eng, _ := m.pickEngine("gemini-2.5-flash"); eng == nil || eng.Name() != "antigravity" {
		t.Fatalf("ready antigravity should keep its catalog priority, got %v", eng)
	}
}

func TestPublicModelsHideNotReadySubscriptionEngines(t *testing.T) {
	m := newTestManager(
		&fakeEngine{name: "codex", ready: false, models: []string{"gpt-5.5"}},
		&fakeEngine{name: "custom", ready: true, models: []string{"models/gemini-2.5-flash"}},
	)
	for _, mi := range m.ListModels() {
		if mi.Engine == "codex" {
			t.Fatalf("not-ready codex model %q leaked into public list", mi.ID)
		}
	}
	all := m.ListAllModels()
	if len(all) != 2 {
		t.Fatalf("admin list should keep every model, got %d", len(all))
	}
}
