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
	// StreamOptions 控制流式附加行为（OpenAI 的 stream_options 字段）。
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	// Tools 是 function calling 的工具定义；为空表示本轮不使用工具。
	Tools []Tool `json:"tools,omitempty"`
	// ToolChoice 原样透传上游：字符串（auto/none/required）或指定函数对象。
	// 网关不解释它的语义，只在存在工具定义时一起下发。
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
	// ParallelToolCalls 是否允许模型一次并行发起多个工具调用。
	ParallelToolCalls *bool `json:"parallel_tool_calls,omitempty"`
	// ImageSize 是图像模型输出边长：512、1K、2K、4K。K 必须大写。
	ImageSize string `json:"image_size,omitempty"`
	// Resolution 是 Omni 视频输出分辨率：360p、720p、1080p、4k。k 必须小写。
	Resolution string          `json:"resolution,omitempty"`
	Extra      json.RawMessage `json:"-"`
}

// StreamOptions 流式选项。
type StreamOptions struct {
	// IncludeUsage 为 true 时，流末尾追加一个 choices 为空、只带 usage 的分片
	// （OpenAI 语义）。不开启时，网关仍会在结束分片上附带 usage，便于
	// 只读最后一块的客户端与协议转换层拿到用量。
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ChatResult 一次对话的结果。
type ChatResult struct {
	Text   string
	Engine string
	Model  string
	// Usage 是引擎拿到的 Token 用量；nil 表示引擎没有任何用量信息，
	// 由网关按文本长度估算（见 TokenUsage.Estimated）。
	Usage *TokenUsage
	// ToolCalls 是模型请求调用的工具。流式场景在流结束后汇总，
	// 增量已通过 StreamSink.ToolCall 逐块下发。
	ToolCalls []ToolCall
	// FinishReason 上游结束原因（stop / length / tool_calls）；空表示未知。
	FinishReason string
}

// EffectiveFinishReason 返回对外暴露的结束原因。上游没给时按是否有
// 工具调用推断，避免客户端把「要调工具」误判成正常结束。
func (r *ChatResult) EffectiveFinishReason() string {
	if r != nil && r.FinishReason != "" {
		return r.FinishReason
	}
	if r != nil && len(r.ToolCalls) > 0 {
		return "tool_calls"
	}
	return "stop"
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

// Tool 是 OpenAI function calling 的工具定义。
type Tool struct {
	Type     string    `json:"type"`
	Function *ToolFunc `json:"function,omitempty"`
}

// ToolFunc 函数工具的名称、说明与参数 JSON Schema。
type ToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolCall 一次完整的工具调用（非流式响应，以及流式结束后的汇总）。
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction 被调用的函数名与 JSON 字符串形式的参数。
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolFunctionDelta 流式场景下的函数名 / 参数增量。
type ToolFunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ToolCallDelta 流式工具调用增量。一次调用会被拆成多块下发，
// 客户端按 Index 归并、按到达顺序拼接 Arguments。
type ToolCallDelta struct {
	Index    int               `json:"index"`
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Function ToolFunctionDelta `json:"function"`
}

// StreamSink 是引擎的流式输出目标：文本增量与工具调用增量都写到这里，
// 由网关决定编码成 chat.completion.chunk 还是 response.* 事件。
type StreamSink interface {
	// Text 追加一段文本增量。
	Text(delta string) error
	// ToolCall 追加一段工具调用增量。
	ToolCall(delta ToolCallDelta) error
}

// TextSink 把只关心文本的回调包装成 StreamSink（工具调用增量被丢弃）。
// fn 为 nil 时返回一个什么都丢弃的 sink，方便调用方免判空。
func TextSink(fn ChatStreamFunc) StreamSink { return textSink{fn: fn} }

type textSink struct{ fn ChatStreamFunc }

func (s textSink) Text(delta string) error {
	if s.fn == nil {
		return nil
	}
	return s.fn(delta)
}

func (s textSink) ToolCall(ToolCallDelta) error { return nil }

// ChatMessage 是 OpenAI 兼容的消息。
type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string 或 []ContentPart
	// Name 是 role=tool 消息对应的函数名（部分上游需要）。
	Name string `json:"name,omitempty"`
	// ToolCalls 是 assistant 发起的工具调用。
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID 是 role=tool 消息应答的那次调用 ID。
	ToolCallID string `json:"tool_call_id,omitempty"`
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

// NewImagePart 构造图片分段（Responses 的 input_image 转 Chat 的 image_url 用）。
func NewImagePart(url string) ContentPart {
	part := ContentPart{Type: "image_url"}
	part.ImageURL = &struct {
		URL string `json:"url"`
	}{URL: url}
	return part
}

// PlainRole 返回给「不支持工具调用」的上游使用的角色：tool 降级成 user
// （结果文本仍然进上下文），developer 归并为 system。这类上游（Kiro、
// WorkBuddy、Antigravity）不认识 tool 角色，原样透传会被直接拒绝。
func (m ChatMessage) PlainRole() string {
	switch strings.ToLower(strings.TrimSpace(m.Role)) {
	case "tool":
		return "user"
	case "developer":
		return "system"
	case "":
		return "user"
	default:
		return m.Role
	}
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
	// ToolCalls 在模型要求调用工具时出现；此时 Content 通常为空串，
	// FinishReason 为 tool_calls。
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
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
	// Usage 只在流结束时出现：结束分片附带用量，或 stream_options.include_usage
	// 单独追加的用量分片（此时 Choices 为空）。协议转换层依赖它补齐
	// response.completed 的 usage，缺失会被当成异常结束。
	Usage *TokenUsage `json:"usage,omitempty"`
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
	// ToolCalls 是工具调用的分块增量，可能一次携带多个 index。
	ToolCalls []ToolCallDelta `json:"tool_calls,omitempty"`
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
	// Kind 用途分类：chat、image、video、music、speech、embedding。
	Kind string `json:"kind,omitempty"`
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
