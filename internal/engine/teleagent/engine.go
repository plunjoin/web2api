// engine.go 实现 teleagent 账号池引擎：凭据为 TeleAgent 桌面端登录态 JWT，
// 无 refresh token，请求经两步 HMAC 签名发往上游聊天网关。
package teleagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/engine/codex"
	"web2api/internal/model"
	"web2api/internal/store"
)

// teleagentAccount 池内账号。
type teleagentAccount struct {
	id, label string
	mu        sync.Mutex
	creds     Credentials
	enabled   bool
	ready     bool
	lastErr   string
	quota     string
	expiresAt time.Time

	cooldownUntil  time.Time
	cooldownReason string

	http *http.Client
}

// Engine teleagent 引擎。
type Engine struct {
	name  string
	proxy string
	st    *store.Store

	mu       sync.RWMutex
	accounts []*teleagentAccount
	ids      map[string]int
	next     atomic.Uint64
	order    engine.AccountOrder

	errMu   sync.Mutex
	lastErr error

	done      chan struct{}
	closeOnce sync.Once
}

// SetAccountOrder 实现 engine.AccountOrderer。
func (e *Engine) SetAccountOrder(ids []string) { e.order.Set(ids) }

// New 构建 teleagent 引擎（账号通过 AddAccount 动态注入）。
// 启动后台维护循环：每 30 分钟刷新积分余额；每日签到由上游
// claimed_today 判定天然幂等，未领取时自动补签。
func New(cfg config.TeleAgentConfig, st *store.Store) *Engine {
	e := &Engine{
		name:  "teleagent",
		proxy: strings.TrimSpace(cfg.Proxy),
		st:    st,
		ids:   map[string]int{},
		done:  make(chan struct{}),
	}
	e.startMaintenance()
	return e
}

// AddAccount 添加账号。凭据里的 token 就是登录态本身，无 refresh 可刷新。
func (e *Engine) AddAccount(id, label string, creds Credentials) error {
	if strings.TrimSpace(creds.Token) == "" {
		return errors.New("token 不能为空")
	}
	client, err := codex.NewStreamHTTPClient(creds.proxy(e.proxy), 300*time.Second)
	if err != nil {
		return err
	}
	accountName, expiresAt := parseTokenClaims(creds.Token)
	if label == "" || label == id {
		if accountName != "" {
			label = accountName
		}
	}
	if label == "" {
		label = "teleagent-" + creds.Scope
	}

	e.mu.Lock()
	if _, exists := e.ids[id]; exists {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 已在池中", id)
	}
	acc := &teleagentAccount{
		id:        id,
		label:     label,
		creds:     creds,
		enabled:   true,
		ready:     true,
		expiresAt: expiresAt,
		http:      client,
	}
	if !expiresAt.IsZero() && time.Now().After(expiresAt) {
		acc.ready = false
		acc.lastErr = "登录已过期，请回 TeleAgent 桌面端重新登录后重新导入"
	}
	e.accounts = append(e.accounts, acc)
	e.ids[id] = len(e.accounts) - 1
	e.mu.Unlock()
	// 入池即异步刷新一次积分余额并尝试补签今日签到。
	go e.refreshAccount(acc)
	return nil
}

// RemoveAccount 移除账号。
func (e *Engine) RemoveAccount(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	idx, ok := e.ids[id]
	if !ok {
		return fmt.Errorf("账号 %s 不在池中", id)
	}
	e.accounts = append(e.accounts[:idx], e.accounts[idx+1:]...)
	delete(e.ids, id)
	for k, v := range e.ids {
		if v > idx {
			e.ids[k] = v - 1
		}
	}
	return nil
}

// SetAutoCheckIn 开关自动签到（持久化到凭据，重启后保留）。
func (e *Engine) SetAutoCheckIn(id string, enabled bool) error {
	e.mu.Lock()
	idx, ok := e.ids[id]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 不在池中", id)
	}
	acc := e.accounts[idx]
	e.mu.Unlock()
	acc.mu.Lock()
	acc.creds.AutoCheckIn = &enabled
	acc.mu.Unlock()
	e.persist(acc)
	// 开关状态变化后刷新一次展示文案：关闭时不补签，打开时立即补签今日。
	go e.refreshAccount(acc)
	return nil
}

