//go:build web2api_unit

package store

import (
	"testing"
	"time"
)

func TestModelAllowlistMatching(t *testing.T) {
	for _, c := range []struct {
		allowed []string
		model   string
		want    bool
	}{
		{nil, "anything", true},
		{[]string{}, "anything", true},
		{[]string{"gemini-3.5-flash-lite"}, "gemini-3.5-flash-lite", true},
		{[]string{"gemini-3.5-flash-lite"}, "GEMINI-3.5-FLASH-LITE", true},
		{[]string{"gemini-3.5-flash-lite"}, "gemini-3.5-flash", false},
		{[]string{"gemini-3.5-*"}, "gemini-3.5-pro", true},
		{[]string{"gemini-3.5-*"}, "gemini-3.6-pro", false},
		{[]string{"veo-*", "imagen-4"}, "veo-3.1-generate-preview", true},
		{[]string{"veo-*", "imagen-4"}, "imagen-4-ultra", false},
		{[]string{"*"}, "x", true},
		{[]string{"gemini-pro"}, "", false},
	} {
		if got := ModelAllowed(c.allowed, c.model); got != c.want {
			t.Errorf("ModelAllowed(%v, %q) = %v, want %v", c.allowed, c.model, got, c.want)
		}
	}
	got := NormalizeModels([]string{" gemini-pro ", "", "GEMINI-PRO", "models/veo-3", "veo-3"})
	if len(got) != 2 || got[0] != "gemini-pro" || got[1] != "veo-3" {
		t.Fatalf("NormalizeModels 去重/去空/去 models/ 前缀失败: %v", got)
	}
}

func TestKeyExpiryAllowlistRPMPersisted(t *testing.T) {
	s := newTestStore(t)
	k, err := s.CreateKey("ext")
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour).Unix()
	models := []string{"gemini-3.5-*", "veo-3.1-generate-preview"}
	rpm := int64(30)
	if k, err = s.UpdateKey(k.ID, KeyUpdate{ExpiresAt: &past, AllowedModels: &models, RPMLimit: &rpm}); err != nil {
		t.Fatal(err)
	}
	if !k.Expired || k.ExpiresAt != past || len(k.AllowedModels) != 2 || k.RPMLimit != 30 {
		t.Fatalf("Key 视图错误: %+v", k)
	}
	info, ok, err := s.LookupKey(k.Key)
	if err != nil || !ok {
		t.Fatalf("LookupKey: %v %v", ok, err)
	}
	if !info.Expired(time.Now().Unix()) || info.RPMLimit != 30 || !info.ModelAllowed("gemini-3.5-flash-lite") || info.ModelAllowed("gemini-2.0") {
		t.Fatalf("KeyAuth 错误: %+v", info)
	}
	// 0 清除过期时间；空数组清除白名单
	zero, empty := int64(0), []string{}
	if k, err = s.UpdateKey(k.ID, KeyUpdate{ExpiresAt: &zero, AllowedModels: &empty}); err != nil {
		t.Fatal(err)
	}
	if k.Expired || k.ExpiresAt != 0 || len(k.AllowedModels) != 0 {
		t.Fatalf("清除失败: %+v", k)
	}
	// 校验
	neg := int64(-1)
	if _, err := s.UpdateKey(k.ID, KeyUpdate{RPMLimit: &neg}); err == nil {
		t.Fatal("负 rpm_limit 应被拒")
	}
	if _, err := s.UpdateKey(k.ID, KeyUpdate{ExpiresAt: &neg}); err == nil {
		t.Fatal("负 expires_at 应被拒")
	}
}

func TestRegenerateKeyKeepsSettingsAndUsage(t *testing.T) {
	s := newTestStore(t)
	k, err := s.CreateKey("regen")
	if err != nil {
		t.Fatal(err)
	}
	limit := int64(1000)
	if _, err := s.UpdateKey(k.ID, KeyUpdate{TokenLimit: &limit}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordRequest(UsageRecord{APIKey: k.Key, Model: "m", Success: true, TotalTokens: 10}); err != nil {
		t.Fatal(err)
	}
	n, err := s.RegenerateKey(k.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n.Key == k.Key || n.ID != k.ID || n.TokenLimit != 1000 || n.TokensUsed != 10 {
		t.Fatalf("重新生成后应换密钥、保留设置与已用额度: %+v", n)
	}
	if _, ok, _ := s.LookupKey(k.Key); ok {
		t.Fatal("旧密钥应立即失效")
	}
	// 以旧密钥字符串 + Key ID 记账（请求进行中被重新生成）仍扣到同一个 Key
	if _, err := s.RecordRequest(UsageRecord{APIKey: k.Key, KeyID: k.ID, Model: "m", Success: true, TotalTokens: 5}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetKey(k.ID); got.TokensUsed != 15 {
		t.Fatalf("按 Key ID 记账失败: used=%d", got.TokensUsed)
	}
	if _, err := s.RegenerateKey(99999); err != ErrNotFound {
		t.Fatalf("不存在的 Key 应 ErrNotFound: %v", err)
	}
}

func TestUsageTimeseriesBucketsAndLatency(t *testing.T) {
	s := newTestStore(t)
	k, _ := s.CreateKey("ts")
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Unix()
	for _, r := range []UsageRecord{
		{TS: base + 60, TotalTokens: 10, Success: true, LatencyMs: 100},
		{TS: base + 120, TotalTokens: 20, Success: true, LatencyMs: 300},
		{TS: base + 3600*2 + 5, TotalTokens: 5, Success: false, LatencyMs: 9000},
	} {
		r.APIKey, r.Model = k.Key, "m"
		if _, err := s.RecordRequest(r); err != nil {
			t.Fatal(err)
		}
	}
	points, err := s.UsageTimeseries(UsageFilter{Since: base}, 3600, 0, base+3600*3)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 4 {
		t.Fatalf("应补齐 4 个小时桶: %d %+v", len(points), points)
	}
	p0 := points[0]
	if p0.TS != base || p0.Requests != 2 || p0.SuccessRequests != 2 || p0.TotalTokens != 30 || p0.ChargedTokens != 30 || p0.AvgLatencyMs != 200 {
		t.Fatalf("第 1 个桶错误: %+v", p0)
	}
	if points[1].Requests != 0 || points[2].Requests != 1 || points[2].SuccessRequests != 0 || points[2].ChargedTokens != 0 || points[2].AvgLatencyMs != 0 {
		t.Fatalf("空桶/失败桶错误: %+v", points)
	}
	// 本地时区偏移 +8h：日桶按本地零点切分
	// base 为 UTC 零点 = 本地 08:00，所有记录同属本地 10-01；查询到本地次日 01:00 应得两个日桶
	days, err := s.UsageTimeseries(UsageFilter{Since: base}, 86400, 8*3600, base+17*3600)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[0].TS != base-8*3600 || days[0].Requests != 3 || days[1].TS != base+16*3600 || days[1].Requests != 0 {
		t.Fatalf("本地日桶切分错误: %+v", days)
	}
	// Since=0（不限时间）不应从 1970 年开始补齐
	all, err := s.UsageTimeseries(UsageFilter{}, 86400, 0, base+3600*3)
	if err != nil || len(all) != 1 {
		t.Fatalf("不限时间应只返回有数据的范围: %d %v", len(all), err)
	}
	rows, err := s.UsageBreakdown(UsageFilter{Since: base})
	if err != nil || len(rows) != 1 || rows[0].AvgLatencyMs != 200 {
		t.Fatalf("breakdown 平均延迟(仅成功请求)错误: %+v %v", rows, err)
	}
}
