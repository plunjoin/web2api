package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"web2api/internal/config"
	"web2api/internal/provider"
	"web2api/internal/store"
)

// mockEngineB 模拟引擎B（OpenAI 兼容上游）。
func mockEngineB(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "gemini-3.7-flash", "object": "model"},
				{"id": "gemini-3.1-flash-image", "object": "model"},
			},
		})
	})

	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		stream, _ := body["stream"].(bool)
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			flusher, _ := w.(http.Flusher)
			for _, tok := range []string{"你", "好", "！"} {
				chunk := map[string]any{
					"id": "chatcmpl-mock", "object": "chat.completion.chunk",
					"created": time.Now().Unix(), "model": body["model"],
					"choices": []map[string]any{{"index": 0, "delta": map[string]any{"content": tok}}},
				}
				data, _ := json.Marshal(chunk)
				_, _ = w.Write([]byte("data: "))
				_, _ = w.Write(data)
				_, _ = w.Write([]byte("\n\n"))
				flusher.Flush()
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-mock", "object": "chat.completion",
			"created": time.Now().Unix(), "model": body["model"],
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "模拟引擎B回复"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	})

	mux.HandleFunc("/v1/images/generations", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"created": time.Now().Unix(),
			"data":    []map[string]any{{"url": "https://example.com/mock.png"}},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestServer(t *testing.T, upstreamURL string, apiKeys []string) *Server {
	t.Helper()
	cfg := &config.Config{
		Server:  config.ServerConfig{Listen: "127.0.0.1:0", APIKeys: apiKeys, DBPath: filepath.Join(t.TempDir(), "test.db")},
		Routing: config.RoutingConfig{DefaultEngine: "auto"},
		EngineA: config.EngineAConfig{Enabled: true},
		EngineB: config.EngineBConfig{
			Enabled:     true,
			Mode:        "upstream",
			BaseURL:     upstreamURL,
			Passthrough: true,
		},
	}
	st, err := store.Open(cfg.Server.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mgr, err := provider.NewManager(cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = mgr.Init(ctx)
	srv, err := NewServer(mgr, st, apiKeys, 0, 0, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func TestGatewayHealth(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("健康检查状态码错误: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("健康检查响应异常: %s", rec.Body.String())
	}
}

func TestGatewayAuth(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, []string{"sk-test"})

	// 无 Authorization 头 → 401
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，实际 %d", rec.Code)
	}

	// 错误 key → 401
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req2.Header.Set("Authorization", "Bearer wrong")
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，实际 %d", rec2.Code)
	}
}

func TestGatewayChatNonStream(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, []string{"sk-key"})

	body := `{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"你好"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer sk-key")
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("对话请求失败: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "模拟引擎B回复") {
		t.Fatalf("响应内容错误: %s", rec.Body.String())
	}
}

func TestGatewayChatStream(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, []string{"sk-test"})

	body := `{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"}],"stream":true}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)

	out := rec.Body.String()
	if !strings.Contains(out, "data:") {
		t.Fatalf("缺少 SSE data: %s", out)
	}
	if !strings.Contains(out, "[DONE]") {
		t.Fatalf("缺少 [DONE]: %s", out)
	}
	// 三个 chunk 独立发送，逐字检查
	for _, tok := range []string{"你", "好", "！"} {
		if !strings.Contains(out, tok) {
			t.Fatalf("流式内容缺失 %q: %s", tok, out)
		}
	}
}

func TestGatewayModels(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("模型列表失败: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "gemini-3.7-flash") {
		t.Fatalf("模型列表缺少引擎B模型: %s", rec.Body.String())
	}
}

func TestGatewayPassthrough(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, nil)

	body := `{"model":"gemini-3.1-flash-image","prompt":"a cat","n":1}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("透传失败: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "mock.png") {
		t.Fatalf("透传响应错误: %s", rec.Body.String())
	}
}

func TestGatewayRoot(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("根路径失败: %d", rec.Code)
	}
}
