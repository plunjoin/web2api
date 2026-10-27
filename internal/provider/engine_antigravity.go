// engine_antigravity.go 引擎 antigravity：Google Antigravity（Cloud AI Companion）
// 订阅号池。凭据为 Google OAuth refresh_token，project_id 自动 onboard。
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/model"
	"web2api/internal/store"
	"web2api/internal/upstream/antigravity"
)

// AntigravityCredentials antigravity 引擎凭据（accounts.credentials JSON）。
type AntigravityCredentials struct {
	RefreshToken string `json:"refresh_token"`
	Email        string `json:"email,omitempty"`
	ProjectID    string `json:"project_id,omitempty"` // cloudaicompanion 项目，留空自动 onboard
	Proxy        string `json:"proxy,omitempty"`
}

// antigravityAccount 池内账号。
type antigravityAccount struct {
	id, label string
	mu        sync.Mutex
	creds     AntigravityCredentials
	enabled   bool

	accessToken string
	expiresAt   time.Time
	ready       bool
	lastErr     string
	planType    string                   // 订阅档位
	credits     string                   // AI Credits 余额描述
	models      []antigravity.ModelMeta  // 上游真实模型目录（网关可路由的全量）
	selector    []string                 // 官方下拉模型（agentModelSorts ∩ 目录）
	quotaGroups []antigravity.QuotaGroup // 官方用量分组（weekly + 5h 窗口）

	cooldownUntil  time.Time
	cooldownReason string
}

func (a *antigravityAccount) proxy(engineProxy string) string {
	if strings.TrimSpace(a.creds.Proxy) != "" {
		return a.creds.Proxy
	}
	return engineProxy
}

// AntigravityEngine antigravity 引擎。
type AntigravityEngine struct {
	name  string
	proxy string
	st    *store.Store

	mu       sync.RWMutex
	accounts []*antigravityAccount
	ids      map[string]int
	next     atomic.Uint64

	errMu   sync.Mutex
	lastErr error
}

// NewAntigravity 构建 antigravity 引擎。
func NewAntigravity(cfg config.AntigravityConfig, st *store.Store) *AntigravityEngine {
	return &AntigravityEngine{
		name:  "antigravity",
		proxy: strings.TrimSpace(cfg.Proxy),
		st:    st,
		ids:   map[string]int{},
	}
}

// AddAccount 添加账号并异步初始化（刷新 token + 获取 project_id）。
func (e *AntigravityEngine) AddAccount(id, label string, creds AntigravityCredentials) error {
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return errors.New("refresh_token 不能为空")
	}
	e.mu.Lock()
	if _, exists := e.ids[id]; exists {
		e.mu.Unlock()
		return fmt.Errorf("账号 %s 已在池中", id)
	}
	acc := &antigravityAccount{id: id, label: label, creds: creds, enabled: true}
	e.accounts = append(e.accounts, acc)
	e.ids[id] = len(e.accounts) - 1
	e.mu.Unlock()
	go e.initialize(acc)
	return nil
}

