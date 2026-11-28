package flow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"web2api/internal/config"
	"web2api/internal/store"
)

func flowPage(email string) string {
	return `<html>boq_labs-ai-sandbox-frontend_20260101.0 "FdrFJe":"123" "SNlM0e":"xsrf-1" "oPEP7c":"` + email + `"</html>`
}

func TestApplySetCookiesMergesAndDeletes(t *testing.T) {
	existing := []Cookie{
		{Name: "OSID", Value: "old", Domain: "flow.google.com", Path: "/"},
		{Name: "SID", Value: "keep", Domain: "google.com", Path: "/project", SameSite: "Lax", HTTPOnly: true},
		{Name: "HSID", Value: "other-path", Domain: "google.com", Path: "/other"},
	}
	got := ApplySetCookies(existing, []string{
		"OSID=new; Path=/; Secure; HttpOnly",
		"SID=rotated; Domain=.google.com; Path=/project",
		"HSID=cleared; Domain=.google.com; Path=/other; Max-Age=0",
		"NID=nope; Domain=example.com; Path=/",
		"SAPISID=added; Domain=.google.com; Path=/; Expires=Wed, 21 Oct 2099 07:28:00 GMT",
	}, "flow.google.com")
	byName := map[string]Cookie{}
	for _, cookie := range got {
		byName[cookie.Name+"|"+cookie.Path] = cookie
	}
	if byName["OSID|/"].Value != "new" || !byName["OSID|/"].HostOnly {
		t.Fatalf("OSID = %+v, want value new and host-only", byName["OSID|/"])
	}
	if byName["SID|/project"].Value != "rotated" || byName["SID|/project"].SameSite != "Lax" || !byName["SID|/project"].HTTPOnly {
		t.Fatalf("SID = %+v, want rotated value and preserved flags", byName["SID|/project"])
	}
	if _, ok := byName["HSID|/other"]; ok {
		t.Fatal("Max-Age=0 should delete HSID")
	}
	if _, ok := byName["NID|/"]; ok {
		t.Fatal("non-google cookie should be ignored")
	}
	if byName["SAPISID|/"].Value != "added" || byName["SAPISID|/"].Expires == 0 {
		t.Fatalf("SAPISID = %+v, want added with expiry", byName["SAPISID|/"])
	}
}

func TestApplySetCookiesReplacesHeaderPlaceholder(t *testing.T) {
	pasted, err := ParseCookies("OSID=old; SID=account")
	if err != nil {
		t.Fatal(err)
	}
	got := ApplySetCookies(pasted, []string{"OSID=new; Path=/; Secure"}, "flow.google.com")
	count := 0
	for _, cookie := range got {
		if cookie.Name == "OSID" {
			count++
			if cookie.Value != "new" || cookie.Domain != "flow.google.com" {
				t.Fatalf("OSID = %+v, want the scoped replacement", cookie)
			}
		}
	}
	if count != 1 {
		t.Fatalf("OSID count = %d, want 1", count)
	}
}

func TestBootstrapFollowsRedirectAndKeepsCookies(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/projects", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "OSID", Value: "from-first", Path: "/", Secure: true, HttpOnly: true})
		http.Redirect(w, r, "/projects/landed", http.StatusFound)
	})
	mux.HandleFunc("/projects/landed", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "OSID=from-first") {
			t.Errorf("second hop missing first hop cookie: %q", r.Header.Get("Cookie"))
		}
		w.Header().Add("Set-Cookie", "SID=from-second; Path=/")
		_, _ = w.Write([]byte(flowPage("a@example.com")))
	})
	client := NewRPCClient(server.Client(), "")
	got, err := client.bootstrap(context.Background(), server.URL+"/projects", "", nil, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "a@example.com" || got.SessionID != "123" || got.BootloaderID == "" {
		t.Fatalf("bootstrap = %+v", got)
	}
	header := CookieHeader(got.Cookies, "127.0.0.1")
	if !strings.Contains(header, "OSID=from-first") || !strings.Contains(header, "SID=from-second") {
		t.Fatalf("merged header = %q", header)
	}
}

func TestBootstrapStopsAtLoginRedirect(t *testing.T) {
	login := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("login"))
	}))
	defer login.Close()
	flowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, login.URL, http.StatusFound)
	}))
	defer flowServer.Close()
	client := NewRPCClient(flowServer.Client(), "")
	_, err := client.bootstrap(context.Background(), flowServer.URL+"/projects", "", []Cookie{
		{Name: "OSID", Value: "dead", Domain: "flow.google.com", Path: "/"},
	}, "127.0.0.1")
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("err = %v, want AuthenticationError", err)
	}
}

