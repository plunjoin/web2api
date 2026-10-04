package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	aistudio "web2api/internal/aistudio2api/aistudio"
	camoufox "web2api/internal/aistudio2api/camoufoxnative"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/model"
)

// NativeAIStudioEngine 引擎B：内置纯 Go 逆向（AI Studio 私有协议 + WAA go 后端）。
// 组装链：AccountStore → AccountPool → 固定出口头 → MakerSuiteHTTPTransport
//
//	→ 每账号懒启动 WAA Go Worker → WorkerProtectedTransport → Client → PooledService
//
// 协议实现源自 Mag1cFall/AIStudio2API（MIT，见 internal/aistudio2api/LICENSE）。
type NativeAIStudioEngine struct {
	name string
	cfg  config.EngineBConfig

	store     *aistudio.AccountStore
	rootDir   string // 新账号创建根目录（auth_states 首路径）
	accounts  []*aistudio.Account
	pool      *aistudio.AccountPool
	headers   *nativeHeaderProvider
	transport *aistudio.MakerSuiteHTTPTransport
	workers   *nativeWorkerManager
	protected *aistudio.WorkerProtectedTransport
	client    *aistudio.Client
	service   *aistudio.PooledService

	idMu  sync.RWMutex
	idMap map[string]string // 管理层 ID → aistudio 账号 ID（邮箱）

	mu       sync.RWMutex
	models   []aistudio.Model
	ready    bool
	lastErr  error
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewNativeAIStudio 构建原生引擎B（不发起网络请求，Init 时才装配）。
func NewNativeAIStudio(cfg config.EngineBConfig) (*NativeAIStudioEngine, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if strings.TrimSpace(cfg.AuthStates) == "" {
		cfg.AuthStates = "auth"
	}
	if cfg.PerAccountConcurrency <= 0 {
		cfg.PerAccountConcurrency = 2
	}
	if cfg.InitTimeoutSeconds <= 0 {
		cfg.InitTimeoutSeconds = 120
	}
	if cfg.RequestTimeoutSeconds <= 0 {
		cfg.RequestTimeoutSeconds = 300
	}
	if cfg.RefreshSeconds <= 0 {
		cfg.RefreshSeconds = 300
	}
	if cfg.UpstreamChannels == "" {
		cfg.UpstreamChannels = "playground,build"
	}
	if cfg.RoutingStrategy == "" {
		cfg.RoutingStrategy = "round-robin"
	}
	return &NativeAIStudioEngine{name: "b", cfg: cfg, stopCh: make(chan struct{}),
		idMap: map[string]string{}}, nil
}

// Init 装配运行时并拉取模型目录。
// 零账号时同样完成装配（空池），保证管理台加号可即时入池。
func (e *NativeAIStudioEngine) Init(ctx context.Context) error {
	paths := splitAndTrim(e.cfg.AuthStates, ",")
	e.store = aistudio.NewAccountStore(paths...)
	if len(paths) > 0 {
		e.rootDir = paths[0]
	}
	accounts, err := e.store.Load()
	if err != nil {
		e.setErr(fmt.Errorf("载入 AI Studio 账号失败: %w", err))
		return e.lastErr
	}
	emptyAccounts := len(accounts) == 0
	if emptyAccounts {
		// 空目录不算致命：记录提示，继续构建空池
		e.setErr(errors.New("AI Studio 账号目录为空（auth_states 未找到 storage-state.json，可在管理台添加）"))
		accounts = []*aistudio.Account{}
	}
	e.accounts = accounts

	e.pool = aistudio.NewAccountPool(accounts, e.cfg.PerAccountConcurrency)
	e.pool.SetRoutingStrategy(e.cfg.RoutingStrategy)
	e.pool.SetUpstreamChannels(parseChannels(e.cfg.UpstreamChannels))

	e.headers, err = newNativeHeaderProvider(accounts, e.cfg.Proxy)
	if err != nil {
		e.setErr(err)
		return err
	}
	e.transport, err = aistudio.NewMakerSuiteHTTPTransport(aistudio.HTTPTransportOptions{
		Pool: e.pool, Signer: aistudio.NewSigner(), Headers: e.headers, GlobalProxy: e.cfg.Proxy,
	})
	if err != nil {
		e.headers.Close()
		e.setErr(err)
		return err
	}
	e.workers = newNativeWorkerManager(e.pool, e.cfg)
	e.protected, err = aistudio.NewWorkerProtectedTransport(aistudio.WorkerProtectedTransportOptions{
		Transport: e.transport, Workers: e.workers, SetupTimeout: time.Duration(e.cfg.InitTimeoutSeconds) * time.Second,
	})
	if err != nil {
		e.headers.Close()
		e.transport.CloseIdleConnections()
		e.setErr(err)
		return err
	}
	requestContext, err := aistudio.NewPoolRequestContextProvider(e.pool)
	if err != nil {
		e.headers.Close()
		e.transport.CloseIdleConnections()
		e.setErr(err)
		return err
	}
	e.client, err = aistudio.NewClient(aistudio.ClientOptions{
		Transport:       e.transport,
		Protected:       e.protected,
		ContextProvider: requestContext,
	})
	if err != nil {
		e.headers.Close()
		e.transport.CloseIdleConnections()
		e.setErr(err)
		return err
	}
	e.service, err = aistudio.NewPooledService(e.pool, e.client)
	if err != nil {
		e.headers.Close()
		e.transport.CloseIdleConnections()
		e.setErr(err)
		return err
	}

	// 拉取模型目录（失败不阻断启动，后续定时重试）
	e.refreshModels(ctx)

	// 后台模型目录刷新
	go e.modelRefreshLoop()

	e.mu.Lock()
	e.ready = true
	if emptyAccounts {
		e.lastErr = errors.New("AI Studio 账号目录为空（请在管理台添加账号，或检查 auth_states 配置）")
	}
	e.mu.Unlock()
	return nil
}

// refreshModels 拉取并缓存模型目录。
func (e *NativeAIStudioEngine) refreshModels(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, time.Duration(e.cfg.InitTimeoutSeconds)*time.Second)
	defer cancel()
	models, err := e.service.Models(cctx)
	e.mu.Lock()
	if err != nil {
		e.lastErr = fmt.Errorf("拉取模型目录失败: %w", err)
	} else {
		e.models = models
		e.lastErr = nil
	}
	e.mu.Unlock()
}

