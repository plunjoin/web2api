package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"web2api/internal/model"
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
		flusher := http.NewResponseController(w)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		if err := flusher.Flush(); err != nil {
			s.logger.Printf("[chat] 初始化流失败: %v", err)
		}

		id := "chatcmpl-" + model.NewID(12)
		created := time.Now().Unix()
		send := func(delta string, finish bool) {
			var fr *string
			if finish {
				st := "stop"
				fr = &st
			}
			chunk := model.Chunk{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   req.Model,
				Choices: []model.ChChoice{{
					Index:        0,
					Delta:        model.Delta{Content: delta},
					FinishReason: fr,
				}},
			}
			data, _ := json.Marshal(chunk)
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			_ = flusher.Flush()
		}

		var runErr error
		res, runErr := s.mgr.ChatStream(ctx, req, func(delta string) error {
			send(delta, false)
			return nil
		})
		if runErr != nil {
			s.logger.Printf("[chat] 流式请求失败 model=%q: %v", req.Model, runErr)
			s.recordChat(r, req, nil, true, runErr, start)
			send("[Error] "+runErr.Error(), true)
		} else {
			s.recordChat(r, req, res, true, nil, start)
			send("", true)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		_ = flusher.Flush()
		return
	}

	// 非流式
	res, err := s.mgr.Chat(ctx, req)
	if err != nil {
		s.logger.Printf("[chat] 请求失败 model=%q: %v", req.Model, err)
		s.recordChat(r, req, nil, false, err, start)
		writeError(w, http.StatusBadGateway, err.Error(), "api_error", nil)
		return
	}
	rec := s.recordChat(r, req, res, false, nil, start)
	resp := model.NewCompletion(req.Model, res.Text)
	resp.ID = "chatcmpl-" + model.NewID(12)
	resp.Usage.PromptTokens = int(rec.PromptTokens)
	resp.Usage.CompletionTokens = int(rec.CompletionTokens)
	resp.Usage.TotalTokens = int(rec.TotalTokens)
	// 扩展响应头：用量来源与本次计费（标准 usage 字段保持 OpenAI 口径不变）。
	w.Header().Set("X-Web2api-Usage-Source", usageSource(rec.Estimated))
	w.Header().Set("X-Web2api-Multiplier", strconv.FormatFloat(rec.Multiplier, 'f', -1, 64))
	w.Header().Set("X-Web2api-Charged-Tokens", strconv.FormatInt(rec.ChargedTokens, 10))
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
func (s *Server) recordChat(r *http.Request, req model.ChatRequest, res *model.ChatResult, stream bool, runErr error, start time.Time) store.UsageRecord {
	rec := store.UsageRecord{
		APIKey:    s.currentKey(r),
		Model:     req.Model,
		Endpoint:  "/v1/chat/completions",
		Stream:    stream,
		Success:   runErr == nil,
		LatencyMs: time.Since(start).Milliseconds(),
	}
	if info, ok := keyInfoFrom(r); ok {
		rec.KeyID = info.ID
	}
	if runErr != nil {
		rec.Error = runErr.Error()
	} else {
		usage := resolveUsage(req, res)
		rec.PromptTokens, rec.CompletionTokens, rec.TotalTokens = usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens
		rec.Estimated = usage.Estimated
		if res != nil {
			rec.Engine = res.Engine
		}
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
