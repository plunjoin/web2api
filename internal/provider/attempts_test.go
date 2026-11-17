package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"web2api/internal/engine"
)

func TestAttemptTraceRecordsOrderAndErrors(t *testing.T) {
	ctx := WithAttemptTrace(context.Background())
	start := time.Now().Add(-5 * time.Millisecond)
	RecordAttempt(ctx, "custom", start, errors.New("upstream 500"))
	RecordAttempt(ctx, "codex", start, nil)

	got := AttemptTrace(ctx)
	if len(got) != 2 {
		t.Fatalf("应记录 2 次尝试，got %+v", got)
	}
	if got[0].Engine != "custom" || got[0].Error == "" || got[0].DurationMs < 0 {
		t.Fatalf("第一次尝试字段不符: %+v", got[0])
	}
	if got[1].Engine != "codex" || got[1].Error != "" {
		t.Fatalf("第二次尝试应为成功: %+v", got[1])
	}
	if AttemptTrace(context.Background()) != nil {
		t.Fatal("未挂载轨迹的 ctx 不应产生记录")
	}
}

func TestAttemptTraceCapsLength(t *testing.T) {
	ctx := WithAttemptTrace(context.Background())
	for i := 0; i < maxAttempts+5; i++ {
		RecordAttempt(ctx, "e", time.Time{}, errors.New("x"))
	}
	if got := AttemptTrace(ctx); len(got) != maxAttempts {
		t.Fatalf("轨迹应封顶 %d 条，got %d", maxAttempts, len(got))
	}
}

func TestErrorKind(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{engine.NewModelNotFound("x", "d"), "model_not_found"},
		{engine.NewModelDisabled("x"), "model_unavailable"},
		{engine.NewUnsupportedEndpoint("x", "video"), "unsupported_endpoint"},
		{engine.NewToolsNotSupported("x", "geminiweb"), "tools_not_supported"},
		{context.DeadlineExceeded, "upstream_timeout"},
		{errors.New("upstream 429 Too Many Requests"), "upstream_rate_limited"},
		{errors.New("401 Unauthorized"), "upstream_auth"},
		{errors.New("引擎 custom 不可用"), "engine_unavailable"},
		{errors.New("something odd"), "upstream_error"},
	}
	for _, tc := range cases {
		if got := ErrorKind(tc.err); got != tc.want {
			t.Fatalf("ErrorKind(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
	if !strings.Contains(truncate(strings.Repeat("x", 400), 300), "…") {
		t.Fatal("truncate 应追加省略号")
	}
}
