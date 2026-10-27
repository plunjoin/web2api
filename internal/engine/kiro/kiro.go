// Package kiro 实现 AWS Kiro 引擎的协议客户端：
// Kiro 私有 auth 端点刷新（prod.us-east-1.auth.desktop.kiro.dev）+
// CodeWhisperer generateAssistantResponse（AWS Event Stream 二进制流解析）。
package kiro

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
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

// 上游常量。
const (
	RefreshURL  = "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"
	GenerateURL = "https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse"
	// GenerateURLHost 与 GenerateURL 同主机，用量查询等 API 拼接用。
	GenerateURLHost = "https://codewhisperer.us-east-1.amazonaws.com"

	// Kiro 用量/元数据查询的客户端 UA（模拟 KiroIDE）
	kiroUserAgent = "aws-sdk-js/1.0.0 ua/2.1 os/windows#10.0 lang/js md/nodejs#20.16.0 api/codewhispererruntime#1.0.0 m/E KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1"

	// generateAssistantResponse 的客户端 UA（与 KiroIDE 流式 API 一致，kiro2api 同款）
	kiroAmzUserAgent    = "aws-sdk-js/1.0.18 KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1"
	kiroStreamUserAgent = "aws-sdk-js/1.0.18 ua/2.1 os/darwin#25.0.0 lang/js md/nodejs#20.16.0 api/codewhispererstreaming#1.0.18 m/E KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1"

	// 注意：generateAssistantResponse 不携带 profileArn——kiro2api 实测不发送；
	// 硬编码他人 profileArn 会被服务端按错误 profile 校验模型，导致 400 Invalid model。

	chatTriggerType = "MANUAL"
	origin          = "AI_EDITOR"
	agentTaskType  = "vibe"

	historyAssistantAck = "I will follow these instructions"
)

// DefaultModels 引擎模型目录（对外 OpenAI 风格模型名）。
var DefaultModels = []string{
	"claude-sonnet-4-5", "claude-sonnet-4", "claude-3-7-sonnet",
}

// ModelMap 外部模型名 → CodeWhisperer modelId 枚举。
// 上游枚举会随 Kiro 版本演进（旧枚举可能报 400 Invalid model），跟随 kiro2api 等项目同步。
var ModelMap = map[string]string{
	"claude-sonnet-4-5":          "CLAUDE_SONNET_4_5_20250929_V1_0",
	"claude-sonnet-4-5-20250929": "CLAUDE_SONNET_4_5_20250929_V1_0",
	"claude-sonnet-4":            "CLAUDE_SONNET_4_5_20250929_V1_0",
	"claude-sonnet-4-20250514":   "CLAUDE_SONNET_4_20250514_V1_0",
	"claude-3-7-sonnet":          "CLAUDE_3_7_SONNET_20250219_V1_0",
	"claude-3-5-haiku":            "auto",
	"claude-haiku-4-5":            "auto",
}

// MapModel 外部模型名 → CodeWhisperer modelId。
func MapModel(model string) (string, error) {
	if id, ok := ModelMap[strings.ToLower(strings.TrimSpace(model))]; ok {
		return id, nil
	}
	up := strings.ToUpper(strings.TrimSpace(model))
	if strings.HasPrefix(up, "CLAUDE_") {
		return up, nil // 允许直接传枚举
	}
	return "", fmt.Errorf("不支持的模型: %s（可用: claude-sonnet-4、claude-3-7-sonnet）", model)
}

// ==================== Token 刷新 ====================

// Token Kiro 令牌（refresh_token 为旋转式，每次刷新返回新值必须写回）。
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type refreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"` // ISO8601
}

// Refresh 用 refresh_token 换新令牌。
func Refresh(ctx context.Context, proxy, refreshToken string) (*Token, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("refresh_token 为空")
	}
	client, err := newHTTPClient(proxy, 30*time.Second)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]string{"refreshToken": refreshToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, RefreshURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kiro auth 请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{Status: resp.StatusCode, Body: string(body)}
	}
	var rr refreshResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, fmt.Errorf("刷新响应解析失败: %w", err)
	}
	if rr.AccessToken == "" {
		return nil, errors.New("刷新响应缺少 accessToken（refresh_token 可能已失效）")
	}
	tok := &Token{AccessToken: rr.AccessToken}
	if rr.RefreshToken != "" {
		tok.RefreshToken = rr.RefreshToken
	} else {
		tok.RefreshToken = refreshToken
	}
	if rr.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, rr.ExpiresAt); err == nil {
			tok.ExpiresAt = t
		}
	}
	return tok, nil
}

