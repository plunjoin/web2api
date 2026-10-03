package gateway

import (
	"net/http"
)

// handlePassthrough 将多模态端点透传到引擎B（AI Studio）。
// 覆盖路径：/v1/images/*、/v1/videos/*、/v1/audio/*、/v1/files/*、/v1/embeddings/*
func (s *Server) handlePassthrough(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	eng := s.mgr.PassthroughEngine()
	if eng == nil {
		writeError(w, http.StatusBadGateway, "未启用支持多模态透传的引擎（引擎B）", "api_error", nil)
		return
	}
	err := eng.ServePassthrough(r.Context(), r.Method, path, r.Body, r.Header.Get("Content-Type"), w)
	if err != nil {
		// 头可能已写出，无法再 writeError；尝试记录
		s.logger.Printf("[passthrough] %s %s 失败: %v", r.Method, path, err)
	}
}
