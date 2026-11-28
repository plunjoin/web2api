package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Credentials 是 Flow 账号凭据（accounts.credentials JSON）。
//
// Flow 用的是完整 Google Cookie 登录态：既要 flow.google.com 的会话 Cookie
// （OSID / __Secure-OSID），也要 .google.com 的账号 Cookie（SID / HSID /
// SSID / APISID / SAPISID / __Secure-1PSID / __Secure-3PSID 至少一个）。
// 只给账号 Cookie 会被上游重定向到登录页。
type Credentials struct {
	// Cookies 是浏览器导出的 Cookie 集合。接受三种形态：
	//   - Playwright / CDP 的 JSON 数组：[{"name":..,"value":..,"domain":..}]
	//   - 包了一层 {"cookies": [...]} 的 JSON 对象
	//   - 裸 Cookie 头字符串："OSID=xxx; SID=yyy"
	Cookies string `json:"cookies"`
	// Email 账号邮箱（可空，仅用于管理台展示与日志）。
	Email string `json:"email,omitempty"`
	// Proxy 账号级代理，优先于引擎全局代理。
	Proxy string `json:"proxy,omitempty"`
	// ProjectID 已有的项目 ID；为空时引擎会自己建项目。
	ProjectID string `json:"project_id,omitempty"`
}

// flowSessionCookies 是 flow.google.com 的会话 Cookie 名。
var flowSessionCookies = []string{"OSID", "__Secure-OSID"}

// googleAccountCookies 是 .google.com 的账号 Cookie 名。
var googleAccountCookies = []string{
	"SID", "HSID", "SSID", "APISID", "SAPISID",
	"__Secure-1PSID", "__Secure-3PSID",
}

// Validate 校验凭据是否够用。
func (c Credentials) Validate() error {
	if strings.TrimSpace(c.Cookies) == "" {
		return errors.New("cookies 不能为空")
	}
	cookies, err := ParseCookies(c.Cookies)
	if err != nil {
		return err
	}
	return c.validateCookies(cookies)
}

func hasAny(names map[string]bool, candidates []string) bool {
	for _, candidate := range candidates {
		if names[candidate] {
			return true
		}
	}
	return false
}

// CookiePair 是一条 Cookie（名字 + 值）。
type CookiePair struct {
	Name  string
	Value string
}

// Cookie 是带作用域的浏览器 Cookie。
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain,omitempty"`
	Path     string  `json:"path,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	HTTPOnly bool    `json:"httpOnly,omitempty"`
	SameSite string  `json:"sameSite,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
	HostOnly bool    `json:"hostOnly,omitempty"`
	URL      string  `json:"url,omitempty"`
}

// ParseCookies 把三种形态的 Cookie 输入解析成结构化列表。
//
// 裸 Cookie 头字符串解析出来的条目没有作用域，按 flow.google.com 的默认
// 作用域处理——这是「用户从 DevTools 里复制 Cookie 头」这种最常见场景。
func ParseCookies(raw string) ([]Cookie, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("cookies 为空")
	}
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		var payload any
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return nil, fmt.Errorf("cookies 不是合法 JSON: %w", err)
		}
		var entries []any
		switch typed := payload.(type) {
		case []any:
			entries = typed
		case map[string]any:
			if list, ok := typed["cookies"].([]any); ok {
				entries = list
			} else {
				entries = []any{typed}
			}
		}
		out := make([]Cookie, 0, len(entries))
		for _, entry := range entries {
			item, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			name, _ := item["name"].(string)
			value, _ := item["value"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			cookie := Cookie{Name: name, Value: value}
			cookie.Domain, _ = item["domain"].(string)
			cookie.Path, _ = item["path"].(string)
			cookie.Secure, _ = item["secure"].(bool)
			cookie.HTTPOnly, _ = item["httpOnly"].(bool)
			cookie.HostOnly, _ = item["hostOnly"].(bool)
			cookie.SameSite, _ = item["sameSite"].(string)
			if expires, ok := item["expires"].(float64); ok {
				cookie.Expires = expires
			}
			if url, ok := item["url"].(string); ok {
				cookie.URL = url
			}
			out = append(out, cookie)
		}
		if len(out) == 0 {
			return nil, errors.New("cookies 里没有解析出任何条目")
		}
		return out, nil
	}
	// 裸 Cookie 头：name=value; name2=value2（也允许换行分隔）。
	out := make([]Cookie, 0, 8)
	for _, segment := range strings.FieldsFunc(trimmed, func(r rune) bool { return r == ';' || r == '\n' }) {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		name, value, found := strings.Cut(segment, "=")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, Cookie{
			Name:   name,
			Value:  strings.TrimSpace(value),
			Domain: ".google.com",
			Path:   "/",
		})
	}
	if len(out) == 0 {
		return nil, errors.New("cookies 里没有解析出任何条目")
	}
	return out, nil
}

