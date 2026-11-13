package admin

import (
	"net/http"
	"sort"

	"web2api/internal/store"
)

// ==================== 模型单价（人民币元 / 百万 Token） ====================

// handleListPrices GET /admin/api/prices
// 返回全部已配置单价、默认单价、可选模型列表与展示用汇率。
func (a *API) handleListPrices(w http.ResponseWriter, r *http.Request, _ *Principal) {
	prices, err := a.st.ListModelPrices()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	settings, err := a.st.GetPlatformSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defaultPrice := store.DefaultModelPrice()
	for _, p := range prices {
		if p.Model == store.DefaultModelKey {
			defaultPrice = p
		}
	}
	models := []string{}
	seen := map[string]bool{}
	if a.mgr != nil {
		for _, m := range a.mgr.ListModels() {
			if m.ID != "" && !seen[m.ID] {
				seen[m.ID] = true
				models = append(models, m.ID)
			}
		}
	}
	sort.Strings(models)
	writeJSON(w, http.StatusOK, map[string]any{
		"prices":        prices,
		"default_price": defaultPrice,
		"models":        models,
		"cny_per_usd":   settings.CNYPerUSD,
		"currency":      "CNY",
		"unit":          "元 / 百万 Token",
	})
}

// handlePutPrice PUT /admin/api/prices {model, input, output, cache_read}
func (a *API) handlePutPrice(w http.ResponseWriter, r *http.Request, _ *Principal) {
	var req struct {
		Model     string   `json:"model"`
		Input     *float64 `json:"input"`
		Output    *float64 `json:"output"`
		CacheRead *float64 `json:"cache_read"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.Input == nil || req.Output == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "input 与 output 单价必填"})
		return
	}
	cacheRead := -1.0
	if req.CacheRead != nil {
		cacheRead = *req.CacheRead
	}
	item, err := a.st.SetModelPrice(store.ModelPrice{
		Model: req.Model, Input: *req.Input, Output: *req.Output, CacheRead: cacheRead,
	})
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"price": item})
}

// handleDeletePrice DELETE /admin/api/prices/{model}
func (a *API) handleDeletePrice(w http.ResponseWriter, r *http.Request, _ *Principal) {
	if err := a.st.DeleteModelPrice(r.PathValue("model")); err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
