// routing_order.go 管理台可配置的账号路由顺序。
//
// 顺序是一个全局账号 ID 列表：供应商之间、供应商内部账号之间都由这个列表
// 的先后决定。请求先走列表里最靠前、且声明了该模型的账号；该账号失败后
// 换下一个，同供应商账号都失败才轮到下一个供应商。没有保存过顺序时保持
// 各引擎原有的轮询 / 智能选号行为。
package provider

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"web2api/internal/config"
	"web2api/internal/engine"
	"web2api/internal/model"
)

// ApplyRoutingOrder 保存路由顺序并立即下发到所有支持排序的引擎。
func (m *Manager) ApplyRoutingOrder(ids []string) error {
	if m.st != nil {
		if err := m.st.SetRoutingAccountOrder(ids); err != nil {
			return err
		}
	}
	m.applyRoutingOrder(ids)
	return nil
}

// ReloadRoutingOrder 从存储重新加载顺序（启动装配、账号增删后调用）。
func (m *Manager) ReloadRoutingOrder() {
	if m.st == nil {
		return
	}
	ids, err := m.st.RoutingAccountOrder()
	if err != nil {
		return
	}
	m.applyRoutingOrder(ids)
}

// RoutingOrder 返回当前生效果的账号顺序（只读快照）。
func (m *Manager) RoutingOrder() []string {
	m.routeMu.RLock()
	defer m.routeMu.RUnlock()
	out := make([]string, len(m.routeIDs))
	copy(out, m.routeIDs)
	return out
}

// applyRoutingOrder 把顺序下发给引擎并缓存供应商优先级。
// 列表里没出现的新账号按创建顺序追加到末尾，避免新增账号抢占既有顺序。
func (m *Manager) applyRoutingOrder(ids []string) {
	clean := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}

	// 供应商优先级：取该供应商在顺序里最靠前的账号位置；没有账号的供应商
	// 按引擎注册顺序排在已配置的供应商之后。
	engineOf := map[string]string{}
	if m.st != nil && len(clean) > 0 {
		if accounts, err := m.st.ListAccounts(); err == nil {
			missing := make([]string, 0, len(accounts))
			for _, acc := range accounts {
				id := strconv.FormatInt(acc.ID, 10)
				engineOf[id] = config.NormalizeEngineName(acc.Engine)
				if _, ok := seen[id]; !ok {
					missing = append(missing, id)
				}
			}
			// 新账号按创建顺序（自增 ID）追加到末尾。
			sort.Slice(missing, func(i, j int) bool {
				li, _ := strconv.ParseInt(missing[i], 10, 64)
				lj, _ := strconv.ParseInt(missing[j], 10, 64)
				return li < lj
			})
			for _, id := range missing {
				seen[id] = struct{}{}
				clean = append(clean, id)
			}
		}
	}

	rank := map[string]int{}
	for i, id := range clean {
		eng, ok := engineOf[id]
		if !ok {
			continue
		}
		if r, exists := rank[eng]; !exists || i < r {
			rank[eng] = i
		}
	}

	for _, name := range m.engineOrder {
		if orderer, ok := m.engines[name].(engine.AccountOrderer); ok {
			orderer.SetAccountOrder(clean)
		}
	}

	m.routeMu.Lock()
	m.routeIDs = clean
	m.routeSupplier = rank
	m.routeMu.Unlock()
}

// routingConfigured 管理台是否保存过账号顺序。
func (m *Manager) routingConfigured() bool {
	m.routeMu.RLock()
	defer m.routeMu.RUnlock()
	return len(m.routeIDs) > 0
}

// supplierRank 供应商在顺序里的优先级；未参与配置的供应商排在后面，
// 彼此按引擎注册顺序。
func (m *Manager) supplierRank(name string) int {
	m.routeMu.RLock()
	rank, ok := m.routeSupplier[name]
	m.routeMu.RUnlock()
	if ok {
		return rank
	}
	for i, engName := range m.engineOrder {
		if engName == name {
			return 1<<30 + i
		}
	}
	return 1 << 30
}

// engineCanServeModelNow 引擎当前是否有可用账号能接这个模型。
// 有逐模型可用性的引擎（custom）直接问引擎；其余按目录里的 Available 判断，
// 避免把「目录里有、账号都不可用」的引擎排到前面后白白失败一轮。
func engineCanServeModelNow(eng engine.Engine, modelName string) bool {
	if eng == nil {
		return false
	}
	if checker, ok := eng.(engine.ModelAvailability); ok {
		return checker.HasAvailableModel(modelName)
	}
	name := model.CanonicalModelID(modelName)
	if name == "" {
		return false
	}
	for _, mi := range eng.ListModels() {
		if model.CanonicalModelID(mi.ID) == name {
			return mi.Available
		}
	}
	return false
}

