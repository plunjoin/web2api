// Package gateway 提供 OpenAI 兼容 HTTP 网关 + 管理台：
//
//	POST /v1/chat/completions  — 对话（支持流式 SSE，按模型名路由双引擎，记录用量）
//	GET  /v1/models            — 聚合模型列表
//	GET  /health               — 健康检查（免鉴权）
//	GET  /v1/accounts          — 引擎与账号状态
//	/admin                     — Web 管理台（号池/Key/用量）
//	/admin/api/*               — 管理 REST API（JWT + Redis 会话）
//	GET  /v1/docs              — 对外 API OpenAPI 3.1 文档
//	/v1/images|videos|audio|files/* — 多模态透传（仅 upstream 模式引擎B）
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"web2api/internal/admin"
	"web2api/internal/geminiapi"
	"web2api/internal/limiter"
	"web2api/internal/model"
	"web2api/internal/provider"
	"web2api/internal/store"
	"web2api/internal/upgrade"
	"web2api/internal/webui"
)

// Server HTTP 网关。
type Server struct {
	mgr     *provider.Manager
	st      *store.Store
	apiKeys []string // 种子 Key（启动时导入库；空 = 无鉴权模式）
	limiter *limiter.Limiter
	httpSrv *http.Server
	startAt time.Time
	logger  *log.Logger
	updater *upgrade.Manager
	gemini  *geminiapi.Backend
	rpm     *rpmLimiter
}

// NewServer 创建网关服务。
func NewServer(mgr *provider.Manager, st *store.Store, seedKeys []string, rate, capacity float64, logger *log.Logger) (*Server, error) {
	if logger == nil {
		logger = log.Default()
	}
	// 种子 Key 导入（config 里的 api_keys 持久化为可管理 Key）
	for _, key := range seedKeys {
		if key != "" && key != "*" {
			_ = st.ImportKey(key, "config")
		}
	}
	gemini, err := geminiapi.New(mgr.GeminiAPIConfig(), st)
	if err != nil {
		return nil, err
	}
	return &Server{
		mgr:     mgr,
		st:      st,
		apiKeys: seedKeys,
		limiter: limiter.New(rate, capacity),
		logger:  logger,
		startAt: time.Now(),
		gemini:  gemini,
		rpm:     newRPMLimiter(),
	}, nil
}

// SetUpdater attaches deployment upgrade support before starting the server.
func (s *Server) SetUpdater(updater *upgrade.Manager) { s.updater = updater }

// Handler 返回 http.Handler。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/docs", s.handlePublicDocs)
	mux.HandleFunc("GET /v1/gemini-docs", s.handleGeminiDocs)
	for _, path := range geminiapi.Mounts() {
		mux.HandleFunc(path, s.withGeminiAuth(s.withModelGuard(s.gemini.ServeHTTP)))
	}
	// 本机登录导出工具：脚本本身不含凭据，运行时仍需管理员 JWT。
	mux.HandleFunc("GET /tools/export-storage.sh", s.handleExportToolScript)
	mux.HandleFunc("GET /tools/export-storage.js", s.handleExportToolScript)
	mux.HandleFunc("GET /v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", s.withAuth(s.handleChat))
	// Veo 视频长任务：创建、轮询和下载结果。
	mux.HandleFunc("POST /v1/videos", s.withAuth(s.handleCreateVideo))
	mux.HandleFunc("GET /v1/videos/{id}/content", s.withAuth(s.handleVideoContent))
	mux.HandleFunc("GET /v1/videos/{id}", s.withAuth(s.handleGetVideo))

	// 多模态透传
	for _, sub := range []string{"images", "videos", "audio", "files", "embeddings"} {
		mux.HandleFunc("/v1/"+sub+"/", s.withAuth(s.withModelGuard(s.handlePassthrough)))
		mux.HandleFunc("/v1/"+sub, s.withAuth(s.withModelGuard(s.handlePassthrough)))
	}

	mux.HandleFunc("GET /v1/accounts", s.withAuth(s.handleAccounts))

	// 管理台与 REST
	adminAPI := admin.New(s.mgr, s.st, s.logger, s.updater)
	adminAPI.Mount(mux)
	mux.Handle("GET /admin/assets/", webui.Assets())
	mux.HandleFunc("GET /admin", webui.ServeIndex)
	mux.HandleFunc("GET /admin/", webui.ServeIndex)

	mux.HandleFunc("/", s.handleRoot)

	return s.withLogging(mux)
}

