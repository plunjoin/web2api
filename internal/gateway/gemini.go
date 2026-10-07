package gateway

import (
	"net/http"

	"web2api/internal/geminiapi"
)

func (s *Server) withGeminiAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Authenticate SDK x-goog-api-key / ?key against the local key store.
		clone := r.Clone(r.Context())
		if clone.Header.Get("Authorization") == "" {
			if key := geminiapi.AuthKey(r); key != "" {
				clone.Header.Set("Authorization", "Bearer "+key)
			}
		}
		s.withAuth(next)(w, clone)
	}
}

func (s *Server) handleGeminiDocs(w http.ResponseWriter, r *http.Request) {
	spec := geminiapi.GatewaySpec()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, spec)
}
