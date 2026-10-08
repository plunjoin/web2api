package admin

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"web2api/internal/store"
	"web2api/internal/version"
)

// KeyOptions 创建/修改 Key 时可选的额度与倍率字段。
type KeyOptions struct {
	Name       *string  `json:"name"`
	Enabled    *bool    `json:"enabled"`
	TokenLimit *int64   `json:"token_limit"`
	Multiplier *float64 `json:"multiplier"`
	ResetUsage bool     `json:"reset_usage"`
	// ExpiresAt Unix 秒；0 清除过期时间。
	ExpiresAt     *int64    `json:"expires_at"`
	AllowedModels *[]string `json:"allowed_models"`
	RPMLimit      *int64    `json:"rpm_limit"`
}

func (o KeyOptions) update() store.KeyUpdate {
	return store.KeyUpdate{Name: o.Name, Enabled: o.Enabled, TokenLimit: o.TokenLimit, Multiplier: o.Multiplier, ResetUsage: o.ResetUsage,
		ExpiresAt: o.ExpiresAt, AllowedModels: o.AllowedModels, RPMLimit: o.RPMLimit}
}

func (o KeyOptions) validate() error {
	if o.TokenLimit != nil && *o.TokenLimit < 0 {
		return errors.New("token_limit 不能为负数（0 表示不限）")
	}
	if o.Multiplier != nil {
		if err := store.ValidateMultiplier(*o.Multiplier); err != nil {
			return err
		}
	}
	if o.ExpiresAt != nil && *o.ExpiresAt < 0 {
		return errors.New("expires_at 不能为负数（0 表示永不过期）")
	}
	if o.RPMLimit != nil && (*o.RPMLimit < 0 || *o.RPMLimit > 1_000_000) {
		return errors.New("rpm_limit 必须在 0 到 1000000 之间（0 表示不单独限制）")
	}
	if o.Name != nil && len(*o.Name) > 200 {
		return errors.New("name 最长 200 字节")
	}
	return nil
}

func storeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

// usageWindow 解析 ?days=（1-90，默认 7）。
func usageWindow(r *http.Request) (int, int64) {
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 90 {
			days = n
		}
	}
	return days, time.Now().AddDate(0, 0, -days).Unix()
}

func usageFilter(r *http.Request) (store.UsageFilter, int) {
	days, since := usageWindow(r)
	q := r.URL.Query()
	filter := store.UsageFilter{Since: since, Model: strings.TrimSpace(q.Get("model"))}
	filter.KeyID, _ = strconv.ParseInt(q.Get("key_id"), 10, 64)
	filter.Limit, _ = strconv.Atoi(q.Get("limit"))
	filter.Offset, _ = strconv.Atoi(q.Get("offset"))
	return filter, days
}

// handleUsageRecords 逐请求用量明细：GET /admin/api/usage/records?days&key_id&model&limit&offset
func (a *API) handleUsageRecords(w http.ResponseWriter, r *http.Request) {
	filter, days := usageFilter(r)
	records, total, err := a.st.ListUsageRecords(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days": days, "total": total, "limit": limit, "offset": filter.Offset, "records": records,
	})
}

// handleListMultipliers GET /admin/api/multipliers
func (a *API) handleListMultipliers(w http.ResponseWriter, r *http.Request) {
	items, err := a.st.ListModelMultipliers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defaultMultiplier, _ := a.st.ModelMultiplierFor(store.DefaultModelKey)
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
		"multipliers":        items,
		"default_multiplier": defaultMultiplier,
		"models":             models,
		"formula":            "charged_tokens = ceil(total_tokens × model_multiplier × key_multiplier)",
	})
}

