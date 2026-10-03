package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"web2api/internal/session"
	"web2api/internal/store"
)

const sessionLifetime = 8 * time.Hour
const loginAttemptScript = `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],900) end; return n`

type claims struct {
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	Subject   string `json:"sub"`
	ID        string `json:"jti"`
	IssuedAt  int64  `json:"iat"`
	NotBefore int64  `json:"nbf"`
	Expires   int64  `json:"exp"`
}

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func signJWT(c claims, secret string) (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(data)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil)), nil
}

func verifyJWT(token, secret string) (*claims, error) {
	invalid := errors.New("JWT 无效或已过期")
	if len(token) > 4096 {
		return nil, invalid
	}
	p := strings.Split(token, ".")
	if len(p) != 3 {
		return nil, invalid
	}
	h, err := base64.RawURLEncoding.DecodeString(p[0])
	if err != nil {
		return nil, invalid
	}
	var header struct {
		Alg  string `json:"alg"`
		Type string `json:"typ"`
	}
	if json.Unmarshal(h, &header) != nil || header.Alg != "HS256" || header.Type != "JWT" {
		return nil, invalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(p[2])
	if err != nil {
		return nil, invalid
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(p[0] + "." + p[1]))
	if !hmac.Equal(sig, m.Sum(nil)) {
		return nil, invalid
	}
	data, err := base64.RawURLEncoding.DecodeString(p[1])
	if err != nil {
		return nil, invalid
	}
	c := &claims{}
	if json.Unmarshal(data, c) != nil {
		return nil, invalid
	}
	now := time.Now().Unix()
	if c.Issuer != "web2api" || c.Audience != "web2api-admin" || c.Subject != "1" || c.ID == "" || c.Expires <= now || c.IssuedAt <= 0 || c.IssuedAt > now || c.NotBefore > now || c.Expires <= c.IssuedAt {
		return nil, invalid
	}
	return c, nil
}

func redisPrefix(v *store.AdminSettings) string {
	h := sha256.Sum256([]byte(v.JWTSecret))
	return "web2api:" + hex.EncodeToString(h[:8]) + ":"
}

func (a *API) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	v, err := a.st.AdminSettings()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "读取初始化状态失败"})
		return
	}
	writeJSON(w, 200, map[string]any{"initialized": v != nil})
}

