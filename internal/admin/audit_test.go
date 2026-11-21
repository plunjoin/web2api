package admin

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"web2api/internal/logging"
	"web2api/internal/store"
)

func newAuditTestAPI(t *testing.T) (*API, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &API{st: st, logger: logging.NewTest(io.Discard)}, st
}

func TestAuditedMiddlewareRecordsSuccess(t *testing.T) {
	a, st := newAuditTestAPI(t)
	handler := a.audited("user.balance", func(w http.ResponseWriter, r *http.Request) {
		setAudit(r, "user:7 user@example.com", "调整余额 +10.00 元")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	r := httptest.NewRequest(http.MethodPost, "/admin/api/users/7/balance", nil)
	r = r.WithContext(withPrincipal(r.Context(), &Principal{Root: &store.AdminSettings{Email: "root@example.com"}, Email: "root@example.com"}))
	handler.ServeHTTP(httptest.NewRecorder(), r)

	logs, total, err := st.ListAuditLogs(store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("应写入 1 条审计记录，got total=%d", total)
	}
	got := logs[0]
	if got.Actor != "root@example.com" || got.ActorKind != "root" || got.Action != "user.balance" || got.Result != "ok" {
		t.Fatalf("审计字段不符: %+v", got)
	}
	if got.Target != "user:7 user@example.com" || got.Detail != "调整余额 +10.00 元" {
		t.Fatalf("目标/摘要不符: %+v", got)
	}
}

func TestAuditedMiddlewareRecordsFailure(t *testing.T) {
	a, st := newAuditTestAPI(t)
	handler := a.audited("account.delete", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "账号不存在"})
	})
	r := httptest.NewRequest(http.MethodDelete, "/admin/api/accounts/9", nil)
	r = r.WithContext(withPrincipal(r.Context(), &Principal{Root: &store.AdminSettings{Email: "root@example.com"}, Email: "root@example.com"}))
	handler.ServeHTTP(httptest.NewRecorder(), r)

	logs, _, err := st.ListAuditLogs(store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("应写入 1 条审计记录，got %d", len(logs))
	}
	if logs[0].Result != "fail" || logs[0].Error != "账号不存在" {
		t.Fatalf("失败审计应带上错误原因: %+v", logs[0])
	}
	if logs[0].Target != "DELETE /admin/api/accounts/9" {
		t.Fatalf("未补充 target 时应回退到方法+路径: %q", logs[0].Target)
	}
}

