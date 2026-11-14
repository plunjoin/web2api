package workbuddy

import (
	"errors"
	"strings"
	"testing"
	"time"

	"web2api/internal/config"
)

// TestBuildWindowsSplitsPlanAndExtra 用真实额度形状验证三窗口拆分。
// 数据取自本机 magpie 的 quotas.json（国服免费号：套餐 0/500，赠送 0/1700）。
func TestBuildWindowsSplitsPlanAndExtra(t *testing.T) {
	sum := map[string]any{
		"CreditGroups": []any{
			map[string]any{"Kind": "plan_base", "CycleTotalCapacity": 500.0, "CycleUsedCapacity": 0.0},
			map[string]any{"Kind": "plan_bonus", "CycleTotalCapacity": 1700.0, "CycleUsedCapacity": 0.0},
		},
	}
	wins := buildWindows(sum)
	if len(wins) != 3 {
		t.Fatalf("want 3 windows, got %d: %+v", len(wins), wins)
	}
	if wins[0].Name != "Credits" || wins[0].Limit != 500 || !wins[0].Aside {
		t.Errorf("Credits window wrong: %+v", wins[0])
	}
	if wins[1].Name != "Extra credits" || wins[1].Limit != 1700 || !wins[1].Aside {
		t.Errorf("Extra window wrong: %+v", wins[1])
	}
	if wins[2].Name != "Total credits" || wins[2].Limit != 2200 || wins[2].Aside {
		t.Errorf("Total window wrong (must not be aside): %+v", wins[2])
	}
}

// TestBuildWindowsExhausted 国际服免费号：套餐与赠送都用尽，Total 停号。
func TestBuildWindowsExhausted(t *testing.T) {
	sum := map[string]any{
		"CreditGroups": []any{
			map[string]any{"Kind": "plan_base", "CycleTotalCapacity": 100.0, "CycleUsedCapacity": 100.0},
			map[string]any{"Kind": "addon", "CycleTotalCapacity": 630.0, "CycleUsedCapacity": 630.0},
		},
	}
	q := &Quota{Windows: buildWindows(sum)}
	if !q.Exhausted() {
		t.Fatalf("want exhausted, windows: %+v", q.Windows)
	}
	if got := q.Display(); got == "" || !strings.HasSuffix(got, "（已用尽）") {
		t.Errorf("Display should mark exhausted: %q", got)
	}
}

func TestOnlyTotalStopsAccount(t *testing.T) {
	// 套餐用尽但赠送还有：Total 未满，账号仍可用。
	q := &Quota{Windows: []Window{
		{Name: "Credits", Amount: 500, Limit: 500, Aside: true},
		{Name: "Extra credits", Amount: 100, Limit: 1700, Aside: true},
		{Name: "Total credits", Amount: 600, Limit: 2200},
	}}
	if q.Exhausted() {
		t.Fatal("extra credits left must keep the account alive")
	}
}

// TestApplyRunsOutEarliest 验证「最早作废的积分」用于智能路由。
func TestApplyRunsOutEarliest(t *testing.T) {
	end1 := time.Now().Add(72 * time.Hour).UnixMilli()
	end2 := time.Now().Add(24 * time.Hour).UnixMilli() // 更早
	sum := map[string]any{
		"Packages": []any{
			map[string]any{"PackageCode": "A"},
			map[string]any{"PackageCode": "B"},
		},
	}
	pkgs := []map[string]any{
		{"PackageCode": "A", "CycleCapacityRemainPrecise": 1500.0, "DeductionEndTime": float64(end1)},
		{"PackageCode": "B", "CycleCapacityRemainPrecise": 300.0, "DeductionEndTime": float64(end2)},
	}
	q := &Quota{Windows: []Window{
		{Name: "Credits", Aside: true},
		{Name: "Total credits"},
	}}
	applyRunsOut(q, sum, pkgs)
	total := q.Windows[1]
	if total.RunsOutAt == "" || total.RunsOut != 300 {
		t.Fatalf("want earliest package (300 @ sooner), got %+v", total)
	}
	if !q.RunsOutAt().After(time.Now()) {
		t.Fatalf("RunsOutAt should be in the future: %v", q.RunsOutAt())
	}
}

