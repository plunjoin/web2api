// Package engine 定义引擎抽象。引擎A（gemini.google.com 网页版原生逆向）
// 与引擎B（AI Studio / 任意 OpenAI 兼容服务）都实现本接口，由网关统一调度。
package engine

import (
	"context"
	"io"
	"net/http"

	"web2api/internal/model"
)

// Engine 统一引擎接口。
type Engine interface {
	// Name 引擎标识：a / b。
	Name() string
	// Describe 引擎说明。
	Describe() string
	// Ready 引擎是否可用（已初始化 / 上游可达）。
	Ready() bool
	// ListModels 返回该引擎可用的模型列表。
	ListModels() []model.ModelInfo
	// Chat 非流式对话。
	Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error)
	// ChatStream 流式对话，每收到增量调用一次 onDelta，返回完整结果。
	ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error)
	// Passthrough 返回是否需要将多模态端点透传到该引擎。
	Passthrough() bool
	// ServePassthrough 若 Passthrough 为 true，将请求原样转发并返回响应状态/内容。
	ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w PassthroughWriter) error
	// Status 返回引擎运行状态（供 /v1/accounts 展示）。
	Status() map[string]any
}

// PassthroughWriter 抽象透传响应的写出目标（网关的 http.ResponseWriter）。
type PassthroughWriter interface {
	Header() http.Header
	WriteHeader(status int)
	Write(p []byte) (int, error)
}
