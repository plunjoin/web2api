package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/engine/custom"
	"web2api/internal/model"
	"web2api/internal/store"
)

type probeRecordingEngine struct {
	*fakeEngine
	kind  string
	calls int
}

func (e *probeRecordingEngine) Chat(context.Context, model.ChatRequest) (*model.ChatResult, error) {
	e.calls++
	return &model.ChatResult{Text: e.name}, e.chatErr
}

func (e *probeRecordingEngine) ListModels() []model.ModelInfo {
	models := e.fakeEngine.ListModels()
	if e.kind != "" {
		for i := range models {
			models[i].Kind = e.kind
		}
	}
	return models
}

func TestProbeModelForTargetEngineIsolation(t *testing.T) {
	first := &probeRecordingEngine{fakeEngine: &fakeEngine{name: "codex", ready: true, models: []string{"shared"}}}
	target := &probeRecordingEngine{fakeEngine: &fakeEngine{name: "custom", ready: true, models: []string{"shared"}}}
	m := newTestManager(first, target)
	ctx := context.Background()
	for _, probe := range []func() (string, error){
		func() (string, error) { return m.ProbeModel(ctx, "shared") },
		func() (string, error) { return m.ProbeModelForTarget(ctx, "shared", "", "") },
	} {
		if detail, err := probe(); err != nil || detail != "codex" {
			t.Fatalf("legacy probe: detail=%q err=%v", detail, err)
		}
	}
	// 全局同名条目的用途不能污染显式目标的用途。
	first.kind = model.KindVideo
	if detail, err := m.ProbeModelForTarget(ctx, "shared", "custom", ""); err != nil || detail != "custom" {
		t.Fatalf("target probe: detail=%q err=%v", detail, err)
	}
	if first.calls != 2 || target.calls != 1 {
		t.Fatalf("wrong engines called: first=%d target=%d", first.calls, target.calls)
	}
	target.ready = false
	for _, name := range []string{"custom", "missing"} {
		if _, err := m.ProbeModelForTarget(ctx, "shared", name, ""); err == nil {
			t.Fatalf("unavailable target %s must fail", name)
		}
	}
	if first.calls != 2 || target.calls != 1 {
		t.Fatal("unavailable target fell back or was called")
	}
	if _, err := m.ProbeModelForTarget(ctx, "shared", "codex", "https://example.com"); err == nil {
		t.Fatal("non-custom provider must be rejected")
	}
}

