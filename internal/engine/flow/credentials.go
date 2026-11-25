package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	pairs, err := ParseCookies(c.Cookies)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for _, pair := range pairs {
		names[pair.Name] = true
	}
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
