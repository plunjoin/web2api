//go:build web2api_unit

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
		{"admin.css", "text/css", ".btn-primary"},
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

// A release must never pair new HTML with a cached stylesheet from an older one:
// the page links content-hashed asset URLs and the assets carry validators.
func TestIndexLinksContentHashedAssets(t *testing.T) {
	rec := httptest.NewRecorder()
	ServeIndex(rec, httptest.NewRequest(http.MethodGet, "/admin/", nil))
	body := rec.Body.String()
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("index must not be cached, got %q", rec.Header().Get("Cache-Control"))
	}
	for _, name := range []string{"admin.css", "vue.global.prod.js"} {
		url := AssetURL(name)
		if !strings.Contains(url, "?v=") || !strings.Contains(body, `"`+url+`"`) {
			t.Fatalf("index must link %s with a content hash, want %q", name, url)
		}
		if strings.Contains(body, `"/admin/assets/`+name+`"`) {
			t.Fatalf("index still links the unversioned %s", name)
		}

		versioned := httptest.NewRecorder()
		Assets().ServeHTTP(versioned, httptest.NewRequest(http.MethodGet, url, nil))
		if versioned.Code != http.StatusOK || !strings.Contains(versioned.Header().Get("Cache-Control"), "immutable") || versioned.Header().Get("ETag") == "" {
			t.Fatalf("%s: status=%d cache=%q etag=%q", url, versioned.Code, versioned.Header().Get("Cache-Control"), versioned.Header().Get("ETag"))
		}

		plain := httptest.NewRecorder()
		Assets().ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/admin/assets/"+name, nil))
		if plain.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("unversioned %s must revalidate, got %q", name, plain.Header().Get("Cache-Control"))
		}

		req := httptest.NewRequest(http.MethodGet, "/admin/assets/"+name, nil)
		req.Header.Set("If-None-Match", versioned.Header().Get("ETag"))
		revalidated := httptest.NewRecorder()
		Assets().ServeHTTP(revalidated, req)
		if revalidated.Code != http.StatusNotModified {
			t.Fatalf("%s revalidation: want 304, got %d", name, revalidated.Code)
		}
	}
}