// modelRefreshLoop 后台定期刷新模型目录。
func (e *NativeAIStudioEngine) modelRefreshLoop() {
	interval := time.Duration(e.cfg.RefreshSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.refreshModels(context.Background())
		}
	}
}

// Close 释放运行时资源。
func (e *NativeAIStudioEngine) Close() {
	e.stopOnce.Do(func() { close(e.stopCh) })
	if e.workers != nil {
		e.workers.Close()
	}
	if e.transport != nil {
		e.transport.CloseIdleConnections()
	}
	if e.headers != nil {
		e.headers.Close()
	}
}

func (e *NativeAIStudioEngine) setErr(err error) {
	e.mu.Lock()
	e.lastErr = err
	e.mu.Unlock()
}

// Name 引擎标识。
func (e *NativeAIStudioEngine) Name() string { return e.name }

// Describe 引擎说明。
func (e *NativeAIStudioEngine) Describe() string {
	return "aistudio.google.com（内置纯 Go 逆向：WAA go 后端 + 私有协议，零浏览器依赖）"
}

// Ready 引擎可用。
func (e *NativeAIStudioEngine) Ready() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ready && len(e.accounts) > 0
}

// ListModels 模型列表。
func (e *NativeAIStudioEngine) ListModels() []model.ModelInfo {
	e.mu.RLock()
	models := e.models
	e.mu.RUnlock()
	out := make([]model.ModelInfo, 0, len(models))
	for _, m := range models {
		out = append(out, model.ModelInfo{
			ID:          m.ID,
			Object:      "model",
			DisplayName: m.Name,
			Available:   true,
			Engine:      "b",
		})
	}
	return out
}

// buildContents 将 OpenAI messages 转为 AI Studio contents + system。
func buildContents(messages []model.ChatMessage) (string, []aistudio.Content) {
	var system []string
	contents := make([]aistudio.Content, 0, len(messages))
	for _, m := range messages {
		text := m.Text()
		switch m.Role {
		case "system":
			system = append(system, text)
		case "assistant":
			contents = append(contents, aistudio.Content{Role: aistudio.RoleAssistant, Parts: []aistudio.Part{{Text: text}}})
		default:
			contents = append(contents, aistudio.Content{Role: aistudio.RoleUser, Parts: []aistudio.Part{{Text: text}}})
		}
	}
	return strings.Join(system, "\n"), contents
}

