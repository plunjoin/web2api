//go:build web2api_unit

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"web2api/internal/store"
	"web2api/test/support/testredis"
)

func chatAs(t *testing.T, srv *Server, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// 额度在请求入口强制：用尽后 429 insufficient_quota；扣减 = ceil(total × 模型倍率 × Key 倍率)。
func TestGatewayTokenQuotaEnforced(t *testing.T) {
	up := mockEngineB(t) // 非流式上游返回 usage total_tokens=2
	srv := newTestServer(t, up.URL, []string{"sk-gw"})
	k, err := srv.st.CreateKey("limited")
	if err != nil {
		t.Fatal(err)
	}
	limit := int64(5)
	if _, err := srv.st.UpdateKey(k.ID, store.KeyUpdate{TokenLimit: &limit}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.st.SetModelMultiplier("gemini-3.7-flash", 2); err != nil {
		t.Fatal(err)
	}
	body := `{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"}]}`

	rec := chatAs(t, srv, k.Key, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("第 1 次应成功: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Web2api-Usage-Source") != "upstream" || rec.Header().Get("X-Web2api-Charged-Tokens") != "4" || rec.Header().Get("X-Web2api-Multiplier") != "2" {
		t.Fatalf("计费响应头错误: %v", rec.Header())
	}
	var resp struct {
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Usage.TotalTokens != 2 {
		t.Fatalf("应透传上游真实 usage(total=2): %+v", resp.Usage)
	}
	if rec = chatAs(t, srv, k.Key, body); rec.Code != http.StatusOK { // used 4 < 5，仍放行
		t.Fatalf("第 2 次应成功: %d", rec.Code)
	}
	rec = chatAs(t, srv, k.Key, body) // used 8 ≥ 5
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("额度用尽应 429: %d %s", rec.Code, rec.Body.String())
	}
	var errResp struct {
		Error struct{ Message, Type, Code string } `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Type != "insufficient_quota" || errResp.Error.Code != "token_quota_exceeded" ||
		!strings.Contains(errResp.Error.Message, "used 8 of 5") {
		t.Fatalf("额度错误体不清晰: %s", rec.Body.String())
	}
	// 流式请求同样在入口被拒（不会先返回 200 再报错）
	if rec = chatAs(t, srv, k.Key, strings.Replace(body, `"messages"`, `"stream":true,"messages"`, 1)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("流式请求也应 429: %d", rec.Code)
	}
	// 其他 Key 不受影响
	if rec = chatAs(t, srv, "sk-gw", body); rec.Code != http.StatusOK {
		t.Fatalf("不限额 Key 应可用: %d", rec.Code)
	}

	// 重置后恢复；流式上游无 usage → 标记估算
	if _, err := srv.st.UpdateKey(k.ID, store.KeyUpdate{ResetUsage: true}); err != nil {
		t.Fatal(err)
	}
	if rec = chatAs(t, srv, k.Key, strings.Replace(body, `"messages"`, `"stream":true,"messages"`, 1)); rec.Code != http.StatusOK {
		t.Fatalf("重置后流式应成功: %d", rec.Code)
	}
	records, _, err := srv.st.ListUsageRecords(store.UsageFilter{KeyID: k.ID})
	if err != nil || len(records) != 3 {
		t.Fatalf("应有 3 条成功明细（429 不记录）: %d %v", len(records), err)
	}
	if latest := records[0]; !latest.Stream || !latest.Estimated || latest.Multiplier != 2 || latest.ChargedTokens != latest.TotalTokens*2 {
		t.Fatalf("流式明细应为估算并按倍率计费: %+v", latest)
	}
	if first := records[2]; first.Estimated || first.TotalTokens != 2 || first.ChargedTokens != 4 {
		t.Fatalf("非流式明细应为上游真实用量: %+v", first)
	}
}

// 管理 API：Key 额度/倍率字段、模型倍率、逐请求明细，且旧的 {enabled} 用法保持兼容。
func TestAdminQuotaAPI(t *testing.T) {
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
	_, login := do("POST", "/admin/api/auth/login", `{"email":"admin@example.com","password":"test-password"}`, "")
	token := login["access_token"].(string)

	code, out := do("POST", "/admin/api/keys", `{"name":"q","token_limit":1000,"multiplier":1.5}`, token)
	if code != http.StatusOK {
		t.Fatalf("创建带额度 Key: %d %v", code, out)
	}
	key := out["key"].(map[string]any)
	if key["token_limit"].(float64) != 1000 || key["multiplier"].(float64) != 1.5 || key["tokens_remaining"].(float64) != 1000 {
		t.Fatalf("创建返回字段错误: %v", key)
	}
	id := int64(key["id"].(float64))
	path := "/admin/api/keys/" + jsonNumber(id)
	if code, out = do("POST", "/admin/api/keys", `{"name":"bad","multiplier":-1}`, token); code != http.StatusBadRequest {
		t.Fatalf("负倍率应 400: %d %v", code, out)
	}
	if code, out = do("PATCH", path, `{}`, token); code != http.StatusBadRequest {
		t.Fatalf("空 PATCH 应 400: %d", code)
	}
	if code, out = do("PATCH", path, `{"enabled":false}`, token); code != http.StatusOK || out["ok"] != true {
		t.Fatalf("旧 enabled PATCH 应兼容: %d %v", code, out)
	}
	if code, out = do("PATCH", path, `{"enabled":true,"token_limit":10,"multiplier":2,"name":"renamed"}`, token); code != http.StatusOK {
		t.Fatalf("PATCH 额度: %d %v", code, out)
	}
	updated := out["key"].(map[string]any)
	if updated["token_limit"].(float64) != 10 || updated["multiplier"].(float64) != 2 || updated["name"] != "renamed" || updated["enabled"] != true {
		t.Fatalf("PATCH 返回错误: %v", updated)
	}
	if code, _ = do("PATCH", "/admin/api/keys/9999", `{"reset_usage":true}`, token); code != http.StatusNotFound {
		t.Fatalf("不存在的 Key 应 404: %d", code)
	}

	if code, out = do("PUT", "/admin/api/multipliers", `{"model":"gemini-3.7-flash","multiplier":3}`, token); code != http.StatusOK {
		t.Fatalf("设置模型倍率: %d %v", code, out)
	}
	if code, _ = do("PUT", "/admin/api/multipliers", `{"model":"*","multiplier":1.2}`, token); code != http.StatusOK {
		t.Fatalf("设置默认倍率: %d", code)
	}
	code, out = do("GET", "/admin/api/multipliers", "", token)
	if code != http.StatusOK || out["default_multiplier"].(float64) != 1.2 || len(out["multipliers"].([]any)) != 2 {
		t.Fatalf("倍率列表: %d %v", code, out)
	}

	// 2 tokens × 3 × 2 = 12 ≥ 10 → 下一次 429
	plain := updated["key"].(string)
	body := `{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"}]}`
	if rec := chatAs(t, srv, plain, body); rec.Code != http.StatusOK || rec.Header().Get("X-Web2api-Charged-Tokens") != "12" {
		t.Fatalf("对话计费: %d %v", rec.Code, rec.Header())
	}
	if rec := chatAs(t, srv, plain, body); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("超额应 429: %d", rec.Code)
	}

	code, out = do("GET", "/admin/api/keys", "", token)
	var found map[string]any
	for _, item := range out["keys"].([]any) {
		if m := item.(map[string]any); int64(m["id"].(float64)) == id {
			found = m
		}
	}
	if found == nil || found["tokens_used"].(float64) != 12 || found["tokens_remaining"].(float64) != 0 || found["quota_exhausted"] != true || found["charged_tokens_24h"].(float64) != 12 {
		t.Fatalf("Key 列表额度字段: %v", found)
	}

	code, out = do("GET", "/admin/api/usage/records?days=1&key_id="+jsonNumber(id), "", token)
	if code != http.StatusOK || out["total"].(float64) != 1 {
		t.Fatalf("逐请求明细: %d %v", code, out)
	}
	record := out["records"].([]any)[0].(map[string]any)
	if record["total_tokens"].(float64) != 2 || record["charged_tokens"].(float64) != 12 || record["multiplier"].(float64) != 6 ||
		record["estimated"] != false || record["key_name"] != "renamed" || strings.Contains(record["key_masked"].(string), plain) {
		t.Fatalf("明细字段: %v", record)
	}
	if _, leaked := record["api_key"]; leaked {
		t.Fatalf("明细不应返回完整 Key: %v", record)
	}

	code, out = do("GET", "/admin/api/usage?days=1", "", token)
	if code != http.StatusOK || out["usage"] == nil || out["breakdown"] == nil || out["totals"].(map[string]any)["charged_tokens"].(float64) != 12 {
		t.Fatalf("用量聚合: %d %v", code, out)
	}
	code, out = do("GET", "/admin/api/overview", "", token)
	if code != http.StatusOK || out["keys_exhausted"].(float64) != 1 || out["charged_tokens_24h"].(float64) != 12 {
		t.Fatalf("总览额度字段: %v", out)
	}

	if code, out = do("PATCH", path, `{"reset_usage":true}`, token); code != http.StatusOK || out["key"].(map[string]any)["tokens_used"].(float64) != 0 {
		t.Fatalf("重置: %d %v", code, out)
	}
	if rec := chatAs(t, srv, plain, body); rec.Code != http.StatusOK {
		t.Fatalf("重置后应恢复: %d", rec.Code)
	}
	if code, _ = do("DELETE", "/admin/api/multipliers/gemini-3.7-flash", "", token); code != http.StatusOK {
		t.Fatalf("删除倍率: %d", code)
	}
	code, docs := do("GET", "/admin/api/docs", "", token)
	paths := docs["paths"].(map[string]any)
	if code != http.StatusOK || paths["/admin/api/multipliers"] == nil || paths["/admin/api/usage/records"] == nil {
		t.Fatal("OpenAPI 缺少额度/倍率接口")
	}
}

func jsonNumber(v int64) string {
	data, _ := json.Marshal(v)
	return string(data)
}
