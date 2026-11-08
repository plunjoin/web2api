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

// ChatMediaEngine 可选能力：能在 /v1/chat/completions 的文本响应里直接带回
// 音频/图片等媒体。目前只有引擎B 原生模式支持（把 inline data 转成 data URI）。
// 第三方 OpenAI 兼容端点不支持——它们的 chat 只能返回文本，音乐/视频模型
// 硬发过去会被上游以 “Unhandled generated data mime type: audio/mpeg” 拒掉。
type ChatMediaEngine interface {
	ChatSupportsMedia() bool
}

// ToolEngine 可选能力：支持 OpenAI function calling 透传——把 tools /
// tool_choice 下发给上游，并把上游的 tool_calls 增量回传。
//
// 不实现本接口的引擎（Gemini 网页逆向、Kiro、WorkBuddy 等）收到带 tools 的
// 请求时只能忽略工具定义、用纯文本回答，网关侧不做报错：这类上游本来就没有
// 可用的函数调用通道，报错只会让客户端完全不可用。
type ToolEngine interface {
	Engine
	// ChatStreamSink 与 ChatStream 等价，但把文本与工具调用增量都写到 sink。
	ChatStreamSink(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error)
}

// CatalogRefresher 可选能力：管理台点「获取模型」时，向供应商重新拉取一次
// 模型目录并更新引擎内存里的目录。返回的是这次拉到的最新列表（ListModels
// 的口径），空列表表示供应商确实没有模型，不是失败。
type CatalogRefresher interface {
	RefreshCatalog(ctx context.Context) ([]model.ModelInfo, error)
}

// ModelAvailability 由「目录条目带逐模型可用性」的引擎实现。多数引擎的
// ListModels 是静态目录或只反映账号整体状态；custom 会把停用/冷却账号声明的
// 模型也列出来（Available=false）。路由认领模型时必须问引擎「现在是否真的有
// 可用账号能接这个模型」，否则引擎级 Ready 会把别的引擎能接的同名模型抢走。
type ModelAvailability interface {
	HasAvailableModel(modelName string) bool
}

// PassthroughWriter 抽象透传响应的写出目标（网关的 http.ResponseWriter）。
type PassthroughWriter interface {
	Header() http.Header
	WriteHeader(status int)
	Write(p []byte) (int, error)
}
