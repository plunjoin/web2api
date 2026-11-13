package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"web2api/internal/config"
	"web2api/internal/engine"
	aistudio "web2api/internal/engine/aistudio"
	"web2api/internal/engine/antigravity"
	"web2api/internal/engine/codex"
	"web2api/internal/engine/custom"
	"web2api/internal/engine/geminiweb"
	"web2api/internal/engine/kiro"
	"web2api/internal/engine/teleagent"
	"web2api/internal/engine/upstream"
	"web2api/internal/engine/workbuddy"
	"web2api/internal/model"
	"web2api/internal/store"
)

// Manager 引擎注册表、模型路由与号池动态管理。
type Manager struct {
	mu sync.RWMutex

	st  *store.Store
	cfg *config.Config

	// 引擎A/B 为历史双引擎，保留专用字段以兼容 Veo 等能力接口。
	engineA *geminiweb.Engine
	engineB initializableEngine

	// 全部引擎（含 a/b）的注册表与顺序。
	engineOrder []string
	engines     map[string]engine.Engine
	// 有账号池的引擎（号池动态管理统一分发）。
	pools map[string]accountPool

	defaultEngine   string
	geminiWebModels []string
	aiStudioModels  []string
	// 新引擎（codex/antigravity/kiro/workbuddy）的模型路由配置。
	engineModels map[string][]string

	// 管理台保存的账号路由顺序（全局账号 ID 列表）与供应商优先级缓存。
	routeMu       sync.RWMutex
	routeIDs      []string
	routeSupplier map[string]int
	// routeEngine 账号 ID → 引擎名，供会话粘性定位账号所属供应商。
	routeEngine map[string]string

	// 会话粘性（会话键 → 上次接单账号）。
	affinity affinityCache

	// 管理台「获取模型」保存的目录快照归属（canonical model → engine），带 TTL 缓存。
	// 实时目录会随上游 /v3/config、账号重连等波动，快照是持久的归属记录。
	snapMu       sync.RWMutex
	snapOwner    map[string]string
	snapLoadedAt time.Time
}

// accountPool 有账号池的引擎统一接口（引擎A/B 与订阅引擎都实现）。
type accountPool interface {
	RemoveAccount(engineID string) error
	SetEnabled(engineID string, enabled bool) error
	AccountStates() []engine.AccountState
}

func (m *Manager) GeminiAPIConfig() config.GeminiAPIConfig { return m.cfg.GeminiAPI }

func (m *Manager) Close() {
	for _, name := range m.engineOrder {
		if eng, ok := m.engines[name]; ok {
			if closer, ok := eng.(interface{ Close() }); ok {
				closer.Close()
			}
		}
	}
}

// register 登记引擎与其账号池能力。
func (m *Manager) register(name string, eng engine.Engine, pool accountPool) {
	if eng == nil {
		return
	}
	if m.engines[name] != nil {
		return
	}
	m.engines[name] = eng
	m.engineOrder = append(m.engineOrder, name)
	if pool != nil {
		m.pools[name] = pool
	}
}

// Engines 全部引擎。
func (m *Manager) Engines() map[string]engine.Engine {
	out := make(map[string]engine.Engine, len(m.engines))
	for k, v := range m.engines {
		out[k] = v
	}
	return out
}

// EngineMeta 管理台引擎元数据。
type EngineMeta struct {
	Name        string `json:"name"`
	Describe    string `json:"describe"`
	Enabled     bool   `json:"enabled"`
	AccountPool bool   `json:"account_pool"`
	OAuth       string `json:"oauth,omitempty"`      // codex/antigravity=authorize+code；workbuddy=start+poll
	Credential  string `json:"credential,omitempty"` // 凭据字段说明
}

// initializableEngine 需要异步初始化的引擎。
type initializableEngine interface {
	engine.Engine
	Init(ctx context.Context) error
}

// AccountOverview 管理台账号视图：存储记录 + 引擎实时状态。
type AccountOverview struct {
	store.Account
	Live engine.AccountState `json:"live"`
	// Provider 是第三方 Key 的供应商身份，即归一化后的 base_url。同一地址的
	// 多个 Key 属于同一个供应商，管理台据此分组；内置引擎为空。
	// 不使用余额查询返回的厂商名：那个名字取决于上游是否应答，会在刷新前后变化。
	Provider string `json:"provider,omitempty"`
}

// AccountInput 是统一账号添加协议。引擎A/B 沿用 storage-state JSON；
// 订阅引擎（codex/antigravity/kiro/workbuddy）使用 credentials JSON 对象。
type AccountInput struct {
	Engine       string          `json:"engine"`
	Label        string          `json:"label"`
	Email        string          `json:"email"`
	StorageState string          `json:"storage_state"`
	Locale       string          `json:"locale,omitempty"`
	Timezone     string          `json:"timezone,omitempty"`
	Proxy        string          `json:"proxy,omitempty"`
	Credentials  json.RawMessage `json:"credentials,omitempty"` // 订阅引擎凭据对象
}

// NewManager 构建引擎管理器。
func NewManager(cfg *config.Config, st *store.Store) (*Manager, error) {
	m := &Manager{
		st:              st,
		cfg:             cfg,
		defaultEngine:   strings.ToLower(cfg.Routing.DefaultEngine),
		geminiWebModels: cfg.Routing.GeminiWebModels,
		aiStudioModels:  cfg.Routing.AIStudioModels,
		engineModels:    cfg.Routing.EngineModels,
		engines:         map[string]engine.Engine{},
		pools:           map[string]accountPool{},
	}
	if m.defaultEngine == "" {
		m.defaultEngine = "auto"
	}

	if cfg.GeminiWeb.Enabled {
		ea, err := geminiweb.New(cfg.GeminiWeb)
		if err != nil {
			return nil, fmt.Errorf("引擎A 初始化失败: %w", err)
		}
		m.engineA = ea
		ea.SetStore(st)
		m.register("geminiweb", ea, ea)
	}
	if cfg.AIStudio.Enabled {
		mode := strings.ToLower(strings.TrimSpace(cfg.AIStudio.Mode))
		if mode == "" {
			mode = "native"
		}
		var eb initializableEngine
		if mode == "native" {
			native, err := aistudio.NewNative(cfg.AIStudio)
			if err != nil {
				return nil, err
			}
			eb = native
		} else {
			upstreamEngine, err := aistudio.New(cfg.AIStudio, 300*time.Second)
			if err != nil {
				return nil, err
			}
			eb = upstreamEngine
		}
		if eb != nil {
			m.engineB = eb
			m.register("aistudio", eb, nil)
			// native 模式的账号池能力在具体实例上
			if native, ok := eb.(*aistudio.NativeEngine); ok && native != nil {
				m.pools["aistudio"] = native
			}
		}
	}
	// 订阅引擎：codex / antigravity / kiro / workbuddy
	if cfg.Engines.Codex.Enabled {
		ec := codex.New(cfg.Engines.Codex, st)
		m.register(ec.Name(), ec, ec)
	}
	if cfg.Engines.Antigravity.Enabled {
		ea := antigravity.New(cfg.Engines.Antigravity, st)
		m.register(ea.Name(), ea, ea)
	}
	if cfg.Engines.Kiro.Enabled {
		ek := kiro.New(cfg.Engines.Kiro, st)
		m.register(ek.Name(), ek, ek)
	}
	if cfg.Engines.WorkBuddy.Enabled {
		eb := workbuddy.New(cfg.Engines.WorkBuddy, st)
		m.register(eb.Name(), eb, eb)
	}
	if cfg.Engines.TeleAgent.On() {
		et := teleagent.New(cfg.Engines.TeleAgent, st)
		m.register(et.Name(), et, et)
	}
	if cfg.Engines.Custom.Enabled {
		ec := custom.New(cfg.Engines.Custom, st)
		m.register(ec.Name(), ec, ec)
	}
	if m.engineA == nil && m.engineB == nil && !cfg.GeminiAPI.Enabled && len(m.engines) == 0 {
		return nil, errors.New("至少启用一个后端（engine_a、engine_b、engines 或 gemini_api）")
	}
	return m, nil
}

