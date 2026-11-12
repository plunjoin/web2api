package workbuddy

import (
	"testing"
	"time"
)

func TestWindowReset(t *testing.T) {
	cn := "429 WorkBuddy: 您的使用量已超出频率限制，将在 2026-11-02 19:17:02 UTC+8 重置，您也可以切换其他模型继续使用。"
	got := windowReset(cn)
	want := time.Date(2026, 11, 2, 11, 17, 2, 0, time.UTC)
	if got.IsZero() || !got.Equal(want) {
		t.Errorf("windowReset(cn) = %v, want %v", got, want)
	}

	en := "429 usage exceeds frequency limit, but don't worry, your usage will reset at 2026-10-20 19:47:41 UTC+8, alternatively, you can switch to the other models to continue using it."
	got = windowReset(en)
	want = time.Date(2026, 10, 20, 11, 47, 41, 0, time.UTC)
	if got.IsZero() || !got.Equal(want) {
		t.Errorf("windowReset(en) = %v, want %v", got, want)
	}

	if !windowReset("no reset here").IsZero() {
		t.Errorf("windowReset should be zero for text without reset")
	}
}

func TestDsWindowEstimate(t *testing.T) {
	spec := dsSpec(SiteCN) // cap 2e8
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	rows := []dsRow{
		{at: now.Add(-2 * time.Hour), tokens: 5e6},
		{at: now.Add(-1 * time.Hour), tokens: 1e7},
	}
	w := dsWindow(rows, time.Time{}, now, spec)
	if w == nil {
		t.Fatal("expected a window")
	}
	if !w.Aside {
		t.Errorf("estimated window should be aside")
	}
	if w.Limit != 2e8 || w.Amount != 1.5e7 {
		t.Errorf("window = %+v", w)
	}
	if w.Name != "deepseek-v4.1-flash tokens (est.)" {
		t.Errorf("name = %q", w.Name)
	}
}

func TestDsWindowLapsed(t *testing.T) {
	spec := dsSpec(SiteCN)
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	// 26 小时前，窗口已过（24h 窗口）。
	rows := []dsRow{{at: now.Add(-26 * time.Hour), tokens: 1e6}}
	if w := dsWindow(rows, time.Time{}, now, spec); w != nil {
		t.Errorf("lapsed window should be nil, got %+v", w)
	}
}

func TestDsWindowRefused(t *testing.T) {
	spec := dsSpec(SiteCN)
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	reset := now.Add(3 * time.Hour)
	w := dsWindow(nil, reset, now, spec)
	if w == nil {
		t.Fatal("expected a refused card")
	}
	if w.Amount != spec.cap || w.Used != 100 {
		t.Errorf("refused card should be full: %+v", w)
	}
	if !w.Aside {
		t.Errorf("refused card stays aside in our account-scoped routing")
	}
}

func TestDsWindowBreak(t *testing.T) {
	spec := dsSpec(SiteCN)
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	// 两条请求间隔 25h：第二条开新窗口，只有第二条的 token 算进当前窗口。
	rows := []dsRow{
		{at: now.Add(-26 * time.Hour), tokens: 9e7},
		{at: now.Add(-1 * time.Hour), tokens: 1e6},
	}
	w := dsWindow(rows, time.Time{}, now, spec)
	if w == nil {
		t.Fatal("expected a window")
	}
	if w.Amount != 1e6 {
		t.Errorf("window should only count the second row, got %v", w.Amount)
	}
}
