//go:build web2api_unit

package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGHCR 模拟 GHCR 实测行为：public=true 时匿名可读；带 Basic 凭据一律 403 DENIED。
func fakeGHCR(t *testing.T, public bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/":
			w.Header().Set("WWW-Authenticate", `Bearer realm="https://ghcr.io/token",service="ghcr.io"`)
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/token":
			if r.Header.Get("Authorization") != "" || !public {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"errors":[{"code":"DENIED","message":"denied"}]}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "anon"})
		case strings.HasPrefix(r.URL.Path, "/v2/plunjoin/web2api/manifests/"):
			if r.Header.Get("Authorization") != "Bearer anon" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

var deniedPull = errors.New("拉取镜像失败（HTTP 403）: denied: requested access to the resource is denied")

func TestExplainPullErrorPublicImageBlamesDaemonAndRejectedCredentials(t *testing.T) {
	registry := fakeGHCR(t, true)
	m := &Manager{image: "ghcr.io/plunjoin/web2api:latest", registryUser: "someone", registryPassword: "expired",
		registryHTTP: &http.Client{Transport: rewriteHostTransport{base: registry.URL}}}
	msg := m.explainPullError(context.Background(), deniedPull).Error()
	for _, want := range []string{"denied", "是公开镜像", "不需要任何仓库凭据", "被仓库拒绝", "建议清空这两个变量", "docker pull ghcr.io/plunjoin/web2api:latest"} {
		if !strings.Contains(msg, want) {
			t.Errorf("诊断缺少 %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "expired") {
		t.Fatal("诊断信息不得包含凭据内容")
	}
}

func TestExplainPullErrorPrivateOrWrongNameNamesCredentialSettings(t *testing.T) {
	registry := fakeGHCR(t, false)
	m := &Manager{image: "ghcr.io/plunjoin/web2api:latest",
		registryHTTP: &http.Client{Transport: rewriteHostTransport{base: registry.URL}}}
	msg := m.explainPullError(context.Background(), deniedPull).Error()
	for _, want := range []string{"无法匿名读取", "HTTP 403", "镜像名错误或为私有包", "WEB2API_UPGRADE_IMAGE", "WEB2API_UPGRADE_REGISTRY_USER=<GitHub 用户名>", "WEB2API_UPGRADE_REGISTRY_PASSWORD", "read:packages", "classic"} {
		if !strings.Contains(msg, want) {
			t.Errorf("诊断缺少 %q:\n%s", want, msg)
		}
	}
	// 只填用户名：明确指出密码为空
	m.registryUser = "someone"
	if msg = m.explainPullError(context.Background(), deniedPull).Error(); !strings.Contains(msg, "WEB2API_UPGRADE_REGISTRY_PASSWORD 为空") {
		t.Fatalf("应提示密码为空:\n%s", msg)
	}
}

func TestExplainPullErrorUnreachableRegistrySuggestsProxy(t *testing.T) {
	registry := httptest.NewServer(http.NotFoundHandler())
	base := registry.URL
	registry.Close() // 连接被拒
	m := &Manager{image: "ghcr.io/plunjoin/web2api:latest",
		registryHTTP: &http.Client{Transport: rewriteHostTransport{base: base}}}
	msg := m.explainPullError(context.Background(), deniedPull).Error()
	if !strings.Contains(msg, "无法访问 ghcr.io") || !strings.Contains(msg, "WEB2API_PROXY") {
		t.Fatalf("应提示代理设置:\n%s", msg)
	}
}

func TestExplainPullErrorKeepsOtherErrors(t *testing.T) {
	m := &Manager{image: "ghcr.io/plunjoin/web2api:latest"}
	other := errors.New("拉取镜像失败: no space left on device")
	if got := m.explainPullError(context.Background(), other); got != other {
		t.Fatalf("非 403 错误应原样返回: %v", got)
	}
	if m.explainPullError(context.Background(), nil) != nil {
		t.Fatal("nil 应原样返回")
	}
}
