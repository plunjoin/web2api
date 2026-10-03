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
	"web2api/internal/engine/geminiweb"
	"web2api/internal/model"
)

// geminiAccount 引擎A 池内账号。
type geminiAccount struct {
	id      string
	label   string
	client  *geminiweb.Client
	enabled bool
}

// GeminiWebEngine 引擎A：gemini.google.com 网页版原生逆向账号池（支持运行时增删启停）。
type GeminiWebEngine struct {
	name      string
	proxy     string
	refresh   time.Duration
	timeout   time.Duration
	cookieDir string

	mu       sync.RWMutex
	accounts []*geminiAccount // 保序
	ids      map[string]int   // id → accounts 下标
	next     atomic.Uint64

	errMu   sync.Mutex
	lastErr error
}

// NewGeminiWeb 构建引擎A（账号通过 AddAccount 动态注入）。
func NewGeminiWeb(cfg config.EngineAConfig) (*GeminiWebEngine, error) {
	// 允许测试、嵌入式调用和手写配置省略这些值；避免零值 duration
	// 让后台初始化立即超时或自动续期循环失效。
	if cfg.RefreshSeconds <= 0 {
		cfg.RefreshSeconds = 600
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 300
	}
	return &GeminiWebEngine{
		name:      "a",
		proxy:     cfg.Proxy,
		refresh:   time.Duration(cfg.RefreshSeconds) * time.Second,
		timeout:   time.Duration(cfg.TimeoutSeconds) * time.Second,
		cookieDir: cfg.CookiePath,
		ids:       map[string]int{},
	}, nil
}

// AddAccount 添加账号并异步初始化（Cookie 校验在后台完成）。
// id 为外部（数据库）主键；label 仅用于展示。
func (e *GeminiWebEngine) AddAccount(id, label, psid, psidts string) error {
	if strings.TrimSpace(psid) == "" {
		return errors.New("__Secure-1PSID 不能为空")
	}
	e.mu.Lock()
	if _, exists := e.ids[id]; exists {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 已在池中", id)
	}
	client := geminiweb.NewClient(label, psid, psidts, e.cookieDir, e.proxy, e.timeout)
	e.accounts = append(e.accounts, &geminiAccount{id: id, label: label, client: client, enabled: true})
	e.ids[id] = len(e.accounts) - 1
	e.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
		defer cancel()
		if err := client.Init(ctx); err != nil {
			e.setErr(fmt.Errorf("账号 %s(%s) 初始化失败: %w", label, id, err))
		} else {
			client.StartAutoRefresh(ctx, e.refresh)
			e.setErr(nil)
		}
	}()
	return nil
}

// RemoveAccount 移除账号。
func (e *GeminiWebEngine) RemoveAccount(id string) error {
	e.mu.Lock()
	idx, ok := e.ids[id]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 不在池中", id)
	}
	acc := e.accounts[idx]
	e.accounts = append(e.accounts[:idx], e.accounts[idx+1:]...)
	delete(e.ids, id)
	for k, v := range e.ids {
		if v > idx {
			e.ids[k] = v - 1
		}
	}
	e.mu.Unlock()
	acc.client.StopAutoRefresh()
	return nil
}

// SetEnabled 启停账号。
func (e *GeminiWebEngine) SetEnabled(id string, enabled bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	idx, ok := e.ids[id]
	if !ok {
		return fmt.Errorf("账号 %s 不在池中", id)
	}
	e.accounts[idx].enabled = enabled
	return nil
}

// ReplaceCredentials 更新凭据（重建客户端）。
func (e *GeminiWebEngine) ReplaceCredentials(id, psid, psidts string) error {
	e.mu.RLock()
	label := id
	if idx, ok := e.ids[id]; ok {
		label = e.accounts[idx].label
	}
	e.mu.RUnlock()
	if err := e.RemoveAccount(id); err != nil {
		return err
	}
	return e.AddAccount(id, label, psid, psidts)
}

