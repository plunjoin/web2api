//go:build web2api_unit

package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openPlatformStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mustUser(t *testing.T, s *Store, email string, balance int64, multiplier float64) User {
	t.Helper()
	u, err := s.CreateUser(NewUser{Email: email, PasswordHash: "x", Multiplier: multiplier, InitialBalance: balance, Operator: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func assertIntegrity(t *testing.T, s *Store) {
	t.Helper()
	bad, err := s.LedgerIntegrity()
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Fatalf("ledger 与余额不一致的用户: %v", bad)
	}
}

// 用户 Key 的请求：扣费 = ceil(total × 模型倍率 × Key 倍率 × 用户倍率)，从余额扣并写 usage 流水。
func TestUserKeyChargesBalance(t *testing.T) {
	s := openPlatformStore(t)
	u := mustUser(t, s, "Alice@Example.com", 1000, 2)
	if u.Email != "alice@example.com" || u.Balance != 1000 {
		t.Fatalf("创建用户: %+v", u)
	}
	k, err := s.CreateUserKey(u.ID, "laptop", 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	keyMultiplier := 0.5
	if _, err := s.UpdateKey(k.ID, KeyUpdate{Multiplier: &keyMultiplier}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetModelMultiplier("gemini-x", 1.5); err != nil {
		t.Fatal(err)
	}
	rec, err := s.RecordRequest(UsageRecord{KeyID: k.ID, APIKey: k.Key, Model: "gemini-x", Success: true, PromptTokens: 60, CompletionTokens: 41})
	if err != nil {
		t.Fatal(err)
	}
	// 101 × 1.5 × 0.5 × 2 = 151.5 → 152
	if rec.ChargedTokens != 152 || rec.UserID != u.ID || rec.UserMultiplier != 2 || rec.BalanceAfter != 848 {
		t.Fatalf("扣费记录: %+v", rec)
	}
	got, _ := s.GetUser(u.ID)
	if got.Balance != 848 {
		t.Fatalf("余额 = %d, want 848", got.Balance)
	}
	entries, total, _, err := s.ListLedger(LedgerFilter{UserID: u.ID})
	if err != nil || total != 2 {
		t.Fatalf("流水: %v %d", err, total)
	}
	usage := entries[0]
	if usage.Kind != LedgerUsage || usage.Amount != -152 || usage.BalanceBefore != 1000 || usage.BalanceAfter != 848 || usage.RefID != rec.ID {
		t.Fatalf("usage 流水: %+v", usage)
	}
	key, _ := s.GetKey(k.ID)
	if key.TokensUsed != 152 || key.UserID != u.ID {
		t.Fatalf("Key 已用: %+v", key)
	}
	// 失败请求不扣费、不写流水
	if _, err := s.RecordRequest(UsageRecord{KeyID: k.ID, APIKey: k.Key, Model: "gemini-x", Success: false, Error: "boom"}); err != nil {
		t.Fatal(err)
	}
	if _, total, _, _ := s.ListLedger(LedgerFilter{UserID: u.ID}); total != 2 {
		t.Fatalf("失败请求不应写流水, total=%d", total)
	}
	// 用户维度过滤
	records, n, err := s.ListUsageRecords(UsageFilter{UserID: u.ID})
	if err != nil || n != 2 || records[1].UserEmail != "alice@example.com" {
		t.Fatalf("按用户查询用量: %v %d %+v", err, n, records)
	}
	assertIntegrity(t, s)
}

// 无归属 Key（config / 管理员创建）保持原行为：不扣任何用户余额、不写流水。
func TestUnownedKeyUnchanged(t *testing.T) {
	s := openPlatformStore(t)
	u := mustUser(t, s, "bob@example.com", 500, 1)
	k, _ := s.CreateKey("admin-key")
	rec, err := s.RecordRequest(UsageRecord{KeyID: k.ID, APIKey: k.Key, Model: "m", Success: true, TotalTokens: 100})
	if err != nil || rec.ChargedTokens != 100 || rec.UserID != 0 || rec.UserMultiplier != 1 {
		t.Fatalf("无归属 Key 计费: %v %+v", err, rec)
	}
	info, _, _ := s.LookupKey(k.Key)
	if info.UserID != 0 || !info.UserEnabled || info.BalanceExhausted() {
		t.Fatalf("无归属 Key 鉴权信息: %+v", info)
	}
	got, _ := s.GetUser(u.ID)
	if got.Balance != 500 {
		t.Fatalf("其他用户余额被改动: %d", got.Balance)
	}
	if _, total, _, _ := s.ListLedger(LedgerFilter{}); total != 1 { // 只有 bob 的初始余额
		t.Fatalf("不应产生 usage 流水, total=%d", total)
	}
}

// 余额不足：入口检查 balance<=0；跨线的那次请求完整扣费（可为负），之后被拒绝。
func TestInsufficientBalance(t *testing.T) {
	s := openPlatformStore(t)
	broke := mustUser(t, s, "broke@example.com", 0, 1)
	k, _ := s.CreateUserKey(broke.ID, "", 0, 0)
	info, found, err := s.LookupKey(k.Key)
	if err != nil || !found || info.UserID != broke.ID || !info.BalanceExhausted() {
		t.Fatalf("零余额应判定不足: %+v %v", info, err)
	}
	u := mustUser(t, s, "edge@example.com", 10, 1)
	k2, _ := s.CreateUserKey(u.ID, "", 0, 0)
	if info, _, _ := s.LookupKey(k2.Key); info.BalanceExhausted() || info.Balance != 10 {
		t.Fatalf("有余额时应放行: %+v", info)
	}
	rec, err := s.RecordRequest(UsageRecord{KeyID: k2.ID, APIKey: k2.Key, Model: "m", Success: true, TotalTokens: 25})
	if err != nil || rec.BalanceAfter != -15 {
		t.Fatalf("跨线请求: %v %+v", err, rec)
	}
	if info, _, _ := s.LookupKey(k2.Key); !info.BalanceExhausted() {
		t.Fatalf("余额为负后应拒绝: %+v", info)
	}
	// 停用用户后 Key 不可用
	disabled := false
	if _, err := s.UpdateUser(u.ID, UserUpdate{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if info, _, _ := s.LookupKey(k2.Key); info.UserEnabled {
		t.Fatal("停用用户的 Key 应标记 UserEnabled=false")
	}
	assertIntegrity(t, s)
}

func TestAdminAdjustBalance(t *testing.T) {
	s := openPlatformStore(t)
	u := mustUser(t, s, "carol@example.com", 0, 1)
	entry, err := s.AdjustBalance(u.ID, 500, "线下转账 #1", "admin@example.com")
	if err != nil || entry.BalanceBefore != 0 || entry.BalanceAfter != 500 || entry.Kind != LedgerAdjust || entry.Operator != "admin@example.com" {
		t.Fatalf("加余额: %v %+v", err, entry)
	}
	if entry, err = s.AdjustBalance(u.ID, -200, "退款", "admin@example.com"); err != nil || entry.BalanceAfter != 300 {
		t.Fatalf("减余额: %v %+v", err, entry)
	}
	if _, err := s.AdjustBalance(u.ID, -301, "超扣", "admin"); !errors.Is(err, ErrBalanceWouldNegative) {
		t.Fatalf("扣成负数应拒绝: %v", err)
	}
	if _, err := s.AdjustBalance(u.ID, 0, "", "admin"); !errors.Is(err, ErrInvalidLedgerAmount) {
		t.Fatalf("0 金额应拒绝: %v", err)
	}
	if _, err := s.AdjustBalance(999, 10, "", "admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的用户: %v", err)
	}
	got, _ := s.GetUser(u.ID)
	if got.Balance != 300 {
		t.Fatalf("余额 = %d", got.Balance)
	}
	_, _, sums, _ := s.ListLedger(LedgerFilter{UserID: u.ID})
	if sums["credit"] != 500 || sums["debit"] != 200 {
		t.Fatalf("收支合计: %v", sums)
	}
	assertIntegrity(t, s)
}

func TestRedeemOnce(t *testing.T) {
	s := openPlatformStore(t)
	u := mustUser(t, s, "dave@example.com", 0, 1)
	other := mustUser(t, s, "erin@example.com", 0, 1)
	codes, err := s.CreateRedeemCodes(NewCodes{Amount: 1000, Count: 3, Note: "活动", Operator: "admin"})
	if err != nil || len(codes) != 3 || codes[0].Batch == "" || codes[0].Status != "unused" {
		t.Fatalf("生成兑换码: %v %+v", err, codes)
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if seen[c.Code] || len(c.Code) != len("W2A-XXXX-XXXX-XXXX-XXXX") {
			t.Fatalf("兑换码格式或重复: %q", c.Code)
		}
		seen[c.Code] = true
	}
	// 用户输入可带小写和空格
	res, err := s.Redeem(u.ID, "  "+lower(codes[0].Code)+" ")
	if err != nil || res.Amount != 1000 || res.Entry.BalanceAfter != 1000 || res.Entry.Kind != LedgerRedeem || res.Entry.RefID != codes[0].ID {
		t.Fatalf("兑换: %v %+v", err, res)
	}
	if _, err := s.Redeem(u.ID, codes[0].Code); !errors.Is(err, ErrCodeRedeemed) {
		t.Fatalf("同一用户二次兑换: %v", err)
	}
	if _, err := s.Redeem(other.ID, codes[0].Code); !errors.Is(err, ErrCodeRedeemed) {
		t.Fatalf("其他用户兑换已用码: %v", err)
	}
	if _, err := s.Redeem(u.ID, "W2A-NOPE-NOPE-NOPE-NOPE"); !errors.Is(err, ErrCodeNotFound) {
		t.Fatalf("不存在的码: %v", err)
	}
	if _, err := s.SetRedeemCodeEnabled(codes[1].ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Redeem(u.ID, codes[1].Code); !errors.Is(err, ErrCodeDisabled) {
		t.Fatalf("停用的码: %v", err)
	}
	if _, err := s.SetRedeemCodeEnabled(codes[0].ID, false); !errors.Is(err, ErrCodeRedeemed) {
		t.Fatalf("已兑换的码不能再修改: %v", err)
	}
	c, _ := s.GetRedeemCode(codes[0].ID)
	if c.Status != "redeemed" || c.RedeemedBy != u.ID || c.RedeemedEmail != "dave@example.com" {
		t.Fatalf("兑换后状态: %+v", c)
	}
	stats, _ := s.RedeemCodeStats()
	if stats.Total != 3 || stats.Redeemed != 1 || stats.Unused != 1 || stats.RedeemedAmount != 1000 || stats.UnusedAmount != 1000 {
		t.Fatalf("统计: %+v", stats)
	}
	if n, _ := s.DisableRedeemBatch(codes[0].Batch); n != 1 {
		t.Fatalf("停用批次应只影响 1 个未兑换启用码, got %d", n)
	}
	// 过期
	if _, err := s.CreateRedeemCodes(NewCodes{Amount: 1, Count: 1, ExpiresAt: time.Now().Add(-time.Hour).Unix()}); err == nil {
		t.Fatal("过去的过期时间应拒绝")
	}
	soon, err := s.CreateRedeemCodes(NewCodes{Amount: 5, Count: 1, ExpiresAt: time.Now().Unix() + 1})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := s.Redeem(u.ID, soon[0].Code); !errors.Is(err, ErrCodeExpired) {
		t.Fatalf("过期的码: %v", err)
	}
	history, _, _ := s.ListRedeemCodes(CodeFilter{UserID: u.ID})
	if len(history) != 1 || history[0].ID != codes[0].ID {
		t.Fatalf("兑换记录: %+v", history)
	}
	assertIntegrity(t, s)
}

// 并发兑换同一个码：只能成功一次，总入账等于面额。
func TestConcurrentRedeemSingleWinner(t *testing.T) {
	s := openPlatformStore(t)
	codes, err := s.CreateRedeemCodes(NewCodes{Amount: 777, Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	users := make([]User, 16)
	for i := range users {
		users[i] = mustUser(t, s, "racer"+string(rune('a'+i))+"@example.com", 0, 1)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, redeemedErrs := 0, 0
	for i := range users {
		for attempt := 0; attempt < 2; attempt++ { // 每个用户也并发重复提交
			wg.Add(1)
			go func(id int64) {
				defer wg.Done()
				_, err := s.Redeem(id, codes[0].Code)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					wins++
				case errors.Is(err, ErrCodeRedeemed):
					redeemedErrs++
				default:
					t.Errorf("unexpected: %v", err)
				}
			}(users[i].ID)
		}
	}
	wg.Wait()
	if wins != 1 || redeemedErrs != 31 {
		t.Fatalf("wins=%d redeemedErrs=%d", wins, redeemedErrs)
	}
	var sum int64
	for _, u := range users {
		got, _ := s.GetUser(u.ID)
		sum += got.Balance
	}
	if sum != 777 {
		t.Fatalf("总入账 = %d, want 777", sum)
	}
	if _, total, _, _ := s.ListLedger(LedgerFilter{Kind: LedgerRedeem}); total != 1 {
		t.Fatalf("redeem 流水 = %d, want 1", total)
	}
	assertIntegrity(t, s)
}

// 并发扣费：余额与流水首尾相接，合计一致。
func TestConcurrentChargesKeepLedgerConsistent(t *testing.T) {
	s := openPlatformStore(t)
	u := mustUser(t, s, "busy@example.com", 100000, 1)
	k, _ := s.CreateUserKey(u.ID, "", 0, 0)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(n int64) {
			defer wg.Done()
			if _, err := s.RecordRequest(UsageRecord{KeyID: k.ID, APIKey: k.Key, Model: "m", Success: true, TotalTokens: 10 + n}); err != nil {
				t.Error(err)
			}
		}(int64(i))
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.AdjustBalance(u.ID, 50, "并发充值", "admin"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, _ := s.GetUser(u.ID)
	// Σ(10..49) = 1180
	if want := int64(100000 - 1180 + 250); got.Balance != want {
		t.Fatalf("余额 = %d, want %d", got.Balance, want)
	}
	assertIntegrity(t, s)
}

func TestRefundUsageOnce(t *testing.T) {
	s := openPlatformStore(t)
	u := mustUser(t, s, "frank@example.com", 100, 1)
	k, _ := s.CreateUserKey(u.ID, "", 0, 0)
	rec, _ := s.RecordRequest(UsageRecord{KeyID: k.ID, APIKey: k.Key, Model: "m", Success: true, TotalTokens: 30})
	entry, err := s.RefundUsage(rec.ID, "", "admin")
	if err != nil || entry.Kind != LedgerRefund || entry.Amount != 30 || entry.BalanceAfter != 100 || entry.RefID != rec.ID {
		t.Fatalf("退款: %v %+v", err, entry)
	}
	if _, err := s.RefundUsage(rec.ID, "", "admin"); !errors.Is(err, ErrAlreadyRefunded) {
		t.Fatalf("重复退款: %v", err)
	}
	key, _ := s.GetKey(k.ID)
	if key.TokensUsed != 0 {
		t.Fatalf("退款后 Key 已用应回退: %d", key.TokensUsed)
	}
	unowned, _ := s.CreateKey("x")
	rec2, _ := s.RecordRequest(UsageRecord{KeyID: unowned.ID, APIKey: unowned.Key, Model: "m", Success: true, TotalTokens: 30})
	if _, err := s.RefundUsage(rec2.ID, "", "admin"); !errors.Is(err, ErrNothingToRefund) {
		t.Fatalf("无归属 Key 不可退款: %v", err)
	}
	assertIntegrity(t, s)
}

func TestUserLifecycleAndSettings(t *testing.T) {
	s := openPlatformStore(t)
	settings, err := s.GetPlatformSettings()
	if err != nil || settings.RegistrationOpen || settings.MaxKeysPerUser != 20 || settings.DefaultUserMultiplier != 1 {
		t.Fatalf("默认设置（注册默认关闭）: %v %+v", err, settings)
	}
	settings.RegistrationOpen, settings.SignupBonus, settings.MaxKeysPerUser, settings.VideoTokensPerSecond = true, 1000, 2, 50
	if saved, err := s.SavePlatformSettings(settings); err != nil || !saved.RegistrationOpen || saved.SignupBonus != 1000 || saved.VideoTokensPerSecond != 50 {
		t.Fatalf("保存设置: %v %+v", err, saved)
	}
	settings.MaxKeysPerUser = 0
	if _, err := s.SavePlatformSettings(settings); err == nil {
		t.Fatal("非法 Key 上限应拒绝")
	}
	u, err := s.CreateUser(NewUser{Email: "gina@example.com", PasswordHash: "x", InitialBalance: 1000, InitialKind: LedgerSignupBonus, InitialNote: "注册赠送"})
	if err != nil || u.Nickname != "gina" || u.Balance != 1000 {
		t.Fatalf("注册: %v %+v", err, u)
	}
	if _, err := s.CreateUser(NewUser{Email: "GINA@example.com", PasswordHash: "x"}); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("重复邮箱: %v", err)
	}
	if _, err := s.CreateUserKey(u.ID, "a", 0, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUserKey(u.ID, "b", 0, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUserKey(u.ID, "c", 0, 2); !errors.Is(err, ErrKeyLimitReached) {
		t.Fatalf("超过 Key 上限: %v", err)
	}
	before := u.TokenVersion
	hash := "y"
	u2, err := s.UpdateUser(u.ID, UserUpdate{PasswordHash: &hash})
	if err != nil || u2.TokenVersion != before+1 {
		t.Fatalf("改密应作废旧登录态: %v %+v", err, u2)
	}
	list, total, err := s.ListUsers(UserFilter{Query: "gina"})
	if err != nil || total != 1 || list[0].KeyCount != 2 || list[0].TotalRecharge != 1000 {
		t.Fatalf("用户列表: %v %d %+v", err, total, list)
	}
	if err := s.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	if keys, _ := s.ListUserKeys(u.ID); len(keys) != 0 {
		t.Fatalf("删除用户应删除其 Key: %d", len(keys))
	}
	if _, total, _, _ := s.ListLedger(LedgerFilter{UserID: u.ID}); total != 0 {
		t.Fatalf("删除用户应删除其流水: %d", total)
	}
	assertIntegrity(t, s)
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}
