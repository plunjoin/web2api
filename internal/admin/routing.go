package admin

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"web2api/internal/config"
)

// 引擎在路由页的默认展示顺序（供应商添加顺序）。
var routingEngineOrder = []string{"geminiweb", "aistudio", "codex", "antigravity", "kiro", "workbuddy", "custom"}

type routingAccountView struct {
	ID       int64  `json:"id"`
	Label    string `json:"label"`
	Engine   string `json:"engine"`
	Provider string `json:"provider,omitempty"`
	Enabled  bool   `json:"enabled"`
	Ready    bool   `json:"ready"`
	Status   string `json:"status"`
	Models   int    `json:"models"`
}

type routingSupplierView struct {
	Key      string               `json:"key"`
	Engine   string               `json:"engine"`
	Provider string               `json:"provider,omitempty"`
	Accounts []routingAccountView `json:"accounts"`
}

// handleRouting GET /admin/api/routing：返回按供应商分组的账号与当前顺序。
func (a *API) handleRouting(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.routingView())
}

// handlePutRouting PUT /admin/api/routing：保存账号顺序（全局平铺列表）。
// 传空列表表示清除配置，恢复默认顺序。
func (a *API) handlePutRouting(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Accounts []int64 `json:"accounts"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	// 只保存当前仍存在的账号：删除账号后旧 ID 不再进入顺序。
	known := map[int64]struct{}{}
	for _, ov := range a.mgr.AccountOverview() {
		known[ov.ID] = struct{}{}
	}
	ids := make([]string, 0, len(req.Accounts))
	seen := map[int64]struct{}{}
	for _, id := range req.Accounts {
		if _, ok := known[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	if err := a.mgr.ApplyRoutingOrder(ids); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, a.routingView())
}

func (a *API) routingView() map[string]any {
	accounts := a.mgr.AccountOverview()
	order := a.mgr.RoutingOrder()
	configured := len(order) > 0

	rank := map[int64]int{}
	for i, raw := range order {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			continue
		}
		if _, exists := rank[id]; !exists {
			rank[id] = i
		}
	}

	groups := map[string]*routingSupplierView{}
	firstSeen := map[string]int{}
	for seq, ov := range accounts {
		engine := config.NormalizeEngineName(ov.Engine)
		key := engine
		groupEngine := engine
		provider := ""
		if engine == "custom" {
			provider = ov.Provider
			key = "custom|" + provider
			if provider == "" {
				key = "custom|unknown"
			}
		}
		g := groups[key]
		if g == nil {
			g = &routingSupplierView{Key: key, Engine: groupEngine, Provider: provider}
			groups[key] = g
			firstSeen[key] = seq
		}
		models := ov.Live.Models
		status := ov.Live.Status
		account := routingAccountView{
			ID: ov.ID, Label: ov.Label, Engine: groupEngine, Provider: provider,
			Enabled: ov.Enabled, Ready: ov.Live.Ready, Status: status, Models: models,
		}
		g.Accounts = append(g.Accounts, account)
	}

	// 同组内：配置过顺序按顺序，否则按账号创建顺序（ID）。
	for _, g := range groups {
		sort.SliceStable(g.Accounts, func(i, j int) bool {
			ri, iok := rank[g.Accounts[i].ID]
			rj, jok := rank[g.Accounts[j].ID]
			if configured {
				if iok != jok {
					return iok
				}
				if iok && ri != rj {
					return ri < rj
				}
			}
			return g.Accounts[i].ID < g.Accounts[j].ID
		})
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		gi, gj := groups[keys[i]], groups[keys[j]]
		// 配置过顺序：按组内最靠前的账号位置排序。
		if configured {
			return groupRank(gi, rank) < groupRank(gj, rank)
		}
		ri, rj := engineRank(gi.Engine), engineRank(gj.Engine)
		if ri != rj {
			return ri < rj
		}
		return firstSeen[keys[i]] < firstSeen[keys[j]]
	})

	suppliers := make([]routingSupplierView, 0, len(keys))
	flat := make([]int64, 0, len(accounts))
	for _, key := range keys {
		g := groups[key]
		if len(g.Accounts) == 0 {
			continue
		}
		suppliers = append(suppliers, *g)
		for _, account := range g.Accounts {
			flat = append(flat, account.ID)
		}
	}
	return map[string]any{
		"configured": configured,
		"order":      flat,
		"suppliers":  suppliers,
	}
}

func groupRank(g *routingSupplierView, rank map[int64]int) int {
	best := 1 << 30
	for _, account := range g.Accounts {
		if r, ok := rank[account.ID]; ok && r < best {
			best = r
		}
	}
	return best
}

func engineRank(engine string) int {
	for i, name := range routingEngineOrder {
		if name == engine {
			return i
		}
	}
	return len(routingEngineOrder)
}
