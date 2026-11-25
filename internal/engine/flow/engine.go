// Package flow 是 Google Flow（flow.google.com）原生引擎。
//
// 与其它引擎不同，Flow 只有「生成」能力：文生图、文生视频，没有对话。
// 每次生成都要先拿一个 reCAPTCHA token（见 captcha.go），这是硬依赖。
package flow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/model"
	"web2api/internal/store"
)

// Engine 是 Flow 号池。
type Engine struct {
	name  string
	proxy string
	st    *store.Store

	mu       sync.RWMutex
	accounts []*flowAccount
	ids      map[string]int
	next     atomic.Uint64
	order    engine.AccountOrder
	// policies 是每账号的调度策略快照（管理台下发）。没有条目的账号视为「不限制」。
	policies map[string]engine.AccountPolicy
	// inflight 统计每账号在跑的图片/视频任务数，用于执行并发上限。
	inflight map[string]*taskInflight

	// captcha 采集 reCAPTCHA token 的打码器。
	captcha CaptchaProvider
	// videoMu 保护 operation → 账号绑定表，轮询与下载要回到原账号。
	videoMu         sync.RWMutex
	videoOperations map[string]videoOperation
	// captchaOnce 保证浏览器只启动一次。
	captchaOnce sync.Once

	errMu   sync.Mutex
	lastErr error
}

// taskInflight 是单个账号在跑的任务计数。
type taskInflight struct {
	image int
	video int
}

// flowAccount 是池内的一个 Flow 账号。
type flowAccount struct {
	id, label string
	mu        sync.Mutex
	creds     Credentials
	cookies   []Cookie
	enabled   bool
	ready     bool
	recaptcha string
	lastErr   string
	client    *RPCClient
	projectID string
	models    []model.ModelInfo

	cooldownUntil  time.Time
	cooldownReason string
}

// proxy 返回账号生效的代理地址。
func (a *flowAccount) proxy(engineProxy string) string {
	if strings.TrimSpace(a.creds.Proxy) != "" {
		return a.creds.Proxy
	}
	return engineProxy
}

// usableLocked 账号当前是否可接单（调用方需持有 a.mu）。
func (a *flowAccount) usableLocked() bool {
	return a.enabled && a.ready && time.Now().After(a.cooldownUntil)
}

// New 构造 Flow 引擎。
func New(cfg config.FlowConfig, st *store.Store) *Engine {
	return &Engine{
		name:            "flow",
		proxy:           strings.TrimSpace(cfg.Proxy),
		st:              st,
		ids:             map[string]int{},
		policies:        map[string]engine.AccountPolicy{},
		inflight:        map[string]*taskInflight{},
		videoOperations: map[string]videoOperation{},
		captcha: &BrowserCaptchaProvider{
			Binary:   strings.TrimSpace(cfg.BrowserPath),
			Proxy:    strings.TrimSpace(cfg.Proxy),
			Headless: cfg.HeadlessEnabled(),
			Warmup:   time.Duration(cfg.Warmup() * float64(time.Second)),
			Timeout:  90 * time.Second,
		},
	}
}

// Name 引擎标识。
func (e *Engine) Name() string { return e.name }

// Describe 引擎说明。
func (e *Engine) Describe() string { return "Google Flow 原生引擎（图片 / 视频生成）" }

// Ready 引擎是否可用。
func (e *Engine) Ready() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, account := range e.accounts {
		account.mu.Lock()
		usable := account.usableLocked()
		account.mu.Unlock()
		if usable {
			return true
		}
	}
	return false
}

// SetAccountOrder 实现 engine.AccountOrderer。
func (e *Engine) SetAccountOrder(ids []string) { e.order.Set(ids) }

// SetAccountPolicy 实现 engine.PolicyStore。
func (e *Engine) SetAccountPolicy(accountID string, policy engine.AccountPolicy) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if policy.Unlimited() {
		delete(e.policies, accountID)
		return
	}
	e.policies[accountID] = policy
}

