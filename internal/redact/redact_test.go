package redact_test

import (
	"errors"
	"testing"

	"web2api/internal/redact"
)

func TestText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "email in Chinese context",
			in:   "上游账号 alice@example.com 登录失败",
			want: "上游账号 [REDACTED_EMAIL] 登录失败",
		},
		{
			name: "sk key in Chinese context",
			in:   "凭据sk-proj-AbC123_xyz已失效",
			want: "凭据[REDACTED_API_KEY]已失效",
		},
		{
			name: "bearer token in Chinese context",
			in:   "请求头 Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig 无效",
			want: "请求头 Authorization: Bearer [REDACTED_BEARER_TOKEN] 无效",
		},
		{
			name: "multiple sensitive fragments",
			in:   "账号 bob@example.com，密钥 sk-live_123456，头 Bearer abc.def.ghi",
			want: "账号 [REDACTED_EMAIL]，密钥 [REDACTED_API_KEY]，头 Bearer [REDACTED_BEARER_TOKEN]",
		},
		{
			name: "no sensitive information",
			in:   "模型 gemini-2.5-pro 未启用，请稍后重试",
			want: "模型 gemini-2.5-pro 未启用，请稍后重试",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redact.Text(tt.in); got != tt.want {
				t.Fatalf("Text(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestError(t *testing.T) {
	err := errors.New("账号 alice@example.com 的 Bearer abc.def 无法使用")
	want := "账号 [REDACTED_EMAIL] 的 Bearer [REDACTED_BEARER_TOKEN] 无法使用"
	if got := redact.Error(err); got != want {
		t.Fatalf("Error(%v) = %q, want %q", err, got, want)
	}
	if got := redact.Error(nil); got != "" {
		t.Fatalf("Error(nil) = %q, want empty string", got)
	}
}