func (a *API) handleSetup(w http.ResponseWriter, r *http.Request) {
	v, err := a.st.AdminSettings()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "读取初始化状态失败"})
		return
	}
	if v != nil {
		writeJSON(w, 409, map[string]any{"error": "系统已经初始化"})
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Nickname string `json:"nickname"`
		RedisURL string `json:"redis_url"`
	}
	if readJSON(r, &input) != nil {
		writeJSON(w, 400, map[string]any{"error": "请求 JSON 无效"})
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Nickname = strings.TrimSpace(input.Nickname)
	input.RedisURL = strings.TrimSpace(input.RedisURL)
	email, e := mail.ParseAddress(input.Email)
	if e != nil || email.Address != input.Email || len(input.Email) > 254 || input.Nickname == "" || utf8.RuneCountInString(input.Nickname) > 64 || utf8.RuneCountInString(input.Password) < 8 || len(input.Password) > 72 {
		writeJSON(w, 400, map[string]any{"error": "请填写有效邮箱、1–64 字昵称和至少 8 字密码（密码最多 72 字节）"})
		return
	}
	redis, err := session.New(input.RedisURL)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	// Verify both connectivity and session write/read/delete permissions before committing setup.
	probe, err := randomSecret()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "生成初始化参数失败"})
		return
	}
	key := "web2api:setup:" + probe
	if _, err = redis.Command(r.Context(), "SET", key, probe, "EX", "30"); err == nil {
		var got string
		got, err = redis.Command(r.Context(), "GET", key)
		if err == nil && got != probe {
			err = errors.New("Redis 校验失败")
		}
		_, cleanupErr := redis.Command(r.Context(), "DEL", key)
		if err == nil {
			err = cleanupErr
		}
	}
	if err == nil {
		// Login throttling requires EVAL, INCR and EXPIRE permissions too.
		_, err = redis.Command(r.Context(), "EVAL", loginAttemptScript, "1", key+":limits")
		_, cleanupErr := redis.Command(r.Context(), "DEL", key+":limits")
		if err == nil {
			err = cleanupErr
		}
	}
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "Redis 连接或读写验证失败，请检查地址、认证和权限"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "密码处理失败"})
		return
	}
	secret, err := randomSecret()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "生成 JWT 密钥失败"})
		return
	}
	err = a.st.InitializeAdmin(&store.AdminSettings{Email: input.Email, Nickname: input.Nickname, PasswordHash: string(hash), RedisURL: input.RedisURL, JWTSecret: secret})
	if errors.Is(err, store.ErrInitialized) {
		writeJSON(w, 409, map[string]any{"error": err.Error()})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "保存初始化配置失败"})
		return
	}
	writeJSON(w, 201, map[string]any{"initialized": true})
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if readJSON(r, &input) != nil || len(input.Password) > 72 || len(input.Email) > 254 {
		writeJSON(w, 400, map[string]any{"error": "登录参数无效"})
		return
	}
	v, err := a.st.AdminSettings()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "读取管理员失败"})
		return
	}
	if v == nil {
		writeJSON(w, 409, map[string]any{"error": "请先初始化系统"})
		return
	}
	redis, err := session.New(v.RedisURL)
	if err != nil {
		writeJSON(w, 503, map[string]any{"error": "Redis 配置无效"})
		return
	}
	// A bounded account-wide counter also limits guesses with varying email addresses.
	limitKey := redisPrefix(v) + "login-attempts"
	n, err := redis.Command(r.Context(), "EVAL", loginAttemptScript, "1", limitKey)
	if err != nil {
		writeJSON(w, 503, map[string]any{"error": "Redis 暂不可用，请稍后重试"})
		return
	}
	count, err := strconv.Atoi(n)
	if err != nil {
		writeJSON(w, 503, map[string]any{"error": "Redis 响应无效"})
		return
	}
	if count > 10 {
		w.Header().Set("Retry-After", "900")
		writeJSON(w, 429, map[string]any{"error": "登录尝试过多，请 15 分钟后重试"})
		return
	}
	passwordOK := bcrypt.CompareHashAndPassword([]byte(v.PasswordHash), []byte(input.Password)) == nil
	if !passwordOK || strings.ToLower(strings.TrimSpace(input.Email)) != v.Email {
		writeJSON(w, 401, map[string]any{"error": "邮箱或密码错误"})
		return
	}
	id, err := randomSecret()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "创建会话失败"})
		return
	}
	now := time.Now().Unix()
	c := claims{Issuer: "web2api", Audience: "web2api-admin", Subject: "1", ID: id, IssuedAt: now, NotBefore: now, Expires: now + int64(sessionLifetime.Seconds())}
	token, err := signJWT(c, v.JWTSecret)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "创建 JWT 失败"})
		return
	}
	if _, err = redis.Command(r.Context(), "SET", redisPrefix(v)+"session:"+id, c.Subject, "EX", strconv.FormatInt(int64(sessionLifetime.Seconds()), 10)); err != nil {
		writeJSON(w, 503, map[string]any{"error": "保存登录会话失败"})
		return
	}
	_, _ = redis.Command(r.Context(), "DEL", limitKey)
	writeJSON(w, 200, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": int64(sessionLifetime.Seconds()), "user": v})
}

func (a *API) authenticate(r *http.Request) (*store.AdminSettings, *claims, int, error) {
	v, err := a.st.AdminSettings()
	if err != nil {
		return nil, nil, 500, errors.New("读取管理员失败")
	}
	if v == nil {
		return nil, nil, 401, errors.New("请先初始化系统")
	}
	c, err := verifyJWT(bearer(r), v.JWTSecret)
	if err != nil {
		return nil, nil, 401, err
	}
	redis, err := session.New(v.RedisURL)
	if err != nil {
		return nil, nil, 503, errors.New("Redis 配置无效")
	}
	sub, err := redis.Command(r.Context(), "GET", redisPrefix(v)+"session:"+c.ID)
	if err != nil {
		return nil, nil, 503, errors.New("Redis 暂不可用，请稍后重试")
	}
	if sub != c.Subject {
		return nil, nil, 401, errors.New("登录已失效，请重新登录")
	}
	return v, c, 200, nil
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	v, _, code, err := a.authenticate(r)
	if err != nil {
		writeJSON(w, code, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"user": v})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	v, c, code, err := a.authenticate(r)
	if err != nil {
		writeJSON(w, code, map[string]any{"error": err.Error()})
		return
	}
	redis, _ := session.New(v.RedisURL)
	if _, err = redis.Command(r.Context(), "DEL", redisPrefix(v)+"session:"+c.ID); err != nil {
		writeJSON(w, 503, map[string]any{"error": "退出失败，请稍后重试"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
