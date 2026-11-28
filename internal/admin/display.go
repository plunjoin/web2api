package admin

import (
	"strings"

	"web2api/internal/model"
	"web2api/internal/store"
)

// modelDisplayNames 返回按归一化 ID 索引的展示名，供用量/计费等只存模型 ID 的接口在读取时补全。
// 优先用当前引擎目录，再补已保存的目录快照；展示名与 ID 相同（或为空）时不记录。
func (a *API) modelDisplayNames() map[string]string {
	out := map[string]string{}
	if a.mgr != nil {
		for _, m := range a.mgr.ListAllModels() {
			addModelDisplayName(out, m.ID, m.DisplayName)
		}
	}
	if a.st != nil {
		if catalog, err := a.st.CatalogModels(); err == nil {
			for _, m := range catalog {
				addModelDisplayName(out, m.Model, m.DisplayName)
			}
		}
	}
	return out
}

func addModelDisplayName(out map[string]string, id, displayName string) {
	key := model.CanonicalModelID(id)
	name := strings.TrimSpace(displayName)
	if key == "" || !hasDistinctModelDisplayName(id, name) {
		return
	}
	if _, ok := out[key]; !ok {
		out[key] = name
	}
}

// usageRecordsWithDisplay 给只存模型 ID 的用量记录补展示名，不修改数据库行。
func usageBreakdownWithDisplay(rows []store.UsageBreakdownRow, names map[string]string) []store.UsageBreakdownRow {
	if len(rows) == 0 {
		return rows
	}
	out := append([]store.UsageBreakdownRow(nil), rows...)
	for i := range out {
		out[i].ModelDisplayName = modelDisplayName(names, out[i].Model)
	}
	return out
}

func usageRecordsWithDisplay(records []store.UsageRecord, names map[string]string) []store.UsageRecord {
	if len(records) == 0 {
		return records
	}
	out := append([]store.UsageRecord(nil), records...)
	for i := range out {
		out[i].ModelDisplayName = modelDisplayName(names, out[i].Model)
		out[i].ActualModelDisplayName = modelDisplayName(names, out[i].ActualModel)
	}
	return out
}

func modelDisplayName(names map[string]string, id string) string {
	return names[model.CanonicalModelID(id)]
}

func hasDistinctModelDisplayName(id, displayName string) bool {
	name := strings.TrimSpace(displayName)
	if name == "" {
		return false
	}
	id = strings.TrimSpace(id)
	if name == id || strings.EqualFold(name, "models/"+id) || strings.EqualFold("models/"+name, id) {
		return false
	}
	return true
}
