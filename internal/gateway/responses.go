package gateway

import (
	"io"
	"net/http"
	"time"

	"web2api/internal/engine"
	"web2api/internal/ir"
	"web2api/internal/model"
	"web2api/internal/provider"
	"web2api/internal/store"
	"web2api/internal/wire/responses"
)

// handleResponses 处理 POST /v1/responses。
//
// 仅做协议无关的中枢职责：读 body → wire.responses 解码 → 模型白名单 → manager
// → wire.responses 编码 → 写 body。请求解析与 response.* 事件流的构造都下沉到了
// wire/responses，本函数不再持有任何协议形状。
func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取请求体失败", "invalid_request_error", nil)
		return
	}
	chatReq, err := responses.Decode(body, s.logger.Printf)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
		return
	}
	// 统一 IR 入口：与 /v1/chat/completions 走同一套规范化不变量。
	chatReq = ir.NormalizeChat(chatReq, s.logger.Printf)
	if !s.modelAllowed(w, r, chatReq.Model) {
		return
	}

	start := time.Now()
	if chatReq.Stream {
		s.handleResponsesStream(w, r, chatReq, start)
		return
	}

	ctx := engine.WithAccountTracking(provider.WithSessionHint(r.Context(), sessionHintFrom(r)))
	res, err := s.mgr.Chat(ctx, chatReq)
	if err != nil {
		s.logger.Printf("[responses] 请求失败 model=%q: %v", chatReq.Model, err)
		s.recordChat(ctx, r, chatReq, res, false, err, start, "/v1/responses", upstreamErrorStatus(err), 0)
		s.writeResponsesError(w, err)
		return
	}
	rec := s.recordChat(ctx, r, chatReq, res, false, nil, start, "/v1/responses", http.StatusOK, 0)
	usage := usageFromRecord(rec)
	writeJSON(w, http.StatusOK, responses.NonStreamResponse(chatReq, res, usage))
}

// handleResponsesStream 以 response.* 事件流返回结果。
func (s *Server) handleResponsesStream(w http.ResponseWriter, r *http.Request, req model.ChatRequest, start time.Time) {
	sse := newSSEStream(w)
	if err := sse.Start(); err != nil {
		s.logger.Printf("[responses] 初始化流失败: %v", err)
		return
	}
	defer sse.Close()

	ctx := engine.WithAccountTracking(provider.WithSessionHint(r.Context(), sessionHintFrom(r)))
	enc := responses.NewStreamEncoder(sse, req.Model)
	enc.Open()
	sink := newTimedSink(enc.Sink(), start)
	res, runErr := s.mgr.ChatStream(ctx, req, sink)
	if runErr != nil {
		s.logger.Printf("[responses] 流式请求失败 model=%q: %v", req.Model, runErr)
		s.recordChat(ctx, r, req, res, true, runErr, start, "/v1/responses", http.StatusOK, sink.TTFT())
		enc.EmitFailed(runErr)
		return
	}
	rec := s.recordChat(ctx, r, req, res, true, nil, start, "/v1/responses", http.StatusOK, sink.TTFT())
	enc.Finish(res, usageFromRecord(rec))
}

// writeResponsesError 复用 chat 的错误语义：模型无人声明 404、
// 端点与模型用途不匹配 400，其余 502。
func (s *Server) writeResponsesError(w http.ResponseWriter, err error) {
	if provider.IsModelDisabled(err) {
		writeError(w, http.StatusNotFound, err.Error(), "invalid_request_error", "model_unavailable")
		return
	}
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
}

// usageFromRecord 把落库的用量记录转成协议用量。
func usageFromRecord(rec store.UsageRecord) model.TokenUsage {
	return model.TokenUsage{
		PromptTokens:     rec.PromptTokens,
		CompletionTokens: rec.CompletionTokens,
		TotalTokens:      rec.TotalTokens,
		Estimated:        rec.Estimated,
	}.Normalize()
}
