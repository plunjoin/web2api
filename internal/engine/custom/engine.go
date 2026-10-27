// engine_custom.go 第三方 Key 引擎：
// 管理台填写 base_url + api_key，按 OpenAI 兼容协议转发 /v1/chat/completions。
package custom

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
	"web2api/internal/engine/codex"
	"web2api/internal/engine/custombalance"
)

// Credentials 第三方 Key 凭据（accounts.credentials JSON）。
type Credentials struct {
	BaseURL string   `json:"base_url"`         // OpenAI 兼容根地址，可带或不带 /v1
	APIKey  string   `json:"api_key"`          // 上游 API Key
	Models  []string `json:"models,omitempty"` // 可选静态模型列表（上游无 /v1/models 时用）
	Proxy   string   `json:"proxy,omitempty"`  // 账号级代理，优先于引擎全局代理
}

// customAccount 池内账号：每个供应商一张卡（base_url + key）。
type customAccount struct {
	id, label string
	mu        sync.Mutex
	creds     Credentials
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

// usableLocked 账号当前是否可接单（调用方需持有 a.mu）。
func (a *customAccount) usableLocked() bool {
	return a.enabled && a.ready && time.Now().After(a.cooldownUntil)
}

// declaresLocked 账号目录是否声明了该模型（调用方需持有 a.mu）。
// 目录为空即「没声明任何模型」——不再被当成「任意模型都能试」。
func (a *customAccount) declaresLocked(modelID string) bool {
	want := normalizeModelID(modelID)
	if want == "" {
		return false
	}
	for _, m := range a.models {
		if normalizeModelID(m.ID) == want {
			return true
		}
	}
	return false
}

// normalizeModelID 统一模型 ID 口径：去空白、去 models/ 前缀、小写。
// Google 的 OpenAI 兼容端点目录会带 models/ 前缀，客户端通常用裸名。
func normalizeModelID(id string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(id), "models/"))
}