// handlePutMultiplier PUT /admin/api/multipliers {model, multiplier}；model="*" 为默认倍率。
func (a *API) handlePutMultiplier(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model      string   `json:"model"`
		Multiplier *float64 `json:"multiplier"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.Multiplier == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "multiplier 必填"})
		return
	}
	item, err := a.st.SetModelMultiplier(req.Model, *req.Multiplier)
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"multiplier": item})
}

// handleDeleteMultiplier DELETE /admin/api/multipliers/{model}
func (a *API) handleDeleteMultiplier(w http.ResponseWriter, r *http.Request) {
	if err := a.st.DeleteModelMultiplier(r.PathValue("model")); err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRegenerateKey POST /admin/api/keys/{id}/regenerate：换发新密钥，旧密钥立即失效，其余设置保留。
func (a *API) handleRegenerateKey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效 ID"})
		return
	}
	key, err := a.st.RegenerateKey(id)
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key})
}

// handleUsageTimeseries GET /admin/api/usage/timeseries?days&key_id&model
// days=1 按小时分桶，其余按本地自然日分桶。
func (a *API) handleUsageTimeseries(w http.ResponseWriter, r *http.Request) {
	filter, days := usageFilter(r)
	now := time.Now()
	_, offset := now.Zone()
	bucket, label := int64(86400), "day"
	if days <= 1 {
		bucket, label = 3600, "hour"
	}
	points, err := a.st.UsageTimeseries(filter, bucket, int64(offset), now.Unix())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days, "bucket": label, "bucket_seconds": bucket, "points": points})
}

// handleUsageExport GET /admin/api/usage/export.csv?days&key_id&model：导出逐请求明细（最多 100000 行，Key 脱敏）。
func (a *API) handleUsageExport(w http.ResponseWriter, r *http.Request) {
	filter, days := usageFilter(r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="web2api-usage-%dd-%s.csv"`, days, time.Now().Format("20060102-150405")))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("\xEF\xBB\xBF")) // UTF-8 BOM，便于 Excel 正确识别中文
	out := csv.NewWriter(w)
	_ = out.Write([]string{"id", "time", "key_id", "key_name", "key_masked", "engine", "model", "endpoint", "stream", "success",
		"prompt_tokens", "completion_tokens", "total_tokens", "usage_source", "model_multiplier", "key_multiplier", "multiplier",
		"charged_tokens", "latency_ms", "error"})
	const pageSize = 500
	for offset, written := 0, 0; written < 100000; offset += pageSize {
		filter.Limit, filter.Offset = pageSize, offset
		records, _, err := a.st.ListUsageRecords(filter)
		if err != nil || len(records) == 0 {
			break
		}
		for _, rec := range records {
			source := "upstream"
			if rec.Estimated {
				source = "estimated"
			}
			_ = out.Write([]string{
				strconv.FormatInt(rec.ID, 10), time.Unix(rec.TS, 0).Format(time.RFC3339), strconv.FormatInt(rec.KeyID, 10), rec.KeyName,
				rec.KeyMasked, rec.Engine, rec.Model, rec.Endpoint, strconv.FormatBool(rec.Stream), strconv.FormatBool(rec.Success),
				strconv.FormatInt(rec.PromptTokens, 10), strconv.FormatInt(rec.CompletionTokens, 10), strconv.FormatInt(rec.TotalTokens, 10),
				source, strconv.FormatFloat(rec.ModelMultiplier, 'f', -1, 64), strconv.FormatFloat(rec.KeyMultiplier, 'f', -1, 64),
				strconv.FormatFloat(rec.Multiplier, 'f', -1, 64), strconv.FormatInt(rec.ChargedTokens, 10),
				strconv.FormatInt(rec.LatencyMs, 10), rec.Error,
			})
			written++
		}
		if len(records) < pageSize {
			break
		}
	}
	out.Flush()
}

// handleModels GET /admin/api/models：聚合模型目录，附带生效倍率。
func (a *API) handleModels(w http.ResponseWriter, r *http.Request) {
	type modelView struct {
		ID          string  `json:"id"`
		DisplayName string  `json:"display_name,omitempty"`
		Engine      string  `json:"engine,omitempty"`
		Available   bool    `json:"available"`
		Multiplier  float64 `json:"multiplier"`
	}
	out := []modelView{}
	if a.mgr != nil {
		for _, m := range a.mgr.ListModels() {
			multiplier, _ := a.st.ModelMultiplierFor(m.ID)
			out = append(out, modelView{ID: m.ID, DisplayName: m.DisplayName, Engine: m.Engine, Available: m.Available, Multiplier: multiplier})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, http.StatusOK, map[string]any{"models": out})
}

// handleVersion GET /admin/api/version
func (a *API) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.Get())
}
