package gateway

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed export-storage.sh export-storage.js
var exportToolScripts embed.FS

func (s *Server) handleExportToolScript(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/tools/")
	if name != "export-storage.sh" && name != "export-storage.js" {
		http.NotFound(w, r)
		return
	}
	data, err := exportToolScripts.ReadFile(name)
	if err != nil {
		http.Error(w, "script unavailable", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasSuffix(name, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	_, _ = w.Write(data)
}
