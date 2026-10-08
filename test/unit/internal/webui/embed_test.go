//go:build web2api_unit

package webui

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var assetRef = regexp.MustCompile(`(?:src|href)="(/admin/assets/[^"]+)"`)

func indexBody(t *testing.T) (string, []string) {
	t.Helper()
	rec := httptest.NewRecorder()
	ServeIndex(rec, httptest.NewRequest(http.MethodGet, "/admin/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("index status %d", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("index must not be cached, got %q", rec.Header().Get("Cache-Control"))
	}
	body := rec.Body.String()
	var refs []string
	for _, m := range assetRef.FindAllStringSubmatch(body, -1) {
		refs = append(refs, m[1])
	}
	return body, refs
}

// All page dependencies must be served by the binary even with no CDN access.
func TestEmbeddedAssets(t *testing.T) {
	body, refs := indexBody(t)
	if strings.Contains(body, "cdn.jsdelivr.net") || strings.Contains(body, "unpkg.com") {
		t.Fatal("page must not load anything from a CDN")
	}
	var js, css bool
	for _, ref := range refs {
		rec := httptest.NewRecorder()
		Assets().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ref, nil))
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Fatalf("%s: status=%d", ref, rec.Code)
		}
		ct := rec.Header().Get("Content-Type")
		switch {
		case strings.HasSuffix(ref, ".js"):
			js = js || strings.Contains(ct, "javascript")
		case strings.HasSuffix(ref, ".css"):
			css = css || strings.Contains(ct, "text/css")
		}
	}
	if !js || !css {
		t.Fatalf("index must reference embedded JS and CSS, refs=%v", refs)
	}
}

// A release must never pair new HTML with cached assets from an older one: every asset the
// page links is content-addressed (hash in the file name), cached forever, and revalidatable.
// The page must not append ?v= to those names: chunks import each other as ./index-xxx.js,
// and a second URL for the same module would execute the app twice.
func TestIndexLinksContentHashedAssets(t *testing.T) {
	body, refs := indexBody(t)
	if strings.Contains(body, "?v=") {
		t.Fatal("content-hashed assets must be linked by their plain name")
	}
	for _, ref := range refs {
		name := strings.TrimPrefix(ref, "/admin/assets/")
		if !isContentHashed(name) {
			t.Fatalf("%s is not content-hashed", name)
		}
		if AssetURL(name) != ref {
			t.Fatalf("AssetURL(%q) = %q, want %q", name, AssetURL(name), ref)
		}
		rec := httptest.NewRecorder()
		Assets().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ref, nil))
		if !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") || rec.Header().Get("ETag") == "" {
			t.Fatalf("%s: cache=%q etag=%q", ref, rec.Header().Get("Cache-Control"), rec.Header().Get("ETag"))
		}
		req := httptest.NewRequest(http.MethodGet, ref, nil)
		req.Header.Set("If-None-Match", rec.Header().Get("ETag"))
		revalidated := httptest.NewRecorder()
		Assets().ServeHTTP(revalidated, req)
		if revalidated.Code != http.StatusNotModified {
			t.Fatalf("%s revalidation: want 304, got %d", ref, revalidated.Code)
		}
	}
}

func TestContentHashedNames(t *testing.T) {
	for name, want := range map[string]bool{
		"index-DTrA9p-0.js":  true,
		"index-BH6H87Ct.css": true,
		"users-DskD3b5Z.js":  true,
		"admin.css":          false,
		"vue.global.prod.js": false,
		"index.js":           false,
	} {
		if got := isContentHashed(name); got != want {
			t.Errorf("isContentHashed(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestMissingAssetIs404(t *testing.T) {
	rec := httptest.NewRecorder()
	Assets().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/assets/nope-12345678.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}
