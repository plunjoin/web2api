package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"web2api/internal/session"
	"web2api/internal/store"
	"web2api/internal/version"
)

// Principal 当前登录主体：根管理员（admin_settings）或平台用户（users 表）。
type Principal struct {
	Root     *store.AdminSettings
	User     *store.User
	Email    string
	Nickname string
}

func rootPrincipal(v *store.AdminSettings) *Principal {
	return &Principal{Root: v, Email: v.Email, Nickname: v.Nickname}
}

// IsAdmin 根管理员或 role=admin 的用户。
func (p *Principal) IsAdmin() bool {
	return p != nil && (p.Root != nil || (p.User != nil && p.User.Role == store.RoleAdmin))
}

// Operator 写入流水 / 兑换码的操作人标识。
func (p *Principal) Operator() string {
	if p == nil {
		return "system"
	}
	if p.User != nil {
		return p.User.Email
	}
	return p.Email
}

// view 前端使用的当前用户信息。旧字段 email、nickname 保持不变。
func (p *Principal) view() map[string]any {
	if p.User != nil {
		u := p.User
		return map[string]any{
			"id": u.ID, "email": u.Email, "nickname": u.Nickname, "role": u.Role, "kind": "user",
			"balance": u.Balance, "multiplier": u.Multiplier, "created_at": u.CreatedAt,
		}
	}
	return map[string]any{"id": 0, "email": p.Email, "nickname": p.Nickname, "role": store.RoleAdmin, "kind": "root"}
}

// clientIP 取来源 IP（反向代理常见头优先，仅用于限流计数）。
func clientIP(r *http.Request) string {
	for _, h := range []string{"CF-Connecting-IP", "X-Real-IP"} {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			return v
		}
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func hashPart(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:8])
}

// bump 计数器 +1（窗口 15 分钟），返回当前值。
func bump(r *http.Request, redis *session.Redis, key string) (int, error) {
	n, err := redis.Command(r.Context(), "EVAL", loginAttemptScript, "1", key)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(n)
}

// issueSession 签发 JWT 并写入 Redis 会话。
func issueSession(r *http.Request, v *store.AdminSettings, redis *session.Redis, subject string, tokenVersion int64) (string, error) {
	id, err := randomSecret()
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	c := claims{Issuer: "web2api", Audience: "web2api-admin", Subject: subject, ID: id, IssuedAt: now, NotBefore: now,
		Expires: now + int64(sessionLifetime.Seconds()), Version: tokenVersion}
	token, err := signJWT(c, v.JWTSecret)
	if err != nil {
		return "", err
	}
	if _, err = redis.Command(r.Context(), "SET", redisPrefix(v)+"session:"+id, c.Subject, "EX", strconv.FormatInt(int64(sessionLifetime.Seconds()), 10)); err != nil {
		return "", err
	}
	return token, nil
}

func (a *API) loginPlatformUser(w http.ResponseWriter, r *http.Request, v *store.AdminSettings, redis *session.Redis, email, password string) {
	emailKey := redisPrefix(v) + "login-attempts:u:" + hashPart(email)
	ipKey := redisPrefix(v) + "login-attempts:ip:" + hashPart(clientIP(r))
	byEmail, err := bump(r, redis, emailKey)
	if err != nil {
		writeJSON(w, 503, map[string]any{"error": "Redis 暂不可用，请稍后重试"})
		return
	}
	byIP, err := bump(r, redis, ipKey)
	if err != nil {
		writeJSON(w, 503, map[string]any{"error": "Redis 暂不可用，请稍后重试"})
		return
	}
	if byEmail > 10 || byIP > 60 {
		w.Header().Set("Retry-After", "900")
		writeJSON(w, 429, map[string]any{"error": "登录尝试过多，请 15 分钟后重试"})
		return
	}
	user, err := a.st.GetUserByEmail(email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeJSON(w, 500, map[string]any{"error": "读取用户失败"})
		return
	}
	hash := user.PasswordHash
	if hash == "" {
		hash = dummyHash // 用户不存在时也做一次 bcrypt，避免通过耗时探测邮箱是否注册
	}
	passwordOK := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	if err != nil || !passwordOK {
		writeJSON(w, 401, map[string]any{"error": "邮箱或密码错误"})
		return
	}
	if !user.Enabled {
		writeJSON(w, 403, map[string]any{"error": "账号已停用，请联系管理员"})
		return
	}
	token, err := issueSession(r, v, redis, "u:"+strconv.FormatInt(user.ID, 10), user.TokenVersion)
	if err != nil {
		writeJSON(w, 503, map[string]any{"error": "保存登录会话失败"})
		return
	}
	_, _ = redis.Command(r.Context(), "DEL", emailKey)
	a.st.TouchUserLogin(user.ID)
	writeJSON(w, 200, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": int64(sessionLifetime.Seconds()),
		"user": (&Principal{User: &user}).view()})
}

