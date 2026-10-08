//go:build web2api_unit

package store

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestChargedTokensMultiplierMath(t *testing.T) {
	for _, c := range []struct {
		total      int64
		multiplier float64
		want       int64
	}{
		{100, 1, 100},
		{100, 1.1, 110}, // 100×1.1 = 110.00000000000001 不能进位成 111
		{3, 1.5, 5},     // 4.5 向上取整
		{7, 0.1, 1},     // 0.7 向上取整，非零用量至少扣 1
		{10, 0.25, 3},   // 2.5 → 3
		{1000, 0.3, 300},
		{1234, 2, 2468},
		{100, 0, 0}, // 倍率 0：不计费
		{0, 5, 0},
		{-5, 2, 0},
		{1 << 40, 1.5, (1 << 40) * 3 / 2},
	} {
		if got := ChargedTokens(c.total, c.multiplier); got != c.want {
			t.Errorf("ChargedTokens(%d, %v) = %d, want %d", c.total, c.multiplier, got, c.want)
		}
	}
	if got := EffectiveMultiplier(1.5, 2); got != 3 {
		t.Fatalf("EffectiveMultiplier = %v", got)
	}
	if got := ChargedTokens(100, EffectiveMultiplier(1.1, 0.5)); got != 55 {
		t.Fatalf("100×1.1×0.5 charged = %d, want 55", got)
	}
}

func TestValidateMultiplierAndQuotaExceeded(t *testing.T) {
	for _, bad := range []float64{-0.1, math.NaN(), math.Inf(1), MaxMultiplier + 1} {
		if ValidateMultiplier(bad) == nil {
			t.Errorf("ValidateMultiplier(%v) 应失败", bad)
		}
	}
	for _, ok := range []float64{0, 0.5, 1, MaxMultiplier} {
		if err := ValidateMultiplier(ok); err != nil {
			t.Errorf("ValidateMultiplier(%v) = %v", ok, err)
		}
	}
	for _, c := range []struct {
		limit, used int64
		want        bool
	}{{0, 1 << 50, false}, {100, 99, false}, {100, 100, true}, {100, 150, true}} {
		if got := QuotaExceeded(c.limit, c.used); got != c.want {
			t.Errorf("QuotaExceeded(%d, %d) = %v", c.limit, c.used, got)
		}
	}
}

