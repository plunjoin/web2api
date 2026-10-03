package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"web2api/internal/model"
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
	ctx := r.Context()

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
		apiKey := s.currentKey(r)
		if runErr != nil {
			s.logger.Printf("[chat] 流式请求失败 model=%q: %v", req.Model, runErr)
			_ = s.st.RecordUsage(apiKey, "", req.Model, 0, 0, false)
			send("[Error] "+runErr.Error(), true)
		} else {
			engine := ""
			completion := ""
			if res != nil {
				engine = res.Engine
				completion = res.Text
			}
			_ = s.st.RecordUsage(apiKey, engine, req.Model,
				estimateTokens(messagesText(req.Messages)), estimateTokens(completion), true)
			send("", true)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		_ = flusher.Flush()
		return
	}

	// 非流式
	res, err := s.mgr.Chat(ctx, req)
	apiKey := s.currentKey(r)
	if err != nil {
		s.logger.Printf("[chat] 请求失败 model=%q: %v", req.Model, err)
		_ = s.st.RecordUsage(apiKey, "", req.Model, 0, 0, false)
		writeError(w, http.StatusBadGateway, err.Error(), "api_error", nil)
		return
	}
	_ = s.st.RecordUsage(apiKey, res.Engine, req.Model,
		estimateTokens(messagesText(req.Messages)), estimateTokens(res.Text), true)
	resp := model.NewCompletion(req.Model, res.Text)
	resp.ID = "chatcmpl-" + model.NewID(12)
	resp.Usage.PromptTokens = int(estimateTokens(messagesText(req.Messages)))
	resp.Usage.CompletionTokens = int(estimateTokens(res.Text))
	resp.Usage.TotalTokens = resp.Usage.PromptTokens + resp.Usage.CompletionTokens
	writeJSON(w, http.StatusOK, resp)
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