// AccountState 账号实时状态（供管理台）。
type AccountState struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
	Ready   bool   `json:"ready"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Models  int    `json:"models"`
}

// AccountStates 全部账号状态。
func (e *GeminiWebEngine) AccountStates() []AccountState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]AccountState, 0, len(e.accounts))
	for _, acc := range e.accounts {
		state := AccountState{
			ID:      acc.id,
			Label:   acc.label,
			Enabled: acc.enabled,
			Models:  len(acc.client.Models()),
		}
		if !acc.enabled {
			state.Status = "disabled"
		} else if !acc.client.Ready() {
			state.Status = "initializing"
			state.Detail = "等待 Cookie 初始化"
		} else {
			switch acc.client.AccountStatus() {
			case geminiweb.StatusAvailable:
				state.Status = "ok"
				state.Ready = true
			case geminiweb.StatusUnauthenticated:
				state.Status = "error"
				state.Detail = "登录态失效，请重新提取 Cookie"
			case geminiweb.StatusTempUnavailable:
				state.Status = "error"
				state.Detail = "账号临时不可用"
			default:
				state.Status = "error"
				state.Detail = fmt.Sprintf("状态码 %d", acc.client.AccountStatus())
			}
		}
		out = append(out, state)
	}
	return out
}

// EnabledLabels 兼容旧接口：启用的账号名列表。
func (e *GeminiWebEngine) EnabledLabels() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]string, 0, len(e.accounts))
	for _, acc := range e.accounts {
		if acc.enabled {
			out = append(out, acc.label)
		}
	}
	return out
}

func (e *GeminiWebEngine) setErr(err error) {
	e.errMu.Lock()
	e.lastErr = err
	e.errMu.Unlock()
}

func (e *GeminiWebEngine) getErr() error {
	e.errMu.Lock()
	defer e.errMu.Unlock()
	return e.lastErr
}

// Name 引擎标识。
func (e *GeminiWebEngine) Name() string { return e.name }

// Describe 引擎说明。
func (e *GeminiWebEngine) Describe() string {
	return "gemini.google.com 网页版（原生 Go 逆向，Cookie 会话）"
}

// Ready 至少一个账号可用。
func (e *GeminiWebEngine) Ready() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, acc := range e.accounts {
		if acc.enabled && acc.client.Ready() && acc.client.AccountStatus() == geminiweb.StatusAvailable {
			return true
		}
	}
	return false
}

// ListModels 聚合全部账号的模型（去重）。
func (e *GeminiWebEngine) ListModels() []model.ModelInfo {
	e.mu.RLock()
	accounts := append([]*geminiAccount(nil), e.accounts...)
	e.mu.RUnlock()

	seen := map[string]bool{}
	var out []model.ModelInfo
	for _, acc := range accounts {
		if !acc.enabled {
			continue
		}
		for _, m := range acc.client.Models() {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			out = append(out, model.ModelInfo{
				ID:          m.Name,
				Object:      "model",
				DisplayName: m.DisplayName,
				Available:   m.Available,
				Engine:      "a",
			})
		}
	}
	return out
}

// pick 轮询选择可用账号；返回 nil 表示无可用。
func (e *GeminiWebEngine) pick() *geminiAccount {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := len(e.accounts)
	if n == 0 {
		return nil
	}
	start := int(e.next.Add(1))
	for i := 0; i < n; i++ {
		acc := e.accounts[(start+i)%n]
		if acc.enabled && acc.client.Ready() && acc.client.AccountStatus() == geminiweb.StatusAvailable {
			return acc
		}
	}
	return nil
}

// usableCount 可用账号数。
func (e *GeminiWebEngine) usableCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	count := 0
	for _, acc := range e.accounts {
		if acc.enabled && acc.client.Ready() && acc.client.AccountStatus() == geminiweb.StatusAvailable {
			count++
		}
	}
	return count
}

// messagesToPrompt 将 OpenAI messages 转为 Gemini 网页版单轮 prompt。
func messagesToPrompt(messages []model.ChatMessage) string {
	var system []string
	var convo []string
	for _, m := range messages {
		content := m.Text()
		switch m.Role {
		case "system":
			system = append(system, content)
		case "assistant":
			convo = append(convo, "Assistant: "+content)
		default:
			convo = append(convo, "User: "+content)
		}
	}
	prompt := strings.Join(convo, "\n\n")
	if len(system) > 0 {
		prompt = "[System Instructions]\n" + strings.Join(system, "\n") + "\n\n[Conversation]\n" + prompt
	}
	return prompt
}

// Chat 非流式对话（可用账号间故障切换）。
func (e *GeminiWebEngine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	prompt := messagesToPrompt(req.Messages)
	attempts := e.usableCount()
	var lastErr error
	for i := 0; i < attempts; i++ {
		acc := e.pick()
		if acc == nil {
			if lastErr == nil {
				lastErr = errors.New("引擎A 无可用账号（未添加账号、初始化中或配额耗尽）")
			}
			break
		}
		text, err := acc.client.Generate(ctx, prompt, req.Model)
		if err == nil {
			return &model.ChatResult{Text: text, Engine: "a", Model: req.Model}, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// ChatStream 流式对话（可用账号间故障切换）。
func (e *GeminiWebEngine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	prompt := messagesToPrompt(req.Messages)
	attempts := e.usableCount()
	var lastErr error
	for i := 0; i < attempts; i++ {
		acc := e.pick()
		if acc == nil {
			if lastErr == nil {
				lastErr = errors.New("引擎A 无可用账号（未添加账号、初始化中或配额耗尽）")
			}
			break
		}
		text, err := acc.client.GenerateStream(ctx, prompt, req.Model, onDelta)
		if err == nil {
			return &model.ChatResult{Text: text, Engine: "a", Model: req.Model}, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// Passthrough 引擎A 不支持多模态透传。
func (e *GeminiWebEngine) Passthrough() bool { return false }

// ServePassthrough 不适用。
func (e *GeminiWebEngine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("引擎A 不支持多模态透传")
}

// Status 引擎状态（旧接口保留）。
func (e *GeminiWebEngine) Status() map[string]any {
	states := e.AccountStates()
	accounts := make([]map[string]any, 0, len(states))
	for _, st := range states {
		accounts = append(accounts, map[string]any{
			"id":     st.ID,
			"label":  st.Label,
			"ready":  st.Ready,
			"status": st.Status,
			"detail": st.Detail,
			"models": st.Models,
		})
	}
	var errMsg string
	if err := e.getErr(); err != nil {
		errMsg = err.Error()
	}
	return map[string]any{
		"engine":   "a",
		"ready":    e.Ready(),
		"accounts": accounts,
		"error":    errMsg,
	}
}

func statusText(code int) string {
	switch code {
	case geminiweb.StatusAvailable:
		return "available"
	case geminiweb.StatusUnauthenticated:
		return "unauthenticated"
	case geminiweb.StatusTempUnavailable:
		return "temporarily_unavailable"
	default:
		return fmt.Sprintf("status_%d", code)
	}
}
