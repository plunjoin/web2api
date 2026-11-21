package store

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestTruncateAuditUTF8(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"未截断", "审计😀", 20, "审计😀"},
		{"中文边界", "审计审计", 6, "审计…"},
		{"中文中间", "审计审计", 7, "审计…"},
		{"emoji中间", "😀😀", 5, "😀…"},
		{"不足一个字符", "中文", 2, "…"},
		{"零上限", "中文", 0, "…"},
		{"负上限", "中文", -1, "…"},
		{"ASCII", strings.Repeat("x", 400), 300, strings.Repeat("x", 300) + "…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateAudit(tc.in, tc.n)
			if got != tc.want {
				t.Fatalf("truncateAudit(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("截断结果必须是合法 UTF-8: %q", got)
			}
		})
	}
}
