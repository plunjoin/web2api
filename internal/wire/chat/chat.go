// Package chat 实现 OpenAI /v1/chat/completions 协议的入站解码与出站编码。
//
// 它把「协议形状」（请求解析、SSE chunk 结构）从 gateway 抽离出来，使网关只做
// 读 body → 选协议翻译 → manager → 写 body，不再持有任何协议知识。解码后的
// ChatRequest 由调用方统一走 ir.NormalizeChat 规范化，再交给 manager。
package chat

import (
	"encoding/json"
	"fmt"
	"time"

	"web2api/internal/model"
)

// StreamWriter 是 SSE 输出目标（gateway 的 *sseStream 满足）。
// 编码层只管「按协议格式写事件」，心跳 / flush / 客户端断开由实现方负责。
type StreamWriter interface {
	Write(event string, data []byte) error
	Err() error
}

// Decode 把 OpenAI Chat Completions 请求体解析为内部 ChatRequest。
// 协议级的默认值（model 缺省为 gemini-flash、messages 必填）在此处理；
// 不含 IR 规范化（由调用方统一走 ir.NormalizeChat）。
func Decode(body []byte) (model.ChatRequest, error) {
	var req model.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return req, fmt.Errorf("Invalid JSON body: %w", err)
	}
	if len(req.Messages) == 0 {
		return req, fmt.Errorf("messages is required")
	}
	if req.Model == "" {
		req.Model = "gemini-flash"
	}
	return req, nil
}

// NonStreamResponse 构造 /v1/chat/completions 的非流式响应体。
// usage 已由调用方从落库用量记录换算好，本函数只负责拼装 OpenAI 形状。
func NonStreamResponse(req model.ChatRequest, res *model.ChatResult, usage model.TokenUsage) model.ChatCompletionResponse {
	if res == nil {
		res = &model.ChatResult{}
	}
	resp := model.NewCompletion(req.Model, res.Text)
	if len(res.ToolCalls) > 0 {
		resp.Choices[0].Message.ToolCalls = res.ToolCalls
		resp.Choices[0].FinishReason = res.EffectiveFinishReason()
	}
	resp.Usage = model.Usage{
		PromptTokens:     int(usage.PromptTokens),
		CompletionTokens: int(usage.CompletionTokens),
		TotalTokens:      int(usage.TotalTokens),
	}
	return resp
}

// StreamEncoder 以 chat.completion.chunk 事件流编码流式结果。
// 它是 model.StreamSink 的实现：文本与工具调用增量分别写成分片，结束分片
// 附带用量；finish_reason 有工具调用时必须是 tool_calls，否则客户端会把
// 「模型要求执行工具」当成正常结束。
type StreamEncoder struct {
	req     model.ChatRequest
	w       StreamWriter
	id      string
	created int64
}

// NewStreamEncoder 创建流式编码器。
func NewStreamEncoder(req model.ChatRequest, w StreamWriter) *StreamEncoder {
	return &StreamEncoder{
		req:     req,
		w:       w,
		id:      "chatcmpl-" + model.NewID(12),
		created: time.Now().Unix(),
	}
}

// Sink 返回承接文本与工具调用增量的 model.StreamSink。
func (e *StreamEncoder) Sink() model.StreamSink {
	return &chunkSink{
		text: func(delta string) error {
			return e.writeDelta(model.Delta{Content: delta}, nil, nil)
		},
		tool: func(tc model.ToolCallDelta) error {
			return e.writeDelta(model.Delta{ToolCalls: []model.ToolCallDelta{tc}}, nil, nil)
		},
	}
}

// writeDelta 输出一个标准分片；finish 非空表示这是结束分片。
func (e *StreamEncoder) writeDelta(delta model.Delta, finish *string, usage *model.TokenUsage) error {
	if e.w.Err() != nil {
		return e.w.Err()
	}
	return e.WriteRaw(delta, finish, usage)
}

// WriteRaw 写分片，不做 Err() 短路（错误路径下，即使之前已写过错误也想把
// 最后一条提示推给客户端）。
func (e *StreamEncoder) WriteRaw(delta model.Delta, finish *string, usage *model.TokenUsage) error {
	data, err := json.Marshal(model.Chunk{
		ID:      e.id,
		Object:  "chat.completion.chunk",
		Created: e.created,
		Model:   e.req.Model,
		Choices: []model.ChChoice{{Index: 0, Delta: delta, FinishReason: finish}},
		Usage:   usage,
	})
	if err != nil {
		return nil
	}
	return e.w.Write("", data)
}

// Finish 写结束分片与可选的 include_usage 空 choices 用量分片，并补 [DONE]。
// usage 为本次落库的归一化用量。
func (e *StreamEncoder) Finish(res *model.ChatResult, usage model.TokenUsage, includeUsage bool) error {
	if res == nil {
		res = &model.ChatResult{}
	}
	finish := res.EffectiveFinishReason()
	if err := e.writeDelta(model.Delta{}, &finish, &usage); err != nil {
		return err
	}
	if includeUsage {
		data, err := json.Marshal(model.Chunk{
			ID:      e.id,
			Object:  "chat.completion.chunk",
			Created: e.created,
			Model:   e.req.Model,
			Choices: []model.ChChoice{},
			Usage:   &usage,
		})
		if err != nil {
			return err
		}
		if err := e.w.Write("", data); err != nil {
			return err
		}
	}
	return nil
}

// chunkSink 把增量分发到 chat.completion.chunk 编码器。
// text / tool 任一为 nil 时对应类型的增量被丢弃。
type chunkSink struct {
	text func(string) error
	tool func(model.ToolCallDelta) error
}

func (s *chunkSink) Text(delta string) error {
	if s.text == nil {
		return nil
	}
	return s.text(delta)
}

func (s *chunkSink) ToolCall(delta model.ToolCallDelta) error {
	if s.tool == nil {
		return nil
	}
	return s.tool(delta)
}
