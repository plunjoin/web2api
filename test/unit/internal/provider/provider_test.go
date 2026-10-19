//go:build web2api_unit

package provider

import (
	"context"
	"io"
	"testing"

	"web2api/internal/engine"
	"web2api/internal/model"
)

func TestGeminiCredentialsFromStorageState(t *testing.T) {
	raw := `{"cookies":[{"name":"SID","value":"sid"},{"name":"__Secure-1PSID","value":"psid"},{"name":"__Secure-1PSIDTS","value":"psidts"}],"origins":[]}`
	creds, err := geminiCredentialsFromStorageState(raw)
	if err != nil {
		t.Fatalf("提取引擎2 Cookie 失败: %v", err)
	}
	if creds.PSID != "psid" || creds.PSIDTS != "psidts" {
		t.Fatalf("Cookie 提取结果错误: %+v", creds)
	}
}

func TestGeminiCredentialsFromStorageStateRequiresCookies(t *testing.T) {
	if _, err := geminiCredentialsFromStorageState(`{"cookies":[]}`); err == nil {
		t.Fatal("缺少 Gemini Cookie 时应返回错误")
	}
}

func TestMessagesToPrompt(t *testing.T) {
	messages := []model.ChatMessage{
		{Role: "system", Content: "你是一个助手"},
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "你好！"},
		{Role: "user", Content: "介绍一下自己"},
	}
	prompt := messagesToPrompt(messages)
	want := "[System Instructions]\n你是一个助手\n\n[Conversation]\nUser: 你好\n\nAssistant: 你好！\n\nUser: 介绍一下自己"
	if prompt != want {
		t.Fatalf("prompt 转换错误:\n got %q\nwant %q", prompt, want)
	}
}

func TestMessagesToPromptNoSystem(t *testing.T) {
	messages := []model.ChatMessage{
		{Role: "user", Content: "hi"},
	}
	prompt := messagesToPrompt(messages)
	if prompt != "User: hi" {
		t.Fatalf("无 system 时 prompt 错误: %q", prompt)
	}
}

func TestMatchAny(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		want     bool
	}{
		{"gemini-flash", []string{"gemini-flash", "gemini-pro"}, true},
		{"gemini-3.7-flash", []string{"gemini-3"}, true},
		{"veo-3.1", []string{"gemini-*"}, false},
		{"gemini-flash-lite", []string{"gemini-flash*"}, true},
		{"custom-model", []string{"gemini"}, false},
	}
	for _, c := range cases {
		if got := matchAny(c.name, c.patterns); got != c.want {
			t.Errorf("matchAny(%q, %v) = %v, want %v", c.name, c.patterns, got, c.want)
		}
	}
}

func TestIsMultimodalModel(t *testing.T) {
	cases := map[string]bool{
		"gemini-3.1-flash-image": true,
		"veo-3.1-fast":           true,
		"lyria-3-pro":            true,
		"gemini-3.8-flash-tts":   true,
		"gemini-3.7-flash":       false,
		"gemini-pro":             false,
	}
	for name, want := range cases {
		if got := isMultimodalModel(name); got != want {
			t.Errorf("isMultimodalModel(%q) = %v, want %v", name, got, want)
		}
	}
}

// stubEngine 仅用于路由单测。
type stubEngine struct {
	name    string
	ready   bool
	models  []model.ModelInfo
}

func (e *stubEngine) Name() string                                    { return e.name }
func (e *stubEngine) Describe() string                                { return e.name }
func (e *stubEngine) Ready() bool                                     { return e.ready }
func (e *stubEngine) ListModels() []model.ModelInfo                   { return e.models }
func (e *stubEngine) Chat(context.Context, model.ChatRequest) (*model.ChatResult, error) {
	return nil, nil
}
func (e *stubEngine) ChatStream(context.Context, model.ChatRequest, model.ChatStreamFunc) (*model.ChatResult, error) {
	return nil, nil
}
func (e *stubEngine) Passthrough() bool { return false }
func (e *stubEngine) ServePassthrough(context.Context, string, string, io.Reader, string, engine.PassthroughWriter) error {
	return nil
}
func (e *stubEngine) Status() map[string]any { return nil }