func TestResolveSitePrecedence(t *testing.T) {
	// 引擎 base_url 是旧的 codebuddy.ai，但账号明确指定国际服 → 国际服端点优先。
	e := &Engine{baseURL: "https://www.codebuddy.ai"}
	ep, site := e.resolve(Credentials{Site: "workbuddy-ai"})
	if site.ID != SiteIntl.ID || ep != SiteIntl.Endpoint {
		t.Errorf("account site should win over engine base: %s %s", site.ID, ep)
	}
	// 账号没有 site/base_url → 沿用引擎 base_url（旧账号兼容）。
	ep, site = e.resolve(Credentials{})
	if ep != "https://www.codebuddy.ai" || site.ID != SiteCN.ID {
		t.Errorf("legacy fallback wrong: %s %s", site.ID, ep)
	}
	// 账号级 base_url 最高优先级。
	ep, _ = e.resolve(Credentials{BaseURL: "https://example.test/"})
	if ep != "https://example.test" {
		t.Errorf("account base_url should win: %s", ep)
	}
}

func TestResolveDefaultCN(t *testing.T) {
	e := &Engine{}
	ep, site := e.resolve(Credentials{})
	if ep != SiteCN.Endpoint || site.ID != SiteCN.ID {
		t.Errorf("empty everything should default to CN WorkBuddy: %s %s", site.ID, ep)
	}
}

func TestErrorClassification(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
		name   string
	}{
		{403, `{"code":11140,"msg":"account has been restricted due to unusual activity"}`, IsRestricted, "restricted"},
		{400, `{"code":14018,"msg":"credits exhausted"}`, IsCreditsExhausted, "exhausted"},
		{400, `{"error":{"internal_code":"11159"}}`, func(e error) bool { return codeOf(e) == 11159 }, "internal_code"},
		{400, `{"error":"11158:no entitlement"}`, func(e error) bool { return codeOf(e) == 11158 }, "string code"},
		{429, `rate limited`, IsRateLimited, "rate limited"},
		{401, `{"code":12153}`, IsUnauthorized, "unauthorized"},
		{400, `{"code":11128,"msg":"unapproved channel"}`, IsChannelRefused, "channel refused"},
		{400, `{"code":12351,"msg":"prompt is too long"}`, func(e error) bool {
			return !IsCreditsExhausted(e) && !IsRestricted(e) && !IsChannelRefused(e)
		}, "plain error"},
	}
	for _, c := range cases {
		err := newUpstreamError(c.status, []byte(c.body))
		if !c.check(err) {
			t.Errorf("%s: classification failed for %d %s -> %v (code=%d)", c.name, c.status, c.body, err, codeOf(err))
		}
	}
}

// TestUnauthorizedExcludesTypedErrors 403 同时被上游用于风控/额度/渠道，
// 不能一律当 token 失效换号重试。
func TestUnauthorizedExcludesTypedErrors(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`{"code":11140,"msg":"restricted"}`, false},
		{`{"code":14018,"msg":"credits exhausted"}`, false},
		{`{"code":11128,"msg":"unapproved channel"}`, false},
		{`{"code":12345,"msg":"forbidden"}`, true},
	}
	for _, c := range cases {
		err := newUpstreamError(403, []byte(c.body))
		if got := IsUnauthorized(err); got != c.want {
			t.Errorf("IsUnauthorized(403 %s) = %v, want %v", c.body, got, c.want)
		}
	}
	// 401 永远是授权问题。
	if !IsUnauthorized(newUpstreamError(401, []byte(`{}`))) {
		t.Error("401 should be unauthorized")
	}
}