// initialize 首次初始化：刷新 token → 确保 project_id。
func (e *AntigravityEngine) initialize(acc *antigravityAccount) {
	acc.mu.Lock()
	refreshToken := acc.creds.RefreshToken
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tok, err := antigravity.Refresh(ctx, proxy, refreshToken)
	if err != nil {
		acc.mu.Lock()
		acc.ready = false
		acc.lastErr = "token 刷新失败: " + err.Error()
		acc.mu.Unlock()
		e.setErr(fmt.Errorf("antigravity 账号 %s 初始化失败: %w", acc.label, err))
		return
	}
	acc.mu.Lock()
	acc.accessToken = tok.AccessToken
	acc.expiresAt = tok.ExpiresAt
	if tok.RefreshToken != "" {
		acc.creds.RefreshToken = tok.RefreshToken
	}
	if tok.Email != "" && acc.creds.Email == "" {
		acc.creds.Email = tok.Email
	}
	projectID := acc.creds.ProjectID
	var planType, credits string
	acc.mu.Unlock()

	// 先查账号信息（套餐 + AI Credits + project 兜底）
	if info, lerr := antigravity.LoadCodeAssist(ctx, proxy, tok.AccessToken); lerr == nil && info != nil {
		planType = info.PlanType
		credits = info.Credits
		if projectID == "" && info.ProjectID != "" {
			projectID = info.ProjectID
		}
	}
	if projectID == "" {
		info, err := antigravity.EnsureProject(ctx, proxy, tok.AccessToken)
		if err != nil {
			acc.mu.Lock()
			acc.ready = false
			acc.lastErr = "获取 project_id 失败: " + err.Error()
			acc.mu.Unlock()
			e.setErr(fmt.Errorf("antigravity 账号 %s onboard 失败: %w", acc.label, err))
			return
		}
		projectID = info.ProjectID
		if planType == "" {
			planType = info.PlanType
		}
		acc.mu.Lock()
		acc.creds.ProjectID = projectID
		acc.mu.Unlock()
		if e.st != nil {
			if dbID, perr := strconv.ParseInt(acc.id, 10, 64); perr == nil {
				_ = e.st.UpdateAccountRawCredentials(dbID, marshalCompact(acc.creds))
			}
		}
	}
	acc.mu.Lock()
	acc.planType = planType
	acc.credits = credits
	acc.ready = true
	acc.lastErr = ""
	acc.mu.Unlock()
	// LoadCodeAssist 直接返回 project_id 时同样要写回凭据并落盘
	// （否则后续 refreshCatalog 读到空 project_id，模型目录永远拉不到）。
	needPersist := false
	acc.mu.Lock()
	if acc.creds.ProjectID == "" && projectID != "" {
		acc.creds.ProjectID = projectID
		needPersist = true
	}
	acc.mu.Unlock()
	if needPersist && e.st != nil {
		if dbID, perr := strconv.ParseInt(acc.id, 10, 64); perr == nil {
			acc.mu.Lock()
			raw := marshalCompact(acc.creds)
			acc.mu.Unlock()
			_ = e.st.UpdateAccountRawCredentials(dbID, raw)
		}
	}
	e.setErr(nil)

	// best-effort：拉取真实目录与官方用量（不阻塞 ready）
	go e.refreshCatalog(acc)
	go e.refreshQuotaSummary(acc)
}

// refreshCatalog 拉取账号的真实模型目录与官方下拉模型列表。
func (e *AntigravityEngine) refreshCatalog(acc *antigravityAccount) {
	acc.mu.Lock()
	proxy := acc.proxy(e.proxy)
	accessToken := acc.accessToken
	projectID := acc.creds.ProjectID
	acc.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	catalog, err := antigravity.FetchCatalog(ctx, proxy, accessToken, projectID)
	if err != nil {
		return // 目录拉取失败时继续用已有目录
	}
	acc.mu.Lock()
	acc.models = catalog.Models
	acc.selector = catalog.SelectorModels
	acc.mu.Unlock()
}

// assembleQuota 组合权益展示（对齐官方 IDE "View Usage"）：
// AI Credits 余额 + 各模型分组的周/5 小时剩余比例（含重置倒计时）。
// 官方分组数据缺失时回退为模型 5h 剩余比例的最小值摘要。
func assembleQuota(credits string, groups []antigravity.QuotaGroup, models []antigravity.ModelMeta) string {
	var parts []string
	if s := strings.TrimSpace(credits); s != "" {
		parts = append(parts, s)
	}
	if len(groups) > 0 {
		var segs []string
		for _, g := range groups {
			if seg := formatQuotaGroup(g); seg != "" {
				segs = append(segs, seg)
			}
		}
		if len(segs) > 0 {
			parts = append(parts, strings.Join(segs, "；"))
		}
		return strings.Join(parts, " · ")
	}
	// 回退：无官方分组数据时，取目录中可用模型（remaining>0）的最低剩余比例；
	// 被禁用/耗尽的 0 值条目（如 *-disabled 模型）不参与计算，仅在全部为 0 时显示 0%。
	min := -1.0
	sawZero := false
	for _, m := range models {
		if m.QuotaRemaining < 0 {
			continue // 未知
		}
		if m.QuotaRemaining == 0 {
			sawZero = true
			continue
		}
		if min < 0 || m.QuotaRemaining < min {
			min = m.QuotaRemaining
		}
	}
	switch {
	case min >= 0:
		parts = append(parts, fmt.Sprintf("模型额度剩 %.0f%%", min*100))
	case sawZero:
		parts = append(parts, "模型额度剩 0%")
	}
	return strings.Join(parts, " · ")
}

