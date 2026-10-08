package admin

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"web2api/internal/session"
	"web2api/internal/store"
)

// ==================== 用户控制台 /api/user/* ====================

var emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// userSafeRecord 给普通用户看的用量记录：上游错误里可能带号池账号邮箱，统一脱敏。
func userSafeRecord(rec store.UsageRecord) store.UsageRecord {
	if rec.Error != "" {
		rec.Error = emailPattern.ReplaceAllString(rec.Error, "***")
		if len(rec.Error) > 300 {
			rec.Error = rec.Error[:300] + "…"
		}
	}
	rec.UserEmail = ""
	return rec
}

func startOfToday(now time.Time) int64 {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Unix()
}

func sumBreakdown(rows []store.UsageBreakdownRow) map[string]int64 {
	var requests, success, total, charged int64
	for _, r := range rows {
		requests += r.Requests
		success += r.SuccessRequests
		total += r.TotalTokens
		charged += r.ChargedTokens
	}
	return map[string]int64{"requests": requests, "success_requests": success, "total_tokens": total, "charged_tokens": charged}
}

// handleUserOverview GET /api/user/overview
func (a *API) handleUserOverview(w http.ResponseWriter, r *http.Request, p *Principal) {
	u := p.User
	now := time.Now()
	today, err := a.st.UsageBreakdown(store.UsageFilter{Since: startOfToday(now), UserID: u.ID})
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	week, err := a.st.UsageBreakdown(store.UsageFilter{Since: now.AddDate(0, 0, -7).Unix(), UserID: u.ID})
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	_, offset := now.Zone()
	series, err := a.st.UsageTimeseries(store.UsageFilter{Since: now.AddDate(0, 0, -13).Unix(), UserID: u.ID}, 86400, int64(offset), now.Unix())
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	recent, _, err := a.st.ListUsageRecords(store.UsageFilter{UserID: u.ID, Limit: 8})
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	for i := range recent {
		recent[i] = userSafeRecord(recent[i])
	}
	keys, _ := a.st.ListUserKeys(u.ID)
	enabledKeys := 0
	for _, k := range keys {
		if k.Enabled {
			enabledKeys++
		}
	}
	settings, _ := a.st.GetPlatformSettings()
	// 按模型汇总 7 天消耗（前 5）
	byModel := map[string]int64{}
	for _, row := range week {
		byModel[row.Model] += row.ChargedTokens
	}
	type modelShare struct {
		Model         string `json:"model"`
		ChargedTokens int64  `json:"charged_tokens"`
	}
	models := []modelShare{}
	for m, v := range byModel {
		models = append(models, modelShare{m, v})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ChargedTokens > models[j].ChargedTokens })
	if len(models) > 5 {
		models = models[:5]
	}
	writeJSON(w, 200, map[string]any{
		"user":         p.view(),
		"balance":      u.Balance,
		"multiplier":   u.Multiplier,
		"today":        sumBreakdown(today),
		"week":         sumBreakdown(week),
		"series":       series,
		"recent":       recent,
		"keys_total":   len(keys),
		"keys_enabled": enabledKeys,
		"top_models":   models,
		"announcement": settings.Announcement,
	})
}

func (a *API) ownKey(w http.ResponseWriter, r *http.Request, p *Principal) (store.APIKey, bool) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "无效 ID"})
		return store.APIKey{}, false
	}
	k, err := a.st.GetKey(id)
	if err != nil || k.UserID != p.User.ID {
		writeJSON(w, 404, map[string]any{"error": "Key 不存在"})
		return store.APIKey{}, false
	}
	return k, true
}

// handleUserKeys GET /api/user/keys
func (a *API) handleUserKeys(w http.ResponseWriter, r *http.Request, p *Principal) {
	keys, err := a.st.ListUserKeys(p.User.ID)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	since := time.Now().AddDate(0, 0, -7).Unix()
	charged := map[int64]int64{}
	requests := map[int64]int64{}
	if rows, err := a.st.UsageBreakdown(store.UsageFilter{Since: since, UserID: p.User.ID}); err == nil {
		for _, row := range rows {
			charged[row.KeyID] += row.ChargedTokens
			requests[row.KeyID] += row.Requests
		}
	}
	type keyView struct {
		store.APIKey
		Charged7d  int64 `json:"charged_7d"`
		Requests7d int64 `json:"requests_7d"`
	}
	out := make([]keyView, 0, len(keys))
	for _, k := range keys {
		out = append(out, keyView{APIKey: k, Charged7d: charged[k.ID], Requests7d: requests[k.ID]})
	}
	settings, _ := a.st.GetPlatformSettings()
	writeJSON(w, 200, map[string]any{"keys": out, "max_keys": settings.MaxKeysPerUser})
}