// TestShortErrKeepsCardsReadable HTML 错误页（nginx 的 401 页）只留状态码，
// 长错误截断：否则额度那一行会把账号卡片撑出横向滚动条。
func TestShortErrKeepsCardsReadable(t *testing.T) {
	html := newUpstreamError(401, []byte("<html><head><title>401 Authorization Required</title></head></html>"))
	if got := shortErr(html); got != "上游 HTTP 401" {
		t.Errorf("HTML body should collapse to status, got %q", got)
	}
	long := newUpstreamError(502, []byte(strings.Repeat("上游错误", 100)))
	got := shortErr(long)
	if n := len([]rune(got)); n > 140 {
		t.Errorf("long error should be clipped, got %d runes", n)
	}
	if !strings.HasPrefix(got, "上游 HTTP 502: ") {
		t.Errorf("status prefix missing: %q", got)
	}
	if shortErr(nil) != "" {
		t.Error("nil error should be empty")
	}
}

func TestUpstreamStatus(t *testing.T) {
	if got := UpstreamStatus(newUpstreamError(502, []byte(`bad gateway`))); got != 502 {
		t.Errorf("UpstreamStatus = %d, want 502", got)
	}
	if got := UpstreamStatus(errors.New("boom")); got != 0 {
		t.Errorf("non-upstream error should be 0, got %d", got)
	}
}

// TestBuildWindowsSingleWindow 只有一个池时退化成单一 Credits 窗口（不能停号标记置为 aside）。
func TestBuildWindowsSingleWindow(t *testing.T) {
	sum := map[string]any{
		"CreditGroups": []any{
			map[string]any{"Kind": "plan_base", "CycleTotalCapacity": 800.0, "CycleUsedCapacity": 200.0},
		},
	}
	wins := buildWindows(sum)
	if len(wins) != 1 || wins[0].Name != "Credits" || wins[0].Aside {
		t.Fatalf("want single Credits window, got %+v", wins)
	}
	if wins[0].Limit != 800 || wins[0].Amount != 200 {
		t.Errorf("window amounts wrong: %+v", wins[0])
	}
	q := &Quota{Windows: wins}
	if q.Exhausted() {
		t.Error("200/800 must not be exhausted")
	}
}

func codeOf(err error) int {
	if ue, ok := err.(*UpstreamError); ok {
		return ue.Code
	}
	return 0
}

func TestParseCodeVariants(t *testing.T) {
	cases := map[string]int{
		`{"code":14018}`:                      14018,
		`{"data":{"code":11140}}`:             11140,
		`{"error":{"code":11128}}`:            11128,
		`{"error":{"internal_code":"11159"}}`: 11159,
		`{"error":"11158:no entitlement"}`:    11158,
		`{"code":0,"msg":"ok"}`:               0,
		`not json`:                            0,
	}
	for body, want := range cases {
		if got := parseCode([]byte(body)); got != want {
			t.Errorf("parseCode(%s) = %d, want %d", body, got, want)
		}
	}
}

// TestPickPrefersEarliestRunsOut 智能路由：积分先过期的账号优先接单。
func TestPickPrefersEarliestRunsOut(t *testing.T) {
	e := New(config.WorkBuddyConfig{}, nil)
	now := time.Now()
	a1 := &workbuddyAccount{id: "1", label: "late", enabled: true, ready: true,
		quota: &Quota{Windows: []Window{{Name: "Total credits", Limit: 100, RunsOutAt: now.Add(72 * time.Hour).Format(time.RFC3339)}}}}
	a2 := &workbuddyAccount{id: "2", label: "soon", enabled: true, ready: true,
		quota: &Quota{Windows: []Window{{Name: "Total credits", Limit: 100, RunsOutAt: now.Add(time.Hour).Format(time.RFC3339)}}}}
	// 先放 late，确保选择不是「列表顺序」造成的。
	e.accounts = []*workbuddyAccount{a1, a2}
	e.ids = map[string]int{"1": 0, "2": 1}
	got := e.pick()
	if got == nil || got.id != "2" {
		t.Fatalf("want earliest-expiring account (2), got %+v", got)
	}
}

