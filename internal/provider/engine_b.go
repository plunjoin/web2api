package provider

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

// AIStudioEngine 引擎B：AI Studio（OpenAI 兼容上游，支持多模态透传）。
type AIStudioEngine struct {
	name string
	up   *upstream.OpenAI
}

// NewAIStudio 从配置构建引擎B。
func NewAIStudio(cfg config.EngineBConfig, timeout time.Duration) (*AIStudioEngine, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("引擎B 未配置 base_url")
	}
	up := upstream.New("b", cfg.BaseURL, cfg.APIKey, cfg.Passthrough, timeout)
	return &AIStudioEngine{name: "b", up: up}, nil
}

// Init 探测引擎并拉取模型列表。
func (b *AIStudioEngine) Init(ctx context.Context) error {
	b.up.RefreshModels(ctx)
	return nil
}

// Name 引擎标识。
func (b *AIStudioEngine) Name() string { return b.name }

// Describe 引擎说明。
func (b *AIStudioEngine) Describe() string {
	return "aistudio.google.com（AI Studio，OpenAI 兼容上游，支持多模态）"
}

// Ready 引擎可用。
func (b *AIStudioEngine) Ready() bool { return b.up.Ready() }

// ListModels 上游模型列表。
func (b *AIStudioEngine) ListModels() []model.ModelInfo {
	models := b.up.ListModels()
	out := make([]model.ModelInfo, 0, len(models))
	for _, m := range models {
		m.Engine = "b"
		out = append(out, m)
	}
	return out
}

// Chat 非流式。
func (b *AIStudioEngine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	return b.up.Chat(ctx, req)
}

// ChatStream 流式。
func (b *AIStudioEngine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	return b.up.ChatStream(ctx, req, onDelta)
}

// Passthrough 是否透传多模态。
func (b *AIStudioEngine) Passthrough() bool { return b.up.Passthrough() }

// ServePassthrough 透传。
func (b *AIStudioEngine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return b.up.ServePassthrough(ctx, method, path, body, contentType, w)
}

// Status 上游状态。
func (b *AIStudioEngine) Status() map[string]any {
	return map[string]any{
		"engine":      "b",
		"ready":       b.Ready(),
		"base_url":    b.up.BaseURL(),
		"passthrough": b.Passthrough(),
		"error":       b.up.ReadyErr(),
	}
}
