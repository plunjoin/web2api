// Package workbuddy 实现腾讯 WorkBuddy（CodeBuddy 桌面 Agent）的协议客户端：
// 两个发行版（国服 copilot.tencent.com / 国际服 www.workbuddy.ai）的 OAuth
// 登录、token 刷新、聊天补全（OpenAI Chat Completions 协议，SSE 流式），
// 以及「自签」——把请求伪装成 WorkBuddy 桌面 CLI 发出（UA / X-* 头 /
// gzip / 会话 ID 全套齐备，缺一项上游都可能拒绝）。
package workbuddy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	xproxy "golang.org/x/net/proxy"

	"web2api/internal/model"
)

// DefaultBaseURL 默认上游（国服 WorkBuddy）。
var DefaultBaseURL = SiteCN.Endpoint

// 旧 CodeBuddy CLI 的伪装常量：仅用于 base_url 仍是 www.codebuddy.ai 的
// 旧账号，保持它们升级后仍能登录。
const (
	legacyCLIVer   = "1.0.7"
	legacyCLIUA    = "CLI/" + legacyCLIVer + " CodeBuddy/" + legacyCLIVer
	legacyAuthUA   = "CLI/1.0.8 CodeBuddy/1.0.8"
	pendingLogin   = 11217 // 轮询返回：等待用户在浏览器完成登录
	channelErrCode = 11128 // 上游渠道检测错误码（请求中含"污染"内容触发）
)

// systemKeyReplacements system 消息竞品关键词替换（上游风控检测）。
var systemKeyReplacements = []struct{ from, to string }{
	{"Claude Code", "CodeBuddy Code"},
	{"Anthropic's official CLI for Claude", "Tencent's official CLI for CodeBuddy"},
	{"Anthropic", "Tencent"},
	{"Claude", "CodeBuddy"},
}

// StreamAuth 是一次请求所需的账号身份（在锁外快照，避免并发读写凭据）。
type StreamAuth struct {
	Token        string
	UID          string
	Domain       string
	EnterpriseID string
	Site         Site
	Endpoint     string // 已解析的上游根地址（不含末尾斜杠）
}

func (a StreamAuth) domain() string {
	if strings.TrimSpace(a.Domain) != "" {
		return a.Domain
	}
	// 优先用实际上游地址的 host：旧 CodeBuddy 账号的 Site 仍是国服，
	// 但 X-Domain 必须与请求域名一致。
	if h := hostOf(a.Endpoint); h != "" {
		return h
	}
	return a.Site.host()
}

// identity 返回请求使用的客户端身份。旧 CodeBuddy 账号继续伪装 CLI，
// 新 WorkBuddy 伪装桌面端；上游按 UA / X-IDE-* 识别客户端，混用会被拒。
func (a StreamAuth) identity() (ua, ideType, ideName, ideVer string) {
	if IsLegacyCodeBuddy(a.Endpoint) {
		return legacyCLIUA, "CLI", "CLI", legacyCLIVer
	}
	return a.Site.cliAgent(), "WorkBuddy", "WorkBuddy", a.Site.App
}

// applySignedHeaders 按 WorkBuddy 桌面客户端的身份签名一个请求。
func applySignedHeaders(req *http.Request, a StreamAuth) {
	ua, ideType, ideName, ideVer := a.identity()
	req.Header.Set("Authorization", "Bearer "+a.Token)
	req.Header.Set("X-User-Id", a.UID)
	req.Header.Set("X-Domain", a.domain())
	req.Header.Set("X-Product", "SaaS")
	req.Header.Set("X-IDE-Type", ideType)
	req.Header.Set("X-IDE-Name", ideName)
	req.Header.Set("X-IDE-Version", ideVer)
	req.Header.Set("User-Agent", ua)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("X-Agent-Intent", "craft")
	req.Header.Set("X-Agent-Purpose", "conversation")
	if strings.TrimSpace(a.EnterpriseID) != "" {
		req.Header.Set("X-Enterprise-Id", a.EnterpriseID)
		req.Header.Set("X-Tenant-Id", a.EnterpriseID)
	}
}

