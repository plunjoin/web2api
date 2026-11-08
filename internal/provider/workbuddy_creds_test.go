package provider

import (
	"testing"

	"web2api/internal/engine/workbuddy"
)

// TestMergeWorkBuddyCredsKeepsSite 管理台只提交新 token 时，站点/身份/代理
// 必须保留——否则国际服账号会被悄悄打回国服端点。
func TestMergeWorkBuddyCredsKeepsSite(t *testing.T) {
	old := workbuddy.Credentials{
		Token: "old", RefreshToken: "rt", Email: "a@b.c", Site: "workbuddy-ai",
		UID: "u1", Domain: "www.workbuddy.ai", EnterpriseID: "ent",
		BaseURL: "https://example.test", Proxy: "socks5://127.0.0.1:1080",
	}
	next := workbuddy.Credentials{Token: "new"}

	got := mergeWorkBuddyCreds(old, next)
	if got.Token != "new" {
		t.Errorf("token should be replaced, got %q", got.Token)
	}
	if got.Site != "workbuddy-ai" || got.UID != "u1" || got.Domain != "www.workbuddy.ai" ||
		got.EnterpriseID != "ent" || got.BaseURL != "https://example.test" ||
		got.Proxy != "socks5://127.0.0.1:1080" || got.RefreshToken != "rt" || got.Email != "a@b.c" {
		t.Errorf("identity fields must be preserved: %+v", got)
	}

	// 显式改站点时以新值为准。
	got = mergeWorkBuddyCreds(old, workbuddy.Credentials{Token: "new", Site: "workbuddy"})
	if got.Site != "workbuddy" {
		t.Errorf("explicit site should win, got %q", got.Site)
	}
	if got.ExpiresAt != 0 {
		t.Errorf("manual token keeps unknown expiry, got %d", got.ExpiresAt)
	}
}
