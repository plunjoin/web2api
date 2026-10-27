// engine_kiro.go 引擎 kiro：AWS Kiro（CodeWhisperer 订阅号池）。
// 凭据为 Kiro IDE 的 refresh_token（旋转式，刷新后自动回写数据库）。
package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/model"
	"web2api/internal/store"
	"web2api/internal/engine/kiro"
)

// KiroCredentials kiro 引擎凭据（accounts.credentials JSON）。
type KiroCredentials struct {
	RefreshToken string `json:"refresh_token"`
	Email        string `json:"email,omitempty"`
	Proxy        string `json:"proxy,omitempty"`
}

// kiroAccount 池内账号。
type kiroAccount struct {
	id, label string
	mu        sync.Mutex
	creds     KiroCredentials
	enabled   bool

	accessToken  string
	refreshToken string // 运行时旋转值
	expiresAt    time.Time
	ready        bool
	lastErr      string
	email        string // 用量接口返回的账号邮箱
	subscription string // 订阅类型（Free/Pro）
	quota        string // 剩余额度描述（CREDIT）

	cooldownUntil  time.Time
	cooldownReason string
}

func (a *kiroAccount) proxy(engineProxy string) string {
	if strings.TrimSpace(a.creds.Proxy) != "" {
		return a.creds.Proxy
	}
	return engineProxy
}

// KiroEngine kiro 引擎。
type KiroEngine struct {
	name  string
	proxy string
	st    *store.Store

	mu       sync.RWMutex
	accounts []*kiroAccount
	ids      map[string]int
	next     atomic.Uint64

	errMu   sync.Mutex
	lastErr error
}

// NewKiro 构建 kiro 引擎。
func NewKiro(cfg config.KiroConfig, st *store.Store) *KiroEngine {
	return &KiroEngine{
		name:  "kiro",
		proxy: strings.TrimSpace(cfg.Proxy),
		st:    st,
		ids:   map[string]int{},
	}
}

// AddAccount 添加账号并异步首刷 token。
func (e *KiroEngine) AddAccount(id, label string, creds KiroCredentials) error {
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return errors.New("refresh_token 不能为空")
	}
	e.mu.Lock()
	if _, exists := e.ids[id]; exists {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 已在池中", id)
	}
	acc := &kiroAccount{id: id, label: label, creds: creds, enabled: true, refreshToken: creds.RefreshToken}
	e.accounts = append(e.accounts, acc)
	e.ids[id] = len(e.accounts) - 1
	e.mu.Unlock()
	go e.ensureToken(acc)
	return nil
}

