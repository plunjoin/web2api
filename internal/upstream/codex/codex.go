// Package codex 实现 OpenAI Codex（ChatGPT 订阅号）引擎的协议客户端：
// OAuth 授权/刷新（auth.openai.com，PKCE）+ chatgpt.com/backend-api/codex/responses
// 推理端点（OpenAI Responses 协议，SSE 流式）。
package codex

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// OAuth 与上游常量。
const (
	ClientID      = "app_EMoamEEZ73f0CkXaXp7hrann" // Codex CLI 官方 client_id
	AuthorizeURL  = "https://auth.openai.com/oauth/authorize"
	TokenURL      = "https://auth.openai.com/oauth/token"
	RedirectURI   = "http://localhost:1455/auth/callback"
	AuthScopes    = "openid profile email offline_access"
	RefreshScopes = "openid profile email"

	ResponsesURL = "https://chatgpt.com/backend-api/codex/responses"

	Originator = "codex-tui"
	CLIVersion = "0.146.0" // 上游最低要求 0.144.0，跟随官方发布更新
	UserAgent  = "codex-tui/" + CLIVersion + " (Ubuntu 22.4.0; x86_64) xterm-256color"
)

// DefaultInstructions 客户端未提供 system 消息时注入的 Codex 身份提示。
const DefaultInstructions = "You are Codex, a coding agent based on GPT-5, running as a CLI tool. " +
	"Follow the user's instructions carefully and concisely, using Markdown for code."

// ModelAliases 请求模型到上游 slug 的映射（未命中时原样透传）。
var ModelAliases = map[string]string{
	"codex-mini-latest": "gpt-5.3-codex",
	"gpt-5-codex":       "gpt-5.3-codex",
}

// DefaultModels 引擎默认模型目录（以上游实际支持为准）。
var DefaultModels = []string{
	"gpt-5.6", "gpt-5.6-codex", "gpt-6", "gpt-6-astra",
	"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex",
	"gpt-5.3-codex-spark", "gpt-5.2",
}

// ==================== OAuth ====================

// Token OAuth 令牌集合。
type Token struct {
	AccessToken  string
	IDToken      string
	RefreshToken string
	ExpiresAt    time.Time
	ChatGPTAccount string // ID token 中的 chatgpt_account_id
	Email        string    // ID token 中的 email
	PlanType     string    // ID token 中的 chatgpt_plan_type
}