// Init 初始化各引擎并装配号池。
func (m *Manager) Init(ctx context.Context) error {
	var errs []string
	// 引擎A 无需整体初始化：账号由 bootstrapAccounts 动态注入并异步自检
	if m.engineB != nil {
		if err := m.engineB.Init(ctx); err != nil {
			errs = append(errs, "引擎B: "+err.Error())
		}
	}
	if err := m.bootstrapAccounts(ctx); err != nil {
		errs = append(errs, "号池装配: "+err.Error())
	}
	// 账号装配完成后再下发管理台保存的路由顺序，新增账号顺延到末尾。
	m.ReloadRoutingOrder()
	m.ReloadAffinity()
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// bootstrapAccounts 启动装配：存储 → 号池（含 config 静态账号迁移、auth 目录反向导入）。
func (m *Manager) bootstrapAccounts(ctx context.Context) error {
	// 先把历史引擎名（a/b）写回规范名并合并重复行，否则同一个账号会在管理台
	// 出现两行，其中一行因为号池里没有对应 ID 而永远显示「未加载」。
	_, _ = m.st.NormalizeLegacyEngines()

	accounts, err := m.st.ListAccounts()
	if err != nil {
		return fmt.Errorf("读取账号存储失败: %w", err)
	}

	// 首次启动：把 config 里的静态账号迁移进存储（作为种子）
	if len(accounts) == 0 {
		for _, acc := range m.cfg.GeminiWeb.Accounts {
			if strings.TrimSpace(acc.PSID) == "" {
				continue
			}
			creds, _ := json.Marshal(geminiweb.Credentials{PSID: acc.PSID, PSIDTS: acc.PSIDTS})
			if _, err := m.st.CreateAccount("geminiweb", seedLabel("geminiweb", acc.Name), string(creds), true); err != nil {
				continue
			}
		}
		// 订阅引擎种子（codex / antigravity / kiro / workbuddy）
		for _, acc := range m.cfg.Engines.Codex.Accounts {
			if strings.TrimSpace(acc.RefreshToken) == "" {
				continue
			}
			creds := engine.MarshalCompact(codex.Credentials{RefreshToken: acc.RefreshToken, Email: acc.Email})
			_, _ = m.st.CreateAccount("codex", seedLabel("codex", firstNonEmpty(acc.Name, acc.Email)), creds, true)
		}
		for _, acc := range m.cfg.Engines.Antigravity.Accounts {
			if strings.TrimSpace(acc.RefreshToken) == "" {
				continue
			}
			creds := engine.MarshalCompact(antigravity.Credentials{RefreshToken: acc.RefreshToken, Email: acc.Email, ProjectID: acc.ProjectID})
			_, _ = m.st.CreateAccount("antigravity", seedLabel("antigravity", firstNonEmpty(acc.Name, acc.Email)), creds, true)
		}
		for _, acc := range m.cfg.Engines.Kiro.Accounts {
			if strings.TrimSpace(acc.RefreshToken) == "" {
				continue
			}
			creds := engine.MarshalCompact(kiro.Credentials{RefreshToken: acc.RefreshToken, Email: acc.Email})
			_, _ = m.st.CreateAccount("kiro", seedLabel("kiro", firstNonEmpty(acc.Name, acc.Email)), creds, true)
		}
		for _, acc := range m.cfg.Engines.WorkBuddy.Accounts {
			if strings.TrimSpace(acc.Token) == "" && strings.TrimSpace(acc.RefreshToken) == "" {
				continue
			}
			creds := engine.MarshalCompact(workbuddy.Credentials{
				Token: acc.Token, RefreshToken: acc.RefreshToken, Email: acc.Email,
				Site: acc.Site, UID: acc.UID, Domain: acc.Domain, EnterpriseID: acc.EnterpriseID,
				BaseURL: acc.BaseURL, Proxy: acc.Proxy,
			})
			_, _ = m.st.CreateAccount("workbuddy", seedLabel("workbuddy", firstNonEmpty(acc.Name, acc.Email)), creds, true)
		}
		for _, acc := range m.cfg.Engines.TeleAgent.Accounts {
			if strings.TrimSpace(acc.Token) == "" {
				continue
			}
			creds := engine.MarshalCompact(teleagent.Credentials{
				Token: acc.Token, DeviceID: acc.DeviceID, InstallID: acc.InstallID,
				APIKey: acc.APIKey, AppVersion: acc.AppVersion, Scope: acc.Scope, Email: acc.Email, Proxy: acc.Proxy,
			})
			_, _ = m.st.CreateAccount("teleagent", seedLabel("teleagent", firstNonEmpty(acc.Name, acc.Email)), creds, true)
		}
		for _, acc := range m.cfg.Engines.Custom.Accounts {
			if strings.TrimSpace(acc.BaseURL) == "" || strings.TrimSpace(acc.APIKey) == "" {
				continue
			}
			creds := engine.MarshalCompact(custom.Credentials{BaseURL: acc.BaseURL, APIKey: acc.APIKey, Models: acc.Models})
			_, _ = m.st.CreateAccount("custom", seedLabel("custom", firstNonEmpty(acc.Name, acc.BaseURL)), creds, true)
		}
		accounts, err = m.st.ListAccounts()
		if err != nil {
			return err
		}
	}

	for _, acc := range accounts {
		switch acc.Engine {
		case "a", "geminiweb":
			if m.engineA == nil {
				continue
			}
			var creds geminiweb.Credentials
			if err := json.Unmarshal([]byte(acc.Credentials), &creds); err != nil {
				continue
			}
			_ = m.engineA.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds.PSID, creds.PSIDTS)
			if !acc.Enabled {
				_ = m.engineA.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
			}
		case "b", "aistudio":
			native, ok := m.engineB.(*aistudio.NativeEngine)
			if !ok || native == nil {
				continue
			}
			var creds aistudio.Credentials
			if err := json.Unmarshal([]byte(acc.Credentials), &creds); err != nil {
				continue
			}
			id := fmt.Sprintf("%d", acc.ID)
			if len(creds.StorageState) > 0 {
				if err := native.AttachOrAdd(id, acc.Label, string(creds.StorageState), creds.Locale, creds.Timezone, creds.Proxy); err != nil {
					_ = m.st.UpdateAccountStatus(acc.ID, "error", err.Error())
				}
			} else if poolHas(native, acc.Label) {
				native.RegisterAccountID(id, acc.Label)
			}
			if !acc.Enabled {
				_ = native.SetEnabled(id, false)
			}
		}
	}

	// 反向导入：auth 目录已存在但存储没有的账号（已有的 auth 目录直接复用）
	if native, ok := m.engineB.(*aistudio.NativeEngine); ok && native != nil {
		for _, poolID := range native.PoolAccountIDs() {
			if _, err := m.st.GetAccountByLabel("aistudio", poolID); err == nil {
				continue
			}
			if _, err := m.st.CreateAccount("aistudio", poolID, "{}", true); err == nil {
				native.RegisterAccountID(fmt.Sprintf("%d", lastAccountID(m.st)), poolID)
			}
		}
	}

	// 订阅引擎账号装配（codex / antigravity / kiro / workbuddy）
	for _, acc := range accounts {
		switch acc.Engine {
		case "codex":
			if eng := m.codexEngine(); eng != nil {
				var creds codex.Credentials
				if json.Unmarshal([]byte(acc.Credentials), &creds) == nil && creds.RefreshToken != "" {
					_ = eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds)
					if !acc.Enabled {
						_ = eng.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
					}
				}
			}
		case "antigravity":
			if eng := m.antigravityEngine(); eng != nil {
				var creds antigravity.Credentials
				if json.Unmarshal([]byte(acc.Credentials), &creds) == nil && creds.RefreshToken != "" {
					_ = eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds)
					if !acc.Enabled {
						_ = eng.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
					}
				}
			}
		case "kiro":
			if eng := m.kiroEngine(); eng != nil {
				var creds kiro.Credentials
				if json.Unmarshal([]byte(acc.Credentials), &creds) == nil && creds.RefreshToken != "" {
					_ = eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds)
					if !acc.Enabled {
						_ = eng.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
					}
				}
			}
		case "workbuddy":
			if eng := m.mWorkBuddyEngine(); eng != nil {
				var creds workbuddy.Credentials
				if json.Unmarshal([]byte(acc.Credentials), &creds) == nil && creds.Token != "" {
					_ = eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds)
					if !acc.Enabled {
						_ = eng.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
					}
				}
			}
		case "teleagent":
			if eng := m.teleagentEngine(); eng != nil {
				var creds teleagent.Credentials
				if json.Unmarshal([]byte(acc.Credentials), &creds) == nil && creds.Token != "" {
					_ = eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds)
					if !acc.Enabled {
						_ = eng.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
					}
				}
			}
		case "custom":
			if eng := m.customEngine(); eng != nil {
				var creds custom.Credentials
				if json.Unmarshal([]byte(acc.Credentials), &creds) == nil && creds.BaseURL != "" && creds.APIKey != "" {
					_ = eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds)
					if !acc.Enabled {
						_ = eng.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
					}
				}
			}
		}
	}
	return nil
}

// codexEngine / antigravityEngine / kiroEngine / mWorkBuddyEngine 返回订阅引擎实例。
func (m *Manager) codexEngine() *codex.Engine {
	if eng, ok := m.engines["codex"]; ok {
		if c, ok := eng.(*codex.Engine); ok {
			return c
		}
	}
	return nil
}

func (m *Manager) antigravityEngine() *antigravity.Engine {
	if eng, ok := m.engines["antigravity"]; ok {
		if a, ok := eng.(*antigravity.Engine); ok {
			return a
		}
	}
	return nil
}

func (m *Manager) kiroEngine() *kiro.Engine {
	if eng, ok := m.engines["kiro"]; ok {
		if k, ok := eng.(*kiro.Engine); ok {
			return k
		}
	}
	return nil
}

func (m *Manager) mWorkBuddyEngine() *workbuddy.Engine {
	if eng, ok := m.engines["workbuddy"]; ok {
		if w, ok := eng.(*workbuddy.Engine); ok {
			return w
		}
	}
	return nil
}

func (m *Manager) teleagentEngine() *teleagent.Engine {
	if eng, ok := m.engines["teleagent"]; ok {
		if t, ok := eng.(*teleagent.Engine); ok {
			return t
		}
	}
	return nil
}

func (m *Manager) customEngine() *custom.Engine {
	if eng, ok := m.engines["custom"]; ok {
		if c, ok := eng.(*custom.Engine); ok {
			return c
		}
	}
	return nil
}

func poolHas(native *aistudio.NativeEngine, accountID string) bool {
	for _, id := range native.PoolAccountIDs() {
		if strings.EqualFold(id, accountID) {
			return true
		}
	}
	return false
}

func lastAccountID(st *store.Store) int64 {
	accounts, _ := st.ListAccounts()
	if len(accounts) == 0 {
		return 0
	}
	return accounts[len(accounts)-1].ID
}

func seedLabel(engine, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return engine + "-账号"
	}
	return name
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// EngineA / EngineB 返回引擎（未启用时返回 nil 接口）。
func (m *Manager) GeminiWeb() engine.Engine {
	if m.engineA == nil {
		return nil
	}
	return m.engineA
}

func (m *Manager) AIStudio() engine.Engine {
	if m.engineB == nil {
		return nil
	}
	return m.engineB
}

// EngineForModel 按当前路由规则预览模型会落到哪个引擎（用量失败记录用）。
// 不保证该引擎此刻 Ready；引擎未命中时返回空串。
func (m *Manager) EngineForModel(modelName string) string {
	eng, _ := m.pickEngine(modelName)
	if eng == nil {
		return ""
	}
	return eng.Name()
}

// EngineReady 指定引擎是否就绪（未注册返回 false）。
func (m *Manager) EngineReady(name string) bool {
	eng, ok := m.engines[name]
	if !ok {
		return false
	}
	return eng.Ready()
}

// EngineReadyMap 全部引擎的就绪状态（engine_a/engine_b 含在其中）。
func (m *Manager) EngineReadyMap() map[string]bool {
	out := make(map[string]bool, len(m.engines))
	for name, eng := range m.engines {
		out[name] = eng.Ready()
	}
	return out
}

// EngineProxy 返回引擎的全局代理配置（账号级代理优先级在引擎内部处理）。
func (m *Manager) EngineProxy(name string) string {
	switch name {
	case "a", "geminiweb":
		return m.cfg.GeminiWeb.Proxy
	case "b", "aistudio":
		return m.cfg.AIStudio.Proxy
	case "codex":
		return m.cfg.Engines.Codex.Proxy
	case "antigravity":
		return m.cfg.Engines.Antigravity.Proxy
	case "kiro":
		return m.cfg.Engines.Kiro.Proxy
	case "workbuddy":
		return m.cfg.Engines.WorkBuddy.Proxy
	case "custom":
		return m.cfg.Engines.Custom.Proxy
	}
	return ""
}

// WorkBuddyBaseURL workbuddy 引擎的上游地址。
func (m *Manager) WorkBuddyBaseURL() string {
	if u := strings.TrimSpace(m.cfg.Engines.WorkBuddy.BaseURL); u != "" {
		return u
	}
	return workbuddy.DefaultBaseURL
}

// mergeWorkBuddyCreds 用新凭据的非空字段覆盖旧凭据，其余保留。
// ExpiresAt 以新提交为准：手填 token 的到期时间未知（0），交给上游 401 兜底，
// 而不是沿用旧 token 的时间导致刚换上就判定过期。
func mergeWorkBuddyCreds(old, next workbuddy.Credentials) workbuddy.Credentials {
	if next.Site == "" {
		next.Site = old.Site
	}
	if next.UID == "" {
		next.UID = old.UID
	}
	if next.Domain == "" {
		next.Domain = old.Domain
	}
	if next.EnterpriseID == "" {
		next.EnterpriseID = old.EnterpriseID
	}
	if next.BaseURL == "" {
		next.BaseURL = old.BaseURL
	}
	if next.Proxy == "" {
		next.Proxy = old.Proxy
	}
	if next.Email == "" {
		next.Email = old.Email
	}
	if next.RefreshToken == "" {
		next.RefreshToken = old.RefreshToken
	}
	return next
}

// ==================== OAuth 授权会话 ====================

// OAuthPendingSession 管理台发起的 OAuth 授权会话（authorize → exchange）。
type OAuthPendingSession struct {
	Engine       string `json:"engine"`
	SessionID    string `json:"session_id"`
	State        string `json:"-"`
	CodeVerifier string `json:"-"`
	AuthURL      string `json:"auth_url"`
	Proxy        string `json:"-"`
	// Site 是 workbuddy 的发行版（workbuddy / workbuddy-ai）；其它引擎为空。
	Site      string    `json:"-"`
	CreatedAt time.Time `json:"-"`
}

// oauthSessions OAuth 会话注册表（内存，30 分钟过期）。
var (
	oauthSessionsMu sync.Mutex
	oauthSessions   = map[string]*OAuthPendingSession{}
)

// SaveOAuthSession 登记授权会话。
func SaveOAuthSession(sess *OAuthPendingSession) {
	oauthSessionsMu.Lock()
	defer oauthSessionsMu.Unlock()
	// 清理过期
	for k, v := range oauthSessions {
		if time.Since(v.CreatedAt) > 30*time.Minute {
			delete(oauthSessions, k)
		}
	}
	oauthSessions[sess.SessionID] = sess
}

