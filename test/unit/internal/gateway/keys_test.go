//go:build web2api_unit

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"web2api/internal/store"
	"web2api/test/support/testredis"
)

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var body struct {
		Error struct{ Type, Code, Message string } `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error.Type, body.Error.Code
}

func TestGatewayKeyExpiryRPMAndAllowlist(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, []string{"sk-gw"})
	body := `{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"}]}`

	// 过期
	k, _ := srv.st.CreateKey("exp")
	past := time.Now().Add(-time.Minute).Unix()
	if _, err := srv.st.UpdateKey(k.ID, store.KeyUpdate{ExpiresAt: &past}); err != nil {
		t.Fatal(err)
	}
	rec := chatAs(t, srv, k.Key, body)
	if typ, code := errorCode(t, rec); rec.Code != http.StatusUnauthorized || typ != "authentication_error" || code != "key_expired" {
		t.Fatalf("过期 Key 应 401 key_expired: %d %s", rec.Code, rec.Body.String())
	}
	future := time.Now().Add(time.Hour).Unix()
	_, _ = srv.st.UpdateKey(k.ID, store.KeyUpdate{ExpiresAt: &future})
	if rec = chatAs(t, srv, k.Key, body); rec.Code != http.StatusOK {
		t.Fatalf("延期后应可用: %d", rec.Code)
	}

	// RPM
	r, _ := srv.st.CreateKey("rpm")
	rpm := int64(2)
	_, _ = srv.st.UpdateKey(r.ID, store.KeyUpdate{RPMLimit: &rpm})
	codes := []int{}
	for i := 0; i < 3; i++ {
		codes = append(codes, chatAs(t, srv, r.Key, body).Code)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != http.StatusTooManyRequests {
		// 极少数情况下跨越整分钟会重置窗口，重试一轮
		if time.Now().Second() > 1 {
			t.Fatalf("RPM=2 时第 3 次应 429: %v", codes)
		}
	}
	rec = chatAs(t, srv, r.Key, body)
	if rec.Code == http.StatusTooManyRequests {
		if typ, code := errorCode(t, rec); typ != "rate_limit_error" || code != "rpm_limit_exceeded" || rec.Header().Get("Retry-After") == "" {
			t.Fatalf("RPM 错误体/Retry-After 不正确: %s %v", rec.Body.String(), rec.Header())
		}
	}
	if rec = chatAs(t, srv, "sk-gw", body); rec.Code != http.StatusOK {
		t.Fatalf("其他 Key 不受单 Key RPM 影响: %d", rec.Code)
	}

	// 模型白名单
	a, _ := srv.st.CreateKey("allow")
	models := []string{"gemini-3.7-*"}
	_, _ = srv.st.UpdateKey(a.ID, store.KeyUpdate{AllowedModels: &models})
	if rec = chatAs(t, srv, a.Key, body); rec.Code != http.StatusOK {
		t.Fatalf("白名单内模型应放行: %d %s", rec.Code, rec.Body.String())
	}
	rec = chatAs(t, srv, a.Key, strings.Replace(body, "gemini-3.7-flash", "gemini-2.5-pro", 1))
	if typ, code := errorCode(t, rec); rec.Code != http.StatusForbidden || typ != "permission_error" || code != "model_not_allowed" {
		t.Fatalf("白名单外模型应 403 model_not_allowed: %d %s", rec.Code, rec.Body.String())
	}
	do := func(method, path, contentType, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+a.Key)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	// 视频
	rec = do("POST", "/v1/videos", "application/json", `{"model":"veo-3.1-generate-preview","prompt":"cat"}`)
	if _, code := errorCode(t, rec); rec.Code != http.StatusForbidden || code != "model_not_allowed" {
		t.Fatalf("视频接口也应校验白名单: %d %s", rec.Code, rec.Body.String())
	}
	// Gemini 原生：模型在路径中
	rec = do("POST", "/v1beta/models/gemini-2.5-pro:generateContent", "application/json", `{"contents":[]}`)
	if _, code := errorCode(t, rec); rec.Code != http.StatusForbidden || code != "model_not_allowed" {
		t.Fatalf("Gemini 原生路径应校验白名单: %d %s", rec.Code, rec.Body.String())
	}
	rec = do("POST", "/v1beta/models/gemini-3.7-flash:generateContent", "application/json", `{"contents":[]}`)
	if _, code := errorCode(t, rec); code == "model_not_allowed" {
		t.Fatalf("Gemini 原生白名单内模型不应被拒: %s", rec.Body.String())
	}
	// Gemini 原生：模型在 JSON 体中（如 interactions）
	rec = do("POST", "/v1beta/interactions", "application/json", `{"model":"models/gemini-2.5-pro","input":"hi"}`)
	if _, code := errorCode(t, rec); rec.Code != http.StatusForbidden || code != "model_not_allowed" {
		t.Fatalf("请求体中的模型也应校验: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayRecordsLatencyAndKeyID(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, []string{"sk-gw"})
	k, _ := srv.st.CreateKey("lat")
	if rec := chatAs(t, srv, k.Key, `{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"}]}`); rec.Code != 200 {
		t.Fatalf("chat: %d", rec.Code)
	}
	records, _, err := srv.st.ListUsageRecords(store.UsageFilter{KeyID: k.ID})
	if err != nil || len(records) != 1 {
		t.Fatalf("应有 1 条明细: %v %v", records, err)
	}
	if records[0].KeyID != k.ID || records[0].LatencyMs < 0 || records[0].KeyMasked == "" || strings.Contains(records[0].KeyMasked, k.Key) {
		t.Fatalf("明细字段错误: %+v", records[0])
	}
}

func TestAdminExtendedKeyAndUsageAPI(t *testing.T) {
	up := mockEngineB(t)
	srv := newTestServer(t, up.URL, []string{"sk-gw"})
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	do := func(method, path, body, token string) (int, map[string]any) {
		rec := call(method, path, body, token)
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

	exp := time.Now().Add(24 * time.Hour).Unix()
	code, out := do("POST", "/admin/api/keys", `{"name":"ext","expires_at":`+jsonNumber(exp)+`,"allowed_models":["gemini-3.7-*"],"rpm_limit":60}`, token)
	if code != http.StatusOK {
		t.Fatalf("创建: %d %v", code, out)
	}
	key := out["key"].(map[string]any)
	if int64(key["expires_at"].(float64)) != exp || key["rpm_limit"].(float64) != 60 || len(key["allowed_models"].([]any)) != 1 {
		t.Fatalf("创建返回字段错误: %v", key)
	}
	id := jsonNumber(int64(key["id"].(float64)))
	oldKey := key["key"].(string)
	if code, out = do("PATCH", "/admin/api/keys/"+id, `{"rpm_limit":-1}`, token); code != http.StatusBadRequest {
		t.Fatalf("负 rpm_limit 应 400: %d %v", code, out)
	}
	if code, out = do("PATCH", "/admin/api/keys/"+id, `{"allowed_models":[],"expires_at":0}`, token); code != http.StatusOK {
		t.Fatalf("清除白名单/过期: %d %v", code, out)
	}
	if k := out["key"].(map[string]any); len(k["allowed_models"].([]any)) != 0 || k["expires_at"].(float64) != 0 {
		t.Fatalf("清除失败: %v", k)
	}
	// 重新生成
	code, out = do("POST", "/admin/api/keys/"+id+"/regenerate", "", token)
	if code != http.StatusOK || out["key"].(map[string]any)["key"].(string) == oldKey {
		t.Fatalf("重新生成: %d %v", code, out)
	}
	newKey := out["key"].(map[string]any)["key"].(string)
	if rec := chatAs(t, srv, oldKey, `{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"}]}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("旧密钥应 401: %d", rec.Code)
	}
	if rec := chatAs(t, srv, newKey, `{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"}]}`); rec.Code != http.StatusOK {
		t.Fatalf("新密钥应可用: %d", rec.Code)
	}
	if code, _ = do("POST", "/admin/api/keys/99999/regenerate", "", token); code != http.StatusNotFound {
		t.Fatalf("不存在 Key 重新生成应 404: %d", code)
	}
	// 时间序列
	code, out = do("GET", "/admin/api/usage/timeseries?days=1", "", token)
	if code != http.StatusOK || out["bucket"] != "hour" || len(out["points"].([]any)) < 24 {
		t.Fatalf("timeseries: %d %v", code, out)
	}
	var total float64
	for _, p := range out["points"].([]any) {
		total += p.(map[string]any)["requests"].(float64)
	}
	if total != 1 {
		t.Fatalf("timeseries 请求数应为 1: %v", total)
	}
	if code, out = do("GET", "/admin/api/usage/timeseries?days=7", "", token); code != http.StatusOK || out["bucket"] != "day" {
		t.Fatalf("7 天应按日: %d %v", code, out)
	}
	// CSV 导出（Key 脱敏）
	rec := call("GET", "/admin/api/usage/export.csv?days=7", "", token)
	csvText := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(csvText, "charged_tokens") || !strings.Contains(csvText, "gemini-3.7-flash") || strings.Contains(csvText, newKey) {
		t.Fatalf("CSV 导出错误: %d %q", rec.Code, csvText)
	}
	if len(strings.Split(strings.TrimSpace(csvText), "\n")) != 2 {
		t.Fatalf("CSV 应含表头 + 1 行: %q", csvText)
	}
	if call("GET", "/admin/api/usage/export.csv", "", "").Code != http.StatusUnauthorized {
		t.Fatal("CSV 导出必须鉴权")
	}
	// 模型目录与版本
	if code, out = do("GET", "/admin/api/models", "", token); code != http.StatusOK {
		t.Fatalf("models: %d %v", code, out)
	}
	if _, ok := out["models"].([]any); !ok {
		t.Fatalf("models 字段缺失: %v", out)
	}
	if code, out = do("GET", "/admin/api/version", "", token); code != http.StatusOK || out["version"] == "" || out["go_version"] == "" {
		t.Fatalf("version: %d %v", code, out)
	}
	if code, out = do("GET", "/admin/api/overview", "", token); code != http.StatusOK || out["version"] == nil {
		t.Fatalf("overview 应含 version: %d %v", code, out)
	}
}
