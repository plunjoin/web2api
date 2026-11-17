package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"web2api/internal/logging"
	"web2api/internal/store"
)

func newAuditTestAPI(t *testing.T) (*API, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &API{st: st, logger: logging.NewTest(io.Discard)}, st
}

func TestAuditedMiddlewareRecordsSuccess(t *testing.T) {
	a, st := newAuditTestAPI(t)
	handler := a.audited("user.balance", func(w http.ResponseWriter, r *http.Request) {
		setAudit(r, "user:7 user@example.com", "调整余额 +10.00 元")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	r := httptest.NewRequest(http.MethodPost, "/admin/api/users/7/balance", nil)
	r = r.WithContext(withPrincipal(r.Context(), &Principal{Root: &store.AdminSettings{Email: "root@example.com"}, Email: "root@example.com"}))
	handler.ServeHTTP(httptest.NewRecorder(), r)

	logs, total, err := st.ListAuditLogs(store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("应写入 1 条审计记录，got total=%d", total)
	}
	got := logs[0]
	if got.Actor != "root@example.com" || got.ActorKind != "root" || got.Action != "user.balance" || got.Result != "ok" {
		t.Fatalf("审计字段不符: %+v", got)
	}
	if got.Target != "user:7 user@example.com" || got.Detail != "调整余额 +10.00 元" {
		t.Fatalf("目标/摘要不符: %+v", got)
	}
}

func TestAuditedMiddlewareRecordsFailure(t *testing.T) {
	a, st := newAuditTestAPI(t)
	handler := a.audited("account.delete", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "账号不存在"})
	})
	r := httptest.NewRequest(http.MethodDelete, "/admin/api/accounts/9", nil)
	r = r.WithContext(withPrincipal(r.Context(), &Principal{Root: &store.AdminSettings{Email: "root@example.com"}, Email: "root@example.com"}))
	handler.ServeHTTP(httptest.NewRecorder(), r)

	logs, _, err := st.ListAuditLogs(store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("应写入 1 条审计记录，got %d", len(logs))
	}
	if logs[0].Result != "fail" || logs[0].Error != "账号不存在" {
		t.Fatalf("失败审计应带上错误原因: %+v", logs[0])
	}
	if logs[0].Target != "DELETE /admin/api/accounts/9" {
		t.Fatalf("未补充 target 时应回退到方法+路径: %q", logs[0].Target)
	}
}

func TestLoginAuditDoesNotStorePassword(t *testing.T) {
	a, st := newAuditTestAPI(t)
	r := httptest.NewRequest(http.MethodPost, "/admin/api/auth/login", nil)
	a.auditLogin(r, "admin@example.com", "root", errAuditBadPassword)
	logs, _, err := st.ListAuditLogs(store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Action != "auth.login" || logs[0].Result != "fail" {
		t.Fatalf("登录失败审计不符: %+v", logs)
	}
	if logs[0].Actor != "admin@example.com" || logs[0].Error != "邮箱或密码错误" {
		t.Fatalf("登录审计字段不符: %+v", logs[0])
	}
}