// TestEditionsAreSeparateProviders 国服与国际服是两个供应商：账号只能进自己的池，
// 每个引擎的目录也只报自己发行版的模型。
func TestEditionsAreSeparateProviders(t *testing.T) {
	cn := New(config.WorkBuddyConfig{}, nil)
	defer cn.Close()
	ai := NewAI(config.WorkBuddyConfig{}, nil)
	defer ai.Close()

	if err := cn.AddAccount("1", "cn", Credentials{Token: "t", Site: "workbuddy"}); err != nil {
		t.Fatal(err)
	}
	if err := cn.AddAccount("2", "intl", Credentials{Token: "t", Site: "workbuddy-ai"}); err == nil {
		t.Fatal("CN provider must reject an international account")
	}
	if err := ai.AddAccount("3", "intl", Credentials{Token: "t", Site: "workbuddy-ai"}); err != nil {
		t.Fatal(err)
	}
	if err := ai.AddAccount("4", "cn", Credentials{Token: "t", Site: "workbuddy"}); err == nil {
		t.Fatal("international provider must reject a CN account")
	}
	// 旧 CodeBuddy 账号沿国服接纳。
	if err := cn.AddAccount("5", "legacy", Credentials{Token: "t", BaseURL: "https://www.codebuddy.ai"}); err != nil {
		t.Fatal(err)
	}

	cnIDs := map[string]bool{}
	for _, m := range cn.ListModels() {
		cnIDs[m.ID] = true
		if m.Engine != "workbuddy" {
			t.Fatalf("CN catalog engine = %q", m.Engine)
		}
	}
	aiIDs := map[string]bool{}
	for _, m := range ai.ListModels() {
		aiIDs[m.ID] = true
		if m.Engine != "workbuddy-ai" {
			t.Fatalf("AI catalog engine = %q", m.Engine)
		}
	}
	if !cnIDs["hy4-preview-f"] || aiIDs["hy4-preview-f"] {
		t.Fatal("hy4-preview-f must belong to the CN catalog only")
	}
	if cnIDs["gpt-5.5"] || !aiIDs["gpt-5.5"] {
		t.Fatal("gpt-5.5 must belong to the international catalog only")
	}
	if !cnIDs["glm-5.3"] || !aiIDs["glm-5.3"] {
		t.Fatal("shared model names must appear in both catalogs")
	}
}

func TestPickSkipsExhausted(t *testing.T) {
	e := New(config.WorkBuddyConfig{}, nil)
	a1 := &workbuddyAccount{id: "1", enabled: true, ready: true,
		quota: &Quota{Windows: []Window{{Name: "Total credits", Amount: 100, Limit: 100}}}}
	a2 := &workbuddyAccount{id: "2", enabled: true, ready: true}
	e.accounts = []*workbuddyAccount{a1, a2}
	e.ids = map[string]int{"1": 0, "2": 1}
	if got := e.pick(); got == nil || got.id != "2" {
		t.Fatalf("want non-exhausted account, got %+v", got)
	}
}

func TestSiteFor(t *testing.T) {
	if SiteFor("workbuddy-ai").ID != SiteIntl.ID {
		t.Error("workbuddy-ai should map to international")
	}
	if SiteFor("workbuddy").ID != SiteCN.ID {
		t.Error("workbuddy should map to CN")
	}
	if SiteFor("").ID != SiteCN.ID {
		t.Error("empty should map to CN")
	}
	if SiteForBaseURL("https://www.workbuddy.ai").ID != SiteIntl.ID {
		t.Error("base url intl")
	}
	if !IsLegacyCodeBuddy("https://www.codebuddy.ai") {
		t.Error("codebuddy.ai should be legacy")
	}
	if IsLegacyCodeBuddy("https://copilot.tencent.com") {
		t.Error("copilot.tencent.com is not legacy")
	}
}