// ==================== 推理 ====================

// Message 输入消息（role: system/user/assistant）。
type Message struct {
	Role string
	Text string
}

// Request 推理请求。
type Request struct {
	Model    string
	Messages []Message
}

// Result 推理结果。
type Result struct {
	Text string
	// CodeWhisperer 不返回 usage，由网关按文本估算
}

type userInputMessage struct {
	Content string `json:"content"`
	ModelID string `json:"modelId"`
	Images  []any  `json:"images"` // 始终是数组（空数组也带），与官方客户端一致
	Origin  string `json:"origin"`
}

type assistantResponseMessage struct {
	Content  []any  `json:"content"`
	ToolUses []any  `json:"toolUses"`
}

type historyItem struct {
	UserInputMessage         *userInputMessage         `json:"userInputMessage,omitempty"`
	AssistantResponseMessage *assistantResponseMessage `json:"assistantResponseMessage,omitempty"`
}

type conversationState struct {
	AgentContinuationId string `json:"agentContinuationId"` // 代理延续 ID（uuid），与官方客户端一致
	AgentTaskType       string `json:"agentTaskType"`       // 固定 "vibe"
	ChatTriggerType     string `json:"chatTriggerType"`
	ConversationID      string `json:"conversationId"`
	CurrentMessage      struct {
		UserInputMessage userInputMessage `json:"userInputMessage"`
	} `json:"currentMessage"`
	History []historyItem `json:"history"`
}

type generateRequest struct {
	ConversationState conversationState `json:"conversationState"`
	// 不发 profileArn（见常量区注释）
}

func buildRequest(req *Request, modelID string) (*generateRequest, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("消息为空")
	}
	out := &generateRequest{}
	cs := &out.ConversationState
	cs.AgentContinuationId = uuid.NewString()
	cs.AgentTaskType = agentTaskType
	cs.ChatTriggerType = chatTriggerType
	cs.ConversationID = uuid.NewString()
	cs.CurrentMessage.UserInputMessage = userInputMessage{
		Content: "", ModelID: modelID, Images: []any{}, Origin: origin,
	}

	var history []historyItem
	pendingUser := ""
	flushUser := func(text string) {
		history = append(history, historyItem{UserInputMessage: &userInputMessage{
			Content: text, ModelID: modelID, Images: []any{}, Origin: origin,
		}})
	}
	for _, msg := range req.Messages {
		switch strings.ToLower(msg.Role) {
		case "system":
			// system → user + assistant 确认对
			flushUser(msg.Text)
			history = append(history, historyItem{AssistantResponseMessage: &assistantResponseMessage{
				Content: []any{map[string]any{"text": historyAssistantAck}},
			}})
		case "user":
			pendingUser = msg.Text
		case "assistant":
			if pendingUser != "" {
				flushUser(pendingUser)
				pendingUser = ""
			}
			history = append(history, historyItem{AssistantResponseMessage: &assistantResponseMessage{
				Content: []any{map[string]any{"text": msg.Text}},
			}})
		}
	}
	// 最后一条 user 消息作为 currentMessage
	if pendingUser == "" {
		return nil, errors.New("最后一条消息必须是 user")
	}
	cs.CurrentMessage.UserInputMessage.Content = pendingUser
	cs.History = history
	return out, nil
}

// Stream 发起 generateAssistantResponse 并流式回调增量文本（真流式解析事件流）。
// Stream 发起 generateAssistantResponse 并流式回调增量文本（真流式解析事件流）。
// 具名模型枚举被账号拒绝时（免费档常见，仅开放 auto），自动降级 auto 重试一次。
func Stream(ctx context.Context, proxy, accessToken string, req *Request, onDelta func(string) error) (*Result, error) {
	modelID, err := MapModel(req.Model)
	if err != nil {
		return nil, err
	}
	client, err := newHTTPClient(proxy, 0)
	if err != nil {
		return nil, err
	}
	res, err := generateOnce(ctx, client, accessToken, req, modelID, onDelta)
	if err == nil {
		return res, nil
	}
	if isInvalidModel(err) && modelID != "auto" {
		// 账号 profile 未开放该具名枚举：降级 auto（上游自动路由模型）
		return generateOnce(ctx, client, accessToken, req, "auto", onDelta)
	}
	return nil, err
}