// PeekOAuthSession 读取授权会话但不删除。设备授权轮询会反复查同一个会话，
// 又要在 poll 时知道用户选的是国服还是国际服，所以需要只读入口。
func PeekOAuthSession(engine, sessionID string) (*OAuthPendingSession, bool) {
	oauthSessionsMu.Lock()
	defer oauthSessionsMu.Unlock()
	sess, ok := oauthSessions[sessionID]
	if !ok || (engine != "" && sess.Engine != engine) || time.Since(sess.CreatedAt) > 30*time.Minute {
		return nil, false
	}
	return sess, true
}

// TakeOAuthSession 取出并删除授权会话（一次性）。
func TakeOAuthSession(engine, sessionID string) (*OAuthPendingSession, error) {
	oauthSessionsMu.Lock()
	defer oauthSessionsMu.Unlock()
	sess, ok := oauthSessions[sessionID]
	if !ok {
		return nil, errors.New("授权会话不存在或已过期（30 分钟内有效）")
	}
	delete(oauthSessions, sessionID)
	if engine != "" && sess.Engine != engine {
		return nil, fmt.Errorf("授权会话属于引擎 %s", sess.Engine)
	}
	if time.Since(sess.CreatedAt) > 30*time.Minute {
		return nil, errors.New("授权会话已过期，请重新发起")
	}
	return sess, nil
}

// ==================== 号池动态管理 ====================

// AddAccountFromStorageState 是统一账号添加入口。引擎A/B 提交 storage_state；
// 订阅引擎（codex/antigravity/kiro/workbuddy）提交 credentials 凭据对象。
func (m *Manager) AddAccountFromStorageState(req AccountInput) (store.Account, error) {
	acc, err := m.addAccountFromStorageState(req)
	if err == nil {
		// 新账号不抢占既有顺序，按创建顺序追加到末尾。
		m.ReloadRoutingOrder()
	}
	return acc, err
}

func (m *Manager) addAccountFromStorageState(req AccountInput) (store.Account, error) {
	engineName := strings.ToLower(strings.TrimSpace(req.Engine))
	if engineName == "" {
		engineName = "aistudio"
	}
	switch engineName {
	case "a", "gemini", "geminiweb", "b", "aistudio":
		if strings.TrimSpace(req.StorageState) == "" {
			return store.Account{}, errors.New("storage_state 不能为空")
		}
	}
	switch engineName {
	case "a", "gemini", "geminiweb":
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = strings.TrimSpace(req.Email)
		}
		if label == "" {
			return store.Account{}, errors.New("label 或 email 至少填写一个")
		}
		creds, err := geminiCredentialsFromStorageState(req.StorageState)
		if err != nil {
			return store.Account{}, err
		}
		return m.addGeminiCredentials(label, creds.PSID, creds.PSIDTS)
	case "b", "aistudio":
		if strings.TrimSpace(req.Email) == "" {
			return store.Account{}, errors.New("email 不能为空")
		}
		return m.addAIStudioAccount(req.Email, req.StorageState, req.Locale, req.Timezone, req.Proxy)
	case "codex":
		var creds codex.Credentials
		if err := decodeEngineCredentials(req, &creds); err != nil {
			return store.Account{}, err
		}
		if creds.RefreshToken == "" {
			return store.Account{}, errors.New("refresh_token 不能为空")
		}
		if creds.Email == "" {
			creds.Email = strings.TrimSpace(req.Email)
		}
		if creds.Proxy == "" {
			creds.Proxy = strings.TrimSpace(req.Proxy)
		}
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = strings.TrimSpace(creds.Email)
		}
		return m.addCodexAccount(label, creds)
	case "antigravity":
		var creds antigravity.Credentials
		if err := decodeEngineCredentials(req, &creds); err != nil {
			return store.Account{}, err
		}
		if creds.RefreshToken == "" {
			return store.Account{}, errors.New("refresh_token 不能为空")
		}
		if creds.Email == "" {
			creds.Email = strings.TrimSpace(req.Email)
		}
		if creds.Proxy == "" {
			creds.Proxy = strings.TrimSpace(req.Proxy)
		}
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = strings.TrimSpace(creds.Email)
		}
		return m.addAntigravityAccount(label, creds)
	case "kiro":
		var creds kiro.Credentials
		if err := decodeEngineCredentials(req, &creds); err != nil {
			return store.Account{}, err
		}
		if creds.RefreshToken == "" {
			return store.Account{}, errors.New("refresh_token 不能为空")
		}
		if creds.Email == "" {
			creds.Email = strings.TrimSpace(req.Email)
		}
		if creds.Proxy == "" {
			creds.Proxy = strings.TrimSpace(req.Proxy)
		}
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = strings.TrimSpace(creds.Email)
		}
		return m.addKiroAccount(label, creds)
	case "workbuddy":
		var creds workbuddy.Credentials
		if err := decodeEngineCredentials(req, &creds); err != nil {
			return store.Account{}, err
		}
		if creds.Token == "" {
			return store.Account{}, errors.New("token 不能为空")
		}
		if creds.Email == "" {
			creds.Email = strings.TrimSpace(req.Email)
		}
		if creds.Email == "" {
			creds.Email = workbuddy.JWTEmail(creds.Token)
		}
		if creds.Proxy == "" {
			creds.Proxy = strings.TrimSpace(req.Proxy)
		}
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = strings.TrimSpace(creds.Email)
		}
		return m.addWorkBuddyAccount(label, creds)
	case "custom":
		var creds custom.Credentials
		if err := decodeEngineCredentials(req, &creds); err != nil {
			return store.Account{}, err
		}
		if strings.TrimSpace(creds.BaseURL) == "" {
			return store.Account{}, errors.New("base_url 不能为空")
		}
		if strings.TrimSpace(creds.APIKey) == "" {
			return store.Account{}, errors.New("api_key 不能为空")
		}
		if creds.Proxy == "" {
			creds.Proxy = strings.TrimSpace(req.Proxy)
		}
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = strings.TrimSpace(req.Email)
		}
		if label == "" {
			label = "自定义供应商"
		}
		return m.addCustomAccount(label, creds)
	case "teleagent":
		var creds teleagent.Credentials
		if err := decodeEngineCredentials(req, &creds); err != nil {
			return store.Account{}, err
		}
		if creds.Token == "" {
			return store.Account{}, errors.New("token 不能为空")
		}
		// 浏览器登录导入没有 device/install 指纹：自动生成（与桌面端等价，服务端只校验一致性）。
		if creds.DeviceID == "" {
			creds.DeviceID = teleagent.NewDeviceID()
		}
		if creds.InstallID == "" {
			creds.InstallID = teleagent.NewInstallID()
		}
		if creds.AppVersion == "" {
			creds.AppVersion = teleagent.DefaultAppVersion
		}
		if creds.APIKey == "" {
			creds.APIKey = teleagent.DefaultAPIKey
		}
		if creds.Proxy == "" {
			creds.Proxy = strings.TrimSpace(req.Proxy)
		}
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = teleagent.AccountName(creds.Token) // JWT 里的脱敏手机号
		}
		if label == "" {
			label = strings.TrimSpace(req.Email)
		}
		if label == "" {
			// Web 端 Cookie 的 token 可能没有 accountName 声明，兜底用时间戳命名。
			label = "TeleAgent " + time.Now().Format("2006-01-02 15:04")
		}
		return m.addTeleAgentAccount(label, creds)
	default:
		return store.Account{}, fmt.Errorf("未知引擎: %s（支持 a、b、codex、antigravity、kiro、workbuddy、teleagent、custom）", req.Engine)
	}
}

// decodeEngineCredentials 解析订阅引擎凭据对象（credentials JSON）。
func decodeEngineCredentials(req AccountInput, out any) error {
	if len(req.Credentials) == 0 {
		return errors.New("credentials 不能为空")
	}
	if err := json.Unmarshal(req.Credentials, out); err != nil {
		return fmt.Errorf("credentials 不是合法 JSON: %w", err)
	}
	return nil
}

// addCodexAccount 添加 codex 账号（存储 + 入池）。
func (m *Manager) addCodexAccount(label string, creds codex.Credentials) (store.Account, error) {
	eng := m.codexEngine()
	if eng == nil {
		return store.Account{}, errors.New("codex 引擎未启用")
	}
	if label == "" {
		return store.Account{}, errors.New("label 或 email 至少填写一个")
	}
	acc, err := m.st.CreateAccount("codex", label, engine.MarshalCompact(creds), true)
	if err != nil {
		return store.Account{}, err
	}
	if err := eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds); err != nil {
		_ = m.st.UpdateAccountStatus(acc.ID, "error", err.Error())
		return acc, nil
	}
	return acc, nil
}

// addAntigravityAccount 添加 antigravity 账号（存储 + 入池）。
func (m *Manager) addAntigravityAccount(label string, creds antigravity.Credentials) (store.Account, error) {
	eng := m.antigravityEngine()
	if eng == nil {
		return store.Account{}, errors.New("antigravity 引擎未启用")
	}
	if label == "" {
		return store.Account{}, errors.New("label 或 email 至少填写一个")
	}
	acc, err := m.st.CreateAccount("antigravity", label, engine.MarshalCompact(creds), true)
	if err != nil {
		return store.Account{}, err
	}
	if err := eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds); err != nil {
		_ = m.st.UpdateAccountStatus(acc.ID, "error", err.Error())
		return acc, nil
	}
	return acc, nil
}

// addKiroAccount 添加 kiro 账号（存储 + 入池）。
func (m *Manager) addKiroAccount(label string, creds kiro.Credentials) (store.Account, error) {
	eng := m.kiroEngine()
	if eng == nil {
		return store.Account{}, errors.New("kiro 引擎未启用")
	}
	if label == "" {
		return store.Account{}, errors.New("label 或 email 至少填写一个")
	}
	acc, err := m.st.CreateAccount("kiro", label, engine.MarshalCompact(creds), true)
	if err != nil {
		return store.Account{}, err
	}
	if err := eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds); err != nil {
		_ = m.st.UpdateAccountStatus(acc.ID, "error", err.Error())
		return acc, nil
	}
	return acc, nil
}

// addWorkBuddyAccount 添加 workbuddy 账号（存储 + 入池）。
func (m *Manager) addWorkBuddyAccount(label string, creds workbuddy.Credentials) (store.Account, error) {
	eng := m.mWorkBuddyEngine()
	if eng == nil {
		return store.Account{}, errors.New("workbuddy 引擎未启用")
	}
	if strings.TrimSpace(label) == "" {
		// 手填 opaque token 可能没有邮箱；自动名只含发行版与时间，绝不泄露 token。
		label = fmt.Sprintf("%s-%d", workbuddy.SiteFor(creds.Site).Name, time.Now().UnixNano())
	}
	acc, err := m.st.CreateAccountWithUniqueLabel("workbuddy", label, engine.MarshalCompact(creds), true)
	if err != nil {
		return store.Account{}, err
	}
	if err := eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds); err != nil {
		_ = m.st.UpdateAccountStatus(acc.ID, "error", err.Error())
		return acc, nil
	}
	return acc, nil
}

// addCustomAccount 添加第三方 Key 账号（存储 + 入池）。
func (m *Manager) addCustomAccount(label string, creds custom.Credentials) (store.Account, error) {
	eng := m.customEngine()
	if eng == nil {
		return store.Account{}, errors.New("custom 引擎未启用（engines.custom.enabled）")
	}
	if label == "" {
		return store.Account{}, errors.New("label 不能为空")
	}
	acc, err := m.st.CreateAccount("custom", label, engine.MarshalCompact(creds), true)
	if err != nil {
		return store.Account{}, err
	}
	if err := eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds); err != nil {
		_ = m.st.UpdateAccountStatus(acc.ID, "error", err.Error())
		return acc, nil
	}
	return acc, nil
}