func TestProbeModelForTargetProviderAndMedia(t *testing.T) {
	var wrong, selected, chat, image, embedding atomic.Int32
	serve := func(calls *atomic.Int32, fail bool) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method != http.MethodPost {
				// 保留静态目录，避免异步刷新改变测试用的用途。
				http.NotFound(w, r)
				return
			}
			calls.Add(1)
			if fail {
				http.Error(w, "limited", http.StatusTooManyRequests)
				return
			}
			switch r.URL.Path {
			case "/v1/chat/completions":
				chat.Add(1)
				_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"selected"}}]}`)
			case "/v1/images/generations":
				image.Add(1)
				_, _ = io.WriteString(w, `{"data":[]}`)
			case "/v1/embeddings":
				embedding.Add(1)
				_, _ = io.WriteString(w, `{"data":[]}`)
			default:
				http.NotFound(w, r)
			}
		}))
	}
	a, b := serve(&wrong, true), serve(&selected, false)
	defer a.Close()
	defer b.Close()
	e := custom.New(config.CustomConfig{}, nil)
	models := []string{"shared", "gpt-image-1", "text-embedding-3-small", "veo-3", "lyria-002", "tts-1"}
	for _, acc := range []struct{ id, url string }{{"a", a.URL}, {"b", b.URL}} {
		if err := e.AddAccount(acc.id, acc.id, custom.Credentials{BaseURL: acc.url + "/v1/", APIKey: "test", Models: models}); err != nil {
			t.Fatal(err)
		}
	}
	other := &probeRecordingEngine{fakeEngine: &fakeEngine{name: "codex", ready: true, models: models}}
	m := newTestManager(other, e)
	ctx := context.Background()
	for _, id := range models {
		if _, err := m.ProbeModelForTarget(ctx, id, "custom", "custom|"+b.URL+"/v1/"); err != nil {
			t.Fatalf("probe %s: %v", id, err)
		}
	}
	if selected.Load() != 3 || wrong.Load() != 0 || other.calls != 0 || chat.Load() != 1 || image.Load() != 1 || embedding.Load() != 1 {
		t.Fatalf("wrong endpoints: selected=%d wrong=%d other=%d chat=%d image=%d embedding=%d", selected.Load(), wrong.Load(), other.calls, chat.Load(), image.Load(), embedding.Load())
	}
	if err := e.SetEnabled("b", false); err != nil {
		t.Fatal(err)
	}
	for _, id := range models {
		if _, err := m.ProbeModelForTarget(ctx, id, "custom", b.URL); err == nil {
			t.Fatalf("disabled provider accepted %s", id)
		}
	}
	for _, provider := range []string{"https://missing.invalid", "custom|"} {
		if _, err := m.ProbeModelForTarget(ctx, "shared", "custom", provider); err == nil {
			t.Fatalf("missing provider %q accepted", provider)
		}
	}
	if selected.Load() != 3 || wrong.Load() != 0 || other.calls != 0 {
		t.Fatal("unavailable target fell back")
	}
	if err := e.SetEnabled("b", true); err != nil {
		t.Fatal(err)
	}
	// 目标上游限流也不能换供应商。
	if _, err := m.ProbeModelForTarget(ctx, "shared", "custom", a.URL); err == nil {
		t.Fatal("limited provider must fail")
	}
	if selected.Load() != 3 || wrong.Load() != 1 || other.calls != 0 {
		t.Fatal("limited provider fell back")
	}
}

func TestAccountOverviewUnloadedCustomProvider(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "overview.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateAccount("custom", "unloaded", `{"base_url":" https://example.com/v1/ "}`, false); err != nil {
		t.Fatal(err)
	}
	m := newTestManager()
	m.st = st
	for _, pool := range []map[string]accountPool{nil, {"custom": custom.New(config.CustomConfig{}, st)}} {
		m.pools = pool
		got := m.AccountOverview()
		if len(got) != 1 || got[0].Provider != "https://example.com" || got[0].Live.Status != "unloaded" {
			t.Fatalf("overview=%+v", got)
		}
	}
}

type fakeEngine struct {
	name        string
	ready       bool
	models      []string
	unavailable map[string]bool // 目录里存在但当前没有可用账号声明的模型
	chatErr     error
	streamErr   error
}

func (f *fakeEngine) Name() string     { return f.name }
func (f *fakeEngine) Describe() string { return f.name }
func (f *fakeEngine) Ready() bool      { return f.ready }
func (f *fakeEngine) ListModels() []model.ModelInfo {
	out := make([]model.ModelInfo, 0, len(f.models))
	for _, id := range f.models {
		out = append(out, model.ModelInfo{
			ID:        id,
			Engine:    f.name,
			Available: f.ready && !f.unavailable[id],
			Kind:      "chat",
		})
	}
	return out
}

// HasAvailableModel 让 fakeEngine 实现 engine.ModelAvailability，用于验证
// 「目录里有、但没有可用账号声明」的模型不会抢走路由。
func (f *fakeEngine) HasAvailableModel(modelName string) bool {
	for _, id := range f.models {
		if model.CanonicalModelID(id) == model.CanonicalModelID(modelName) {
			return f.ready && !f.unavailable[id]
		}
	}
	return false
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

// staticEngine 是不实现 engine.ModelAvailability 的引擎（workbuddy 就是这种）：
// 它只有一个静态目录，Ready() 为 false 表示账号全部不可用。
type staticEngine struct {
	name   string
	ready  bool
	models []string
}

func (s *staticEngine) Name() string     { return s.name }
func (s *staticEngine) Describe() string { return s.name }
func (s *staticEngine) Ready() bool      { return s.ready }
func (s *staticEngine) ListModels() []model.ModelInfo {
	out := make([]model.ModelInfo, 0, len(s.models))
	for _, id := range s.models {
		out = append(out, model.ModelInfo{ID: id, Engine: s.name, Available: s.ready, Kind: "chat"})
	}
	return out
}
func (s *staticEngine) Chat(context.Context, model.ChatRequest) (*model.ChatResult, error) {
	return nil, engine.UnavailableError(s)
}
func (s *staticEngine) ChatStream(context.Context, model.ChatRequest, model.ChatStreamFunc) (*model.ChatResult, error) {
	return nil, engine.UnavailableError(s)
}
func (s *staticEngine) Passthrough() bool { return false }
func (s *staticEngine) ServePassthrough(context.Context, string, string, io.Reader, string, engine.PassthroughWriter) error {
	return nil
}
func (s *staticEngine) Status() map[string]any { return nil }

// 号池全挂的静态目录订阅引擎（如 WorkBuddy）仍要认领自己的模型：否则请求会落入
// auto 兜底被送去 AI Studio，管理台显示「没有符合条件的 AI Studio 账户」，
// 与真实原因（没有可用的 WorkBuddy 账号）无关。
func TestNotReadyStaticSubscriptionClaimsItsOwnModel(t *testing.T) {
	m := newTestManager(
		&staticEngine{name: "aistudio", ready: true, models: []string{"gemini-flash"}},
		&staticEngine{name: "workbuddy", ready: false, models: []string{"hy3", "glm-5.3"}},
	)
	eng, owned := m.pickEngine("hy3")
	if eng == nil || eng.Name() != "workbuddy" || !owned {
		t.Fatalf("hy3 应归属 workbuddy（专属），got eng=%v owned=%v", eng, owned)
	}
	// 同名模型有可用的第三方时，优先给能接单的那一方。
	m2 := newTestManager(
		&staticEngine{name: "workbuddy", ready: false, models: []string{"glm-5.3"}},
		&staticEngine{name: "custom", ready: true, models: []string{"glm-5.3"}},
	)
	if eng, _ := m2.pickEngine("glm-5.3"); eng == nil || eng.Name() != "custom" {
		t.Fatalf("可用第三方应接单，got %v", eng)
	}
}

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

// 声明目标模型的 custom 账号全部停用/冷却时，不能让「引擎级 Ready」继续把模型
// 抢走：那样请求只会得到「声明了模型的账号均不可用」，同样声明该模型的 aistudio
// 永远轮不到。应按「有可用账号声明该模型」认领。
func TestSubscriptionCatalogRequiresAvailableModel(t *testing.T) {
	m := newTestManager(
		&fakeEngine{name: "custom", ready: true, models: []string{"models/gpt-4o"}, unavailable: map[string]bool{"models/gpt-4o": true}},
		&fakeEngine{name: "aistudio", ready: true, models: []string{"gpt-4o"}},
	)
	eng, owned := m.pickEngine("gpt-4o")
	if eng == nil || eng.Name() != "aistudio" || !owned {
		t.Fatalf("模型级不可用时应让给 aistudio: eng=%v owned=%v", eng, owned)
	}

	// 没有别的引擎声明它时，仍保留给 custom（专属），让调用方看到
	// 「账号均不可用」的真实错误，而不是被 auto 兜底送去不认识的引擎。
	only := newTestManager(
		&fakeEngine{name: "custom", ready: true, models: []string{"models/gpt-4o"}, unavailable: map[string]bool{"models/gpt-4o": true}},
	)
	eng, owned = only.pickEngine("gpt-4o")
	if eng == nil || eng.Name() != "custom" || !owned {
		t.Fatalf("无备选时仍应归属 custom: eng=%v owned=%v", eng, owned)
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

// 管理台探测不受上下架开关限制：已下架的模型也要能选到承接引擎并真的打到上游，
// 否则「测试」永远返回已下架，误关之后无法恢复。
func TestProbeBypassesDisabledGate(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.SetModelEnabled("gpt-4o", false); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(&fakeEngine{name: "custom", ready: true, models: []string{"models/gpt-4o"}})
	m.st = st
	if eng, _ := m.pickEngine("gpt-4o"); eng != nil {
		t.Fatalf("下架模型不应被正常路由认领, got %s", eng.Name())
	}
	if eng, _ := m.pickEngineForProbe("gpt-4o"); eng == nil || eng.Name() != "custom" {
		t.Fatalf("探测应绕过下架闸门, got %v", eng)
	}
	eng, _, err := m.resolveChatEngineCtx(withProbeRequest(context.Background()), model.ChatRequest{Model: "gpt-4o"})
	if err != nil || eng == nil || eng.Name() != "custom" {
		t.Fatalf("探测请求应路由到 custom: eng=%v err=%v", eng, err)
	}
	if _, _, err := m.resolveChatEngine(model.ChatRequest{Model: "gpt-4o"}); !engine.IsModelDisabled(err) {
		t.Fatalf("普通请求仍应被下架闸门拦截, got %v", err)
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

// 上游把模型从 /v3/config 里摘掉（或账号全体重连中）时，实时目录不再声明它，
// 第 2/3.5 步都会落空，模型掉进 auto 兜底被送去 AI Studio，检查结果变成毫不
// 相干的「没有符合条件的 AI Studio 账户」。管理台「获取模型」保存的快照仍然
// 记得归属：只要快照说它属于某订阅引擎，就留在那里报真实错误。
func TestSnapshotOwnerKeepsModelOutOfAutoFallback(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.ReplaceEngineCatalog("workbuddy", []store.CatalogModel{
		{Engine: "workbuddy", Model: "deepseek-v4.1-flash-sg", DisplayName: "Deepseek-V4.1-Flash"},
	}); err != nil {
		t.Fatal(err)
	}
	// 获取模型默认把新出现的模型下架；这里模拟已开放的模型。
	if _, err := st.SetModelEnabled("deepseek-v4.1-flash-sg", true); err != nil {
		t.Fatal(err)
	}
	// 实时目录里只有另外两个 deepseek 模型：上游这次刷新没带 -sg 变体。
	m := newTestManager(
		&staticEngine{name: "aistudio", ready: true, models: []string{"gemini-flash"}},
		&staticEngine{name: "workbuddy", ready: true, models: []string{"deepseek-v4.1-flash", "deepseek-v4-pro"}},
	)
	m.st = st
	eng, owned := m.pickEngine("deepseek-v4.1-flash-sg")
	if eng == nil || eng.Name() != "workbuddy" || !owned {
		t.Fatalf("快照归属应把 -sg 留在 workbuddy: eng=%v owned=%v", eng, owned)
	}
	if eng, _ := m.pickEngineForProbe("deepseek-v4.1-flash-sg"); eng == nil || eng.Name() != "workbuddy" {
		t.Fatalf("管理台检查也应路由到 workbuddy, got %v", eng)
	}
}

// 快照归属只认订阅/custom 引擎：引擎A/B 的快照行过期时不能把实时目录还
// 承接着的同名模型钉死在过期归属上，auto 兜底与跨引擎备胎要照常工作。
func TestSnapshotOwnerIgnoresABEngines(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.ReplaceEngineCatalog("geminiweb", []store.CatalogModel{
		{Engine: "geminiweb", Model: "gemini-pro"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetModelEnabled("gemini-pro", true); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(
		&staticEngine{name: "geminiweb", ready: false, models: []string{}},
		&staticEngine{name: "aistudio", ready: true, models: []string{"gemini-flash"}},
	)
	m.st = st
	// gemini-pro 不在任何实时目录里：过期快照归属（geminiweb，引擎A/B）不认领，
	// 落入 auto 兜底交给就绪的 aistudio。
	eng, owned := m.pickEngine("gemini-pro")
	if eng == nil || eng.Name() != "aistudio" || owned {
		t.Fatalf("过期 geminiweb 快照不应认领: eng=%v owned=%v", eng, owned)
	}
}

// 零账号 codex 的静态目录里写死了 gpt-5.4：WorkBuddy 实时目录恰好丢了它时，
// 「声明-不可用」启发式会按注册顺序把模型抢给 codex，报「引擎 codex 不可用」。
// 快照归属（用户显式「获取模型」的记录）必须压过这个启发式。
func TestSnapshotOwnerBeatsDeclaredUnavailableHeuristic(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.ReplaceEngineCatalog("workbuddy", []store.CatalogModel{
		{Engine: "workbuddy", Model: "gpt-5.4", DisplayName: "GPT-5.4"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetModelEnabled("gpt-5.4", true); err != nil {
		t.Fatal(err)
	}
	// codex 注册在 workbuddy 之前：启发式单走会先抢到它。WorkBuddy 实时目录
	// 这次刷新没带 gpt-5.4（只剩国服模型）。
	m := newTestManager(
		&staticEngine{name: "codex", ready: false, models: []string{"gpt-5.4", "gpt-5.5"}},
		&staticEngine{name: "workbuddy", ready: true, models: []string{"hy3", "glm-5.3"}},
	)
	m.st = st
	eng, owned := m.pickEngine("gpt-5.4")
	if eng == nil || eng.Name() != "workbuddy" || !owned {
		t.Fatalf("gpt-5.4 应按快照归属 workbuddy: eng=%v owned=%v", eng, owned)
	}
	// 没有快照记录的模型才轮到「声明-不可用」启发式（保持既有行为）。
	eng, owned = m.pickEngine("gpt-5.5")
	if eng == nil || eng.Name() != "codex" || !owned {
		t.Fatalf("无快照时 gpt-5.5 仍应按启发式归属 codex: eng=%v owned=%v", eng, owned)
	}
}

// 上游把模型从目录摘掉、只剩 model_settings 里的孤儿记录时，管理台探测不该
// 再掉进 auto 兜底甩给 aistudio（报「没有符合条件的 AI Studio 账户」），
// 而应返回「没有引擎声明该模型」。
func TestProbeOrphanModelIsNotFound(t *testing.T) {
	m := newTestManager(
		&staticEngine{name: "aistudio", ready: true, models: []string{"gemini-flash"}},
		&staticEngine{name: "workbuddy", ready: true, models: []string{"deepseek-v4.1-flash", "deepseek-v4-pro"}},
	)
	if eng, owned := m.pickEngineForProbe("deepseek-v4.1-flash-sg"); eng != nil || owned {
		t.Fatalf("孤儿模型探测应返回 not-found: eng=%v owned=%v", eng, owned)
	}
	// 多模态/声明模型仍照常归属（不被误伤）。
	if eng, _ := m.pickEngineForProbe("gemini-flash"); eng == nil || eng.Name() != "aistudio" {
		t.Fatalf("aistudio 声明模型仍应接单, got %v", eng)
	}
}
