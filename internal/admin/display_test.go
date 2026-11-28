package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"web2api/internal/logging"
	"web2api/internal/store"
)

func TestUsageAPIsAttachModelDisplayNames(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "display.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.ReplaceEngineCatalog("geminiweb", []store.CatalogModel{
		{Engine: "geminiweb", Model: "models/gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReplaceEngineCatalog("custom|https://example.com", []store.CatalogModel{
		{Engine: "custom|https://example.com", Model: "qwen3.5-397b", DisplayName: "Qwen3.5-397B"},
		{Engine: "custom|https://example.com", Model: "deepseek-v4.1-flash", DisplayName: "deepseek-v4.1-flash"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RecordRequest(store.UsageRecord{TS: time.Now().Unix(), Engine: "geminiweb", Model: "gemini-2.5-flash", Endpoint: "/v1/chat/completions", Success: true}); err != nil {
		t.Fatal(err)
	}
	a := &API{st: st, logger: logging.NewTest(io.Discard)}

	t.Run("admin usage records", func(t *testing.T) {
		w := httptest.NewRecorder()
		a.handleUsageRecords(w, httptest.NewRequest(http.MethodGet, "/admin/api/usage/records?days=3650", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var body struct {
			Records []struct {
				ModelDisplayName string `json:"model_display_name"`
			} `json:"records"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Records) != 1 || body.Records[0].ModelDisplayName != "Gemini 2.5 Flash" {
			t.Fatalf("display name missing: %s", w.Body.String())
		}
	})

	t.Run("admin models", func(t *testing.T) {
		w := httptest.NewRecorder()
		a.handleModels(w, httptest.NewRequest(http.MethodGet, "/admin/api/models", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var body struct {
			Models []struct {
				ID          string `json:"id"`
				Engine      string `json:"engine"`
				DisplayName string `json:"display_name"`
			} `json:"models"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		byID := map[string]struct {
			Engine      string
			DisplayName string
		}{}
		for _, m := range body.Models {
			byID[m.ID] = struct {
				Engine      string
				DisplayName string
			}{m.Engine, m.DisplayName}
		}
		if got := byID["qwen3.5-397b"]; got.DisplayName != "Qwen3.5-397B" || got.Engine != "custom|https://example.com" {
			t.Fatalf("快照展示名应保留: %+v", body.Models)
		}
		if got := byID["deepseek-v4.1-flash"]; got.DisplayName != "" {
			t.Fatalf("等于 ID 的展示名应视为空: %+v", body.Models)
		}
	})

	t.Run("admin prices", func(t *testing.T) {
		if _, err := st.SetModelPrice(store.ModelPrice{Model: "gemini-2.5-flash", Input: 1, Output: 2, CacheRead: 3}); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		a.handleListPrices(w, httptest.NewRequest(http.MethodGet, "/admin/api/prices", nil), nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var body struct {
			Prices []struct {
				DisplayName string `json:"display_name"`
			} `json:"prices"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		for _, p := range body.Prices {
			if p.DisplayName == "Gemini 2.5 Flash" {
				return
			}
		}
		t.Fatalf("display name missing: %s", w.Body.String())
	})
}
