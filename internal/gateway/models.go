package gateway

import (
	"net/http"
	"time"

	"web2api/internal/model"
)

// handleModels 处理 GET /v1/models。
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	models := s.mgr.ListModels()
	if models == nil {
		models = []model.ModelInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   models,
	})
}

// handleHealth 处理 GET /health（免鉴权探活）。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	engA := s.mgr.EngineA()
	engB := s.mgr.EngineB()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"uptime":     time.Since(s.startAt).String(),
		"engine_a":   engA != nil && engA.Ready(),
		"engine_b":   engB != nil && engB.Ready(),
		"gemini_api": s.gemini.Enabled(),
	})
}

// handleAccounts 处理 GET /v1/accounts。
func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"accounts": s.mgr.Status(),
	})
}
