package webui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeIndexWithBrand(t *testing.T) {
	rec := httptest.NewRecorder()
	ServeIndexWithBrand(rec, httptest.NewRequest("GET", "/admin", nil), `A&B "站点"`, `https://cdn.example.com/logo.png?a=1&b=2`)
	body := rec.Body.String()
	if !strings.Contains(body, `<title>A&amp;B &#34;站点&#34;</title>`) {
		t.Fatalf("site name was not injected into title")
	}
	if !strings.Contains(body, `href="https://cdn.example.com/logo.png?a=1&amp;b=2"`) {
		t.Fatalf("site logo was not injected into favicon")
	}
}