// handleUserCreateKey POST /api/user/keys {name, token_limit}
func (a *API) handleUserCreateKey(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Name       string `json:"name"`
		TokenLimit int64  `json:"token_limit"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "请求 JSON 无效"})
		return
	}
	settings, _ := a.st.GetPlatformSettings()
	k, err := a.st.CreateUserKey(p.User.ID, req.Name, req.TokenLimit, settings.MaxKeysPerUser)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"key": k})
}

// handleUserPatchKey PATCH /api/user/keys/{id} {name?, enabled?, token_limit?, reset_usage?}
func (a *API) handleUserPatchKey(w http.ResponseWriter, r *http.Request, p *Principal) {
	k, ok := a.ownKey(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Name       *string `json:"name"`
		Enabled    *bool   `json:"enabled"`
		TokenLimit *int64  `json:"token_limit"`
		ResetUsage bool    `json:"reset_usage"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "请求 JSON 无效"})
		return
	}
	if req.Name != nil && len(*req.Name) > 200 {
		writeJSON(w, 400, map[string]any{"error": "名称最长 200 字节"})
		return
	}
	updated, err := a.st.UpdateKey(k.ID, store.KeyUpdate{Name: req.Name, Enabled: req.Enabled, TokenLimit: req.TokenLimit, ResetUsage: req.ResetUsage})
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"key": updated})
}

// handleUserDeleteKey DELETE /api/user/keys/{id}
func (a *API) handleUserDeleteKey(w http.ResponseWriter, r *http.Request, p *Principal) {
	k, ok := a.ownKey(w, r, p)
	if !ok {
		return
	}
	if err := a.st.DeleteKey(k.ID); err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleUserRegenerateKey POST /api/user/keys/{id}/regenerate
func (a *API) handleUserRegenerateKey(w http.ResponseWriter, r *http.Request, p *Principal) {
	k, ok := a.ownKey(w, r, p)
	if !ok {
		return
	}
	updated, err := a.st.RegenerateKey(k.ID)
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"key": updated})
}

// handleUserUsageRecords GET /api/user/usage/records?days&key_id&model&limit&offset
func (a *API) handleUserUsageRecords(w http.ResponseWriter, r *http.Request, p *Principal) {
	filter, days := usageFilter(r)
	filter.UserID = p.User.ID
	records, total, err := a.st.ListUsageRecords(filter)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	for i := range records {
		records[i] = userSafeRecord(records[i])
	}
	breakdown, _ := a.st.UsageBreakdown(filter)
	writeJSON(w, 200, map[string]any{"days": days, "total": total, "records": records, "totals": sumBreakdown(breakdown)})
}

// handleUserUsageTimeseries GET /api/user/usage/timeseries?days
func (a *API) handleUserUsageTimeseries(w http.ResponseWriter, r *http.Request, p *Principal) {
	filter, days := usageFilter(r)
	filter.UserID = p.User.ID
	now := time.Now()
	_, offset := now.Zone()
	bucket, label := int64(86400), "day"
	if days <= 1 {
		bucket, label = 3600, "hour"
	}
	points, err := a.st.UsageTimeseries(filter, bucket, int64(offset), now.Unix())
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"days": days, "bucket": label, "points": points})
}

func ledgerFilter(r *http.Request) store.LedgerFilter {
	q := r.URL.Query()
	f := store.LedgerFilter{Kind: q.Get("kind"), Query: q.Get("q")}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	f.Offset, _ = strconv.Atoi(q.Get("offset"))
	f.UserID, _ = strconv.ParseInt(q.Get("user_id"), 10, 64)
	if days, err := strconv.Atoi(q.Get("days")); err == nil && days > 0 && days <= 3650 {
		f.Since = time.Now().AddDate(0, 0, -days).Unix()
	}
	return f
}

// handleUserLedger GET /api/user/ledger?kind&days&limit&offset
func (a *API) handleUserLedger(w http.ResponseWriter, r *http.Request, p *Principal) {
	f := ledgerFilter(r)
	f.UserID, f.Query = p.User.ID, ""
	entries, total, sums, err := a.st.ListLedger(f)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	for i := range entries {
		entries[i].UserEmail = ""
		if entries[i].Kind != store.LedgerRedeem {
			entries[i].Operator = "" // 不向用户暴露管理员账号
		}
	}
	writeJSON(w, 200, map[string]any{"entries": entries, "total": total, "sums": sums, "balance": p.User.Balance})
}