// Engine 第三方 OpenAI 兼容 Key 号池。
type Engine struct {
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
func New(cfg config.CustomConfig, st *store.Store) *Engine {
	return &Engine{
		name:  "custom",
		proxy: strings.TrimSpace(cfg.Proxy),
		st:    st,
		ids:   map[string]int{},
	}
}

func (e *Engine) buildUpstream(creds Credentials) (*upstream.OpenAI, error) {
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
	// 300s 只约束「响应头到达时间」：长回答的 SSE 不能被总时长硬切断。
	client, err := codex.NewStreamHTTPClient(proxy, 300*time.Second)
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
func (e *Engine) AddAccount(id, label string, creds Credentials) error {
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

func (e *Engine) refreshAccount(ctx context.Context, acc *customAccount) {
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
		// 没有模型目录：账号仍在池中展示，但不再对任意模型开放——按声明选号后
		// 这种账号接不到任何请求，必须把原因写清楚，否则管理台只看到「就绪」。
		acc.ready = true
		acc.lastErr = "未拉到模型目录且未手填 models：该账号不会承接任何模型（请点“获取模型列表”或手填）"
		if err := up.ReadyErr(); err != "" {
			acc.lastErr = "未拉到模型目录，该账号不会承接任何模型: " + err
		}
	}
	acc.mu.Unlock()
	e.refreshBalance(acc)
}

// refreshBalance 查询余额（best-effort）：先官方供应商，再 Sub2API /v1/usage → /user/balance → NewAPI。
func (e *Engine) refreshBalance(acc *customAccount) {
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

// ReplaceCredentials 更新凭据（重建上游客户端）。
func (e *Engine) ReplaceCredentials(id string, creds Credentials) error {
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
func (e *Engine) AccountStates() []engine.AccountState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]engine.AccountState, 0, len(e.accounts))
	for _, acc := range e.accounts {
		acc.mu.Lock()
		tier := acc.provider
		if tier == "" {
			tier = upstream.NormalizeBaseURL(acc.creds.BaseURL)
		}
		state := engine.AccountState{
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
		case len(acc.models) == 0:
			// 没有模型目录 → 不会承接任何模型，管理台必须能一眼看出来。
			state.Status = "error"
			if strings.TrimSpace(state.Detail) == "" {
				state.Detail = "未声明任何模型：不会接单，请点“获取模型列表”或手填 models"
			}
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

func (e *Engine) Name() string { return e.name }

func (e *Engine) Describe() string {
	return "第三方 OpenAI 兼容 Key（base_url + api_key，常见供应商余额查询）"
}

// Ready 是否至少有一个「能真正接单」的账号：启用、就绪、未冷却，且目录非空。
// 目录为空的账号已经不能承接任何模型（不再兜底），计入就绪只会误导管理台与 /health。
func (e *Engine) Ready() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, acc := range e.accounts {
		acc.mu.Lock()
		ok := acc.usableLocked() && len(acc.models) > 0
		acc.mu.Unlock()
		if ok {
			return true
		}
	}
	return false
}

// ListModels 合并各供应商模型目录（去重）。
// available 的口径：该模型至少出现在一个可用账号（启用、就绪、未冷却）的目录里。
func (e *Engine) ListModels() []model.ModelInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	seen := map[string]model.ModelInfo{}
	order := make([]string, 0)
	for _, acc := range e.accounts {
		acc.mu.Lock()
		enabled := acc.usableLocked()
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

// candidates 按模型精确挑选候选账号。
// 返回值：usable 为「声明了该模型且当前可用」的账号（按轮转起点排序）；
// declared 为声明了该模型的账号总数（不看可用态）；total 为号池账号总数。
// 关键约束：没有声明该模型的账号一律不入选，目录为空的账号也不再兜底。
func (e *Engine) candidates(modelName string) (usable []*customAccount, declared int, total int) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := len(e.accounts)
	if n == 0 {
		return nil, 0, 0
	}
	want := normalizeModelID(modelName)
	start := int(e.next.Add(1))
	usable = make([]*customAccount, 0, n)
	for i := 0; i < n; i++ {
		acc := e.accounts[(start+i)%n]
		acc.mu.Lock()
		has := acc.declaresLocked(want)
		ok := has && acc.usableLocked()
		acc.mu.Unlock()
		if has {
			declared++
		}
		if ok {
			usable = append(usable, acc)
		}
	}
	return usable, declared, n
}

// pick 按模型精确选号：只返回目录里明确声明了该模型的可用账号。
// 这里刻意不再保留「找不到就随便抓一个就绪号」的兜底——那会把 lyria / veo
// 这类模型打到完全不相干的第三方供应商上，最后以误导性的上游 404 收场。
func (e *Engine) pick(modelName string) (*customAccount, error) {
	usable, declared, total := e.candidates(modelName)
	if len(usable) > 0 {
		return usable[0], nil
	}
	switch {
	case total == 0:
		return nil, errors.New("custom: 号池没有账号")
	case declared == 0:
		return nil, e.missError(modelName)
	default:
		return nil, fmt.Errorf("custom: 声明了模型 %q 的 %d 个账号当前均不可用（已停用 / 初始化中 / 冷却中）",
			strings.TrimPrefix(strings.TrimSpace(modelName), "models/"), declared)
	}
}

// missError 生成「没有账号声明该模型」的明确错误，附带号池现状便于排查。
func (e *Engine) missError(modelName string) error {
	want := normalizeModelID(modelName)
	if want == "" {
		return engine.NewModelNotFound("", "请求未指定模型，custom 号池无法选号")
	}
	display := strings.TrimPrefix(strings.TrimSpace(modelName), "models/")
	e.mu.RLock()
	accounts := append([]*customAccount(nil), e.accounts...)
	e.mu.RUnlock()
	if len(accounts) == 0 {
		return engine.NewModelNotFound(display, fmt.Sprintf("custom 号池没有账号，无法承接模型 %q", display))
	}
	const shown = 3
	empty := 0
	parts := make([]string, 0, shown)
	for i, acc := range accounts {
		acc.mu.Lock()
		label := acc.label
		if strings.TrimSpace(label) == "" {
			label = acc.id
		}
		state := fmt.Sprintf("%d 个模型", len(acc.models))
		if len(acc.models) == 0 {
			empty++
			state = "无模型目录"
		}
		if !acc.enabled {
			state += "、已停用"
		}
		acc.mu.Unlock()
		if i < shown {
			parts = append(parts, fmt.Sprintf("%s（%s）", label, state))
		}
	}
	detail := fmt.Sprintf("custom 号池没有账号声明模型 %q（%s", display, strings.Join(parts, "、"))
	if len(accounts) > shown {
		detail += fmt.Sprintf(" 等 %d 个账号", len(accounts))
	}
	detail += "）"
	if empty > 0 {
		detail += fmt.Sprintf("；其中 %d 个账号没有模型目录，未拉到目录的账号不再兜底接单，请在管理台获取模型列表或手填 models", empty)
	}
	return engine.NewModelNotFound(display, detail)
}

// isMediaMimeRejection 上游以「产出不了这种 MIME 类型」为由拒绝请求。
// 典型场景：把 Lyria 音乐模型发到只能返回文本的 chat 端点，Google 会回
// “Unhandled generated data mime type: audio/mpeg”。这不是参数写错，
// 而是端点选错了，需要提示调用方换端点。
func isMediaMimeRejection(msg string) bool {
	if !strings.Contains(msg, "mime type") && !strings.Contains(msg, "mimetype") {
		return false
	}
	return strings.Contains(msg, "audio/") || strings.Contains(msg, "video/") || strings.Contains(msg, "image/")
}

func (e *Engine) markCooldown(acc *customAccount, reason string, d time.Duration) {
	acc.mu.Lock()
	acc.cooldownUntil = time.Now().Add(d)
	acc.cooldownReason = reason
	acc.lastErr = reason
	acc.mu.Unlock()
}

// ServeModelPassthrough 按模型把 OpenAI 兼容端点请求原样转发到拥有该模型的账号上游
// （/v1/embeddings、/v1/images/generations 等）。与 ServePassthrough 的区别：先解析
// 模型归属的账号再转发；解析失败（引擎不可用/无可用账号）时返回错误且未写出响应。
func (e *Engine) ServeModelPassthrough(ctx context.Context, modelID, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	up, err := e.upstreamFor(modelID)
	if err != nil {
		return err
	}
	return up.ServePassthrough(ctx, method, path, body, contentType, w)
}

// ProbeImage 用账号上游探测图片模型（/v1/images/generations）。
func (e *Engine) ProbeImage(ctx context.Context, modelID string) (string, error) {
	up, err := e.upstreamFor(modelID)
	if err != nil {
		return "", err
	}
	return up.ProbeImage(ctx, modelID)
}

// ProbeEmbedding 用账号上游探测向量模型（/v1/embeddings）。
func (e *Engine) ProbeEmbedding(ctx context.Context, modelID string) (string, error) {
	up, err := e.upstreamFor(modelID)
	if err != nil {
		return "", err
	}
	return up.ProbeEmbedding(ctx, modelID)
}

func (e *Engine) upstreamFor(modelID string) (*upstream.OpenAI, error) {
	acc, err := e.pick(modelID)
	if err != nil {
		return nil, err
	}
	acc.mu.Lock()
	up := acc.up
	acc.mu.Unlock()
	if up == nil {
		return nil, errors.New("上游客户端未初始化")
	}
	return up, nil
}

// stripChatTools 去掉 tools / tool_choice，并把 tool 历史折成普通文本。
// custom 适配器专属的 400 降级路径：上游（MiniMax 等）拒收 Codex 自带工具
// schema 时，按能力表 RetryNoToolsOn400 用纯文本重试一次。
func stripChatTools(req model.ChatRequest) model.ChatRequest {
	out := req
	out.Tools = nil
	out.ToolChoice = nil
	out.ParallelToolCalls = nil
	msgs := make([]model.ChatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch strings.ToLower(m.Role) {
		case "tool":
			text := m.Text()
			if name := strings.TrimSpace(m.Name); name != "" {
				text = "[" + name + "] " + text
			}
			msgs = append(msgs, model.ChatMessage{Role: "user", Content: text})
		case "assistant":
			msg := m
			msg.ToolCalls = nil
			if msg.Content == nil {
				msg.Content = ""
			}
			if strings.TrimSpace(msg.Text()) == "" && len(m.ToolCalls) > 0 {
				continue
			}
			msgs = append(msgs, msg)
		default:
			msgs = append(msgs, m)
		}
	}
	out.Messages = msgs
	return out
}

// isHTTPStatus 判断错误文案是否含「HTTP <code>」（上游 OpenAI 兼容客户端惯用格式）。
func isHTTPStatus(err error, code int) bool {
	if err == nil {
		return false
	}
	needle := strings.ToLower(fmt.Sprintf("http %d", code))
	return strings.Contains(strings.ToLower(err.Error()), needle)
}

func (e *Engine) chatLoop(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	// 严格按模型选号：只有目录里声明了该模型的账号才有资格接单。
	// 没有任何账号声明 → 直接 model_not_found，绝不换号改投别的供应商。
	usable, _, _ := e.candidates(req.Model)
	if len(usable) == 0 {
		// 复用 pick 的分支错误：区分「号池为空」「无人声明该模型」「声明了但都不可用」。
		if _, err := e.pick(req.Model); err != nil {
			return nil, err
		}
		return nil, errors.New("custom: 没有可用账号")
	}
	var lastErr error
	for _, acc := range usable {
		if ctx.Err() != nil {
			return nil, ctx.Err()
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
		call := func(r model.ChatRequest) (*model.ChatResult, error) {
			if sink != nil {
				return up.ChatStreamSink(ctx, r, sink)
			}
			return up.Chat(ctx, r)
		}
		res, err = call(req)
		// Codex / Responses 自带的复杂 tools 在 MiniMax 等上游常 400：
		// 按能力表降级——去掉 tools 再试一次，保证纯文本对话可用。
		if err != nil && len(req.Tools) > 0 && engine.CapsFor(e.name, req.Model).RetryNoToolsOn400 && isHTTPStatus(err, 400) {
			if retry, retryErr := call(stripChatTools(req)); retryErr == nil {
				res, err = retry, nil
			}
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
			// 上游以「产出不了这种 MIME」为由拒绝：说明模型用途和端点不匹配
			// （把音乐/语音模型发到了只能返回文本的 chat 端点），换成能看懂的提示。
			if kind := model.ClassifyKind(req.Model, "", nil); model.ChatIncompatibleKind(kind) && isMediaMimeRejection(msg) {
				return nil, engine.NewUnsupportedEndpoint(req.Model, kind)
			}
			// 模型不存在、参数非法等业务错误属于「这一次请求」的问题，不是账号健康问题：
			// 既不写进账号详情（避免错误详情挂到不相干的账号上），也不换号重试。
			return nil, fmt.Errorf("custom: %w", err)
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("custom: %w", lastErr)
	}
	return nil, errors.New("custom 没有可用账号")
}

func (e *Engine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, nil)
}

func (e *Engine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	if onDelta == nil {
		return e.chatLoop(ctx, req, nil)
	}
	return e.chatLoop(ctx, req, model.TextSink(onDelta))
}

// ChatStreamSink 流式对话（支持工具调用增量）。
func (e *Engine) ChatStreamSink(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, sink)
}

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

func (e *Engine) Passthrough() bool { return false }

func (e *Engine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("custom 引擎不支持透传")
}

func (e *Engine) Close() {}

// ProbeCustomModels 管理台「获取模型列表」：用临时凭据探测上游 /v1/models。
func ProbeCustomModels(ctx context.Context, engineProxy string, creds Credentials) ([]string, error) {
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