// CookieHeader 构造发给 flow.google.com 的 Cookie 头。
//
// 规则跟浏览器一致：按域/路径过滤、secure 只在 https 下发送、按路径长度
// 从长到短排序（同名 Cookie 可能同时存在于不同路径，长的优先）。
func CookieHeader(cookies []Cookie, targetHost string) string {
	targetHost = strings.ToLower(strings.TrimSpace(targetHost))
	if targetHost == "" {
		targetHost = "flow.google.com"
	}
	type scoped struct {
		path  string
		order int
		pair  CookiePair
	}
	matched := make([]scoped, 0, len(cookies))
	seen := map[string]bool{}
	for index, cookie := range cookies {
		if cookie.Name == "" || seen[cookie.Name+"\x00"+cookie.Value] {
			continue
		}
		if !cookieMatchesHost(cookie, targetHost) {
			continue
		}
		seen[cookie.Name+"\x00"+cookie.Value] = true
		matched = append(matched, scoped{path: cookie.Path, order: index, pair: CookiePair{Name: cookie.Name, Value: cookie.Value}})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if len(matched[i].path) != len(matched[j].path) {
			return len(matched[i].path) > len(matched[j].path)
		}
		return matched[i].order < matched[j].order
	})
	parts := make([]string, 0, len(matched))
	for _, item := range matched {
		parts = append(parts, item.pair.Name+"="+item.pair.Value)
	}
	return strings.Join(parts, "; ")
}