// AccountPolicy 返回账号当前的策略快照。
func (e *Engine) AccountPolicy(accountID string) engine.AccountPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.policies[strings.TrimSpace(accountID)]
}

// policyEnabledLocked 判断账号的策略是否允许接这类任务。
func (e *Engine) policyEnabledLocked(account *flowAccount, kind engine.PolicyKind) bool {
	if account == nil || kind == "" {
		return true
	}
	policy, ok := e.policies[account.id]
	if !ok {
		return true
	}
	return policy.EnabledFor(kind)
}

// policyAcquireLocked 占用账号该类型的并发额度；额度已满时返回 false。
func (e *Engine) policyAcquireLocked(account *flowAccount, kind engine.PolicyKind) bool {
	if account == nil || kind == "" {
		return true
	}
	policy, ok := e.policies[account.id]
	if !ok {
		return true
	}
	limit := policy.ConcurrencyFor(kind)
	if limit <= 0 {
		return true
	}
	counter := e.inflight[account.id]
	if counter == nil {
		counter = &taskInflight{}
		e.inflight[account.id] = counter
	}
	switch kind {
	case engine.PolicyKindImage:
		if counter.image >= limit {
			return false
		}
		counter.image++
	case engine.PolicyKindVideo:
		if counter.video >= limit {
			return false
		}
		counter.video++
	}
	return true
}

// policyReleaseLocked 归还账号该类型的并发额度。
func (e *Engine) policyReleaseLocked(account *flowAccount, kind engine.PolicyKind) {
	if account == nil || kind == "" {
		return
	}
	counter := e.inflight[account.id]
	if counter == nil {
		return
	}
	switch kind {
	case engine.PolicyKindImage:
		if counter.image > 0 {
			counter.image--
		}
	case engine.PolicyKindVideo:
		if counter.video > 0 {
			counter.video--
		}
	}
}

// acquirePolicySlot 占用账号该类型的并发额度。
func (e *Engine) acquirePolicySlot(account *flowAccount, kind engine.PolicyKind) bool {
	if account == nil || kind == "" {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.policyAcquireLocked(account, kind)
}

// releasePolicySlot 归还账号该类型的并发额度。
func (e *Engine) releasePolicySlot(account *flowAccount, kind engine.PolicyKind) {
	if account == nil || kind == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policyReleaseLocked(account, kind)
}

// AddAccount 添加一个 Flow 账号。
func (e *Engine) AddAccount(id, label string, creds Credentials) error {
	if err := creds.Validate(); err != nil {
		return err
	}
	cookies, err := ParseCookies(creds.Cookies)
	if err != nil {
		return err
	}
	httpClient, err := newBrowserHTTPClient(creds.Proxy, e.proxy)
	if err != nil {
		return err
	}
	e.mu.Lock()
	if _, exists := e.ids[id]; exists {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 已在池中", id)
	}
	account := &flowAccount{
		id:      id,
		label:   label,
		creds:   creds,
		cookies: cookies,
		enabled: true,
		// Cookie 校验通过就能接单。ready 没有第二步初始化，
		// 之前从不赋值，账号会永远停在「初始化中」、usableLocked 永远为 false。
		ready:     true,
		client:    NewRPCClient(httpClient, ""),
		projectID: strings.TrimSpace(creds.ProjectID),
		models:    flowModelCatalog(e.name),
	}
	e.accounts = append(e.accounts, account)
	e.ids[id] = len(e.accounts) - 1
	e.mu.Unlock()
	return nil
}

// RemoveAccount 移除账号。
func (e *Engine) RemoveAccount(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	index, ok := e.ids[id]
	if !ok {
		return fmt.Errorf("账号 %s 不在池中", id)
	}
	e.accounts = append(e.accounts[:index], e.accounts[index+1:]...)
	delete(e.ids, id)
	delete(e.policies, id)
	delete(e.inflight, id)
	e.videoMu.Lock()
	for operationID, binding := range e.videoOperations {
		if binding.AccountID == id {
			delete(e.videoOperations, operationID)
		}
	}
	e.videoMu.Unlock()
	for key, value := range e.ids {
		if value > index {
			e.ids[key] = value - 1
		}
	}
	return nil
}

// SetEnabled 启停账号。
func (e *Engine) SetEnabled(id string, enabled bool) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	index, ok := e.ids[id]
	if !ok {
		return fmt.Errorf("账号 %s 不在池中", id)
	}
	account := e.accounts[index]
	account.mu.Lock()
	defer account.mu.Unlock()
	account.enabled = enabled
	return nil
}

