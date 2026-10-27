package gateway

import (
	"testing"

	"web2api/internal/store"
)

func TestKeyUsageReport(t *testing.T) {
	now := int64(1_800_000_000)
	cases := []struct {
		name          string
		info          store.KeyAuth
		wantStatus    string
		wantRemaining int64
	}{
		{"unlimited", store.KeyAuth{Enabled: true, TokensUsed: 120}, "active", -1},
		{"limited", store.KeyAuth{Enabled: true, TokenLimit: 1000, TokensUsed: 250}, "active", 750},
		{"exhausted", store.KeyAuth{Enabled: true, TokenLimit: 1000, TokensUsed: 1200}, "quota_exhausted", 0},
		{"expired", store.KeyAuth{Enabled: true, ExpiresAt: now - 1}, "expired", -1},
		{"disabled", store.KeyAuth{Enabled: false}, "disabled", -1},
		{"user balance tighter", store.KeyAuth{Enabled: true, TokenLimit: 1000, TokensUsed: 100, UserID: 7, UserEnabled: true, Balance: 300}, "active", 300},
		{"user balance only", store.KeyAuth{Enabled: true, UserID: 7, UserEnabled: true, Balance: 5000}, "active", 5000},
		{"user broke", store.KeyAuth{Enabled: true, UserID: 7, UserEnabled: true, Balance: 0}, "insufficient_balance", 0},
		{"user disabled", store.KeyAuth{Enabled: true, UserID: 7, UserEnabled: false, Balance: 10}, "account_disabled", 10},
	}
	for _, c := range cases {
		got := keyUsageReport(c.info, "", now)
		if got["status"] != c.wantStatus || got["remaining"] != c.wantRemaining {
			t.Errorf("%s: status=%v remaining=%v, want %s %d", c.name, got["status"], got["remaining"], c.wantStatus, c.wantRemaining)
		}
		if got["isValid"] != (c.wantStatus == "active") {
			t.Errorf("%s: isValid=%v", c.name, got["isValid"])
		}
		if got["unit"] != "tokens" || got["planName"] != "web2api" {
			t.Errorf("%s: unit/planName = %v/%v", c.name, got["unit"], got["planName"])
		}
	}
	if got := keyUsageReport(store.KeyAuth{Enabled: true}, " team-a ", now); got["planName"] != "team-a" {
		t.Errorf("planName from key name = %v", got["planName"])
	}
}