// CheckIn 立即签到一次，返回结果文案。幂等：今日已签到时直接返回已签到状态。
func (e *Engine) CheckIn(id string) (string, error) {
	e.mu.RLock()
	idx, ok := e.ids[id]
	if !ok {
		e.mu.RUnlock()
		return "", fmt.Errorf("账号 %s 不在池中", id)
	}
	acc := e.accounts[idx]
	e.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	acc.mu.Lock()
	creds, client := acc.creds, acc.http
	acc.mu.Unlock()

	st, err := dailyTaskStatus(ctx, client, creds)
	if err != nil {
		return "", err
	}
	if !st.Visible {
		return "今日签到活动未开放", nil
	}
	if st.Claimed {
		return fmt.Sprintf("今日已签到（+%.0f）", st.Claimable), nil
	}
	if err := claimDailyTask(ctx, client, creds); err != nil {
		return "", err
	}
	// 刷新展示文案（积分 + 签到状态）。
	go e.refreshAccount(acc)
	return fmt.Sprintf("签到成功（+%.0f）", st.Claimable), nil
}

// SetEnabled 启停账号。
func (e *Engine) SetEnabled(id string, enabled bool) error {
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
func (e *Engine) ReplaceCredentials(id string, creds Credentials) error {
	label, err := e.labelOf(id)
	if err != nil {
		return err
	}
	if err := e.RemoveAccount(id); err != nil {
		return err
	}
	return e.AddAccount(id, label, creds)
}

func (e *Engine) labelOf(id string) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	idx, ok := e.ids[id]
	if !ok {
		return "", fmt.Errorf("账号 %s 不在池中", id)
	}
	return e.accounts[idx].label, nil
}

