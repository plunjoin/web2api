package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// handlePassthrough 将多模态端点透传到上游。
// 覆盖路径：/v1/images/*、/v1/videos/*、/v1/audio/*、/v1/files/*、/v1/embeddings/*
// 路由顺序：请求体 model 命中订阅/custom 引擎目录时，转发到对应第三方 Key 上游
// （embedding 等模型由 OpenAI 兼容供应商提供）；否则回退引擎B（仅 upstream 模式）。
func (s *Server) handlePassthrough(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if modelID, body := passthroughModelFromBody(r); modelID != "" {
		rw := &recordingWriter{ResponseWriter: w}
		handled, err := s.mgr.ServeModelPassthrough(r.Context(), modelID, r.Method, path, bytes.NewReader(body), r.Header.Get("Content-Type"), rw)
		if handled {
			if err != nil {
				s.logger.Printf("[passthrough] %s %s 模型路由失败 model=%q: %v", r.Method, path, modelID, err)
				if !rw.wrote {
					writeError(w, http.StatusBadGateway, "上游转发失败: "+err.Error(), "api_error", nil)
				}
			}
			return
		}
	}
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

// passthroughModelFromBody 读取 JSON 请求体的 model 字段（供模型路由），
// 并把请求体重置为可重复读。返回空 model 表示不适用（非 JSON / 无 model）。
func passthroughModelFromBody(r *http.Request) (string, []byte) {
	if r.Body == nil || !strings.Contains(r.Header.Get("Content-Type"), "json") {
		return "", nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		return "", nil
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var probe struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &probe) != nil {
		return "", body
	}
	model := strings.TrimSpace(probe.Model)
	if model == "" {
		return "", body
	}
	return strings.TrimPrefix(model, "models/"), body
}

// recordingWriter 记录响应是否已开始写出；转发中途失败（连接断开等）时，
// 用它判断还能否改写为错误响应，避免给客户端留下空 200。
type recordingWriter struct {
	http.ResponseWriter
	wrote bool
}

func (rw *recordingWriter) WriteHeader(code int) {
	rw.wrote = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *recordingWriter) Write(p []byte) (int, error) {
	rw.wrote = true
	return rw.ResponseWriter.Write(p)
}
