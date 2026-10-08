package admin

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"web2api/internal/store"
)

// ==================== 管理台：用户、余额、兑换码、账单、设置 ====================

// handleAdminUsers GET /admin/api/users?q&role&status&limit&offset
func (a *API) handleAdminUsers(w http.ResponseWriter, r *http.Request, p *Principal) {
	q := r.URL.Query()
	f := store.UserFilter{Query: q.Get("q"), Role: q.Get("role"), Status: q.Get("status")}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	f.Offset, _ = strconv.Atoi(q.Get("offset"))
	users, total, err := a.st.ListUsers(f)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"users": users, "total": total})
}

// handleAdminCreateUser POST /admin/api/users {email, password, nickname, role, balance, multiplier, note}
func (a *API) handleAdminCreateUser(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Email      string   `json:"email"`
		Password   string   `json:"password"`
		Nickname   string   `json:"nickname"`
		Role       string   `json:"role"`
		Balance    int64    `json:"balance"`
		Multiplier *float64 `json:"multiplier"`
		Note       string   `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "请求 JSON 无效"})
		return
	}
	if err := validPassword(req.Password); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	if v, _ := a.st.AdminSettings(); v != nil && store.NormalizeEmail(req.Email) == v.Email {
		writeJSON(w, 409, map[string]any{"error": store.ErrEmailTaken.Error()})
		return
	}
	settings, _ := a.st.GetPlatformSettings()
	multiplier := settings.DefaultUserMultiplier
	if req.Multiplier != nil {
		multiplier = *req.Multiplier
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "密码处理失败"})
		return
	}
	user, err := a.st.CreateUser(store.NewUser{Email: req.Email, Nickname: req.Nickname, PasswordHash: string(hash), Role: req.Role,
		Multiplier: multiplier, Note: req.Note, InitialBalance: req.Balance, InitialKind: store.LedgerAdjust,
		InitialNote: "创建账号时的初始余额", Operator: p.Operator()})
	if errors.Is(err, store.ErrEmailTaken) {
		writeJSON(w, 409, map[string]any{"error": err.Error()})
		return
	}
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"user": user})
}

func (a *API) pathUser(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "无效 ID"})
		return store.User{}, false
	}
	u, err := a.st.GetUser(id)
	if err != nil {
		storeError(w, err)
		return store.User{}, false
	}
	return u, true
}

// handleAdminUser GET /admin/api/users/{id}：用户详情（含 Key、近期流水与 7 天用量）。
func (a *API) handleAdminUser(w http.ResponseWriter, r *http.Request, p *Principal) {
	u, ok := a.pathUser(w, r)
	if !ok {
		return
	}
	keys, _ := a.st.ListUserKeys(u.ID)
	ledger, _, sums, _ := a.st.ListLedger(store.LedgerFilter{UserID: u.ID, Limit: 20})
	week, _ := a.st.UsageBreakdown(store.UsageFilter{Since: time.Now().AddDate(0, 0, -7).Unix(), UserID: u.ID})
	writeJSON(w, 200, map[string]any{"user": u, "keys": keys, "ledger": ledger, "week": sumBreakdown(week),
		"total_credit": sums["credit"], "total_debit": sums["debit"]})
}

// handleAdminPatchUser PATCH /admin/api/users/{id} {nickname, role, enabled, multiplier, note}
func (a *API) handleAdminPatchUser(w http.ResponseWriter, r *http.Request, p *Principal) {
	u, ok := a.pathUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Nickname   *string  `json:"nickname"`
		Role       *string  `json:"role"`
		Enabled    *bool    `json:"enabled"`
		Multiplier *float64 `json:"multiplier"`
		Note       *string  `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "请求 JSON 无效"})
		return
	}
	if p.User != nil && p.User.ID == u.ID && ((req.Enabled != nil && !*req.Enabled) || (req.Role != nil && *req.Role != store.RoleAdmin)) {
		writeJSON(w, 400, map[string]any{"error": "不能停用自己或取消自己的管理员权限"})
		return
	}
	updated, err := a.st.UpdateUser(u.ID, store.UserUpdate{Nickname: req.Nickname, Role: req.Role, Enabled: req.Enabled, Multiplier: req.Multiplier, Note: req.Note})
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": updated})
}

