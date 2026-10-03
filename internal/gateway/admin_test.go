package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"web2api/internal/testredis"
)

// TestAdminAPIFull 管理台 REST 的完整链路：鉴权、Key、账号、用量。
func TestAdminAPIFull(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, []string{"sk-gw"})

	do := func(method, path, body, token string) (int, map[string]any) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	redis := testredis.Start(t)
	setupBody, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "test-password", "nickname": "管理员", "redis_url": redis.URL()})
	if code, out := do("POST", "/admin/api/setup", string(setupBody), ""); code != http.StatusCreated {
		t.Fatalf("setup: %d %v", code, out)
	}
	code, login := do("POST", "/admin/api/auth/login", `{"email":"admin@example.com","password":"test-password"}`, "")
	if code != http.StatusOK {
		t.Fatalf("login: %d %v", code, login)
	}
	token := login["access_token"].(string)

	// 1. 无 token → 401
	code, _ = do("GET", "/admin/api/overview", "", "")
	if code != http.StatusUnauthorized {
		t.Fatalf("管理接口无凭证应 401，实际 %d", code)
	}

	// 2. 错误 token → 401
	code, _ = do("GET", "/admin/api/overview", "", "wrong")
	if code != http.StatusUnauthorized {
		t.Fatalf("错误凭证应 401，实际 %d", code)
	}

	// 3. 正确 token → overview
	code, out := do("GET", "/admin/api/overview", "", token)
	if code != http.StatusOK {
		t.Fatalf("overview 失败: %d %v", code, out)
	}
	if out["keys_total"].(float64) < 1 {
		t.Fatalf("种子 Key 应已导入: %v", out)
	}

	// 管理 API 提供可导入 Postman/Insomnia 的 OpenAPI 文档。
	code, docs := do("GET", "/admin/api/docs", "", token)
	if code != http.StatusOK {
		t.Fatalf("OpenAPI 文档获取失败: %d %v", code, docs)
	}
	if docs["openapi"] != "3.1.0" {
		t.Fatalf("OpenAPI 版本错误: %v", docs["openapi"])
	}
	paths, ok := docs["paths"].(map[string]any)
	if !ok || paths["/admin/api/keys"] == nil {
		t.Fatalf("OpenAPI 文档缺少 Key 接口: %v", docs["paths"])
	}

	// 4. 创建 Key
	code, out = do("POST", "/admin/api/keys", `{"name":"测试Key"}`, token)
	if code != http.StatusOK {
		t.Fatalf("创建 Key 失败: %d %v", code, out)
	}
	keyObj := out["key"].(map[string]any)
	newKey := keyObj["key"].(string)
	if !strings.HasPrefix(newKey, "sk-") {
		t.Fatalf("Key 格式错误: %s", newKey)
	}

	// 5. 用新 Key 调网关（鉴权走库）
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+newKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("新 Key 应可用: %d %s", rec.Code, rec.Body.String())
	}

	// 6. 添加 Gemini 号（异步初始化，不入池失败也不影响记录）
	code, out = do("POST", "/admin/api/accounts/gemini",
		`{"label":"主号","psid":"test-psid","psidts":"test-psidts"}`, token)
	if code != http.StatusOK {
		t.Fatalf("添加 Gemini 号失败: %d %v", code, out)
	}
	accObj := out["account"].(map[string]any)
	accID := int64(accObj["id"].(float64))

	// 7. 账号列表包含新账号
	code, out = do("GET", "/admin/api/accounts", "", token)
	if code != http.StatusOK {
		t.Fatalf("账号列表失败: %d", code)
	}
	accounts := out["accounts"].([]any)
	if len(accounts) != 1 {
		t.Fatalf("应有 1 个账号: %v", accounts)
	}

	// 8. 停用账号
	code, _ = do("PATCH", "/admin/api/accounts/1", `{"enabled":false}`, token)
	if code != http.StatusOK {
		t.Fatalf("停用失败: %d", code)
	}

	// 9. 删除账号
	code, _ = do("DELETE", "/admin/api/accounts/1", "", token)
	if code != http.StatusOK {
		t.Fatalf("删除失败: %d", code)
	}
	_ = accID

	// 10. 用量（第 5 步产生一条成功记录）
	code, out = do("GET", "/admin/api/usage?days=1", "", token)
	if code != http.StatusOK {
		t.Fatalf("用量查询失败: %d", code)
	}
	usage := out["usage"].([]any)
	if len(usage) != 1 {
		t.Fatalf("应有 1 条用量记录: %v", usage)
	}
	row := usage[0].(map[string]any)
	if row["api_key"] != newKey {
		t.Fatalf("用量 Key 不符: %v", row)
	}

	// 11. 管理台页面可访问
	req2 := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "号池管理台") {
		t.Fatalf("管理台页面异常: %d", rec2.Code)
	}
}

// TestAuthDisabledMode 无任何 Key 时网关放行（本地模式）。
func TestAuthDisabledMode(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, nil) // 不配置任何 key

	// 无 Authorization 也能对话
	body := `{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("无鉴权模式应放行: %d %s", rec.Code, rec.Body.String())
	}

	// 但一旦创建 Key，旧模式关闭：需要 Key
	st := srv.st
	if _, err := st.CreateKey("first"); err != nil {
		t.Fatal(err)
	}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("已有 Key 后无凭证应 401: %d", rec2.Code)
	}
}