// formatQuotaGroup 格式化一个官方分组："Gemini 周69%(5d3h) · 5h100%"。
func formatQuotaGroup(g antigravity.QuotaGroup) string {
	var buckets []string
	for _, b := range g.Buckets {
		if s := formatQuotaBucket(b); s != "" {
			buckets = append(buckets, s)
		}
	}
	if len(buckets) == 0 {
		return ""
	}
	return strings.TrimSpace(shortGroupName(g.DisplayName) + " " + strings.Join(buckets, " · "))
}

// shortGroupName 官方分组名缩短显示（"Claude and GPT models" → "Claude/GPT"）。
func shortGroupName(display string) string {
	n := strings.ToLower(display)
	switch {
	case strings.Contains(n, "gemini"):
		return "Gemini"
	case strings.Contains(n, "claude"), strings.Contains(n, "gpt"):
		return "Claude/GPT"
	}
	return strings.TrimSpace(display)
}

// formatQuotaBucket 格式化单个配额窗口："周69%(5d3h)" / "5h100%"。
// 剩余 100% 或无重置时间时不带倒计时。
func formatQuotaBucket(b antigravity.QuotaBucket) string {
	window := quotaWindow(b)
	if window == "" {
		return ""
	}
	label := "5h"
	if window == "weekly" {
		label = "周"
	}
	pct := int(math.Round(b.Remaining * 100))
	if pct < 0 {
		pct = 0
	}
	if pct >= 100 || b.ResetAt.IsZero() {
		return fmt.Sprintf("%s%d%%", label, pct)
	}
	return fmt.Sprintf("%s%d%%(%s)", label, pct, formatCountdown(b.ResetAt))
}

// quotaWindow 判定配额窗口类型（优先 Window 字段，回退解析 ID）。
func quotaWindow(b antigravity.QuotaBucket) string {
	if w := strings.ToLower(strings.TrimSpace(b.Window)); w == "weekly" || w == "5h" {
		return w
	}
	id := strings.ToLower(b.ID)
	if strings.Contains(id, "weekly") {
		return "weekly"
	}
	if strings.Contains(id, "5h") {
		return "5h"
	}
	return ""
}

