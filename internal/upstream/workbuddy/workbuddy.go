// Package workbuddy 实现腾讯 CodeBuddy/WorkBuddy 引擎的协议客户端：
// www.codebuddy.ai 的 OAuth 设备授权流程 + /v2/chat/completions
// （OpenAI Chat Completions 协议透传，SSE 流式）。
package workbuddy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	xproxy "golang.org/x/net/proxy"
)

// DefaultBaseURL 上游默认地址。
const DefaultBaseURL = "https://www.codebuddy.ai"

// CLI 客户端伪装常量。
const (
	cliVersion   = "1.0.7"
	cliUserAgent = "CLI/" + cliVersion + " CodeBuddy/" + cliVersion
	authUA       = "CLI/1.0.8 CodeBuddy/1.0.8"

	pendingLoginCode = 11217 // 轮询返回：等待用户在浏览器完成登录
	channelErrorCode = 11128 // 上游渠道检测错误码（请求中含"污染"内容触发）
)

// DefaultModels 默认模型目录（以账号实际权益为准，model 原样透传上游）。
var DefaultModels = []string{
	"auto-chat", "claude-4.0", "claude-3.7", "gpt-5", "gpt-5-mini",
	"gemini-2.5-pro", "gemini-2.5-flash",
}

// systemKeyReplacements system 消息竞品关键词替换（上游风控检测）。
var systemKeyReplacements = []struct{ from, to string }{
	{"Claude Code", "CodeBuddy Code"},
	{"Anthropic's official CLI for Claude", "Tencent's official CLI for CodeBuddy"},
	{"Anthropic", "Tencent"},
	{"Claude", "CodeBuddy"},
}

// ==================== OAuth（设备授权式） ====================

// AuthStart 发起授权返回的会话信息。
type AuthStart struct {
	State  string
	AuthURL string
}

// StartAuth 发起登录授权：返回 state 与用户浏览器打开的 authUrl。
func StartAuth(ctx context.Context, proxy, baseURL string) (*AuthStart, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	nonce := hex.EncodeToString(randomBytes(16))
	payload, _ := json.Marshal(map[string]string{"nonce": nonce})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/v2/plugin/auth/state?platform=CLI&nonce="+nonce, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setAuthHeaders(req)
	resp, err := doRequest(ctx, proxy, req, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("发起授权失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("发起授权 HTTP %d: %s", resp.StatusCode, snippet(body))
	}
	var out struct {
		Code int `json:"code"`
		Data struct {
			State   string `json:"state"`
			AuthURL string `json:"authUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("授权响应解析失败: %w", err)
	}
	if out.Data.State == "" {
		return nil, fmt.Errorf("授权响应缺少 state: %s", snippet(body))
	}
	return &AuthStart{State: out.Data.State, AuthURL: out.Data.AuthURL}, nil
}

// Token 授权结果。
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Email        string
}

// ErrPendingLogin 轮询时用户尚未完成登录。
var ErrPendingLogin = errors.New("等待用户在浏览器完成登录")

// PollToken 轮询授权结果（用户完成登录前返回 ErrPendingLogin）。
func PollToken(ctx context.Context, proxy, baseURL, state string) (*Token, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/v2/plugin/auth/token?state="+url.QueryEscape(state), nil)
	if err != nil {
		return nil, err
	}
	setAuthHeaders(req)
	traceID := hex.EncodeToString(randomBytes(8))
	req.Header.Set("b3", traceID+"-"+hex.EncodeToString(randomBytes(8))+"-1-")
	req.Header.Set("X-B3-TraceId", traceID)
	req.Header.Set("X-B3-SpanId", hex.EncodeToString(randomBytes(8)))
	req.Header.Set("X-B3-Sampled", "1")
	resp, err := doRequest(ctx, proxy, req, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("轮询授权失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("轮询授权 HTTP %d: %s", resp.StatusCode, snippet(body))
	}
	var out struct {
		Code int `json:"code"`
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresIn     int64  `json:"expiresIn"`
			Domain        string `json:"domain"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("轮询响应解析失败: %w", err)
	}
	if out.Code == pendingLoginCode {
		return nil, ErrPendingLogin
	}
	if out.Code != 0 || out.Data.AccessToken == "" {
		return nil, fmt.Errorf("授权未完成（code=%d）: %s", out.Code, strings.TrimSpace(out.Message))
	}
	tok := &Token{
		AccessToken:  out.Data.AccessToken,
		RefreshToken: out.Data.RefreshToken,
	}
	if out.Data.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(out.Data.ExpiresIn-300) * time.Second)
	}
	return tok, nil
}

