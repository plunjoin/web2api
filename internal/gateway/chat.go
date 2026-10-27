package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"web2api/internal/model"
	"web2api/internal/provider"
	"web2api/internal/store"
)

// handleChat 处理 /v1/chat/completions。
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取请求体失败", "invalid_request_error", nil)
		return
	}
	var req model.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error(), "invalid_request_error", nil)
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages is required", "invalid_request_error", nil)
		return
	}
	if req.Model == "" {
		req.Model = "gemini-flash"
	}
	if !s.modelAllowed(w, r, req.Model) {
		return
	}
	ctx := r.Context()
	start := time.Now()

	// 流式 SSE
	if req.Stream {
		sse := newSSEStream(w)
		if err := sse.Start(); err != nil {
			s.logger.Printf("[chat] 初始化流失败: %v", err)
			return
		}
		defer sse.Close()

		id := "chatcmpl-" + model.NewID(12)
		created := time.Now().Unix()
		writeChunk := func(chunk model.Chunk) error {
			data, err := json.Marshal(chunk)
			if err != nil {
				return nil
			}
			return sse.Write("", data)
		}
		// writeDelta 输出一个标准分片；finish 非空表示这是结束分片。
		writeDelta := func(delta model.Delta, finish *string, usage *model.TokenUsage) error {
			return writeChunk(model.Chunk{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   req.Model,
				Choices: []model.ChChoice{{Index: 0, Delta: delta, FinishReason: finish}},
				Usage:   usage,
			})
		}

		sink := &chatSink{
			text: func(delta string) error {
				return writeDelta(model.Delta{Content: delta}, nil, nil)
			},
			tool: func(tc model.ToolCallDelta) error {
				return writeDelta(model.Delta{ToolCalls: []model.ToolCallDelta{tc}}, nil, nil)
			},
		}
		res, runErr := s.mgr.ChatStream(ctx, req, sink)
		if runErr != nil {
			s.logger.Printf("[chat] 流式请求失败 model=%q: %v", req.Model, runErr)
			s.recordChat(r, req, res, true, runErr, start, "/v1/chat/completions")
			stop := "stop"
			_ = writeDelta(model.Delta{Content: "[Error] " + runErr.Error()}, &stop, nil)
		} else {
			rec := s.recordChat(r, req, res, true, nil, start, "/v1/chat/completions")
			usage := model.TokenUsage{
				PromptTokens:     rec.PromptTokens,
				CompletionTokens: rec.CompletionTokens,
				TotalTokens:      rec.TotalTokens,
				Estimated:        rec.Estimated,
			}.Normalize()
			// 结束分片附带 usage：协议转换层（CC Switch 等）靠它补齐
			// response.completed 的用量，缺失会被当成异常结束而截断输出。
			// finish_reason 有工具调用时必须是 tool_calls，否则客户端会把
			// 「模型要求执行工具」当成正常结束。
			finish := res.EffectiveFinishReason()
			_ = writeDelta(model.Delta{}, &finish, &usage)
			if req.StreamOptions != nil && req.StreamOptions.IncludeUsage {
				// OpenAI 语义：include_usage 时额外追加一个 choices 为空的用量分片。
				_ = writeChunk(model.Chunk{
					ID:      id,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   req.Model,
					Choices: []model.ChChoice{},
					Usage:   &usage,
				})
			}
		}
		_ = sse.Write("", []byte("[DONE]"))
		return
	}

	// 非流式
	res, err := s.mgr.Chat(ctx, req)
	if err != nil {
		s.logger.Printf("[chat] 请求失败 model=%q: %v", req.Model, err)
		s.recordChat(r, req, res, false, err, start, "/v1/chat/completions")
		// 「没有任何账号/引擎声明该模型」是客户端问题，返回 404 + model_not_found，
		// 不要包装成 502 让下游误以为可以重试。
		if provider.IsModelNotFound(err) {
			writeError(w, http.StatusNotFound, err.Error(), "invalid_request_error", "model_not_found")
			return
		}
		// 模型用途与端点不匹配（音乐模型打到 chat）：400 + 明确的正确入口。
		if provider.IsUnsupportedEndpoint(err) {
			writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_endpoint")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error(), "api_error", nil)
		return
	}
	rec := s.recordChat(r, req, res, false, nil, start, "/v1/chat/completions")
	if res == nil {
		res = &model.ChatResult{}
	}
	resp := model.NewCompletion(req.Model, res.Text)
	resp.ID = "chatcmpl-" + model.NewID(12)
	if len(res.ToolCalls) > 0 {
		resp.Choices[0].Message.ToolCalls = res.ToolCalls
		resp.Choices[0].FinishReason = res.EffectiveFinishReason()
	}
	resp.Usage.PromptTokens = int(rec.PromptTokens)
	resp.Usage.CompletionTokens = int(rec.CompletionTokens)
	resp.Usage.TotalTokens = int(rec.TotalTokens)
	// 扩展响应头：用量来源与本次计费（标准 usage 字段保持 OpenAI 口径不变）。
	w.Header().Set("X-Web2api-Usage-Source", usageSource(rec.Estimated))
	w.Header().Set("X-Web2api-Multiplier", strconv.FormatFloat(rec.Multiplier, 'f', -1, 64))
	w.Header().Set("X-Web2api-Charged-Tokens", strconv.FormatInt(rec.ChargedTokens, 10))
	if rec.UserID > 0 && rec.ChargedTokens > 0 {
		w.Header().Set("X-Web2api-Balance", strconv.FormatInt(rec.BalanceAfter, 10))
	}
	writeJSON(w, http.StatusOK, resp)
}

