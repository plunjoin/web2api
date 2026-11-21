package admin

import (
	"strings"
	"testing"
	"unicode/utf8"

	"web2api/internal/store"
)

func TestUserSafeRecordRedactsAndTruncates(t *testing.T) {
	rec := store.UsageRecord{
		Error:        "账号 alice@example.com，密钥 sk-live_123456，头 Bearer abc.def 失败：" + strings.Repeat("中文😀", 100),
		UserEmail:    "owner@example.com",
		AccountID:    7,
		AccountLabel: "slot-7",
		Provider:     "upstream",
		Attempts:     `[{"error":"secret"}]`,
	}
	got := userSafeRecord(rec)
	for _, secret := range []string{"alice@example.com", "sk-live_123456", "abc.def"} {
		if strings.Contains(got.Error, secret) {
			t.Fatalf("错误信息仍包含敏感片段 %q: %q", secret, got.Error)
		}
	}
	for _, placeholder := range []string{"[REDACTED_EMAIL]", "[REDACTED_API_KEY]", "[REDACTED_BEARER_TOKEN]"} {
		if !strings.Contains(got.Error, placeholder) {
			t.Fatalf("错误信息缺少占位符 %q: %q", placeholder, got.Error)
		}
	}
	if !utf8.ValidString(got.Error) || !strings.HasSuffix(got.Error, "…") {
		t.Fatalf("错误信息应按 UTF-8 安全截断: %q", got.Error)
	}
	if got.UserEmail != "" || got.AccountID != 0 || got.AccountLabel != "" || got.Provider != "" || got.Attempts != "" {
		t.Fatalf("用量记录仍包含内部字段: %+v", got)
	}
}
