// engine workbuddy：腾讯 WorkBuddy 订阅号池。
//
// 支持国服（copilot.tencent.com）与国际服（www.workbuddy.ai）两个 build
// 同池混跑，每个账号带自己的站点/域/企业身份；额度从上游实时读取，
// 智能路由优先花掉「积分最快作废」的账号，遇错自动切下一个账号。
package workbuddy

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

// Credentials workbuddy 引擎凭据（accounts.credentials JSON）。
type Credentials struct {
	Token        string `json:"token"`                   // Bearer accessToken
	RefreshToken string `json:"refresh_token,omitempty"` // 刷新用
	Email        string `json:"email,omitempty"`         // 账号标识
	ExpiresAt    int64  `json:"expires_at,omitempty"`    // Unix 秒，0 表示未知
	// Site 是发行版：workbuddy（国服）/ workbuddy-ai（国际服）。
	// 空值表示沿用 base_url 或默认国服（兼容旧账号）。
	Site string `json:"site,omitempty"`
	// UID / Domain / EnterpriseID 是上游身份，登录时拿到，请求要带上。
	UID          string `json:"uid,omitempty"`
	Domain       string `json:"domain,omitempty"`
	EnterpriseID string `json:"enterprise_id,omitempty"`
	// BaseURL 按账号覆盖上游地址（留空则按 Site / 引擎配置推导）。
	BaseURL string `json:"base_url,omitempty"`
	Proxy   string `json:"proxy,omitempty"`
}

// workbuddyAccount 池内账号。
type workbuddyAccount struct {
	id, label string
	mu        sync.Mutex
	// refreshMu 串行化 token 刷新：并发请求同时看到过期时，只有一个能花掉
	// refresh_token（它是一次性的），其余等它刷新完再读新 token。
	refreshMu sync.Mutex
	creds     Credentials
	enabled   bool

	ready   bool
	lastErr string

	cooldownUntil  time.Time
	cooldownReason string

	quota      *Quota
	quotaAt    time.Time
	refreshing bool
}

func (a *workbuddyAccount) proxy(engineProxy string) string {
	if strings.TrimSpace(a.creds.Proxy) != "" {
		return a.creds.Proxy
	}
	return engineProxy
}

// tokenExpired 本地判断 token 过期（提前 5 分钟视为过期）。
func (a *workbuddyAccount) tokenExpired() bool {
	if a.creds.ExpiresAt <= 0 {
		return false // 未知过期时间，乐观视为有效，靠上游 401 兜底
	}
	return time.Now().Unix() > a.creds.ExpiresAt-300
}

// available 报告账号当前能否接单。
func (a *workbuddyAccount) available(now time.Time) bool {
	if !a.enabled || !a.ready || a.tokenExpired() || now.Before(a.cooldownUntil) {
		return false
	}
	if a.quota != nil && a.quota.Exhausted() {
		return false
	}
	return true
}

// Engine workbuddy 引擎。
type Engine struct {
	name    string
	proxy   string
	baseURL string // 引擎级上游覆盖（config.base_url）
	st      *store.Store

	mu       sync.RWMutex
	accounts []*workbuddyAccount
	ids      map[string]int
	next     atomic.Uint64

	errMu   sync.Mutex
	lastErr error

	done      chan struct{}
	closeOnce sync.Once
}

// New 构建 workbuddy 引擎。
func New(cfg config.WorkBuddyConfig, st *store.Store) *Engine {
	e := &Engine{
		name:    "workbuddy",
		proxy:   strings.TrimSpace(cfg.Proxy),
		baseURL: strings.TrimSpace(cfg.BaseURL),
		st:      st,
		ids:     map[string]int{},
		done:    make(chan struct{}),
	}
	go e.quotaLoop()
	return e
}