// addTeleAgentAccount 添加 teleagent 账号（存储 + 入池）。
func (m *Manager) addTeleAgentAccount(label string, creds teleagent.Credentials) (store.Account, error) {
	eng := m.teleagentEngine()
	if eng == nil {
		return store.Account{}, errors.New("teleagent 引擎未启用（engines.teleagent.enabled）")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = teleagent.AccountName(creds.Token)
	}
	if label == "" {
		return store.Account{}, errors.New("label 不能为空（且 token 里没有 accountName）")
	}
	acc, err := m.st.CreateAccount("teleagent", label, engine.MarshalCompact(creds), true)
	if err != nil {
		return store.Account{}, err
	}
	if err := eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds); err != nil {
		_ = m.st.UpdateAccountStatus(acc.ID, "error", err.Error())
		return acc, nil
	}
	return acc, nil
}

// UpdateSubscriptionCredentials 更新订阅/第三方引擎账号凭据。
func (m *Manager) UpdateSubscriptionCredentials(id int64, credentials json.RawMessage) error {
	acc, err := m.st.GetAccount(id)
	if err != nil {
		return err
	}
	switch acc.Engine {
	case "codex":
		var creds codex.Credentials
		if err := json.Unmarshal(credentials, &creds); err != nil || creds.RefreshToken == "" {
			return errors.New("credentials.refresh_token 不能为空")
		}
		if err := m.st.UpdateAccountCredentials(id, string(credentials)); err != nil {
			return err
		}
		return m.codexEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
	case "antigravity":
		var creds antigravity.Credentials
		if err := json.Unmarshal(credentials, &creds); err != nil || creds.RefreshToken == "" {
			return errors.New("credentials.refresh_token 不能为空")
		}
		if err := m.st.UpdateAccountCredentials(id, string(credentials)); err != nil {
			return err
		}
		return m.antigravityEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
	case "kiro":
		var creds kiro.Credentials
		if err := json.Unmarshal(credentials, &creds); err != nil || creds.RefreshToken == "" {
			return errors.New("credentials.refresh_token 不能为空")
		}
		if err := m.st.UpdateAccountCredentials(id, string(credentials)); err != nil {
			return err
		}
		return m.kiroEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
	case "workbuddy":
		var creds workbuddy.Credentials
		if err := json.Unmarshal(credentials, &creds); err != nil || creds.Token == "" {
			return errors.New("credentials.token 不能为空")
		}
		// 管理台「更新凭据」只提交 token：站点/身份/代理是入池时确定的，
		// 不能因为一次换 token 就把国际服号打回国服。
		var old workbuddy.Credentials
		if json.Unmarshal([]byte(acc.Credentials), &old) == nil {
			creds = mergeWorkBuddyCreds(old, creds)
		}
		if err := m.st.UpdateAccountCredentials(id, engine.MarshalCompact(creds)); err != nil {
			return err
		}
		return m.mWorkBuddyEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
	case "custom":
		var creds custom.Credentials
		if err := json.Unmarshal(credentials, &creds); err != nil {
			return errors.New("credentials 不是合法 JSON")
		}
		if strings.TrimSpace(creds.BaseURL) == "" {
			return errors.New("credentials.base_url 不能为空")
		}
		if strings.TrimSpace(creds.APIKey) == "" {
			return errors.New("credentials.api_key 不能为空")
		}
		if err := m.st.UpdateAccountCredentials(id, engine.MarshalCompact(creds)); err != nil {
			return err
		}
		return m.customEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
	case "teleagent":
		var creds teleagent.Credentials
		if err := json.Unmarshal(credentials, &creds); err != nil || creds.Token == "" {
			return errors.New("credentials.token 不能为空")
		}
		// 「更新凭据」只提交 token：设备/安装指纹、apiKey、版本是入池时确定的，
		// 换 token 不应把它们重置（否则签名会变、且浏览器导入时可能丢失 asar 读到的值）。
		var old teleagent.Credentials
		if json.Unmarshal([]byte(acc.Credentials), &old) == nil {
			if creds.DeviceID == "" {
				creds.DeviceID = old.DeviceID
			}
			if creds.InstallID == "" {
				creds.InstallID = old.InstallID
			}
			if creds.APIKey == "" {
				creds.APIKey = old.APIKey
			}
			if creds.AppVersion == "" {
				creds.AppVersion = old.AppVersion
			}
			if creds.Scope == "" {
				creds.Scope = old.Scope
			}
		}
		if creds.DeviceID == "" {
			creds.DeviceID = teleagent.NewDeviceID()
		}
		if creds.InstallID == "" {
			creds.InstallID = teleagent.NewInstallID()
		}
		if creds.AppVersion == "" {
			creds.AppVersion = teleagent.DefaultAppVersion
		}
		if err := m.st.UpdateAccountCredentials(id, engine.MarshalCompact(creds)); err != nil {
			return err
		}
		return m.teleagentEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
	}
	return fmt.Errorf("引擎 %s 不支持凭据更新", acc.Engine)
}

