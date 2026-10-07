package upgrade

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type imageRef struct {
	registry   string
	name       string
	repository string
	tag        string
	digest     string
}

func parseImageRef(image string) (imageRef, error) {
	raw := strings.TrimSpace(image)
	if raw == "" {
		return imageRef{}, fmt.Errorf("镜像地址为空")
	}
	ref := imageRef{}
	if at := strings.LastIndex(raw, "@"); at > 0 && strings.Contains(raw[at+1:], ":") {
		ref.digest = raw[at+1:]
		raw = raw[:at]
	}
	slash := strings.LastIndex(raw, "/")
	if colon := strings.LastIndex(raw, ":"); colon > slash {
		ref.tag = raw[colon+1:]
		raw = raw[:colon]
	}
	ref.registry = registryAddress(raw)
	if ref.registry == "docker.io" && !strings.Contains(strings.Split(raw, "/")[0], ".") && !strings.HasPrefix(raw, "localhost") {
		ref.repository = raw
		if !strings.Contains(raw, "/") {
			ref.repository = "library/" + raw
		}
		ref.name = "docker.io/" + ref.repository
	} else {
		ref.name = raw
		ref.repository = strings.TrimPrefix(raw, ref.registry+"/")
	}
	if ref.repository == "" || strings.Contains(ref.repository, " ") {
		return imageRef{}, fmt.Errorf("无法解析镜像地址")
	}
	return ref, nil
}

func imageCreateQuery(image string) string {
	ref, err := parseImageRef(image)
	if err != nil {
		return "fromImage=" + url.QueryEscape(image)
	}
	query := url.Values{}
	query.Set("fromImage", ref.name)
	switch {
	case ref.digest != "":
		query.Set("tag", ref.digest)
	case ref.tag != "":
		query.Set("tag", ref.tag)
	}
	return query.Encode()
}

func encodeRegistryAuth(fields map[string]string) (string, error) {
	credentials, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(credentials), nil
}

func newRegistryHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if raw := strings.TrimSpace(os.Getenv("WEB2API_PROXY")); raw != "" {
		if proxyURL, err := url.Parse(raw); err == nil && proxyURL.Host != "" {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &http.Client{Timeout: 20 * time.Second, Transport: transport}
}

func registryPingURL(registry string) string {
	host := registry
	scheme := "https"
	if host == "docker.io" || host == "index.docker.io" {
		host = "registry-1.docker.io"
	}
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.") {
		scheme = "http"
	}
	return scheme + "://" + host + "/v2/"
}

// pullLatest asks the registry for a bearer token before Docker pulls.
// Docker otherwise posts an OAuth password grant, and GHCR answers that with
// HTTP 403 even for a public image. A rejected credential falls back to an
// anonymous pull.
func (m *Manager) pullLatest(ctx context.Context) error {
	auth := m.pullAuth(ctx)
	err := m.docker.pull(ctx, m.image, auth)
	if err == nil || !registryDenied(err) || auth == "" {
		return err
	}
	fallback := ""
	if anon, ok := m.bearerAuth(ctx, "", ""); ok && anon != auth {
		fallback = anon
	}
	if retryErr := m.docker.pull(ctx, m.image, fallback); retryErr == nil {
		return nil
	}
	if fallback != "" {
		if retryErr := m.docker.pull(ctx, m.image, ""); retryErr == nil {
			return nil
		}
	}
	return err
}

func (m *Manager) pullAuth(ctx context.Context) string {
	if m.registryHTTP == nil {
		return m.registryAuth
	}
	if auth, ok := m.bearerAuth(ctx, m.registryUser, m.registryPassword); ok {
		return auth
	}
	return m.registryAuth
}

func (m *Manager) bearerAuth(ctx context.Context, user, password string) (string, bool) {
	ref, err := parseImageRef(m.image)
	if err != nil {
		return "", false
	}
	token, err := m.fetchBearer(ctx, ref, user, password)
	if err != nil || token == "" {
		return "", false
	}
	encoded, err := encodeRegistryAuth(map[string]string{
		"registrytoken": token,
		"serveraddress": ref.registry,
	})
	if err != nil {
		return "", false
	}
	return encoded, true
}