// Start 启动 HTTP 服务（阻塞）。
func (s *Server) Start(listen string) error {
	s.httpSrv = &http.Server{
		Addr:              listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
	}
	s.logger.Printf("[gateway] OpenAI 兼容网关监听 %s", listen)
	s.logger.Printf("[gateway] 管理台: http://localhost:%s/admin", portOf(listen))
	return s.httpSrv.ListenAndServe()
}

// Shutdown 优雅关闭。
func (s *Server) Shutdown(ctx context.Context) error {
	defer s.gemini.Close()
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

func portOf(listen string) string {
	if idx := strings.LastIndex(listen, ":"); idx >= 0 {
		return listen[idx+1:]
	}
	return listen
}

// withLogging 访问日志中间件。
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		// 管理台静态资源不打日志
		if !strings.HasPrefix(r.URL.Path, "/admin") {
			s.logger.Printf("[http] %s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.status = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

// Unwrap/Flush 让 http.ResponseController 穿透访问日志包装器，保证 SSE
// 分片能及时发送到客户端，而不是等整个响应结束后才一次性写出。
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// authDisabled 无鉴权模式（库中无 Key 且配置未提供）。
func (s *Server) authDisabled() bool {
	if len(s.apiKeys) > 0 {
		return false
	}
	keys, err := s.st.ListKeys()
	if err != nil {
		return false
	}
	return len(keys) == 0
}

// bearerKey 提取 Authorization Bearer。
func bearerKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

// withAuth 鉴权中间件：动态查 Key 库 + 限流。
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authDisabled() {
			s.logger.Printf("[auth] 警告：当前无任何 API Key，网关处于无鉴权模式")
			next(w, r)
			return
		}
		key := bearerKey(r)
		if key == "" {
			writeError(w, http.StatusUnauthorized, "Invalid API key", "authentication_error", nil)
			return
		}
		info, found, err := s.st.LookupKey(key)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "校验 Key 失败: "+err.Error(), "api_error", nil)
			return
		}
		if !found || !info.Enabled {
			writeError(w, http.StatusUnauthorized, "Invalid API key", "authentication_error", nil)
			return
		}
		now := time.Now()
		if info.Expired(now.Unix()) {
			writeKeyExpired(w, info.ExpiresAt)
			return
		}
		if info.Exhausted() {
			writeQuotaExceeded(w, info)
			return
		}
		if ok, retryAfter := s.rpm.Allow(info.ID, info.RPMLimit, now); !ok {
			writeRPMExceeded(w, info.RPMLimit, retryAfter)
			return
		}
		if !s.limiter.Allow(key) {
			writeError(w, http.StatusTooManyRequests, "Rate limit exceeded, please retry later", "rate_limit_error", nil)
			return
		}
		next(w, withKeyInfo(r, info))
	}
}

// currentKey 当前请求的调用方 Key（无鉴权模式记 anonymous）。
func (s *Server) currentKey(r *http.Request) string {
	if s.authDisabled() {
		return "anonymous"
	}
	if key := bearerKey(r); key != "" {
		return key
	}
	return "anonymous"
}

// writeQuotaExceeded Key 的 Token 额度用尽：HTTP 429 + OpenAI 兼容 insufficient_quota。
func writeQuotaExceeded(w http.ResponseWriter, info store.KeyAuth) {
	w.Header().Set("X-Web2api-Token-Limit", strconv.FormatInt(info.TokenLimit, 10))
	w.Header().Set("X-Web2api-Tokens-Used", strconv.FormatInt(info.TokensUsed, 10))
	writeError(w, http.StatusTooManyRequests, fmt.Sprintf(
		"API key token quota exhausted (Token 额度已用尽): used %d of %d charged tokens. "+
			"Ask the administrator to raise token_limit or reset usage for this key.",
		info.TokensUsed, info.TokenLimit), "insufficient_quota", "token_quota_exceeded")
}

func writeError(w http.ResponseWriter, status int, msg, errType string, code any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(model.ErrorResponse{
		Error: model.ErrorDetail{Message: msg, Type: errType, Code: code},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// handleRoot 根路径提示。
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "web2api",
		"status":  "running",
		"endpoints": []string{
			"GET  /health",
			"GET  /v1/docs",
			"GET  /tools/export-storage.sh",
			"POST /v1/chat/completions",
			"POST /v1/videos",
			"GET  /v1/videos/{id}",
			"GET  /v1/videos/{id}/content",
			"GET  /v1/models",
			"GET  /v1/accounts",
			"GET  /admin        (号池管理台)",
			"GET  /admin/api/docs (管理 API OpenAPI 文档)",
			"多模态透传: /v1/images|videos|audio|files/*（仅 upstream 模式）",
		},
	})
}