// dummyHash 是一个固定口令的 bcrypt 哈希，仅用于对齐不存在用户的登录耗时。
var dummyHash = func() string {
	h, _ := bcrypt.GenerateFromPassword([]byte("web2api-timing-equalizer"), bcrypt.DefaultCost)
	return string(h)
}()

func validPassword(password string) error {
	if utf8.RuneCountInString(password) < 8 || len(password) > 72 {
		return errors.New("密码至少 8 个字符，最多 72 字节")
	}
	return nil
}

// handleRegister POST /api/auth/register {email, password, nickname}：开放注册时创建普通用户并直接登录。
func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	v, err := a.st.AdminSettings()
	if err != nil || v == nil {
		writeJSON(w, 409, map[string]any{"error": "系统尚未初始化"})
		return
	}
	settings, err := a.st.GetPlatformSettings()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "读取平台设置失败"})
		return
	}
	if !settings.RegistrationOpen {
		writeJSON(w, 403, map[string]any{"error": "当前未开放注册，请联系管理员开通账号"})
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Nickname string `json:"nickname"`
	}
	if readJSON(r, &input) != nil {
		writeJSON(w, 400, map[string]any{"error": "请求 JSON 无效"})
		return
	}
	input.Email = store.NormalizeEmail(input.Email)
	if err := store.ValidateEmail(input.Email); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	if err := validPassword(input.Password); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	if input.Email == v.Email {
		writeJSON(w, 409, map[string]any{"error": store.ErrEmailTaken.Error()})
		return
	}
	redis, err := session.New(v.RedisURL)
	if err != nil {
		writeJSON(w, 503, map[string]any{"error": "Redis 配置无效"})
		return
	}
	n, err := bump(r, redis, redisPrefix(v)+"register:ip:"+hashPart(clientIP(r)))
	if err != nil {
		writeJSON(w, 503, map[string]any{"error": "Redis 暂不可用，请稍后重试"})
		return
	}
	if n > 10 {
		w.Header().Set("Retry-After", "900")
		writeJSON(w, 429, map[string]any{"error": "注册过于频繁，请稍后再试"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "密码处理失败"})
		return
	}
	user, err := a.st.CreateUser(store.NewUser{
		Email: input.Email, Nickname: input.Nickname, PasswordHash: string(hash), Role: store.RoleUser,
		Multiplier: settings.DefaultUserMultiplier, InitialBalance: settings.SignupBonus, InitialKind: store.LedgerSignupBonus,
		InitialNote: "注册赠送", Operator: "system",
	})
	if errors.Is(err, store.ErrEmailTaken) {
		writeJSON(w, 409, map[string]any{"error": err.Error()})
		return
	}
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	token, err := issueSession(r, v, redis, "u:"+strconv.FormatInt(user.ID, 10), user.TokenVersion)
	if err != nil {
		writeJSON(w, 201, map[string]any{"user": (&Principal{User: &user}).view()})
		return
	}
	a.st.TouchUserLogin(user.ID)
	writeJSON(w, 201, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": int64(sessionLifetime.Seconds()),
		"user": (&Principal{User: &user}).view()})
}

// handlePublicConfig GET /api/public/config：登录/注册页需要的公开信息（不含任何敏感配置）。
func (a *API) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	v, _ := a.st.AdminSettings()
	settings, _ := a.st.GetPlatformSettings()
	writeJSON(w, 200, map[string]any{
		"initialized":       v != nil,
		"registration_open": settings.RegistrationOpen,
		"signup_bonus":      settings.SignupBonus,
		"site_name":         settings.SiteName,
		"version":           version.Get().Version,
	})
}

// userAuth 平台用户接口鉴权（根管理员没有个人余额，不能使用用户控制台）。
func (a *API) userAuth(next func(http.ResponseWriter, *http.Request, *Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _, p, code, err := a.authenticateAny(r)
		if err != nil {
			writeJSON(w, code, map[string]any{"error": err.Error()})
			return
		}
		if p.User == nil {
			writeJSON(w, 403, map[string]any{"error": "初始化管理员没有个人余额，请使用普通用户账号登录控制台"})
			return
		}
		next(w, r, p)
	}
}

// adminAuth 管理接口鉴权并传入操作人。
func (a *API) adminAuth(next func(http.ResponseWriter, *http.Request, *Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _, p, code, err := a.authenticateAny(r)
		if err != nil {
			writeJSON(w, code, map[string]any{"error": err.Error()})
			return
		}
		if !p.IsAdmin() {
			writeJSON(w, 403, map[string]any{"error": "需要管理员权限"})
			return
		}
		next(w, r, p)
	}
}
