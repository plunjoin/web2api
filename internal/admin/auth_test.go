package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"web2api/internal/store"
	"web2api/internal/testredis"
)

func TestSetupLoginSessionLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { st.Close() }()
	mux := http.NewServeMux()
	a := New(nil, st, nil)
	a.Mount(mux)
	do := func(method, path string, body any, token string) (int, map[string]any) {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(data)))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("non-JSON: %s", w.Body)
		}
		return w.Code, out
	}
	if code, out := do("GET", "/admin/api/setup", nil, ""); code != 200 || out["initialized"] != false {
		t.Fatalf("status %d %v", code, out)
	}
	if code, _ := do("GET", "/admin/api/auth/me", nil, "old-static-token"); code != 401 {
		t.Fatal(code)
	}
	for _, endpoint := range []struct{ method, path string }{{"GET", "/admin/api/upgrade"}, {"POST", "/admin/api/upgrade/check"}, {"POST", "/admin/api/upgrade"}} {
		if code, _ := do(endpoint.method, endpoint.path, nil, ""); code != 401 {
			t.Fatalf("upgrade endpoint must require admin login: %s %d", endpoint.path, code)
		}
	}
	r := testredis.Start(t)
	setup := map[string]string{"email": "ADMIN@example.com", "password": "a-password-123", "nickname": "自定义昵称", "redis_url": r.URL()}
	bad := map[string]string{"email": "admin@example.com", "password": "short", "nickname": "Admin", "redis_url": r.URL()}
	if code, _ := do("POST", "/admin/api/setup", bad, ""); code != 400 {
		t.Fatalf("weak password: %d", code)
	}
	bad["password"] = "strong-password"
	bad["redis_url"] = "redis://127.0.0.1:0/0"
	if code, _ := do("POST", "/admin/api/setup", bad, ""); code != 400 {
		t.Fatalf("unreachable redis: %d", code)
	}
	if v, _ := st.AdminSettings(); v != nil {
		t.Fatal("failed setup must not initialize")
	}
	if code, out := do("POST", "/admin/api/setup", setup, ""); code != 201 {
		t.Fatalf("setup %d %v", code, out)
	}
	if code, _ := do("POST", "/admin/api/setup", setup, ""); code != 409 {
		t.Fatalf("duplicate setup: %d", code)
	}
	v, err := st.AdminSettings()
	if err != nil || v.Email != "admin@example.com" || v.PasswordHash == setup["password"] || !strings.HasPrefix(v.PasswordHash, "$2") {
		t.Fatalf("stored settings: %v %v", v, err)
	}
	// Reopen SQLite and rebuild the handler to simulate a server restart.
	st.Close()
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a = New(nil, st, nil)
	mux = http.NewServeMux()
	a.Mount(mux)
	if code, out := do("GET", "/admin/api/setup", nil, ""); code != 200 || out["initialized"] != true {
		t.Fatalf("restart setup status: %d %v", code, out)
	}
	login := map[string]string{"email": "admin@example.com", "password": "wrong-password"}
	if code, _ := do("POST", "/admin/api/auth/login", login, ""); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	login["password"] = setup["password"]
	code, out := do("POST", "/admin/api/auth/login", login, "")
	if code != 200 {
		t.Fatalf("login %d %v", code, out)
	}
	token := out["access_token"].(string)
	if code, out := do("GET", "/admin/api/upgrade", nil, token); code != 200 || out["enabled"] != false {
		t.Fatalf("disabled upgrade status: %d %v", code, out)
	}
	if code, _ := do("POST", "/admin/api/upgrade", nil, token); code != 503 {
		t.Fatalf("disabled upgrade must not start: %d", code)
	}
	serialized, _ := json.Marshal(out)
	if strings.Contains(string(serialized), "password_hash") || strings.Contains(string(serialized), "redis_url") || strings.Contains(string(serialized), "jwt_secret") {
		t.Fatal("login leaks secrets")
	}
	if code, out := do("GET", "/admin/api/auth/me", nil, token); code != 200 || out["user"].(map[string]any)["nickname"] != setup["nickname"] {
		t.Fatalf("profile %d %v", code, out)
	}
	// A new handler still accepts a valid persisted signing secret and Redis session.
	a = New(nil, st, nil)
	mux = http.NewServeMux()
	a.Mount(mux)
	if code, _ := do("GET", "/admin/api/auth/me", nil, token); code != 200 {
		t.Fatalf("JWT after restart: %d", code)
	}
	if code, _ := do("GET", "/admin/api/auth/me", nil, token+"invalid"); code != 401 {
		t.Fatalf("tampered token: %d", code)
	}
	x := httptest.NewRequest("GET", "/admin/api/auth/me", nil)
	x.Header.Set("X-Admin-Token", token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, x)
	if w.Code != 401 {
		t.Fatal("legacy header accepted")
	}
	if code, _ := do("POST", "/admin/api/auth/logout", nil, token); code != 200 {
		t.Fatalf("logout %d", code)
	}
	if code, _ := do("GET", "/admin/api/auth/me", nil, token); code != 401 {
		t.Fatalf("logged out token: %d", code)
	}
	_, out = do("POST", "/admin/api/auth/login", login, "")
	token = out["access_token"].(string)
	r.Close()
	if code, _ := do("GET", "/admin/api/auth/me", nil, token); code != 503 {
		t.Fatalf("Redis outage must fail closed: %d", code)
	}
}

func TestJWTValidation(t *testing.T) {
	now := time.Now().Unix()
	valid := claims{Issuer: "web2api", Audience: "web2api-admin", Subject: "1", ID: "session-id", IssuedAt: now, NotBefore: now, Expires: now + 3600}
	token, _ := signJWT(valid, "secret")
	if _, err := verifyJWT(token, "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyJWT(token, "other-secret"); err == nil {
		t.Fatal("wrong secret accepted")
	}
	for _, mutate := range []func(*claims){
		func(c *claims) { c.Expires = now - 1 }, func(c *claims) { c.Audience = "other" }, func(c *claims) { c.Issuer = "other" }, func(c *claims) { c.Subject = "2" }, func(c *claims) { c.ID = "" }, func(c *claims) { c.NotBefore = now + 60 }, func(c *claims) { c.IssuedAt = now + 60 },
	} {
		c := valid
		mutate(&c)
		token, _ := signJWT(c, "secret")
		if _, err := verifyJWT(token, "secret"); err == nil {
			t.Fatalf("invalid claims accepted: %+v", c)
		}
	}
	for _, token := range []string{"", "a.b.c", "static-admin-token", "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.e30."} {
		if _, err := verifyJWT(token, "secret"); err == nil {
			t.Fatalf("invalid JWT accepted: %s", token)
		}
	}
}

func TestLoginThrottled(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := testredis.Start(t)
	a := New(nil, st, nil)
	body, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "a-password-123", "nickname": "Admin", "redis_url": r.URL()})
	w := httptest.NewRecorder()
	a.handleSetup(w, httptest.NewRequest("POST", "/admin/api/setup", strings.NewReader(string(body))))
	if w.Code != 201 {
		t.Fatal(w.Body)
	}
	for i := 0; i < 11; i++ {
		w = httptest.NewRecorder()
		a.handleLogin(w, httptest.NewRequest("POST", "/admin/api/auth/login", strings.NewReader(`{"email":"someone@example.com","password":"wrong"}`)))
		want := 401
		if i == 10 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d: %d %s", i, w.Code, w.Body)
		}
	}
}
