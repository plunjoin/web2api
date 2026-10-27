// engine_workbuddy.go 引擎 workbuddy：腾讯 CodeBuddy/WorkBuddy 订阅号池。
// 凭据为 www.codebuddy.ai 的 Bearer token（OAuth 设备授权流程获取，暂无刷新接口）。
package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/model"
	"web2api/internal/store"
	"web2api/internal/engine/workbuddy"
)

// WorkBuddyCredentials workbuddy 引擎凭据（accounts.credentials JSON）。
type WorkBuddyCredentials struct {
	Token        string `json:"token"`           // Bearer accessToken
	RefreshToken string `json:"refresh_token,omitempty"`
	Email        string `json:"email,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"` // Unix 秒，0 表示未知
	Proxy        string `json:"proxy,omitempty"`
}

// workbuddyAccount 池内账号。
type workbuddyAccount struct {
	id, label string
	mu        sync.Mutex
	creds     WorkBuddyCredentials
	enabled   bool

	ready   bool
	lastErr string

	cooldownUntil  time.Time
	cooldownReason string
}

func (a *workbuddyAccount) proxy(engineProxy string) string {
	if strings.TrimSpace(a.creds.Proxy) != "" {
		return a.creds.Proxy
	}
	return engineProxy
}

// tokenExpired 本地判断 token 过期（expires_at 提前 5 分钟）。
func (a *workbuddyAccount) tokenExpired() bool {
	if a.creds.ExpiresAt <= 0 {
		return false // 未知过期时间，乐观视为有效，靠上游 401 兜底
	}
	return time.Now().Unix() > a.creds.ExpiresAt
}

// WorkBuddyEngine workbuddy 引擎。
type WorkBuddyEngine struct {
	name    string
	proxy   string
	baseURL string
	st      *store.Store

	mu       sync.RWMutex
	accounts []*workbuddyAccount
	ids      map[string]int
	next     atomic.Uint64

	errMu   sync.Mutex
	lastErr error
}

// NewWorkBuddy 构建 workbuddy 引擎。
func NewWorkBuddy(cfg config.WorkBuddyConfig, st *store.Store) *WorkBuddyEngine {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = workbuddy.DefaultBaseURL
	}
	return &WorkBuddyEngine{
		name:    "workbuddy",
		proxy:   strings.TrimSpace(cfg.Proxy),
		baseURL: baseURL,
		st:      st,
		ids:     map[string]int{},
	}
}

// AddAccount 添加账号（token 即时校验延后到首次请求）。
func (e *WorkBuddyEngine) AddAccount(id, label string, creds WorkBuddyCredentials) error {
	if strings.TrimSpace(creds.Token) == "" {
		return errors.New("token 不能为空")
	}
	e.mu.Lock()
	if _, exists := e.ids[id]; exists {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 已在池中", id)
	}
	acc := &workbuddyAccount{id: id, label: label, creds: creds, enabled: true, ready: true}
	e.accounts = append(e.accounts, acc)
	e.ids[id] = len(e.accounts) - 1
	e.mu.Unlock()
	return nil
}

// RemoveAccount 移除账号。
func (e *WorkBuddyEngine) RemoveAccount(id string) error {
	e.mu.Lock()
	idx, ok := e.ids[id]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 不在池中", id)
	}
	e.accounts = append(e.accounts[:idx], e.accounts[idx+1:]...)
	delete(e.ids, id)
	for k, v := range e.ids {
		if v > idx {
			e.ids[k] = v - 1
		}
	}
	e.mu.Unlock()
	return nil
}

// SetEnabled 启停账号。
func (e *WorkBuddyEngine) SetEnabled(id string, enabled bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	idx, ok := e.ids[id]
	if !ok {
		return fmt.Errorf("账号 %s 不在池中", id)
	}
	e.accounts[idx].mu.Lock()
	e.accounts[idx].enabled = enabled
	e.accounts[idx].mu.Unlock()
	return nil
}

// ReplaceCredentials 更新凭据（重建账号）。
func (e *WorkBuddyEngine) ReplaceCredentials(id string, creds WorkBuddyCredentials) error {
	e.mu.RLock()
	idx, ok := e.ids[id]
	label := id
	if ok {
		label = e.accounts[idx].label
	}
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("账号 %s 不在池中", id)
	}
	if err := e.RemoveAccount(id); err != nil {
		return err
	}
	return e.AddAccount(id, label, creds)
}

// AccountStates 全部账号状态（管理台）。
func (e *WorkBuddyEngine) AccountStates() []AccountState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]AccountState, 0, len(e.accounts))
	for _, acc := range e.accounts {
		acc.mu.Lock()
		state := AccountState{
			ID:      acc.id,
			Label:   acc.label,
			Enabled: acc.enabled,
			Detail:  acc.lastErr,
			Tier:    acc.creds.Email,
			Models:  len(workbuddy.DefaultModels),
		}
		switch {
		case !acc.enabled:
			state.Status = "disabled"
		case time.Now().Before(acc.cooldownUntil):
			state.Status = "error"
			state.Detail = "冷却中: " + acc.cooldownReason
		case acc.tokenExpired():
			state.Status = "error"
			state.Detail = "token 已过期，请重新授权"
		case !acc.ready:
			state.Status = "initializing"
		default:
			state.Status = "ok"
			state.Ready = true
		}
		acc.mu.Unlock()
		out = append(out, state)
	}
	return out
}

// ---- Engine 接口 ----

func (e *WorkBuddyEngine) Name() string { return e.name }