// applyChatHeaders 在签名头之上补聊天专用头（OpenAI SDK 的 X-Stainless-*、
// Agent 类型、会话 ID）。WorkBuddy 后端按 X-Conversation-Request-ID 计一次
// 用户发送，所以一次发送内的所有步骤要共用同一个。
func applyChatHeaders(req *http.Request, a StreamAuth) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Type", "main")
	req.Header.Set("X-Agent-Runtime", "local-agent")
	req.Header.Set("X-Stainless-Retry-Count", "0")
	req.Header.Set("X-Stainless-Timeout", "1200")
	req.Header.Set("X-Stainless-Lang", "js")
	if IsLegacyCodeBuddy(a.Endpoint) {
		// 旧 CLI 的 SDK 版本；改成桌面端的值会让老账号看起来像换了个客户端。
		req.Header.Set("X-Stainless-Package-Version", "5.10.1")
		req.Header.Set("X-Stainless-Runtime-Version", "v22.13.1")
	} else {
		req.Header.Set("X-Stainless-Package-Version", "6.25.0")
		req.Header.Set("X-Stainless-Runtime-Version", "v22.21.1")
	}
	req.Header.Set("X-Stainless-OS", "Windows")
	req.Header.Set("X-Stainless-Arch", "x64")
	req.Header.Set("X-Stainless-Runtime", "node")
}

// hostOf 从地址里取 host（非法地址返回空）。
func hostOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// newConversationIDs 生成一次请求的会话/消息 ID。
func newConversationIDs() (conversation, request, message, reqID string) {
	conversation = uuid.NewString()
	request = hex.EncodeToString(randomBytes(16))
	message = strings.ReplaceAll(uuid.NewString(), "-", "")
	reqID = strings.ReplaceAll(uuid.NewString(), "-", "")
	return
}

// ==================== OAuth（设备授权式） ====================

// AuthStart 发起授权返回的会话信息。
type AuthStart struct {
	State   string
	AuthURL string
}

// authTarget 从 base_url 推出登录入口、platform 参数与 UA。
func authTarget(baseURL string) (endpoint, platform, ua string, site Site) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	if IsLegacyCodeBuddy(base) {
		return base, "CLI", legacyAuthUA, SiteCN
	}
	site = SiteForBaseURL(base)
	return base, site.Platform, site.cliAgent(), site
}

// StartAuth 发起登录授权：返回 state 与用户浏览器打开的 authUrl。
func StartAuth(ctx context.Context, proxy, baseURL string) (*AuthStart, error) {
	endpoint, platform, ua, _ := authTarget(baseURL)
	nonce := hex.EncodeToString(randomBytes(16))
	payload, _ := json.Marshal(map[string]string{"nonce": nonce})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint+"/v2/plugin/auth/state?platform="+url.QueryEscape(platform)+"&nonce="+nonce, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setAuthHeaders(req, ua)
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
	authURL := out.Data.AuthURL
	// 桌面端版本参数：旧版登录页对低于 5.8.0 的版本隐藏企业微信登录。
	if u, err := url.Parse(authURL); err == nil && u.Scheme == "https" {
		q := u.Query()
		if q.Get("version") == "" {
			q.Set("version", siteApp)
			u.RawQuery = q.Encode()
			authURL = u.String()
		}
	}
	return &AuthStart{State: out.Data.State, AuthURL: authURL}, nil
}

// Token 授权结果。
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Email        string
	UID          string
	Domain       string
	EnterpriseID string
}

// ErrPendingLogin 轮询时用户尚未完成登录。
var ErrPendingLogin = errors.New("等待用户在浏览器完成登录")

