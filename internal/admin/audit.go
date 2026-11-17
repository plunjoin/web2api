package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"web2api/internal/store"
)

// 登录失败原因（写入审计日志，不包含密码等敏感信息）。
var (
	errAuditInvalidLogin    = errors.New("登录参数无效")
	errAuditTooManyAttempts = errors.New("登录尝试过多")
	errAuditBadPassword     = errors.New("邮箱或密码错误")
	errAuditDisabled        = errors.New("账号已停用")
)

// ==================== 请求上下文 ====================

type principalCtxKey struct{}

func withPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

func principalFromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalCtxKey{}).(*Principal)
	return p
}

// auditInfo 由具体 handler 补充的审计信息（被操作对象与可读摘要）。
type auditInfo struct {
	target string
	detail string
}

type auditInfoKey struct{}

// setAudit 在 handler 内补充审计目标与摘要；没有经过审计中间件时为空操作。
func setAudit(r *http.Request, target, detail string) {
	if info, ok := r.Context().Value(auditInfoKey{}).(*auditInfo); ok {
		info.target = target
		info.detail = detail
	}
}

// ==================== 响应状态记录 ====================

// auditStatusWriter 记录状态码与少量响应体，用于把失败原因写进审计日志。
type auditStatusWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *auditStatusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *auditStatusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len() < 2048 {
		remaining := 2048 - w.body.Len()
		if len(p) > remaining {
			w.body.Write(p[:remaining])
		} else {
			w.body.Write(p)
		}
	}
	return w.ResponseWriter.Write(p)
}

func (w *auditStatusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// ==================== 审计中间件 ====================

// audited 包装根管理员管理接口：执行后按状态码记录审计日志。
func (a *API) audited(action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := &auditInfo{}
		r = r.WithContext(context.WithValue(r.Context(), auditInfoKey{}, info))
		sw := &auditStatusWriter{ResponseWriter: w}
		next(sw, r)
		a.finishAudit(r, info, action, sw)
	}
}

// auditedAP 包装带 Principal 的管理接口（adminAuth 的签名）。
func (a *API) auditedAP(action string, next func(http.ResponseWriter, *http.Request, *Principal)) func(http.ResponseWriter, *http.Request, *Principal) {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		info := &auditInfo{}
		r = r.WithContext(context.WithValue(r.Context(), auditInfoKey{}, info))
		sw := &auditStatusWriter{ResponseWriter: w}
		next(sw, r, p)
		a.finishAudit(r, info, action, sw)
	}
}

func (a *API) finishAudit(r *http.Request, info *auditInfo, action string, sw *auditStatusWriter) {
	status := sw.status
	if status == 0 {
		status = http.StatusOK
	}
	p := principalFromContext(r.Context())
	actor, kind := "system", "system"
	if p != nil {
		actor = p.Operator()
		kind = principalKind(p)
	}
	target := info.target
	if target == "" {
		target = r.Method + " " + r.URL.Path
	}
	detail := info.detail
	if detail == "" {
		detail = "status=" + strconv.Itoa(status)
	}
	result := "ok"
	var errText string
	if status >= 400 {
		result = "fail"
		errText = auditErrorText(sw.body.Bytes())
	}
	a.recordAudit(r, actor, kind, action, target, detail, result, errText)
}

func principalKind(p *Principal) string {
	switch {
	case p == nil:
		return "system"
	case p.Root != nil:
		return "root"
	case p.User != nil && p.User.Role == store.RoleAdmin:
		return "admin"
	default:
		return "user"
	}
}

// auditErrorText 从错误响应体里取回错误文案，取不到时用原始片段兜底。
func auditErrorText(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var v struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &v) == nil && strings.TrimSpace(v.Error) != "" {
		return v.Error
	}
	return strings.TrimSpace(string(body))
}

func (a *API) recordAudit(r *http.Request, actor, kind, action, target, detail, result, errText string) {
	entry := store.AuditEntry{
		TS:        time.Now().Unix(),
		Actor:     actor,
		ActorKind: kind,
		IP:        clientIP(r),
		Action:    action,
		Target:    target,
		Detail:    detail,
		Result:    result,
		Error:     errText,
	}
	if err := a.st.RecordAudit(entry); err != nil {
		a.logger.Warnf("[audit] 写入审计日志失败 action=%s actor=%s: %v", action, actor, err)
	}
	if result == "fail" {
		a.logger.Warnf("[audit] %s 失败 actor=%s kind=%s ip=%s target=%q detail=%q: %s",
			action, actor, kind, entry.IP, target, detail, errText)
		return
	}
	a.logger.Infof("[audit] %s actor=%s kind=%s ip=%s target=%q detail=%q",
		action, actor, kind, entry.IP, target, detail)
}

// auditLogin 记录登录成功/失败。登录发生在鉴权之前，操作人取尝试登录的邮箱，
// 不记录密码。
func (a *API) auditLogin(r *http.Request, email, kind string, err error) {
	result := "ok"
	errText := ""
	if err != nil {
		result = "fail"
		errText = err.Error()
	}
	a.recordAudit(r, email, kind, "auth.login", "session", "登录："+email, result, errText)
}

// ==================== 审计查询接口 ====================

// handleAuditLogs GET /admin/api/audit-logs?days&actor&action&result&q&limit&offset
func (a *API) handleAuditLogs(w http.ResponseWriter, r *http.Request, _ *Principal) {
	days := 7
	if v := strings.TrimSpace(r.URL.Query().Get("days")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 90 {
			days = n
		}
	}
	limit := 50
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	offset := 0
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	filter := store.AuditFilter{
		Since:  time.Now().AddDate(0, 0, -days).Unix(),
		Actor:  r.URL.Query().Get("actor"),
		Action: r.URL.Query().Get("action"),
		Result: r.URL.Query().Get("result"),
		Query:  r.URL.Query().Get("q"),
		Limit:  limit,
		Offset: offset,
	}
	records, total, err := a.st.ListAuditLogs(filter)
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "records": records, "days": days})
}
