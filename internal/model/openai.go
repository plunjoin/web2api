// Package model 定义 OpenAI 兼容协议的数据结构，以及网关内部使用的统一消息格式。
package model

import (
	"encoding/json"
	"strings"
	"time"
)

// ChatRequest 是 OpenAI 兼容的 /v1/chat/completions 请求体。
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	User        string        `json:"user,omitempty"`
	// ImageSize 是图像模型输出边长：512、1K、2K、4K。K 必须大写。
	ImageSize string `json:"image_size,omitempty"`
	// Resolution 是 Omni 视频输出分辨率：360p、720p、1080p、4k。k 必须小写。
	Resolution string          `json:"resolution,omitempty"`
	Extra      json.RawMessage `json:"-"`
}

// ChatResult 一次对话的结果。
type ChatResult struct {
	Text   string
	Engine string
	Model  string
	// Usage 是引擎拿到的 Token 用量；nil 表示引擎没有任何用量信息，
	// 由网关按文本长度估算（见 TokenUsage.Estimated）。
	Usage *TokenUsage
}

// TokenUsage 一次请求的 Token 用量。
//
// Estimated=false：数值来自上游权威用量（如 AI Studio usage metadata、
// 上游 OpenAI 兼容接口的 usage 字段）。
// Estimated=true：至少一部分为本地估算（上游未返回用量，或只返回了输入计数）。
type TokenUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	ReasoningTokens  int64 `json:"reasoning_tokens,omitempty"`
	Estimated        bool  `json:"estimated"`
}

// Normalize 补齐 TotalTokens，并保证三个计数非负。
func (u TokenUsage) Normalize() TokenUsage {
	if u.PromptTokens < 0 {
		u.PromptTokens = 0
	}
	if u.CompletionTokens < 0 {
		u.CompletionTokens = 0
	}
	if u.TotalTokens < u.PromptTokens+u.CompletionTokens {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

// ChatStreamFunc 流式回调：每次收到增量文本时调用。
type ChatStreamFunc func(delta string) error

// ChatMessage 是 OpenAI 兼容的消息。
type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string 或 []ContentPart
}

// ContentPart 多模态消息的分段（文本 / 图片 / 文件）。
type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
	FileURL *struct {
		URL string `json:"url"`
	} `json:"file_url,omitempty"`
}

// 将 content 归一化为纯文本（多模态时提取文本段）。
func (m ChatMessage) Text() string {
	switch c := m.Content.(type) {
	case string:
		return c
	case []ContentPart:
		var parts []string
		for _, p := range c {
			if p.Type == "text" && p.Text != "" {
				parts = append(parts, p.Text)
			}
		}
		return strings.Join(parts, "\n")
	case []any:
		var parts []string
		for _, p := range c {
			if pm, ok := p.(map[string]any); ok {
				if t, ok := pm["type"].(string); ok && t == "text" {
					if s, ok := pm["text"].(string); ok {
						parts = append(parts, s)
					}
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// ChatCompletionResponse 是 OpenAI 兼容的非流式响应。
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice 是响应中的一个候选。
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Message 是响应消息。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage 统计。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Chunk 是 OpenAI 兼容的流式分片。
type Chunk struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Created int64      `json:"created"`
	Model   string     `json:"model"`
	Choices []ChChoice `json:"choices"`
}

// ChChoice 流式候选。
type ChChoice struct {
	Index        int     `json:"index"`
	Delta        Delta   `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

// Delta 增量内容。
type Delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// ErrorResponse OpenAI 兼容错误体。
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail 错误详情。
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code,omitempty"`
}

// ModelInfo 统一模型描述。
type ModelInfo struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	DisplayName string `json:"display_name,omitempty"`
	Available   bool   `json:"available"`
	Engine      string `json:"engine,omitempty"`
}

// NewCompletion 构造标准非流式响应。
func NewCompletion(model, text string) ChatCompletionResponse {
	return ChatCompletionResponse{
		ID:      "chatcmpl-" + randHex(12),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{{
			Index:        0,
			Message:      Message{Role: "assistant", Content: text},
			FinishReason: "stop",
		}},
		Usage: Usage{},
	}
}

// NewChunk 构造标准流式分片。
func NewChunk(model, delta string, finish bool) Chunk {
	var fr *string
	if finish {
		s := "stop"
		fr = &s
	}
	return Chunk{
		ID:      "chatcmpl-" + NewID(12),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChChoice{{
			Index:        0,
			Delta:        Delta{Content: delta},
			FinishReason: fr,
		}},
	}
}