func (m *Manager) fetchBearer(ctx context.Context, ref imageRef, user, password string) (string, error) {
	token, status, err := m.requestBearer(ctx, ref, user, password)
	if err == nil {
		return token, nil
	}
	if password != "" && (status == http.StatusUnauthorized || status == http.StatusForbidden) {
		token, _, err = m.requestBearer(ctx, ref, "", "")
	}
	return token, err
}

func (m *Manager) requestBearer(ctx context.Context, ref imageRef, user, password string) (string, int, error) {
	if m.registryHTTP == nil {
		return "", 0, fmt.Errorf("未配置仓库客户端")
	}
	ping, err := http.NewRequestWithContext(ctx, http.MethodGet, registryPingURL(ref.registry), nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := m.registryHTTP.Do(ping)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusOK {
		return "", resp.StatusCode, fmt.Errorf("仓库不需要拉取令牌")
	}
	realm, service, ok := bearerChallenge(resp.Header.Get("WWW-Authenticate"))
	if !ok {
		return "", resp.StatusCode, fmt.Errorf("仓库未返回 Bearer 挑战")
	}
	if service == "" {
		service = ref.registry
	}
	query := url.Values{}
	query.Set("service", service)
	query.Set("scope", "repository:"+ref.repository+":pull")
	tokenURL := realm
	if strings.Contains(realm, "?") {
		tokenURL += "&" + query.Encode()
	} else {
		tokenURL += "?" + query.Encode()
	}
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", 0, err
	}
	if user != "" || password != "" {
		tokenReq.SetBasicAuth(user, password)
	}
	tokenResp, err := m.registryHTTP.Do(tokenReq)
	if err != nil {
		return "", 0, err
	}
	defer tokenResp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(tokenResp.Body, 1<<20))
	if tokenResp.StatusCode != http.StatusOK {
		return "", tokenResp.StatusCode, fmt.Errorf("申请拉取令牌失败（HTTP %d）", tokenResp.StatusCode)
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", tokenResp.StatusCode, err
	}
	token := payload.Token
	if token == "" {
		token = payload.AccessToken
	}
	if token == "" {
		return "", tokenResp.StatusCode, fmt.Errorf("仓库没有返回拉取令牌")
	}
	return token, tokenResp.StatusCode, nil
}

func bearerChallenge(header string) (realm, service string, ok bool) {
	if !strings.Contains(strings.ToLower(header), "bearer") {
		return "", "", false
	}
	realm = challengeParam(header, "realm")
	service = challengeParam(header, "service")
	return realm, service, realm != ""
}

func challengeParam(header, key string) string {
	for _, part := range splitChallenge(header) {
		part = strings.TrimSpace(part)
		if len(part) >= 7 && strings.EqualFold(part[:7], "bearer ") {
			part = strings.TrimSpace(part[7:])
		}
		if len(part) < len(key)+1 || !strings.EqualFold(part[:len(key)], key) {
			continue
		}
		rest := strings.TrimSpace(part[len(key):])
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		rest = strings.TrimSpace(rest[1:])
		if strings.HasPrefix(rest, `"`) {
			rest = rest[1:]
			if end := strings.IndexByte(rest, '"'); end >= 0 {
				return rest[:end]
			}
			return ""
		}
		if end := strings.IndexAny(rest, ", "); end >= 0 {
			return rest[:end]
		}
		return rest
	}
	return ""
}

func splitChallenge(header string) []string {
	var parts []string
	var current strings.Builder
	quoted := false
	for _, char := range header {
		switch char {
		case '"':
			quoted = !quoted
			current.WriteRune(char)
		case ',':
			if quoted {
				current.WriteRune(char)
			} else if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func registryDenied(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "403") || strings.Contains(message, "401") || strings.Contains(message, "denied") || strings.Contains(message, "unauthorized")
}
