package gateway

import (
	"net/http"
	"strings"
	"time"

	"web2api/internal/store"
)

// GET /v1/usage：API Key 自查额度（CC Switch「用量查询」用）。
//
// 鉴权只认 Key 本身（Authorization: Bearer sk-...），不经过 withAuth：
// 额度用尽、已过期的 Key 也要能查到自己的状态，而不是只拿到 429/401。
// 不计 RPM、不记用量。
//
// 返回结构兼容 Sub2API 的 /v1/usage，CC Switch 内置的「Sub2API」模板可以
// 直接解析：isValid / planName / remaining（不限额为 -1）/ unit /
// quota{limit,used,remaining,unit} / status。单位是网关计费 Token
// （= 实际 Token × 模型倍率 × Key 倍率，与管理台口径一致）。
func (s *Server) handleKeyUsage(w http.ResponseWriter, r *http.Request) {
	key := bearerKey(r)
	if key == "" {
		key = strings.TrimSpace(r.Header.Get("x-api-key"))
	}
	if key == "" {
		writeError(w, http.StatusUnauthorized, "Invalid API key", "authentication_error", nil)
		return
	}
	info, found, err := s.st.LookupKey(key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "校验 Key 失败: "+err.Error(), "api_error", nil)
		return
	}
	if !found {
		writeError(w, http.StatusUnauthorized, "Invalid API key", "authentication_error", nil)
		return
	}
	name := ""
	if k, err := s.st.GetKey(info.ID); err == nil {
		name = k.Name
	}
	writeJSON(w, http.StatusOK, keyUsageReport(info, name, time.Now().Unix()))
}

// keyUsageReport 计算 Key 的额度报告。
//
// 剩余额度取「Key 自身额度剩余」与「归属用户余额」中更紧的一个；两者都不限时
// remaining = -1（Sub2API 约定：不限量，不当作余额显示）。
func keyUsageReport(info store.KeyAuth, name string, nowUnix int64) map[string]any {
	const unit = "tokens"
	status := "active"
	switch {
	case !info.Enabled:
		status = "disabled"
	case info.UserID > 0 && !info.UserEnabled:
		status = "account_disabled"
	case info.Expired(nowUnix):
		status = "expired"
	case info.Exhausted():
		status = "quota_exhausted"
	case info.BalanceExhausted():
		status = "insufficient_balance"
	}

	remaining := int64(-1)
	quota := map[string]any{"limit": info.TokenLimit, "used": info.TokensUsed, "unit": unit}
	if info.TokenLimit > 0 {
		left := info.TokenLimit - info.TokensUsed
		if left < 0 {
			left = 0
		}
		remaining = left
		quota["remaining"] = left
	} else {
		quota["remaining"] = nil
	}
	if info.UserID > 0 {
		balance := info.Balance
		if balance < 0 {
			balance = 0
		}
		if remaining < 0 || balance < remaining {
			remaining = balance
		}
	}

	plan := strings.TrimSpace(name)
	if plan == "" {
		plan = "web2api"
	}
	out := map[string]any{
		"object":    "usage",
		"isValid":   status == "active",
		"status":    status,
		"planName":  plan,
		"remaining": remaining,
		"used":      info.TokensUsed,
		"unit":      unit,
		"quota":     quota,
	}
	if info.UserID > 0 {
		out["balance"] = info.Balance
	}
	if info.ExpiresAt > 0 {
		out["expires_at"] = info.ExpiresAt
	}
	return out
}