// RemoveAccount 移除账号。
func (e *KiroEngine) RemoveAccount(id string) error {
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
func (e *KiroEngine) SetEnabled(id string, enabled bool) error {
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
func (e *KiroEngine) ReplaceCredentials(id string, creds KiroCredentials) error {
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
func (e *KiroEngine) AccountStates() []AccountState {
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
			Quota:   acc.quota,
			Models:  len(kiro.DefaultModels),
		}
		if acc.email != "" {
			state.Tier = acc.email
		}
		if acc.subscription != "" {
			state.Tier = acc.subscription
		}
		switch {
		case !acc.enabled:
			state.Status = "disabled"
		case time.Now().Before(acc.cooldownUntil):
			state.Status = "error"
			state.Detail = "冷却中: " + acc.cooldownReason
		case !acc.ready:
			state.Status = "initializing"
			if acc.lastErr == "" {
				state.Detail = "等待 token 刷新"
			}
		default:
			state.Status = "ok"
			state.Ready = true
		}
		acc.mu.Unlock()
		out = append(out, state)
	}
	return out
}

// ensureToken 刷新账号 token（Kiro refresh_token 旋转式，必须回写）。
func (e *KiroEngine) ensureToken(acc *kiroAccount) error {
	acc.mu.Lock()
	refreshToken := acc.refreshToken
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	tok, err := kiro.Refresh(ctx, proxy, refreshToken)
	acc.mu.Lock()
	if err != nil {
		acc.ready = false
		acc.lastErr = "token 刷新失败: " + err.Error()
		acc.mu.Unlock()
		e.setErr(fmt.Errorf("kiro 账号 %s token 刷新失败: %w", acc.label, err))
		return err
	}
	acc.accessToken = tok.AccessToken
	acc.refreshToken = tok.RefreshToken
	acc.expiresAt = tok.ExpiresAt
	acc.ready = true
	acc.lastErr = ""
	// 旋转式 refresh_token 回写数据库（不动账号状态）
	if e.st != nil && tok.RefreshToken != acc.creds.RefreshToken {
		acc.creds.RefreshToken = tok.RefreshToken
		if dbID, perr := strconv.ParseInt(acc.id, 10, 64); perr == nil {
			_ = e.st.UpdateAccountRawCredentials(dbID, marshalCompact(acc.creds))
		}
	}
	accessToken := acc.accessToken
	acc.mu.Unlock()
	e.setErr(nil)
	// 异步刷新用量（余额展示；失败不影响 ready）
	go e.refreshUsage(acc, proxy, accessToken)
	return nil
}

// refreshUsage 拉取账号用量与订阅信息（CREDIT 剩余额度）。
func (e *KiroEngine) refreshUsage(acc *kiroAccount, proxy, accessToken string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	limits, err := kiro.GetUsageLimits(ctx, proxy, accessToken)
	if err != nil {
		return // best-effort
	}
	remaining := limits.AvailableCredits()
	var quota string
	if remaining == float64(int64(remaining)) {
		quota = fmt.Sprintf("剩余 %.0f 次", remaining)
	} else {
		quota = fmt.Sprintf("剩余 %.1f 次", remaining)
	}
	sub := limits.SubscriptionInfo.SubscriptionTitle
	if sub == "" {
		sub = limits.SubscriptionInfo.Type
	}
	acc.mu.Lock()
	acc.quota = quota
	if limits.UserInfo.Email != "" {
		acc.email = limits.UserInfo.Email
		if acc.creds.Email == "" {
			acc.creds.Email = limits.UserInfo.Email
		}
	}
	if sub != "" {
		acc.subscription = sub
	}
	acc.mu.Unlock()
}

// token 返回有效 access_token（提前 5 分钟刷新）。
func (e *KiroEngine) token(acc *kiroAccount) (string, error) {
	acc.mu.Lock()
	needRefresh := acc.accessToken == "" || (acc.expiresAt.After(time.Time{}) && time.Until(acc.expiresAt) < 5*time.Minute)
	tok := acc.accessToken
	acc.mu.Unlock()
	if needRefresh {
		if err := e.ensureToken(acc); err != nil {
			return "", err
		}
		acc.mu.Lock()
		tok = acc.accessToken
		acc.mu.Unlock()
	}
	if tok == "" {
		return "", errors.New("账号没有可用 token")
	}
	return tok, nil
}

// ---- Engine 接口 ----

func (e *KiroEngine) Name() string { return e.name }

func (e *KiroEngine) Describe() string {
	return "AWS Kiro（CodeWhisperer 订阅号池，Claude 模型）"
}

// Ready 至少一个账号可用。
func (e *KiroEngine) Ready() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, acc := range e.accounts {
		acc.mu.Lock()
		ok := acc.enabled && acc.ready && time.Now().After(acc.cooldownUntil)
		acc.mu.Unlock()
		if ok {
			return true
		}
	}
	return false
}

