package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAccountCRUD(t *testing.T) {
	s := newTestStore(t)

	acc, err := s.CreateAccount("a", "主号", `{"psid":"x"}`, true)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Status != "initializing" {
		t.Fatalf("新账号状态应为 initializing: %s", acc.Status)
	}

	// 重复标签应失败（唯一索引）
	if _, err := s.CreateAccount("a", "主号", "{}", true); err == nil {
		t.Fatal("重复标签应报错")
	}

	got, err := s.GetAccount(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Label != "主号" || got.Credentials != `{"psid":"x"}` {
		t.Fatalf("读取不符: %+v", got)
	}

	if err := s.UpdateAccountStatus(acc.ID, "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountEnabled(acc.ID, false); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetAccount(acc.ID)
	if got.Enabled || got.Status != "disabled" {
		t.Fatalf("停用后状态不符: %+v", got)
	}

	list, err := s.ListAccounts()
	if err != nil || len(list) != 1 {
		t.Fatalf("列表不符: %v %v", list, err)
	}

	if err := s.DeleteAccount(acc.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccount(acc.ID); err == nil {
		t.Fatal("重复删除应报 ErrNotFound")
	}
}

func TestAPIKeyCRUD(t *testing.T) {
	s := newTestStore(t)

	key, err := s.CreateKey("测试")
	if err != nil {
		t.Fatal(err)
	}
	if len(key.Key) < 20 || key.Key[:3] != "sk-" {
		t.Fatalf("Key 格式异常: %q", key.Key)
	}

	ok, err := s.KeyEnabled(key.Key)
	if err != nil || !ok {
		t.Fatalf("Key 应有效: %v %v", ok, err)
	}
	ok, _ = s.KeyEnabled("sk-not-exist")
	if ok {
		t.Fatal("不存在的 Key 不应有效")
	}

	if err := s.SetKeyEnabled(key.ID, false); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.KeyEnabled(key.Key); ok {
		t.Fatal("禁用后 Key 不应有效")
	}

	if err := s.DeleteKey(key.ID); err != nil {
		t.Fatal(err)
	}
}

func TestUsageRecordAndSummary(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()

	if err := s.RecordUsage("sk-1", "a", "gemini-flash", 100, 50, true); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsage("sk-1", "a", "gemini-flash", 30, 20, false); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsage("sk-1", "b", "veo-3", 10, 0, true); err != nil {
		t.Fatal(err)
	}

	rows, err := s.UsageSummary(now - 3600)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("应聚合为 2 行: %v", rows)
	}
	var totalReq, totalPrompt int64
	for _, r := range rows {
		totalReq += r.Requests
		totalPrompt += r.PromptTokens
	}
	if totalReq != 3 || totalPrompt != 140 {
		t.Fatalf("聚合不符: req=%d prompt=%d", totalReq, totalPrompt)
	}

	// 时间窗口外
	if rows, _ := s.UsageSummary(now + 100000); len(rows) != 0 {
		t.Fatalf("未来窗口不应有数据: %v", rows)
	}
}