// isInvalidModel 判断上游 400 是否为模型枚举不被接受（INVALID_MODEL_ID）。
func isInvalidModel(err error) bool {
	var ue *UpstreamError
	if !errors.As(err, &ue) || ue.Status != http.StatusBadRequest {
		return false
	}
	return strings.Contains(ue.Body, "INVALID_MODEL_ID") ||
		strings.Contains(ue.Body, "Invalid model")
}

// generateOnce 用指定 modelId 发起一次推理。
func generateOnce(ctx context.Context, client *http.Client, accessToken string, req *Request, modelID string, onDelta func(string) error) (*Result, error) {
	body, err := buildRequest(req, modelID)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, GenerateURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	// KiroIDE 流式 API 伪装头（kiro2api 同款；缺失可能被服务端风控拒绝）
	httpReq.Header.Set("x-amzn-kiro-agent-mode", "spec")
	httpReq.Header.Set("x-amz-user-agent", kiroAmzUserAgent)
	httpReq.Header.Set("User-Agent", kiroStreamUserAgent)
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("codewhisperer 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, &UpstreamError{Status: resp.StatusCode, Body: string(b)}
	}
	return parseEventStream(resp.Body, onDelta)
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
	if err := json.Unmarshal([]byte(e.Body), &m); err == nil && m.Message != "" {
		msg = m.Message
	}
	if strings.Contains(strings.ToLower(e.Body), "too many requests") || e.Status == 429 {
		return "上游限流（TooManyRequests）: " + strings.TrimSpace(msg)
	}
	return fmt.Sprintf("上游 HTTP %d: %s", e.Status, strings.TrimSpace(msg))
}

// IsUnauthorized 判断是否需要刷新 token 重试。
func IsUnauthorized(err error) bool {
	var ue *UpstreamError
	if errors.As(err, &ue) {
		return ue.Status == http.StatusUnauthorized
	}
	return false
}

// IsSuspended 判断上游 403 是否为账号被封停（安全锁定）。
// 这种情况刷新 token 无效，需要用户到 app.kiro.dev 验证身份恢复。
func IsSuspended(err error) bool {
	var ue *UpstreamError
	if !errors.As(err, &ue) || ue.Status != http.StatusForbidden {
		return false
	}
	lower := strings.ToLower(ue.Body)
	return strings.Contains(lower, "suspended") || strings.Contains(lower, "locked your account")
}

// IsRateLimited 判断上游限流。
func IsRateLimited(err error) bool {
	var ue *UpstreamError
	if errors.As(err, &ue) {
		return ue.Status == http.StatusTooManyRequests ||
			strings.Contains(strings.ToLower(ue.Body), "too many requests")
	}
	return false
}

// ==================== AWS Event Stream 解析 ====================

// assistantEvent assistantResponseEvent 的扁平 payload。
type assistantEvent struct {
	Content string `json:"content"`
	Input   string `json:"input"`
	Name    string `json:"name"`
	ToolID  string `json:"toolUseId"`
	Stop    bool   `json:"stop"`
}

