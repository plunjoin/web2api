// Package geminiapi provides a lossless gateway to the official Gemini API.
// Google owns model validation, managed agents, state, callbacks and SSE event
// schemas; the gateway replaces authentication without narrowing their payloads.
package geminiapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"web2api/internal/config"
	"web2api/internal/store"
)

type Backend struct {
	base        *url.URL
	key         string
	accessToken string
	enabled     bool
	transport   http.RoundTripper
	timeout     time.Duration
	store       *store.Store
}

func New(cfg config.GeminiAPIConfig, st *store.Store) (*Backend, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com"
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") || base.RawQuery != "" || base.Fragment != "" || base.User != nil {
		return nil, fmt.Errorf("gemini_api.base_url 必须是 http(s) API 根地址")
	}
	base.Path = strings.TrimRight(base.Path, "/")
	if strings.HasSuffix(base.Path, "/v1") || strings.HasSuffix(base.Path, "/v1beta") {
		return nil, fmt.Errorf("gemini_api.base_url 不应包含 /v1 或 /v1beta")
	}
	if cfg.Enabled && strings.TrimSpace(cfg.APIKey) == "" && strings.TrimSpace(cfg.AccessToken) == "" {
		return nil, fmt.Errorf("启用 gemini_api 需要 api_key 或 OAuth access_token")
	}
	if cfg.TimeoutSeconds < 0 {
		return nil, fmt.Errorf("gemini_api.timeout_seconds 不能为负数")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	transport.ResponseHeaderTimeout = 5 * time.Minute
	if cfg.Proxy != "" {
		proxy, err := url.Parse(cfg.Proxy)
		if err != nil || proxy.Host == "" {
			return nil, fmt.Errorf("invalid gemini_api.proxy")
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	return &Backend{base: base, key: cfg.APIKey, accessToken: cfg.AccessToken, enabled: cfg.Enabled, transport: transport,
		timeout: time.Duration(cfg.TimeoutSeconds) * time.Second, store: st}, nil
}

func (b *Backend) Enabled() bool { return b != nil && b.enabled }
func (b *Backend) Close() {
	if b != nil {
		if t, ok := b.transport.(*http.Transport); ok {
			t.CloseIdleConnections()
		}
	}
}

// AuthKey accepts SDK-style authentication as well as the gateway's bearer key.
// The value is always a gateway key; it is never used as Google's credential.
func AuthKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		parts := strings.Fields(auth)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
		return ""
	}
	if key := r.Header.Get("X-Goog-Api-Key"); key != "" {
		return key
	}
	return r.URL.Query().Get("key")
}

func failure(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": status, "message": message}})
}

func (b *Backend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !b.Enabled() {
		failure(w, http.StatusServiceUnavailable, "官方 Gemini 后端未启用，请配置 gemini_api 或 WEB2API_GEMINI_API_KEY")
		return
	}
	if b.timeout > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), b.timeout)
		defer cancel()
		r = r.WithContext(ctx)
	}
	var uploadTarget *url.URL
	if strings.HasPrefix(r.URL.Path, "/gemini-upload/") {
		if b.store == nil {
			failure(w, 503, "upload session storage unavailable")
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/gemini-upload/")
		saved, err := b.store.GeminiUploadURL(id)
		if err != nil {
			failure(w, http.StatusNotFound, "upload session not found or expired")
			return
		}
		uploadTarget, err = url.Parse(saved)
		if err != nil || !b.sameOrigin(uploadTarget) {
			failure(w, 502, "invalid upstream upload URL")
			return
		}
	}
	proxy := &httputil.ReverseProxy{
		Transport:     b.transport,
		FlushInterval: -1,
		Rewrite: func(p *httputil.ProxyRequest) {
			if strings.HasPrefix(p.In.URL.Path, "/gemini/") {
				p.Out.URL.Path = strings.TrimPrefix(p.Out.URL.Path, "/gemini")
				if p.Out.URL.RawPath != "" {
					p.Out.URL.RawPath = strings.TrimPrefix(p.Out.URL.RawPath, "/gemini")
				}
			}
			p.SetURL(b.base)
			if uploadTarget != nil {
				target := *uploadTarget
				p.Out.URL = &target
				p.Out.Host = target.Host
			}
			query := p.Out.URL.Query()
			query.Del("key")
			p.Out.URL.RawQuery = query.Encode()
			p.Out.Header.Del("Authorization")
			p.Out.Header.Del("Cookie")
			p.Out.Header.Del("X-Goog-Api-Key")
			if b.key != "" {
				p.Out.Header.Set("X-Goog-Api-Key", b.key)
			}
			if b.accessToken != "" {
				p.Out.Header.Set("Authorization", "Bearer "+b.accessToken)
			}
		},
		ModifyResponse: func(response *http.Response) error {
			if uploadURL := response.Header.Get("X-Goog-Upload-URL"); uploadURL != "" {
				target, err := url.Parse(uploadURL)
				if err != nil {
					return fmt.Errorf("invalid upstream upload URL")
				}
				target = response.Request.URL.ResolveReference(target)
				if !b.sameOrigin(target) {
					return fmt.Errorf("upstream upload URL has a different origin")
				}
				if b.store == nil {
					return fmt.Errorf("upload session storage unavailable")
				}
				random := make([]byte, 24)
				if _, err := rand.Read(random); err != nil {
					return err
				}
				id := hex.EncodeToString(random)
				if err := b.store.SaveGeminiUploadURL(id, target.String(), time.Now().Add(48*time.Hour)); err != nil {
					return err
				}
				scheme := "http"
				if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
					scheme = "https"
				}
				local := url.URL{Scheme: scheme, Host: r.Host, Path: "/gemini-upload/" + id}
				response.Header.Set("X-Goog-Upload-URL", local.String())
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// Do not expose URLs or headers that could contain API/upload secrets.
			failure(w, http.StatusBadGateway, "Gemini upstream request failed")
		},
	}
	proxy.ServeHTTP(w, r)
}

func (b *Backend) sameOrigin(u *url.URL) bool {
	return u != nil && u.User == nil && u.Scheme == b.base.Scheme && strings.EqualFold(u.Host, b.base.Host)
}