// formatCountdown 格式化重置倒计时："5d3h" / "4h3m" / "3m"。
func formatCountdown(t time.Time) string {
	d := time.Until(t)
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd%dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh%dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// RemoveAccount 移除账号。
func (e *AntigravityEngine) RemoveAccount(id string) error {
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
func (e *AntigravityEngine) SetEnabled(id string, enabled bool) error {
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
func (e *AntigravityEngine) ReplaceCredentials(id string, creds AntigravityCredentials) error {
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
func (e *AntigravityEngine) AccountStates() []AccountState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]AccountState, 0, len(e.accounts))
	for _, acc := range e.accounts {
		acc.mu.Lock()
		models := len(acc.models)
		if len(acc.selector) > 0 {
			models = len(acc.selector) // 对齐官方下拉的模型数量
		}
		state := AccountState{
			ID:      acc.id,
			Label:   acc.label,
			Enabled: acc.enabled,
			Detail:  acc.lastErr,
			Tier:    acc.creds.Email,
			Quota:   assembleQuota(acc.credits, acc.quotaGroups, acc.models),
			Models:  models,
		}
		if acc.planType != "" {
			state.Tier = acc.planType
		}
		switch {
		case !acc.enabled:
			state.Status = "disabled"
		case time.Now().Before(acc.cooldownUntil):
			state.Status = "error"
			state.Detail = "冷却中: " + acc.cooldownReason
		case !acc.ready:
			state.Status = "initializing"
			if acc.lastErr == "" {
				state.Detail = "等待初始化"
			}
		default:
			state.Status = "ok"
			state.Ready = true
		}
		acc.mu.Unlock()
		out = append(out, state)
	}
	return out
}

// ensureToken 刷新账号 token。
func (e *AntigravityEngine) ensureToken(acc *antigravityAccount) error {
	acc.mu.Lock()
	refreshToken := acc.creds.RefreshToken
	proxy := acc.proxy(e.proxy)
	acc.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	tok, err := antigravity.Refresh(ctx, proxy, refreshToken)
	acc.mu.Lock()
	defer acc.mu.Unlock()
	if err != nil {
		acc.ready = false
		acc.lastErr = "token 刷新失败: " + err.Error()
		e.setErr(fmt.Errorf("antigravity 账号 %s token 刷新失败: %w", acc.label, err))
		return err
	}
	acc.accessToken = tok.AccessToken
	acc.expiresAt = tok.ExpiresAt
	if tok.RefreshToken != "" {
		acc.creds.RefreshToken = tok.RefreshToken
	}
	acc.ready = true
	acc.lastErr = ""
	if e.st != nil {
		if dbID, perr := strconv.ParseInt(acc.id, 10, 64); perr == nil {
			_ = e.st.UpdateAccountRawCredentials(dbID, marshalCompact(acc.creds))
		}
	}
	e.setErr(nil)
	// 顺带刷新套餐/目录与官方用量（token 约每小时刷新一次，额度展示随之更新）
	go e.refreshCatalog(acc)
	go e.refreshEntitlement(acc, proxy, tok.AccessToken)
	go e.refreshQuotaSummary(acc)
	return nil
}

// refreshEntitlement 刷新账号套餐与 AI Credits 余额（best-effort，失败不影响 ready）。
func (e *AntigravityEngine) refreshEntitlement(acc *antigravityAccount, proxy, accessToken string) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	info, err := antigravity.LoadCodeAssist(ctx, proxy, accessToken)
	if err != nil || info == nil {
		return
	}
	acc.mu.Lock()
	if info.PlanType != "" {
		acc.planType = info.PlanType
	}
	if info.ProjectID != "" && acc.creds.ProjectID == "" {
		acc.creds.ProjectID = info.ProjectID
	}
	acc.credits = info.Credits
	acc.mu.Unlock()
}

// refreshQuotaSummary 拉取官方用量分组（weekly + 5h 剩余，对齐 IDE "View Usage"）。
// best-effort，失败不影响 ready；注意 RetrieveQuotaSummary 走直连 HTTP/2，不经过代理。
func (e *AntigravityEngine) refreshQuotaSummary(acc *antigravityAccount) {
	acc.mu.Lock()
	accessToken := acc.accessToken
	acc.mu.Unlock()
	if accessToken == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	groups, err := antigravity.RetrieveQuotaSummary(ctx, accessToken)
	if err != nil || len(groups) == 0 {
		return
	}
	acc.mu.Lock()
	acc.quotaGroups = groups
	acc.mu.Unlock()
}

// token 返回有效 access_token + project_id。
func (e *AntigravityEngine) token(acc *antigravityAccount) (string, string, error) {
	acc.mu.Lock()
	needRefresh := acc.accessToken == "" || (acc.expiresAt.After(time.Time{}) && time.Until(acc.expiresAt) < 3*time.Minute)
	tok := acc.accessToken
	projectID := acc.creds.ProjectID
	acc.mu.Unlock()
	if needRefresh {
		if err := e.ensureToken(acc); err != nil {
			return "", "", err
		}
		acc.mu.Lock()
		tok = acc.accessToken
		projectID = acc.creds.ProjectID
		acc.mu.Unlock()
	}
	if tok == "" {
		return "", "", errors.New("账号没有可用 token")
	}
	if projectID == "" {
		return "", "", errors.New("账号缺少 project_id")
	}
	return tok, projectID, nil
}

// ---- Engine 接口 ----

func (e *AntigravityEngine) Name() string { return e.name }

func (e *AntigravityEngine) Describe() string {
	return "Google Antigravity（Cloud AI Companion 订阅号池，v1internal 协议）"
}

