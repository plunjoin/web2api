package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	aistudio "web2api/internal/aistudio2api/aistudio"
	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/model"
	"web2api/internal/store"
	"web2api/internal/upstream/workbuddy"
)

// Manager 引擎注册表、模型路由与号池动态管理。
type Manager struct {
	mu sync.RWMutex

	st  *store.Store
	cfg *config.Config

	// 引擎A/B 为历史双引擎，保留专用字段以兼容 Veo 等能力接口。
	engineA *GeminiWebEngine
	engineB initializableEngine

	// 全部引擎（含 a/b）的注册表与顺序。
	engineOrder []string
	engines     map[string]engine.Engine
	// 有账号池的引擎（号池动态管理统一分发）。
	pools map[string]accountPool

	defaultEngine string
	engineAModels []string
	engineBModels []string
	// 新引擎（codex/antigravity/kiro/workbuddy）的模型路由配置。
	engineModels map[string][]string
}

// accountPool 有账号池的引擎统一接口（引擎A/B 与订阅引擎都实现）。
type accountPool interface {
	RemoveAccount(engineID string) error
	SetEnabled(engineID string, enabled bool) error
	AccountStates() []AccountState
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
	OAuth       string `json:"oauth,omitempty"`       // codex/antigravity=authorize+code；workbuddy=start+poll
	Credential  string `json:"credential,omitempty"`   // 凭据字段说明
}

// initializableEngine 需要异步初始化的引擎。
type initializableEngine interface {
	engine.Engine
	Init(ctx context.Context) error
}

// AccountOverview 管理台账号视图：存储记录 + 引擎实时状态。
type AccountOverview struct {
	store.Account
	Live AccountState `json:"live"`
}

// GeminiCredentials 引擎A 凭据结构。
type GeminiCredentials struct {
	PSID   string `json:"psid"`
	PSIDTS string `json:"psidts"`
}