// parseEventStream 解析 AWS Event Stream 二进制分块流。
// 帧结构：total_len(4B) headers_len(4B) prelude_crc(4B) headers(TLV) payload(JSON) message_crc(4B)
func parseEventStream(r io.Reader, onDelta func(string) error) (*Result, error) {
	result := &Result{}
	reader := bufio.NewReaderSize(r, 32<<10)
	sawAny := false
	for {
		var head [8]byte
		if _, err := io.ReadFull(reader, head[:]); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if sawAny && result.Text != "" {
				return result, nil
			}
			return nil, fmt.Errorf("读取事件流失败: %w", err)
		}
		totalLen := binary.BigEndian.Uint32(head[0:4])
		headersLen := binary.BigEndian.Uint32(head[4:8])
		if totalLen < 16 || totalLen > 16<<20 {
			// 非法帧长度：跳过或终止
			if sawAny {
				break
			}
			return nil, errors.New("事件流帧长度非法")
		}
		// 剩余 = prelude_crc(4) + headers + payload + message_crc(4)
		rest := make([]byte, totalLen-8)
		if _, err := io.ReadFull(reader, rest); err != nil {
			if sawAny && result.Text != "" {
				return result, nil
			}
			return nil, fmt.Errorf("读取事件帧失败: %w", err)
		}
		payloadLen := int(totalLen) - 16 - int(headersLen)
		if payloadLen <= 0 || 4+int(headersLen)+payloadLen > len(rest) {
			continue // 忽略无 payload 的帧
		}
		// rest 布局：prelude_crc(4) + headers(headersLen) + payload + message_crc(4)
		payload := rest[4+int(headersLen) : 4+int(headersLen)+payloadLen]
		if len(bytes.TrimSpace(payload)) == 0 {
			continue
		}
		var ev assistantEvent
		if err := json.Unmarshal(payload, &ev); err != nil {
			continue // metadataEvent 等非数据事件
		}
		sawAny = true
		if ev.Content != "" {
			result.Text += ev.Content
			if onDelta != nil {
				if err := onDelta(ev.Content); err != nil {
					return nil, err
				}
			}
		}
		if ev.Stop {
			return result, nil
		}
	}
	if result.Text == "" {
		return nil, errors.New("上游没有返回任何内容")
	}
	return result, nil
}

// ==================== 用量查询 ====================

// UsageLimits getUsageLimits 响应。
type UsageLimits struct {
	UsageBreakdownList []UsageBreakdown `json:"usageBreakdownList"`
	UserInfo           struct {
		Email string `json:"email"`
	} `json:"userInfo"`
	DaysUntilReset   int `json:"daysUntilReset"`
	SubscriptionInfo struct {
		SubscriptionTitle string `json:"subscriptionTitle"`
		Type              string `json:"type"`
	} `json:"subscriptionInfo"`
}

// UsageBreakdown 单类资源的用量（关注 resourceType=CREDIT）。
type UsageBreakdown struct {
	ResourceType              string `json:"resourceType"`
	UsageLimitWithPrecision    float64 `json:"usageLimitWithPrecision"`
	CurrentUsageWithPrecision  float64 `json:"currentUsageWithPrecision"`
	DisplayName               string `json:"displayName"`
	FreeTrialInfo             *struct {
		FreeTrialStatus           string  `json:"freeTrialStatus"`
		UsageLimitWithPrecision   float64 `json:"usageLimitWithPrecision"`
		CurrentUsageWithPrecision float64 `json:"currentUsageWithPrecision"`
	} `json:"freeTrialInfo,omitempty"`
}

// AvailableCredits 计算剩余可用次数（基础额度 + 有效试用额度 - 已用）。
func (u *UsageLimits) AvailableCredits() float64 {
	for _, b := range u.UsageBreakdownList {
		if b.ResourceType != "CREDIT" {
			continue
		}
		total := b.UsageLimitWithPrecision
		used := b.CurrentUsageWithPrecision
		if b.FreeTrialInfo != nil && b.FreeTrialInfo.FreeTrialStatus == "ACTIVE" {
			total += b.FreeTrialInfo.UsageLimitWithPrecision
			used += b.FreeTrialInfo.CurrentUsageWithPrecision
		}
		if rem := total - used; rem > 0 {
			return rem
		}
		return 0
	}
	return 0
}

// GetUsageLimits 查询账号用量与订阅信息。
func GetUsageLimits(ctx context.Context, proxy, accessToken string) (*UsageLimits, error) {
	client, err := newHTTPClient(proxy, 30*time.Second)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		GenerateURLHost+"/getUsageLimits?isEmailRequired=true&origin=AI_EDITOR&resourceType=AGENTIC_REQUEST", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-amz-user-agent", kiroUserAgent)
	req.Header.Set("User-Agent", kiroUserAgent)
	req.Header.Set("amz-sdk-invocation-id", fmt.Sprintf("%d-%s", time.Now().UnixNano(), "web2api"))
	req.Header.Set("amz-sdk-request", "attempt=1; max=1")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Connection", "close")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("用量查询失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{Status: resp.StatusCode, Body: string(body)}
	}
	var out UsageLimits
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("用量响应解析失败: %w", err)
	}
	return &out, nil
}

// ==================== 公共 ====================

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