// ReplaceCredentials 替换账号凭据。
func (e *Engine) ReplaceCredentials(id string, creds Credentials) error {
	if err := creds.Validate(); err != nil {
		return err
	}
	cookies, err := ParseCookies(creds.Cookies)
	if err != nil {
		return err
	}
	httpClient, err := newBrowserHTTPClient(creds.Proxy, e.proxy)
	if err != nil {
		return err
	}
	e.mu.RLock()
	index, ok := e.ids[id]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("账号 %s 不在池中", id)
	}
	account := e.accounts[index]
	account.mu.Lock()
	defer account.mu.Unlock()
	account.creds = creds
	account.cookies = cookies
	account.client = NewRPCClient(httpClient, "")
	account.projectID = strings.TrimSpace(creds.ProjectID)
	account.lastErr = ""
	return nil
}

// AccountStates 返回每个账号的实时状态（管理台展示用）。
func (e *Engine) AccountStates() []engine.AccountState {
	e.mu.RLock()
	accounts := append([]*flowAccount(nil), e.accounts...)
	e.mu.RUnlock()
	states := make([]engine.AccountState, 0, len(accounts))
	for _, account := range accounts {
		account.mu.Lock()
		state := engine.AccountState{
			ID:      account.id,
			Label:   account.label,
			Enabled: account.enabled,
			Ready:   account.ready,
			Models:  len(account.models),
			Tier:    "Flow",
		}
		switch {
		case !account.enabled:
			state.Status = "disabled"
		case account.cooldownUntil.After(time.Now()):
			state.Status = "error"
			state.Detail = account.cooldownReason
		case account.lastErr != "":
			state.Status = "error"
			state.Detail = account.lastErr
		case !account.ready:
			state.Status = "initializing"
		default:
			state.Status = "ok"
		}
		account.mu.Unlock()
		states = append(states, state)
	}
	return states
}

// ListModels 返回引擎的模型目录。
func (e *Engine) ListModels() []model.ModelInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	available := false
	for _, account := range e.accounts {
		account.mu.Lock()
		if account.usableLocked() {
			available = true
		}
		account.mu.Unlock()
	}
	catalog := flowModelCatalog(e.name)
	for i := range catalog {
		catalog[i].Available = available
	}
	return catalog
}