// handleAdminDeleteUser DELETE /admin/api/users/{id}：删除用户及其 Key 与流水（用量明细保留）。
func (a *API) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request, p *Principal) {
	u, ok := a.pathUser(w, r)
	if !ok {
		return
	}
	if p.User != nil && p.User.ID == u.ID {
		writeJSON(w, 400, map[string]any{"error": "不能删除自己"})
		return
	}
	if err := a.st.DeleteUser(u.ID); err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleAdminUserPassword POST /admin/api/users/{id}/password {password}：重置密码并让该用户的旧登录态失效。
func (a *API) handleAdminUserPassword(w http.ResponseWriter, r *http.Request, p *Principal) {
	u, ok := a.pathUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "请求 JSON 无效"})
		return
	}
	if err := validPassword(req.Password); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "密码处理失败"})
		return
	}
	h := string(hash)
	if _, err := a.st.UpdateUser(u.ID, store.UserUpdate{PasswordHash: &h}); err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleAdminAdjustBalance POST /admin/api/users/{id}/balance {amount, note}：amount 正数加、负数减。
func (a *API) handleAdminAdjustBalance(w http.ResponseWriter, r *http.Request, p *Principal) {
	u, ok := a.pathUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Amount int64  `json:"amount"`
		Note   string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "请求 JSON 无效（amount 必须是整数）"})
		return
	}
	if strings.TrimSpace(req.Note) == "" {
		writeJSON(w, 400, map[string]any{"error": "请填写备注，便于对账"})
		return
	}
	entry, err := a.st.AdjustBalance(u.ID, req.Amount, req.Note, p.Operator())
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"entry": entry, "balance": entry.BalanceAfter})
}

// handleAdminLedger GET /admin/api/ledger?user_id&kind&q&days&limit&offset
func (a *API) handleAdminLedger(w http.ResponseWriter, r *http.Request, p *Principal) {
	entries, total, sums, err := a.st.ListLedger(ledgerFilter(r))
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"entries": entries, "total": total, "sums": sums})
}

// handleAdminRefund POST /admin/api/usage/records/{id}/refund {note}
func (a *API) handleAdminRefund(w http.ResponseWriter, r *http.Request, p *Principal) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "无效 ID"})
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	_ = readJSON(r, &req)
	entry, err := a.st.RefundUsage(id, req.Note, p.Operator())
	if err != nil {
		status := 400
		if errors.Is(err, store.ErrNotFound) {
			status = 404
		} else if errors.Is(err, store.ErrAlreadyRefunded) {
			status = 409
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"entry": entry})
}

// ---------- 兑换码 ----------

func codeFilter(r *http.Request) store.CodeFilter {
	q := r.URL.Query()
	f := store.CodeFilter{Status: q.Get("status"), Batch: q.Get("batch"), Query: q.Get("q")}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	f.Offset, _ = strconv.Atoi(q.Get("offset"))
	f.UserID, _ = strconv.ParseInt(q.Get("user_id"), 10, 64)
	return f
}

// handleAdminCodes GET /admin/api/redeem-codes?status&batch&q&limit&offset
func (a *API) handleAdminCodes(w http.ResponseWriter, r *http.Request, p *Principal) {
	codes, total, err := a.st.ListRedeemCodes(codeFilter(r))
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	stats, _ := a.st.RedeemCodeStats()
	writeJSON(w, 200, map[string]any{"codes": codes, "total": total, "stats": stats})
}