// quotaLoop 定时刷新额度。WorkBuddy 的积分按自然日/月重置，用尽的账号在
// 重置后应当自己回到池里，而不是等人重新加号。
func (e *Engine) quotaLoop() {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-e.done:
			return
		case <-t.C:
			e.RefreshQuota(context.Background(), "")
		}
	}
}

// resolve 推导账号的上游地址与站点。
// 优先级：账号 base_url > 账号 site > 引擎 base_url > 默认国服。
func (e *Engine) resolve(c Credentials) (string, Site) {
	if b := strings.TrimSpace(c.BaseURL); b != "" {
		return strings.TrimRight(b, "/"), SiteForBaseURL(b)
	}
	if s := strings.TrimSpace(c.Site); s != "" {
		site := SiteFor(s)
		return site.Endpoint, site
	}
	if b := strings.TrimSpace(e.baseURL); b != "" {
		return strings.TrimRight(b, "/"), SiteForBaseURL(b)
	}
	return SiteCN.Endpoint, SiteCN
}

func (e *Engine) authFrom(c Credentials) StreamAuth {
	endpoint, site := e.resolve(c)
	return StreamAuth{
		Token: c.Token, UID: c.UID, Domain: c.Domain,
		EnterpriseID: c.EnterpriseID, Site: site, Endpoint: endpoint,
	}
}