// buildGenerateRequest 组装协议请求。
func (e *NativeAIStudioEngine) buildGenerateRequest(req model.ChatRequest) aistudio.GenerateRequest {
	system, contents := buildContents(req.Messages)
	gen := aistudio.GenerateRequest{
		ID:       "web2api-" + model.NewID(8),
		Model:    req.Model,
		System:   system,
		Contents: contents,
	}
	if req.Temperature != nil {
		gen.Config.Temperature = req.Temperature
	}
	if req.TopP != nil {
		gen.Config.TopP = req.TopP
	}
	if req.MaxTokens != nil {
		value := int64(*req.MaxTokens)
		gen.Config.MaxOutputTokens = &value
	}
	return gen
}

// generate 执行一次生成并消费事件流。
func (e *NativeAIStudioEngine) generate(ctx context.Context, req model.ChatRequest, onDelta func(string) error) (*model.ChatResult, error) {
	if e.service == nil {
		return nil, errors.New("引擎B 未初始化")
	}
	genReq := e.buildGenerateRequest(req)
	timeout := time.Duration(e.cfg.RequestTimeoutSeconds) * time.Second
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 必须通过 PooledService 生成请求：它负责从账号池取得租约、填充
	// GenerateRequest.AccountID，并把同一租约传给协议上下文和传输层。
	// 直接调用 Client.Generate 会遗漏账户 ID，最终报“请求上下文缺少账户 ID”。
	events, err := e.service.Generate(cctx, genReq)
	if err != nil {
		e.setErr(err)
		return nil, err
	}
	var full strings.Builder
	finished := false
	for event := range events {
		if event.Err != nil {
			e.setErr(event.Err)
			return nil, event.Err
		}
		switch event.Kind {
		case aistudio.EventText:
			full.WriteString(event.Text)
			if onDelta != nil {
				if err := onDelta(event.Text); err != nil {
					e.setErr(err)
					return nil, err
				}
			}
		case aistudio.EventMedia:
			// 图片/音媒体：以 Markdown 形式附加。AI Studio 原生协议通常
			// 返回 inline data，没有 URL；必须转成 data URI 才能通过
			// OpenAI 的纯文本 content 传回调用方。
			if event.Media != nil {
				mediaURL := strings.TrimSpace(event.Media.URL)
				if mediaURL == "" && len(event.Media.Data) > 0 {
					mime := strings.TrimSpace(event.Media.MIME)
					if mime == "" {
						mime = "application/octet-stream"
					}
					mediaURL = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(event.Media.Data)
				}
				if mediaURL == "" {
					break
				}
				segment := "\n![media](" + mediaURL + ")\n"
				full.WriteString(segment)
				if onDelta != nil {
					if err := onDelta(segment); err != nil {
						e.setErr(err)
						return nil, err
					}
				}
			}
		case aistudio.EventFinish:
			finished = true
		case aistudio.EventError:
			if event.Err != nil {
				e.setErr(event.Err)
				return nil, event.Err
			}
			err := errors.New("AI Studio 返回未知错误事件")
			e.setErr(err)
			return nil, err
		}
	}
	_ = finished
	return &model.ChatResult{Text: full.String(), Engine: "b", Model: req.Model}, nil
}

// Chat 非流式对话。
func (e *NativeAIStudioEngine) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	return e.generate(ctx, req, nil)
}

// ChatStream 流式对话。
func (e *NativeAIStudioEngine) ChatStream(ctx context.Context, req model.ChatRequest, onDelta model.ChatStreamFunc) (*model.ChatResult, error) {
	return e.generate(ctx, req, onDelta)
}

// GenerateVideo 创建 Veo 长任务。视频协议与聊天 GenerateContent 是两套
// 独立的 AI Studio RPC，必须通过 PooledService 取得账号租约后调用。
func (e *NativeAIStudioEngine) GenerateVideo(ctx context.Context, request aistudio.VideoRequest) (aistudio.VideoOperation, error) {
	if e.service == nil {
		return aistudio.VideoOperation{}, errors.New("引擎B 未初始化")
	}
	return e.service.GenerateVideo(ctx, request)
}