// AccountStates 全部账号状态（管理台）。
func (e *Engine) AccountStates() []engine.AccountState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]engine.AccountState, 0, len(e.accounts))
	for _, acc := range e.accounts {
		acc.mu.Lock()
		autoCheckIn := acc.creds.AutoCheckInEnabled()
		state := engine.AccountState{
			ID:          acc.id,
			Label:       acc.label,
			Enabled:     acc.enabled,
			Detail:      acc.lastErr,
			Tier:        acc.creds.Email,
			Quota:       acc.quota,
			Models:      len(DefaultModels),
			AutoCheckIn: &autoCheckIn,
		}
		switch {
		case !acc.enabled:
			state.Status = "disabled"
		case time.Now().Before(acc.cooldownUntil):
			state.Status = "error"
			state.Detail = "冷却中: " + acc.cooldownReason
		case !acc.ready:
			state.Status = "error"
			if acc.lastErr == "" {
				state.Detail = "登录已过期或凭据不可用"
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

// ---- Engine 接口 ----

func (e *Engine) Name() string { return e.name }

func (e *Engine) Describe() string {
	return "TeleAgent（天翼 AI 星辰超级智能体，桌面端登录态复用）"
}

// Ready 至少一个账号可用。
func (e *Engine) Ready() bool {
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

// RefreshCatalog TeleAgent 没有模型列表接口，返回内置目录快照。
func (e *Engine) RefreshCatalog(context.Context) ([]model.ModelInfo, error) {
	if !e.hasEnabledAccount() {
		return nil, errors.New("teleagent 没有可用账号，无法获取模型")
	}
	return e.ListModels(), nil
}

func (e *Engine) hasEnabledAccount() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, acc := range e.accounts {
		acc.mu.Lock()
		ok := acc.enabled
		acc.mu.Unlock()
		if ok {
			return true
		}
	}
	return false
}

func (e *Engine) usableCount() int {
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
	return n
}

// candidates 返回本次请求可尝试的账号（按顺序）。
func (e *Engine) candidates(ctx context.Context) []*teleagentAccount {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := len(e.accounts)
	if n == 0 {
		return nil
	}
	start := 0
	if !e.order.Active() {
		start = int(e.next.Add(1))
	}
	now := time.Now()
	list := make([]*teleagentAccount, 0, n)
	for i := 0; i < n; i++ {
		acc := e.accounts[(start+i)%n]
		acc.mu.Lock()
		ok := acc.enabled && acc.ready && now.After(acc.cooldownUntil)
		acc.mu.Unlock()
		if ok {
			list = append(list, acc)
		}
	}
	e.order.Sort(list, func(acc *teleagentAccount) string { return acc.id })
	// 会话粘性：上次接单的账号排到最前，失败时仍可换下一个。
	return engine.PreferAccount(ctx, list, func(acc *teleagentAccount) string { return acc.id })
}

func (e *Engine) markCooldown(acc *teleagentAccount, reason string, d time.Duration) {
	acc.mu.Lock()
	acc.cooldownUntil = time.Now().Add(d)
	acc.cooldownReason = reason
	acc.mu.Unlock()
}

// chatOnce 单账号一次请求。
func (e *Engine) chatOnce(ctx context.Context, acc *teleagentAccount, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	acc.mu.Lock()
	creds := acc.creds
	client := acc.http
	acc.mu.Unlock()
	return chatStream(ctx, client, creds, req, sink)
}

// chatLoop 轮询账号直到成功或池耗尽。
func (e *Engine) chatLoop(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	if !isKnownModel(req.Model) {
		return nil, engine.NewModelNotFound(req.Model, fmt.Sprintf("teleagent 没有模型 %q（可用：chat-pro / chat-flagship / chat-flash / chat-lite / chat-nano）", req.Model))
	}
	if !e.Ready() {
		return nil, engine.UnavailableError(e)
	}
	if sink == nil {
		sink = model.TextSink(nil)
	}
	var lastErr error
	for _, acc := range e.candidates(ctx) {
		res, err := e.chatOnce(ctx, acc, req, sink)
		if err == nil {
			res.Engine = e.name
			engine.NoteServedAccount(ctx, acc.id)
			return res, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "40109") || strings.Contains(msg, "封禁") || strings.Contains(msg, "banned"):
			e.markCooldown(acc, "账号封禁", 10*time.Minute)
		case strings.Contains(msg, "40108") || strings.Contains(msg, "401") || strings.Contains(msg, "403"):
			e.markCooldown(acc, "凭据失效", 10*time.Minute)
		case strings.Contains(msg, "402") || strings.Contains(msg, "积分不足"):
			e.markCooldown(acc, "积分不足", 5*time.Minute)
		case strings.Contains(msg, "429") || strings.Contains(msg, "限流"):
			e.markCooldown(acc, "上游限流", 5*time.Minute)
		case strings.Contains(msg, "400") || strings.Contains(msg, "404") || strings.Contains(msg, "422"):
			// 请求级错误：不是账号健康问题，不换号重试。
			return nil, fmt.Errorf("teleagent: %w", err)
		default:
			// 网络/超时/5xx：换下一个账号。
			e.markCooldown(acc, "上游请求失败", time.Minute)
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("teleagent: %w", lastErr)
	}
	return nil, errors.New("teleagent 没有可用账号")
}

// Chat 非流式对话。
func (e *Engine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, nil)
}

// ChatStream 流式对话。
func (e *Engine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, model.TextSink(onDelta))
}

// ChatStreamSink 流式对话（支持工具调用增量）。
func (e *Engine) ChatStreamSink(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, sink)
}

func (e *Engine) setErr(err error) {
	e.errMu.Lock()
	e.lastErr = err
	e.errMu.Unlock()
}

