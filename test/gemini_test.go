package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"web2api/internal/config"
	"web2api/internal/gateway"
	"web2api/internal/geminiapi"
	"web2api/internal/provider"
	"web2api/internal/store"
)

func officialGateway(t *testing.T, upstream string, enabled bool) (*gateway.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := &config.Config{GeminiAPI: config.GeminiAPIConfig{Enabled: enabled, BaseURL: upstream, APIKey: "google-server-key"}, EngineB: config.EngineBConfig{Enabled: !enabled, Mode: "upstream", BaseURL: upstream}}
	mgr, err := provider.NewManager(cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := gateway.NewServer(mgr, st, []string{"sk-local"}, 0, 0, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	return srv, st
}

func TestOfficialParametersAndResponsesRemainIntact(t *testing.T) {
	payload := `{"model":"gemini-nano-banana-2.1","input":[{"type":"image","data":"aGVsbG8=","mime_type":"image/png","resolution":"high"},{"type":"text","text":"编辑图片","annotations":[{"type":"speech_metadata","speaker":"A","style":"calm"}]}],"system_instruction":"中文回答","generation_config":{"max_output_tokens":4096,"seed":123,"stop_sequences":["END"],"thinking_level":"high","thinking_summaries":"auto","tool_choice":{"allowed_tools":{"mode":"any","tools":["f"]}},"speech_config":{"speakers":[{"speaker":"A","voice":"Kore","language":"zh-CN"}]},"transcription_config":{"mode":{"type":"verbatim","diarization_mode":"speaker","timestamp_granularities":["word"]},"custom_vocabulary":["web2api"],"language_codes":["zh-CN"]},"video_config":{"task":"edit"}},"response_format":[{"type":"image","aspect_ratio":"16:9","image_size":"2K","mime_type":"image/jpeg","delivery":"uri"},{"type":"text","mime_type":"application/json","schema":{"type":"object","properties":{"text":{"type":"string"}}}}],"tools":[{"type":"retrieval","retrieval_types":["vertex_ai_search"],"vertex_ai_search_config":{"datastores":["store"],"engine":"engine"}},{"type":"mcp_server","name":"test","url":"https://example.com/mcp","headers":{"Authorization":"Bearer mcp-token"},"allowed_tools":[{"mode":"auto","tools":["f"]}]}],"previous_interaction_id":"parent","store":true,"stream":false,"background":true,"service_tier":"priority","safety_settings":[{"type":"harassment","threshold":"off","method":"probability"}],"environment":{"type":"remote","env":{"TOKEN":{"credential":"credential-id"}},"network":{"allowlist":[{"domain":"example.com","credential":"c","transform":[{"X-Custom":"v"}]}]},"sources":[{"type":"inline","target":"a.txt","content":"hello","encoding":"utf8"}]},"webhook_config":{"uris":["https://example.com/callback"],"user_metadata":{"task":1}},"labels":{"project":"test"},"continuation_token":"opaque-token","future_field":{"unknown":true}}`
	response := `{"id":"official-id","status":"incomplete","continuation_token":"next-token","steps":[{"type":"thought","signature":"sig","summary":[{"type":"text","text":"summary"}]},{"type":"function_call","id":"call","name":"f","arguments":{"x":1}},{"type":"model_output","content":[{"type":"image","uri":"https://example.com/image","mime_type":"image/jpeg"},{"type":"text","text":"回答","annotations":[{"type":"url_citation","url":"https://example.com","start_index":0,"end_index":6}]}]}],"usage":{"total_thought_tokens":10,"total_cached_tokens":20,"grounding_tool_count":[{"type":"google_search","count":2}],"input_tokens_by_modality":[{"modality":"image","tokens":100}]}}`
	errCh := make(chan string, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		err := ""
		if string(body) != payload || r.URL.Path != "/v1beta/interactions" {
			err = "payload/path changed"
		}
		if r.Header.Get("X-Goog-Api-Key") != "google-server-key" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			err = "credential replacement failed"
		}
		if r.URL.Query().Get("key") != "" || r.URL.Query().Get("custom") != "1" {
			err = "query handling failed"
		}
		if r.Header.Get("Api-Revision") != "2026-05-20" {
			err = "revision header lost"
		}
		errCh <- err
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "google-id")
		_, _ = io.WriteString(w, response)
	}))
	defer up.Close()
	srv, _ := officialGateway(t, up.URL, true)
	req := httptest.NewRequest("POST", "/v1beta/interactions?key=sk-local&custom=1", strings.NewReader(payload))
	req.Header.Set("Api-Revision", "2026-05-20")
	req.Header.Set("Cookie", "private=caller")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != response || w.Header().Get("X-Request-Id") != "google-id" {
		t.Fatalf("response changed: %d %s", w.Code, w.Body.String())
	}
	if err := <-errCh; err != "" {
		t.Fatal(err)
	}
}