// JWTEmail 解析 accessToken（JWT）中的 email。
func JWTEmail(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64Decode(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email    string `json:"email"`
		Username string `json:"preferred_username"`
		Sub      string `json:"sub"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	if claims.Email != "" {
		return claims.Email
	}
	return claims.Username
}

// ==================== 推理 ====================

// Message OpenAI 风格消息。
type Message struct {
	Role    string
	Text    string
}

// Request 推理请求。
type Request struct {
	Model       string
	Messages     []Message
	MaxTokens    int
	Temperature  *float64
}

// Result 推理结果。
type Result struct {
	Text             string
	PromptTokens     int64
	CompletionTokens int64
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatBody struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

func buildChatBody(req *Request) (*chatBody, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("消息为空")
	}
	body := &chatBody{Model: req.Model, Stream: true, Temperature: req.Temperature}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	}
	for _, msg := range req.Messages {
		text := msg.Text
		role := strings.ToLower(msg.Role)
		// 风控规避：过滤含错误标记的 assistant 消息（触发上游 11128 渠道检测）
		if role == "assistant" && (strings.Contains(text, "Error: API error") || strings.Contains(text, "API error:")) {
			continue
		}
		if role == "system" {
			for _, r := range systemKeyReplacements {
				text = strings.ReplaceAll(text, r.from, r.to)
			}
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		body.Messages = append(body.Messages, chatMessage{Role: role, Content: text})
	}
	// 上游要求至少 2 条消息：单条 user 前补 system
	if len(body.Messages) == 1 {
		body.Messages = append([]chatMessage{{Role: "system", Content: "You are a helpful assistant."}}, body.Messages...)
	}
	if len(body.Messages) == 0 {
		return nil, errors.New("没有可发送的消息")
	}
	return body, nil
}

// Stream 发起聊天补全（上游强制流式）并回调增量文本。
func Stream(ctx context.Context, proxy, baseURL, accessToken, userID string, req *Request, onDelta func(string) error) (*Result, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	body, err := buildChatBody(req)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/v2/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setChatHeaders(httpReq, accessToken, userID)
	resp, err := doRequest(ctx, proxy, httpReq, 0)
	if err != nil {
		return nil, fmt.Errorf("codebuddy 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, &UpstreamError{Status: resp.StatusCode, Body: string(b)}
	}
	return parseSSE(resp.Body, onDelta)
}

// UpstreamError 上游非 200 响应。
type UpstreamError struct {
	Status int
	Body   string
}

func (e *UpstreamError) Error() string {
	msg := e.Body
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(e.Body), &m) == nil && m.Message != "" {
		msg = m.Message
	}
	return fmt.Sprintf("上游 HTTP %d: %s", e.Status, strings.TrimSpace(msg))
}

// IsUnauthorized 判断 token 失效。
func IsUnauthorized(err error) bool {
	var ue *UpstreamError
	if errors.As(err, &ue) {
		return ue.Status == http.StatusUnauthorized || ue.Status == http.StatusForbidden
	}
	return false
}

// IsRateLimited 判断上游限流。
func IsRateLimited(err error) bool {
	var ue *UpstreamError
	if errors.As(err, &ue) {
		return ue.Status == http.StatusTooManyRequests
	}
	return false
}

func parseSSE(r io.Reader, onDelta func(string) error) (*Result, error) {
	result := &Result{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				result.Text += ch.Delta.Content
				if onDelta != nil {
					if err := onDelta(ch.Delta.Content); err != nil {
						return nil, err
					}
				}
			}
		}
		if chunk.Usage != nil {
			result.PromptTokens = chunk.Usage.PromptTokens
			result.CompletionTokens = chunk.Usage.CompletionTokens
		}
	}
	if err := scanner.Err(); err != nil {
		if result.Text != "" {
			return result, nil
		}
		return nil, err
	}
	if result.Text == "" {
		return nil, errors.New("上游没有返回任何内容")
	}
	return result, nil
}

// ==================== 公共 ====================

func setAuthHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", authUA)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("X-No-Authorization", "true")
	req.Header.Set("X-No-User-Id", "true")
	req.Header.Set("X-No-Enterprise-Id", "true")
	req.Header.Set("X-No-Department-Info", "true")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("X-Request-ID", strings.ReplaceAll(uuid.NewString(), "-", ""))
}

func setChatHeaders(req *http.Request, accessToken, userID string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", cliUserAgent)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("X-Domain", "www.codebuddy.ai")
	req.Header.Set("X-Product", "SaaS")
	req.Header.Set("X-Agent-Intent", "craft")
	req.Header.Set("X-IDE-Type", "CLI")
	req.Header.Set("X-IDE-Name", "CLI")
	req.Header.Set("X-IDE-Version", cliVersion)
	if userID == "" {
		userID = "b5be3a67-237e-4ee6-9b9a-0b9ecd7b454b"
	}
	req.Header.Set("X-User-Id", userID)
	req.Header.Set("X-Conversation-ID", uuid.NewString())
	req.Header.Set("X-Conversation-Request-ID", hex.EncodeToString(randomBytes(16)))
	req.Header.Set("X-Conversation-Message-ID", strings.ReplaceAll(uuid.NewString(), "-", ""))
	req.Header.Set("X-Request-ID", strings.ReplaceAll(uuid.NewString(), "-", ""))
	req.Header.Set("x-stainless-arch", "x64")
	req.Header.Set("x-stainless-lang", "js")
	req.Header.Set("x-stainless-os", "Windows")
	req.Header.Set("x-stainless-package-version", "5.10.1")
	req.Header.Set("x-stainless-retry-count", "0")
	req.Header.Set("x-stainless-runtime", "node")
	req.Header.Set("x-stainless-runtime-version", "v22.13.1")
}

func doRequest(ctx context.Context, proxy string, req *http.Request, timeout time.Duration) (*http.Response, error) {
	client, err := newHTTPClient(proxy, timeout)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

// newHTTPClient 支持 http(s)/socks5 代理。
func newHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        8,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
	}
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("解析代理地址失败: %w", err)
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			transport.Proxy = http.ProxyURL(u)
		case "socks5", "socks5h":
			dialer, err := xproxy.FromURL(u, xproxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("创建 SOCKS5 代理失败: %w", err)
			}
			if cd, ok := dialer.(xproxy.ContextDialer); ok {
				transport.DialContext = cd.DialContext
			}
		default:
			return nil, fmt.Errorf("不支持的代理协议: %s", u.Scheme)
		}
	}
	client := &http.Client{Transport: transport}
	if timeout > 0 {
		client.Timeout = timeout
	}
	return client, nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func base64Decode(s string) ([]byte, error) {
	s = strings.TrimRight(s, "=")
	if len(s)%4 == 2 {
		s += "=="
	} else if len(s)%4 == 3 {
		s += "="
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	var out []byte
	var buf, bits int
	for _, c := range s {
		idx := strings.IndexRune(alphabet, c)
		if idx < 0 {
			return nil, errors.New("非法 base64 字符")
		}
		buf = buf<<6 | idx
		bits += 6
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(buf>>bits))
		}
	}
	return out, nil
}

func snippet(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
