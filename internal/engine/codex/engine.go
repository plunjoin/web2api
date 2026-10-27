// engine_go 引擎 codex：OpenAI Codex（ChatGPT 订阅号）账号池。
// 凭据为 OAuth refresh_token，运行时自动刷新 access_token，401 自动重试。
package codex

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
)

// Credentials codex 引擎凭据（accounts.credentials JSON）。
type Credentials struct {
	RefreshToken     string `json:"refresh_token"`
	Email            string `json:"email,omitempty"`
	ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
	Proxy            string `json:"proxy,omitempty"`
}

// codexAccount 池内账号。
type codexAccount struct {
	id, label string
	mu        sync.Mutex
	creds     Credentials
	enabled   bool

	accessToken string
	expiresAt   time.Time
	ready       bool
	lastErr     string

	quota           string // 5 小时窗用量百分比描述
	cooldownUntil  time.Time
	cooldownReason string
}

func (a *codexAccount) proxy(engineProxy string) string {
	if strings.TrimSpace(a.creds.Proxy) != "" {
		return a.creds.Proxy
	}
	return engineProxy
}

// Engine codex 引擎。
type Engine struct {
	name  string
	proxy string
	st    *store.Store

	mu       sync.RWMutex
	accounts []*codexAccount
	ids      map[string]int
	next     atomic.Uint64

	errMu   sync.Mutex
	lastErr error
}

// NewCodex 构建 codex 引擎（账号通过 AddAccount 动态注入）。
func New(cfg config.CodexConfig, st *store.Store) *Engine {
	return &Engine{
		name:  "codex",
		proxy: strings.TrimSpace(cfg.Proxy),
		st:    st,
		ids:   map[string]int{},
	}
}

// AddAccount 添加账号并异步首刷 token。
func (e *Engine) AddAccount(id, label string, creds Credentials) error {
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return errors.New("refresh_token 不能为空")
	}
	e.mu.Lock()
	if _, exists := e.ids[id]; exists {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 已在池中", id)
	}
	acc := &codexAccount{id: id, label: label, creds: creds, enabled: true}
	e.accounts = append(e.accounts, acc)
	e.ids[id] = len(e.accounts) - 1
	e.mu.Unlock()
	go e.ensureToken(acc)
	return nil
}

