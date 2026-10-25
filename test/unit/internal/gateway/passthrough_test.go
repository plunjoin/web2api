//go:build web2api_unit

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
	"sync"
	"testing"
	"time"

	"web2api/internal/config"
	"web2api/internal/provider"
	"web2api/internal/store"
)

// mockCustomUpstream 模拟第三方 OpenAI 兼容供应商（含向量端点）。
func mockCustomUpstream(t *testing.T) (*httptest.Server, *customUpstreamCalls) {
	t.Helper()
	calls := &customUpstreamCalls{}
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		calls.record(r, "")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "models/gemini-embedding-001", "object": "model"},
				{"id": "gpt-image-test", "object": "model"},
			},
		})
	})

	mux.HandleFunc("/v1/embeddings", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls.record(r, body["model"].(string))
		if r.Header.Get("Authorization") != "Bearer sk-up" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"model":  body["model"],
			"data":   []map[string]any{{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2, 0.3}}},
		})
	})

	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls.record(r, "")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-custom", "object": "chat.completion",
			"created": time.Now().Unix(), "model": body["model"],
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "custom 上游回复"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	})

	mux.HandleFunc("/v1/images/generations", func(w http.ResponseWriter, r *http.Request) {
		calls.record(r, "")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"url": "https://example.com/custom-image.png"}},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, calls
}

type customUpstreamCalls struct {
	mu        sync.Mutex
	auths     []string
	models    []string
	endpoints []string
}

func (c *customUpstreamCalls) record(r *http.Request, model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.auths = append(c.auths, r.Header.Get("Authorization"))
	c.models = append(c.models, model)
	c.endpoints = append(c.endpoints, r.URL.Path)
}

func (c *customUpstreamCalls) embeddingsModels() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []string{}
	for i, ep := range c.endpoints {
		if ep == "/v1/embeddings" {
			out = append(out, c.models[i])
		}
	}
	return out
}

// newPassthroughTestServer 引擎B（upstream 模式）+ custom 引擎各指向不同 mock，
// 验证多模态透传按模型路由到 custom 上游。
func newPassthroughTestServer(t *testing.T) (*Server, *customUpstreamCalls) {
	t.Helper()
	engineB := mockEngineB(t)
	customUp, calls := mockCustomUpstream(t)
	cfg := &config.Config{
		Server:  config.ServerConfig{Listen: "127.0.0.1:0", APIKeys: []string{"sk-gw"}, DBPath: filepath.Join(t.TempDir(), "test.db")},
		Routing: config.RoutingConfig{DefaultEngine: "auto"},
		EngineB: config.EngineBConfig{Enabled: true, Mode: "upstream", BaseURL: engineB.URL, Passthrough: true},
		Engines: config.EnginesConfig{Custom: config.CustomConfig{Enabled: true}},
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
	srv, err := NewServer(mgr, st, cfg.Server.APIKeys, 0, 0, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	creds, _ := json.Marshal(provider.CustomCredentials{
		BaseURL: customUp.URL + "/v1",
		APIKey:  "sk-up",
		Models:  []string{"models/gemini-embedding-001", "gpt-image-test"},
	})
	if _, err := srv.mgr.AddAccountFromStorageState(provider.AccountInput{
		Engine: "custom", Label: "mock-custom", Credentials: json.RawMessage(creds),
	}); err != nil {
		t.Fatal(err)
	}
	return srv, calls
}

func TestGatewayEmbeddingsRouteToCustom(t *testing.T) {
	srv, calls := newPassthroughTestServer(t)

	do := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer sk-gw")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}

	// 带前缀的模型 ID（/v1/models 原样返回的形式）
	rec := do(`{"model":"models/gemini-embedding-001","input":"你好"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("带前缀模型应转发 custom 上游: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"embedding"`) {
		t.Fatalf("向量响应错误: %s", rec.Body.String())
	}

	// 裸模型名也应命中目录（models/ 前缀宽松匹配）
	rec = do(`{"model":"gemini-embedding-001","input":"你好"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("裸模型名应转发 custom 上游: %d %s", rec.Code, rec.Body.String())
	}

	models := calls.embeddingsModels()
	if len(models) != 2 {
		t.Fatalf("custom 上游应收到 2 次 embeddings 请求: %v", calls.endpoints)
	}
	if models[0] != "models/gemini-embedding-001" || models[1] != "gemini-embedding-001" {
		t.Fatalf("上游收到的 model 应原样转发: %v", models)
	}

	// 未知模型回退引擎B（upstream mock 无 embeddings 端点 → 404）
	rec = do(`{"model":"no-such-model","input":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未知模型应回退引擎B: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayImagesRouteToCustomByModel(t *testing.T) {
	srv, _ := newPassthroughTestServer(t)

	// gpt-image-test 命中 custom 目录 → 转发 custom 上游
	body := `{"model":"gpt-image-test","prompt":"a cat"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer sk-gw")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "custom-image.png") {
		t.Fatalf("生图应按模型路由到 custom 上游: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayChatRoutesToCustomWithBareModel(t *testing.T) {
	srv, _ := newPassthroughTestServer(t)

	// 目录带 models/ 前缀时，聊天请求用裸名也应命中 custom 引擎
	body := `{"model":"gemini-embedding-001","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer sk-gw")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("裸名聊天应路由到 custom 引擎: %d %s", rec.Code, rec.Body.String())
	}
}