func TestLoginAuditDoesNotStorePassword(t *testing.T) {
	a, st := newAuditTestAPI(t)
	r := httptest.NewRequest(http.MethodPost, "/admin/api/auth/login", nil)
	a.auditLogin(r, "admin@example.com", "root", errAuditBadPassword)
	logs, _, err := st.ListAuditLogs(store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Action != "auth.login" || logs[0].Result != "fail" {
		t.Fatalf("登录失败审计不符: %+v", logs)
	}
	if logs[0].Actor != "admin@example.com" || logs[0].Error != "邮箱或密码错误" {
		t.Fatalf("登录审计字段不符: %+v", logs[0])
	}
}

func TestLogoutAuditRecordsSuccess(t *testing.T) {
	a, st := newAuditTestAPI(t)
	const secret = "audit-test-jwt-secret"
	redisURL := startAuditTestRedis(t, rootSubject, false)
	if err := st.InitializeAdmin(&store.AdminSettings{
		Email: "root@example.com", Nickname: "root", PasswordHash: "unused",
		RedisURL: redisURL, JWTSecret: secret,
	}); err != nil {
		t.Fatal(err)
	}
	token := auditTestToken(t, secret, rootSubject, "session-1")
	r := httptest.NewRequest(http.MethodPost, "/admin/api/auth/logout", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	a.Mount(mux)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	logs, total, err := st.ListAuditLogs(store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("退出登录应只写 1 条审计记录，got total=%d", total)
	}
	got := logs[0]
	if got.Action != "auth.logout" || got.Actor != "root@example.com" || got.ActorKind != "root" ||
		got.Target != "session" || got.Detail != "退出登录" || got.Result != "ok" || got.Error != "" {
		t.Fatalf("退出登录审计字段不符: %+v", got)
	}
}

func TestLogoutAuditRecordsFailure(t *testing.T) {
	a, st := newAuditTestAPI(t)
	const secret = "audit-test-jwt-secret"
	redisURL := startAuditTestRedis(t, rootSubject, true)
	if err := st.InitializeAdmin(&store.AdminSettings{
		Email: "root@example.com", Nickname: "root", PasswordHash: "unused",
		RedisURL: redisURL, JWTSecret: secret,
	}); err != nil {
		t.Fatal(err)
	}
	token := auditTestToken(t, secret, rootSubject, "session-1")
	r := httptest.NewRequest(http.MethodPost, "/admin/api/auth/logout", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	a.Mount(mux)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	logs, total, err := st.ListAuditLogs(store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("退出失败应只写 1 条审计记录，got total=%d", total)
	}
	got := logs[0]
	if got.Action != "auth.logout" || got.Actor != "root@example.com" || got.ActorKind != "root" ||
		got.Result != "fail" || got.Error == "" {
		t.Fatalf("退出失败审计字段不符: %+v", got)
	}
}

func TestUserKeyCreateAuditDoesNotStoreSecret(t *testing.T) {
	a, st := newAuditTestAPI(t)
	const secret = "audit-test-jwt-secret"
	u, err := st.CreateUser(store.NewUser{
		Email: "user@example.com", Nickname: "user", PasswordHash: "unused", Role: store.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	subject := "u:" + strconv.FormatInt(u.ID, 10)
	redisURL := startAuditTestRedis(t, subject, false)
	if err := st.InitializeAdmin(&store.AdminSettings{
		Email: "root@example.com", Nickname: "root", PasswordHash: "unused",
		RedisURL: redisURL, JWTSecret: secret,
	}); err != nil {
		t.Fatal(err)
	}
	token := auditTestToken(t, secret, subject, "session-user")
	r := httptest.NewRequest(http.MethodPost, "/api/user/keys", strings.NewReader(`{"name":"prod","cost_limit":1000000}`))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	a.Mount(mux)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Key store.APIKey `json:"key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Key.Key == "" {
		t.Fatal("创建 Key 的响应缺少 key")
	}

	logs, total, err := st.ListAuditLogs(store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("创建 Key 应只写 1 条审计记录，got total=%d", total)
	}
	got := logs[0]
	if got.Action != "user.key.create" || got.Actor != u.Email || got.ActorKind != "user" || got.Result != "ok" {
		t.Fatalf("创建 Key 审计字段不符: %+v", got)
	}
	if got.Target != "key:"+strconv.FormatInt(body.Key.ID, 10) || got.Detail != "创建 Key prod" {
		t.Fatalf("创建 Key 审计目标/摘要不符: %+v", got)
	}
	assertAuditHasNoSecret(t, logs, body.Key.Key)
}

func TestUserRedeemAuditDoesNotStoreCode(t *testing.T) {
	a, st := newAuditTestAPI(t)
	u, err := st.CreateUser(store.NewUser{
		Email: "user@example.com", Nickname: "user", PasswordHash: "unused", Role: store.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	codes, err := st.CreateRedeemCodes(store.NewCodes{Amount: 2_000_000, Count: 1, Batch: "audit-test", Operator: "root@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	bodyBytes, _ := json.Marshal(map[string]string{"code": codes[0].Code})
	p := &Principal{User: &u}
	r := httptest.NewRequest(http.MethodPost, "/api/user/redeem", strings.NewReader(string(bodyBytes)))
	r = r.WithContext(withPrincipal(r.Context(), p))
	w := httptest.NewRecorder()
	a.auditedAP("user.redeem", a.handleUserRedeem)(w, r, p)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	logs, total, err := st.ListAuditLogs(store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("兑换应只写 1 条审计记录，got total=%d", total)
	}
	got := logs[0]
	if got.Action != "user.redeem" || got.Actor != u.Email || got.ActorKind != "user" || got.Result != "ok" {
		t.Fatalf("兑换审计字段不符: %+v", got)
	}
	if got.Target != "redeem" || got.Detail != "兑换充值码（金额 2.00 元）" {
		t.Fatalf("兑换审计目标/摘要不符: %+v", got)
	}
	assertAuditHasNoSecret(t, logs, codes[0].Code)
}

func auditTestToken(t *testing.T, secret, subject, id string) string {
	t.Helper()
	now := time.Now().Unix()
	token, err := signJWT(claims{
		Issuer: "web2api", Audience: "web2api-admin", Subject: subject, ID: id,
		IssuedAt: now, NotBefore: now, Expires: now + 3600,
	}, secret)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// startAuditTestRedis 只实现会话校验与退出所需的 GET/DEL，避免测试依赖外部 Redis。
func startAuditTestRedis(t *testing.T, sessionSubject string, failDelete bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				for {
					args, err := readRedisCommand(br)
					if err != nil || len(args) == 0 {
						return
					}
					switch strings.ToUpper(args[0]) {
					case "GET":
						_, _ = io.WriteString(c, "$"+strconv.Itoa(len(sessionSubject))+"\r\n"+sessionSubject+"\r\n")
					case "DEL":
						if failDelete {
							_, _ = io.WriteString(c, "-ERR unavailable\r\n")
							return
						}
						_, _ = io.WriteString(c, ":1\r\n")
					default:
						_, _ = io.WriteString(c, "+OK\r\n")
					}
				}
			}(conn)
		}
	}()
	return "redis://" + ln.Addr().String() + "/0"
}

func readRedisCommand(br *bufio.Reader) ([]string, error) {
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if !strings.HasPrefix(line, "*") {
		return nil, errors.New("invalid RESP array")
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil || n < 0 {
		return nil, errors.New("invalid RESP array length")
	}
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		header, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		header = strings.TrimSuffix(strings.TrimSuffix(header, "\n"), "\r")
		if !strings.HasPrefix(header, "$") {
			return nil, errors.New("invalid RESP bulk header")
		}
		size, err := strconv.Atoi(header[1:])
		if err != nil || size < 0 {
			return nil, errors.New("invalid RESP bulk length")
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(br, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:size]))
	}
	return args, nil
}

func assertAuditHasNoSecret(t *testing.T, logs []store.AuditEntry, secret string) {
	t.Helper()
	if secret == "" {
		t.Fatal("测试 secret 不能为空")
	}
	for _, entry := range logs {
		for name, value := range map[string]string{
			"actor": entry.Actor, "target": entry.Target, "detail": entry.Detail, "error": entry.Error,
		} {
			if strings.Contains(value, secret) {
				t.Fatalf("审计字段 %s 泄露明文 secret: %q", name, value)
			}
		}
	}
}