// GetGenerateVideoOperation 查询 Veo 长任务状态。
func (e *NativeAIStudioEngine) GetGenerateVideoOperation(ctx context.Context, operationID string) (aistudio.VideoOperation, error) {
	if e.service == nil {
		return aistudio.VideoOperation{}, errors.New("引擎B 未初始化")
	}
	return e.service.GetGenerateVideoOperation(ctx, operationID)
}

// DownloadVideoFile 下载已完成 Veo 任务绑定的媒体文件。
func (e *NativeAIStudioEngine) DownloadVideoFile(ctx context.Context, fileID string) (aistudio.MediaStream, error) {
	if e.service == nil {
		return aistudio.MediaStream{}, errors.New("引擎B 未初始化")
	}
	return e.service.DownloadFile(ctx, fileID)
}

// Passthrough 原生模式不透传（多模态能力已内置于对话协议）。
func (e *NativeAIStudioEngine) Passthrough() bool { return false }

// ServePassthrough 原生模式不支持透传。
func (e *NativeAIStudioEngine) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w engine.PassthroughWriter) error {
	return errors.New("引擎B 原生模式不使用透传：图片模型走 /v1/chat/completions（模型见 /v1/models）")
}

// Status 引擎与账号状态。
func (e *NativeAIStudioEngine) Status() map[string]any {
	e.mu.RLock()
	models := len(e.models)
	errMsg := ""
	if e.lastErr != nil {
		errMsg = e.lastErr.Error()
	}
	e.mu.RUnlock()
	accounts := make([]map[string]any, 0, len(e.accounts))
	for _, acc := range e.accounts {
		accounts = append(accounts, map[string]any{
			"id":     acc.ID,
			"label":  acc.Config.Label,
			"state":  string(acc.State),
			"tier":   strconv.FormatInt(int64(acc.BenefitTier), 10),
			"models": len(acc.Models),
		})
	}
	workerReady := 0
	if e.workers != nil {
		workerReady = e.workers.ReadyCount()
	}
	return map[string]any{
		"engine":       "b",
		"mode":         "native",
		"ready":        e.Ready(),
		"auth_states":  e.cfg.AuthStates,
		"accounts":     accounts,
		"worker_ready": workerReady,
		"models":       models,
		"routing":      e.cfg.RoutingStrategy,
		"channels":     e.cfg.UpstreamChannels,
		"error":        errMsg,
	}
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ==================== 动态号池管理 ====================

// RegisterAccountID 登记管理 ID ↔ aistudio 账号 ID（邮箱）的映射（启动装配已有账号时用）。
func (e *NativeAIStudioEngine) RegisterAccountID(managedID, aistudioID string) {
	e.idMu.Lock()
	e.idMap[managedID] = aistudioID
	e.idMu.Unlock()
}

// PoolAccountIDs 池内账号 ID（邮箱）列表。
func (e *NativeAIStudioEngine) PoolAccountIDs() []string {
	statuses := e.pool.Status()
	ids := make([]string, 0, len(statuses))
	for _, s := range statuses {
		ids = append(ids, s.ID)
	}
	return ids
}

// AttachOrAdd 池中已有该邮箱则仅登记映射，否则用给定凭据创建并入池。
func (e *NativeAIStudioEngine) AttachOrAdd(id, email, storageJSON, locale, timezone, proxy string) error {
	for _, poolID := range e.PoolAccountIDs() {
		if strings.EqualFold(poolID, email) {
			e.RegisterAccountID(id, poolID)
			return nil
		}
	}
	return e.AddAccount(id, email, storageJSON, locale, timezone, proxy)
}

// RefreshAccountModels 刷新指定账号的模型目录（健康检查用）。
func (e *NativeAIStudioEngine) RefreshAccountModels(ctx context.Context, accountID string) ([]aistudio.Model, error) {
	if e.service == nil {
		return nil, errors.New("引擎B 未初始化")
	}
	models, err := e.service.RefreshAccountModels(ctx, accountID)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.models = models
	e.mu.Unlock()
	return models, nil
}

// AddAccount 添加 AI Studio 账号（凭据为 storage-state JSON 文本）。
func (e *NativeAIStudioEngine) AddAccount(id, email, storageJSON, locale, timezone, proxy string) error {
	if e.rootDir == "" {
		return errors.New("engine_b.auth_states 未配置")
	}
	var state aistudio.StorageState
	if err := json.Unmarshal([]byte(storageJSON), &state); err != nil {
		return fmt.Errorf("storage-state 不是合法 JSON: %w", err)
	}
	if err := state.Validate(); err != nil {
		return fmt.Errorf("storage-state 无效: %w", err)
	}
	cfg := aistudio.DefaultAccountConfig(email)
	if locale != "" {
		cfg.Locale = locale
	}
	if timezone != "" {
		cfg.Timezone = timezone
	}
	if proxy != "" {
		cfg.Proxy = proxy
	}
	createStore := aistudio.NewAccountStore(e.rootDir)
	acc, lease, err := createStore.Create(cfg, state)
	if err != nil {
		return fmt.Errorf("创建账号目录失败: %w", err)
	}
	if err := lease.Release(); err != nil {
		_ = createStore.Delete(acc)
		return fmt.Errorf("发布账号失败: %w", err)
	}
	if err := e.pool.Add(acc); err != nil {
		_ = createStore.Delete(acc)
		return fmt.Errorf("账号入池失败: %w", err)
	}
	if err := e.headers.Add(acc); err != nil {
		_, _ = e.pool.Remove(acc.ID, func(*aistudio.Account) error { return nil })
		_ = createStore.Delete(acc)
		return err
	}
	e.idMu.Lock()
	e.idMap[id] = acc.ID
	e.idMu.Unlock()
	return nil
}

// removeWithRetry 从池中移除账号；遇 ErrAccountLeased（活跃请求/后台 auth 刷新占用）时带重试。
func (e *NativeAIStudioEngine) removeWithRetry(accountID string, wait time.Duration) (*aistudio.Account, error) {
	deadline := time.Now().Add(wait)
	for {
		acc, err := e.pool.Remove(accountID, func(*aistudio.Account) error { return nil })
		if err == nil {
			return acc, nil
		}
		if !errors.Is(err, aistudio.ErrAccountLeased) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("账号正被后台刷新或活跃请求占用（已等待 %.0f 秒）: %w", wait.Seconds(), err)
		}
		time.Sleep(time.Second)
	}
}

