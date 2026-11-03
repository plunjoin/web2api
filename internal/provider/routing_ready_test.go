package provider

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"web2api/internal/engine"
	"web2api/internal/model"
	"web2api/internal/store"
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

func newTestManager(engines ...engine.Engine) *Manager {
	m := &Manager{engines: map[string]engine.Engine{}, defaultEngine: "auto"}
	for _, e := range engines {
		m.engines[e.Name()] = e
		m.engineOrder = append(m.engineOrder, e.Name())
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

// 带 tools 的请求落到不支持 function calling 的引擎（fakeEngine 未实现
// ToolEngine）时，必须显式报错，而不是静默退化成「忽略 tools、仅回文本」。
// 这正是 CC Switch / Codex 经网关请求 geminiweb / workbuddy / kiro 时
// 「静默没工具行为」的根因：工具被吞但请求照常「成功」。
func TestChatWithToolsOnNonToolEngineRejected(t *testing.T) {
	m := newTestManager(&fakeEngine{name: "geminiweb", ready: true, models: []string{"gemini-flash"}})
	req := model.ChatRequest{
		Model: "gemini-flash",
		Tools: []model.Tool{{Type: "function", Function: &model.ToolFunc{Name: "exec_command"}}},
	}
	if _, err := m.Chat(context.Background(), req); err == nil {
		t.Fatal("want tools_not_supported error, got nil")
	} else if !IsToolsNotSupported(err) {
		t.Fatalf("want tools_not_supported, got %v", err)
	}
	if _, err := m.ChatStream(context.Background(), req, model.TextSink(nil)); err == nil {
		t.Fatal("ChatStream: want tools_not_supported error, got nil")
	} else if !IsToolsNotSupported(err) {
		t.Fatalf("ChatStream: want tools_not_supported, got %v", err)
	}
}

// 带 tools 的请求落到支持 function calling 的引擎时，不应被工具闸门拦截。
func TestChatWithToolsOnToolEngineAllowed(t *testing.T) {
	m := newTestManager(&toolEngine{fakeEngine: &fakeEngine{name: "aistudio", ready: true, models: []string{"gemini-flash"}}})
	req := model.ChatRequest{
		Model: "gemini-flash",
		Tools: []model.Tool{{Type: "function", Function: &model.ToolFunc{Name: "exec_command"}}},
	}
	if _, err := m.Chat(context.Background(), req); err != nil {
		t.Fatalf("tool-capable engine should not be blocked, got %v", err)
	}
}

// toolEngine 是实现了 engine.ToolEngine 的 fakeEngine，用于验证「支持工具的
// 引擎不被工具闸门拦截」。
type toolEngine struct{ *fakeEngine }

func (toolEngine) ChatStreamSink(context.Context, model.ChatRequest, model.StreamSink) (*model.ChatResult, error) {
	return &model.ChatResult{}, nil
}

// 号池挂了（default_engine 选中引擎 Ready=false）时，按「切换不算失败」语义落到
// 另一个 Ready 引擎，而不是报该引擎的不可用错误。
func TestAlternateFallsThroughOnDeadDefaultEngine(t *testing.T) {
	m := newTestManager(
		&fakeEngine{name: "geminiweb", ready: false, models: []string{}},
		&fakeEngine{name: "aistudio", ready: true, models: []string{}},
	)
	m.defaultEngine = "geminiweb"
	// "foo" 不被任何目录声明，走 default_engine 路径（owned=false）。
	eng, _, err := m.resolveChatEngine(model.ChatRequest{Model: "foo"})
	if err != nil {
		t.Fatalf("switching is not a failure: got error %v", err)
	}
	if eng == nil || eng.Name() != "aistudio" {
		t.Fatalf("dead pool should fall through to a Ready engine, got %v", eng)
	}
}

// custom 透明透传引擎不再因「目录未命中」被整体跳过：当它是唯一 Ready 的引擎时，
// 旧实现会返回「没有可用引擎」，新实现会把它作为兜底（切换不算失败）。
func TestAlternateNoLongerSkipsCustomOnCatalogMiss(t *testing.T) {
	m := newTestManager(
		&fakeEngine{name: "geminiweb", ready: false, models: []string{}},
		&fakeEngine{name: "custom", ready: true, models: []string{"models/gpt-5"}},
	)
	m.defaultEngine = "geminiweb"
	// "foo" 不在 custom 目录里，但 custom 按 base_url 透传，仍应作为兜底接单。
	eng, _, err := m.resolveChatEngine(model.ChatRequest{Model: "foo"})
	if err != nil {
		t.Fatalf("custom should be a valid fallback: got error %v", err)
	}
	if eng == nil || eng.Name() != "custom" {
		t.Fatalf("custom (only Ready engine) should be chosen, got %v", eng)
	}
}

// 下架按裸名生效：带 models/ 前缀下架后，裸名请求不能再被同名的 custom 目录认领。
func TestDisabledModelNotClaimedByCustom(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.SetModelEnabled("models/GPT-4o", false); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(
		&fakeEngine{name: "aistudio", ready: true, models: []string{"gemini-flash"}},
		&fakeEngine{name: "custom", ready: true, models: []string{"models/gpt-4o"}},
	)
	m.st = st
	eng, _ := m.pickEngine("gpt-4o")
	if eng != nil {
		t.Fatalf("下架的 gpt-4o 不应被任何引擎认领, got %s", eng.Name())
	}
	if _, _, err := m.resolveChatEngine(model.ChatRequest{Model: "gpt-4o"}); !engine.IsModelDisabled(err) {
		t.Fatalf("下架模型应返回 model_unavailable, got %v", err)
	}
	// 公开目录同样不展示。
	for _, mi := range m.ListModels() {
		if model.CanonicalModelID(mi.ID) == "gpt-4o" {
			t.Fatalf("下架模型不应出现在公开目录: %+v", mi)
		}
	}
}

// 主引擎挂了也不能把已下架的模型换到同名的第三方 Key 上。
func TestAlternateSkipsDisabledModel(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.SetModelEnabled("gpt-4o", false); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(
		&fakeEngine{name: "geminiweb", ready: false, models: []string{}},
		&fakeEngine{name: "custom", ready: true, models: []string{"models/gpt-4o"}},
	)
	m.st = st
	m.defaultEngine = "geminiweb"
	if alt := m.alternate(m.engines["geminiweb"], "gpt-4o"); alt != nil {
		t.Fatalf("下架模型不应有备胎引擎, got %s", alt.Name())
	}
}