// Status 引擎运行状态。
func (e *Engine) Status() map[string]any {
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

// Passthrough teleagent 不支持多模态端点透传。
func (e *Engine) Passthrough() bool { return false }

func (e *Engine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("teleagent 引擎不支持透传")
}

// Close 关闭引擎（停止后台维护循环）。
func (e *Engine) Close() {
	e.closeOnce.Do(func() { close(e.done) })
}

// ---- 后台维护：积分余额刷新 + 每日自动签到 ----

// startMaintenance 每 30 分钟对全部可用账号刷新一次积分并尝试签到。
func (e *Engine) startMaintenance() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		e.refreshAll()
		for {
			select {
			case <-e.done:
				return
			case <-ticker.C:
				e.refreshAll()
			}
		}
	}()
}

func (e *Engine) refreshAll() {
	e.mu.RLock()
	accs := make([]*teleagentAccount, len(e.accounts))
	copy(accs, e.accounts)
	e.mu.RUnlock()
	for _, acc := range accs {
		select {
		case <-e.done:
			return
		default:
		}
		acc.mu.Lock()
		ok := acc.enabled && acc.ready
		acc.mu.Unlock()
		if !ok {
			continue
		}
		e.refreshAccount(acc)
	}
}

// refreshAccount 刷新单个账号的积分余额，并在每日任务未领取时自动签到。
// 积分门户失败不影响聊天与账号 ready 状态，只写入展示文案。
func (e *Engine) refreshAccount(acc *teleagentAccount) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	acc.mu.Lock()
	creds, client := acc.creds, acc.http
	acc.mu.Unlock()

	checkIn := "签到状态未知"
	if st, err := dailyTaskStatus(ctx, client, creds); err == nil && st.Visible {
		switch {
		case st.Claimed:
			checkIn = fmt.Sprintf("今日已签到（+%.0f）", st.Claimable)
		case !creds.AutoCheckInEnabled():
			checkIn = fmt.Sprintf("未签到（可领 +%.0f，自动签到已关闭）", st.Claimable)
		default:
			if err := claimDailyTask(ctx, client, creds); err == nil {
				checkIn = fmt.Sprintf("今日已签到（+%.0f）", st.Claimable)
			} else {
				checkIn = "今日签到失败: " + truncateErr(err)
			}
		}
	}

	balance, _, err := pointsBalance(ctx, client, creds)
	acc.mu.Lock()
	if err != nil {
		acc.quota = "积分查询失败"
	} else {
		acc.quota = fmt.Sprintf("积分 %g · %s", balance, checkIn)
	}
	acc.mu.Unlock()
}

func truncateErr(err error) string {
	msg := err.Error()
	if len(msg) > 80 {
		msg = msg[:80] + "…"
	}
	return msg
}

// persist 把账号凭据写回数据库（自动签到开关等运行时字段），不改账号状态。
func (e *Engine) persist(acc *teleagentAccount) {
	if e.st == nil {
		return
	}
	id, err := strconv.ParseInt(acc.id, 10, 64)
	if err != nil {
		return
	}
	acc.mu.Lock()
	creds := engine.MarshalCompact(acc.creds)
	acc.mu.Unlock()
	_ = e.st.UpdateAccountRawCredentials(id, creds)
}

// ---- 凭据工具 ----

// proxy 返回账号级代理（优先）或引擎全局代理。
func (c Credentials) proxy(engineProxy string) string {
	if strings.TrimSpace(c.Proxy) != "" {
		return c.Proxy
	}
	return engineProxy
}

// AccountName 从 token 里取账号名（JWT accountName，通常是脱敏手机号）。失败返回空串。
func AccountName(token string) string {
	name, _ := parseTokenClaims(token)
	return name
}

// parseTokenClaims 从 `<tenant>:<JWT>` 里解析账号名与过期时间（不校验签名）。
func parseTokenClaims(token string) (accountName string, expiresAt time.Time) {
	jwt := token
	if i := strings.Index(token, ":"); i >= 0 {
		jwt = token[i+1:]
	}
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return "", time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return "", time.Time{}
	}
	var claims struct {
		AccountName string `json:"accountName"`
		Exp         int64  `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return "", time.Time{}
	}
	if claims.Exp > 0 {
		expiresAt = time.Unix(claims.Exp, 0)
	}
	return claims.AccountName, expiresAt
}