func TestEveryOfficialOperationHasGatewayRoute(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-Api-Key") != "google-server-key" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"method": r.Method, "path": r.URL.Path})
	}))
	defer up.Close()
	srv, _ := officialGateway(t, up.URL, true)
	count := 0
	for path, value := range geminiapi.Spec()["paths"].(map[string]any) {
		for _, version := range []string{"v1", "v1beta"} {
			actual := strings.ReplaceAll(path, "{api_version}", version)
			for _, parameter := range []string{"interactionsId", "agentsId", "environment", "voicesId", "triggerId", "id", "path"} {
				actual = strings.ReplaceAll(actual, "{"+parameter+"}", "sample")
			}
			for _, method := range []string{"get", "post", "delete", "patch", "put"} {
				if _, ok := value.(map[string]any)[method]; !ok {
					continue
				}
				t.Run(method+actual, func(t *testing.T) {
					req := httptest.NewRequest(strings.ToUpper(method), actual, strings.NewReader(`{"future_field":true}`))
					req.Header.Set("X-Goog-Api-Key", "sk-local")
					w := httptest.NewRecorder()
					srv.Handler().ServeHTTP(w, req)
					var result map[string]string
					_ = json.Unmarshal(w.Body.Bytes(), &result)
					if w.Code != 200 || result["method"] != strings.ToUpper(method) || result["path"] != actual {
						t.Fatalf("route missing: %d %s", w.Code, w.Body.String())
					}
				})
				count++
			}
		}
	}
	if count == 0 {
		t.Fatal("empty source spec")
	}
	t.Logf("verified %d official operations across v1/v1beta", count)
}

func TestOfficialEscapedPaths(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.EscapedPath())
	}))
	defer up.Close()
	srv, _ := officialGateway(t, up.URL+"/api", true)
	for _, prefix := range []string{"", "/gemini"} {
		path := "/v1beta/environments/env/files/a%2Fb%20%E4%B8%AD.txt"
		req := httptest.NewRequest("GET", prefix+path, nil)
		req.Header.Set("X-Goog-Api-Key", "sk-local")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != 200 || w.Body.String() != "/api"+path {
			t.Fatalf("escaped path changed: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestOfficialAuthFailuresAndErrors(t *testing.T) {
	errorBody := `{"error":{"code":429,"message":"quota","details":[{"retryDelay":"3s"}]}}`
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(429)
		_, _ = io.WriteString(w, errorBody)
	}))
	defer up.Close()
	srv, _ := officialGateway(t, up.URL, true)
	for _, kind := range []string{"bearer", "google", "query", "invalid", "missing"} {
		t.Run(kind, func(t *testing.T) {
			p := "/v1beta/interactions/id?include_input=true&stream=false&last_event_id=event-1"
			if kind == "query" {
				p += "&key=sk-local"
			}
			r := httptest.NewRequest("GET", p, nil)
			if kind == "bearer" {
				r.Header.Set("Authorization", "Bearer sk-local")
			}
			if kind == "google" {
				r.Header.Set("X-Goog-Api-Key", "sk-local")
			}
			if kind == "invalid" {
				r.Header.Set("X-Goog-Api-Key", "wrong")
			}
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, r)
			if kind == "invalid" || kind == "missing" {
				if w.Code != 401 {
					t.Fatal(w.Code)
				}
				return
			}
			if w.Code != 429 || w.Body.String() != errorBody || w.Header().Get("Retry-After") != "3" {
				t.Fatalf("error altered: %d %s", w.Code, w.Body)
			}
		})
	}
	disabled, _ := officialGateway(t, up.URL, false)
	r := httptest.NewRequest("POST", "/v1beta/interactions", nil)
	r.Header.Set("X-Goog-Api-Key", "sk-local")
	w := httptest.NewRecorder()
	disabled.Handler().ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("disabled backend: %d", w.Code)
	}
}

func TestOfficialSSEFlushAndCancellation(t *testing.T) {
	finished := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: step.delta\nid: e1\ndata: {\"event_type\":\"step.delta\",\"event_id\":\"e1\",\"index\":0,\"delta\":{\"type\":\"text\",\"text\":\"hi\"}}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(finished)
	}))
	defer up.Close()
	srv, _ := officialGateway(t, up.URL, true)
	host := httptest.NewServer(srv.Handler())
	defer host.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", host.URL+"/v1beta/interactions/id?stream=true&last_event_id=e0", nil)
	r.Header.Set("X-Goog-Api-Key", "sk-local")
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	line := make([]byte, len("event: step.delta\n"))
	if _, err := io.ReadFull(response.Body, line); err != nil || string(line) != "event: step.delta\n" {
		t.Fatalf("SSE buffered or altered: %s %v", line, err)
	}
	_ = response.Body.Close()
	cancel()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect did not cancel upstream")
	}
}

