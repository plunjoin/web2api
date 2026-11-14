// engine workbuddy：腾讯 WorkBuddy 订阅号池。
//
// 国服（copilot.tencent.com）与国际服（www.workbuddy.ai）是两个供应商：
// workbuddy 只收国服账号，workbuddy-ai 只收国际服账号。两边各自选号、
// 各自报目录，同名模型才能在路由组里分成两个成员。额度从上游实时读取，
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

	"github.com/google/uuid"

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
	// AutoCheckIn 是否每日自动签到（仅国服；国际服无签到活动）。nil 视为开启，
	// 与桌面端每日签到一致。
	AutoCheckIn *bool `json:"auto_checkin,omitempty"`
}

// AutoCheckInEnabled 报告该账号是否开启自动签到（缺省视为开启）。
func (c Credentials) AutoCheckInEnabled() bool { return c.AutoCheckIn == nil || *c.AutoCheckIn }

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

	// config 是 /v3/config 读到的真实模型目录（含免费标识与价格）。
	config        []ConfigModel
	configAt      time.Time
	configErr     string
	refreshingCfg bool

	// deepseek-v4.1-flash 专属 token 窗口：本账号的应答记录与仍在生效的拒绝重置时刻。
	dsRows      []dsRow
	dsRestUntil time.Time

	// 每日签到（仅国服）：上次尝试时间与结果文案。
	checkinAt  time.Time
	checkinMsg string
}

func (a *workbuddyAccount) proxy(engineProxy string) string {
	if strings.TrimSpace(a.creds.Proxy) != "" {
		return a.creds.Proxy
	}
	return engineProxy
}

// addDsRow 记录一条 deepseek 应答（token 数），只保留最近两天的记录以控内存。
func (a *workbuddyAccount) addDsRow(at time.Time, tokens float64) {
	if tokens <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dsRows = append(a.dsRows, dsRow{at: at, tokens: tokens})
	if cut := at.Add(-2 * dsSpan); len(a.dsRows) > 0 && a.dsRows[0].at.Before(cut) {
		i := 0
		for i < len(a.dsRows) && a.dsRows[i].at.Before(cut) {
			i++
		}
		a.dsRows = append([]dsRow(nil), a.dsRows[i:]...)
	}
}