// orderedServeEngines 返回当前可承接该模型的引擎，按配置的供应商顺序排列。
func (m *Manager) orderedServeEngines(modelName string) []engine.Engine {
	var list []engine.Engine
	for _, engName := range m.engineOrder {
		eng, ok := m.engines[engName]
		if !ok || eng == nil || !eng.Ready() {
			continue
		}
		if engineCanServeModelNow(eng, modelName) {
			list = append(list, eng)
		}
	}
	if len(list) < 2 {
		return list
	}
	sort.SliceStable(list, func(i, j int) bool {
		return m.supplierRank(list[i].Name()) < m.supplierRank(list[j].Name())
	})
	return list
}

// explicitChatEngine 配置里点名要走的引擎（routing.engine_models 等）。
// 点名模型不参与跨供应商兜底：配置写的是「就该走它」，失败时返回它自己的错误。
func (m *Manager) explicitChatEngine(name string) (engine.Engine, bool) {
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
	return nil, false
}

// chatEnginePlan 返回本次对话按顺序尝试的引擎列表。
// 绝大多数模型可以跨供应商兜底；配置点名与多模态模型仍保持单一归属。
func (m *Manager) chatEnginePlan(ctx context.Context, req model.ChatRequest) ([]engine.Engine, model.ChatRequest, error) {
	name := model.CanonicalModelID(req.Model)
	probe := isProbeRequest(ctx)
	// 视频等无法经 chat/completions 产出结果的模型走原来的用途闸门。
	if kind := m.kindOf(req.Model); model.ChatIncompatibleKind(kind) {
		if mediaEng, ok := m.chatMediaEngine(); ok {
			if !mediaEng.Ready() {
				return nil, req, engine.UnavailableError(mediaEng)
			}
			return []engine.Engine{mediaEng}, req, nil
		}
		return nil, req, engine.NewUnsupportedEndpoint(req.Model, kind)
	}
	if !probe && m.modelDisabled(name) {
		return nil, req, engine.NewModelDisabled(req.Model)
	}
	if probe {
		eng, r, err := m.resolveChatEngineCtx(ctx, req)
		if err != nil {
			return nil, r, err
		}
		return []engine.Engine{eng}, r, nil
	}

	// 显式配置 / 多模态：单一归属，不做跨供应商重试。
	if eng, ok := m.explicitChatEngine(name); ok {
		if !eng.Ready() {
			return nil, req, engine.UnavailableError(eng)
		}
		return []engine.Engine{eng}, req, nil
	}
	if isMultimodalModel(name) {
		// 目录里有承接方（如第三方 Key 的 gpt-image-1）就保持单一归属；
		// 目录里没有时才回落到引擎B 的多模态通道。
		if list := m.orderedServeEngines(name); len(list) > 0 {
			return list[:1], req, nil
		}
		if eng, ok := m.engines["aistudio"]; ok {
			if !eng.Ready() {
				return nil, req, engine.UnavailableError(eng)
			}
			return []engine.Engine{eng}, req, nil
		}
		return nil, req, engine.NewModelNotFound(req.Model, m.ownershipHint(req.Model))
	}

	list := m.orderedServeEngines(name)
	if len(list) == 0 {
		// 没有就绪的承接方：交给原逻辑返回最准确的错误（声明它的引擎为何不可用）。
		eng, r, err := m.resolveChatEngineCtx(ctx, req)
		if err != nil {
			return nil, r, err
		}
		return []engine.Engine{eng}, r, nil
	}
	// 未配置顺序时保持原有首选引擎，仅把它排到最前；配置了顺序则完全按顺序。
	if !m.routingConfigured() {
		if primary, _ := m.pickEngine(name); primary != nil {
			list = moveEngineFirst(list, primary)
		}
	}
	return list, req, nil
}

func moveEngineFirst(list []engine.Engine, target engine.Engine) []engine.Engine {
	for i, eng := range list {
		if eng == target {
			copy(list[1:i+1], list[:i])
			list[0] = target
			return list
		}
	}
	return list
}

// shouldFailover 失败是否值得换下一个供应商。业务语义错误（模型不存在、
// 已下架、端点不匹配）换一家也一样，直接返回；其余按「这家不通换下家」处理。
func shouldFailover(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	switch {
	case engine.IsModelNotFound(err), engine.IsModelDisabled(err),
		engine.IsUnsupportedEndpoint(err), engine.IsToolsNotSupported(err):
		return false
	}
	// 参数/格式类 4xx 是「这次请求」的问题，换供应商也一样。
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "http 400") || strings.Contains(msg, "http 422") {
		return false
	}
	return true
}

// streamGate 记录本次流式请求是否已经向客户端写过增量。
// 已经写出内容后不能再换供应商，否则客户端会收到两份拼接的回复。
type streamGate struct {
	inner model.StreamSink
	wrote atomic.Bool
}

func (g *streamGate) Text(delta string) error {
	g.wrote.Store(true)
	return g.inner.Text(delta)
}

func (g *streamGate) ToolCall(delta model.ToolCallDelta) error {
	g.wrote.Store(true)
	return g.inner.ToolCall(delta)
}

func (g *streamGate) Wrote() bool { return g.wrote.Load() }

// 保证 streamGate 满足 model.StreamSink。
var _ model.StreamSink = (*streamGate)(nil)
