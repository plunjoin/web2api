package store

import (
	"path/filepath"
	"testing"
)

func TestAuditLogRecordAndFilter(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	entries := []AuditEntry{
		{Actor: "admin@example.com", ActorKind: "root", IP: "127.0.0.1", Action: "user.balance", Target: "user:2 a@b.c", Detail: "调整余额 +10.00 元", Result: "ok"},
		{Actor: "admin@example.com", ActorKind: "root", IP: "127.0.0.1", Action: "user.delete", Target: "user:3", Result: "fail", Error: "不能删除自己"},
		{Actor: "u@example.com", ActorKind: "user", Action: "auth.login", Target: "session", Result: "ok"},
	}
	for _, e := range entries {
		if err := st.RecordAudit(e); err != nil {
			t.Fatal(err)
		}
	}

	all, total, err := st.ListAuditLogs(AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("应为 3 条，got total=%d len=%d", total, len(all))
	}
	if all[0].Action != "auth.login" {
		t.Fatalf("应按新→旧排序，首条 got %q", all[0].Action)
	}

	failed, total, err := st.ListAuditLogs(AuditFilter{Result: "fail", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(failed) != 1 || failed[0].Error != "不能删除自己" {
		t.Fatalf("按结果筛选失败: total=%d entries=%+v", total, failed)
	}

	byPrefix, total, err := st.ListAuditLogs(AuditFilter{Action: "user.", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(byPrefix) != 2 {
		t.Fatalf("按 action 前缀筛选失败: total=%d entries=%+v", total, byPrefix)
	}

	byKeyword, total, err := st.ListAuditLogs(AuditFilter{Query: "10.00", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(byKeyword) != 1 {
		t.Fatalf("关键词搜索失败: total=%d entries=%+v", total, byKeyword)
	}

	_, total, err = st.ListAuditLogs(AuditFilter{Actor: "admin", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("按操作人筛选失败: total=%d", total)
	}
}
