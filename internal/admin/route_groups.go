// route_groups.go 同名模型路由组：目录快照里同一个模型名被多个供应商提供时，
// 自动形成一个路由组（与 magpie 的 found groups 一致）。网关请求这个模型名时
// 会按组内模式在供应商之间选择并自动故障切换。
package admin

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"web2api/internal/config"
	"web2api/internal/model"
	"web2api/internal/provider"
	"web2api/internal/store"
)

type routeGroupMemberView struct {
	Key             string  `json:"key"` // 路由组键：引擎名或 custom|<base_url>
	Engine          string  `json:"engine"`
	Provider        string  `json:"provider,omitempty"`
	Model           string  `json:"model"` // 该供应商目录里的原始模型 ID
	Display         string  `json:"display,omitempty"`
	Kind            string  `json:"kind,omitempty"`
	Free            bool    `json:"free,omitempty"`
	Accounts        int     `json:"accounts"`
	EnabledAccounts int     `json:"enabled_accounts"`
	ReadyAccounts   int     `json:"ready_accounts"`
	Ready           bool    `json:"ready"`
	Direct          bool    `json:"direct,omitempty"` // 引擎直连上游，不依赖账号行
	Multiplier      float64 `json:"multiplier"`
}

type routeGroupView struct {
	Key     string                 `json:"key"` // 归一化模型名
	Name    string                 `json:"name"`
	Enabled bool                   `json:"enabled"`
	Mode    string                 `json:"mode"`
	Members []routeGroupMemberView `json:"members"`
}

type routeGroupAccountStat struct {
	accounts int
	enabled  int
	ready    int
	rank     int
}

// handleRouteGroups GET /admin/api/route-groups。
func (a *API) handleRouteGroups(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"groups": a.routeGroups(),
		"modes":  provider.RouteGroupModes,
	})
}

