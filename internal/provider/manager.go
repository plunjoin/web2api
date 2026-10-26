package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	aistudio "web2api/internal/aistudio"
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
	if cfg.Engines.Custom.Enabled {
		ec := NewCustom(cfg.Engines.Custom, st)
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
		for _, acc := range m.cfg.Engines.Custom.Accounts {
			if strings.TrimSpace(acc.BaseURL) == "" || strings.TrimSpace(acc.APIKey) == "" {
				continue
			}
			creds := marshalCompact(CustomCredentials{BaseURL: acc.BaseURL, APIKey: acc.APIKey, Models: acc.Models})
			_, _ = m.st.CreateAccount("custom", seedLabel("custom", firstNonEmpty(acc.Name, acc.BaseURL)), creds, true)
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

	// 反向导入：auth 目录已存在但存储没有的账号（已有的 auth 目录直接复用）
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
		case "custom":
			if eng := m.customEngine(); eng != nil {
				var creds CustomCredentials
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

func (m *Manager) customEngine() *CustomEngine {
	if eng, ok := m.engines["custom"]; ok {
		if c, ok := eng.(*CustomEngine); ok {
			return c
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

// ==================== OAuth 授权会话 ====================

// OAuthPendingSession 管理台发起的 OAuth 授权会话（authorize → exchange）。
type OAuthPendingSession struct {
	Engine       string    `json:"engine"`
	SessionID    string    `json:"session_id"`
	State        string    `json:"-"`
	CodeVerifier string    `json:"-"`
	AuthURL      string    `json:"auth_url"`
	Proxy        string    `json:"-"`
	CreatedAt    time.Time `json:"-"`
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
	case "custom":
		var creds CustomCredentials
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
	default:
		return store.Account{}, fmt.Errorf("未知引擎: %s（支持 a、b、codex、antigravity、kiro、workbuddy、custom）", req.Engine)
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

// addCustomAccount 添加第三方 Key 账号（存储 + 入池）。
func (m *Manager) addCustomAccount(label string, creds CustomCredentials) (store.Account, error) {
	eng := m.customEngine()
	if eng == nil {
		return store.Account{}, errors.New("custom 引擎未启用（engines.custom.enabled）")
	}
	if label == "" {
		return store.Account{}, errors.New("label 不能为空")
	}
	acc, err := m.st.CreateAccount("custom", label, marshalCompact(creds), true)
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
	case "custom":
		var creds CustomCredentials
		if err := json.Unmarshal(credentials, &creds); err != nil {
			return errors.New("credentials 不是合法 JSON")
		}
		if strings.TrimSpace(creds.BaseURL) == "" {
			return errors.New("credentials.base_url 不能为空")
		}
		if strings.TrimSpace(creds.APIKey) == "" {
			return errors.New("credentials.api_key 不能为空")
		}
		if err := m.st.UpdateAccountCredentials(id, marshalCompact(creds)); err != nil {
			return err
		}
		return m.customEngine().ReplaceCredentials(fmt.Sprintf("%d", id), creds)
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
// （配置显式指定、任一引擎目录命中、多模态关键词）：专属模型引擎不可用时
// 不做跨引擎切换——其他引擎不认识该模型，切换只会产生误导性错误。
func (m *Manager) pickEngine(modelName string) (engine.Engine, bool) {
	name := strings.ToLower(strings.TrimSpace(modelName))

	// 1) 显式模型路由：engine_a/b_models + routing.engine_models.<engine>
	//    配置命中即为「专属」：写了就是点名要这个引擎，它挂了就报它的真实错误，
	//    不再被 alternate 带去别的引擎（换过去只会拿到假 404 或错模型）。
	if len(m.engineAModels) > 0 && matchAny(name, m.engineAModels) {
		if eng, ok := m.engines["a"]; ok {
			return eng, true
		}
	}
	if len(m.engineBModels) > 0 && matchAny(name, m.engineBModels) {
		if eng, ok := m.engines["b"]; ok {
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

	// 4) 多模态模型（音乐/视频/生图/语音/向量）只能由声明它的引擎接单。
	//    目录命中已在第 2、3 步处理；这里只处理「当前目录里还没有」的多模态请求：
	//    交给引擎B 并标为专属，引擎B 不可用时直接报引擎B 的真实错误，
	//    绝不再走 default/auto 兜底改投第三方 Key（lyria 打到 MiniMax 的根因）。
	if isMultimodalModel(name) {
		if eng, ok := m.engines["b"]; ok {
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
		if engName == "a" || engName == "b" {
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

// matchCoreCatalog 按模型 id 精确命中引擎A/B 的实时目录（按注册顺序，a 先于 b）。
// 命中即视为模型专属：其他引擎不认识该模型，引擎不可用时直接返回该引擎的
// 具体错误，而不是切换引擎后产生误导性错误。
func (m *Manager) matchCoreCatalog(modelName string) (engine.Engine, bool) {
	for _, engName := range m.engineOrder {
		if engName != "a" && engName != "b" {
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
	name := strings.TrimPrefix(strings.TrimSpace(modelName), "models/")
	if name == "" {
		return false
	}
	for _, mi := range eng.ListModels() {
		id := strings.TrimPrefix(strings.TrimSpace(mi.ID), "models/")
		if id != "" && strings.EqualFold(id, name) {
			return true
		}
	}
	return false
}

// isSubscriptionEngine 是否为「模型专属」引擎（订阅号池或第三方 Key）。
// 命中后引擎不可用时不做跨引擎切换，避免误导性错误。
func isSubscriptionEngine(name string) bool {
	switch name {
	case "codex", "antigravity", "kiro", "workbuddy", "custom":
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
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return "", errors.New("model 不能为空")
	}
	kind := model.ClassifyKind(modelID, "", nil)
	for _, mi := range m.ListAllModels() {
		if strings.EqualFold(mi.ID, modelID) {
			if mi.Kind != "" {
				kind = mi.Kind
			}
			break
		}
	}
	switch kind {
	case model.KindImage:
		return m.probeImageModel(ctx, modelID)
	case model.KindEmbedding:
		return m.probeEmbeddingModel(ctx, modelID)
	case model.KindVideo, model.KindMusic, model.KindSpeech:
		return m.probeCatalogModel(modelID, kind)
	default:
		return m.probeChatModel(ctx, modelID)
	}
}

func (m *Manager) probeChatModel(ctx context.Context, modelID string) (string, error) {
	maxTokens := 16
	temp := 0.0
	res, err := m.Chat(ctx, model.ChatRequest{
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

func (m *Manager) probeImageModel(ctx context.Context, modelID string) (string, error) {
	eng, _ := m.pickEngine(modelID)
	switch e := eng.(type) {
	case *CustomEngine:
		return e.ProbeImage(ctx, modelID)
	case *AIStudioEngine:
		if e != nil && e.up != nil {
			return e.up.ProbeImage(ctx, modelID)
		}
	}
	// 原生 AI Studio 等：图片走 chat/completions。
	return m.probeChatModel(ctx, modelID)
}

func (m *Manager) probeEmbeddingModel(ctx context.Context, modelID string) (string, error) {
	eng, _ := m.pickEngine(modelID)
	switch e := eng.(type) {
	case *CustomEngine:
		return e.ProbeEmbedding(ctx, modelID)
	case *AIStudioEngine:
		if e != nil && e.up != nil {
			return e.up.ProbeEmbedding(ctx, modelID)
		}
	}
	return "", fmt.Errorf("当前引擎不支持向量探测，请手动验证")
}

// probeCatalogModel 视频/音乐/语音类模型的可接单检查。
// 不做真实生成（昂贵且会产生计费），但也不做「在目录里就算过」的假探测：
// 至少要满足「有账号声明该模型」+「归属引擎能接单」两条。
func (m *Manager) probeCatalogModel(modelID, kind string) (string, error) {
	eng, _ := m.pickEngine(modelID)
	if eng == nil {
		return "", newModelNotFound(modelID, m.ownershipHint(modelID))
	}
	if !engineHasModel(eng, modelID) {
		return "", newModelNotFound(modelID, m.ownershipHint(modelID))
	}
	// 目录里有、但没有任何可用账号声明它（引擎自报 available=false）同样不算可用。
	want := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(modelID), "models/"))
	for _, mi := range m.ListAllModels() {
		id := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(mi.ID), "models/"))
		if id != want {
			continue
		}
		if !mi.Available {
			return "", fmt.Errorf("模型 %s 在 %s 引擎目录中，但没有可用账号声明它（账号停用 / 未拉到目录 / 冷却中）", modelID, eng.Name())
		}
		break
	}
	if !eng.Ready() {
		return "", engineUnavailableError(eng)
	}
	return fmt.Sprintf("引擎 %s 已声明该模型且引擎可接单（%s 模型未做生成探测）", eng.Name(), kindLabel(kind)), nil
}

func kindLabel(kind string) string {
	switch kind {
	case model.KindVideo:
		return "视频"
	case model.KindMusic:
		return "音乐"
	case model.KindSpeech:
		return "语音"
	case model.KindImage:
		return "生图"
	case model.KindEmbedding:
		return "向量"
	default:
		return "对话"
	}
}

// Chat 非流式对话。
func (m *Manager) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	eng, req, err := m.resolveChatEngine(req)
	if err != nil {
		return nil, err
	}
	return eng.Chat(ctx, req)
}

// ChatStream 流式对话。
func (m *Manager) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	eng, req, err := m.resolveChatEngine(req)
	if err != nil {
		return nil, err
	}
	return eng.ChatStream(ctx, req, onDelta)
}

// resolveChatEngine 选定引擎。目录命中的模型归属其引擎：引擎不可用时直接返回
// 该引擎的具体错误；引擎B 收到目录外模型时不再静默换模，由引擎B 返回明确的
// 目录错误，避免「换用不可用模型后报无可用账户」这类误导性错误。
func (m *Manager) resolveChatEngine(req model.ChatRequest) (engine.Engine, model.ChatRequest, error) {
	// 用途闸门：音乐/视频/向量模型走 /v1/chat/completions 一定拿不到结果。
	// 第三方 OpenAI 兼容端点只会把请求原样发出去，然后被上游以
	// “Unhandled generated data mime type: audio/mpeg” 之类的 400 拒掉。
	// 能出媒体（引擎B 原生）就交给它，否则直接给调用方一个能看懂的 400。
	kind := m.kindOf(req.Model)
	if model.ChatIncompatibleKind(kind) {
		if mediaEng, ok := m.chatMediaEngine(); ok {
			if !mediaEng.Ready() {
				return nil, req, engineUnavailableError(mediaEng)
			}
			return mediaEng, req, nil
		}
		return nil, req, newUnsupportedEndpoint(req.Model, kind)
	}

	eng, owned := m.pickEngine(req.Model)
	if eng == nil {
		return nil, req, newModelNotFound(req.Model, m.ownershipHint(req.Model))
	}
	if !eng.Ready() {
		// 多模态（lyria / veo / image / tts 等）与目录已标明归属的模型禁止换引擎：
		// 换到 custom 只会把请求打到不相干的第三方供应商，最终以假 404 收场。
		// 目标引擎不可用时返回它自己的真实错误（如 engine b: worker_ready=0）。
		if owned || isMultimodalModel(strings.ToLower(strings.TrimSpace(req.Model))) {
			return nil, req, engineUnavailableError(eng)
		}
		alt := m.alternate(eng, req.Model)
		if alt != nil {
			eng = alt
		} else {
			return nil, req, engineUnavailableError(eng)
		}
	}
	return eng, req, nil
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
		switch {
		case statusString(status["error"]) != "":
			msg += ": " + statusString(status["error"])
		case statusString(status["worker_error"]) != "":
			// 账号 ready 但 WAA Worker 起不来：这层信息最容易被漏掉，单独透出。
			msg += ": WAA Worker 未就绪（worker_ready=" + statusInt(status["worker_ready"]) + "）: " + statusString(status["worker_error"])
		case status["worker_ready"] != nil && statusInt(status["worker_ready"]) == "0":
			msg += ": WAA Worker 未就绪（worker_ready=0），账号在池但没有任何可用的工人"
		case emptyStatusList(status["accounts"]):
			msg += ": 没有可用账号（请在管理台添加账号，或检查 auth_states 配置）"
		case firstAccountError(status["accounts"]) != "":
			// 账号级错误（凭据失效 / 冷却中等）
			msg += ": " + firstAccountError(status["accounts"])
		}
	}
	return errors.New(msg)
}

// statusString 安全读取状态里的字符串字段。
func statusString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// statusInt 安全读取状态里的整数字段（数字或字符串形式）。
func statusInt(v any) string {
	switch n := v.(type) {
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case string:
		return strings.TrimSpace(n)
	default:
		return "0"
	}
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
// 声明式号池（第三方 Key）只认自己的模型目录：目标模型不在其目录里就不换过去，
// 否则换过去只会拿到 model_not_found，把真正的故障源（如引擎B 挂了）盖掉。
func (m *Manager) alternate(cur engine.Engine, modelName string) engine.Engine {
	for _, name := range m.engineOrder {
		eng, ok := m.engines[name]
		if !ok || eng == cur {
			continue
		}
		if !eng.Ready() {
			continue
		}
		if _, strict := eng.(*CustomEngine); strict && !engineHasModel(eng, modelName) {
			continue
		}
		return eng
	}
	return nil
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
		for _, mi := range eng.ListModels() {
			id := strings.TrimSpace(mi.ID)
			if id == "" {
				continue
			}
			if _, skip := disabled[id]; skip {
				continue
			}
			mi = withModelKind(mi)
			mi.ID = id
			if mi.Engine == "" {
				mi.Engine = name
			}
			if idx, ok := indexByID[id]; ok {
				// 同名：保留先出现的条目，合并可用态与更具体的用途。
				out[idx].Available = out[idx].Available || mi.Available
				out[idx].Kind = model.PreferKind(out[idx].Kind, mi.Kind)
				if out[idx].DisplayName == "" {
					out[idx].DisplayName = mi.DisplayName
				}
				continue
			}
			indexByID[id] = len(out)
			out = append(out, mi)
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
	eng, ok := m.matchSubscriptionCatalog(name)
	if !ok {
		return false, nil
	}
	custom, isCustom := eng.(*CustomEngine)
	if !isCustom {
		// 订阅引擎不是通用 OpenAI 兼容上游，维持引擎B回退。
		return false, nil
	}
	return true, custom.ServeModelPassthrough(ctx, name, method, path, body, contentType, w)
}
