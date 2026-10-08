//go:build web2api_unit

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"web2api/internal/store"
)

// 用户 Key：从用户余额扣费；余额不足 402；用户停用 403；不计费接口默认不开放。
func TestGatewayChargesUserBalance(t *testing.T) {
	up := mockEngineB(t) // total_tokens=2
	srv := newTestServer(t, up.URL, []string{"sk-gw"})
	u, err := srv.st.CreateUser(store.NewUser{Email: "u@example.com", PasswordHash: "x", Multiplier: 1.5, InitialBalance: 4})
	if err != nil {
		t.Fatal(err)
	}
	k, err := srv.st.CreateUserKey(u.ID, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.st.SetModelMultiplier("gemini-3.7-flash", 2); err != nil {
		t.Fatal(err)
	}
	body := `{"model":"gemini-3.7-flash","messages":[{"role":"user","content":"hi"}]}`
	rec := chatAs(t, srv, k.Key, body)
	// 2 × 2 × 1 × 1.5 = 6 → 余额 4-6 = -2（跨线请求完整返回）
	if rec.Code != http.StatusOK || rec.Header().Get("X-Web2api-Charged-Tokens") != "6" || rec.Header().Get("X-Web2api-Balance") != "-2" {
		t.Fatalf("首个请求: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	rec = chatAs(t, srv, k.Key, body)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("余额不足应 402: %d %s", rec.Code, rec.Body.String())
	}
	var e struct {
		Error struct{ Type, Code, Message string } `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	if e.Error.Type != "insufficient_quota" || e.Error.Code != "insufficient_balance" || !strings.Contains(e.Error.Message, "余额不足") {
		t.Fatalf("错误体: %+v", e.Error)
	}
	entries, total, _, _ := srv.st.ListLedger(store.LedgerFilter{UserID: u.ID, Kind: store.LedgerUsage})
	if total != 1 || entries[0].Amount != -6 || entries[0].BalanceBefore != 4 {
		t.Fatalf("usage 流水: %d %+v", total, entries)
	}
	// 充值后恢复
	if _, err := srv.st.AdjustBalance(u.ID, 100, "top up", "admin"); err != nil {
		t.Fatal(err)
	}
	if rec = chatAs(t, srv, k.Key, body); rec.Code != http.StatusOK || rec.Header().Get("X-Web2api-Balance") != "92" {
		t.Fatalf("充值后: %d %v", rec.Code, rec.Header())
	}
	// 视频：未配置单价时用户 Key 不可用；配置后可进入（mock 上游不支持视频，只验证不再是 403 endpoint_not_allowed）
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"veo-3","prompt":"x","size":"4k","seconds":8}`))
	req.Header.Set("Authorization", "Bearer "+k.Key)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "endpoint_not_allowed") {
		t.Fatalf("用户 Key 视频默认关闭: %d %s", w.Code, w.Body.String())
	}
	// Gemini 原生（不计 Token）默认不向用户 Key 开放；无归属 Key 不受影响
	gr := httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	gr.Header.Set("x-goog-api-key", k.Key)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, gr)
	if w.Code != http.StatusForbidden {
		t.Fatalf("用户 Key 调 Gemini 原生应 403: %d %s", w.Code, w.Body.String())
	}
	gr = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	gr.Header.Set("x-goog-api-key", "sk-gw")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, gr)
	if w.Code == http.StatusForbidden {
		t.Fatalf("config Key 调 Gemini 原生不应被拦: %d %s", w.Code, w.Body.String())
	}
	// 停用用户
	disabled := false
	if _, err := srv.st.UpdateUser(u.ID, store.UserUpdate{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if rec = chatAs(t, srv, k.Key, body); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "account_disabled") {
		t.Fatalf("停用用户应 403: %d %s", rec.Code, rec.Body.String())
	}
	// config Key 行为不变
	if rec = chatAs(t, srv, "sk-gw", body); rec.Code != http.StatusOK || rec.Header().Get("X-Web2api-Balance") != "" {
		t.Fatalf("config Key: %d %v", rec.Code, rec.Header())
	}
	if bad, _ := srv.st.LedgerIntegrity(); len(bad) != 0 {
		t.Fatalf("ledger integrity: %v", bad)
	}
}