func TestModelMultiplierFallback(t *testing.T) {
	s := newTestStore(t)
	if got, _ := s.ModelMultiplierFor("gemini-x"); got != 1 {
		t.Fatalf("未配置倍率应为 1，得到 %v", got)
	}
	if _, err := s.SetModelMultiplier("*", 0.5); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ModelMultiplierFor("gemini-x"); got != 0.5 {
		t.Fatalf("应回落到 * 默认倍率 0.5，得到 %v", got)
	}
	if _, err := s.SetModelMultiplier("gemini-x", 3); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ModelMultiplierFor("gemini-x"); got != 3 {
		t.Fatalf("精确匹配应优先，得到 %v", got)
	}
	if _, err := s.SetModelMultiplier("gemini-x", 4); err != nil { // upsert
		t.Fatal(err)
	}
	if got, _ := s.ModelMultiplierFor("gemini-x"); got != 4 {
		t.Fatalf("upsert 后应为 4，得到 %v", got)
	}
	if _, err := s.SetModelMultiplier("", 1); err == nil {
		t.Fatal("空 model 应拒绝")
	}
	if _, err := s.SetModelMultiplier("m", -1); err == nil {
		t.Fatal("负倍率应拒绝")
	}
	if err := s.DeleteModelMultiplier("gemini-x"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteModelMultiplier("gemini-x"); err != ErrNotFound {
		t.Fatalf("重复删除应 ErrNotFound，得到 %v", err)
	}
	list, err := s.ListModelMultipliers()
	if err != nil || len(list) != 1 || list[0].Model != "*" {
		t.Fatalf("list = %+v %v", list, err)
	}
}

func TestRecordRequestChargesQuotaWithMultipliers(t *testing.T) {
	s := newTestStore(t)
	k, err := s.CreateKey("quota")
	if err != nil {
		t.Fatal(err)
	}
	limit, keyMult := int64(500), 2.0
	if k, err = s.UpdateKey(k.ID, KeyUpdate{TokenLimit: &limit, Multiplier: &keyMult}); err != nil {
		t.Fatal(err)
	}
	if k.TokensRemaining == nil || *k.TokensRemaining != 500 || k.QuotaExhausted {
		t.Fatalf("初始额度视图错误: %+v", k)
	}
	if _, err := s.SetModelMultiplier("gemini-pro", 1.5); err != nil {
		t.Fatal(err)
	}

	// 成功请求：100 tokens × 1.5 × 2 = 300
	rec, err := s.RecordRequest(UsageRecord{APIKey: k.Key, Model: "gemini-pro", Engine: "b", Success: true,
		PromptTokens: 60, CompletionTokens: 40, TotalTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if rec.KeyID != k.ID || rec.ModelMultiplier != 1.5 || rec.KeyMultiplier != 2 || rec.Multiplier != 3 || rec.ChargedTokens != 300 {
		t.Fatalf("记录倍率/计费错误: %+v", rec)
	}
	// 失败请求：只记录，不扣额度
	if rec, err = s.RecordRequest(UsageRecord{APIKey: k.Key, Model: "gemini-pro", Success: false, Error: "boom"}); err != nil || rec.ChargedTokens != 0 {
		t.Fatalf("失败请求不应计费: %+v %v", rec, err)
	}
	info, found, err := s.LookupKey(k.Key)
	if err != nil || !found || info.TokensUsed != 300 || info.Exhausted() {
		t.Fatalf("扣减后: %+v found=%v err=%v", info, found, err)
	}
	// 估算请求：未配置模型 → 倍率 1 × Key 2；250 tokens → 500，跨过额度线
	if rec, err = s.RecordRequest(UsageRecord{APIKey: k.Key, Model: "other", Success: true, PromptTokens: 200, CompletionTokens: 50, Estimated: true}); err != nil {
		t.Fatal(err)
	}
	if rec.TotalTokens != 250 || rec.ChargedTokens != 500 || !rec.Estimated {
		t.Fatalf("估算记录错误: %+v", rec)
	}
	info, _, _ = s.LookupKey(k.Key)
	if info.TokensUsed != 800 || !info.Exhausted() {
		t.Fatalf("应已用尽额度: %+v", info)
	}
	got, _ := s.GetKey(k.ID)
	if !got.QuotaExhausted || got.TokensRemaining == nil || *got.TokensRemaining != 0 {
		t.Fatalf("Key 视图应显示已用尽: %+v", got)
	}

	// 明细与聚合
	records, total, err := s.ListUsageRecords(UsageFilter{KeyID: k.ID})
	if err != nil || total != 3 || len(records) != 3 {
		t.Fatalf("records = %d/%d %v", len(records), total, err)
	}
	if records[0].KeyName != "quota" || records[0].KeyMasked == k.Key || records[0].Model != "other" {
		t.Fatalf("最新一条明细错误: %+v", records[0])
	}
	breakdown, err := s.UsageBreakdown(UsageFilter{})
	if err != nil || len(breakdown) != 2 {
		t.Fatalf("breakdown = %+v %v", breakdown, err)
	}
	var charged, estimated int64
	for _, row := range breakdown {
		charged += row.ChargedTokens
		estimated += row.EstimatedRequests
	}
	if charged != 800 || estimated != 1 {
		t.Fatalf("breakdown 合计 charged=%d estimated=%d", charged, estimated)
	}
	// 旧聚合表保持兼容（按 key+engine+model 分组；失败请求 engine 为空单独一行）
	if summary, _ := s.UsageSummary(0); len(summary) != 3 {
		t.Fatalf("usage_log 聚合应有 3 行: %d", len(summary))
	}

	// 重置已用 / 提高额度后恢复可用
	if got, err = s.UpdateKey(k.ID, KeyUpdate{ResetUsage: true}); err != nil || got.TokensUsed != 0 || got.QuotaExhausted {
		t.Fatalf("重置后: %+v %v", got, err)
	}
	unlimited := int64(0)
	if got, err = s.UpdateKey(k.ID, KeyUpdate{TokenLimit: &unlimited}); err != nil || got.TokensRemaining != nil {
		t.Fatalf("不限额时 tokens_remaining 应为 null: %+v %v", got, err)
	}
	negative := int64(-1)
	if _, err := s.UpdateKey(k.ID, KeyUpdate{TokenLimit: &negative}); err == nil {
		t.Fatal("负额度应拒绝")
	}
	if _, err := s.UpdateKey(9999, KeyUpdate{ResetUsage: true}); err != ErrNotFound {
		t.Fatalf("不存在的 Key 应 ErrNotFound: %v", err)
	}
}

func TestRecordRequestAnonymousIsNotCharged(t *testing.T) {
	s := newTestStore(t)
	rec, err := s.RecordRequest(UsageRecord{APIKey: "anonymous", Model: "m", Success: true, TotalTokens: 10})
	if err != nil || rec.KeyID != 0 || rec.ChargedTokens != 10 {
		t.Fatalf("anonymous 记录: %+v %v", rec, err)
	}
	if keys, _ := s.ListKeys(); len(keys) != 0 {
		t.Fatal("不应凭空创建 Key")
	}
}

// 旧库（无额度列）打开后自动补列，旧 Key 默认不限额、倍率 1。
func TestMigrationAddsQuotaColumnsToExistingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE api_keys (id INTEGER PRIMARY KEY AUTOINCREMENT, key TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1, created_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO api_keys (key, name, enabled, created_at) VALUES ('sk-old', 'old', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	keys, err := s.ListKeys()
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys = %+v %v", keys, err)
	}
	if k := keys[0]; k.TokenLimit != 0 || k.TokensUsed != 0 || k.Multiplier != 1 || k.TokensRemaining != nil {
		t.Fatalf("旧 Key 默认值错误: %+v", k)
	}
	// 再次打开不重复加列
	_ = s.Close()
	if s, err = Open(path); err != nil {
		t.Fatal(err)
	}
}