// PollToken 轮询授权结果（用户完成登录前返回 ErrPendingLogin）。
func PollToken(ctx context.Context, proxy, baseURL, state string) (*Token, error) {
	endpoint, _, ua, _ := authTarget(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		endpoint+"/v2/plugin/auth/token?state="+url.QueryEscape(state), nil)
	if err != nil {
		return nil, err
	}
	setAuthHeaders(req, ua)
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
			ExpiresIn    int64  `json:"expiresIn"`
			Domain       string `json:"domain"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("轮询响应解析失败: %w", err)
	}
	if out.Code == pendingLogin {
		return nil, ErrPendingLogin
	}
	if out.Code != 0 || out.Data.AccessToken == "" {
		return nil, fmt.Errorf("授权未完成（code=%d）: %s", out.Code, strings.TrimSpace(out.Message))
	}
	tok := &Token{
		AccessToken:  out.Data.AccessToken,
		RefreshToken: out.Data.RefreshToken,
		Domain:       out.Data.Domain,
		Email:        JWTEmail(out.Data.AccessToken),
	}
	if out.Data.ExpiresIn > 0 {
		// 提前 5 分钟视为过期，避免请求发出时正好到期；有效期本身不足 5 分钟时
		// 不要减成负数（会让新 token 一落地就被当成过期）。
		skew := int64(300)
		if out.Data.ExpiresIn <= skew {
			skew = out.Data.ExpiresIn / 6
		}
		tok.ExpiresAt = time.Now().Add(time.Duration(out.Data.ExpiresIn-skew) * time.Second)
	}
	// 取账号 uid / 企业 id：请求签名里的 X-User-Id 必须是 uid，不是邮箱。
	if uid, ent := fetchIdentity(ctx, proxy, endpoint, ua, tok.AccessToken, tok.Domain, state); uid != "" {
		tok.UID = uid
		tok.EnterpriseID = ent
	}
	return tok, nil
}

// fetchIdentity 调 /v2/plugin/login/account 取 uid 与企业 id。失败不致命。
func fetchIdentity(ctx context.Context, proxy, endpoint, ua, token, domain, state string) (uid, entID string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		endpoint+"/v2/plugin/login/account?state="+url.QueryEscape(state), nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Domain", domain)
	req.Header.Set("X-No-User-Id", "true")
	req.Header.Set("X-No-Enterprise-Id", "true")
	resp, err := doRequest(ctx, proxy, req, 20*time.Second)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Data struct {
			UID          string `json:"uid"`
			EnterpriseID string `json:"enterpriseId"`
			Enterprise   struct {
				EnterpriseID string `json:"enterpriseId"`
			} `json:"enterprise"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &out) != nil {
		return "", ""
	}
	ent := out.Data.EnterpriseID
	if ent == "" {
		ent = out.Data.Enterprise.EnterpriseID
	}
	return strings.TrimSpace(out.Data.UID), strings.TrimSpace(ent)
}

