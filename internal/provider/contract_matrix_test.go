package provider

import (
	"context"
	"errors"
	"testing"

	"web2api/internal/model"
)

// 契约矩阵（docs/web2api-responses-adapter-plan.md §4 阶段 3）：
// 失败用量明细必须能显示引擎来源——Manager.Chat / ChatStream 出错时仍返回
// 带 Engine 的 ChatResult，网关 recordChat 不再拿到 Engine 为空的记录。
// 纯文本 / function tools / local_shell 入站被剥 / 400 降级分别在
// gateway(responses_tools_test.go)、ir(ir_test.go)、upstream(openai_test.go)、
// provider(engines_tools_test.go / antigravity_tools_test.go) 覆盖。

func TestManagerChatFailureCarriesEngine(t *testing.T) {
	failing := &fakeEngine{name: "custom", ready: true, models: []string{"models/MiniMax-M2"}, chatErr: errors.New("上游返回 HTTP 400: bad tools")}
	m := newTestManager(failing)
	res, err := m.Chat(context.Background(), model.ChatRequest{Model: "MiniMax-M2"})
	if err == nil {
		t.Fatal("want error")
	}
	if res == nil || res.Engine != "custom" {
		t.Fatalf("failure result must carry engine, got %#v", res)
	}
}

func TestManagerChatStreamFailureCarriesEngine(t *testing.T) {
	failing := &fakeEngine{name: "custom", ready: true, models: []string{"models/MiniMax-M2"}, streamErr: errors.New("上游返回 HTTP 429")}
	m := newTestManager(failing)
	res, err := m.ChatStream(context.Background(), model.ChatRequest{Model: "MiniMax-M2"}, model.TextSink(nil))
	if err == nil {
		t.Fatal("want error")
	}
	if res == nil || res.Engine != "custom" {
		t.Fatalf("failure result must carry engine, got %#v", res)
	}
}

func TestManagerChatModelNotFoundHasNoEngine(t *testing.T) {
	// 「没有任何引擎声明该模型」发生在选引擎之前，没有引擎来源可记。
	m := newTestManager()
	res, err := m.Chat(context.Background(), model.ChatRequest{Model: "nobody-declares-this"})
	if err == nil || res != nil {
		t.Fatalf("model_not_found should return bare error, got res=%#v err=%v", res, err)
	}
}