// recordDsRefusal 记录仍在生效的 deepseek 拒绝重置时刻（只保留最晚的）。
func (a *workbuddyAccount) recordDsRefusal(reset time.Time) {
	if reset.IsZero() {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if reset.After(a.dsRestUntil) {
		a.dsRestUntil = reset
	}
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
	order    engine.AccountOrder

	errMu   sync.Mutex
	lastErr error

	done      chan struct{}
	closeOnce sync.Once
}

// New 构建国服 workbuddy 引擎。
func New(cfg config.WorkBuddyConfig, st *store.Store) *Engine {
	return newEngine("workbuddy", cfg, st)
}

// NewAI 构建国际服 WorkBuddy AI 引擎。与国服共用代码，但是独立的供应商：
// 只接国际服账号，目录也只报国际服的。
func NewAI(cfg config.WorkBuddyConfig, st *store.Store) *Engine {
	return newEngine("workbuddy-ai", cfg, st)
}

func newEngine(name string, cfg config.WorkBuddyConfig, st *store.Store) *Engine {
	e := &Engine{
		name:    name,
		proxy:   strings.TrimSpace(cfg.Proxy),
		baseURL: strings.TrimSpace(cfg.BaseURL),
		st:      st,
		ids:     map[string]int{},
		done:    make(chan struct{}),
	}
	go e.quotaLoop()
	go e.checkinLoop()
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

// home 是该引擎的发行版。国服与国际服是两个供应商，账号没有显式站点时
// 按引擎归属落地，而不是一律回国服。
func (e *Engine) home() Site {
	if e.name == SiteIntl.ID {
		return SiteIntl
	}
	return SiteCN
}

// resolve 推导账号的上游地址与站点。
// 优先级：账号 base_url > 账号 site > 引擎 base_url > 引擎自己的发行版。
func (e *Engine) resolve(c Credentials) (string, Site) {
	// 账号自己声明的站点优先于引擎级地址：引擎 base_url 只是给没写站点的旧账号兜底，
	// 不能把国际服账号改判回国服（测试桩和自建反代的地址也不是发行版地址）。
	if _, site := resolveSite(c); strings.TrimSpace(c.Site) != "" || knownEditionURL(c.BaseURL) {
		if b := strings.TrimSpace(c.BaseURL); b != "" && knownEditionURL(b) {
			return strings.TrimRight(b, "/"), site
		}
		return site.Endpoint, site
	}
	if b := strings.TrimSpace(c.BaseURL); b != "" {
		return strings.TrimRight(b, "/"), SiteForBaseURL(b)
	}
	if b := strings.TrimSpace(e.baseURL); b != "" {
		return strings.TrimRight(b, "/"), SiteForBaseURL(b)
	}
	site := e.home()
	return site.Endpoint, site
}

// accepts 报告账号是否属于这个供应商。旧 CodeBuddy 账号沿国服引擎接纳，
// 避免升级后失去入口。
func (e *Engine) accepts(c Credentials) bool {
	endpoint, site := e.resolve(c)
	if IsLegacyCodeBuddy(endpoint) {
		return e.home().ID == SiteCN.ID
	}
	return site.ID == e.home().ID
}

func (e *Engine) authFrom(c Credentials) StreamAuth {
	endpoint, site := e.resolve(c)
	return StreamAuth{
		Token: c.Token, UID: c.UID, Domain: c.Domain,
		EnterpriseID: c.EnterpriseID, Site: site, Endpoint: endpoint,
	}
}

// AddAccount 添加账号（token 即时校验延后到首次请求）。
// 另一个发行版的账号不进这个池：国服与国际服是两个供应商，混进来会让
// 同名模型无法在路由组里分开。
func (e *Engine) AddAccount(id, label string, creds Credentials) error {
	if strings.TrimSpace(creds.Token) == "" && strings.TrimSpace(creds.RefreshToken) == "" {
		return errors.New("token 与 refresh_token 不能同时为空")
	}
	if !e.accepts(creds) {
		_, site := e.resolve(creds)
		return fmt.Errorf("账号属于 %s，不能加入 %s", site.Name, e.home().Name)
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
	go e.refreshConfig(acc)
	go e.checkinAccount(acc)
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
		if site.ID == SiteCN.ID {
			v := acc.creds.AutoCheckInEnabled()
			state.AutoCheckIn = &v
			state.CheckIn = acc.checkinMsg
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
	if e.home().ID == SiteIntl.ID {
		return "WorkBuddy AI（国际服 www.workbuddy.ai，积分智能路由）"
	}
	return "腾讯 WorkBuddy（国服 copilot.tencent.com，积分智能路由）"
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

// RefreshCatalog 触发 /v3/config 刷新，返回当前聚合目录（真实 config 优先，
// 读不到时退回内置目录）。
func (e *Engine) RefreshCatalog(context.Context) ([]model.ModelInfo, error) {
	if !e.hasEnabledAccount() {
		return nil, errors.New("workbuddy 没有可用账号，无法获取模型")
	}
	e.mu.RLock()
	targets := make([]*workbuddyAccount, 0, len(e.accounts))
	for _, acc := range e.accounts {
		targets = append(targets, acc)
	}
	e.mu.RUnlock()
	for _, acc := range targets {
		go e.refreshConfig(acc)
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

// ListModels 聚合本供应商账号的模型目录（同名去重，/v3/config 优先于内置目录）。
// 有账号读到真实 config 时用它的模型（含免费标识与积分倍率），否则退回本
// 发行版的内置兜底目录。没有账号时也只报本发行版的目录。
func (e *Engine) ListModels() []model.ModelInfo {
	e.mu.RLock()
	sites := map[string]Site{}
	anyAvailable := false
	legacy := false
	now := time.Now()
	// 按站点聚合各账号读到的 config 模型（同名去重，先到先得）。
	configBySite := map[string][]ConfigModel{}
	seenCfg := map[string]map[string]bool{}
	for _, acc := range e.accounts {
		acc.mu.Lock()
		endpoint, site := e.resolve(acc.creds)
		if IsLegacyCodeBuddy(endpoint) {
			legacy = true
		} else {
			sites[site.ID] = site
			if len(acc.config) > 0 {
				if seenCfg[site.ID] == nil {
					seenCfg[site.ID] = map[string]bool{}
				}
				for _, m := range acc.config {
					if seenCfg[site.ID][m.ID] {
						continue
					}
					seenCfg[site.ID][m.ID] = true
					configBySite[site.ID] = append(configBySite[site.ID], m)
				}
			}
		}
		if acc.available(now) {
			anyAvailable = true
		}
		acc.mu.Unlock()
	}
	e.mu.RUnlock()
	if len(sites) == 0 && !legacy {
		sites[e.home().ID] = e.home()
	}
	seen := map[string]bool{}
	out := make([]model.ModelInfo, 0, 40)
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
				Free:        m.Free,
			})
		}
	}
	addCfg := func(models []ConfigModel) {
		for _, m := range models {
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
				Free:        m.Free,
				Rate:        m.Rate,
			})
		}
	}
	if legacy {
		// 旧 CodeBuddy 账号优先展示它自己的目录（无免费标识）。
		add(LegacyModels())
	}
	// 只报本发行版的目录。另一个发行版是独立供应商，由它自己的引擎报。
	for _, sid := range []string{e.home().ID} {
		site, ok := sites[sid]
		if !ok {
			continue
		}
		if cfgs := configBySite[sid]; len(cfgs) > 0 {
			addCfg(cfgs)
		} else {
			add(SiteModels(site))
		}
	}
	return out
}

// SetAccountOrder 实现 engine.AccountOrderer：管理台保存的账号顺序优先于轮询。
func (e *Engine) SetAccountOrder(ids []string) { e.order.Set(ids) }

// pick 智能选号：优先花掉「积分最快作废」的账号；都没读到到期时间时轮询。
func (e *Engine) pick() *workbuddyAccount { return e.pickWhere("", nil) }

// pickFor 在 pick 的基础上优先选「站点目录里明确支持该模型」的账号。
// 两个 build 的模型目录不同（国际服没有 Hy/GLM 系列，国服没有 GPT/Gemini 系列），
// 把请求发给不认识的账号只会换来一次上游报错。没有账号声明该模型时退回全部
// 可用账号——目录只是兜底快照，上游可能支持目录外的模型。
func (e *Engine) pickFor(modelName string) *workbuddyAccount {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || modelName == "auto" {
		return e.pick()
	}
	if acc := e.pickWhere(modelName, func(site Site, legacy bool) bool {
		return siteSupports(site, legacy, modelName)
	}); acc != nil {
		return acc
	}
	return e.pick()
}

// pickForExcept 在一次请求内按序选号。配置了路由顺序时严格按管理台顺序，
// 跳过本次请求已试过的账号；未配置时保持原来的智能选号（到期优先/轮询）。
func (e *Engine) pickForExcept(ctx context.Context, modelName string, tried map[string]struct{}) *workbuddyAccount {
	// 会话粘性：上次接单的账号仍可用、支持该模型且本轮没试过时优先。
	if preferred := engine.PreferredAccount(ctx); preferred != "" {
		if _, skip := tried[preferred]; !skip {
			if acc := e.pickPreferred(modelName, preferred); acc != nil {
				return acc
			}
		}
	}
	if !e.order.Active() {
		return e.pickFor(modelName)
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	now := time.Now()
	available := make([]*workbuddyAccount, 0, len(e.accounts))
	supported := make([]*workbuddyAccount, 0, len(e.accounts))
	modelName = strings.TrimSpace(modelName)
	for _, acc := range e.accounts {
		acc.mu.Lock()
		ok := acc.available(now)
		// deepseek-v4.1-flash 被限流的账号，该模型在重置前不再选它。
		if ok && dsModelName(modelName) && now.Before(acc.dsRestUntil) {
			ok = false
		}
		matches := false
		if ok {
			endpoint, site := e.resolve(acc.creds)
			matches = siteSupports(site, IsLegacyCodeBuddy(endpoint), modelName)
		}
		acc.mu.Unlock()
		if !ok {
			continue
		}
		available = append(available, acc)
		if matches {
			supported = append(supported, acc)
		}
	}
	// 目录里没有账号声明该模型时退回全部可用账号（目录只是兜底快照）。
	if len(supported) > 0 {
		available = supported
	}
	e.order.Sort(available, func(acc *workbuddyAccount) string { return acc.id })
	for _, acc := range available {
		if _, skip := tried[acc.id]; skip {
			continue
		}
		return acc
	}
	return nil
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

// modelDefaultEffort 返回模型在 /v3/config 里的默认推理档位（读不到返回 ""）。
// 上游对不带 reasoning_effort 的请求默认不思考（模型仍会答，但不思考）。
func (e *Engine) modelDefaultEffort(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || modelName == "auto" {
		return ""
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, acc := range e.accounts {
		acc.mu.Lock()
		effort := configDefaultEffort(acc.config, modelName)
		acc.mu.Unlock()
		if effort != "" {
			return effort
		}
	}
	return ""
}

func configDefaultEffort(models []ConfigModel, id string) string {
	for _, m := range models {
		if m.ID == id {
			return m.DefaultEffort
		}
	}
	return ""
}

// pickPreferred 按 ID 取会话粘性指定的账号：必须当前可用且支持该模型，
// 否则返回 nil 交给正常选号，粘性不阻断故障切换。
func (e *Engine) pickPreferred(modelName, id string) *workbuddyAccount {
	modelName = strings.TrimSpace(modelName)
	e.mu.RLock()
	defer e.mu.RUnlock()
	now := time.Now()
	for _, acc := range e.accounts {
		if acc.id != id {
			continue
		}
		acc.mu.Lock()
		ok := acc.available(now)
		if ok && dsModelName(modelName) && now.Before(acc.dsRestUntil) {
			ok = false
		}
		supported := true
		if ok && modelName != "" && modelName != "auto" {
			endpoint, site := e.resolve(acc.creds)
			supported = siteSupports(site, IsLegacyCodeBuddy(endpoint), modelName)
		}
		acc.mu.Unlock()
		if ok && supported {
			return acc
		}
		return nil
	}
	return nil
}

// pickWhere 按可选过滤条件选号（filter 在账号锁内调用，读到的 creds 是一致快照）。
func (e *Engine) pickWhere(modelName string, filter func(site Site, legacy bool) bool) *workbuddyAccount {
	e.mu.RLock()
	defer e.mu.RUnlock()
	now := time.Now()
	var best *workbuddyAccount
	var bestAt time.Time
	var candidates []*workbuddyAccount
	for _, acc := range e.accounts {
		acc.mu.Lock()
		ok := acc.available(now)
		// deepseek-v4.1-flash 被限流的账号，该模型在重置前不再选它（其他模型不受影响）。
		if ok && dsModelName(modelName) && now.Before(acc.dsRestUntil) {
			ok = false
		}
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
		e.setQuota(acc, e.withDsWindow(acc, &Quota{Err: shortErr(err), ReadAt: time.Now()}))
		return
	}
	acc.mu.Lock()
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()
	e.setQuota(acc, e.withDsWindow(acc, fetchQuota(ctx, proxy, auth, "")))
}

// withDsWindow 把 deepseek-v4.1-flash 的专属 token 窗口并进额度快照（展示用）。
// 积分接口读不到这个窗口，所以这里用本账号的应答记录估算；有拒绝则钉满。
func (e *Engine) withDsWindow(acc *workbuddyAccount, q *Quota) *Quota {
	if q == nil {
		return q
	}
	now := time.Now()
	acc.mu.Lock()
	rows := append([]dsRow(nil), acc.dsRows...)
	rest := acc.dsRestUntil
	if !rest.IsZero() && !rest.After(now) {
		rest, acc.dsRestUntil = time.Time{}, time.Time{}
	}
	_, site := e.resolve(acc.creds)
	acc.mu.Unlock()
	if w := dsWindow(rows, rest, now, dsSpec(site)); w != nil {
		q.Windows = append(q.Windows, *w)
	}
	return q
}

func (e *Engine) setQuota(acc *workbuddyAccount, q *Quota) {
	acc.mu.Lock()
	acc.quota = q
	acc.mu.Unlock()
}

// CheckIn 立即签到一次，返回结果文案。幂等：今日已签到时直接返回已签到状态。
// 国际服没有签到活动，直接返回说明。
func (e *Engine) CheckIn(id string) (string, error) {
	e.mu.RLock()
	idx, ok := e.ids[id]
	if !ok {
		e.mu.RUnlock()
		return "", fmt.Errorf("账号 %s 不在池中", id)
	}
	acc := e.accounts[idx]
	e.mu.RUnlock()

	acc.mu.Lock()
	_, site := e.resolve(acc.creds)
	acc.mu.Unlock()
	if site.ID != SiteCN.ID {
		return "国际服 WorkBuddy AI 没有签到活动", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	auth, err := e.freshAuth(ctx, acc)
	if err != nil {
		return "", err
	}
	acc.mu.Lock()
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()
	msg, err := checkin(ctx, proxy, auth)
	if err != nil {
		return "", err
	}
	e.setCheckin(acc, msg)
	return msg, nil
}

// SetAutoCheckIn 开关自动签到（持久化到凭据，重启后保留）。仅国服有效。
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
	// 打开时立即补签今日；关闭时不补签。
	if enabled {
		go e.checkinAccount(acc)
	}
	return nil
}

// checkinLoop 每 30 分钟对全部国服账号尝试一次当日签到（每天每账号至多一次）。
func (e *Engine) checkinLoop() {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-e.done:
			return
		case <-t.C:
			e.checkinAll()
		}
	}
}

func (e *Engine) checkinAll() {
	e.mu.RLock()
	accs := make([]*workbuddyAccount, len(e.accounts))
	copy(accs, e.accounts)
	e.mu.RUnlock()
	now := time.Now()
	for _, acc := range accs {
		acc.mu.Lock()
		ok := acc.enabled && acc.ready
		sameDay := !acc.checkinAt.IsZero() && acc.checkinAt.Year() == now.Year() && acc.checkinAt.YearDay() == now.YearDay()
		acc.mu.Unlock()
		if !ok || sameDay {
			continue
		}
		e.checkinAccount(acc)
	}
}

// checkinAccount 对一个账号尝试签到：非国服或未开启自动签到则跳过；结果写入
// 展示文案，失败不影响账号 ready/接单。
func (e *Engine) checkinAccount(acc *workbuddyAccount) {
	acc.mu.Lock()
	_, site := e.resolve(acc.creds)
	enabled := acc.creds.AutoCheckInEnabled()
	acc.mu.Unlock()
	if site.ID != SiteCN.ID || !enabled {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	auth, err := e.freshAuth(ctx, acc)
	if err != nil {
		e.setCheckin(acc, "签到失败: "+shortErr(err))
		return
	}
	acc.mu.Lock()
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()
	msg, err := checkin(ctx, proxy, auth)
	if err != nil {
		msg = "签到失败: " + clipRunes(err.Error(), 80)
	}
	e.setCheckin(acc, msg)
}

func (e *Engine) setCheckin(acc *workbuddyAccount, msg string) {
	acc.mu.Lock()
	acc.checkinMsg = msg
	acc.checkinAt = time.Now()
	acc.mu.Unlock()
}

// refreshConfig 异步读取账号的 /v3/config 模型目录（含免费标识）。并发去重。
func (e *Engine) refreshConfig(acc *workbuddyAccount) {
	acc.mu.Lock()
	if acc.refreshingCfg {
		acc.mu.Unlock()
		return
	}
	acc.refreshingCfg = true
	acc.mu.Unlock()
	defer func() {
		acc.mu.Lock()
		acc.refreshingCfg = false
		acc.configAt = time.Now()
		acc.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	auth, err := e.freshAuth(ctx, acc)
	if err != nil {
		acc.mu.Lock()
		acc.configErr = shortErr(err)
		acc.mu.Unlock()
		return
	}
	acc.mu.Lock()
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()
	models, err := fetchConfig(ctx, proxy, auth)
	acc.mu.Lock()
	if err != nil {
		acc.configErr = shortErr(err)
	} else {
		acc.config, acc.configErr = models, ""
	}
	acc.mu.Unlock()
}

func (acc *workbuddyAccount) configStale(d time.Duration) bool {
	acc.mu.Lock()
	defer acc.mu.Unlock()
	return len(acc.config) == 0 || time.Since(acc.configAt) > d
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
	// 同一会话复用上游会话 ID：WorkBuddy 的 prompt cache 按会话归并，
	// 每次新 ID 都会让前缀缓存无法命中（缓存字段始终为 0）。
	if conv := engine.ConversationID(ctx); conv != "" {
		cReq.ConversationID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("web2api:"+conv)).String()
	}
	// 带默认推理档位（与 magpie 一致）：max_tokens ≤ 1024 的短答不思考
	// （否则预算全花在推理上、正文空），其余按模型的 config 默认档位下发。
	if cReq.ReasoningEffort == "" && (cReq.MaxTokens == 0 || cReq.MaxTokens > 1024) {
		cReq.ReasoningEffort = e.modelDefaultEffort(req.Model)
	}
	var lastErr error
	tried := map[string]struct{}{}
	for attempt := 0; attempt < e.usableCount(); attempt++ {
		acc := e.pickForExcept(ctx, req.Model, tried)
		if acc == nil {
			break
		}
		tried[acc.id] = struct{}{}
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
			if dsModelName(req.Model) {
				acc.addDsRow(time.Now(), float64(res.PromptTokens+res.CompletionTokens))
			}
			if acc.quotaStale(5 * time.Minute) {
				go e.refreshQuota(acc)
			}
			engine.NoteServedAccount(ctx, acc.id)
			return &model.ChatResult{
				Text:        res.Text,
				ToolCalls:   res.ToolCalls,
				Engine:      e.name,
				Model:       req.Model,
				ActualModel: res.Model,
				Usage:       engine.SubResultUsageCached(res.PromptTokens, res.CompletionTokens, 0, res.CachedTokens),
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
			// deepseek-v4.1-flash 的 429 带专属 token 窗口的重置时刻：记下来，
			// 供额度卡片展示，并在选号时停掉该模型的这个账号直到重置。
			if dsModelName(req.Model) {
				if reset := windowReset(err.Error()); !reset.IsZero() && reset.After(time.Now()) {
					acc.recordDsRefusal(reset)
					e.markCooldown(acc, "deepseek-v4.1-flash 限流至 "+reset.Format(time.RFC3339), 3*time.Minute)
					go e.refreshQuota(acc)
					continue
				}
			}
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