// resolveUsage 优先使用引擎返回的上游权威用量；引擎没有用量时按文本长度估算，
// 并标记 Estimated=true（管理台显示为“估算”）。
func resolveUsage(req model.ChatRequest, res *model.ChatResult) model.TokenUsage {
	if res != nil && res.Usage != nil {
		return res.Usage.Normalize()
	}
	completion := ""
	if res != nil {
		completion = res.Text
	}
	return model.TokenUsage{
		PromptTokens:     estimateTokens(messagesText(req.Messages)),
		CompletionTokens: estimateTokens(completion),
		Estimated:        true,
	}.Normalize()
}

func usageSource(estimated bool) string {
	if estimated {
		return "estimated"
	}
	return "upstream"
}

// recordChat 写入逐请求用量并按倍率扣减 Key 额度。失败请求只记录、不扣额度。
// endpoint 为调用入口（/v1/chat/completions 或 /v1/responses），用于用量归因。
func (s *Server) recordChat(r *http.Request, req model.ChatRequest, res *model.ChatResult, stream bool, runErr error, start time.Time, endpoint string) store.UsageRecord {
	rec := store.UsageRecord{
		APIKey:    s.currentKey(r),
		Model:     req.Model,
		Endpoint:  endpoint,
		Stream:    stream,
		Success:   runErr == nil,
		LatencyMs: time.Since(start).Milliseconds(),
	}
	if info, ok := keyInfoFrom(r); ok {
		rec.KeyID = info.ID
	}
	if res != nil && strings.TrimSpace(res.Engine) != "" {
		rec.Engine = res.Engine
	} else if s.mgr != nil {
		rec.Engine = s.mgr.EngineForModel(req.Model)
	}
	if runErr != nil {
		rec.Error = runErr.Error()
	} else {
		usage := resolveUsage(req, res)
		rec.PromptTokens, rec.CompletionTokens, rec.TotalTokens = usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens
		rec.Estimated = usage.Estimated
	}
	saved, err := s.st.RecordRequest(rec)
	if err != nil {
		s.logger.Printf("[usage] 记录用量失败 model=%q: %v", req.Model, err)
		return rec
	}
	return saved
}

// estimateTokens 粗略估算 Token 数（中英混合 ~4 字符/Token）。
func estimateTokens(text string) int64 {
	if text == "" {
		return 0
	}
	runes := 0
	for range text {
		runes++
	}
	return int64((runes + 3) / 4)
}

// messagesText 拼接全部消息文本（估算 prompt tokens）。
func messagesText(messages []model.ChatMessage) string {
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(m.Text())
		sb.WriteString("\n")
	}
	return sb.String()
}

// chatSink 把引擎的流式增量编码成 chat.completion.chunk。
// text / tool 任一为 nil 时对应类型的增量被丢弃。
type chatSink struct {
	text func(string) error
	tool func(model.ToolCallDelta) error
}

func (s *chatSink) Text(delta string) error {
	if s.text == nil {
		return nil
	}
	return s.text(delta)
}

func (s *chatSink) ToolCall(delta model.ToolCallDelta) error {
	if s.tool == nil {
		return nil
	}
	return s.tool(delta)
}