// RemoveAccount 删除账号（移出池 + 删除目录与凭据文件）。
// 池原生语义下禁用账号仍在池中，可直接 Remove；占用中则最多等 30 秒重试。
func (e *NativeAIStudioEngine) RemoveAccount(id string) error {
	e.idMu.Lock()
	accountID, ok := e.idMap[id]
	if ok {
		delete(e.idMap, id)
	}
	e.idMu.Unlock()
	if !ok {
		return fmt.Errorf("账号 %s 不在管理范围", id)
	}

	acc, removeErr := e.removeWithRetry(accountID, 30*time.Second)
	if removeErr != nil && !errors.Is(removeErr, aistudio.ErrAccountNotFound) {
		// 出池失败（被占用）：回滚映射，交还上层提示重试
		e.idMu.Lock()
		e.idMap[id] = accountID
		e.idMu.Unlock()
		return removeErr
	}
	_ = e.headers.Remove(accountID)
	e.workers.CloseAccount(accountID)
	if acc != nil {
		if err := e.store.Delete(acc); err != nil {
			return fmt.Errorf("删除账号目录失败: %w", err)
		}
		return nil
	}
	// 账号不在池中（历史遗留）：按目录约定 root/<邮箱> 直接清理磁盘
	if e.rootDir == "" {
		return fmt.Errorf("账号 %s 不在池中且未配置 auth_states，无法清理目录", accountID)
	}
	if err := os.RemoveAll(filepath.Join(e.rootDir, accountID)); err != nil {
		return fmt.Errorf("清理账号目录失败: %w", err)
	}
	return nil
}

// SetEnabled 启停账号：池原生管理通道（AcquireAccount 按 ID 取租约、不受 ready 状态限制，
// 租约内 SaveConfig 持久化 account.json 并迁移状态；账号留在池中，调度自动跳过）。
// 占用中最多等 3 秒，超时报"稍后重试"。
func (e *NativeAIStudioEngine) SetEnabled(id string, enabled bool) error {
	e.idMu.RLock()
	accountID, ok := e.idMap[id]
	e.idMu.RUnlock()
	if !ok {
		return fmt.Errorf("账号 %s 不在管理范围", id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	lease, err := e.pool.AcquireAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("账号正被活跃请求或后台任务占用，请稍后重试")
		}
		if errors.Is(err, aistudio.ErrAccountNotFound) {
			return fmt.Errorf("账号不在池中（可能装配失败，删除后重新添加）: %w", err)
		}
		return err
	}
	defer func() { _ = lease.Release() }()
	newCfg := lease.Account().Config
	newCfg.Enabled = enabled
	if err := lease.SaveConfig(newCfg); err != nil {
		return fmt.Errorf("更新账号配置失败: %w", err)
	}
	if !enabled {
		e.workers.CloseAccount(accountID)
	}
	return nil
}