// cookieMatchesHost 判断 Cookie 是否会发给目标主机。
func cookieMatchesHost(cookie Cookie, host string) bool {
	domain := strings.ToLower(strings.TrimSpace(cookie.Domain))
	if domain == "" {
		// 没有域信息：按 flow.google.com 处理（裸 Cookie 头场景）。
		return host == "flow.google.com" || strings.HasSuffix(host, ".google.com")
	}
	domain = strings.TrimPrefix(domain, ".")
	if cookie.HostOnly {
		return domain == host
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// CookieNames 返回 Cookie 名字集合（用于校验与日志）。
func CookieNames(cookies []Cookie) map[string]bool {
	names := make(map[string]bool, len(cookies))
	for _, cookie := range cookies {
		names[cookie.Name] = true
	}
	return names
}

// cookieKey 是同一条 Cookie 的合并键：域、路径、名字。
//
// 同名 Cookie 可以同时存在于 flow.google.com 和 .google.com，也可以
// 挂在不同路径上。刷新时只能覆盖同一作用域的那一条。
func cookieKey(cookie Cookie) string {
	domain := strings.ToLower(strings.TrimSpace(cookie.Domain))
	domain = strings.TrimPrefix(domain, ".")
	path := cookie.Path
	if path == "" {
		path = "/"
	}
	return domain + "\x00" + path + "\x00" + cookie.Name
}

// ApplySetCookies 把响应里的 Set-Cookie 合并进现有集合。
//
// headers 是这一跳的全部 Set-Cookie 头，每条头只含一个 Cookie。
// MaxAge 为负数或过期时间已过表示服务端要清掉这条 Cookie，不是更新它。
// requestHost 是发出这些头的响应主机；头里没有 Domain 时按主机专属处理。
func ApplySetCookies(cookies []Cookie, headers []string, requestHost string) []Cookie {
	parsed := make([]*http.Cookie, 0, len(headers))
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		parsed = append(parsed, (&http.Response{Header: http.Header{"Set-Cookie": {header}}}).Cookies()...)
	}
	if len(parsed) == 0 {
		return cookies
	}
	requestHost = strings.ToLower(strings.TrimSpace(requestHost))
	out := append([]Cookie(nil), cookies...)
	for _, item := range parsed {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		domain := strings.ToLower(strings.TrimSpace(item.Domain))
		hostOnly := domain == ""
		if hostOnly {
			domain = requestHost
		}
		domain = strings.TrimPrefix(domain, ".")
		if !cookieDomainMatchesHost(domain, requestHost) {
			continue
		}
		path := item.Path
		if path == "" {
			path = "/"
		}
		next := Cookie{
			Name:     item.Name,
			Value:    item.Value,
			Domain:   domain,
			Path:     path,
			Secure:   item.Secure,
			HTTPOnly: item.HttpOnly,
			SameSite: sameSiteName(item.SameSite),
			HostOnly: hostOnly,
		}
		if !item.Expires.IsZero() {
			next.Expires = float64(item.Expires.Unix())
		}
		expired := item.MaxAge < 0 || (!item.Expires.IsZero() && !item.Expires.After(time.Now()))
		key := cookieKey(next)
		replaced := false
		kept := make([]Cookie, 0, len(out)+1)
		for _, existing := range out {
			if !sameCookieScope(existing, next, key) {
				kept = append(kept, existing)
				continue
			}
			replaced = true
			if expired {
				continue
			}
			if existing.SameSite != "" && next.SameSite == "" {
				next.SameSite = existing.SameSite
			}
			if existing.HTTPOnly && !next.HTTPOnly {
				next.HTTPOnly = true
			}
			kept = append(kept, next)
		}
		out = kept
		if !replaced && !expired {
			out = append(out, next)
		}
	}
	return out
}

// cookieDomainMatchesHost 按浏览器规则判断响应能否为这个主机写 Cookie。
// 既覆盖 flow.google.com 自己，也覆盖它显式指定的父域 .google.com。
func cookieDomainMatchesHost(domain, host string) bool {
	domain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "."))
	host = strings.ToLower(strings.TrimSpace(host))
	if domain == "" || host == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// sameCookieScope 判断两条 Cookie 是不是同一作用域的同一条。
//
// 从 Cookie 头粘贴进来的条目一律记成 .google.com，并没有真实作用域。
// 服务端后来按具体域下发同名 Cookie 时，用它替换这条占位，避免 jar 里留下两条同名 Cookie。
func sameCookieScope(existing, next Cookie, nextKey string) bool {
	if cookieKey(existing) == nextKey {
		return true
	}
	if existing.Name != next.Name {
		return false
	}
	existingDomain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(existing.Domain)), ".")
	return existingDomain == "google.com" && (existing.Path == "" || existing.Path == "/")
}

// sameSiteName 把标准库的 SameSite 枚举还原成存储用的名字。
func sameSiteName(mode http.SameSite) string {
	switch mode {
	case http.SameSiteStrictMode:
		return "Strict"
	case http.SameSiteLaxMode:
		return "Lax"
	case http.SameSiteNoneMode:
		return "None"
	default:
		return ""
	}
}

// MarshalCookies 把 Cookie 集合编成凭据里存的 JSON 数组。
func MarshalCookies(cookies []Cookie) (string, error) {
	if err := (Credentials{Cookies: "probe"}).validateCookies(cookies); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(cookies)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// validateCookies 校验已解析的 Cookie 是否够组成一份 Flow 登录态。
func (c Credentials) validateCookies(cookies []Cookie) error {
	names := CookieNames(cookies)
	var missing []string
	if !hasAny(names, flowSessionCookies) {
		missing = append(missing, "flow.google.com 会话 Cookie（OSID 或 __Secure-OSID）")
	}
	if !hasAny(names, googleAccountCookies) {
		missing = append(missing, ".google.com 账号 Cookie（SID/HSID/SSID/APISID/SAPISID）")
	}
	if len(missing) > 0 {
		return fmt.Errorf("Google Cookies 不完整，缺少%s", strings.Join(missing, "、"))
	}
	return nil
}