func TestResumableUploadUsesSQLiteAndStreamsBinary(t *testing.T) {
	payload := bytes.Repeat([]byte{0x00, 0xff, 0x34, 0x0a}, (9<<20)/4)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-Api-Key") != "google-server-key" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/upload/v1beta/files" {
			w.Header().Set("X-Goog-Upload-URL", "http://"+r.Host+"/upload/session?upload_id=opaque&key=never-expose")
			w.WriteHeader(200)
			return
		}
		if r.URL.Path == "/upload/session" {
			body, _ := io.ReadAll(r.Body)
			if !bytes.Equal(body, payload) || r.Header.Get("X-Goog-Upload-Command") != "upload, finalize" || r.Header.Get("X-Goog-Upload-Offset") != "0" || r.URL.Query().Get("upload_id") != "opaque" {
				w.WriteHeader(400)
				return
			}
			_, _ = io.WriteString(w, `{"file":{"name":"files/uploaded","state":"ACTIVE","uri":"https://generativelanguage.googleapis.com/v1beta/files/uploaded"}}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer up.Close()
	srv, st := officialGateway(t, up.URL, true)
	start := httptest.NewRequest("POST", "/upload/v1beta/files", strings.NewReader(`{"file":{"display_name":"large.bin"}}`))
	start.Host = "gateway.example"
	start.Header.Set("X-Goog-Api-Key", "sk-local")
	start.Header.Set("X-Goog-Upload-Protocol", "resumable")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, start)
	location := w.Header().Get("X-Goog-Upload-URL")
	u, err := url.Parse(location)
	if err != nil || u.Host != "gateway.example" || !strings.HasPrefix(u.Path, "/gemini-upload/") || strings.Contains(location, "never-expose") {
		t.Fatalf("invalid upload location: %d %s", w.Code, location)
	}
	if _, err := st.GeminiUploadURL(strings.TrimPrefix(u.Path, "/gemini-upload/")); err != nil {
		t.Fatal("upload was not stored in SQLite", err)
	}
	// Recreate the official backend: session URLs are not tied to process memory.
	backend, err := geminiapi.New(config.GeminiAPIConfig{Enabled: true, BaseURL: up.URL, APIKey: "google-server-key"}, st)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	finish := httptest.NewRequest("POST", location, bytes.NewReader(payload))
	finish.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	finish.Header.Set("X-Goog-Upload-Offset", "0")
	w = httptest.NewRecorder()
	backend.ServeHTTP(w, finish)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "ACTIVE") {
		t.Fatalf("binary upload: %d %s", w.Code, w.Body)
	}
}

func TestOfficialDocsKeepSchemasAndValidReferences(t *testing.T) {
	srv, _ := officialGateway(t, "https://generativelanguage.googleapis.com", true)
	for _, endpoint := range []string{"/v1/docs", "/v1/gemini-docs"} {
		t.Run(endpoint, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", endpoint, nil))
			var spec map[string]any
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &spec) != nil {
				t.Fatal(w.Code)
			}
			paths := spec["paths"].(map[string]any)
			if paths["/v1beta/interactions"] == nil || paths["/v1/interactions/{interactionsId}/cancel"] == nil {
				t.Fatal("official operations omitted")
			}
			prefix := ""
			if endpoint == "/v1/docs" {
				prefix = "Gemini_"
			}
			schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
			for name, original := range geminiapi.Spec()["components"].(map[string]any)["schemas"].(map[string]any) {
				value, ok := schemas[prefix+name].(map[string]any)
				if !ok {
					t.Fatal("schema omitted", name)
				}
				for field := range properties(original) {
					if _, ok := properties(value)[field]; !ok {
						t.Fatal("field omitted", name, field)
					}
				}
			}
			var walk func(any)
			walk = func(value any) {
				switch v := value.(type) {
				case map[string]any:
					if ref, ok := v["$ref"].(string); ok && strings.HasPrefix(ref, "#/components/") {
						var target any = spec
						for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
							m, ok := target.(map[string]any)
							if !ok {
								t.Fatalf("bad ref %s", ref)
							}
							target = m[part]
						}
						if target == nil {
							t.Fatalf("unresolved %s", ref)
						}
					}
					for _, child := range v {
						walk(child)
					}
				case []any:
					for _, child := range v {
						walk(child)
					}
				}
			}
			walk(spec)
		})
	}
}

func properties(value any) map[string]any {
	m, _ := value.(map[string]any)
	p, _ := m["properties"].(map[string]any)
	return p
}

func TestGeminiConfigValidationAndOAuth(t *testing.T) {
	for _, cfg := range []config.GeminiAPIConfig{{Enabled: true}, {BaseURL: "file:///tmp/a"}, {BaseURL: "https://example.com/v1beta"}, {TimeoutSeconds: -1}} {
		if backend, err := geminiapi.New(cfg, nil); err == nil {
			backend.Close()
			t.Fatalf("invalid config accepted: %+v", cfg)
		}
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer server-oauth" {
			w.WriteHeader(401)
			return
		}
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer up.Close()
	backend, err := geminiapi.New(config.GeminiAPIConfig{Enabled: true, BaseURL: up.URL, AccessToken: "server-oauth"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	w := httptest.NewRecorder()
	backend.ServeHTTP(w, httptest.NewRequest("POST", "/v1beta/files:register", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