// geminiCredentialsFromStorageState 从引擎B协议的标准 Cookie 数组中提取
// Gemini 网页协议需要的两个会话 Cookie。
func geminiCredentialsFromStorageState(raw string) (geminiweb.Credentials, error) {
	var state struct {
		Cookies []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"cookies"`
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return geminiweb.Credentials{}, fmt.Errorf("storage-state 不是合法 JSON: %w", err)
	}
	var creds geminiweb.Credentials
	for _, cookie := range state.Cookies {
		switch strings.TrimSpace(cookie.Name) {
		case "__Secure-1PSID":
			creds.PSID = strings.TrimSpace(cookie.Value)
		case "__Secure-1PSIDTS":
			creds.PSIDTS = strings.TrimSpace(cookie.Value)
		}
	}
	if creds.PSID == "" || creds.PSIDTS == "" {
		return geminiweb.Credentials{}, errors.New("storage-state 缺少 __Secure-1PSID 或 __Secure-1PSIDTS Cookie")
	}
	return creds, nil
}

func (m *Manager) addGeminiCredentials(label, psid, psidts string) (store.Account, error) {
	if m.engineA == nil {
		return store.Account{}, errors.New("引擎A 未启用")
	}
	creds, _ := json.Marshal(geminiweb.Credentials{PSID: psid, PSIDTS: psidts})
	acc, err := m.st.CreateAccount("geminiweb", label, string(creds), true)
	if err != nil {
		return store.Account{}, err
	}
	if err := m.engineA.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, psid, psidts); err != nil {
		_ = m.st.UpdateAccountStatus(acc.ID, "error", err.Error())
		return acc, nil
	}
	return acc, nil
}

// AddGeminiAccount 添加引擎A 账号（Gemini 网页号）。
func (m *Manager) AddGeminiAccount(label, psid, psidts string) (store.Account, error) {
	state, _ := json.Marshal(map[string]any{"cookies": []map[string]any{
		{"name": "__Secure-1PSID", "value": psid},
		{"name": "__Secure-1PSIDTS", "value": psidts},
	}})
	return m.AddAccountFromStorageState(AccountInput{Engine: "geminiweb", Label: label, StorageState: string(state)})
}

// AddAIStudioAccount 添加引擎B 账号（AI Studio 号）。
func (m *Manager) AddAIStudioAccount(email, storageJSON, locale, timezone, proxy string) (store.Account, error) {
	return m.AddAccountFromStorageState(AccountInput{
		Engine: "aistudio", Email: email, StorageState: storageJSON,
		Locale: locale, Timezone: timezone, Proxy: proxy,
	})
}

func (m *Manager) addAIStudioAccount(email, storageJSON, locale, timezone, proxy string) (store.Account, error) {
	native, ok := m.engineB.(*aistudio.NativeEngine)
	if !ok || native == nil {
		return store.Account{}, errors.New("引擎B 原生模式未启用")
	}
	var state json.RawMessage
	if err := json.Unmarshal([]byte(storageJSON), &state); err != nil {
		return store.Account{}, fmt.Errorf("storage-state 不是合法 JSON: %w", err)
	}
	creds, _ := json.Marshal(aistudio.Credentials{
		StorageState: state, Locale: locale, Timezone: timezone, Proxy: proxy,
	})
	acc, err := m.st.CreateAccount("aistudio", email, string(creds), true)
	if err != nil {
		return store.Account{}, err
	}
	if err := native.AddAccount(fmt.Sprintf("%d", acc.ID), email, storageJSON, locale, timezone, proxy); err != nil {
		_ = m.st.UpdateAccountStatus(acc.ID, "error", err.Error())
		return acc, fmt.Errorf("AI Studio 账号入池失败: %w", err)
	}
	return acc, nil
}

// UpdateGeminiCredentials 更新引擎A 账号凭据。
func (m *Manager) UpdateGeminiCredentials(id int64, psid, psidts string) error {
	if m.engineA == nil {
		return errors.New("引擎A 未启用")
	}
	creds, _ := json.Marshal(geminiweb.Credentials{PSID: psid, PSIDTS: psidts})
	if err := m.st.UpdateAccountCredentials(id, string(creds)); err != nil {
		return err
	}
	return m.engineA.ReplaceCredentials(fmt.Sprintf("%d", id), psid, psidts)
}

// RemoveAccount 删除账号（出池 + 删库 + 删凭据文件）。
func (m *Manager) RemoveAccount(id int64) error {
	acc, err := m.st.GetAccount(id)
	if err != nil {
		return err
	}
	engineID := fmt.Sprintf("%d", id)
	if pool, ok := m.pools[acc.Engine]; ok {
		if err := pool.RemoveAccount(engineID); err != nil {
			// 账号不在池中（引擎未启用/已出池）时跳过，仅删除存储记录
			if !strings.Contains(err.Error(), "不在池中") {
				return err
			}
		}
	}
	if err := m.st.DeleteAccount(id); err != nil {
		return err
	}
	m.ReloadRoutingOrder()
	return nil
}

// SetAccountEnabled 启停账号。
func (m *Manager) SetAccountEnabled(id int64, enabled bool) error {
	acc, err := m.st.GetAccount(id)
	if err != nil {
		return err
	}
	engineID := fmt.Sprintf("%d", id)
	if pool, ok := m.pools[acc.Engine]; ok {
		if err := pool.SetEnabled(engineID, enabled); err != nil {
			// 账号不在池中（引擎未启用）时仅更新存储记录
			if !strings.Contains(err.Error(), "不在池中") {
				return err
			}
		}
	}
	return m.st.SetAccountEnabled(id, enabled)
}

// CheckAccount 触发一次账号健康检查（引擎A：重新初始化；引擎B：刷新目录模型）。
func (m *Manager) CheckAccount(ctx context.Context, id int64) error {
	acc, err := m.st.GetAccount(id)
	if err != nil {
		return err
	}
	engineID := fmt.Sprintf("%d", id)
	switch acc.Engine {
	case "a", "geminiweb":
		if m.engineA == nil {
			return errors.New("引擎A 未启用")
		}
		var creds geminiweb.Credentials
		if err := json.Unmarshal([]byte(acc.Credentials), &creds); err != nil {
			return err
		}
		if err := m.engineA.ReplaceCredentials(engineID, creds.PSID, creds.PSIDTS); err != nil {
			return err
		}
		_ = m.st.UpdateAccountStatus(id, "initializing", "重新初始化中")
		return nil
	case "b", "aistudio":
		native, ok := m.engineB.(*aistudio.NativeEngine)
		if !ok || native == nil {
			return errors.New("引擎B 原生模式未启用")
		}
		if _, err := native.RefreshAccountModels(ctx, acc.Label); err != nil {
			return err
		}
		_ = m.st.UpdateAccountStatus(id, "ok", "")
		return nil
	case "codex", "antigravity", "kiro", "workbuddy", "custom":
		// 订阅/第三方引擎：重建账号（重新初始化 token / 探测模型）
		if err := m.UpdateSubscriptionCredentials(id, json.RawMessage(acc.Credentials)); err != nil {
			return err
		}
		_ = m.st.UpdateAccountStatus(id, "initializing", "重新初始化中")
		return nil
	}
	return fmt.Errorf("未知引擎: %s", acc.Engine)
}

// AccountCheckIn 立即触发一次账号签到（teleagent / workbuddy 国服），返回结果文案。
func (m *Manager) AccountCheckIn(id int64) (string, error) {
	acc, err := m.st.GetAccount(id)
	if err != nil {
		return "", err
	}
	switch acc.Engine {
	case "teleagent":
		eng := m.teleagentEngine()
		if eng == nil {
			return "", errors.New("teleagent 引擎未启用")
		}
		return eng.CheckIn(fmt.Sprintf("%d", id))
	case "workbuddy":
		eng := m.mWorkBuddyEngine()
		if eng == nil {
			return "", errors.New("workbuddy 引擎未启用")
		}
		return eng.CheckIn(fmt.Sprintf("%d", id))
	default:
		return "", fmt.Errorf("账号 %s 引擎不支持签到", acc.Engine)
	}
}

// SetAccountAutoCheckIn 开关账号自动签到（teleagent / workbuddy 国服）。
func (m *Manager) SetAccountAutoCheckIn(id int64, enabled bool) error {
	acc, err := m.st.GetAccount(id)
	if err != nil {
		return err
	}
	switch acc.Engine {
	case "teleagent":
		eng := m.teleagentEngine()
		if eng == nil {
			return errors.New("teleagent 引擎未启用")
		}
		return eng.SetAutoCheckIn(fmt.Sprintf("%d", id), enabled)
	case "workbuddy":
		eng := m.mWorkBuddyEngine()
		if eng == nil {
			return errors.New("workbuddy 引擎未启用")
		}
		return eng.SetAutoCheckIn(fmt.Sprintf("%d", id), enabled)
	default:
		return fmt.Errorf("账号 %s 引擎不支持签到", acc.Engine)
	}
}

// AccountOverview 号池总览（存储记录 + 引擎实时状态合并）。
func (m *Manager) AccountOverview() []AccountOverview {
	accounts, err := m.st.ListAccounts()
	if err != nil {
		return nil
	}
	// 收集每个引擎的实时账号状态
	live := map[string]map[string]engine.AccountState{}
	for name, pool := range m.pools {
		states := map[string]engine.AccountState{}
		for _, st := range pool.AccountStates() {
			states[st.ID] = st
		}
		live[name] = states
	}
	out := make([]AccountOverview, 0, len(accounts))
	for _, acc := range accounts {
		ov := AccountOverview{Account: acc}
		// 供应商身份来自持久化凭据，不依赖账号是否已加载到引擎。
		if acc.Engine == "custom" {
			ov.Provider = customProvider(acc.Credentials)
		}
		engineID := fmt.Sprintf("%d", acc.ID)
		if states, ok := live[acc.Engine]; ok {
			if st, ok := states[engineID]; ok {
				ov.Live = st
			} else {
				ov.Live = engine.AccountState{ID: engineID, Label: acc.Label, Enabled: acc.Enabled, Status: "unloaded"}
			}
		} else {
			// 引擎未启用（无池）
			ov.Live = engine.AccountState{ID: engineID, Label: acc.Label, Enabled: acc.Enabled, Status: "unloaded", Detail: "引擎未启用"}
		}
		if acc.Engine == "workbuddy" && ov.Live.Status == "unloaded" {
			var creds workbuddy.Credentials
			if json.Unmarshal([]byte(acc.Credentials), &creds) == nil {
				site := workbuddy.SiteFor(creds.Site)
				if strings.TrimSpace(creds.BaseURL) != "" {
					site = workbuddy.SiteForBaseURL(creds.BaseURL)
				}
				ov.Live.Service = site.ID
			}
		}
		out = append(out, ov)
	}
	return out
}

// AccountRef 返回接单账号的展示元数据：所属引擎、显示名称与供应商。
// 供应商只对第三方 Key 有值（归一化 base_url）；内置引擎返回空。
// 账号已被删除或 ID 非法时 ok=false，调用方回退到引擎名。
func (m *Manager) AccountRef(id string) (engineName, label, provider string, ok bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if err != nil || n <= 0 || m.st == nil {
		return "", "", "", false
	}
	acc, err := m.st.GetAccount(n)
	if err != nil {
		return "", "", "", false
	}
	if acc.Engine == "custom" {
		provider = customProvider(acc.Credentials)
	}
	return acc.Engine, acc.Label, provider, true
}

// customProvider 从第三方 Key 的凭据里取出供应商身份：归一化后的 base_url。
// 凭据缺失或没有地址时返回空，管理台会把这类账号归到「未识别地址」。
func customProvider(credentials string) string {
	var creds custom.Credentials
	if err := json.Unmarshal([]byte(credentials), &creds); err != nil {
		return ""
	}
	return upstream.NormalizeBaseURL(creds.BaseURL)
}

// ==================== 路由与对话 ====================

// pickEngine 按模型名路由引擎。第二个返回值表示模型是否为该引擎专属
// （配置显式指定、任一引擎目录命中、多模态关键词）：专属模型引擎不可用时
// 不做跨引擎切换——其他引擎不认识该模型，切换只会产生误导性错误。
func (m *Manager) pickEngine(modelName string) (engine.Engine, bool) {
	name := model.CanonicalModelID(modelName)
	// 下架是「这个模型不对下游开放」，不是「换一个引擎再试」。
	// 先于目录匹配判断，否则同名模型会从已下架的第三方 Key 目录里被认领出去。
	if m.modelDisabled(name) {
		return nil, false
	}
	return m.matchEngine(name)
}

// 探测标记定义在 engine 包：引擎内部（如 custom 的图片/向量探测选号）
// 也要能识别它，否则已下架模型永远只能在入口层绕过开关、到了引擎再被拦一次。
func withProbeRequest(ctx context.Context) context.Context {
	return engine.WithProbeContext(ctx)
}

func isProbeRequest(ctx context.Context) bool {
	return engine.IsProbeContext(ctx)
}

// pickEngineForProbe 管理台探测专用：只看哪个引擎能接单，不看上下架开关。
func (m *Manager) pickEngineForProbe(modelName string) (engine.Engine, bool) {
	name := model.CanonicalModelID(modelName)
	eng, owned := m.matchEngine(name)
	if eng == nil {
		return nil, false
	}
	// 探测要回答「这个模型现在能不能接」，不是「丢给 auto 兜底试试」。owned=false
	// 意味着 matchEngine 只走到 default_engine / auto 兜底（没被任何引擎目录认领）。
	// 此时若真没有任何引擎声明该模型（如上游把 deepseek-v4.1-flash-sg 从目录摘掉，
	// 只剩 model_settings 里一条孤儿记录），继续兜底只会落到 aistudio，报出
	// 「没有符合条件的 AI Studio 账户」这种和真实归属无关的错误。这里拦下，
	// 让上游返回 model_not_found 的准确提示。
	if !owned && !m.anyEngineDeclares(name) {
		return nil, false
	}
	return eng, owned
}

// anyEngineDeclares 是否有任何注册引擎的实时目录声明了该模型（大小写不敏感）。
func (m *Manager) anyEngineDeclares(name string) bool {
	name = model.CanonicalModelID(name)
	if name == "" {
		return false
	}
	for _, engName := range m.engineOrder {
		if eng, ok := m.engines[engName]; ok && engineHasModel(eng, name) {
			return true
		}
	}
	return false
}

// matchEngine 按模型名挑选承接引擎（不含上下架判断）。
func (m *Manager) matchEngine(name string) (engine.Engine, bool) {
	// 1) 显式模型路由：engine_a/b_models + routing.engine_models.<engine>
	//    配置命中即为「专属」：写了就是点名要这个引擎，它挂了就报它的真实错误，
	//    不再被 alternate 带去别的引擎（换过去只会拿到假 404 或错模型）。
	if len(m.geminiWebModels) > 0 && matchAny(name, m.geminiWebModels) {
		if eng, ok := m.engines["geminiweb"]; ok {
			return eng, true
		}
	}
	if len(m.aiStudioModels) > 0 && matchAny(name, m.aiStudioModels) {
		if eng, ok := m.engines["aistudio"]; ok {
			return eng, true
		}
	}
	if eng, ok := m.matchConfiguredModels(name); ok {
		return eng, true
	}

	// 2) 订阅/custom 目录精确命中（先于多模态启发式，避免 gpt-image-* 等被误送引擎B）
	if eng, ok := m.matchSubscriptionCatalog(name); ok {
		return eng, true
	}

	// 3) 引擎A/B 实时目录精确命中：裸名模型（如引擎A 的 gemini-argon、引擎B
	//    目录中的正式 ID）归属到拥有它的引擎，不再落入 auto 兜底后被静默换模。
	//    订阅引擎优先于 A/B，既有路由（models/* 镜像目录等）保持不变。
	if eng, ok := m.matchCoreCatalog(name); ok {
		return eng, true
	}

	// 3.5) 快照归属：实时目录随上游波动（WorkBuddy 的 /v3/config 一刷新，
	//      deepseek-v4.1-flash-sg 这类模型就会暂时消失），模型会从第 2 步
	//      落空、被 3.6 的「声明-不可用」启发式按引擎注册顺序抢给先声明的
	//      引擎（如零账号的 codex 抢走 WorkBuddy 的 gpt-5.4），或掉进 auto
	//      兜底被送去 AI Studio。管理台「获取模型」保存的快照是用户显式的
	//      归属记录：快照说它属于哪个订阅/custom 引擎，就留在那里报真实
	//      错误，绝不改投别的引擎兜底。
	if eng, ok := m.matchSnapshotOwner(name); ok {
		return eng, true
	}

	// 3.6) 目录里有、但当前没有可用账号的模型（custom / 订阅引擎）：仍判给它（专属）。
	//      否则会落入 auto 兜底被送去根本不认识该模型的引擎，真实原因
	//      「声明了模型的账号均不可用」反而看不到。快照没记录时这是兜底启发式；
	//      多个引擎都声明时按注册顺序取第一个。
	if eng, ok := m.matchDeclaredUnavailable(name); ok {
		return eng, true
	}

	// 4) 多模态模型（音乐/视频/生图/语音/向量）只能由声明它的引擎接单。
	//    目录命中已在第 2、3 步处理；这里只处理「当前目录里还没有」的多模态请求：
	//    交给引擎B 并标为专属，引擎B 不可用时直接报引擎B 的真实错误，
	//    绝不再走 default/auto 兜底改投第三方 Key（lyria 打到 MiniMax 的根因）。
	if isMultimodalModel(name) {
		if eng, ok := m.engines["aistudio"]; ok {
			return eng, true
		}
		// 引擎B 未启用：这类模型没有任何承接方，交给上层返回 model_not_found。
		return nil, false
	}

	// 5) default_engine 显式指定（支持任意注册引擎）
	//    这里刻意保持「非专属」语义：default_engine 是「其余模型默认去哪」，
	//    不是逐个模型的点名，所以引擎A/B 仍保留 auto 降级；订阅/custom 本身
	//    只认目录，标为专属（换过去也接不了）。
	if m.defaultEngine != "auto" && m.defaultEngine != "" {
		if eng, ok := m.engines[m.defaultEngine]; ok {
			return eng, isSubscriptionEngine(m.defaultEngine)
		}
	}

	// 6) auto 回退：引擎B协议是主协议，其次引擎A，最后任意 Ready 引擎
	if eng, ok := m.engines["aistudio"]; ok {
		return eng, false
	}
	if eng, ok := m.engines["geminiweb"]; ok {
		return eng, false
	}
	for _, engName := range m.engineOrder {
		if eng, ok := m.engines[engName]; ok && eng.Ready() {
			return eng, false
		}
	}
	return nil, false
}

// matchConfiguredModels 按 routing.engine_models.<engine> 匹配。
// 按引擎注册顺序遍历而不是直接 range map：map 遍历顺序随机，同一个模型
// 可能在多次重启间落到不同引擎，排查线上问题时非常难受。
func (m *Manager) matchConfiguredModels(name string) (engine.Engine, bool) {
	for _, engName := range m.engineOrder {
		patterns := m.engineModels[engName]
		if len(patterns) == 0 || !matchAny(name, patterns) {
			continue
		}
		if eng, ok := m.engines[engName]; ok {
			return eng, true
		}
	}
	return nil, false
}

// matchSubscriptionCatalog 按模型 id 精确命中订阅/custom 引擎目录。
func (m *Manager) matchSubscriptionCatalog(modelName string) (engine.Engine, bool) {
	for _, engName := range m.engineOrder {
		if engName == "geminiweb" || engName == "aistudio" {
			continue
		}
		eng, ok := m.engines[engName]
		if !ok {
			continue
		}
		// 号池为空/全部不可用的订阅引擎不认领模型：它们的目录是写死的静态列表
		// （antigravity/workbuddy 都含 gemini-2.5-flash），不判 Ready 会把本该由
		// custom/A/B 承接的同名模型抢走，然后报「没有可用账号」。
		if !eng.Ready() {
			continue
		}
		// 要按「有可用账号声明该模型」认领：custom 目录会保留停用/冷却账号
		// 的模型（Available=false），只判引擎级 Ready 会把这类模型抢走后报
		// 「声明了模型的账号均不可用」，而同样声明它的 aistudio 不会被尝试。
		if engineCanServeModel(eng, modelName) {
			return eng, true
		}
	}
	return nil, false
}

// matchCoreCatalog 按模型 id 精确命中引擎A/B 的实时目录（按注册顺序，a 先于 b）。
// 命中即视为模型专属：其他引擎不认识该模型，引擎不可用时直接返回该引擎的
// 具体错误，而不是切换引擎后产生误导性错误。
func (m *Manager) matchCoreCatalog(modelName string) (engine.Engine, bool) {
	for _, engName := range m.engineOrder {
		if engName != "geminiweb" && engName != "aistudio" {
			continue
		}
		eng, ok := m.engines[engName]
		if !ok {
			continue
		}
		if engineHasModel(eng, modelName) {
			return eng, true
		}
	}
	return nil, false
}

// engineHasModel 引擎目录是否包含该模型（大小写不敏感）。
// Google OpenAI 兼容端点的目录 id 带 models/ 前缀，两侧均做去前缀的宽松匹配，
// 客户端用裸名或带前缀名都能命中。
func engineHasModel(eng engine.Engine, modelName string) bool {
	if eng == nil {
		return false
	}
	name := model.CanonicalModelID(modelName)
	if name == "" {
		return false
	}
	for _, mi := range eng.ListModels() {
		if id := model.CanonicalModelID(mi.ID); id != "" && id == name {
			return true
		}
	}
	return false
}

// matchDeclaredUnavailable 找出「目录里声明了该模型、但当前没有可用账号」的引擎，
// 用于给它保留归属、返回准确的错误。订阅引擎（workbuddy 等）的目录是静态的，
// 没有模型级可用性概念，Ready() 为 false 就是全部账号不可用；custom 还要求
// 声明该模型的账号本身可用（HasAvailableModel）。
//
// 不做这一步的后果：WorkBuddy 唯一的账号掉了 401，它的模型会落入 auto 兜底被
// 送去 AI Studio，模型页于是显示「没有符合条件的 AI Studio 账户」，与真实原因
// （WorkBuddy 没有可用账号）毫不相干。
func (m *Manager) matchDeclaredUnavailable(modelName string) (engine.Engine, bool) {
	for _, engName := range m.engineOrder {
		// 引擎A/B 的目录大而杂，交给后面的 auto 兜底，不在这里抢占。
		if engName == "geminiweb" || engName == "aistudio" {
			continue
		}
		eng, ok := m.engines[engName]
		if !ok {
			continue
		}
		if engineHasModel(eng, modelName) && !engineCanActuallyServe(eng, modelName) {
			return eng, true
		}
	}
	return nil, false
}

// engineCanActuallyServe 引擎当前是否真有账号能接这个模型：
// 引擎级 Ready（至少一个账号可用）+ 模型级可用性（实现了该接口的引擎）。
func engineCanActuallyServe(eng engine.Engine, modelName string) bool {
	if eng == nil || !eng.Ready() {
		return false
	}
	return engineCanServeModel(eng, modelName)
}

// snapshotTTL 目录快照归属缓存的存活期。快照只在管理台点「获取模型」时整表
// 替换，TTL 只影响上游摘掉/加回模型后路由感知快照的延迟。
const snapshotTTL = 30 * time.Second

// matchSnapshotOwner 按「获取模型」保存的目录快照判断模型归属。只认订阅/custom
// 引擎（模型专属语义）：引擎A/B 的快照行交给实时目录与 auto 兜底，否则快照
// 过期会把 A/B 还在实时承接的同名模型钉死在过期归属上、掐断跨引擎兜底。
// 归属引擎已不再注册（配置里关掉了）时同样不认领。
func (m *Manager) matchSnapshotOwner(modelName string) (engine.Engine, bool) {
	if m.st == nil {
		return nil, false
	}
	name := model.CanonicalModelID(modelName)
	if name == "" {
		return nil, false
	}
	engName, ok := m.snapshotOwner(name)
	if !ok {
		return nil, false
	}
	// 第三方 Key 的快照行按供应商分桶（custom|<base_url>），归属都落在 custom 引擎。
	if strings.HasPrefix(engName, "custom|") {
		engName = "custom"
	}
	if !isSubscriptionEngine(engName) {
		return nil, false
	}
	eng, ok := m.engines[engName]
	if !ok {
		return nil, false
	}
	return eng, true
}

// snapshotOwner 读取快照归属（带 TTL 缓存；读失败时视为无快照，不拦截路由）。
func (m *Manager) snapshotOwner(name string) (string, bool) {
	m.snapMu.RLock()
	if m.snapOwner != nil && time.Since(m.snapLoadedAt) < snapshotTTL {
		eng, ok := m.snapOwner[name]
		m.snapMu.RUnlock()
		return eng, ok
	}
	m.snapMu.RUnlock()

	m.snapMu.Lock()
	defer m.snapMu.Unlock()
	if m.snapOwner != nil && time.Since(m.snapLoadedAt) < snapshotTTL {
		eng, ok := m.snapOwner[name]
		return eng, ok
	}
	index := map[string]string{}
	if rows, err := m.st.CatalogModels(); err == nil {
		for _, row := range rows {
			id := model.CanonicalModelID(row.Model)
			if id == "" || row.Engine == "" {
				continue
			}
			// 同名模型跨引擎重复时保留先到的一条；CatalogModels 按引擎名排序，
			// 顺序稳定，重启间不会漂移。
			if _, exists := index[id]; !exists {
				index[id] = row.Engine
			}
		}
	}
	m.snapOwner = index
	m.snapLoadedAt = time.Now()
	eng, ok := index[name]
	return eng, ok
}

// engineCanServeModel 引擎当前是否有可用账号能接这个模型。
// 实现了 ModelAvailability 的引擎（custom）按模型级可用性判断；其余引擎的
// 目录本身就是账号可用性的体现，按目录是否声明该模型判断即可。
func engineCanServeModel(eng engine.Engine, modelName string) bool {
	if eng == nil {
		return false
	}
	if checker, ok := eng.(engine.ModelAvailability); ok {
		return checker.HasAvailableModel(modelName)
	}
	return engineHasModel(eng, modelName)
}

// modelDisabled 报告模型是否被管理台下架。读失败时不拦截：下架名单拿不到
// 不该把全部请求变成 404，目录与入口还有各自的校验兜底。
func (m *Manager) modelDisabled(modelName string) bool {
	if m.st == nil {
		return false
	}
	name := model.CanonicalModelID(modelName)
	if name == "" {
		return false
	}
	enabled, err := m.st.ModelEnabled(name)
	return err == nil && !enabled
}

// isSubscriptionEngine 是否为「模型专属」引擎（订阅号池或第三方 Key）。
// 命中后引擎不可用时不做跨引擎切换，避免误导性错误。
func isSubscriptionEngine(name string) bool {
	switch name {
	case "codex", "antigravity", "kiro", "workbuddy", "teleagent", "custom":
		return true
	}
	return false
}

// matchAny 前缀/子串匹配。
func matchAny(name string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(name, strings.TrimSuffix(p, "*")) {
				return true
			}
		} else if name == p || strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// isMultimodalModel 判断模型是否应走引擎B（多模态生成）。
func isMultimodalModel(name string) bool {
	for _, kw := range []string{"image", "video", "veo", "lyria", "audio", "tts", "nano", "omni", "embedding", "music"} {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}

// ProbeModel 管理台模型检查：按用途走对应上游探测接口。
// 生图 → /v1/images/generations；向量 → /v1/embeddings；其余对话模型 → chat；
// 视频/音乐/语音只做目录可达性检查（避免昂贵的真实生成）。
func (m *Manager) ProbeModel(ctx context.Context, modelID string) (string, error) {
	return m.probeModel(ctx, modelID, nil)
}

// ProbeModelForTarget 显式目标只在指定引擎/供应商内探测，绝不回退到全局路由。
// 未指定目标时保留 ProbeModel 的历史行为。
func (m *Manager) ProbeModelForTarget(ctx context.Context, modelID, engineName, provider string) (string, error) {
	engineName = strings.TrimSpace(engineName)
	provider = strings.TrimSpace(provider)
	if provider != "" && engineName != "custom" {
		return "", errors.New("provider 仅支持 custom 引擎")
	}
	if engineName == "" {
		return m.ProbeModel(ctx, modelID)
	}
	eng := m.engines[engineName]
	if eng == nil {
		return "", fmt.Errorf("引擎 %s 未启用", engineName)
	}
	if provider != "" {
		ctx = custom.WithProviderScope(ctx, provider)
	}
	return m.probeModel(ctx, modelID, eng)
}

func (m *Manager) probeModel(ctx context.Context, modelID string, target engine.Engine) (string, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return "", errors.New("model 不能为空")
	}
	// 管理台探测不受上下架开关限制：已下架的模型也要真的打到上游。
	ctx = withProbeRequest(ctx)
	var models []model.ModelInfo
	if target == nil {
		models = m.ListAllModels()
	} else {
		if e, ok := target.(*custom.Engine); ok {
			var err error
			models, err = e.ProbeModels(ctx)
			if err != nil {
				return "", err
			}
		} else {
			models = target.ListModels()
		}
		if !target.Ready() {
			return "", engine.UnavailableError(target)
		}
	}
	kind := model.ClassifyKind(modelID, "", nil)
	for _, mi := range models {
		if strings.EqualFold(mi.ID, modelID) {
			if mi.Kind != "" {
				kind = mi.Kind
			}
			break
		}
	}
	switch kind {
	case model.KindImage:
		return m.probeImageModel(ctx, modelID, target)
	case model.KindEmbedding:
		return m.probeEmbeddingModel(ctx, modelID, target)
	case model.KindVideo, model.KindMusic, model.KindSpeech:
		return m.probeCatalogModelForTarget(modelID, kind, target, models)
	default:
		return m.probeChatModel(ctx, modelID, target)
	}
}

func (m *Manager) probeChatModel(ctx context.Context, modelID string, target engine.Engine) (string, error) {
	maxTokens := 16
	temp := 0.0
	chat := m.Chat
	if target != nil {
		chat = target.Chat
	}
	res, err := chat(ctx, model.ChatRequest{
		Model:       modelID,
		Messages:    []model.ChatMessage{{Role: "user", Content: "ping"}},
		MaxTokens:   &maxTokens,
		Temperature: &temp,
	})
	if err != nil {
		return "", err
	}
	detail := ""
	if res != nil {
		detail = strings.TrimSpace(res.Text)
	}
	if detail == "" {
		return "响应正常", nil
	}
	if len(detail) > 120 {
		detail = detail[:120] + "…"
	}
	return detail, nil
}

func (m *Manager) probeImageModel(ctx context.Context, modelID string, eng engine.Engine) (string, error) {
	target := eng
	if eng == nil {
		eng, _ = m.pickEngineForProbe(modelID)
	}
	switch e := eng.(type) {
	case *custom.Engine:
		return e.ProbeImage(ctx, modelID)
	case *aistudio.Engine:
		return e.ProbeImage(ctx, modelID)
	}
	// 原生 AI Studio 等：图片走 chat/completions。
	return m.probeChatModel(ctx, modelID, target)
}

func (m *Manager) probeEmbeddingModel(ctx context.Context, modelID string, eng engine.Engine) (string, error) {
	if eng == nil {
		eng, _ = m.pickEngineForProbe(modelID)
	}
	switch e := eng.(type) {
	case *custom.Engine:
		return e.ProbeEmbedding(ctx, modelID)
	case *aistudio.Engine:
		return e.ProbeEmbedding(ctx, modelID)
	}
	return "", fmt.Errorf("当前引擎不支持向量探测，请手动验证")
}

// probeCatalogModel 视频/音乐/语音类模型的可接单检查。
// 不做真实生成（昂贵且会产生计费），但也不做「在目录里就算过」的假探测：
// 至少要满足「有账号声明该模型」+「归属引擎能接单」两条。
func (m *Manager) probeCatalogModel(modelID, kind string) (string, error) {
	return m.probeCatalogModelForTarget(modelID, kind, nil, nil)
}

func (m *Manager) probeCatalogModelForTarget(modelID, kind string, eng engine.Engine, models []model.ModelInfo) (string, error) {
	if eng == nil {
		eng, _ = m.pickEngineForProbe(modelID)
		if eng == nil || !engineHasModel(eng, modelID) {
			return "", engine.NewModelNotFound(modelID, m.ownershipHint(modelID))
		}
		models = eng.ListModels()
	}
	// 目录里有、但没有任何可用账号声明它（引擎自报 available=false）同样不算可用。
	// 必须查被选中引擎自己的目录：全局 ListAllModels 会把别的引擎/别的写法合并进来，
	// 可能拿 aistudio 的条目判断 custom 是否可用（两个方向都会误判）。
	want := model.CanonicalModelID(modelID)
	found, available := false, false
	for _, mi := range models {
		if model.CanonicalModelID(mi.ID) != want {
			continue
		}
		found = true
		available = available || mi.Available
	}
	if !found {
		return "", engine.NewModelNotFound(modelID, "目标引擎/供应商未声明该模型")
	}
	if !available {
		return "", fmt.Errorf("模型 %s 在 %s 引擎目录中，但没有可用账号声明它（账号停用 / 未拉到目录 / 冷却中）", modelID, eng.Name())
	}
	if !eng.Ready() {
		return "", engine.UnavailableError(eng)
	}
	return fmt.Sprintf("引擎 %s 已声明该模型且引擎可接单（%s 模型未做生成探测）", eng.Name(), engine.KindLabel(kind)), nil
}

// Chat 非流式对话。按供应商 / 账号顺序尝试，一个供应商不通再换下一个。
func (m *Manager) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	ctx, affinityKey, preferred := m.affinityContext(ctx, req)
	engines, req, err := m.chatEnginePlan(ctx, req)
	if err != nil {
		return nil, err
	}
	engines = m.preferAffinityEngine(engines, preferred)
	var (
		lastRes *model.ChatResult
		lastErr error
	)
	for i, eng := range engines {
		if err := m.requireToolSupport(eng, req); err != nil {
			lastErr = err
			// 有工具支持的下家就继续试，全部不支持时把明确的错误返回给调用方。
			if i < len(engines)-1 {
				continue
			}
			break
		}
		res, err := eng.Chat(ctx, req)
		if err == nil {
			m.noteAffinity(affinityKey, ctx)
			return res, nil
		}
		// 失败也带回引擎名，用量明细才不会显示「—」。
		lastRes = &model.ChatResult{Engine: eng.Name(), Model: req.Model}
		lastErr = err
		if i == len(engines)-1 || !shouldFailover(ctx, err) {
			break
		}
	}
	return lastRes, lastErr
}

// ChatStream 流式对话。sink 同时承接文本与工具调用增量：引擎实现
// engine.ToolEngine 时工具调用会被透传，否则工具定义被忽略、只走文本。
func (m *Manager) ChatStream(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	if sink == nil {
		sink = model.TextSink(nil)
	}
	ctx, affinityKey, preferred := m.affinityContext(ctx, req)
	engines, req, err := m.chatEnginePlan(ctx, req)
	if err != nil {
		return nil, err
	}
	engines = m.preferAffinityEngine(engines, preferred)
	var (
		res     *model.ChatResult
		lastErr error
		lastRes *model.ChatResult
	)
	for i, eng := range engines {
		if err := m.requireToolSupport(eng, req); err != nil {
			lastErr = err
			if i < len(engines)-1 {
				continue
			}
			break
		}
		// 记录是否已经向客户端写过内容：写过就不能再换供应商，
		// 否则调用方会收到两段拼接的回复。
		gate := &streamGate{inner: sink}
		if toolEng, ok := eng.(engine.ToolEngine); ok {
			res, err = toolEng.ChatStreamSink(ctx, req, gate)
		} else {
			res, err = eng.ChatStream(ctx, req, gate.Text)
		}
		if err == nil {
			m.noteAffinity(affinityKey, ctx)
			return res, nil
		}
		lastRes = &model.ChatResult{Engine: eng.Name(), Model: req.Model}
		lastErr = err
		if gate.Wrote() || i == len(engines)-1 || !shouldFailover(ctx, err) {
			break
		}
	}
	return lastRes, lastErr
}

// resolveChatEngine 选定引擎。目录命中的模型归属其引擎：引擎不可用时直接返回
// 该引擎的具体错误；引擎B 收到目录外模型时不再静默换模，由引擎B 返回明确的
// 目录错误，避免「换用不可用模型后报无可用账户」这类误导性错误。
func (m *Manager) resolveChatEngine(req model.ChatRequest) (engine.Engine, model.ChatRequest, error) {
	return m.resolveChatEngineCtx(context.Background(), req)
}

// resolveChatEngineCtx 与 resolveChatEngine 相同，但会识别管理台探测标记：
// 探测请求跳过上下架闸门，并允许从已下架模型原有的承接引擎/备胎里选路，
// 这样「测试」才能反映上游真实状态，也能把误关的模型测回来后重新开放。
func (m *Manager) resolveChatEngineCtx(ctx context.Context, req model.ChatRequest) (engine.Engine, model.ChatRequest, error) {
	// 用途闸门：音乐/视频/向量模型走 /v1/chat/completions 一定拿不到结果。
	// 第三方 OpenAI 兼容端点只会把请求原样发出去，然后被上游以
	// “Unhandled generated data mime type: audio/mpeg” 之类的 400 拒掉。
	// 能出媒体（引擎B 原生）就交给它，否则直接给调用方一个能看懂的 400。
	kind := m.kindOf(req.Model)
	if model.ChatIncompatibleKind(kind) {
		if mediaEng, ok := m.chatMediaEngine(); ok {
			if !mediaEng.Ready() {
				return nil, req, engine.UnavailableError(mediaEng)
			}
			return mediaEng, req, nil
		}
		return nil, req, engine.NewUnsupportedEndpoint(req.Model, kind)
	}

	// 管理台探测要能验证已下架的模型：跳过上下架闸门，否则永远只能得到
	// 「已下架」，误关后无法通过检查恢复。
	probe := isProbeRequest(ctx)
	if !probe && m.modelDisabled(req.Model) {
		return nil, req, engine.NewModelDisabled(req.Model)
	}

	var eng engine.Engine
	var owned bool
	if probe {
		eng, owned = m.pickEngineForProbe(req.Model)
	} else {
		eng, owned = m.pickEngine(req.Model)
	}
	if eng == nil {
		return nil, req, engine.NewModelNotFound(req.Model, m.ownershipHint(req.Model))
	}
	if !eng.Ready() {
		// 多模态（lyria / veo / image / tts 等）与目录已标明归属的模型禁止换引擎：
		// 换到 custom 只会把请求打到不相干的第三方供应商，最终以假 404 收场。
		// 目标引擎不可用时返回它自己的真实错误（如 engine b: worker_ready=0）。
		if owned || isMultimodalModel(strings.ToLower(strings.TrimSpace(req.Model))) {
			return nil, req, engine.UnavailableError(eng)
		}
		var alt engine.Engine
		if probe {
			alt = m.alternateForProbe(eng, req.Model)
		} else {
			alt = m.alternate(eng, req.Model)
		}
		if alt != nil {
			eng = alt
		} else {
			return nil, req, engine.UnavailableError(eng)
		}
	}
	return eng, req, nil
}

// requireToolSupport 请求带 function calling 工具时，目标引擎必须实现
// engine.ToolEngine（有 function calling 通道）。否则以前会静默退化成「忽略
// tools、仅回文本」，调用方完全无从察觉工具被吞。现在明确报错（见
// engine.ToolsNotSupportedError），让调用方改用支持工具的引擎。
func (m *Manager) requireToolSupport(eng engine.Engine, req model.ChatRequest) error {
	if len(req.Tools) == 0 {
		return nil
	}
	if _, ok := eng.(engine.ToolEngine); ok {
		return nil
	}
	return engine.NewToolsNotSupported(req.Model, eng.Name())
}

// chatMediaEngine 返回能在 /v1/chat/completions 里直接产出媒体的引擎
// （目前只有引擎B 原生模式）。没有则返回 ok=false。
func (m *Manager) chatMediaEngine() (engine.Engine, bool) {
	for _, engName := range m.engineOrder {
		eng, ok := m.engines[engName]
		if !ok || eng == nil {
			continue
		}
		if capable, ok := eng.(engine.ChatMediaEngine); ok && capable.ChatSupportsMedia() {
			return eng, true
		}
	}
	return nil, false
}

// kindOf 模型用途：优先取目录里的 kind（上游能力标记比名字更准），否则按名字推断。
func (m *Manager) kindOf(modelID string) string {
	name := strings.TrimPrefix(strings.TrimSpace(modelID), "models/")
	kind := model.ClassifyKind(name, "", nil)
	if name == "" {
		return kind
	}
	for _, mi := range m.ListAllModels() {
		if !strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(mi.ID), "models/"), name) {
			continue
		}
		if mi.Kind != "" {
			return mi.Kind
		}
		break
	}
	return kind
}

// ownershipHint 说明「查了哪些引擎、各自目录多大」，让 model_not_found 能自解释。
func (m *Manager) ownershipHint(modelName string) string {
	name := strings.TrimPrefix(strings.TrimSpace(modelName), "models/")
	if name == "" {
		return "请求未指定模型"
	}
	parts := make([]string, 0, len(m.engineOrder))
	for _, engName := range m.engineOrder {
		eng, ok := m.engines[engName]
		if !ok || eng == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s(%d)", engName, len(eng.ListModels())))
	}
	msg := fmt.Sprintf("当前号池无 %s：没有任何引擎或账号声明该模型", name)
	if len(parts) > 0 {
		msg += "（参与路由的引擎与目录规模：" + strings.Join(parts, "、") + "）"
	}
	return msg
}

// GenerateVideo 通过引擎B的 AI Studio GenerateVideo 协议创建 Veo 长任务。
func (m *Manager) GenerateVideo(ctx context.Context, request aistudio.VideoRequest) (aistudio.VideoOperation, error) {
	native, ok := m.engineB.(*aistudio.NativeEngine)
	if !ok || native == nil {
		return aistudio.VideoOperation{}, errors.New("当前未启用引擎B原生 Veo 协议")
	}
	if !native.Ready() {
		return aistudio.VideoOperation{}, engine.UnavailableError(native)
	}
	return native.GenerateVideo(ctx, request)
}

// GetGenerateVideoOperation 查询 Veo 长任务状态。
func (m *Manager) GetGenerateVideoOperation(ctx context.Context, operationID string) (aistudio.VideoOperation, error) {
	native, ok := m.engineB.(*aistudio.NativeEngine)
	if !ok || native == nil {
		return aistudio.VideoOperation{}, errors.New("当前未启用引擎B原生 Veo 协议")
	}
	if !native.Ready() {
		return aistudio.VideoOperation{}, engine.UnavailableError(native)
	}
	return native.GetGenerateVideoOperation(ctx, operationID)
}

// DownloadVideoFile 下载已完成 Veo 任务的输出文件。
func (m *Manager) DownloadVideoFile(ctx context.Context, fileID string) (aistudio.MediaStream, error) {
	native, ok := m.engineB.(*aistudio.NativeEngine)
	if !ok || native == nil {
		return aistudio.MediaStream{}, errors.New("当前未启用引擎B原生 Veo 协议")
	}
	if !native.Ready() {
		return aistudio.MediaStream{}, engine.UnavailableError(native)
	}
	return native.DownloadVideoFile(ctx, fileID)
}

// alternate 返回另一个引擎（按注册顺序优先就绪的）。
// 声明式号池（第三方 Key）只认自己的模型目录：目标模型不在其目录里就不换过去，
// 否则换过去只会拿到 model_not_found，把真正的故障源（如引擎B 挂了）盖掉。
// alternate 在选中的引擎不可用（号池挂了）时，按「切换不算失败」的语义挑一个
// 备胎引擎：优先选同样声明了该模型（同名模型）的 Ready 引擎，让请求落到承载
// 同款模型的另一个号池；没有任何同模型引擎时，才退而求其次选任意 Ready 引擎。
//
// 与旧实现的关键区别：custom 引擎即使目录里没有该裸名也参与兜底（它按 base_url
// 转发，模型是透传的），不再因「目录未命中」被整体跳过而错失唯一的可用引擎。
// 切换成功即视为正常请求，不会把「换引擎」本身汇报成失败。
func (m *Manager) alternate(cur engine.Engine, modelName string) engine.Engine {
	// 下架的模型没有备胎：换到同名的第三方 Key 上，正是下架被绕过的那条路。
	if m.modelDisabled(modelName) {
		return nil
	}
	return m.alternateForProbe(cur, modelName)
}

// alternateForProbe 管理台探测专用：不做上下架判断，已下架的模型也允许
// 换到声明它的就绪引擎上验证。
func (m *Manager) alternateForProbe(cur engine.Engine, modelName string) engine.Engine {
	var fallback engine.Engine
	for _, name := range m.engineOrder {
		eng, ok := m.engines[name]
		if !ok || eng == cur {
			continue
		}
		if !eng.Ready() {
			continue
		}
		// 优先：同名模型直接命中（含 custom 的透传目录）。
		if engineHasModel(eng, modelName) {
			return eng
		}
		if fallback == nil {
			fallback = eng
		}
	}
	return fallback
}

// ListModels 聚合模型列表：按裸模型 ID 去重（同名跨引擎只保留一条，先出现的引擎优先）。
// 对外身份就是裸 ID，客户端不指定引擎；实际请求由 pickEngine 按路由规则选上游。
// 管理台标记为不可用的模型会被过滤，下游 /v1/models 与用户控制台看不到。
func (m *Manager) ListModels() []model.ModelInfo {
	return m.listModels(true)
}

// ListAllModels 聚合全部模型（含管理台标记为不可用的），供管理台展示与启停。
func (m *Manager) ListAllModels() []model.ModelInfo {
	return m.listModels(false)
}

func (m *Manager) listModels(publicOnly bool) []model.ModelInfo {
	var disabled map[string]struct{}
	if publicOnly && m.st != nil {
		if set, err := m.st.DisabledModels(); err == nil {
			disabled = set
		}
	}
	var out []model.ModelInfo
	indexByID := map[string]int{}
	for _, name := range m.engineOrder {
		eng, ok := m.engines[name]
		if !ok {
			continue
		}
		// 公开目录（/v1/models、Codex 目录）不展示没有可用账号的订阅/custom 引擎
		// 的静态模型，免得客户端选到一个必然失败的模型；管理台仍可看到全部。
		if publicOnly && name != "geminiweb" && name != "aistudio" && !eng.Ready() {
			continue
		}
		for _, mi := range eng.ListModels() {
			// 对外统一裸模型 ID：Google 的 OpenAI 兼容端点会返回 models/xxx，
			// 和其他引擎的 xxx 是同一个模型，不能并列成两条。
			id := publicModelID(mi.ID)
			if id == "" {
				continue
			}
			if _, skip := disabled[model.CanonicalModelID(id)]; skip {
				continue
			}
			mi = withModelKind(mi)
			mi.ID = id
			if mi.Engine == "" {
				mi.Engine = name
			}
			// 按归一化 ID 合并：models/xxx、XXX、xxx 视为同一个模型，
			// 只保留一条（先出现的引擎优先）。
			key := model.CanonicalModelID(id)
			if idx, ok := indexByID[key]; ok {
				// 同名：保留先出现的条目，合并可用态与更具体的用途。
				out[idx].Available = out[idx].Available || mi.Available
				out[idx].Kind = model.PreferKind(out[idx].Kind, mi.Kind)
				out[idx].Free = out[idx].Free || mi.Free
				if out[idx].DisplayName == "" {
					out[idx].DisplayName = mi.DisplayName
				}
				if out[idx].Engine == "" {
					out[idx].Engine = mi.Engine
				}
				continue
			}
			indexByID[key] = len(out)
			out = append(out, mi)
		}
	}
	return out
}

// publicModelID 返回下游模型目录使用的裸模型 ID（去掉 Google 的 models/ 前缀，
// 大小写不敏感）。路由与下架设置本来就按裸名工作，目录也保持同一口径，
// 客户端拿到的 ID 才能直接回填到请求里。
func publicModelID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) >= len("models/") && strings.EqualFold(id[:len("models/")], "models/") {
		id = id[len("models/"):]
	}
	return strings.TrimSpace(id)
}

func withModelKind(mi model.ModelInfo) model.ModelInfo {
	if mi.Kind == "" {
		mi.Kind = model.ClassifyKind(mi.ID, mi.DisplayName, nil)
	}
	return mi
}

// Status 全部引擎状态（engine_a/engine_b 历史键 + engine_<name> 动态键）。
func (m *Manager) Status() map[string]any {
	out := map[string]any{}
	for name, eng := range m.engines {
		key := "engine_" + name
		if name == "geminiweb" || name == "aistudio" {
			key = "engine_" + name
		}
		out[key] = eng.Status()
	}
	return out
}

// EngineMetaList 管理台引擎元数据列表。
func (m *Manager) EngineMetaList() []EngineMeta {
	metas := []EngineMeta{
		{Name: "geminiweb", Describe: "Gemini Web（gemini.google.com 网页号，Cookie 会话）", Enabled: m.engines["geminiweb"] != nil, AccountPool: true, Credential: "psid/psidts Cookie"},
		{Name: "aistudio", Describe: "AI Studio（原生逆向 / OpenAI 兼容上游）", Enabled: m.engines["aistudio"] != nil, AccountPool: true, Credential: "storage-state.json"},
		{Name: "codex", Describe: "GPT（ChatGPT 订阅号）", Enabled: m.engines["codex"] != nil, AccountPool: true, OAuth: "authorize", Credential: "refresh_token（OAuth 授权或 Codex CLI auth.json 导入）"},
		{Name: "antigravity", Describe: "Antigravity（Google Cloud AI Companion）", Enabled: m.engines["antigravity"] != nil, AccountPool: true, OAuth: "authorize", Credential: "refresh_token（Google OAuth 授权）"},
		{Name: "kiro", Describe: "AWS Kiro（CodeWhisperer，Claude 模型）", Enabled: m.engines["kiro"] != nil, AccountPool: true, Credential: "refresh_token（Kiro IDE ~/.aws/sso/cache/kiro-auth-token.json）"},
		{Name: "workbuddy", Describe: "腾讯 WorkBuddy（国服/国际服号池，积分智能路由）", Enabled: m.engines["workbuddy"] != nil, AccountPool: true, OAuth: "start+poll", Credential: "token / refresh_token（国服 copilot.tencent.com 或国际服 www.workbuddy.ai）"},
		{Name: "teleagent", Describe: "TeleAgent（天翼 AI 星辰超级智能体，GLM 系列）", Enabled: m.engines["teleagent"] != nil, AccountPool: true, OAuth: "authorize", Credential: "token（浏览器登录授权，或本机导入 scripts/import-teleagent-accounts.go）"},
		{Name: "custom", Describe: "第三方 OpenAI 兼容 Key（支持常见供应商余额查询）", Enabled: m.engines["custom"] != nil, AccountPool: true, Credential: "base_url + api_key；余额：MiniMax/OpenRouter/硅基流动/DeepSeek/Sub2API/NewAPI 等"},
	}
	return metas
}

// PassthroughEngine 返回支持透传的引擎（当前只有引擎B upstream 模式）。
func (m *Manager) PassthroughEngine() engine.Engine {
	if m.engineB != nil && m.engineB.Passthrough() {
		return m.engineB
	}
	return nil
}

// ServePassthrough 透传到支持引擎。
func (m *Manager) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	eng := m.PassthroughEngine()
	if eng == nil {
		return errors.New("没有启用支持多模态透传的引擎")
	}
	return eng.ServePassthrough(ctx, method, path, body, contentType, w)
}

// ServeModelPassthrough 按模型路由多模态透传：请求体的模型命中订阅/custom 引擎目录时，
// 转发到对应上游（当前仅 custom 第三方 Key 的 OpenAI 兼容端点支持）。
// handled=false 表示没有引擎认领该模型，调用方应回退引擎B（upstream 模式）。
func (m *Manager) ServeModelPassthrough(ctx context.Context, modelID, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) (bool, error) {
	name := strings.TrimSpace(modelID)
	if name == "" {
		return false, nil
	}
	// 下架的模型不走透传：否则 embeddings / images 会按目录命中直接打到第三方 Key。
	if m.modelDisabled(name) {
		return true, engine.NewModelDisabled(name)
	}
	eng, ok := m.matchSubscriptionCatalog(name)
	if !ok {
		return false, nil
	}
	custom, isCustom := eng.(*custom.Engine)
	if !isCustom {
		// 订阅引擎不是通用 OpenAI 兼容上游，维持引擎B回退。
		return false, nil
	}
	return true, custom.ServeModelPassthrough(ctx, name, method, path, body, contentType, w)
}