// Ready 至少一个账号可用。
func (e *AntigravityEngine) Ready() bool {
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

// ListModels antigravity 模型目录：优先上游真实目录（fetchAvailableModels），
// 拉取失败时回落内置默认目录。
func (e *AntigravityEngine) ListModels() []model.ModelInfo {
	e.mu.RLock()
	enabled := 0
	real := map[string]bool{}
	for _, acc := range e.accounts {
		acc.mu.Lock()
		if !acc.enabled {
			acc.mu.Unlock()
			continue
		}
		enabled++
		for _, m := range acc.models {
			real[m.ID] = true
		}
		acc.mu.Unlock()
	}
	e.mu.RUnlock()

	var out []model.ModelInfo
	add := func(id string) {
		out = append(out, model.ModelInfo{
			ID:          id,
			Object:      "model",
			DisplayName: id,
			Available:   enabled > 0,
			Engine:      e.name,
		})
	}
	if len(real) > 0 {
		ids := make([]string, 0, len(real))
		for id := range real {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			add(id)
		}
		return out
	}
	for _, id := range antigravity.DefaultModels {
		add(id)
	}
	return out
}

// resolveModel 请求模型名 → 上游真实模型 ID。
// 顺序：真实目录精确命中 → 内置目录精确命中 → 裸名变体解析（gemini-3-pro →
// gemini-3-pro-preview/-high/-low/-tiered 中存在的一个）→ 原样直传。
func (e *AntigravityEngine) resolveModel(acc *antigravityAccount, name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	// 收集该账号可见的真实目录
	acc.mu.Lock()
	catalog := make([]string, 0, len(acc.models))
	for _, m := range acc.models {
		catalog = append(catalog, m.ID)
	}
	acc.mu.Unlock()
	if len(catalog) > 0 {
		for _, id := range catalog {
			if strings.EqualFold(id, lower) {
				return id
			}
		}
		// 裸名 → 变体：优先 preview，再按 high/medium/low/tiered 顺序
		for _, suffix := range []string{"-preview", "-high", "-medium", "-low", "-tiered"} {
			variant := strings.TrimSuffix(lower, "-preview") + suffix
			for _, id := range catalog {
				if strings.EqualFold(id, variant) {
					return id
				}
			}
		}
		return lower // 真实目录存在但未命中：透传（让上游给出明确错误）
	}
	// 无真实目录：内置目录兼容映射
	if mapped, ok := antigravity.DefaultModelVariants[lower]; ok {
		return mapped
	}
	return lower
}

// pick 轮询选择可用账号。
func (e *AntigravityEngine) pick() *antigravityAccount {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := len(e.accounts)
	if n == 0 {
		return nil
	}
	start := int(e.next.Add(1))
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

func (e *AntigravityEngine) markCooldown(acc *antigravityAccount, reason string, d time.Duration) {
	acc.mu.Lock()
	acc.cooldownUntil = time.Now().Add(d)
	acc.cooldownReason = reason
	acc.mu.Unlock()
}

// toAntigravityRequest ChatRequest → antigravity.Request。
// 是否透传 tools 由能力表决定：Claude 等非 Gemini 模型走同一条 v1internal，
// 但不接受 Gemini functionDeclarations，v0.3.8 一律透传后 Codex 自带 tools
// 会让这些模型整请求 400——只有 Gemini 系才开透传（CapsFor）。
func toAntigravityRequest(req model.ChatRequest, projectID string) *antigravity.Request {
	out := &antigravity.Request{
		Model:     req.Model,
		System:    subSystemText(req),
		MaxTokens: derefInt(req.MaxTokens),
		ProjectID: projectID,
	}
	withTools := CapsFor("antigravity", req.Model).Tools
	if withTools {
		out.ToolChoice = req.ToolChoice
		for _, tool := range req.Tools {
			if tool.Function == nil || strings.TrimSpace(tool.Function.Name) == "" {
				continue
			}
			out.Tools = append(out.Tools, antigravity.Tool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			})
		}
	}
	for _, m := range req.Messages {
		role := strings.ToLower(m.Role)
		switch role {
		case "system":
			continue
		case "tool":
			if withTools {
				out.Messages = append(out.Messages, antigravity.Message{
					Role:       "tool",
					Text:       m.Text(),
					ToolCallID: m.ToolCallID,
					ToolName:   m.Name,
				})
			} else {
				// 无工具通道时把 tool 结果折成普通 user 文本，避免上游拒收。
				text := m.Text()
				if name := strings.TrimSpace(m.Name); name != "" {
					text = "[" + name + "] " + text
				}
				out.Messages = append(out.Messages, antigravity.Message{Role: "user", Text: text})
			}
		case "assistant":
			msg := antigravity.Message{
				Role:      "assistant",
				Text:      m.Text(),
				ImageURLs: subImageURLs(m.Content),
			}
			if withTools {
				for _, call := range m.ToolCalls {
					msg.ToolCalls = append(msg.ToolCalls, antigravity.ToolCall{
						ID:         call.ID,
						Name:       call.Function.Name,
						Arguments:  call.Function.Arguments,
						ThoughtSig: thoughtSigFromExtra(call.ExtraContent),
					})
				}
			}
			// 无工具通道时丢掉「只含 tool_calls」的空 assistant，避免上游收到
			// 连续空 model/user 回合（多轮 Codex 工具史）。
			if strings.TrimSpace(msg.Text) == "" && len(msg.ToolCalls) == 0 && len(msg.ImageURLs) == 0 {
				continue
			}
			out.Messages = append(out.Messages, msg)
		default:
			out.Messages = append(out.Messages, antigravity.Message{
				Role:      m.PlainRole(),
				Text:      m.Text(),
				ImageURLs: subImageURLs(m.Content),
			})
		}
	}
	return out
}

// thoughtSigFromExtra 从 Gemini OpenAI 兼容端点的 extra_content 里取出 thought_signature。
func thoughtSigFromExtra(extra json.RawMessage) string {
	if len(extra) == 0 {
		return ""
	}
	var wrap struct {
		Google struct {
			ThoughtSignature string `json:"thought_signature"`
		} `json:"google"`
	}
	if json.Unmarshal(extra, &wrap) == nil && strings.TrimSpace(wrap.Google.ThoughtSignature) != "" {
		return wrap.Google.ThoughtSignature
	}
	return ""
}

// chatOnce 单账号一次请求（401 刷新重试；带 tools 的 400 会无工具再试一次）。
func (e *AntigravityEngine) chatOnce(ctx context.Context, acc *antigravityAccount, req *antigravity.Request, onDelta func(string) error, onTool func(antigravity.ToolCallDelta) error) (*antigravity.Result, error) {
	attempt := func(r *antigravity.Request, toolsCB func(antigravity.ToolCallDelta) error) (*antigravity.Result, error) {
		accessToken, projectID, err := e.token(acc)
		if err != nil {
			return nil, err
		}
		r.ProjectID = projectID
		r.Model = e.resolveModel(acc, r.Model)
		return antigravity.StreamWithTools(ctx, acc.proxy(e.proxy), accessToken, r, onDelta, toolsCB)
	}
	res, err := attempt(req, onTool)
	if err != nil && antigravity.IsUnauthorized(err) {
		if refreshErr := e.ensureToken(acc); refreshErr != nil {
			return nil, fmt.Errorf("token 已失效且刷新失败: %w", refreshErr)
		}
		res, err = attempt(req, onTool)
	}
	// Gemini 工具协议偶发不被上游接受（schema / 模型档位）：去掉 tools 再试，
	// 恢复成 v0.3.7「静默丢工具、纯文本仍可用」的行为，避免整请求挂死。
	// 仅对开了工具透传的模型（能力表 RetryNoToolsOn400）生效；req.Tools
	// 非空本身就说明走的是 Gemini 工具通道。
	// Messages 必须深拷贝：浅拷贝会改坏原 req，换号重试时工具历史会被污染。
	if err != nil && len(req.Tools) > 0 && CapsFor("antigravity", req.Model).RetryNoToolsOn400 && antigravity.IsBadRequest(err) {
		stripped := *req
		stripped.Tools = nil
		stripped.ToolChoice = nil
		stripped.Messages = make([]antigravity.Message, 0, len(req.Messages))
		for _, m := range req.Messages {
			cp := m
			cp.ToolCalls = nil
			if strings.EqualFold(cp.Role, "tool") {
				text := cp.Text
				if name := strings.TrimSpace(cp.ToolName); name != "" {
					text = "[" + name + "] " + text
				}
				cp.Role = "user"
				cp.Text = text
				cp.ToolCallID = ""
				cp.ToolName = ""
			}
			if strings.EqualFold(cp.Role, "assistant") && strings.TrimSpace(cp.Text) == "" && len(m.ToolCalls) > 0 && len(cp.ImageURLs) == 0 {
				continue
			}
			stripped.Messages = append(stripped.Messages, cp)
		}
		if retry, retryErr := attempt(&stripped, nil); retryErr == nil {
			return retry, nil
		}
	}
	if err != nil && antigravity.IsResourceExhausted(err) {
		e.markCooldown(acc, "上游 429 限流", 5*time.Minute)
		go e.refreshCatalog(acc)
		go e.refreshQuotaSummary(acc)
	}
	return res, err
}

// chatLoop 轮询账号直到成功或池耗尽。
func (e *AntigravityEngine) chatLoop(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	if !e.Ready() {
		return nil, engineUnavailableError(e)
	}
	if sink == nil {
		sink = model.TextSink(nil)
	}
	// 先取 projectID 占位（chatOnce 内 token() 会重取）
	cReq := toAntigravityRequest(req, "")
	var lastErr error
	for attempt := 0; attempt < e.usableCount(); attempt++ {
		acc := e.pick()
		if acc == nil {
			break
		}
		res, err := e.chatOnce(ctx, acc, cReq, sink.Text, func(delta antigravity.ToolCallDelta) error {
			extra := json.RawMessage(nil)
			if strings.TrimSpace(delta.ThoughtSig) != "" {
				b, mErr := json.Marshal(map[string]any{
					"google": map[string]string{"thought_signature": delta.ThoughtSig},
				})
				if mErr == nil {
					extra = b
				}
			}
			return sink.ToolCall(model.ToolCallDelta{
				Index: delta.Index,
				ID:    delta.ID,
				Type:  "function",
				Function: model.ToolFunctionDelta{
					Name:      delta.Name,
					Arguments: delta.Arguments,
				},
				ExtraContent: extra,
			})
		})
		if err == nil {
			result := &model.ChatResult{
				Text:   res.Text,
				Engine: e.name,
				Model:  req.Model,
				Usage:  subResultUsage(res.PromptTokens, res.CompletionTokens+res.ReasoningTokens, res.ReasoningTokens),
			}
			for _, call := range res.ToolCalls {
				tc := model.ToolCall{
					ID:   call.ID,
					Type: "function",
					Function: model.ToolFunction{
						Name:      call.Name,
						Arguments: call.Arguments,
					},
				}
				if strings.TrimSpace(call.ThoughtSig) != "" {
					if b, mErr := json.Marshal(map[string]any{
						"google": map[string]string{"thought_signature": call.ThoughtSig},
					}); mErr == nil {
						tc.ExtraContent = b
						model.RememberToolCallExtra(tc.ID, tc.ExtraContent)
					}
				}
				result.ToolCalls = append(result.ToolCalls, tc)
			}
			if len(result.ToolCalls) > 0 {
				result.FinishReason = "tool_calls"
			}
			return result, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if antigravity.IsUnauthorized(err) || strings.Contains(err.Error(), "刷新失败") || strings.Contains(err.Error(), "project_id") {
			e.markCooldown(acc, "凭据/项目异常", 10*time.Minute)
			continue
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("antigravity: %w", lastErr)
	}
	return nil, errors.New("antigravity 没有可用账号")
}

func (e *AntigravityEngine) usableCount() int {
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

// Chat 非流式对话。
func (e *AntigravityEngine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, nil)
}

// ChatStream 流式对话。
func (e *AntigravityEngine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, model.TextSink(onDelta))
}

// ChatStreamSink 流式对话（支持工具调用增量）。
func (e *AntigravityEngine) ChatStreamSink(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	return e.chatLoop(ctx, req, sink)
}

func (e *AntigravityEngine) setErr(err error) {
	e.errMu.Lock()
	e.lastErr = err
	e.errMu.Unlock()
}

// Status 引擎运行状态。
func (e *AntigravityEngine) Status() map[string]any {
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

// Passthrough antigravity 不支持多模态端点透传。
func (e *AntigravityEngine) Passthrough() bool { return false }

func (e *AntigravityEngine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("antigravity 引擎不支持透传")
}

// Close 关闭引擎。
func (e *AntigravityEngine) Close() {}