// handleUserRedeem POST /api/user/redeem {code}
func (a *API) handleUserRedeem(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		writeJSON(w, 400, map[string]any{"error": "请输入兑换码"})
		return
	}
	v, _ := a.st.AdminSettings()
	var redis *session.Redis
	failKey := ""
	if v != nil {
		redis, _ = session.New(v.RedisURL)
		failKey = redisPrefix(v) + "redeem-fail:" + strconv.FormatInt(p.User.ID, 10)
	}
	if redis != nil {
		if n, err := redis.Command(r.Context(), "GET", failKey); err == nil {
			if count, _ := strconv.Atoi(n); count >= 20 {
				w.Header().Set("Retry-After", "900")
				writeJSON(w, 429, map[string]any{"error": "兑换失败次数过多，请 15 分钟后再试"})
				return
			}
		}
	}
	res, err := a.st.Redeem(p.User.ID, req.Code)
	if err != nil {
		if redis != nil {
			_, _ = bump(r, redis, failKey)
		}
		status := 400
		switch {
		case errors.Is(err, store.ErrCodeNotFound):
			status = 404
		case errors.Is(err, store.ErrCodeRedeemed), errors.Is(err, store.ErrCodeDisabled), errors.Is(err, store.ErrCodeExpired):
			status = 409
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"amount": res.Amount, "balance": res.Entry.BalanceAfter, "entry": res.Entry})
}

// handleUserRedeemHistory GET /api/user/redeem
func (a *API) handleUserRedeemHistory(w http.ResponseWriter, r *http.Request, p *Principal) {
	codes, total, err := a.st.ListRedeemCodes(store.CodeFilter{UserID: p.User.ID, Limit: 100})
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	type view struct {
		Code       string `json:"code"`
		Amount     int64  `json:"amount"`
		RedeemedAt int64  `json:"redeemed_at"`
		Note       string `json:"note"`
	}
	out := make([]view, 0, len(codes))
	for _, c := range codes {
		out = append(out, view{Code: store.MaskCode(c.Code), Amount: c.Amount, RedeemedAt: c.RedeemedAt, Note: c.Note})
	}
	writeJSON(w, 200, map[string]any{"history": out, "total": total})
}

// handleUserProfile PATCH /api/user/profile {nickname}
func (a *API) handleUserProfile(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Nickname *string `json:"nickname"`
	}
	if err := readJSON(r, &req); err != nil || req.Nickname == nil {
		writeJSON(w, 400, map[string]any{"error": "请提供昵称"})
		return
	}
	u, err := a.st.UpdateUser(p.User.ID, store.UserUpdate{Nickname: req.Nickname})
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": (&Principal{User: &u}).view()})
}

// handleUserPassword POST /api/user/password {old_password, new_password}：改密后签发新登录态，旧会话全部失效。
func (a *API) handleUserPassword(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Old string `json:"old_password"`
		New string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil || len(req.Old) > 72 {
		writeJSON(w, 400, map[string]any{"error": "请求无效"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(p.User.PasswordHash), []byte(req.Old)) != nil {
		writeJSON(w, 400, map[string]any{"error": "当前密码不正确"})
		return
	}
	if err := validPassword(req.New); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.New), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "密码处理失败"})
		return
	}
	hashStr := string(hash)
	u, err := a.st.UpdateUser(p.User.ID, store.UserUpdate{PasswordHash: &hashStr})
	if err != nil {
		storeError(w, err)
		return
	}
	out := map[string]any{"ok": true}
	if v, _ := a.st.AdminSettings(); v != nil {
		if redis, err := session.New(v.RedisURL); err == nil {
			if token, err := issueSession(r, v, redis, "u:"+strconv.FormatInt(u.ID, 10), u.TokenVersion); err == nil {
				out["access_token"] = token
			}
		}
	}
	writeJSON(w, 200, out)
}

// handleUserModels GET /api/user/models：可用模型及对该用户生效的倍率（模型倍率 × 用户倍率）。
func (a *API) handleUserModels(w http.ResponseWriter, r *http.Request, p *Principal) {
	type modelView struct {
		ID          string  `json:"id"`
		DisplayName string  `json:"display_name,omitempty"`
		Available   bool    `json:"available"`
		Multiplier  float64 `json:"multiplier"`
	}
	out := []modelView{}
	if a.mgr != nil {
		seen := map[string]bool{}
		for _, m := range a.mgr.ListModels() {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			mm, _ := a.st.ModelMultiplierFor(m.ID)
			out = append(out, modelView{ID: m.ID, DisplayName: m.DisplayName, Available: m.Available, Multiplier: mm * p.User.Multiplier})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, 200, map[string]any{"models": out, "user_multiplier": p.User.Multiplier,
		"formula": "扣费 Token = ⌈总 Token × 模型倍率 × Key 倍率 × 用户倍率⌉"})
}