// OAuthSession 一次授权会话（state + PKCE verifier）。
type OAuthSession struct {
	SessionID    string
	State        string
	CodeVerifier string
	AuthURL      string
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewOAuthSession 生成授权链接会话。用户浏览器打开 AuthURL 登录后，
// 将跳转 URL（含 code）或 code 本身回填给 ExchangeCode。
func NewOAuthSession() *OAuthSession {
	verifier := randomHex(64) // OpenAI PKCE：verifier 为 hex 而非 base64url
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := randomHex(32)
	sess := &OAuthSession{
		SessionID:    randomHex(16),
		State:        state,
		CodeVerifier: verifier,
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", ClientID)
	q.Set("redirect_uri", RedirectURI)
	q.Set("scope", AuthScopes)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	sess.AuthURL = AuthorizeURL + "?" + q.Encode()
	return sess
}

// ExtractCode 从回调 URL 或裸 code 中提取授权码。
func ExtractCode(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "code=") {
		return raw
	}
	if u, err := url.Parse(raw); err == nil {
		if code := u.Query().Get("code"); code != "" {
			return code
		}
	}
	if idx := strings.Index(raw, "code="); idx >= 0 {
		code := raw[idx+len("code="):]
		if amp := strings.IndexAny(code, "&?#"); amp >= 0 {
			code = code[:amp]
		}
		return code
	}
	return raw
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// ExchangeCode 用授权码换 token。
func ExchangeCode(ctx context.Context, proxy, code, verifier string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", RedirectURI)
	form.Set("code_verifier", verifier)
	return postToken(ctx, proxy, form)
}

// Refresh 用 refresh_token 换新 access_token。
func Refresh(ctx context.Context, proxy, refreshToken string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", ClientID)
	form.Set("scope", RefreshScopes)
	return postToken(ctx, proxy, form)
}

func postToken(ctx context.Context, proxy string, form url.Values) (*Token, error) {
	client, err := NewProxyHTTPClient(proxy, 30*time.Second)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("originator", Originator)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth.openai.com 请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("token 响应解析失败（HTTP %d）: %s", resp.StatusCode, snippet(body))
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("OAuth 错误: %s（%s）", tr.Error, tr.ErrorDesc)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("token 响应缺少 access_token（HTTP %d）: %s", resp.StatusCode, snippet(body))
	}
	tok := &Token{
		AccessToken:  tr.AccessToken,
		IDToken:      tr.IDToken,
		RefreshToken: tr.RefreshToken,
	}
	if tr.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	if claims, err := DecodeIDToken(tr.IDToken); err == nil {
		tok.ChatGPTAccount = claims.ChatGPTAccountID
		tok.Email = claims.Email
		tok.PlanType = claims.PlanType
	}
	if tok.RefreshToken == "" && form.Get("refresh_token") != "" {
		tok.RefreshToken = form.Get("refresh_token") // 刷新响应无新 RT 时保留旧值
	}
	return tok, nil
}

// IDTokenClaims ID token 的关键 claims。
type IDTokenClaims struct {
	Email            string `json:"email"`
	ChatGPTAccountID string `json:"chatgpt_account_id"`
	ChatGPTUserID    string `json:"chatgpt_user_id"`
	PlanType         string `json:"chatgpt_plan_type"`
}

// DecodeIDToken 解析 ID token 的 payload（不验签；token 来自 auth.openai.com 直连）。
func DecodeIDToken(idToken string) (*IDTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("id_token 格式无效")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Auth IDTokenClaims `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		return nil, err
	}
	return &wrapper.Auth, nil
}

// ==================== 推理（Responses 协议） ====================

// Message 输入消息（role: system/user/assistant；ImageURL 为 data URL 或 http URL）。
type Message struct {
	Role      string
	Text      string
	ImageURLs []string
}

// Request Responses 推理请求。
type Request struct {
	Model        string
	Instructions string
	Messages     []Message
	MaxTokens    int
}

// Result 推理结果与用量。
type Result struct {
	Text             string
	PromptTokens     int64
	CompletionTokens int64
	ReasoningTokens  int64
	// PrimaryUsedPercent 上游响应头 x-codex-primary-used-percent（5 小时窗用量百分比）。
	PrimaryUsedPercent float64
	PrimaryResetAfter int64 // x-codex-primary-reset-after-seconds，0 表示未返回
}

type responsesItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content []responsesPart `json:"content"`
}

type responsesPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type responsesBody struct {
	Store           bool            `json:"store"`
	Stream          bool            `json:"stream"`
	Model           string          `json:"model"`
	Instructions    string          `json:"instructions,omitempty"`
	Input           []responsesItem `json:"input"`
	Reasoning       *reasoning      `json:"reasoning,omitempty"`
	Include         []string        `json:"include,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
}

type reasoning struct {
	Effort  string `json:"effort"`
	Summary string `json:"summary"`
}

// MapModel 请求模型 → 上游 slug。
func MapModel(model string) string {
	if mapped, ok := ModelAliases[model]; ok {
		return mapped
	}
	return model
}

func buildResponsesBody(req *Request) (*responsesBody, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("消息为空")
	}
	body := &responsesBody{
		Store:     false,
		Stream:    true,
		Model:     MapModel(req.Model),
		Include:   []string{"reasoning.encrypted_content"},
		Reasoning: &reasoning{Effort: "medium", Summary: "auto"},
	}
	if req.MaxTokens > 0 {
		body.MaxOutputTokens = req.MaxTokens
	}
	for _, msg := range req.Messages {
		switch strings.ToLower(msg.Role) {
		case "system":
			if body.Instructions == "" {
				body.Instructions = msg.Text
			} else {
				body.Instructions += "\n\n" + msg.Text
			}
		case "user", "assistant":
			item := responsesItem{Type: "message", Role: strings.ToLower(msg.Role)}
			partType := "input_text"
			if item.Role == "assistant" {
				partType = "output_text"
			}
			if strings.TrimSpace(msg.Text) != "" {
				item.Content = append(item.Content, responsesPart{Type: partType, Text: msg.Text})
			}
			for _, img := range msg.ImageURLs {
				if strings.TrimSpace(img) == "" {
					continue
				}
				item.Content = append(item.Content, responsesPart{Type: "input_image", ImageURL: img})
			}
			if len(item.Content) == 0 {
				continue
			}
			body.Input = append(body.Input, item)
		}
	}
	if len(body.Input) == 0 {
		return nil, errors.New("没有可发送的 user/assistant 消息")
	}
	if strings.TrimSpace(body.Instructions) == "" {
		body.Instructions = DefaultInstructions
	}
	return body, nil
}