func (e *WorkBuddyEngine) Describe() string {
	return "腾讯 CodeBuddy/WorkBuddy（SaaS 订阅号池）"
}

// Ready 至少一个账号可用。
func (e *WorkBuddyEngine) Ready() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, acc := range e.accounts {
		acc.mu.Lock()
		ok := acc.enabled && acc.ready && !acc.tokenExpired() && time.Now().After(acc.cooldownUntil)
		acc.mu.Unlock()
		if ok {
			return true
		}
	}
	return false
}

// ListModels workbuddy 模型目录。
func (e *WorkBuddyEngine) ListModels() []model.ModelInfo {
	e.mu.RLock()
	enabled := 0
	for _, acc := range e.accounts {
		acc.mu.Lock()
		if acc.enabled && !acc.tokenExpired() {
			enabled++
		}
		acc.mu.Unlock()
	}
	e.mu.RUnlock()
	out := make([]model.ModelInfo, 0, len(workbuddy.DefaultModels))
	for _, id := range workbuddy.DefaultModels {
		out = append(out, model.ModelInfo{
			ID:          id,
			Object:      "model",
			DisplayName: id,
			Available:   enabled > 0,
			Engine:      e.name,
		})
	}
	return out
}

// pick 轮询选择可用账号。
func (e *WorkBuddyEngine) pick() *workbuddyAccount {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := len(e.accounts)
	if n == 0 {
		return nil
	}
	start := int(e.next.Add(1))
	for i := 0; i < n; i++ {
		acc := e.accounts[(start+i)%n]
		acc.mu.Lock()
		ok := acc.enabled && acc.ready && !acc.tokenExpired() && time.Now().After(acc.cooldownUntil)
		acc.mu.Unlock()
		if ok {
			return acc
		}
	}
	return nil
}

func (e *WorkBuddyEngine) markCooldown(acc *workbuddyAccount, reason string, d time.Duration) {
	acc.mu.Lock()
	acc.cooldownUntil = time.Now().Add(d)
	acc.cooldownReason = reason
	acc.mu.Unlock()
}

// toWorkBuddyRequest ChatRequest → workbuddy.Request。
func toWorkBuddyRequest(req model.ChatRequest) *workbuddy.Request {
	out := &workbuddy.Request{
		Model:       req.Model,
		MaxTokens:   derefInt(req.MaxTokens),
		Temperature: req.Temperature,
	}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, workbuddy.Message{Role: m.PlainRole(), Text: m.Text()})
	}
	return out
}

// chatLoop 轮询账号直到成功或池耗尽。
func (e *WorkBuddyEngine) chatLoop(ctx context.Context, req model.ChatRequest, onDelta func(string) error) (*model.ChatResult, error) {
	if !e.Ready() {
		return nil, engineUnavailableError(e)
	}
	cReq := toWorkBuddyRequest(req)
	var lastErr error
	for attempt := 0; attempt < e.usableCount(); attempt++ {
		acc := e.pick()
		if acc == nil {
			break
		}
		acc.mu.Lock()
		token := acc.creds.Token
		userID := acc.creds.Email
		proxy := acc.proxy(e.proxy)
		acc.mu.Unlock()
		res, err := workbuddy.Stream(ctx, proxy, e.baseURL, token, userID, cReq, onDelta)
		if err == nil {
			return &model.ChatResult{
				Text:   res.Text,
				Engine: e.name,
				Model:  req.Model,
				Usage:  subResultUsage(res.PromptTokens, res.CompletionTokens, 0),
			}, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if workbuddy.IsUnauthorized(err) {
			e.markCooldown(acc, "token 失效，请重新授权", 30*time.Minute)
			acc.mu.Lock()
			acc.lastErr = "token 失效，请重新授权"
			acc.mu.Unlock()
			continue
		}
		if workbuddy.IsRateLimited(err) {
			e.markCooldown(acc, "上游限流", 3*time.Minute)
			continue
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("workbuddy: %w", lastErr)
	}
	return nil, errors.New("workbuddy 没有可用账号")
}

func (e *WorkBuddyEngine) usableCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := 0
	for _, acc := range e.accounts {
		acc.mu.Lock()
		if acc.enabled && acc.ready && !acc.tokenExpired() {
			n++
		}
		acc.mu.Unlock()
	}
	if n == 0 {
		return 1
	}
	return n
}

// Chat 非流式对话。
func (e *WorkBuddyEngine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, nil)
}

// ChatStream 流式对话。
func (e *WorkBuddyEngine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, func(delta string) error {
		if onDelta != nil {
			return onDelta(delta)
		}
		return nil
	})
}

func (e *WorkBuddyEngine) setErr(err error) {
	e.errMu.Lock()
	e.lastErr = err
	e.errMu.Unlock()
}

// Status 引擎运行状态。
func (e *WorkBuddyEngine) Status() map[string]any {
	accounts := e.AccountStates()
	ready := 0
	for _, st := range accounts {
		if st.Ready {
			ready++
		}
	}
	out := map[string]any{
		"name":        e.name,
		"describe":    e.Describe(),
		"ready":       ready > 0,
		"ready_count": ready,
		"total":       len(accounts),
		"accounts":    accounts,
	}
	e.errMu.Lock()
	if e.lastErr != nil {
		out["error"] = e.lastErr.Error()
	}
	e.errMu.Unlock()
	return out
}

// Passthrough workbuddy 不支持多模态端点透传。
func (e *WorkBuddyEngine) Passthrough() bool { return false }

func (e *WorkBuddyEngine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("workbuddy 引擎不支持透传")
}

// Close 关闭引擎。
func (e *WorkBuddyEngine) Close() {}