func TestPersistRefreshedCookiesIsConditional(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	before := []Cookie{
		{Name: "OSID", Value: "old", Domain: "flow.google.com", Path: "/"},
		{Name: "SID", Value: "account", Domain: "google.com", Path: "/"},
	}
	encoded, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	creds := Credentials{Cookies: string(encoded), Email: "a@example.com"}
	account, err := st.CreateAccount("flow", "main", mustJSON(t, creds), true)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(config.FlowConfig{}, st)
	if err := engine.AddAccount(strconv.FormatInt(account.ID, 10), "main", creds); err != nil {
		t.Fatal(err)
	}
	refreshed := []Cookie{
		{Name: "OSID", Value: "new", Domain: "flow.google.com", Path: "/"},
		{Name: "SID", Value: "account", Domain: "google.com", Path: "/"},
	}
	err = engine.persistRefreshedCookies(engine.accounts[0], before, creds.Cookies, Bootstrap{Email: "a@example.com", Cookies: refreshed})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "initializing" {
		t.Fatalf("status = %q, want initializing left untouched", stored.Status)
	}
	var got Credentials
	if err := json.Unmarshal([]byte(stored.Credentials), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Cookies, `"value":"new"`) {
		t.Fatalf("stored cookies = %s, want refreshed value", got.Cookies)
	}
	stale := []Cookie{
		{Name: "OSID", Value: "stale", Domain: "flow.google.com", Path: "/"},
		{Name: "SID", Value: "account", Domain: "google.com", Path: "/"},
	}
	err = engine.persistRefreshedCookies(engine.accounts[0], before, creds.Cookies, Bootstrap{Email: "a@example.com", Cookies: stale})
	if err != nil {
		t.Fatal(err)
	}
	stored, err = st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.Credentials, "stale") {
		t.Fatalf("stale refresh overwrote newer cookies: %s", stored.Credentials)
	}
}

func TestPersistRefreshedCookiesRejectsOwnedEmail(t *testing.T) {
	engine := New(config.FlowConfig{}, nil)
	first := Credentials{Cookies: "OSID=a; SID=a", Email: "a@example.com"}
	second := Credentials{Cookies: "OSID=b; SID=b", Email: "b@example.com"}
	if err := engine.AddAccount("1", "first", first); err != nil {
		t.Fatal(err)
	}
	if err := engine.AddAccount("2", "second", second); err != nil {
		t.Fatal(err)
	}
	before, err := ParseCookies(second.Cookies)
	if err != nil {
		t.Fatal(err)
	}
	err = engine.persistRefreshedCookies(engine.accounts[1], before, second.Cookies, Bootstrap{
		Email: "a@example.com",
		Cookies: []Cookie{
			{Name: "OSID", Value: "stolen", Domain: "flow.google.com", Path: "/"},
			{Name: "SID", Value: "stolen", Domain: "google.com", Path: "/"},
		},
	})
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("err = %v, want AuthenticationError", err)
	}
	if strings.Contains(engine.accounts[1].creds.Cookies, "stolen") {
		t.Fatal("owned email still wrote cookies")
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestBootstrapRejectsOtherGoogleHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(flowPage("a@example.com")))
	}))
	defer server.Close()
	client := NewRPCClient(server.Client(), "")
	_, err := client.bootstrap(context.Background(), server.URL+"/projects", "", nil, "flow.google.com")
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("err = %v, want AuthenticationError", err)
	}
}

func TestPersistRefreshedCookiesFromPastedHeader(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "header.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	raw := "OSID=old; SID=account"
	creds := Credentials{Cookies: raw, Email: "a@example.com", ProjectID: "p1"}
	account, err := st.CreateAccount("flow", "pasted", mustJSON(t, creds), true)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(config.FlowConfig{}, st)
	if err := engine.AddAccount(strconv.FormatInt(account.ID, 10), "pasted", creds); err != nil {
		t.Fatal(err)
	}
	before, err := ParseCookies(raw)
	if err != nil {
		t.Fatal(err)
	}
	refreshed := ApplySetCookies(before, []string{"OSID=new; Path=/; Secure; HttpOnly"}, "flow.google.com")
	err = engine.persistRefreshedCookies(engine.accounts[0], before, raw, Bootstrap{Email: "a@example.com", Cookies: refreshed})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "initializing" {
		t.Fatalf("status = %q, want initializing", stored.Status)
	}
	var got Credentials
	if err := json.Unmarshal([]byte(stored.Credentials), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Cookies, `"value":"new"`) || strings.Contains(got.Cookies, "OSID=old") {
		t.Fatalf("stored cookies = %s, want the refreshed jar", got.Cookies)
	}
	if got.ProjectID != "p1" || got.Email != "a@example.com" {
		t.Fatalf("credentials lost fields: %+v", got)
	}
}
