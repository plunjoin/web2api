package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// All page dependencies must be served by the binary even with no CDN access.
func TestEmbeddedAssets(t *testing.T) {
	for _, asset := range []struct{ name, contentType, marker string }{
		{"vue.global.prod.js", "javascript", "Vue"},
		{"admin.css", "text/css", ".bg-slate-900"},
	} {
		t.Run(asset.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Assets().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/assets/"+asset.name, nil))
			if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), asset.contentType) || !strings.Contains(rec.Body.String(), asset.marker) {
				t.Fatalf("embedded asset unavailable: status=%d type=%q", rec.Code, rec.Header().Get("Content-Type"))
			}
		})
	}
	rec := httptest.NewRecorder()
	ServeIndex(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if strings.Contains(rec.Body.String(), "cdn.jsdelivr.net") || !strings.Contains(rec.Body.String(), "/admin/assets/vue.global.prod.js") {
		t.Fatal("page must load its JavaScript from the embedded asset route")
	}
}
