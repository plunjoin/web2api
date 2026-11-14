package provider

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"web2api/internal/config"
	"web2api/internal/engine/workbuddy"
	"web2api/internal/store"
)

func TestAddWorkBuddyWithoutLabel(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "workbuddy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()
	e := workbuddy.NewAI(config.WorkBuddyConfig{BaseURL: server.URL}, st)
	defer e.Close()
	m := newTestManager(e)
	m.st = st
	jwt := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"email":"jwt@example.com"}`)) + ".signature"
	for _, tc := range []struct{ token, email string }{{jwt, "jwt@example.com"}, {"opaque-secret-token", ""}} {
		creds, _ := json.Marshal(workbuddy.Credentials{Token: tc.token, Site: "workbuddy-ai", BaseURL: server.URL})
		acc, err := m.AddAccountFromStorageState(AccountInput{Engine: "workbuddy-ai", Credentials: creds})
		if err != nil {
			t.Fatalf("empty label rejected: %v", err)
		}
		var saved workbuddy.Credentials
		if err := json.Unmarshal([]byte(acc.Credentials), &saved); err != nil {
			t.Fatal(err)
		}
		if saved.Email != tc.email || acc.Label == "" || strings.Contains(acc.Label, tc.token) {
			t.Fatalf("unsafe or missing identity: label=%q email=%q", acc.Label, saved.Email)
		}
		if tc.email != "" && acc.Label != tc.email {
			t.Fatalf("JWT email should name account: %q", acc.Label)
		}
	}
}

func TestAddWorkBuddySameEmailAcrossSites(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "workbuddy-labels.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	cn := workbuddy.New(config.WorkBuddyConfig{BaseURL: server.URL}, st)
	defer cn.Close()
	ai := workbuddy.NewAI(config.WorkBuddyConfig{BaseURL: server.URL}, st)
	defer ai.Close()
	m := newTestManager(cn, ai)
	m.st = st
	original, err := st.CreateAccount("workbuddy", "same@example.com", `{"token":"original"}`, false)
	if err != nil {
		t.Fatal(err)
	}
	// 两个发行版的标签空间是分开的：国服重名顺延编号，国际服从自己的 1 开始。
	want := map[string][]string{
		"workbuddy":    {"same@example.com (2)", "same@example.com (3)"},
		"workbuddy-ai": {"same@example.com"},
	}
	for i, site := range []string{"workbuddy", "workbuddy-ai", "workbuddy"} {
		creds, _ := json.Marshal(workbuddy.Credentials{Token: "fixture-token", Email: "same@example.com", Site: site, UID: "uid-" + site, BaseURL: server.URL})
		label := ""
		if i == 2 {
			label = "same@example.com" // 显式指定的名字同样安全地顺延。
		}
		acc, err := m.AddAccountFromStorageState(AccountInput{Engine: site, Label: label, Credentials: creds})
		if err != nil {
			t.Fatalf("same label rejected for %s: %v", site, err)
		}
		names := want[site]
		if acc.Label != names[0] || acc.ID == original.ID || acc.Engine != site {
			t.Fatalf("label=%q engine=%q want %q on %s", acc.Label, acc.Engine, names[0], site)
		}
		want[site] = names[1:]
		var saved workbuddy.Credentials
		if err := json.Unmarshal([]byte(acc.Credentials), &saved); err != nil || saved.Site != site || saved.UID != "uid-"+site {
			t.Fatal("new account identity lost")
		}
		found := false
		for _, eng := range []*workbuddy.Engine{cn, ai} {
			for _, live := range eng.AccountStates() {
				if live.ID == strconv.FormatInt(acc.ID, 10) && live.Label == acc.Label {
					found = eng.Name() == site
				}
			}
		}
		if !found {
			t.Fatal("account must land in its own provider's pool")
		}
	}
	old, err := st.GetAccount(original.ID)
	if err != nil || old != original {
		t.Fatal("adding a namesake must not update the old account")
	}
}
func TestAccountOverviewUnloadedWorkBuddyService(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "workbuddy-overview.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	acc, err := st.CreateAccount("workbuddy", "intl", `{"site":"workbuddy-ai","token":"test"}`, false)
	if err != nil {
		t.Fatal(err)
	}
	m := newTestManager()
	m.st = st
	e := workbuddy.New(config.WorkBuddyConfig{BaseURL: "http://127.0.0.1:1"}, st)
	defer e.Close()
	for _, pools := range []map[string]accountPool{nil, {"workbuddy": e}} {
		m.pools = pools
		got := m.AccountOverview()
		if len(got) != 1 || got[0].Live.Service != "workbuddy-ai" || got[0].Live.Status != "unloaded" {
			t.Fatalf("unloaded overview=%+v", got)
		}
	}
	// 已加载账号保留 resolve 实际采用的发行版（引擎配置 URL 优先于默认站点）。
	if err := e.AddAccount(strconv.FormatInt(acc.ID, 10), acc.Label, workbuddy.Credentials{Token: "test"}); err != nil {
		t.Fatal(err)
	}
	got := m.AccountOverview()
	if got[0].Live.Service != "workbuddy" {
		t.Fatalf("loaded service overwritten: %+v", got[0].Live)
	}
}

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
