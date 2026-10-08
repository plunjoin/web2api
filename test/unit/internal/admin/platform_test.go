//go:build web2api_unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"web2api/internal/store"
	"web2api/test/support/testredis"
)

// 端到端：注册开关、用户登录、角色隔离、兑换（含并发）、管理员加余额与流水。
func TestPlatformUserFlow(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	mux := http.NewServeMux()
	New(nil, st, nil).Mount(mux)
	do := func(method, path string, body any, token string) (int, map[string]any) {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(data)))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	redis := testredis.Start(t)
	if code, out := do("POST", "/admin/api/setup", map[string]string{"email": "root@example.com", "password": "root-password-1", "nickname": "Root", "redis_url": redis.URL()}, ""); code != 201 {
		t.Fatalf("setup %d %v", code, out)
	}
	_, out := do("POST", "/admin/api/auth/login", map[string]string{"email": "root@example.com", "password": "root-password-1"}, "")
	root, _ := out["access_token"].(string)
	if user, _ := out["user"].(map[string]any); root == "" || user["role"] != "admin" || user["kind"] != "root" || user["email"] != "root@example.com" {
		t.Fatalf("root login: %v", out)
	}

	reg := map[string]string{"email": "user@example.com", "password": "user-password-1", "nickname": "小王"}
	if code, out := do("POST", "/api/auth/register", reg, ""); code != 403 {
		t.Fatalf("注册默认关闭: %d %v", code, out)
	}
	if code, _ := do("PUT", "/admin/api/settings", map[string]any{"registration_open": true, "signup_bonus": 100}, root); code != 200 {
		t.Fatalf("open registration: %d", code)
	}
	if code, out := do("GET", "/api/public/config", nil, ""); code != 200 || out["registration_open"] != true {
		t.Fatalf("public config: %d %v", code, out)
	}
	code, out := do("POST", "/api/auth/register", reg, "")
	userToken, _ := out["access_token"].(string)
	if code != 201 || userToken == "" {
		t.Fatalf("register: %d %v", code, out)
	}
	if code, _ := do("POST", "/api/auth/register", reg, ""); code != 409 {
		t.Fatalf("duplicate register: %d", code)
	}
	if code, out := do("GET", "/api/auth/me", nil, userToken); code != 200 || out["user"].(map[string]any)["balance"] != float64(100) {
		t.Fatalf("me: %d %v", code, out)
	}
	// 普通用户不能访问管理接口；根管理员没有用户控制台
	if code, _ := do("GET", "/admin/api/users", nil, userToken); code != 403 {
		t.Fatalf("user → admin api: %d", code)
	}
	if code, _ := do("GET", "/admin/api/keys", nil, userToken); code != 403 {
		t.Fatalf("user → legacy admin api: %d", code)
	}
	if code, _ := do("GET", "/api/user/overview", nil, root); code != 403 {
		t.Fatalf("root → user console: %d", code)
	}
	// 登录
	code, out = do("POST", "/api/auth/login", map[string]string{"email": "USER@example.com", "password": "user-password-1"}, "")
	if code != 200 || out["user"].(map[string]any)["kind"] != "user" {
		t.Fatalf("user login: %d %v", code, out)
	}
	if code, _ := do("POST", "/api/auth/login", map[string]string{"email": "user@example.com", "password": "wrong-password"}, ""); code != 401 {
		t.Fatalf("wrong user password: %d", code)
	}

	// 管理员加余额（必须带备注）
	_, out = do("GET", "/admin/api/users?q=user@", nil, root)
	users := out["users"].([]any)
	uid := int64(users[0].(map[string]any)["id"].(float64))
	path := "/admin/api/users/" + itoa(uid)
	if code, _ := do("POST", path+"/balance", map[string]any{"amount": 500}, root); code != 400 {
		t.Fatalf("no note: %d", code)
	}
	if code, out := do("POST", path+"/balance", map[string]any{"amount": 500, "note": "测试充值"}, root); code != 200 || out["balance"] != float64(600) {
		t.Fatalf("adjust: %d %v", code, out)
	}

	// 兑换码：生成 → 兑换 → 再次兑换失败
	code, out = do("POST", "/admin/api/redeem-codes", map[string]any{"amount": 1000, "count": 2, "note": "单测"}, root)
	if code != 201 {
		t.Fatalf("create codes: %d %v", code, out)
	}
	codes := out["codes"].([]any)
	first := codes[0].(map[string]any)["code"].(string)
	if code, out := do("POST", "/api/user/redeem", map[string]string{"code": first}, userToken); code != 200 || out["balance"] != float64(1600) {
		t.Fatalf("redeem: %d %v", code, out)
	}
	if code, out := do("POST", "/api/user/redeem", map[string]string{"code": first}, userToken); code != 409 {
		t.Fatalf("second redeem: %d %v", code, out)
	}
	// 并发兑换第二个码：恰好一次成功
	second := codes[1].(map[string]any)["code"].(string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, _ := do("POST", "/api/user/redeem", map[string]string{"code": second}, userToken); code == 200 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("并发兑换成功次数 = %d", ok)
	}

	// 用户 Key：创建、只看自己的、不能操作别人的
	code, out = do("POST", "/api/user/keys", map[string]any{"name": "laptop"}, userToken)
	if code != 200 {
		t.Fatalf("create key: %d %v", code, out)
	}
	keyID := int64(out["key"].(map[string]any)["id"].(float64))
	adminKey, _ := st.CreateKey("admin-owned")
	if code, _ := do("DELETE", "/api/user/keys/"+itoa(adminKey.ID), nil, userToken); code != 404 {
		t.Fatalf("user deleting unowned key: %d", code)
	}
	if code, out := do("GET", "/api/user/keys", nil, userToken); code != 200 || len(out["keys"].([]any)) != 1 {
		t.Fatalf("list keys: %d %v", code, out)
	}
	if code, _ := do("PATCH", "/api/user/keys/"+itoa(keyID), map[string]any{"enabled": false}, userToken); code != 200 {
		t.Fatalf("disable own key: %d", code)
	}

	// 流水：注册赠送 + 加余额 + 2 次兑换
	code, out = do("GET", "/api/user/ledger", nil, userToken)
	if code != 200 || out["total"] != float64(4) || out["balance"] != float64(2600) {
		t.Fatalf("user ledger: %d %v", code, out)
	}
	if code, out := do("GET", "/admin/api/ledger?kind=redeem", nil, root); code != 200 || out["total"] != float64(2) {
		t.Fatalf("admin ledger filter: %d %v", code, out)
	}
	if bad, _ := st.LedgerIntegrity(); len(bad) != 0 {
		t.Fatalf("ledger integrity: %v", bad)
	}

	// 停用用户：旧登录态立即失效
	if code, _ := do("PATCH", path, map[string]any{"enabled": false}, root); code != 200 {
		t.Fatal("disable user")
	}
	if code, _ := do("GET", "/api/user/overview", nil, userToken); code != 401 && code != 403 {
		t.Fatalf("disabled user session: %d", code)
	}
	if code, _ := do("POST", "/api/auth/login", map[string]string{"email": "user@example.com", "password": "user-password-1"}, ""); code != 403 {
		t.Fatalf("disabled user login: %d", code)
	}
}

// 用户登录失败只锁该邮箱，不影响管理员和其他用户。
func TestUserLoginThrottleIsPerEmail(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "throttle.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	redis := testredis.Start(t)
	a := New(nil, st, nil)
	body, _ := json.Marshal(map[string]string{"email": "root@example.com", "password": "root-password-1", "nickname": "Root", "redis_url": redis.URL()})
	w := httptest.NewRecorder()
	a.handleSetup(w, httptest.NewRequest("POST", "/admin/api/setup", strings.NewReader(string(body))))
	login := func(email, password string) int {
		w := httptest.NewRecorder()
		a.handleLogin(w, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"`+email+`","password":"`+password+`"}`)))
		return w.Code
	}
	for i := 0; i < 11; i++ {
		login("victim@example.com", "wrong")
	}
	if code := login("victim@example.com", "wrong"); code != 429 {
		t.Fatalf("victim throttled: %d", code)
	}
	if code := login("root@example.com", "root-password-1"); code != 200 {
		t.Fatalf("root must still log in: %d", code)
	}
}

func itoa(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
