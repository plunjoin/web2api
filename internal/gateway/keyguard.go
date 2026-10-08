package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"web2api/internal/store"
)

// keyInfoCtx 在请求上下文中保存已通过鉴权的 Key 信息（无鉴权模式下不存在）。
type keyInfoCtx struct{}

func withKeyInfo(r *http.Request, info store.KeyAuth) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), keyInfoCtx{}, info))
}

func keyInfoFrom(r *http.Request) (store.KeyAuth, bool) {
	info, ok := r.Context().Value(keyInfoCtx{}).(store.KeyAuth)
	return info, ok
}

// rpmLimiter 每个 Key 的固定窗口（自然分钟）请求计数。
type rpmLimiter struct {
	mu      sync.Mutex
	windows map[int64]*rpmWindow
}

type rpmWindow struct{ minute, count int64 }

func newRPMLimiter() *rpmLimiter { return &rpmLimiter{windows: map[int64]*rpmWindow{}} }

// Allow 返回是否放行，以及被拒时距下一个窗口的秒数。
func (l *rpmLimiter) Allow(keyID, limit int64, now time.Time) (bool, int64) {
	if limit <= 0 {
		return true, 0
	}
	minute := now.Unix() / 60
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.windows[keyID]
	if w == nil || w.minute != minute {
		w = &rpmWindow{minute: minute}
		l.windows[keyID] = w
	}
	if w.count >= limit {
		return false, (minute+1)*60 - now.Unix()
	}
	w.count++
	return true, 0
}

func writeRPMExceeded(w http.ResponseWriter, limit, retryAfter int64) {
	w.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
	writeError(w, http.StatusTooManyRequests, fmt.Sprintf(
		"API key rate limit exceeded (每分钟请求数超限): limit %d requests per minute, retry after %ds.", limit, retryAfter),
		"rate_limit_error", "rpm_limit_exceeded")
}

func writeKeyExpired(w http.ResponseWriter, expiresAt int64) {
	writeError(w, http.StatusUnauthorized, fmt.Sprintf(
		"API key expired (Key 已过期) at %s. Ask the administrator to extend expires_at.",
		time.Unix(expiresAt, 0).Format(time.RFC3339)), "authentication_error", "key_expired")
}

// modelAllowed 检查当前 Key 的模型白名单；不允许时写出 403 并返回 false。
func (s *Server) modelAllowed(w http.ResponseWriter, r *http.Request, model string) bool {
	info, ok := keyInfoFrom(r)
	if !ok || info.ModelAllowed(model) {
		return true
	}
	writeError(w, http.StatusForbidden, fmt.Sprintf(
		"Model %q is not allowed for this API key (该 Key 不允许使用此模型). Allowed: %s",
		model, strings.Join(info.AllowedModels, ", ")), "permission_error", "model_not_allowed")
	return false
}

// withModelGuard 用于模型不在固定字段里的路由（Gemini 原生、多模态透传）：
// 从路径 models/{model} 与 JSON 请求体的 "model" 字段提取模型并校验白名单。
// 只有设置了白名单的 Key 才会读取请求体。
func (s *Server) withModelGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, ok := keyInfoFrom(r)
		if !ok || len(info.AllowedModels) == 0 {
			next(w, r)
			return
		}
		models := modelsInPath(r.URL.Path)
		if r.Body != nil && strings.Contains(r.Header.Get("Content-Type"), "json") {
			body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
			if err != nil {
				writeError(w, http.StatusBadRequest, "读取请求体失败", "invalid_request_error", nil)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var probe struct {
				Model string `json:"model"`
			}
			if json.Unmarshal(body, &probe) == nil && probe.Model != "" {
				models = append(models, strings.TrimPrefix(probe.Model, "models/"))
			}
		}
		for _, model := range models {
			if !s.modelAllowed(w, r, model) {
				return
			}
		}
		next(w, r)
	}
}

// modelsInPath 提取 .../models/{model}[:method] 中的模型名。
func modelsInPath(path string) []string {
	var out []string
	parts := strings.Split(path, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "models" && parts[i+1] != "" {
			model := parts[i+1]
			if colon := strings.IndexByte(model, ':'); colon >= 0 {
				model = model[:colon]
			}
			if model != "" {
				out = append(out, model)
			}
		}
	}
	return out
}

// userUnmeteredAllowed 用户 Key 调用不计 Token 的接口（Gemini 原生、多模态透传）需管理员显式开放。
func (s *Server) withUserMeteringGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if info, ok := keyInfoFrom(r); ok && info.UserID > 0 {
			settings, err := s.st.GetPlatformSettings()
			if err != nil || !settings.UserUnmeteredRoutes {
				writeError(w, http.StatusForbidden,
					"This endpoint is not metered and is not enabled for user API keys (该接口不计费，未对用户 Key 开放). Use /v1/chat/completions.",
					"permission_error", "endpoint_not_allowed")
				return
			}
		}
		next(w, r)
	}
}
