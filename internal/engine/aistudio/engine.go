package aistudio

import (
	"context"
	"errors"
	"io"
	"time"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/engine/upstream"
	"web2api/internal/model"
)

// Engine 引擎B：AI Studio（OpenAI 兼容上游，支持多模态透传）。
type Engine struct {
	name string
	up   *upstream.OpenAI
}

// NewAIStudio 从配置构建引擎B。
func New(cfg config.EngineBConfig, timeout time.Duration) (*Engine, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("引擎B 未配置 base_url")
	}
	up := upstream.New("b", cfg.BaseURL, cfg.APIKey, cfg.Passthrough, timeout)
	return &Engine{name: "b", up: up}, nil
}

// Init 探测引擎并拉取模型列表。
func (b *Engine) Init(ctx context.Context) error {
	b.up.RefreshModels(ctx)
	return nil
}

// Name 引擎标识。
func (b *Engine) Name() string { return b.name }

// Describe 引擎说明。
func (b *Engine) Describe() string {
	return "google.com（AI Studio，OpenAI 兼容上游，支持多模态）"
}

// Ready 引擎可用。
func (b *Engine) Ready() bool { return b.up.Ready() }

// ProbeImage 用 OpenAI 兼容上游探测图片模型（/v1/images/generations）。
func (b *Engine) ProbeImage(ctx context.Context, modelID string) (string, error) {
	return b.up.ProbeImage(ctx, modelID)
}

// ProbeEmbedding 用 OpenAI 兼容上游探测向量模型（/v1/embeddings）。
func (b *Engine) ProbeEmbedding(ctx context.Context, modelID string) (string, error) {
	return b.up.ProbeEmbedding(ctx, modelID)
}

// ListModels 上游模型列表。
func (b *Engine) ListModels() []model.ModelInfo {
	models := b.up.ListModels()
	out := make([]model.ModelInfo, 0, len(models))
	for _, m := range models {
		m.Engine = "b"
		if m.Kind == "" {
			m.Kind = model.ClassifyKind(m.ID, m.DisplayName, nil)
		}
		out = append(out, m)
	}
	return out
}

// Chat 非流式。
func (b *Engine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	return b.up.Chat(ctx, req)
}

// ChatStream 流式。
func (b *Engine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	return b.up.ChatStream(ctx, req, onDelta)
}

// ChatStreamSink 流式（支持工具调用增量）。
func (b *Engine) ChatStreamSink(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	return b.up.ChatStreamSink(ctx, req, sink)
}

// Passthrough 是否透传多模态。
func (b *Engine) Passthrough() bool { return b.up.Passthrough() }

// ServePassthrough 透传。
func (b *Engine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return b.up.ServePassthrough(ctx, method, path, body, contentType, w)
}

// Status 上游状态。
func (b *Engine) Status() map[string]any {
	return map[string]any{
		"engine":      "b",
		"ready":       b.Ready(),
		"base_url":    b.up.BaseURL(),
		"passthrough": b.Passthrough(),
		"error":       b.up.ReadyErr(),
	}
}
