// providers.go 管理台共用的「当前可接单供应商」口径。
package admin

import (
	"web2api/internal/config"
)

// activeProviders 返回当前可能接单的供应商键：
//   - 已注册且就绪的引擎（如 AI Studio upstream 模式没有账号行，但能直连上游）；
//   - 还有账号的订阅引擎；
//   - 第三方 Key 按地址分组（custom|<base_url>），不使用裸 "custom"。
//
// 目录快照会跨账号删除保留，管理台用它过滤掉已经没有任何承接方的供应商。
// mgr 为空（测试或降级启动）时返回 nil，调用方保持「展示全部快照」的旧行为。
func (a *API) activeProviders() map[string]struct{} {
	if a.mgr == nil {
		return nil
	}
	active := map[string]struct{}{}
	for name, eng := range a.mgr.Engines() {
		if name == "custom" || eng == nil || !eng.Ready() {
			continue
		}
		active[name] = struct{}{}
	}
	for _, ov := range a.mgr.AccountOverview() {
		eng := config.NormalizeEngineName(ov.Engine)
		if eng == "custom" {
			if ov.Provider != "" {
				active["custom|"+ov.Provider] = struct{}{}
			}
			continue
		}
		active[eng] = struct{}{}
	}
	return active
}