func TestListModelsDedupesByBareID(t *testing.T) {
	b := &stubEngine{
		name:  "b",
		ready: true,
		models: []model.ModelInfo{
			{ID: "gpt-4o", DisplayName: "GPT-4o", Available: true, Kind: "chat"},
			{ID: "gemini-flash-image", Available: true, Kind: "image"},
		},
	}
	custom := &stubEngine{
		name:  "custom",
		ready: true,
		models: []model.ModelInfo{
			{ID: "gpt-4o", Available: false, Kind: "chat"},
			{ID: "my-custom-model", Available: true},
		},
	}
	m := &Manager{
		engineOrder: []string{"b", "custom"},
		engines: map[string]engine.Engine{
			"b":      b,
			"custom": custom,
		},
	}

	got := m.ListModels()
	ids := make([]string, 0, len(got))
	byID := map[string]model.ModelInfo{}
	for _, mi := range got {
		ids = append(ids, mi.ID)
		if _, dup := byID[mi.ID]; dup {
			t.Fatalf("重复模型 ID: %q", mi.ID)
		}
		byID[mi.ID] = mi
	}
	if len(ids) != 3 {
		t.Fatalf("期望 3 个唯一模型，got %v", ids)
	}
	gpt := byID["gpt-4o"]
	if gpt.Engine != "b" {
		t.Fatalf("同名保留先出现引擎，got engine=%q", gpt.Engine)
	}
	if !gpt.Available {
		t.Fatal("同名应合并 Available=true")
	}
	if byID["my-custom-model"].Engine != "custom" {
		t.Fatalf("custom 专属模型 engine=%q", byID["my-custom-model"].Engine)
	}
}

func TestPickEnginePrefersCustomCatalogOverMultimodal(t *testing.T) {
	custom := &stubEngine{
		name:  "custom",
		ready: true,
		models: []model.ModelInfo{
			{ID: "gpt-image-2.5-sunburst"},
		},
	}
	b := &stubEngine{
		name:  "b",
		ready: true,
		models: []model.ModelInfo{
			{ID: "gemini-3.1-flash-image"},
			{ID: "gemini-3.7-flash"},
		},
	}
	m := &Manager{
		defaultEngine: "auto",
		engineOrder:   []string{"b", "custom"},
		engines: map[string]engine.Engine{
			"b":      b,
			"custom": custom,
		},
	}

	eng, owned := m.pickEngine("gpt-image-2.5-sunburst")
	if eng == nil || eng.Name() != "custom" || !owned {
		t.Fatalf("期望命中 custom，got eng=%v owned=%v", engName(eng), owned)
	}

	// 未在其他引擎目录的 Gemini 多模态仍走引擎B
	eng, owned = m.pickEngine("gemini-3.1-flash-image")
	if eng == nil || eng.Name() != "b" || owned {
		t.Fatalf("期望多模态走引擎B，got eng=%v owned=%v", engName(eng), owned)
	}
}

func TestApplyEngineBModelFallback(t *testing.T) {
	b := &stubEngine{
		name:  "b",
		ready: true,
		models: []model.ModelInfo{
			{ID: "gemini-3.7-flash"},
			{ID: "gemini-3.1-flash-image"},
		},
	}
	req := applyEngineBModelFallback(b, model.ChatRequest{Model: "gpt-image-2.5-sunburst"})
	if req.Model != "gemini-3.7-flash" {
		t.Fatalf("兜底模型 = %q, want gemini-3.7-flash", req.Model)
	}

	req = applyEngineBModelFallback(b, model.ChatRequest{Model: "gemini-3.1-flash-image"})
	if req.Model != "gemini-3.1-flash-image" {
		t.Fatalf("已在目录中不应改写: %q", req.Model)
	}

	custom := &stubEngine{name: "custom", ready: true}
	req = applyEngineBModelFallback(custom, model.ChatRequest{Model: "gpt-image-2.5-sunburst"})
	if req.Model != "gpt-image-2.5-sunburst" {
		t.Fatalf("非引擎B不应改写: %q", req.Model)
	}
}

func engName(eng engine.Engine) string {
	if eng == nil {
		return "<nil>"
	}
	return eng.Name()
}
