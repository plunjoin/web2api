// providers.go 管理台共用的「当前可接单供应商」口径。
package admin

import (
	"web2api/internal/config"
)

// activeProviders 返回当前可能接单的供应商键：
//   - 已注册、就绪且没有账号行的直连引擎（如 AI Studio upstream 模式）；
//   - 至少还有一个「启用」账号的引擎 / 第三方 Key 地址（custom|<base_url>）。
//
// 停用的账号不再贡献供应商：某个供应商的账号全部停用时，它的模型与路由组
// 成员都不该继续出现在管理台。目录快照会跨账号删除保留，管理台用它过滤掉
// 已经没有任何可接单账号的供应商。
// mgr 为空（测试或降级启动）时返回 nil，调用方保持「展示全部快照」的旧行为。
func (a *API) activeProviders() map[string]struct{} {
	if a.mgr == nil {
		return nil
	}
	active := map[string]struct{}{}
	accounts := map[string]int{}
	enabled := map[string]int{}
	for _, ov := range a.mgr.AccountOverview() {
		eng := config.NormalizeEngineName(ov.Engine)
		accounts[eng]++
		if !ov.Enabled {
			continue
		}
		enabled[eng]++
		if eng == "custom" {
			// 第三方 Key 按地址分组；没有地址的账号无法对应目录快照。
			if ov.Provider != "" {
				active["custom|"+ov.Provider] = struct{}{}
			}
			continue
		}
		active[eng] = struct{}{}
	}
	for name, eng := range a.mgr.Engines() {
		if name == "custom" || eng == nil || !eng.Ready() {
			continue
		}
		// 有账号行但全部停用的引擎不能算活跃；没有账号行的直连引擎才算。
		if accounts[name] > 0 && enabled[name] == 0 {
			continue
		}
		active[name] = struct{}{}
	}
	return active
}
