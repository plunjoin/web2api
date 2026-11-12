package provider

import (
	"context"
	"errors"
	"testing"

	"web2api/internal/model"
)

// 下游目录只保留一条：models/xxx 与 xxx 是同一个模型，输出裸 ID。
func TestListModelsMergesModelsPrefix(t *testing.T) {
	m := newTestManager(
		&fakeEngine{name: "aistudio", ready: true, models: []string{"gemini-2.5-flash"}},
		&fakeEngine{name: "custom", ready: true, models: []string{"models/gemini-2.5-flash"}},
	)
	models := m.ListModels()
	if len(models) != 1 {
		t.Fatalf("want 1 merged model, got %d: %+v", len(models), models)
	}
	if models[0].ID != "gemini-2.5-flash" {
		t.Fatalf("want bare model id, got %q", models[0].ID)
	}
}

type recordingEngine struct {
	fakeEngine
	calls *[]string
	text  string
}

func (r *recordingEngine) Chat(context.Context, model.ChatRequest) (*model.ChatResult, error) {
	*r.calls = append(*r.calls, r.name)
	if r.chatErr != nil {
		return nil, r.chatErr
	}
	return &model.ChatResult{Engine: r.name, Text: r.text}, nil
}

// 配置了供应商顺序时，请求先走排在前面的供应商；它失败后再换下一个。
func TestChatFailsOverAcrossSuppliersInConfiguredOrder(t *testing.T) {
	var calls []string
	first := &recordingEngine{fakeEngine: fakeEngine{name: "custom", ready: true, models: []string{"gpt-5"}}, calls: &calls}
	first.chatErr = errors.New("上游返回 HTTP 500")
	second := &recordingEngine{fakeEngine: fakeEngine{name: "codex", ready: true, models: []string{"gpt-5"}}, calls: &calls, text: "ok"}
	m := newTestManager(second, first) // 注册顺序故意与配置顺序相反
	m.routeIDs = []string{"1", "2"}
	m.routeSupplier = map[string]int{"custom": 0, "codex": 1}

	res, err := m.Chat(context.Background(), model.ChatRequest{Model: "gpt-5"})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if res == nil || res.Text != "ok" {
		t.Fatalf("want second supplier response, got %+v", res)
	}
	if len(calls) != 2 || calls[0] != "custom" || calls[1] != "codex" {
		t.Fatalf("want custom then codex, got %v", calls)
	}
}

// 显式配置点名的引擎不跨供应商兜底，失败就是失败。
func TestExplicitRouteDoesNotFailOver(t *testing.T) {
	var calls []string
	first := &recordingEngine{fakeEngine: fakeEngine{name: "custom", ready: true, models: []string{"gpt-5"}}, calls: &calls}
	first.chatErr = errors.New("上游返回 HTTP 500")
	second := &recordingEngine{fakeEngine: fakeEngine{name: "codex", ready: true, models: []string{"gpt-5"}}, calls: &calls, text: "ok"}
	m := newTestManager(first, second)
	m.engineModels = map[string][]string{"custom": {"gpt-5"}}

	if _, err := m.Chat(context.Background(), model.ChatRequest{Model: "gpt-5"}); err == nil {
		t.Fatal("explicit route should return the target engine error")
	}
	if len(calls) != 1 || calls[0] != "custom" {
		t.Fatalf("explicit route must not try other suppliers, got %v", calls)
	}
}

type orderCaptureEngine struct {
	fakeEngine
	order []string
}

func (o *orderCaptureEngine) SetAccountOrder(ids []string) { o.order = append([]string(nil), ids...) }

// 保存的顺序会下发到每个支持排序的引擎，新增/未知账号不丢。
func TestApplyRoutingOrderPushesToEngines(t *testing.T) {
	a := &orderCaptureEngine{fakeEngine: fakeEngine{name: "custom", ready: true}}
	b := &orderCaptureEngine{fakeEngine: fakeEngine{name: "codex", ready: true}}
	m := newTestManager(a, b)
	m.applyRoutingOrder([]string{"3", "1", "3", ""})
	for _, eng := range []*orderCaptureEngine{a, b} {
		if len(eng.order) != 2 || eng.order[0] != "3" || eng.order[1] != "1" {
			t.Fatalf("%s order = %v, want [3 1]", eng.name, eng.order)
		}
	}
}