// AddAccount 添加账号（token 即时校验延后到首次请求）。
func (e *Engine) AddAccount(id, label string, creds Credentials) error {
	if strings.TrimSpace(creds.Token) == "" && strings.TrimSpace(creds.RefreshToken) == "" {
		return errors.New("token 与 refresh_token 不能同时为空")
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
	go e.refreshQuota(acc)
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
func (e *Engine) AccountStates() []engine.AccountState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	now := time.Now()
	out := make([]engine.AccountState, 0, len(e.accounts))
	for _, acc := range e.accounts {
		acc.mu.Lock()
		endpoint, site := e.resolve(acc.creds)
		tier, models := site.Name, len(site.Models)
		if acc.quota != nil && acc.quota.Plan != "" {
			tier = acc.quota.Plan
		}
		if IsLegacyCodeBuddy(endpoint) {
			tier, models = "CodeBuddy（旧）", len(legacyModels)
		}
		state := engine.AccountState{
			ID:            acc.id,
			Label:         acc.label,
			Enabled:       acc.enabled,
			Detail:        acc.lastErr,
			Tier:          tier,
			Models:        models,
			Quota:         acc.quota.Display(),
			Service:       site.ID,
			QuotaSnapshot: acc.quota.Snapshot(now),
		}
		switch {
		case !acc.enabled:
			state.Status = "disabled"
		case acc.quota != nil && acc.quota.Exhausted():
			state.Status = "error"
			state.Detail = "额度已用尽: " + acc.quota.Display()
		case now.Before(acc.cooldownUntil):
			state.Status = "cooldown"
			state.Detail = "冷却中: " + acc.cooldownReason
			state.Cooldowns = append(state.Cooldowns, engine.AccountCooldown{
				Scope: "account", Until: acc.cooldownUntil.Unix(), Reason: acc.cooldownReason,
			})
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

func (e *Engine) Name() string { return e.name }

func (e *Engine) Describe() string {
	return "腾讯 WorkBuddy（国服/国际服 SaaS 订阅号池，积分智能路由）"
}

// Ready 至少一个账号可用。
func (e *Engine) Ready() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	now := time.Now()
	for _, acc := range e.accounts {
		acc.mu.Lock()
		ok := acc.available(now)
		acc.mu.Unlock()
		if ok {
			return true
		}
	}
	return false
}

// RefreshCatalog WorkBuddy 没有公开模型列表接口（模型随账号权益），
// 有可用账号时返回按站点聚合的内置目录。
func (e *Engine) RefreshCatalog(context.Context) ([]model.ModelInfo, error) {
	if !e.hasEnabledAccount() {
		return nil, errors.New("workbuddy 没有可用账号，无法获取模型")
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

// ListModels 聚合各账号站点的模型目录（同名去重）。没有账号时给国服目录。
func (e *Engine) ListModels() []model.ModelInfo {
	e.mu.RLock()
	sites := map[string]Site{}
	anyAvailable := false
	legacy := false
	now := time.Now()
	for _, acc := range e.accounts {
		acc.mu.Lock()
		endpoint, site := e.resolve(acc.creds)
		if IsLegacyCodeBuddy(endpoint) {
			legacy = true
		} else {
			sites[site.ID] = site
		}
		if acc.available(now) {
			anyAvailable = true
		}
		acc.mu.Unlock()
	}
	e.mu.RUnlock()
	if len(sites) == 0 && !legacy {
		sites[SiteCN.ID] = SiteCN
	}
	seen := map[string]bool{}
	out := make([]model.ModelInfo, 0, len(SiteCN.Models)+len(SiteIntl.Models))
	add := func(entries []ModelEntry) {
		for _, m := range entries {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			out = append(out, model.ModelInfo{
				ID:          m.ID,
				Object:      "model",
				DisplayName: m.Name,
				Available:   anyAvailable,
				Engine:      e.name,
			})
		}
	}
	if legacy {
		// 旧 CodeBuddy 账号优先展示它自己的目录。
		add(LegacyModels())
	}
	for _, sid := range []string{SiteCN.ID, SiteIntl.ID} {
		if site, ok := sites[sid]; ok {
			add(SiteModels(site))
		}
	}
	return out
}

// pick 智能选号：优先花掉「积分最快作废」的账号；都没读到到期时间时轮询。
func (e *Engine) pick() *workbuddyAccount { return e.pickWhere(nil) }

// pickFor 在 pick 的基础上优先选「站点目录里明确支持该模型」的账号。
// 两个 build 的模型目录不同（国际服没有 Hy/GLM 系列，国服没有 GPT/Gemini 系列），
// 把请求发给不认识的账号只会换来一次上游报错。没有账号声明该模型时退回全部
// 可用账号——目录只是兜底快照，上游可能支持目录外的模型。
func (e *Engine) pickFor(modelName string) *workbuddyAccount {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || modelName == "auto" {
		return e.pick()
	}
	if acc := e.pickWhere(func(site Site, legacy bool) bool {
		return siteSupports(site, legacy, modelName)
	}); acc != nil {
		return acc
	}
	return e.pick()
}

// siteSupports 报告站点目录是否声明了该模型（旧 CodeBuddy 用旧目录）。
func siteSupports(site Site, legacy bool, modelName string) bool {
	if legacy {
		return containsString(legacyModels, modelName)
	}
	return containsString(site.Models, modelName)
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// pickWhere 按可选过滤条件选号（filter 在账号锁内调用，读到的 creds 是一致快照）。
func (e *Engine) pickWhere(filter func(site Site, legacy bool) bool) *workbuddyAccount {
	e.mu.RLock()
	defer e.mu.RUnlock()
	now := time.Now()
	var best *workbuddyAccount
	var bestAt time.Time
	var candidates []*workbuddyAccount
	for _, acc := range e.accounts {
		acc.mu.Lock()
		ok := acc.available(now)
		var at time.Time
		if ok && filter != nil {
			endpoint, site := e.resolve(acc.creds)
			ok = filter(site, IsLegacyCodeBuddy(endpoint))
		}
		if ok && acc.quota != nil {
			at = acc.quota.RunsOutAt()
		}
		acc.mu.Unlock()
		if !ok {
			continue
		}
		if at.IsZero() {
			candidates = append(candidates, acc)
			continue
		}
		if best == nil || at.Before(bestAt) {
			best, bestAt = acc, at
		}
	}
	if best != nil {
		return best
	}
	if len(candidates) == 0 {
		return nil
	}
	start := int(e.next.Add(1))
	return candidates[start%len(candidates)]
}

func (e *Engine) markCooldown(acc *workbuddyAccount, reason string, d time.Duration) {
	acc.mu.Lock()
	acc.cooldownUntil = time.Now().Add(d)
	acc.cooldownReason = reason
	acc.mu.Unlock()
}

// toWorkBuddyRequest ChatRequest → Request。
func toWorkBuddyRequest(req model.ChatRequest) *Request {
	out := &Request{
		Model:       req.Model,
		MaxTokens:   engine.DerefInt(req.MaxTokens),
		Temperature: req.Temperature,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}
	for _, m := range req.Messages {
		msg := Message{Role: m.PlainRole(), Text: m.Text(), ToolCalls: m.ToolCalls}
		out.Messages = append(out.Messages, msg)
	}
	return out
}

// freshAuth 返回可直接用于请求的账号身份；token 将过期时先刷新并回写。
func (e *Engine) freshAuth(ctx context.Context, acc *workbuddyAccount) (StreamAuth, error) {
	acc.refreshMu.Lock()
	defer acc.refreshMu.Unlock()

	acc.mu.Lock()
	c := acc.creds
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()

	a := e.authFrom(c)
	expiring := c.ExpiresAt > 0 && time.Now().Unix() >= c.ExpiresAt-300
	if expiring && strings.TrimSpace(c.RefreshToken) == "" {
		if time.Now().Unix() >= c.ExpiresAt {
			return StreamAuth{}, errors.New("token 已过期且没有 refresh_token")
		}
	}
	if expiring && strings.TrimSpace(c.RefreshToken) != "" {
		tok, err := Refresh(ctx, proxy, a, c.RefreshToken)
		if err != nil {
			return StreamAuth{}, err
		}
		acc.mu.Lock()
		acc.creds.Token = tok.AccessToken
		acc.creds.RefreshToken = tok.RefreshToken
		if !tok.ExpiresAt.IsZero() {
			acc.creds.ExpiresAt = tok.ExpiresAt.Unix()
		}
		if tok.Domain != "" {
			acc.creds.Domain = tok.Domain
		}
		c = acc.creds
		acc.mu.Unlock()
		e.persist(acc)
		a = e.authFrom(c)
	}
	if strings.TrimSpace(a.Token) == "" {
		return StreamAuth{}, errors.New("账号没有 access token")
	}
	return a, nil
}

// persist 把轮换后的凭据写回数据库（不动账号状态）。
func (e *Engine) persist(acc *workbuddyAccount) {
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

// refreshQuota 异步读取账号额度。
func (e *Engine) refreshQuota(acc *workbuddyAccount) {
	acc.mu.Lock()
	if acc.refreshing {
		acc.mu.Unlock()
		return
	}
	acc.refreshing = true
	acc.mu.Unlock()
	defer func() {
		acc.mu.Lock()
		acc.refreshing = false
		acc.quotaAt = time.Now()
		acc.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	auth, err := e.freshAuth(ctx, acc)
	if err != nil {
		e.setQuota(acc, &Quota{Err: shortErr(err), ReadAt: time.Now()})
		return
	}
	acc.mu.Lock()
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()
	e.setQuota(acc, fetchQuota(ctx, proxy, auth, ""))
}

func (e *Engine) setQuota(acc *workbuddyAccount, q *Quota) {
	acc.mu.Lock()
	acc.quota = q
	acc.mu.Unlock()
}

func (acc *workbuddyAccount) quotaStale(d time.Duration) bool {
	acc.mu.Lock()
	defer acc.mu.Unlock()
	return acc.quota == nil || time.Since(acc.quotaAt) > d
}

// chatLoop 轮询账号直到成功或池耗尽。sink 非空时走工具透传路径。
func (e *Engine) chatLoop(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	if !e.Ready() {
		return nil, engine.UnavailableError(e)
	}
	cReq := toWorkBuddyRequest(req)
	var lastErr error
	for attempt := 0; attempt < e.usableCount(); attempt++ {
		acc := e.pickFor(req.Model)
		if acc == nil {
			break
		}
		acc.mu.Lock()
		proxy := acc.proxy(e.proxy)
		acc.mu.Unlock()

		auth, err := e.freshAuth(ctx, acc)
		if err != nil {
			if IsUnauthorized(err) || strings.Contains(err.Error(), "refresh") {
				e.markCooldown(acc, "token 刷新失败，请重新授权", 30*time.Minute)
			}
			lastErr = err
			continue
		}
		res, err := Stream(ctx, proxy, auth, cReq, sink)
		if err == nil {
			if acc.quotaStale(5 * time.Minute) {
				go e.refreshQuota(acc)
			}
			return &model.ChatResult{
				Text:      res.Text,
				ToolCalls: res.ToolCalls,
				Engine:    e.name,
				Model:     req.Model,
				Usage:     engine.SubResultUsage(res.PromptTokens, res.CompletionTokens, 0),
			}, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		switch {
		case IsChannelRefused(err):
			// 渠道风控由请求内容触发，与账号无关，换号也没用，直接返回。
			e.setErr(err)
			return nil, fmt.Errorf("workbuddy: %w", err)
		case IsCreditsExhausted(err):
			// 额度用尽：该账号退出本轮到下一次额度刷新。
			e.markCooldown(acc, "额度用尽", 30*time.Minute)
			go e.refreshQuota(acc)
		case IsRestricted(err):
			e.markCooldown(acc, "账号被风控封禁", 6*time.Hour)
		case IsUnauthorized(err):
			// 刷新已试过一次仍 401：标记下线，换下一个账号。
			// 先让 refreshQuota 读到真实额度（token 其实没坏时别误判成授权问题）。
			e.markCooldown(acc, "token 失效，请重新授权", 30*time.Minute)
			acc.mu.Lock()
			acc.lastErr = "token 失效，请重新授权"
			acc.mu.Unlock()
		case IsRateLimited(err):
			e.markCooldown(acc, "上游限流", 3*time.Minute)
		case UpstreamStatus(err) >= 500:
			// 上游 5xx：可能是该账号所在集群的临时故障，短暂冷却后换号重试。
			e.markCooldown(acc, "上游 5xx", time.Minute)
		default:
			// 其余错误（上下文超长等）不再换号。
			e.setErr(err)
			return nil, fmt.Errorf("workbuddy: %w", err)
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("workbuddy: %w", lastErr)
	}
	return nil, errors.New("workbuddy 没有可用账号")
}

func (e *Engine) usableCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	now := time.Now()
	n := 0
	for _, acc := range e.accounts {
		acc.mu.Lock()
		if acc.available(now) {
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
func (e *Engine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, model.TextSink(nil))
}

// ChatStream 流式对话。
func (e *Engine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, model.TextSink(onDelta))
}

// ChatStreamSink 流式对话并透传工具调用增量（实现 engine.ToolEngine）。
func (e *Engine) ChatStreamSink(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, sink)
}

// RefreshQuota 立即刷新额度（id 为空时刷新全部）。refreshQuota 自带并发去重，
// 所以定时任务与请求后的刷新不会叠在一起打上游。
func (e *Engine) RefreshQuota(ctx context.Context, id string) {
	e.mu.RLock()
	var targets []*workbuddyAccount
	for _, acc := range e.accounts {
		if id == "" || acc.id == id {
			targets = append(targets, acc)
		}
	}
	e.mu.RUnlock()
	for _, acc := range targets {
		go e.refreshQuota(acc)
	}
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

// Passthrough workbuddy 不支持多模态端点透传。
func (e *Engine) Passthrough() bool { return false }

func (e *Engine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("workbuddy 引擎不支持透传")
}

// Close 关闭引擎（停止额度定时刷新）。
func (e *Engine) Close() {
	e.closeOnce.Do(func() {
		if e.done != nil {
			close(e.done)
		}
	})
}
