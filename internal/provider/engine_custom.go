// engine_custom.go 第三方 Key 引擎：
// 管理台填写 base_url + api_key，按 OpenAI 兼容协议转发 /v1/chat/completions。
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
	"web2api/internal/engine/upstream"
	"web2api/internal/model"
	"web2api/internal/store"
	"web2api/internal/upstream/codex"
	"web2api/internal/upstream/custombalance"
)

// CustomCredentials 第三方 Key 凭据（accounts.credentials JSON）。
type CustomCredentials struct {
	BaseURL string   `json:"base_url"`         // OpenAI 兼容根地址，可带或不带 /v1
	APIKey  string   `json:"api_key"`          // 上游 API Key
	Models  []string `json:"models,omitempty"` // 可选静态模型列表（上游无 /v1/models 时用）
	Proxy   string   `json:"proxy,omitempty"`  // 账号级代理，优先于引擎全局代理
}

// customAccount 池内账号：每个供应商一张卡（base_url + key）。
type customAccount struct {
	id, label string
	mu        sync.Mutex
	creds     CustomCredentials
	enabled   bool
	ready     bool
	lastErr   string
	up        *upstream.OpenAI
	models    []model.ModelInfo

	provider      string    // DeepSeek / Sub2API / OpenRouter …
	quota         string    // 余额展示文案
	balanceErr    string    // 最近一次余额查询失败原因（有额度时清空）
	lastBalanceAt time.Time // 上次成功/尝试查询时间

	cooldownUntil  time.Time
	cooldownReason string
}

func (a *customAccount) proxy(engineProxy string) string {
	if strings.TrimSpace(a.creds.Proxy) != "" {
		return a.creds.Proxy
	}
	return engineProxy
}

// CustomEngine 第三方 OpenAI 兼容 Key 号池。
type CustomEngine struct {
	name  string
	proxy string
	st    *store.Store

	mu       sync.RWMutex
	accounts []*customAccount
	ids      map[string]int
	next     atomic.Uint64

	errMu   sync.Mutex
	lastErr error
}

// NewCustom 构建第三方 Key 引擎。
func NewCustom(cfg config.CustomConfig, st *store.Store) *CustomEngine {
	return &CustomEngine{
		name:  "custom",
		proxy: strings.TrimSpace(cfg.Proxy),
		st:    st,
		ids:   map[string]int{},
	}
}

func (e *CustomEngine) buildUpstream(creds CustomCredentials) (*upstream.OpenAI, error) {
	baseURL := upstream.NormalizeBaseURL(creds.BaseURL)
	if baseURL == "" {
		return nil, errors.New("base_url 不能为空")
	}
	if strings.TrimSpace(creds.APIKey) == "" {
		return nil, errors.New("api_key 不能为空")
	}
	proxy := strings.TrimSpace(creds.Proxy)
	if proxy == "" {
		proxy = e.proxy
	}
	client, err := codex.NewProxyHTTPClient(proxy, 300*time.Second)
	if err != nil {
		return nil, err
	}
	return upstream.NewWithClient("custom", baseURL, strings.TrimSpace(creds.APIKey), false, client), nil
}

func staticModels(ids []string, engineName string, available bool) []model.ModelInfo {
	out := make([]model.ModelInfo, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, model.ModelInfo{
			ID:          id,
			Object:      "model",
			DisplayName: id,
			Available:   available,
			Engine:      engineName,
			Kind:        model.ClassifyKind(id, id, nil),
		})
	}
	return out
}

// AddAccount 添加第三方供应商账号。
func (e *CustomEngine) AddAccount(id, label string, creds CustomCredentials) error {
	up, err := e.buildUpstream(creds)
	if err != nil {
		return err
	}
	e.mu.Lock()
	if _, exists := e.ids[id]; exists {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 已在池中", id)
	}
	acc := &customAccount{
		id:      id,
		label:   label,
		creds:   creds,
		enabled: true,
		ready:   true,
		up:      up,
		models:  staticModels(creds.Models, e.name, true),
	}
	e.accounts = append(e.accounts, acc)
	e.ids[id] = len(e.accounts) - 1
	e.mu.Unlock()

	// 异步探测模型列表与余额。
	go e.refreshAccount(context.Background(), acc)
	return nil
}