// AIStudioCredentials 引擎B 凭据结构。
type AIStudioCredentials struct {
	StorageState json.RawMessage `json:"storage_state"`
	Locale       string          `json:"locale,omitempty"`
	Timezone     string          `json:"timezone,omitempty"`
	Proxy        string          `json:"proxy,omitempty"`
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
		st:            st,
		cfg:           cfg,
		defaultEngine: strings.ToLower(cfg.Routing.DefaultEngine),
		engineAModels: cfg.Routing.EngineAModels,
		engineBModels: cfg.Routing.EngineBModels,
		engineModels:  cfg.Routing.EngineModels,
		engines:       map[string]engine.Engine{},
		pools:         map[string]accountPool{},
	}
	if m.defaultEngine == "" {
		m.defaultEngine = "auto"
	}

	if cfg.EngineA.Enabled {
		ea, err := NewGeminiWeb(cfg.EngineA)
		if err != nil {
			return nil, fmt.Errorf("引擎A 初始化失败: %w", err)
		}
		m.engineA = ea
		ea.SetStore(st)
		m.register("a", ea, ea)
	}
	if cfg.EngineB.Enabled {
		mode := strings.ToLower(strings.TrimSpace(cfg.EngineB.Mode))
		if mode == "" {
			mode = "native"
		}
		var eb initializableEngine
		if mode == "native" {
			native, err := NewNativeAIStudio(cfg.EngineB)
			if err != nil {
				return nil, err
			}
			eb = native
		} else {
			upstreamEngine, err := NewAIStudio(cfg.EngineB, 300*time.Second)
			if err != nil {
				return nil, err
			}
			eb = upstreamEngine
		}
		if eb != nil {
			m.engineB = eb
			m.register("b", eb, nil)
			// native 模式的账号池能力在具体实例上
			if native, ok := eb.(*NativeAIStudioEngine); ok && native != nil {
				m.pools["b"] = native
			}
		}
	}
	// 订阅引擎：codex / antigravity / kiro / workbuddy
	if cfg.Engines.Codex.Enabled {
		ec := NewCodex(cfg.Engines.Codex, st)
		m.register(ec.Name(), ec, ec)
	}
	if cfg.Engines.Antigravity.Enabled {
		ea := NewAntigravity(cfg.Engines.Antigravity, st)
		m.register(ea.Name(), ea, ea)
	}
	if cfg.Engines.Kiro.Enabled {
		ek := NewKiro(cfg.Engines.Kiro, st)
		m.register(ek.Name(), ek, ek)
	}
	if cfg.Engines.WorkBuddy.Enabled {
		eb := NewWorkBuddy(cfg.Engines.WorkBuddy, st)
		m.register(eb.Name(), eb, eb)
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
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// bootstrapAccounts 启动装配：存储 → 号池（含 config 静态账号迁移、auth 目录反向导入）。
func (m *Manager) bootstrapAccounts(ctx context.Context) error {
	accounts, err := m.st.ListAccounts()
	if err != nil {
		return fmt.Errorf("读取账号存储失败: %w", err)
	}

	// 首次启动：把 config 里的静态账号迁移进存储（作为种子）
	if len(accounts) == 0 {
		for _, acc := range m.cfg.EngineA.Accounts {
			if strings.TrimSpace(acc.PSID) == "" {
				continue
			}
			creds, _ := json.Marshal(GeminiCredentials{PSID: acc.PSID, PSIDTS: acc.PSIDTS})
			if _, err := m.st.CreateAccount("a", seedLabel("a", acc.Name), string(creds), true); err != nil {
				continue
			}
		}
		// 订阅引擎种子（codex / antigravity / kiro / workbuddy）
		for _, acc := range m.cfg.Engines.Codex.Accounts {
			if strings.TrimSpace(acc.RefreshToken) == "" {
				continue
			}
			creds := marshalCompact(CodexCredentials{RefreshToken: acc.RefreshToken, Email: acc.Email})
			_, _ = m.st.CreateAccount("codex", seedLabel("codex", firstNonEmpty(acc.Name, acc.Email)), creds, true)
		}
		for _, acc := range m.cfg.Engines.Antigravity.Accounts {
			if strings.TrimSpace(acc.RefreshToken) == "" {
				continue
			}
			creds := marshalCompact(AntigravityCredentials{RefreshToken: acc.RefreshToken, Email: acc.Email, ProjectID: acc.ProjectID})
			_, _ = m.st.CreateAccount("antigravity", seedLabel("antigravity", firstNonEmpty(acc.Name, acc.Email)), creds, true)
		}
		for _, acc := range m.cfg.Engines.Kiro.Accounts {
			if strings.TrimSpace(acc.RefreshToken) == "" {
				continue
			}
			creds := marshalCompact(KiroCredentials{RefreshToken: acc.RefreshToken, Email: acc.Email})
			_, _ = m.st.CreateAccount("kiro", seedLabel("kiro", firstNonEmpty(acc.Name, acc.Email)), creds, true)
		}
		for _, acc := range m.cfg.Engines.WorkBuddy.Accounts {
			if strings.TrimSpace(acc.Token) == "" {
				continue
			}
			creds := marshalCompact(WorkBuddyCredentials{Token: acc.Token, Email: acc.Email})
			_, _ = m.st.CreateAccount("workbuddy", seedLabel("workbuddy", firstNonEmpty(acc.Name, acc.Email)), creds, true)
		}
		accounts, err = m.st.ListAccounts()
		if err != nil {
			return err
		}
	}

	for _, acc := range accounts {
		switch acc.Engine {
		case "a":
			if m.engineA == nil {
				continue
			}
			var creds GeminiCredentials
			if err := json.Unmarshal([]byte(acc.Credentials), &creds); err != nil {
				continue
			}
			_ = m.engineA.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds.PSID, creds.PSIDTS)
			if !acc.Enabled {
				_ = m.engineA.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
			}
		case "b":
			native, ok := m.engineB.(*NativeAIStudioEngine)
			if !ok || native == nil {
				continue
			}
			var creds AIStudioCredentials
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

	// 反向导入：auth 目录已存在但存储没有的账号（AIStudio2API 生成的 auth 目录直用）
	if native, ok := m.engineB.(*NativeAIStudioEngine); ok && native != nil {
		for _, poolID := range native.PoolAccountIDs() {
			if _, err := m.st.GetAccountByLabel("b", poolID); err == nil {
				continue
			}
			if _, err := m.st.CreateAccount("b", poolID, "{}", true); err == nil {
				native.RegisterAccountID(fmt.Sprintf("%d", lastAccountID(m.st)), poolID)
			}
		}
	}

	// 订阅引擎账号装配（codex / antigravity / kiro / workbuddy）
	for _, acc := range accounts {
		switch acc.Engine {
		case "codex":
			if eng := m.codexEngine(); eng != nil {
				var creds CodexCredentials
				if json.Unmarshal([]byte(acc.Credentials), &creds) == nil && creds.RefreshToken != "" {
					_ = eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds)
					if !acc.Enabled {
						_ = eng.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
					}
				}
			}
		case "antigravity":
			if eng := m.antigravityEngine(); eng != nil {
				var creds AntigravityCredentials
				if json.Unmarshal([]byte(acc.Credentials), &creds) == nil && creds.RefreshToken != "" {
					_ = eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds)
					if !acc.Enabled {
						_ = eng.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
					}
				}
			}
		case "kiro":
			if eng := m.kiroEngine(); eng != nil {
				var creds KiroCredentials
				if json.Unmarshal([]byte(acc.Credentials), &creds) == nil && creds.RefreshToken != "" {
					_ = eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds)
					if !acc.Enabled {
						_ = eng.SetEnabled(fmt.Sprintf("%d", acc.ID), false)
					}
				}
			}
		case "workbuddy":
			if eng := m.mWorkBuddyEngine(); eng != nil {
				var creds WorkBuddyCredentials
				if json.Unmarshal([]byte(acc.Credentials), &creds) == nil && creds.Token != "" {
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
func (m *Manager) codexEngine() *CodexEngine {
	if eng, ok := m.engines["codex"]; ok {
		if c, ok := eng.(*CodexEngine); ok {
			return c
		}
	}
	return nil
}

func (m *Manager) antigravityEngine() *AntigravityEngine {
	if eng, ok := m.engines["antigravity"]; ok {
		if a, ok := eng.(*AntigravityEngine); ok {
			return a
		}
	}
	return nil
}

func (m *Manager) kiroEngine() *KiroEngine {
	if eng, ok := m.engines["kiro"]; ok {
		if k, ok := eng.(*KiroEngine); ok {
			return k
		}
	}
	return nil
}

func (m *Manager) mWorkBuddyEngine() *WorkBuddyEngine {
	if eng, ok := m.engines["workbuddy"]; ok {
		if w, ok := eng.(*WorkBuddyEngine); ok {
			return w
		}
	}
	return nil
}

func poolHas(native *NativeAIStudioEngine, accountID string) bool {
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
func (m *Manager) EngineA() engine.Engine {
	if m.engineA == nil {
		return nil
	}
	return m.engineA
}

func (m *Manager) EngineB() engine.Engine {
	if m.engineB == nil {
		return nil
	}
	return m.engineB
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
	case "a":
		return m.cfg.EngineA.Proxy
	case "b":
		return m.cfg.EngineB.Proxy
	case "codex":
		return m.cfg.Engines.Codex.Proxy
	case "antigravity":
		return m.cfg.Engines.Antigravity.Proxy
	case "kiro":
		return m.cfg.Engines.Kiro.Proxy
	case "workbuddy":
		return m.cfg.Engines.WorkBuddy.Proxy
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

// ==================== OAuth 授权会话 ====================

// OAuthPendingSession 管理台发起的 OAuth 授权会话（authorize → exchange）。
type OAuthPendingSession struct {
	Engine      string    `json:"engine"`
	SessionID   string    `json:"session_id"`
	State       string    `json:"-"`
	CodeVerifier string   `json:"-"`
	AuthURL     string    `json:"auth_url"`
	Proxy       string    `json:"-"`
	CreatedAt   time.Time `json:"-"`
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
	engineName := strings.ToLower(strings.TrimSpace(req.Engine))
	if engineName == "" {
		engineName = "b"
	}
	switch engineName {
	case "a", "gemini", "b", "aistudio":
		if strings.TrimSpace(req.StorageState) == "" {
			return store.Account{}, errors.New("storage_state 不能为空")
		}
	}
	switch engineName {
	case "a", "gemini":
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
		var creds CodexCredentials
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
		var creds AntigravityCredentials
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
		var creds KiroCredentials
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
		var creds WorkBuddyCredentials
		if err := decodeEngineCredentials(req, &creds); err != nil {
			return store.Account{}, err
		}
		if creds.Token == "" {
			return store.Account{}, errors.New("token 不能为空")
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
		return m.addWorkBuddyAccount(label, creds)
	default:
		return store.Account{}, fmt.Errorf("未知引擎: %s（支持 a、b、codex、antigravity、kiro、workbuddy）", req.Engine)
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
func (m *Manager) addCodexAccount(label string, creds CodexCredentials) (store.Account, error) {
	eng := m.codexEngine()
	if eng == nil {
		return store.Account{}, errors.New("codex 引擎未启用")
	}
	if label == "" {
		return store.Account{}, errors.New("label 或 email 至少填写一个")
	}
	acc, err := m.st.CreateAccount("codex", label, marshalCompact(creds), true)
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
func (m *Manager) addAntigravityAccount(label string, creds AntigravityCredentials) (store.Account, error) {
	eng := m.antigravityEngine()
	if eng == nil {
		return store.Account{}, errors.New("antigravity 引擎未启用")
	}
	if label == "" {
		return store.Account{}, errors.New("label 或 email 至少填写一个")
	}
	acc, err := m.st.CreateAccount("antigravity", label, marshalCompact(creds), true)
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
func (m *Manager) addKiroAccount(label string, creds KiroCredentials) (store.Account, error) {
	eng := m.kiroEngine()
	if eng == nil {
		return store.Account{}, errors.New("kiro 引擎未启用")
	}
	if label == "" {
		return store.Account{}, errors.New("label 或 email 至少填写一个")
	}
	acc, err := m.st.CreateAccount("kiro", label, marshalCompact(creds), true)
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
func (m *Manager) addWorkBuddyAccount(label string, creds WorkBuddyCredentials) (store.Account, error) {
	eng := m.mWorkBuddyEngine()
	if eng == nil {
		return store.Account{}, errors.New("workbuddy 引擎未启用")
	}
	if label == "" {
		return store.Account{}, errors.New("label 或 email 至少填写一个")
	}
	acc, err := m.st.CreateAccount("workbuddy", label, marshalCompact(creds), true)
	if err != nil {
		return store.Account{}, err
	}
	if err := eng.AddAccount(fmt.Sprintf("%d", acc.ID), acc.Label, creds); err != nil {
		_ = m.st.UpdateAccountStatus(acc.ID, "error", err.Error())
		return acc, nil
	}
	return acc, nil
}

// UpdateSubscriptionCredentials 更新订阅引擎账号凭据（codex/antigravity/kiro/workbuddy）。
func (m *Manager) UpdateSubscriptionCredentials(id int64, credentials json.RawMessage) error {
	acc, err := m.st.GetAccount(id)
	if err != nil {
		return err
	}
	switch acc.Engine {
	case "codex":
		var creds CodexCredentials
		if err := json.Unmarshal(credentials, &creds); err != nil || creds.RefreshToken == "" {
			return errors.New("credentials.refresh_token 不能为空")
		}
		if err := m.st.UpdateAccountCredentials(id, string(credentials)); err != nil {
			return err
		}
		return m.codexEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
	case "antigravity":
		var creds AntigravityCredentials
		if err := json.Unmarshal(credentials, &creds); err != nil || creds.RefreshToken == "" {
			return errors.New("credentials.refresh_token 不能为空")
		}
		if err := m.st.UpdateAccountCredentials(id, string(credentials)); err != nil {
			return err
		}
		return m.antigravityEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
	case "kiro":
		var creds KiroCredentials
		if err := json.Unmarshal(credentials, &creds); err != nil || creds.RefreshToken == "" {
			return errors.New("credentials.refresh_token 不能为空")
		}
		if err := m.st.UpdateAccountCredentials(id, string(credentials)); err != nil {
			return err
		}
		return m.kiroEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
	case "workbuddy":
		var creds WorkBuddyCredentials
		if err := json.Unmarshal(credentials, &creds); err != nil || creds.Token == "" {
			return errors.New("credentials.token 不能为空")
		}
		if err := m.st.UpdateAccountCredentials(id, string(credentials)); err != nil {
			return err
		}
		return m.mWorkBuddyEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
	}
	return fmt.Errorf("引擎 %s 不支持凭据更新", acc.Engine)
}

// geminiCredentialsFromStorageState 从引擎B协议的标准 Cookie 数组中提取
// Gemini 网页协议需要的两个会话 Cookie。
func geminiCredentialsFromStorageState(raw string) (GeminiCredentials, error) {
	var state struct {
		Cookies []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"cookies"`
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return GeminiCredentials{}, fmt.Errorf("storage-state 不是合法 JSON: %w", err)
	}
	var creds GeminiCredentials
	for _, cookie := range state.Cookies {
		switch strings.TrimSpace(cookie.Name) {
		case "__Secure-1PSID":
			creds.PSID = strings.TrimSpace(cookie.Value)
		case "__Secure-1PSIDTS":
			creds.PSIDTS = strings.TrimSpace(cookie.Value)
		}
	}
	if creds.PSID == "" || creds.PSIDTS == "" {
		return GeminiCredentials{}, errors.New("storage-state 缺少 __Secure-1PSID 或 __Secure-1PSIDTS Cookie")
	}
	return creds, nil
}

func (m *Manager) addGeminiCredentials(label, psid, psidts string) (store.Account, error) {
	if m.engineA == nil {
		return store.Account{}, errors.New("引擎A 未启用")
	}
	creds, _ := json.Marshal(GeminiCredentials{PSID: psid, PSIDTS: psidts})
	acc, err := m.st.CreateAccount("a", label, string(creds), true)
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
	return m.AddAccountFromStorageState(AccountInput{Engine: "a", Label: label, StorageState: string(state)})
}

// AddAIStudioAccount 添加引擎B 账号（AI Studio 号）。
func (m *Manager) AddAIStudioAccount(email, storageJSON, locale, timezone, proxy string) (store.Account, error) {
	return m.AddAccountFromStorageState(AccountInput{
		Engine: "b", Email: email, StorageState: storageJSON,
		Locale: locale, Timezone: timezone, Proxy: proxy,
	})
}

func (m *Manager) addAIStudioAccount(email, storageJSON, locale, timezone, proxy string) (store.Account, error) {
	native, ok := m.engineB.(*NativeAIStudioEngine)
	if !ok || native == nil {
		return store.Account{}, errors.New("引擎B 原生模式未启用")
	}
	var state json.RawMessage
	if err := json.Unmarshal([]byte(storageJSON), &state); err != nil {
		return store.Account{}, fmt.Errorf("storage-state 不是合法 JSON: %w", err)
	}
	creds, _ := json.Marshal(AIStudioCredentials{
		StorageState: state, Locale: locale, Timezone: timezone, Proxy: proxy,
	})
	acc, err := m.st.CreateAccount("b", email, string(creds), true)
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
	creds, _ := json.Marshal(GeminiCredentials{PSID: psid, PSIDTS: psidts})
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
	return m.st.DeleteAccount(id)
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
	case "a":
		if m.engineA == nil {
			return errors.New("引擎A 未启用")
		}
		var creds GeminiCredentials
		if err := json.Unmarshal([]byte(acc.Credentials), &creds); err != nil {
			return err
		}
		if err := m.engineA.ReplaceCredentials(engineID, creds.PSID, creds.PSIDTS); err != nil {
			return err
		}
		_ = m.st.UpdateAccountStatus(id, "initializing", "重新初始化中")
		return nil
	case "b":
		native, ok := m.engineB.(*NativeAIStudioEngine)
		if !ok || native == nil {
			return errors.New("引擎B 原生模式未启用")
		}
		if _, err := native.RefreshAccountModels(ctx, acc.Label); err != nil {
			return err
		}
		_ = m.st.UpdateAccountStatus(id, "ok", "")
		return nil
	case "codex", "antigravity", "kiro", "workbuddy":
		// 订阅引擎：重建账号（重新初始化 token / project）
		if err := m.UpdateSubscriptionCredentials(id, json.RawMessage(acc.Credentials)); err != nil {
			return err
		}
		_ = m.st.UpdateAccountStatus(id, "initializing", "重新初始化中")
		return nil
	}
	return fmt.Errorf("未知引擎: %s", acc.Engine)
}

// AccountOverview 号池总览（存储记录 + 引擎实时状态合并）。
func (m *Manager) AccountOverview() []AccountOverview {
	accounts, err := m.st.ListAccounts()
	if err != nil {
		return nil
	}
	// 收集每个引擎的实时账号状态
	live := map[string]map[string]AccountState{}
	for name, pool := range m.pools {
		states := map[string]AccountState{}
		for _, st := range pool.AccountStates() {
			states[st.ID] = st
		}
		live[name] = states
	}
	out := make([]AccountOverview, 0, len(accounts))
	for _, acc := range accounts {
		ov := AccountOverview{Account: acc}
		engineID := fmt.Sprintf("%d", acc.ID)
		if states, ok := live[acc.Engine]; ok {
			if st, ok := states[engineID]; ok {
				ov.Live = st
			} else {
				ov.Live = AccountState{ID: engineID, Label: acc.Label, Enabled: acc.Enabled, Status: "unloaded"}
			}
		} else {
			// 引擎未启用（无池）
			ov.Live = AccountState{ID: engineID, Label: acc.Label, Enabled: acc.Enabled, Status: "unloaded", Detail: "引擎未启用"}
		}
		out = append(out, ov)
	}
	return out
}

// ==================== 路由与对话 ====================

// pickEngine 按模型名路由引擎。第二个返回值表示模型是否为该引擎专属
//（订阅引擎目录命中或订阅引擎显式路由）：专属模型引擎不可用时不做
// 跨引擎切换——其他引擎不认识该模型，切换只会产生误导性错误。
func (m *Manager) pickEngine(modelName string) (engine.Engine, bool) {
	name := strings.ToLower(strings.TrimSpace(modelName))

	// 1) 显式模型路由：engine_a/b_models + routing.engine_models.<engine>
	if len(m.engineAModels) > 0 && matchAny(name, m.engineAModels) {
		if eng, ok := m.engines["a"]; ok {
			return eng, false
		}
	}
	if len(m.engineBModels) > 0 && matchAny(name, m.engineBModels) {
		if eng, ok := m.engines["b"]; ok {
			return eng, false
		}
	}
	for engName, patterns := range m.engineModels {
		if len(patterns) == 0 {
			continue
		}
		if matchAny(name, patterns) {
			if eng, ok := m.engines[strings.ToLower(engName)]; ok {
				return eng, isSubscriptionEngine(strings.ToLower(engName))
			}
		}
	}

	// 2) 多模态模型走引擎B（保持历史行为）
	if eng, ok := m.engines["b"]; ok && isMultimodalModel(name) {
		return eng, false
	}

	// 3) default_engine 显式指定（支持任意注册引擎）
	if m.defaultEngine != "auto" && m.defaultEngine != "" {
		if eng, ok := m.engines[m.defaultEngine]; ok {
			return eng, isSubscriptionEngine(m.defaultEngine)
		}
	}

	// 4) auto：模型名精确命中某订阅引擎目录 → 该引擎（模型专属）
	for _, engName := range m.engineOrder {
		if engName == "a" || engName == "b" {
			continue
		}
		if eng, ok := m.engines[engName]; ok {
			for _, mi := range eng.ListModels() {
				if strings.EqualFold(mi.ID, name) {
					return eng, true
				}
			}
		}
	}

	// 5) auto 回退：引擎B协议是主协议，其次引擎A，最后任意 Ready 引擎
	if eng, ok := m.engines["b"]; ok {
		return eng, false
	}
	if eng, ok := m.engines["a"]; ok {
		return eng, false
	}
	for _, engName := range m.engineOrder {
		if eng, ok := m.engines[engName]; ok && eng.Ready() {
			return eng, false
		}
	}
	return nil, false
}

// isSubscriptionEngine 是否为订阅引擎（codex/antigravity/kiro/workbuddy）。
func isSubscriptionEngine(name string) bool {
	switch name {
	case "codex", "antigravity", "kiro", "workbuddy":
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

// Chat 非流式对话。
func (m *Manager) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	eng, owned := m.pickEngine(req.Model)
	if eng == nil {
		return nil, errors.New("没有可用的引擎")
	}
	if !eng.Ready() {
		if owned {
			// 模型为该订阅引擎专属：不跨引擎切换，直接返回该引擎的具体错误
			return nil, engineUnavailableError(eng)
		}
		alt := m.alternate(eng)
		if alt != nil {
			eng = alt
		} else {
			return nil, engineUnavailableError(eng)
		}
	}
	return eng.Chat(ctx, req)
}

// ChatStream 流式对话。
func (m *Manager) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	eng, owned := m.pickEngine(req.Model)
	if eng == nil {
		return nil, errors.New("没有可用的引擎")
	}
	if !eng.Ready() {
		if owned {
			return nil, engineUnavailableError(eng)
		}
		alt := m.alternate(eng)
		if alt != nil {
			eng = alt
		} else {
			return nil, engineUnavailableError(eng)
		}
	}
	return eng.ChatStream(ctx, req, onDelta)
}

// GenerateVideo 通过引擎B的 AI Studio GenerateVideo 协议创建 Veo 长任务。
func (m *Manager) GenerateVideo(ctx context.Context, request aistudio.VideoRequest) (aistudio.VideoOperation, error) {
	native, ok := m.engineB.(*NativeAIStudioEngine)
	if !ok || native == nil {
		return aistudio.VideoOperation{}, errors.New("当前未启用引擎B原生 Veo 协议")
	}
	if !native.Ready() {
		return aistudio.VideoOperation{}, engineUnavailableError(native)
	}
	return native.GenerateVideo(ctx, request)
}

// GetGenerateVideoOperation 查询 Veo 长任务状态。
func (m *Manager) GetGenerateVideoOperation(ctx context.Context, operationID string) (aistudio.VideoOperation, error) {
	native, ok := m.engineB.(*NativeAIStudioEngine)
	if !ok || native == nil {
		return aistudio.VideoOperation{}, errors.New("当前未启用引擎B原生 Veo 协议")
	}
	if !native.Ready() {
		return aistudio.VideoOperation{}, engineUnavailableError(native)
	}
	return native.GetGenerateVideoOperation(ctx, operationID)
}

// DownloadVideoFile 下载已完成 Veo 任务的输出文件。
func (m *Manager) DownloadVideoFile(ctx context.Context, fileID string) (aistudio.MediaStream, error) {
	native, ok := m.engineB.(*NativeAIStudioEngine)
	if !ok || native == nil {
		return aistudio.MediaStream{}, errors.New("当前未启用引擎B原生 Veo 协议")
	}
	if !native.Ready() {
		return aistudio.MediaStream{}, engineUnavailableError(native)
	}
	return native.DownloadVideoFile(ctx, fileID)
}

// engineUnavailableError 保留引擎最近一次初始化/健康检查错误，避免线上只看到
// “引擎不可用”而无法判断是账号、代理、网络还是上游协议问题。
func engineUnavailableError(eng engine.Engine) error {
	msg := fmt.Sprintf("引擎 %s 不可用", eng.Name())
	if status := eng.Status(); status != nil {
		if detail, ok := status["error"].(string); ok && strings.TrimSpace(detail) != "" {
			msg += ": " + strings.TrimSpace(detail)
		} else if accounts, ok := status["accounts"]; ok && emptyStatusList(accounts) {
			msg += ": 没有可用账号（请在管理台添加账号，或检查 auth_states 配置）"
		} else if detail := firstAccountError(status["accounts"]); detail != "" {
			// 账号级错误（凭据失效 / 冷却中等）
			msg += ": " + detail
		}
	}
	return errors.New(msg)
}

// firstAccountError 从账号状态列表中提取第一条错误描述。
func firstAccountError(v any) string {
	switch list := v.(type) {
	case []AccountState:
		for _, st := range list {
			if st.Status == "error" && strings.TrimSpace(st.Detail) != "" {
				return st.Label + ": " + strings.TrimSpace(st.Detail)
			}
		}
	case []any:
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				status, _ := m["status"].(string)
				detail, _ := m["detail"].(string)
				label, _ := m["label"].(string)
				if status == "error" && strings.TrimSpace(detail) != "" {
					if label != "" {
						return label + ": " + strings.TrimSpace(detail)
					}
					return strings.TrimSpace(detail)
				}
			}
		}
	}
	return ""
}

func emptyStatusList(v any) bool {
	switch list := v.(type) {
	case []map[string]any:
		return len(list) == 0
	case []any:
		return len(list) == 0
	default:
		// 订阅引擎的 []AccountState 等具体切片类型
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice {
			return rv.Len() == 0
		}
		return false
	}
}

// alternate 返回另一个引擎（按注册顺序优先就绪的）。
func (m *Manager) alternate(cur engine.Engine) engine.Engine {
	for _, name := range m.engineOrder {
		eng, ok := m.engines[name]
		if !ok || eng == cur {
			continue
		}
		if eng.Ready() {
			return eng
		}
	}
	return nil
}

// ListModels 聚合模型列表（引擎A 保持历史裸 ID 行为，其余引擎以 ID@engine 去重）。
func (m *Manager) ListModels() []model.ModelInfo {
	var out []model.ModelInfo
	seen := map[string]bool{}
	for _, name := range m.engineOrder {
		eng, ok := m.engines[name]
		if !ok {
			continue
		}
		for _, mi := range eng.ListModels() {
			key := mi.ID + "@" + name
			if name == "a" {
				key = mi.ID
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, withModelKind(mi))
		}
	}
	return out
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
		if name == "a" || name == "b" {
			key = "engine_" + name
		}
		out[key] = eng.Status()
	}
	return out
}

// EngineMetaList 管理台引擎元数据列表。
func (m *Manager) EngineMetaList() []EngineMeta {
	metas := []EngineMeta{
		{Name: "a", Describe: "gemini.google.com 网页版（Cookie 会话）", Enabled: m.engines["a"] != nil, AccountPool: true, Credential: "psid/psidts Cookie"},
		{Name: "b", Describe: "AI Studio（原生逆向 / OpenAI 兼容上游）", Enabled: m.engines["b"] != nil, AccountPool: true, Credential: "storage-state.json"},
		{Name: "codex", Describe: "OpenAI Codex（ChatGPT 订阅号）", Enabled: m.engines["codex"] != nil, AccountPool: true, OAuth: "authorize", Credential: "refresh_token（OAuth 授权或 Codex CLI auth.json 导入）"},
		{Name: "antigravity", Describe: "Google Antigravity（Cloud AI Companion）", Enabled: m.engines["antigravity"] != nil, AccountPool: true, OAuth: "authorize", Credential: "refresh_token（Google OAuth 授权）"},
		{Name: "kiro", Describe: "AWS Kiro（CodeWhisperer，Claude 模型）", Enabled: m.engines["kiro"] != nil, AccountPool: true, Credential: "refresh_token（Kiro IDE ~/.aws/sso/cache/kiro-auth-token.json）"},
		{Name: "workbuddy", Describe: "腾讯 CodeBuddy/WorkBuddy（SaaS 订阅）", Enabled: m.engines["workbuddy"] != nil, AccountPool: true, OAuth: "start+poll", Credential: "token（OAuth 扫码授权）"},
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