// Refresh 用 refresh_token 换新 token（WorkBuddy 的刷新接口）。
func Refresh(ctx context.Context, proxy string, a StreamAuth, refreshToken string) (*Token, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("refresh_token 为空")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.Endpoint+"/v2/plugin/auth/token/refresh", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	ua, _, _, _ := a.identity()
	req.Header.Set("User-Agent", ua)
	req.Header.Set("X-Refresh-Token", refreshToken)
	req.Header.Set("X-Auth-Refresh-Source", "plugin")
	req.Header.Set("X-Domain", a.domain())
	if strings.TrimSpace(a.EnterpriseID) != "" {
		req.Header.Set("X-Enterprise-Id", a.EnterpriseID)
		req.Header.Set("X-Tenant-Id", a.EnterpriseID)
	}
	resp, err := doRequest(ctx, proxy, req, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("刷新 token 失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, newUpstreamError(resp.StatusCode, body)
	}
	var out struct {
		Code int `json:"code"`
		Data struct {
			AccessToken      string `json:"accessToken"`
			RefreshToken     string `json:"refreshToken"`
			ExpiresIn        int64  `json:"expiresIn"`
			RefreshExpiresIn int64  `json:"refreshExpiresIn"`
			Domain           string `json:"domain"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("刷新响应解析失败: %w", err)
	}
	if out.Code != 0 || out.Data.AccessToken == "" {
		return nil, fmt.Errorf("刷新失败（code=%d）: %s", out.Code, strings.TrimSpace(out.Msg))
	}
	tok := &Token{
		AccessToken:  out.Data.AccessToken,
		RefreshToken: out.Data.RefreshToken,
		Domain:       out.Data.Domain,
	}
	if out.Data.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(out.Data.ExpiresIn-60) * time.Second)
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
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
	Role      string
	Text      string
	ToolCalls []model.ToolCall
}

// Request 推理请求。
type Request struct {
	Model       string
	Messages    []Message
	Tools       []model.Tool
	ToolChoice  json.RawMessage
	MaxTokens   int
	Temperature *float64
	// ConversationID 由网关按会话传入，同一会话复用（上游按它归并用量）。
	ConversationID string
}

// Result 推理结果。
type Result struct {
	Text             string
	ToolCalls        []model.ToolCall
	PromptTokens     int64
	CompletionTokens int64
}

type chatToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type chatMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

type chatBody struct {
	Model       string          `json:"model"`
	Messages    []chatMessage   `json:"messages"`
	Tools       []model.Tool    `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
	Stream      bool            `json:"stream"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
}

func buildChatBody(req *Request) (*chatBody, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("消息为空")
	}
	body := &chatBody{Model: req.Model, Stream: true, Temperature: req.Temperature}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		body.Tools = req.Tools
		if choice := bytes.TrimSpace(req.ToolChoice); len(choice) > 0 && string(choice) != "null" {
			body.ToolChoice = req.ToolChoice
		}
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
		cm := chatMessage{Role: role, Content: text}
		for _, tc := range msg.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, chatToolCall{
				ID: tc.ID, Type: tc.Type,
				Function: struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				}{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
			})
		}
		// assistant 的纯工具调用回合 content 为空，靠 ToolCalls 保留。
		if strings.TrimSpace(text) == "" && len(cm.ToolCalls) == 0 {
			continue
		}
		body.Messages = append(body.Messages, cm)
	}
	// 上游要求至少 2 条消息：单条 user 前补 system。
	if len(body.Messages) == 1 {
		body.Messages = append([]chatMessage{{Role: "system", Content: "You are a helpful assistant."}}, body.Messages...)
	}
	if len(body.Messages) == 0 {
		return nil, errors.New("没有可发送的消息")
	}
	return body, nil
}

// Stream 发起聊天补全（上游强制流式）。body 走 gzip，头按桌面 CLI 签名。
func Stream(ctx context.Context, proxy string, a StreamAuth, req *Request, sink model.StreamSink) (*Result, error) {
	if sink == nil {
		sink = model.TextSink(nil)
	}
	body, err := buildChatBody(req)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	payload, err = gzipBytes(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(a.Endpoint, "/")
	if endpoint == "" {
		endpoint = a.Site.endpoint()
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint+"/v2/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	applySignedHeaders(httpReq, a)
	applyChatHeaders(httpReq, a)
	httpReq.Header.Set("Content-Encoding", "gzip")
	conversation, requestID, message, reqID := newConversationIDs()
	if req.ConversationID != "" {
		conversation = req.ConversationID
	}
	httpReq.Header.Set("X-Conversation-ID", conversation)
	httpReq.Header.Set("X-Conversation-Request-ID", requestID)
	httpReq.Header.Set("X-Conversation-Message-ID", message)
	httpReq.Header.Set("X-Request-ID", reqID)

	resp, err := doRequest(ctx, proxy, httpReq, 0)
	if err != nil {
		return nil, fmt.Errorf("workbuddy 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, newUpstreamError(resp.StatusCode, b)
	}
	return parseSSE(resp.Body, sink)
}

func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UpstreamError 上游非 200 响应（带解析出的业务码）。
type UpstreamError struct {
	Status int
	Body   string
	Code   int
}

func newUpstreamError(status int, body []byte) *UpstreamError {
	e := &UpstreamError{Status: status, Body: string(body)}
	e.Code = parseCode(body)
	return e
}

func (e *UpstreamError) Error() string {
	msg := e.Body
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(e.Body), &m) == nil && m.Message != "" {
		msg = m.Message
	}
	if e.Code != 0 {
		return fmt.Sprintf("上游 HTTP %d (code=%d): %s", e.Status, e.Code, strings.TrimSpace(msg))
	}
	return fmt.Sprintf("上游 HTTP %d: %s", e.Status, strings.TrimSpace(msg))
}

// parseCode 从 WorkBuddy 的几种错误壳里取业务码：
// {code:14018}、{data:{code}}、{error:{code}}、{error:{internal_code:"11159"}}、
// {"error":"11158:..."}。
func parseCode(body []byte) int {
	var v map[string]any
	if json.Unmarshal(body, &v) != nil {
		return 0
	}
	candidates := []any{v["code"]}
	if d, ok := v["data"].(map[string]any); ok {
		candidates = append(candidates, d["code"])
	}
	if e, ok := v["error"].(map[string]any); ok {
		candidates = append(candidates, e["code"], e["internal_code"])
	}
	for _, c := range candidates {
		switch n := c.(type) {
		case float64:
			if n != 0 {
				return int(n)
			}
		case string:
			s := strings.TrimSpace(n)
			if s != "" {
				var code int
				if _, err := fmt.Sscanf(s, "%d", &code); err == nil && code != 0 {
					return code
				}
			}
		}
	}
	if s, ok := v["error"].(string); ok {
		if code := leadingCode(s); code != 0 {
			return code
		}
	}
	return 0
}

func leadingCode(s string) int {
	s = strings.TrimSpace(s)
	idx := strings.IndexByte(s, ':')
	if idx <= 0 {
		return 0
	}
	var code int
	if _, err := fmt.Sscanf(s[:idx], "%d", &code); err == nil {
		return code
	}
	return 0
}

// ==================== 错误分类（供智能路由决策） ====================

// IsUnauthorized 判断 token 失效 / 无权限。403 也常被上游用来表达「风控封禁」
// 「额度用尽」「渠道拒绝」，这些有专门的判定，这里先用业务码排除，免得把
// 换号也没用的错误当成 token 问题反复重试。
func IsUnauthorized(err error) bool {
	var ue *UpstreamError
	if !errors.As(err, &ue) {
		return false
	}
	switch ue.Code {
	case 14018, 11140, channelErrCode:
		return false
	}
	return ue.Status == http.StatusUnauthorized || ue.Status == http.StatusForbidden
}

// UpstreamStatus 返回上游 HTTP 状态码（非上游错误为 0）。
func UpstreamStatus(err error) int {
	var ue *UpstreamError
	if errors.As(err, &ue) {
		return ue.Status
	}
	return 0
}

// IsRateLimited 判断上游限流。
func IsRateLimited(err error) bool {
	var ue *UpstreamError
	if errors.As(err, &ue) {
		return ue.Status == http.StatusTooManyRequests
	}
	return false
}

// IsCreditsExhausted 判断额度用尽（14018）。
func IsCreditsExhausted(err error) bool {
	var ue *UpstreamError
	if !errors.As(err, &ue) {
		return false
	}
	if ue.Code == 14018 {
		return true
	}
	b := strings.ToLower(ue.Body)
	return strings.Contains(b, "credits exhausted") || strings.Contains(b, "额度已用尽") || strings.Contains(b, "額度已用盡")
}

// IsRestricted 判断账号被风控封禁（11140）。
func IsRestricted(err error) bool {
	var ue *UpstreamError
	if !errors.As(err, &ue) {
		return false
	}
	if ue.Code == 11140 {
		return true
	}
	b := strings.ToLower(ue.Body)
	return strings.Contains(b, "restricted due to unusual activity") ||
		strings.Contains(b, "账号存在异常行为") || strings.Contains(b, "帳號存在異常行為")
}

// IsChannelRefused 判断触发渠道检测（11128，多半是 prompt 里的竞品关键词）。
func IsChannelRefused(err error) bool {
	var ue *UpstreamError
	if !errors.As(err, &ue) {
		return false
	}
	if ue.Code == channelErrCode {
		return true
	}
	b := strings.ToLower(ue.Body)
	return strings.Contains(b, "unapproved channel") || strings.Contains(b, "illegal api invocation")
}

func parseSSE(r io.Reader, sink model.StreamSink) (*Result, error) {
	result := &Result{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	accs := map[int]*model.ToolCall{}
	var indices []int
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
					Content   string                `json:"content"`
					ToolCalls []model.ToolCallDelta `json:"tool_calls"`
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
				if err := sink.Text(ch.Delta.Content); err != nil {
					return nil, err
				}
			}
			for _, tc := range ch.Delta.ToolCalls {
				acc := accs[tc.Index]
				if acc == nil {
					acc = &model.ToolCall{Type: "function"}
					accs[tc.Index] = acc
					indices = append(indices, tc.Index)
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Type != "" {
					acc.Type = tc.Type
				}
				acc.Function.Name += tc.Function.Name
				acc.Function.Arguments += tc.Function.Arguments
				if len(tc.ExtraContent) > 0 {
					acc.ExtraContent = tc.ExtraContent
				}
				if err := sink.ToolCall(tc); err != nil {
					return nil, err
				}
			}
		}
		if chunk.Usage != nil {
			result.PromptTokens = chunk.Usage.PromptTokens
			result.CompletionTokens = chunk.Usage.CompletionTokens
		}
	}
	if err := scanner.Err(); err != nil {
		if result.Text != "" || len(indices) > 0 {
			return result, nil
		}
		return nil, err
	}
	if len(indices) > 0 {
		sort.Ints(indices)
		for _, index := range indices {
			call := *accs[index]
			if call.ID == "" {
				call.ID = "call_" + model.NewID(16)
			}
			if call.Type == "" {
				call.Type = "function"
			}
			result.ToolCalls = append(result.ToolCalls, call)
		}
	}
	if result.Text == "" && len(result.ToolCalls) == 0 {
		return nil, errors.New("上游没有返回任何内容")
	}
	return result, nil
}

// ==================== 公共 ====================

func setAuthHeaders(req *http.Request, ua string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("X-No-Authorization", "true")
	req.Header.Set("X-No-User-Id", "true")
	req.Header.Set("X-No-Enterprise-Id", "true")
	req.Header.Set("X-No-Department-Info", "true")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("X-Request-ID", strings.ReplaceAll(uuid.NewString(), "-", ""))
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

// base64Decode 解码 JWT 的 base64url（容忍缺省填充）。
func base64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

func snippet(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