// handleAdminCreateCodes POST /admin/api/redeem-codes {amount, count, expires_at, note, batch}
func (a *API) handleAdminCreateCodes(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Amount    int64  `json:"amount"`
		Count     int    `json:"count"`
		ExpiresAt int64  `json:"expires_at"`
		Note      string `json:"note"`
		Batch     string `json:"batch"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "请求 JSON 无效"})
		return
	}
	codes, err := a.st.CreateRedeemCodes(store.NewCodes{Amount: req.Amount, Count: req.Count, ExpiresAt: req.ExpiresAt, Note: req.Note,
		Batch: req.Batch, Operator: p.Operator()})
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	batch := ""
	if len(codes) > 0 {
		batch = codes[0].Batch
	}
	writeJSON(w, 201, map[string]any{"codes": codes, "batch": batch})
}

// handleAdminPatchCode PATCH /admin/api/redeem-codes/{id} {enabled}
func (a *API) handleAdminPatchCode(w http.ResponseWriter, r *http.Request, p *Principal) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "无效 ID"})
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil || req.Enabled == nil {
		writeJSON(w, 400, map[string]any{"error": "仅支持 enabled 字段"})
		return
	}
	c, err := a.st.SetRedeemCodeEnabled(id, *req.Enabled)
	if err != nil {
		status := 400
		if errors.Is(err, store.ErrNotFound) {
			status = 404
		} else if errors.Is(err, store.ErrCodeRedeemed) {
			status = 409
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"code": c})
}

// handleAdminDeleteCode DELETE /admin/api/redeem-codes/{id}
func (a *API) handleAdminDeleteCode(w http.ResponseWriter, r *http.Request, p *Principal) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "无效 ID"})
		return
	}
	if err := a.st.DeleteRedeemCode(id); err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleAdminDisableBatch POST /admin/api/redeem-codes/batches/{batch}/disable
func (a *API) handleAdminDisableBatch(w http.ResponseWriter, r *http.Request, p *Principal) {
	n, err := a.st.DisableRedeemBatch(r.PathValue("batch"))
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"disabled": n})
}

// handleAdminExportCodes GET /admin/api/redeem-codes/export.csv?status&batch&q
func (a *API) handleAdminExportCodes(w http.ResponseWriter, r *http.Request, p *Principal) {
	f := codeFilter(r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="web2api-codes-%s.csv"`, time.Now().Format("20060102-150405")))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("\xEF\xBB\xBF"))
	out := csv.NewWriter(w)
	_ = out.Write([]string{"code", "amount", "status", "batch", "note", "expires_at", "created_at", "redeemed_email", "redeemed_at"})
	format := func(ts int64) string {
		if ts == 0 {
			return ""
		}
		return time.Unix(ts, 0).Format(time.RFC3339)
	}
	for offset := 0; offset < 100000; offset += 1000 {
		f.Limit, f.Offset = 1000, offset
		codes, _, err := a.st.ListRedeemCodes(f)
		if err != nil || len(codes) == 0 {
			break
		}
		for _, c := range codes {
			_ = out.Write([]string{c.Code, strconv.FormatInt(c.Amount, 10), c.Status, c.Batch, c.Note, format(c.ExpiresAt),
				format(c.CreatedAt), c.RedeemedEmail, format(c.RedeemedAt)})
		}
		if len(codes) < 1000 {
			break
		}
	}
	out.Flush()
}

// ---------- 平台设置 ----------

// handleAdminSettings GET /admin/api/settings
func (a *API) handleAdminSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	v, err := a.st.GetPlatformSettings()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"settings": v})
}

// handleAdminPutSettings PUT /admin/api/settings（字段可部分提供，未提供的保持原值）
func (a *API) handleAdminPutSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	current, err := a.st.GetPlatformSettings()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	var req struct {
		RegistrationOpen      *bool    `json:"registration_open"`
		SignupBonus           *int64   `json:"signup_bonus"`
		DefaultUserMultiplier *float64 `json:"default_user_multiplier"`
		MaxKeysPerUser        *int     `json:"max_keys_per_user"`
		SiteName              *string  `json:"site_name"`
		Announcement          *string  `json:"announcement"`
		VideoTokensPerSecond  *int64   `json:"video_tokens_per_second"`
		UserUnmeteredRoutes   *bool    `json:"user_unmetered_routes"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "请求 JSON 无效"})
		return
	}
	if req.RegistrationOpen != nil {
		current.RegistrationOpen = *req.RegistrationOpen
	}
	if req.SignupBonus != nil {
		current.SignupBonus = *req.SignupBonus
	}
	if req.DefaultUserMultiplier != nil {
		current.DefaultUserMultiplier = *req.DefaultUserMultiplier
	}
	if req.MaxKeysPerUser != nil {
		current.MaxKeysPerUser = *req.MaxKeysPerUser
	}
	if req.SiteName != nil {
		current.SiteName = *req.SiteName
	}
	if req.Announcement != nil {
		current.Announcement = *req.Announcement
	}
	if req.VideoTokensPerSecond != nil {
		current.VideoTokensPerSecond = *req.VideoTokensPerSecond
	}
	if req.UserUnmeteredRoutes != nil {
		current.UserUnmeteredRoutes = *req.UserUnmeteredRoutes
	}
	saved, err := a.st.SavePlatformSettings(current)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"settings": saved})
}