func (e *CustomEngine) refreshAccount(ctx context.Context, acc *customAccount) {
	acc.mu.Lock()
	up := acc.up
	static := append([]string{}, acc.creds.Models...)
	acc.mu.Unlock()
	if up == nil {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	fetched := up.RefreshModels(probeCtx)
	acc.mu.Lock()
	if len(fetched) > 0 {
		out := make([]model.ModelInfo, 0, len(fetched))
		for _, m := range fetched {
			m.Engine = e.name
			if m.Kind == "" {
				m.Kind = model.ClassifyKind(m.ID, m.DisplayName, nil)
			}
			m.Available = acc.enabled
			out = append(out, m)
		}
		acc.models = out
		acc.ready = true
		acc.lastErr = ""
	} else if len(static) > 0 {
		acc.models = staticModels(static, e.name, acc.enabled)
		acc.ready = true
		if err := up.ReadyErr(); err != "" {
			acc.lastErr = "模型列表探测失败，已使用手动模型: " + err
		}
	} else {
		// 无模型目录也可直调（用户自填 model）；标为 ready，细节记错误。
		acc.ready = true
		if err := up.ReadyErr(); err != "" {
			acc.lastErr = "未拉到模型列表（可手动填模型名）: " + err
		}
	}
	acc.mu.Unlock()
	e.refreshBalance(acc)
}

// refreshBalance 查询余额（best-effort）：先官方供应商，再 Sub2API /v1/usage → /user/balance → NewAPI。
func (e *CustomEngine) refreshBalance(acc *customAccount) {
	acc.mu.Lock()
	baseURL := acc.creds.BaseURL
	apiKey := acc.creds.APIKey
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()

	client, err := codex.NewProxyHTTPClient(proxy, 15*time.Second)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := custombalance.Query(ctx, client, baseURL, apiKey)
	acc.mu.Lock()
	acc.lastBalanceAt = time.Now()
	if err != nil {
		acc.balanceErr = err.Error()
		// 保留上次成功余额；无额度时把简短失败原因挂到 detail
		if acc.quota == "" && acc.lastErr == "" {
			msg := err.Error()
			if len(msg) > 120 {
				msg = msg[:120] + "…"
			}
			acc.lastErr = "额度未查到: " + msg
		}
		acc.mu.Unlock()
		return
	}
	acc.provider = res.Provider
	acc.quota = res.Summary
	acc.balanceErr = ""
	if strings.HasPrefix(acc.lastErr, "额度未查到:") {
		acc.lastErr = ""
	}
	acc.mu.Unlock()
}

// RemoveAccount 移除账号。
func (e *CustomEngine) RemoveAccount(id string) error {
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

// SetEnabled 启停账号。
func (e *CustomEngine) SetEnabled(id string, enabled bool) error {
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

// ReplaceCredentials 更新凭据（重建上游客户端）。
func (e *CustomEngine) ReplaceCredentials(id string, creds CustomCredentials) error {
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

// AccountStates 管理台账号状态。
func (e *CustomEngine) AccountStates() []AccountState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]AccountState, 0, len(e.accounts))
	for _, acc := range e.accounts {
		acc.mu.Lock()
		tier := acc.provider
		if tier == "" {
			tier = upstream.NormalizeBaseURL(acc.creds.BaseURL)
		}
		state := AccountState{
			ID:      acc.id,
			Label:   acc.label,
			Enabled: acc.enabled,
			Detail:  acc.lastErr,
			Tier:    tier,
			Quota:   acc.quota,
			Models:  len(acc.models),
		}
		switch {
		case !acc.enabled:
			state.Status = "disabled"
		case time.Now().Before(acc.cooldownUntil):
			state.Status = "error"
			state.Detail = "冷却中: " + acc.cooldownReason
		case !acc.ready:
			state.Status = "initializing"
		default:
			state.Status = "ok"
			state.Ready = true
		}
		// 余额超过 10 分钟未刷新则后台再查一次
		stale := acc.enabled && (acc.lastBalanceAt.IsZero() || time.Since(acc.lastBalanceAt) > 10*time.Minute)
		acc.mu.Unlock()
		if stale {
			go e.refreshBalance(acc)
		}
		out = append(out, state)
	}
	return out
}

// ---- Engine 接口 ----

func (e *CustomEngine) Name() string { return e.name }

func (e *CustomEngine) Describe() string {
	return "第三方 OpenAI 兼容 Key（base_url + api_key，常见供应商余额查询）"
}

func (e *CustomEngine) Ready() bool {
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

// ListModels 合并各供应商模型目录（去重）。
func (e *CustomEngine) ListModels() []model.ModelInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	seen := map[string]model.ModelInfo{}
	order := make([]string, 0)
	for _, acc := range e.accounts {
		acc.mu.Lock()
		enabled := acc.enabled && time.Now().After(acc.cooldownUntil)
		for _, m := range acc.models {
			id := m.ID
			if id == "" {
				continue
			}
			cp := m
			cp.Engine = e.name
			cp.Available = enabled
			if prev, ok := seen[id]; ok {
				cp.Available = prev.Available || cp.Available
			} else {
				order = append(order, id)
			}
			seen[id] = cp
		}
		acc.mu.Unlock()
	}
	out := make([]model.ModelInfo, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	return out
}

func (e *CustomEngine) pick(modelName string) *customAccount {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := len(e.accounts)
	if n == 0 {
		return nil
	}
	want := strings.TrimSpace(modelName)
	start := int(e.next.Add(1))

	// 优先选声明了该模型的账号。
	if want != "" {
		bare := strings.TrimPrefix(want, "models/")
		for i := 0; i < n; i++ {
			acc := e.accounts[(start+i)%n]
			acc.mu.Lock()
			ok := acc.enabled && acc.ready && time.Now().After(acc.cooldownUntil)
			has := len(acc.models) == 0 // 无目录时任意模型都可试
			for _, m := range acc.models {
				// 目录 id 可能带 models/ 前缀（Google OpenAI 兼容端点），宽松匹配。
				if strings.EqualFold(strings.TrimPrefix(m.ID, "models/"), bare) {
					has = true
					break
				}
			}
			acc.mu.Unlock()
			if ok && has {
				return acc
			}
		}
	}
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

func (e *CustomEngine) markCooldown(acc *customAccount, reason string, d time.Duration) {
	acc.mu.Lock()
	acc.cooldownUntil = time.Now().Add(d)
	acc.cooldownReason = reason
	acc.lastErr = reason
	acc.mu.Unlock()
}

func (e *CustomEngine) usableCount() int {
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

// ServeModelPassthrough 按模型把 OpenAI 兼容端点请求原样转发到拥有该模型的账号上游
// （/v1/embeddings、/v1/images/generations 等）。与 ServePassthrough 的区别：先解析
// 模型归属的账号再转发；解析失败（引擎不可用/无可用账号）时返回错误且未写出响应。
func (e *CustomEngine) ServeModelPassthrough(ctx context.Context, modelID, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	up, err := e.upstreamFor(modelID)
	if err != nil {
		return err
	}
	return up.ServePassthrough(ctx, method, path, body, contentType, w)
}

// ProbeImage 用账号上游探测图片模型（/v1/images/generations）。
func (e *CustomEngine) ProbeImage(ctx context.Context, modelID string) (string, error) {
	up, err := e.upstreamFor(modelID)
	if err != nil {
		return "", err
	}
	return up.ProbeImage(ctx, modelID)
}

// ProbeEmbedding 用账号上游探测向量模型（/v1/embeddings）。
func (e *CustomEngine) ProbeEmbedding(ctx context.Context, modelID string) (string, error) {
	up, err := e.upstreamFor(modelID)
	if err != nil {
		return "", err
	}
	return up.ProbeEmbedding(ctx, modelID)
}

func (e *CustomEngine) upstreamFor(modelID string) (*upstream.OpenAI, error) {
	if !e.Ready() {
		return nil, engineUnavailableError(e)
	}
	acc := e.pick(modelID)
	if acc == nil {
		return nil, errors.New("没有可用的第三方账号")
	}
	acc.mu.Lock()
	up := acc.up
	acc.mu.Unlock()
	if up == nil {
		return nil, errors.New("上游客户端未初始化")
	}
	return up, nil
}

func (e *CustomEngine) chatLoop(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	if !e.Ready() {
		return nil, engineUnavailableError(e)
	}
	var lastErr error
	for attempt := 0; attempt < e.usableCount(); attempt++ {
		acc := e.pick(req.Model)
		if acc == nil {
			break
		}
		acc.mu.Lock()
		up := acc.up
		acc.mu.Unlock()
		if up == nil {
			lastErr = errors.New("上游客户端未初始化")
			continue
		}
		var (
			res *model.ChatResult
			err error
		)
		if onDelta != nil {
			res, err = up.ChatStream(ctx, req, onDelta)
		} else {
			res, err = up.Chat(ctx, req)
		}
		if err == nil {
			if res != nil {
				res.Engine = e.name
			}
			return res, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "http 401") || strings.Contains(msg, "http 403") || strings.Contains(msg, "unauthorized"):
			e.markCooldown(acc, "Key 无效或无权限", 30*time.Minute)
		case strings.Contains(msg, "http 429") || strings.Contains(msg, "rate"):
			e.markCooldown(acc, "上游限流", 2*time.Minute)
		default:
			// 模型不存在等业务错误不冷却，直接返回给客户端。
			acc.mu.Lock()
			acc.lastErr = err.Error()
			acc.mu.Unlock()
			return nil, fmt.Errorf("custom: %w", err)
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("custom: %w", lastErr)
	}
	return nil, errors.New("custom 没有可用账号")
}

func (e *CustomEngine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, nil)
}

func (e *CustomEngine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, onDelta)
}

func (e *CustomEngine) Status() map[string]any {
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

func (e *CustomEngine) Passthrough() bool { return false }

func (e *CustomEngine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("custom 引擎不支持透传")
}

func (e *CustomEngine) Close() {}

// ProbeCustomModels 管理台「获取模型列表」：用临时凭据探测上游 /v1/models。
func ProbeCustomModels(ctx context.Context, engineProxy string, creds CustomCredentials) ([]string, error) {
	baseURL := upstream.NormalizeBaseURL(creds.BaseURL)
	if baseURL == "" {
		return nil, errors.New("base_url 不能为空")
	}
	if strings.TrimSpace(creds.APIKey) == "" {
		return nil, errors.New("api_key 不能为空")
	}
	proxy := strings.TrimSpace(creds.Proxy)
	if proxy == "" {
		proxy = strings.TrimSpace(engineProxy)
	}
	client, err := codex.NewProxyHTTPClient(proxy, 20*time.Second)
	if err != nil {
		return nil, err
	}
	up := upstream.NewWithClient("custom-probe", baseURL, strings.TrimSpace(creds.APIKey), false, client)
	models := up.RefreshModels(ctx)
	if len(models) == 0 {
		if err := up.ReadyErr(); err != "" {
			return nil, errors.New(err)
		}
		return nil, errors.New("上游未返回模型列表")
	}
	out := make([]string, 0, len(models))
	for _, m := range models {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out, nil
}