// AccountStates 池内账号实时状态（禁用账号保留在池中，状态由池回报）。
func (e *NativeAIStudioEngine) AccountStates() []AccountState {
	e.idMu.RLock()
	idByAistudio := make(map[string]string, len(e.idMap))
	for managed, aistudioID := range e.idMap {
		idByAistudio[aistudioID] = managed
	}
	e.idMu.RUnlock()

	var out []AccountState
	for _, status := range e.pool.Status() {
		state := AccountState{
			ID:      status.ID,
			Label:   status.Label,
			Enabled: status.Enabled,
			Models:  len(status.Models),
		}
		switch {
		case !status.Enabled || status.State == aistudio.AccountDisabled:
			state.Status = "disabled"
		case status.State == aistudio.AccountReady:
			state.Status = "ok"
			state.Ready = true
		case status.State == aistudio.AccountAuthRequired:
			state.Status = "error"
			state.Detail = "登录态失效"
		case status.State == aistudio.AccountBusy:
			state.Status = "busy"
			state.Detail = "有进行中的请求或后台任务"
		case status.State == aistudio.AccountCooldown:
			state.Status = "cooldown"
			state.Detail = status.Message
		default:
			state.Status = "error"
			state.Detail = status.Message
		}
		if managed, ok := idByAistudio[status.ID]; ok {
			state.ID = managed
		}
		out = append(out, state)
	}
	return out
}

func parseChannels(s string) []aistudio.Channel {
	var out []aistudio.Channel
	for _, name := range splitAndTrim(s, ",") {
		switch strings.ToLower(name) {
		case "playground":
			out = append(out, aistudio.ChannelPlayground)
		case "build":
			out = append(out, aistudio.ChannelBuild)
		}
	}
	if len(out) == 0 {
		out = []aistudio.Channel{aistudio.ChannelPlayground, aistudio.ChannelBuild}
	}
	return out
}

// ==================== 固定出口与公共协议头 ====================

type nativeHeaderState struct {
	mu      sync.Mutex
	client  *http.Client
	headers http.Header
}

// nativeHeaderProvider 每账号固定出口 HTTP 客户端 + 协议公共头缓存。
type nativeHeaderProvider struct {
	mu          sync.RWMutex
	globalProxy string
	states      map[string]*nativeHeaderState
}

func newNativeHeaderProvider(accounts []*aistudio.Account, globalProxy string) (*nativeHeaderProvider, error) {
	provider := &nativeHeaderProvider{
		globalProxy: strings.TrimSpace(globalProxy),
		states:      make(map[string]*nativeHeaderState, len(accounts)),
	}
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if err := provider.Add(account); err != nil {
			provider.Close()
			return nil, err
		}
	}
	return provider, nil
}

