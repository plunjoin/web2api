package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/model"
	"web2api/internal/store"
)

// Manager 引擎注册表、模型路由与号池动态管理。
type Manager struct {
	mu sync.RWMutex

	st      *store.Store
	cfg     *config.Config
	engineA *GeminiWebEngine
	engineB initializableEngine

	defaultEngine string
	engineAModels []string
	engineBModels []string
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

// NewManager 构建引擎管理器。
func NewManager(cfg *config.Config, st *store.Store) (*Manager, error) {
	m := &Manager{
		st:            st,
		cfg:           cfg,
		defaultEngine: strings.ToLower(cfg.Routing.DefaultEngine),
		engineAModels: cfg.Routing.EngineAModels,
		engineBModels: cfg.Routing.EngineBModels,
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
		}
	}
	if m.engineA == nil && m.engineB == nil {
		return nil, errors.New("至少启用一个引擎（engine_a 或 engine_b）")
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

// ==================== 号池动态管理 ====================

// AddGeminiAccount 添加引擎A 账号（Gemini 网页号）。
func (m *Manager) AddGeminiAccount(label, psid, psidts string) (store.Account, error) {
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

// AddAIStudioAccount 添加引擎B 账号（AI Studio 号）。
func (m *Manager) AddAIStudioAccount(email, storageJSON, locale, timezone, proxy string) (store.Account, error) {
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
	switch acc.Engine {
	case "a":
		if m.engineA != nil {
			if err := m.engineA.RemoveAccount(engineID); err != nil {
				return err
			}
		}
	case "b":
		if native, ok := m.engineB.(*NativeAIStudioEngine); ok && native != nil {
			if err := native.RemoveAccount(engineID); err != nil {
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
	switch acc.Engine {
	case "a":
		if m.engineA != nil {
			if err := m.engineA.SetEnabled(engineID, enabled); err != nil {
				return err
			}
		}
	case "b":
		if native, ok := m.engineB.(*NativeAIStudioEngine); ok && native != nil {
			if err := native.SetEnabled(engineID, enabled); err != nil {
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
	}
	return fmt.Errorf("未知引擎: %s", acc.Engine)
}

// AccountOverview 号池总览（存储记录 + 引擎实时状态合并）。
func (m *Manager) AccountOverview() []AccountOverview {
	accounts, err := m.st.ListAccounts()
	if err != nil {
		return nil
	}
	liveA := map[string]AccountState{}
	if m.engineA != nil {
		for _, st := range m.engineA.AccountStates() {
			liveA[st.ID] = st
		}
	}
	liveB := map[string]AccountState{}
	if native, ok := m.engineB.(*NativeAIStudioEngine); ok && native != nil {
		for _, st := range native.AccountStates() {
			liveB[st.ID] = st
		}
	}
	out := make([]AccountOverview, 0, len(accounts))
	for _, acc := range accounts {
		ov := AccountOverview{Account: acc}
		engineID := fmt.Sprintf("%d", acc.ID)
		switch acc.Engine {
		case "a":
			if st, ok := liveA[engineID]; ok {
				ov.Live = st
			} else {
				ov.Live = AccountState{ID: engineID, Label: acc.Label, Enabled: acc.Enabled, Status: "unloaded"}
			}
		case "b":
			if st, ok := liveB[engineID]; ok {
				ov.Live = st
			} else {
				ov.Live = AccountState{ID: engineID, Label: acc.Label, Enabled: acc.Enabled, Status: "unloaded"}
			}
		}
		out = append(out, ov)
	}
	return out
}

// ==================== 路由与对话 ====================

// pickEngine 按模型名路由引擎。
func (m *Manager) pickEngine(modelName string) engine.Engine {
	name := strings.ToLower(strings.TrimSpace(modelName))

	if len(m.engineAModels) > 0 && matchAny(name, m.engineAModels) && m.engineA != nil {
		return m.engineA
	}
	if len(m.engineBModels) > 0 && matchAny(name, m.engineBModels) && m.engineB != nil {
		return m.engineB
	}

	if m.engineB != nil && isMultimodalModel(name) {
		return m.engineB
	}

	switch m.defaultEngine {
	case "a":
		if m.engineA != nil {
			return m.engineA
		}
	case "b":
		if m.engineB != nil {
			return m.engineB
		}
	}

	if m.engineA != nil {
		return m.engineA
	}
	return m.engineB
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
	eng := m.pickEngine(req.Model)
	if eng == nil {
		return nil, errors.New("没有可用的引擎")
	}
	if !eng.Ready() {
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
	eng := m.pickEngine(req.Model)
	if eng == nil {
		return nil, errors.New("没有可用的引擎")
	}
	if !eng.Ready() {
		alt := m.alternate(eng)
		if alt != nil {
			eng = alt
		} else {
			return nil, engineUnavailableError(eng)
		}
	}
	return eng.ChatStream(ctx, req, onDelta)
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
		}
	}
	return errors.New(msg)
}

func emptyStatusList(v any) bool {
	switch list := v.(type) {
	case []map[string]any:
		return len(list) == 0
	case []any:
		return len(list) == 0
	default:
		return false
	}
}

// alternate 返回另一个引擎（优先就绪的）。
func (m *Manager) alternate(cur engine.Engine) engine.Engine {
	if m.engineA != nil && m.engineA.Ready() {
		return m.engineA
	}
	if m.engineB != nil && m.engineB.Ready() && m.engineB != cur {
		return m.engineB
	}
	return nil
}

// ListModels 聚合模型列表。
func (m *Manager) ListModels() []model.ModelInfo {
	var out []model.ModelInfo
	seen := map[string]bool{}
	if m.engineA != nil {
		for _, mi := range m.engineA.ListModels() {
			if !seen[mi.ID] {
				seen[mi.ID] = true
				out = append(out, mi)
			}
		}
	}
	if m.engineB != nil {
		for _, mi := range m.engineB.ListModels() {
			key := mi.ID + "@b"
			if !seen[key] {
				seen[key] = true
				out = append(out, mi)
			}
		}
	}
	return out
}

// Status 全部引擎状态。
func (m *Manager) Status() map[string]any {
	out := map[string]any{}
	if m.engineA != nil {
		out["engine_a"] = m.engineA.Status()
	}
	if m.engineB != nil {
		out["engine_b"] = m.engineB.Status()
	}
	return out
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
