package gateway

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"web2api/internal/ir"
	"web2api/internal/model"
	"web2api/internal/provider"
	"web2api/internal/store"
	"web2api/internal/wire/chat"
)

// handleChat 处理 /v1/chat/completions。
//
// 仅做协议无关的中枢职责：读 body → wire.chat 解码 → 模型白名单 → manager
// → wire.chat 编码 → 写 body。具体的请求解析与 SSE chunk 结构都下沉到了
// wire/chat，本函数不再持有任何协议形状。
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取请求体失败", "invalid_request_error", nil)
		return
	}
	req, err := chat.Decode(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
		return
	}
	// 统一 IR 入口：与 /v1/responses 走同一套规范化不变量（非 function
	// 工具丢弃、空 assistant content、tool name 回填）。
	req = ir.NormalizeChat(req, s.logger.Printf)
	if !s.modelAllowed(w, r, req.Model) {
		return
	}
	ctx := r.Context()
	start := time.Now()

	// 流式 SSE
	if req.Stream {
		s.handleChatStream(w, r, req, start)
		return
	}

	// 非流式
	res, err := s.mgr.Chat(ctx, req)
	if err != nil {
		s.logger.Printf("[chat] 请求失败 model=%q: %v", req.Model, err)
		s.recordChat(r, req, res, false, err, start, "/v1/chat/completions")
		if provider.IsModelNotFound(err) {
			writeError(w, http.StatusNotFound, err.Error(), "invalid_request_error", "model_not_found")
			return
		}
		if provider.IsUnsupportedEndpoint(err) {
			writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_endpoint")
			return
		}
		if provider.IsToolsNotSupported(err) {
			writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "tools_not_supported")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error(), "api_error", nil)
		return
	}
	rec := s.recordChat(r, req, res, false, nil, start, "/v1/chat/completions")
	usage := resolveUsage(req, res).Normalize()
	resp := chat.NonStreamResponse(req, res, usage)
	w.Header().Set("X-Web2api-Usage-Source", usageSource(rec.Estimated))
	w.Header().Set("X-Web2api-Multiplier", strconv.FormatFloat(rec.Multiplier, 'f', -1, 64))
	w.Header().Set("X-Web2api-Charged-Tokens", strconv.FormatInt(rec.ChargedTokens, 10))
	if rec.UserID > 0 && rec.ChargedTokens > 0 {
		w.Header().Set("X-Web2api-Balance", strconv.FormatInt(rec.BalanceAfter, 10))
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleChatStream 以 chat.completion.chunk 事件流返回结果。
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request, req model.ChatRequest, start time.Time) {
	sse := newSSEStream(w)
	if err := sse.Start(); err != nil {
		s.logger.Printf("[chat] 初始化流失败: %v", err)
		return
	}
	defer sse.Close()

	enc := chat.NewStreamEncoder(req, sse)
	res, runErr := s.mgr.ChatStream(r.Context(), req, enc.Sink())
	if runErr != nil {
		s.logger.Printf("[chat] 流式请求失败 model=%q: %v", req.Model, runErr)
		s.recordChat(r, req, res, true, runErr, start, "/v1/chat/completions")
		stop := "stop"
		_ = enc.WriteRaw(model.Delta{Content: "[Error] " + runErr.Error()}, &stop, nil)
		_ = sse.Write("", []byte("[DONE]"))
		return
	}
	_ = s.recordChat(r, req, res, true, nil, start, "/v1/chat/completions")
	usage := resolveUsage(req, res).Normalize()
	includeUsage := req.StreamOptions != nil && req.StreamOptions.IncludeUsage
	if err := enc.Finish(res, usage, includeUsage); err != nil {
		return
	}
	_ = sse.Write("", []byte("[DONE]"))
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