// HasAvailableModel 实现 engine.ModelAvailability。
func (e *Engine) HasAvailableModel(modelName string) bool {
	want := normalizeModelID(modelName)
	if want == "" {
		return false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, account := range e.accounts {
		account.mu.Lock()
		usable := account.usableLocked()
		account.mu.Unlock()
		if !usable {
			continue
		}
		for _, item := range flowModelCatalog(e.name) {
			if normalizeModelID(item.ID) == want {
				return true
			}
		}
	}
	return false
}

// RefreshCatalog 实现 engine.CatalogRefresher：Flow 的目录是静态的。
func (e *Engine) RefreshCatalog(ctx context.Context) ([]model.ModelInfo, error) {
	return e.ListModels(), nil
}

// Chat 通过 Flow 的图片生成 RPC 完成一次文生图请求。
func (e *Engine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	modelID := strings.TrimSpace(req.Model)
	item, ok := findImageModel(modelID)
	if !ok {
		return nil, fmt.Errorf("flow: 模型 %s 不是图片生成模型", modelID)
	}
	account, err := e.pickAccount(modelID, engine.PolicyKindImage)
	if err != nil {
		return nil, err
	}
	if !e.acquirePolicySlot(account, engine.PolicyKindImage) {
		return nil, engine.UnavailableError(e)
	}
	defer e.releasePolicySlot(account, engine.PolicyKindImage)

	return e.generateImage(ctx, account, item, req)
}

// ChatStream 与 Chat 等价：Flow 的生成结果整体返回，没有逐字增量。
func (e *Engine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	result, err := e.Chat(ctx, req)
	if err != nil || result == nil || onDelta == nil {
		return result, err
	}
	if text := strings.TrimSpace(result.Text); text != "" {
		if err := onDelta(text); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Passthrough 不透传：Flow 只有生成端点。
func (e *Engine) Passthrough() bool { return false }

// ServePassthrough 不支持透传。
func (e *Engine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("flow: 不支持透传端点")
}

// Status 返回引擎运行状态。
func (e *Engine) Status() map[string]any {
	accounts := e.AccountStates()
	ready := 0
	for _, state := range accounts {
		if state.Status == "ok" {
			ready++
		}
	}
	return map[string]any{
		"engine":         e.name,
		"accounts":       len(accounts),
		"ready_accounts": ready,
		"models":         len(flowModelCatalog(e.name)),
		"captcha":        "browser",
	}
}

// Close 关闭引擎（释放浏览器进程）。
func (e *Engine) Close() error {
	if provider, ok := e.captcha.(*BrowserCaptchaProvider); ok {
		return provider.Close()
	}
	return nil
}

// markCooldown 让账号短暂冷却。
func (e *Engine) markCooldown(account *flowAccount, reason string, duration time.Duration) {
	account.mu.Lock()
	account.cooldownUntil = time.Now().Add(duration)
	account.cooldownReason = reason
	account.mu.Unlock()
}

// setErr 记录引擎级错误。
func (e *Engine) setErr(err error) {
	e.errMu.Lock()
	e.lastErr = err
	e.errMu.Unlock()
}

// flowModelCatalog 构造 Flow 的模型目录。
func flowModelCatalog(engineName string) []model.ModelInfo {
	out := make([]model.ModelInfo, 0, len(imageModels)+len(videoModels))
	for _, item := range imageModels {
		out = append(out, model.ModelInfo{
			ID:          item.ID,
			Object:      "model",
			DisplayName: item.DisplayName,
			Available:   true,
			Engine:      engineName,
			Kind:        model.KindImage,
		})
	}
	for _, item := range videoModels {
		out = append(out, model.ModelInfo{
			ID:          item.ID,
			Object:      "model",
			DisplayName: item.DisplayName,
			Available:   true,
			Engine:      engineName,
			Kind:        model.KindVideo,
		})
	}
	return out
}

// normalizeModelID 统一模型 ID 口径。
func normalizeModelID(id string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(id), "models/"))
}

// findImageModel 按模型 ID 找图片模型记录。
func findImageModel(modelID string) (imageModel, bool) {
	want := normalizeModelID(modelID)
	for _, item := range imageModels {
		if normalizeModelID(item.ID) == want {
			return item, true
		}
	}
	return imageModel{}, false
}

// findVideoModel 按模型 ID 找视频模型记录。
func findVideoModel(modelID string) (videoModel, bool) {
	want := normalizeModelID(modelID)
	for _, item := range videoModels {
		if normalizeModelID(item.ID) == want {
			return item, true
		}
	}
	return videoModel{}, false
}

// newBrowserHTTPClient 构造与浏览器一致的 HTTP 客户端。
//
// Flow 会检查 TLS 指纹与请求头顺序：普通 Go 客户端的 JA3 与 Chrome 不同，
// 容易被判成机器人。这里复用 AI Studio 引擎那套浏览器指纹传输层。
func newBrowserHTTPClient(accountProxy, engineProxy string) (*http.Client, error) {
	proxy := strings.TrimSpace(accountProxy)
	if proxy == "" {
		proxy = strings.TrimSpace(engineProxy)
	}
	return newFlowHTTPClient(proxy)
}