// RemoveAccount 移除账号。
func (e *Engine) RemoveAccount(id string) error {
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
		state := engine.AccountState{
			ID:      acc.id,
			Label:   acc.label,
			Enabled: acc.enabled,
			Detail:  acc.lastErr,
			Tier:    acc.creds.Email,
			Quota:   acc.quota,
			Models:  len(DefaultModels),
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

// ---- token 生命周期 ----

// ensureToken 刷新账号 token（加锁防并发），成功回写池内状态。
func (e *Engine) ensureToken(acc *codexAccount) error {
	acc.mu.Lock()
	refreshToken := acc.creds.RefreshToken
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	tok, err := Refresh(ctx, proxy, refreshToken)
	acc.mu.Lock()
	defer acc.mu.Unlock()
	if err != nil {
		acc.ready = false
		acc.lastErr = "token 刷新失败: " + err.Error()
		e.setErr(fmt.Errorf("codex 账号 %s token 刷新失败: %w", acc.label, err))
		return err
	}
	acc.accessToken = tok.AccessToken
	acc.expiresAt = tok.ExpiresAt
	if tok.RefreshToken != "" {
		acc.creds.RefreshToken = tok.RefreshToken
	}
	if tok.ChatGPTAccount != "" {
		acc.creds.ChatGPTAccountID = tok.ChatGPTAccount
	}
	if tok.Email != "" && acc.creds.Email == "" {
		acc.creds.Email = tok.Email
	}
	acc.ready = true
	acc.lastErr = ""
	// 回写凭据（refresh_token 轮换），不动账号状态
	if e.st != nil {
		if dbID, err := strconv.ParseInt(acc.id, 10, 64); err == nil {
			_ = e.st.UpdateAccountRawCredentials(dbID, engine.MarshalCompact(acc.creds))
		}
	}
	e.setErr(nil)
	return nil
}

// token 返回有效 access_token；过期（提前 3 分钟）时刷新。
func (e *Engine) token(acc *codexAccount) (string, string, error) {
	acc.mu.Lock()
	needRefresh := acc.accessToken == "" || (acc.expiresAt.After(time.Time{}) && time.Until(acc.expiresAt) < 3*time.Minute)
	tok := acc.accessToken
	acc.mu.Unlock()
	if needRefresh {
		if err := e.ensureToken(acc); err != nil {
			return "", "", err
		}
		acc.mu.Lock()
		tok = acc.accessToken
		acc.mu.Unlock()
	}
	if tok == "" {
		return "", "", errors.New("账号没有可用 token")
	}
	acc.mu.Lock()
	accountID := acc.creds.ChatGPTAccountID
	acc.mu.Unlock()
	return tok, accountID, nil
}

// ---- Engine 接口 ----

func (e *Engine) Name() string { return e.name }

func (e *Engine) Describe() string {
	return "OpenAI Codex（ChatGPT 订阅号池，Responses 协议）"
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

// ListModels codex 模型目录。
func (e *Engine) ListModels() []model.ModelInfo {
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
	out := make([]model.ModelInfo, 0, len(DefaultModels))
	for _, id := range DefaultModels {
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
func (e *Engine) pick() *codexAccount {
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

func (e *Engine) markCooldown(acc *codexAccount, reason string, d time.Duration) {
	acc.mu.Lock()
	acc.cooldownUntil = time.Now().Add(d)
	acc.cooldownReason = reason
	acc.mu.Unlock()
}

// toCodexRequest ChatRequest → Request。
func toCodexRequest(req model.ChatRequest) *Request {
	out := &Request{
		Model:      req.Model,
		MaxTokens:  engine.DerefInt(req.MaxTokens),
		Tools:      toCodexTools(req.Tools),
		ToolChoice: req.ToolChoice,
	}
	for _, m := range req.Messages {
		msg := Message{
			Role:       m.Role,
			Text:       m.Text(),
			ImageURLs:  engine.SubImageURLs(m.Content),
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, call := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID:        call.ID,
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			})
		}
		out.Messages = append(out.Messages, msg)
	}
	return out
}

// toCodexTools 把 OpenAI 工具定义转成 Responses 格式（name 等字段平铺）。
func toCodexTools(tools []model.Tool) []Tool {
	out := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if tool.Function == nil || strings.TrimSpace(tool.Function.Name) == "" {
			continue
		}
		out = append(out, Tool{
			Type:        "function",
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}
	return out
}

// chatOnce 单账号一次请求（401 时刷新重试一次）。
func (e *Engine) chatOnce(ctx context.Context, acc *codexAccount, req *Request, onDelta func(string) error, onTool func(ToolCallDelta) error) (*Result, error) {
	attempt := func() (*Result, error) {
		accessToken, accountID, err := e.token(acc)
		if err != nil {
			return nil, err
		}
		return StreamWithTools(ctx, acc.proxy(e.proxy), accessToken, accountID, req, onDelta, onTool)
	}
	res, err := attempt()
	if err != nil && IsUnauthorized(err) {
		// token 失效：强制刷新后重试一次
		if refreshErr := e.ensureToken(acc); refreshErr != nil {
			return nil, fmt.Errorf("token 已失效且刷新失败: %w", refreshErr)
		}
		res, err = attempt()
	}
	if err != nil && IsRateLimited(err) {
		e.markCooldown(acc, "上游限流", 5*time.Minute)
	}
	if err == nil && res != nil && res.PrimaryUsedPercent > 0 {
		acc.mu.Lock()
		if res.PrimaryResetAfter > 0 {
			hours := res.PrimaryResetAfter / 3600
			minutes := (res.PrimaryResetAfter % 3600) / 60
			acc.quota = fmt.Sprintf("已用 %.0f%%（%d小时%d分后重置）", res.PrimaryUsedPercent, hours, minutes)
		} else {
			acc.quota = fmt.Sprintf("已用 %.0f%%", res.PrimaryUsedPercent)
		}
		acc.mu.Unlock()
	}
	return res, err
}

// chatLoop 轮询账号直到成功或池耗尽。
func (e *Engine) chatLoop(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	if !e.Ready() {
		return nil, engine.UnavailableError(e)
	}
	if sink == nil {
		sink = model.TextSink(nil)
	}
	cReq := toCodexRequest(req)
	var lastErr error
	for attempt := 0; attempt < e.usableCount(); attempt++ {
		acc := e.pick()
		if acc == nil {
			break
		}
		res, err := e.chatOnce(ctx, acc, cReq, sink.Text, func(delta ToolCallDelta) error {
			return sink.ToolCall(model.ToolCallDelta{
				Index: delta.Index,
				ID:    delta.ID,
				Type:  "function",
				Function: model.ToolFunctionDelta{
					Name:      delta.Name,
					Arguments: delta.Arguments,
				},
			})
		})
		if err == nil {
			result := &model.ChatResult{
				Text:   res.Text,
				Engine: e.name,
				Model:  req.Model,
				Usage:  engine.SubResultUsage(res.PromptTokens, res.CompletionTokens, res.ReasoningTokens),
			}
			for _, call := range res.ToolCalls {
				result.ToolCalls = append(result.ToolCalls, model.ToolCall{
					ID:   call.ID,
					Type: "function",
					Function: model.ToolFunction{
						Name:      call.Name,
						Arguments: call.Arguments,
					},
				})
			}
			if len(result.ToolCalls) > 0 {
				result.FinishReason = "tool_calls"
			}
			return result, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if strings.Contains(err.Error(), "不在池中") {
			return nil, err
		}
		// 401 刷新已失败 → 账号短暂冷却，换下一个
		if IsUnauthorized(err) || strings.Contains(err.Error(), "刷新失败") {
			e.markCooldown(acc, "凭据失效", 10*time.Minute)
			continue
		}
		// 请求级错误：换号重试
	}
	if lastErr != nil {
		return nil, fmt.Errorf("codex: %w", lastErr)
	}
	return nil, errors.New("codex 没有可用账号")
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
	if n == 0 {
		return 1 // 保证至少尝试一次以拿到具体错误
	}
	return n
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

// Passthrough codex 不支持多模态端点透传。
func (e *Engine) Passthrough() bool { return false }

func (e *Engine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("codex 引擎不支持透传")
}

// Close 关闭引擎（账号无后台协程，无需清理）。
func (e *Engine) Close() {}