// Add 注册账号固定出口。
func (p *nativeHeaderProvider) Add(account *aistudio.Account) error {
	client, err := aistudio.NewProxyHTTPClient(account.EffectiveProxy(p.globalProxy))
	if err != nil {
		return fmt.Errorf("创建账号 %s 的固定出口: %w", account.ID, err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.states[account.ID]; exists {
		client.CloseIdleConnections()
		return fmt.Errorf("账号固定出口已存在: %s", account.ID)
	}
	p.states[account.ID] = &nativeHeaderState{client: client}
	return nil
}

// ProtocolHeaders 返回账号的公共协议头（首次请求时发现并缓存）。
func (p *nativeHeaderProvider) ProtocolHeaders(ctx context.Context, accountID string) (http.Header, error) {
	p.mu.RLock()
	state := p.states[accountID]
	p.mu.RUnlock()
	if state == nil {
		return nil, fmt.Errorf("账号不存在: %s", accountID)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.headers) == 0 {
		headers, err := aistudio.DiscoverPublicHeaders(ctx, state.client)
		if err != nil {
			return nil, err
		}
		state.headers = headers.Clone()
	}
	return state.headers.Clone(), nil
}

// Remove 删除账号的固定出口。
func (p *nativeHeaderProvider) Remove(accountID string) error {
	p.mu.Lock()
	state := p.states[accountID]
	if state != nil {
		delete(p.states, accountID)
	}
	p.mu.Unlock()
	if state == nil {
		return fmt.Errorf("账号固定出口不存在: %s", accountID)
	}
	state.client.CloseIdleConnections()
	return nil
}

// Close 关闭全部固定出口。
func (p *nativeHeaderProvider) Close() {
	p.mu.Lock()
	states := p.states
	p.states = nil
	p.mu.Unlock()
	for _, state := range states {
		state.client.CloseIdleConnections()
	}
}

// ==================== WAA Go Worker 池 ====================

// nativeWorkerManager 每账号懒启动纯 Go WAA Worker。
type nativeWorkerManager struct {
	pool *aistudio.AccountPool
	cfg  config.EngineBConfig

	mu      sync.Mutex
	workers map[string]*aistudio.NativeWorker
	booting map[string]struct{}
}

func newNativeWorkerManager(pool *aistudio.AccountPool, cfg config.EngineBConfig) *nativeWorkerManager {
	return &nativeWorkerManager{
		pool:    pool,
		cfg:     cfg,
		workers: map[string]*aistudio.NativeWorker{},
		booting: map[string]struct{}{},
	}
}

// Worker 实现 aistudio.ProtectedPreparerProvider：按账号懒启动（复用已就绪实例）。
func (m *nativeWorkerManager) Worker(ctx context.Context, accountID string, modelID string) (aistudio.ProtectedPreparer, error) {
	m.mu.Lock()
	if worker, ok := m.workers[accountID]; ok {
		phase := worker.State().Phase
		if phase == aistudio.WorkerReady || phase == aistudio.WorkerBusy {
			m.mu.Unlock()
			return worker, nil
		}
		// 失效实例：清理后重建
		delete(m.workers, accountID)
		_ = worker.Close()
	} else if _, booting := m.booting[accountID]; booting {
		m.mu.Unlock()
		return nil, fmt.Errorf("账号 %s 的 WAA Worker 正在启动", accountID)
	}
	m.booting[accountID] = struct{}{}
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.booting, accountID)
		m.mu.Unlock()
	}()

	account, err := m.pool.Account(accountID)
	if err != nil {
		return nil, err
	}
	proxy := strings.TrimSpace(account.Config.Proxy)
	if proxy == "" {
		proxy = strings.TrimSpace(m.cfg.Proxy)
	}
	options := camoufox.Options{
		StorageStatePath: account.StoragePath,
		Model:            modelID,
		Locale:           account.Config.Locale,
		Timezone:         account.Config.Timezone,
		Proxy:            proxy,
		TemporaryChat:    m.cfg.TemporaryChat,
	}
	worker, err := aistudio.NewGoWorker(ctx, accountID, options)
	if err != nil {
		return nil, fmt.Errorf("启动账号 %s 的 WAA Go Worker 失败: %w", accountID, err)
	}
	m.mu.Lock()
	m.workers[accountID] = worker
	m.mu.Unlock()
	return worker, nil
}

// ReadyCount 当前就绪 Worker 数。
func (m *nativeWorkerManager) ReadyCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, worker := range m.workers {
		phase := worker.State().Phase
		if phase == aistudio.WorkerReady || phase == aistudio.WorkerBusy {
			count++
		}
	}
	return count
}

// CloseAccount 关闭单个账号的 Worker（账号移除/禁用时调用）。
func (m *nativeWorkerManager) CloseAccount(accountID string) {
	m.mu.Lock()
	worker := m.workers[accountID]
	if worker != nil {
		delete(m.workers, accountID)
	}
	m.mu.Unlock()
	if worker != nil {
		_ = worker.Close()
	}
}

// Close 关闭全部 Worker。
func (m *nativeWorkerManager) Close() {
	m.mu.Lock()
	workers := m.workers
	m.workers = map[string]*aistudio.NativeWorker{}
	m.mu.Unlock()
	for _, worker := range workers {
		_ = worker.Close()
	}
}
