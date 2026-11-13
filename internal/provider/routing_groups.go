// routing_groups.go 同名模型路由组：多个供应商用同一个模型名接单时，
// 网关把它们当作一个组，按组的模式决定先试谁、失败后怎么换。
//
// 组不是人工维护的名单：目录快照里同名的供应商自动成组，和 magpie 的
// found groups（auto-<slug>）一致。只有「两个及以上供应商」才算组；
// 模式按归一化模型名持久化，默认 smart（智能）。
package provider

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"web2api/internal/engine"
	"web2api/internal/model"
)

// 路由组模式。smart 是默认值。
const (
	RouteGroupSmart  = "smart"  // 智能：按模型归属自动首选，失败后按路由顺序换下一个
	RouteGroupOrder  = "order"  // 顺序：严格按「路由顺序」里的供应商顺序
	RouteGroupRotate = "rotate" // 轮询：每个请求换一个供应商开头
	RouteGroupCheap  = "cheap"  // 最便宜：生效模型倍率低的供应商优先
)

// RouteGroupModeInfo 管理台展示用的模式说明。
type RouteGroupModeInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// RouteGroupModes 支持的路由模式（第一项为默认）。
var RouteGroupModes = []RouteGroupModeInfo{
	{ID: RouteGroupSmart, Name: "智能", Desc: "按账号可用性与模型归属自动首选，失败后自动切换下一位"},
	{ID: RouteGroupOrder, Name: "顺序", Desc: "严格按「路由顺序」里的供应商顺序接单"},
	{ID: RouteGroupRotate, Name: "轮询", Desc: "每个请求轮换一个供应商开头，分摊额度"},
	{ID: RouteGroupCheap, Name: "最便宜", Desc: "优先走生效倍率最低的供应商"},
}

func normalizeRouteGroupMode(mode string) (string, bool) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return RouteGroupSmart, true
	}
	for _, m := range RouteGroupModes {
		if m.ID == mode {
			return mode, true
		}
	}
	return "", false
}

// ReloadRoutingGroupModes 从存储重新加载路由组模式（启动装配 / 保存后调用）。
func (m *Manager) ReloadRoutingGroupModes() {
	if m.st == nil {
		return
	}
	modes, err := m.st.RoutingGroupModes()
	if err != nil {
		return
	}
	if modes == nil {
		modes = map[string]string{}
	}
	m.groupMu.Lock()
	m.groupModes = modes
	m.groupMu.Unlock()
}

// SetRoutingGroupMode 保存一个模型路由组的模式并立即生效。
func (m *Manager) SetRoutingGroupMode(modelName, mode string) error {
	key := model.CanonicalModelID(modelName)
	if key == "" {
		return fmt.Errorf("model 不能为空")
	}
	raw := mode
	mode, ok := normalizeRouteGroupMode(mode)
	if !ok {
		return fmt.Errorf("未知的路由模式 %q（可选 smart / order / rotate / cheap）", raw)
	}
	if m.st != nil {
		if err := m.st.SetRoutingGroupMode(key, mode); err != nil {
			return err
		}
	}
	m.groupMu.Lock()
	if m.groupModes == nil {
		m.groupModes = map[string]string{}
	}
	m.groupModes[key] = mode
	m.groupMu.Unlock()
	return nil
}

// RoutingGroupMode 返回模型路由组当前模式；没配置时默认 smart。
func (m *Manager) RoutingGroupMode(modelName string) string {
	key := model.CanonicalModelID(modelName)
	m.groupMu.RLock()
	mode := m.groupModes[key]
	m.groupMu.RUnlock()
	if mode == "" {
		return RouteGroupSmart
	}
	return mode
}

// applyGroupRouting 按路由组模式调整「本次可以接单的引擎」的尝试顺序。
// list 已按供应商路由顺序排好；这里只做模式要求的重排。
func (m *Manager) applyGroupRouting(modelName string, list []engine.Engine) []engine.Engine {
	if len(list) < 2 {
		return list
	}
	key := model.CanonicalModelID(modelName)
	switch m.RoutingGroupMode(key) {
	case RouteGroupOrder:
		// 已按供应商顺序排列，保持。
		return list
	case RouteGroupRotate:
		return m.rotateGroup(key, list)
	case RouteGroupCheap:
		out := append([]engine.Engine(nil), list...)
		sort.SliceStable(out, func(i, j int) bool {
			return m.modelMultiplier(key, out[i]) < m.modelMultiplier(key, out[j])
		})
		return out
	default: // RouteGroupSmart
		// 与原行为一致：没保存过账号顺序时，把目录归属的首选引擎提到最前；
		// 保存过顺序则完全按用户顺序，智能模式只作用于引擎内部的选号。
		if !m.routingConfigured() {
			if primary, ok := m.pickEngine(key); ok {
				return moveEngineFirst(list, primary)
			}
		}
		return list
	}
}

// rotateGroup 用组内计数轮换起始供应商；计数只在进程内，重启后从头开始。
func (m *Manager) rotateGroup(key string, list []engine.Engine) []engine.Engine {
	v, _ := m.groupRotate.LoadOrStore(key, &atomic.Uint64{})
	n := v.(*atomic.Uint64).Add(1) - 1
	off := int(n % uint64(len(list)))
	if off == 0 {
		return list
	}
	out := make([]engine.Engine, 0, len(list))
	out = append(out, list[off:]...)
	out = append(out, list[:off]...)
	return out
}

func (m *Manager) modelMultiplier(modelName string, eng engine.Engine) float64 {
	if eng == nil {
		return 1 << 20
	}
	if m.st == nil {
		return 1
	}
	return m.st.EffectiveModelMultiplier(modelName, eng.Name())
}