// ListModels kiro 模型目录。
func (e *KiroEngine) ListModels() []model.ModelInfo {
	e.mu.RLock()
	enabled := 0
	for _, acc := range e.accounts {
		acc.mu.Lock()
		if acc.enabled {
			enabled++
		}
		acc.mu.Unlock()
	}
	e.mu.RUnlock()
	out := make([]model.ModelInfo, 0, len(kiro.DefaultModels))
	for _, id := range kiro.DefaultModels {
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
func (e *KiroEngine) pick() *kiroAccount {
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
		ok := acc.enabled && acc.ready && time.Now().After(acc.cooldownUntil)
		acc.mu.Unlock()
		if ok {
			return acc
		}
	}
	return nil
}

func (e *KiroEngine) markCooldown(acc *kiroAccount, reason string, d time.Duration) {
	acc.mu.Lock()
	acc.cooldownUntil = time.Now().Add(d)
	acc.cooldownReason = reason
	acc.mu.Unlock()
}

// toKiroRequest ChatRequest → kiro.Request。
func toKiroRequest(req model.ChatRequest) (*kiro.Request, error) {
	out := &kiro.Request{Model: req.Model}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, kiro.Message{Role: m.PlainRole(), Text: m.Text()})
	}
	return out, nil
}

// chatOnce 单账号一次请求（401/403 刷新重试一次）。
func (e *KiroEngine) chatOnce(ctx context.Context, acc *kiroAccount, req *kiro.Request, onDelta func(string) error) (*kiro.Result, error) {
	attempt := func() (*kiro.Result, error) {
		accessToken, err := e.token(acc)
		if err != nil {
			return nil, err
		}
		return kiro.Stream(ctx, acc.proxy(e.proxy), accessToken, req, onDelta)
	}
	res, err := attempt()
	if err != nil && kiro.IsSuspended(err) {
		// 账号被上游封停（安全锁定）：刷新 token 无效，长冷却并透出恢复方式
		e.markCooldown(acc, "账号被上游封停（安全锁定），需到 app.kiro.dev/account/usage 验证身份恢复", time.Hour)
		return nil, err
	}
	if err != nil && kiro.IsUnauthorized(err) {
		if refreshErr := e.ensureToken(acc); refreshErr != nil {
			return nil, fmt.Errorf("token 已失效且刷新失败: %w", refreshErr)
		}
		res, err = attempt()
	}
	if err != nil && kiro.IsRateLimited(err) {
		e.markCooldown(acc, "上游限流", 3*time.Minute)
	}
	return res, err
}

// chatLoop 轮询账号直到成功或池耗尽。
func (e *KiroEngine) chatLoop(ctx context.Context, req model.ChatRequest, onDelta func(string) error) (*model.ChatResult, error) {
	if !e.Ready() {
		return nil, engineUnavailableError(e)
	}
	cReq, err := toKiroRequest(req)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < e.usableCount(); attempt++ {
		acc := e.pick()
		if acc == nil {
			break
		}
		res, err := e.chatOnce(ctx, acc, cReq, onDelta)
		if err == nil {
			return &model.ChatResult{
				Text:   res.Text,
				Engine: e.name,
				Model:  req.Model,
				Usage:  nil, // CodeWhisperer 无 usage，网关按文本估算
			}, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if kiro.IsUnauthorized(err) || strings.Contains(err.Error(), "刷新失败") {
			e.markCooldown(acc, "凭据失效", 10*time.Minute)
			continue
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("kiro: %w", lastErr)
	}
	return nil, errors.New("kiro 没有可用账号")
}

func (e *KiroEngine) usableCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := 0
	for _, acc := range e.accounts {
		acc.mu.Lock()
		if acc.enabled && acc.ready {
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
func (e *KiroEngine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, nil)
}

// ChatStream 流式对话。
func (e *KiroEngine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, func(delta string) error {
		if onDelta != nil {
			return onDelta(delta)
		}
		return nil
	})
}

func (e *KiroEngine) setErr(err error) {
	e.errMu.Lock()
	e.lastErr = err
	e.errMu.Unlock()
}

// Status 引擎运行状态。
func (e *KiroEngine) Status() map[string]any {
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

// Passthrough kiro 不支持多模态端点透传。
func (e *KiroEngine) Passthrough() bool { return false }

func (e *KiroEngine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("kiro 引擎不支持透传")
}

// Close 关闭引擎。
func (e *KiroEngine) Close() {}