// Stream 发起 Responses 推理并流式回调增量文本。
// chatGPTAccountID 为 ID token 中的 chatgpt_account_id（可能为空）。
func Stream(ctx context.Context, proxy, accessToken, chatGPTAccountID string, req *Request, onDelta func(string) error) (*Result, error) {
	body, err := buildResponsesBody(req)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	client, err := newHTTPClient(proxy, 0)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, ResponsesURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Host = "chatgpt.com"
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	if chatGPTAccountID != "" {
		httpReq.Header.Set("chatgpt-account-id", chatGPTAccountID)
	}
	httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
	httpReq.Header.Set("originator", Originator)
	httpReq.Header.Set("version", CLIVersion)
	httpReq.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("chatgpt.com 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, &UpstreamError{Status: resp.StatusCode, Body: string(b)}
	}
	result, err := parseResponsesSSE(resp.Body, onDelta)
	if err == nil && result != nil {
		// 用量百分比快照（5 小时窗口），账号余额展示用
		if v := resp.Header.Get("x-codex-primary-used-percent"); v != "" {
			if f, perr := strconv.ParseFloat(v, 64); perr == nil {
				result.PrimaryUsedPercent = f
			}
		}
		if v := resp.Header.Get("x-codex-primary-reset-after-seconds"); v != "" {
			if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
				result.PrimaryResetAfter = n
			}
		}
	}
	return result, err
}

// UpstreamError 上游非 200 响应。
type UpstreamError struct {
	Status int
	Body   string
}

func (e *UpstreamError) Error() string {
	msg := e.Body
	var m struct {
		Detail string `json:"detail"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(e.Body), &m); err == nil {
		if m.Detail != "" {
			msg = m.Detail
		} else if m.Error != "" {
			msg = m.Error
		}
	}
	return fmt.Sprintf("上游 HTTP %d: %s", e.Status, strings.TrimSpace(msg))
}

// IsUnauthorized 判断是否需要刷新 token 重试。
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

// parseResponsesSSE 逐行解析 Responses SSE 事件流。
func parseResponsesSSE(r io.Reader, onDelta func(string) error) (*Result, error) {
	result := &Result{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			continue
		}
		var base struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw, &base)
		switch base.Type {
		case "response.output_text.delta":
			var ev struct {
				Delta string `json:"delta"`
			}
			if json.Unmarshal(raw, &ev) == nil && ev.Delta != "" {
				result.Text += ev.Delta
				if onDelta != nil {
					if err := onDelta(ev.Delta); err != nil {
						return nil, err
					}
				}
			}
		case "response.completed":
			var ev struct {
				Response struct {
					Usage struct {
						InputTokens  int64 `json:"input_tokens"`
						OutputTokens int64 `json:"output_tokens"`
						TotalTokens  int64 `json:"total_tokens"`
						OutputTokensDetails struct {
							ReasoningTokens int64 `json:"reasoning_tokens"`
						} `json:"output_tokens_details"`
					} `json:"usage"`
				} `json:"response"`
			}
			if json.Unmarshal(raw, &ev) == nil && ev.Response.Usage.TotalTokens > 0 {
				result.PromptTokens = ev.Response.Usage.InputTokens
				result.CompletionTokens = ev.Response.Usage.OutputTokens
				result.ReasoningTokens = ev.Response.Usage.OutputTokensDetails.ReasoningTokens
			}
		case "response.failed":
			var ev struct {
				Response struct {
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				} `json:"response"`
			}
			if json.Unmarshal(raw, &ev) == nil && ev.Response.Error.Message != "" {
				return result, errors.New(ev.Response.Error.Message)
			}
		case "error":
			var ev struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(raw, &ev) == nil && ev.Message != "" {
				return result, errors.New(ev.Message)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if result.Text != "" {
			return result, nil // 已有部分输出，返回可用结果
		}
		return nil, err
	}
	if result.Text == "" && result.CompletionTokens == 0 {
		return nil, errors.New("上游没有返回任何内容")
	}
	return result, nil
}

// ==================== 公共 ====================

// NewProxyHTTPClient 构造支持 http(s)/socks5 代理的客户端。
func NewProxyHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	return newHTTPClient(proxyURL, timeout)
}

// newHTTPClient timeout<=0 表示不限时（流式长连接）。
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
			dialer, err := proxy.FromURL(u, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("创建 SOCKS5 代理失败: %w", err)
			}
			if cd, ok := dialer.(proxy.ContextDialer); ok {
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

func snippet(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