// handlePutRouteGroup PUT /admin/api/route-groups {model, mode}。
func (a *API) handlePutRouteGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
		Mode  string `json:"mode"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "model 不能为空"})
		return
	}
	if err := a.mgr.SetRoutingGroupMode(req.Model, req.Mode); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "groups": a.routeGroups(), "modes": provider.RouteGroupModes})
}

// routeGroups 从目录快照 + 当前账号聚合同名模型路由组。
// 只有两个及以上供应商提供同一个模型名时才是路由组；单个供应商的模型
// 直接走该供应商自己的选号，不需要组。
func (a *API) routeGroups() []routeGroupView {
	stats, active := a.routeGroupAccountStats()
	if len(active) == 0 {
		return []routeGroupView{}
	}

	settings := map[string]store.ModelSetting{}
	if list, err := a.st.ListModelSettings(); err == nil {
		for _, s := range list {
			settings[model.CanonicalModelID(s.Model)] = s
		}
	}

	catalog, err := a.st.CatalogModels()
	if err != nil {
		return []routeGroupView{}
	}

	byKey := map[string]*routeGroupView{}
	seenMember := map[string]map[string]struct{}{}
	for _, m := range catalog {
		key := strings.TrimSpace(m.Engine)
		if _, ok := active[key]; !ok {
			continue
		}
		groupKey := model.CanonicalModelID(m.Model)
		if groupKey == "" {
			continue
		}
		g := byKey[groupKey]
		if g == nil {
			g = &routeGroupView{Key: groupKey, Name: model.PublicModelID(m.Model)}
			byKey[groupKey] = g
		}
		seen := seenMember[groupKey]
		if seen == nil {
			seen = map[string]struct{}{}
			seenMember[groupKey] = seen
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		eng := key
		providerName := ""
		if strings.HasPrefix(key, "custom|") {
			eng = "custom"
			providerName = strings.TrimPrefix(key, "custom|")
		}
		st := stats[key]
		engineReady := false
		if eng != "custom" {
			if e := a.mgr.Engines()[eng]; e != nil {
				engineReady = e.Ready()
			}
		}
		ready := st.ready > 0 || engineReady
		g.Members = append(g.Members, routeGroupMemberView{
			Key: key, Engine: eng, Provider: providerName,
			Model: model.PublicModelID(m.Model), Display: m.DisplayName,
			Kind: m.Kind, Free: m.Free,
			Accounts: st.accounts, EnabledAccounts: st.enabled, ReadyAccounts: st.ready,
			Ready: ready, Direct: st.accounts == 0 && engineReady,
			Multiplier: a.st.EffectiveModelMultiplier(m.Model, key),
		})
		if g.Name == "" && strings.TrimSpace(m.DisplayName) != "" {
			g.Name = m.DisplayName
		}
	}

	out := make([]routeGroupView, 0, len(byKey))
	for key, g := range byKey {
		if len(g.Members) < 2 {
			continue
		}
		// 成员按实际接单优先级排列：配置过账号顺序时按组内最靠前账号，
		// 否则按供应商首次出现顺序（stats 的 rank 已含引擎注册顺序兜底）。
		sort.SliceStable(g.Members, func(i, j int) bool {
			ri, rj := stats[g.Members[i].Key].rank, stats[g.Members[j].Key].rank
			if ri != rj {
				return ri < rj
			}
			return g.Members[i].Key < g.Members[j].Key
		})
		if s, ok := settings[key]; ok {
			g.Enabled = s.Enabled
		} else {
			g.Enabled = true
		}
		g.Mode = a.mgr.RoutingGroupMode(key)
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Enabled != out[j].Enabled {
			return out[i].Enabled
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// routeGroupAccountStats 统计每个供应商（引擎 / custom|url）的账号数与可用数，
// 并给出该供应商在账号路由顺序里的位次。active 只包含仍有账号的供应商，
// 与「模型与倍率」页的过滤口径一致。
func (a *API) routeGroupAccountStats() (map[string]routeGroupAccountStat, map[string]struct{}) {
	stats := map[string]routeGroupAccountStat{}
	active := a.activeProviders()
	if active == nil {
		active = map[string]struct{}{}
	}

	rank := map[int64]int{}
	for i, raw := range a.mgr.RoutingOrder() {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			continue
		}
		if _, exists := rank[id]; !exists {
			rank[id] = i
		}
	}

	engineOrder := map[string]int{}
	for i, name := range a.mgr.EngineOrder() {
		engineOrder[name] = i
	}

	engines := a.mgr.Engines()
	for _, ov := range a.mgr.AccountOverview() {
		eng := config.NormalizeEngineName(ov.Engine)
		// 只有注册在案的引擎才可能接单；存量账号所属引擎被配置关闭时，
		// 它不能作为路由组成员，否则页面会展示一个永远不会被尝试的供应商。
		if engines[eng] == nil {
			continue
		}
		key := eng
		if eng == "custom" {
			if ov.Provider == "" {
				key = "custom|unknown"
			} else {
				key = "custom|" + ov.Provider
			}
		}
		if _, ok := active[key]; !ok {
			continue
		}
		st, seen := stats[key]
		st.accounts++
		if ov.Enabled {
			st.enabled++
		}
		if ov.Live.Ready {
			st.ready++
		}
		// 首选账号在位次里越靠前，供应商越优先；没有配置顺序时按引擎注册顺序。
		pos := 1 << 30
		if r, ok := rank[ov.ID]; ok {
			pos = r
		} else {
			base := engineOrder[eng]
			pos = 1<<29 + base*1000000 + int(ov.ID)
		}
		if !seen || pos < st.rank {
			st.rank = pos
		}
		stats[key] = st
	}
	// 没有账号行但引擎已就绪（如 AI Studio upstream 直连）：保留为组成员，
	// 位次排在所有带账号的供应商之后。
	for key := range active {
		if _, ok := stats[key]; ok || strings.HasPrefix(key, "custom|") {
			continue
		}
		stats[key] = routeGroupAccountStat{rank: 1<<29 + engineOrder[key]*1000000}
	}
	return stats, active
}
