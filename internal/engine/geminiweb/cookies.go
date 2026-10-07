package geminiweb

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Cookie 结构（与 gemini_webapi 缓存文件格式一致）。
type Cookie struct {
	Name    string  `json:"name"`
	Value   string  `json:"value"`
	Domain  string  `json:"domain"`
	Path    string  `json:"path"`
	Expires float64 `json:"expires,omitempty"`
}

// cookieJar 简易 Cookie 集合（按名字索引，Domain 统一 .google.com；自身线程安全）。
type cookieJar struct {
	mu      sync.Mutex
	cookies map[string]*Cookie
}

func newCookieJar() *cookieJar {
	return &cookieJar{cookies: map[string]*Cookie{}}
}

// Set 添加 Cookie。
func (j *cookieJar) Set(name, value string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cookies[name] = &Cookie{
		Name:   name,
		Value:  value,
		Domain: ".google.com",
		Path:   "/",
	}
}

// SetFull 添加带完整属性的 Cookie。
func (j *cookieJar) SetFull(c Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cookies[c.Name] = &c
}

// Get 获取 Cookie。
func (j *cookieJar) Get(name string) string {
	j.mu.Lock()
	defer j.mu.Unlock()
	if c, ok := j.cookies[name]; ok {
		return c.Value
	}
	return ""
}

// Has 判断是否存在。
func (j *cookieJar) Has(name string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	_, ok := j.cookies[name]
	return ok
}

// Header 生成 Cookie 请求头。
func (j *cookieJar) Header() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	var parts []string
	for _, c := range j.cookies {
		if c.Value == "" {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// ApplyToRequest 把 Cookie 写入 http.Request。
func (j *cookieJar) ApplyToRequest(req *http.Request) {
	if value := j.Header(); value != "" {
		req.Header.Set("Cookie", value)
	}
}

// UpdateFromResponse 从响应 Set-Cookie 中更新 Cookie。
func (j *cookieJar) UpdateFromResponse(resp *http.Response) {
	for _, c := range resp.Cookies() {
		if c.MaxAge < 0 {
			j.mu.Lock()
			delete(j.cookies, c.Name)
			j.mu.Unlock()
			continue
		}
		cc := &Cookie{
			Name:   c.Name,
			Value:  c.Value,
			Domain: c.Domain,
			Path:   c.Path,
		}
		if cc.Domain == "" && resp.Request != nil {
			cc.Domain = resp.Request.URL.Hostname()
		}
		if cc.Path == "" {
			cc.Path = "/"
		}
		if !c.Expires.IsZero() {
			cc.Expires = float64(c.Expires.Unix())
		}
		j.mu.Lock()
		j.cookies[c.Name] = cc
		j.mu.Unlock()
	}
}

// CookieStore persists complete sessions without filesystem cookie caches.
type CookieStore interface {
	LoadCookies() ([]byte, error)
	SaveCookies([]byte) error
	DeleteCookies() error
}

// loadCachedJar 从 SQLite 载入 Cookie，并过滤过期项。
func loadCachedJar(store CookieStore) (*cookieJar, error) {
	if store == nil {
		return nil, nil
	}
	data, err := store.LoadCookies()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var list []Cookie
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	jar := newCookieJar()
	now := time.Now().Unix()
	for _, c := range list {
		if c.Value == "" {
			continue
		}
		if c.Expires > 0 && c.Expires < float64(now) {
			continue
		}
		jar.SetFull(c)
	}
	if !jar.Has("__Secure-1PSID") {
		return nil, nil
	}
	return jar, nil
}

// saveCookies 持久化到 SQLite 会话存储（仅 google 域 + 未过期 + 认证 Cookie）。
func saveCookies(store CookieStore, jar *cookieJar) error {
	if store == nil {
		return nil
	}
	psid := jar.Get("__Secure-1PSID")
	if psid == "" {
		return nil
	}
	var list []Cookie
	for _, c := range jar.snapshot() {
		domain := strings.TrimPrefix(c.Domain, ".")
		if domain != "google.com" && !strings.HasSuffix(domain, ".google.com") {
			continue
		}
		isAuth := c.Name == "__Secure-1PSID" || c.Name == "__Secure-1PSIDTS"
		if !isAuth && c.Expires > 0 && c.Expires < float64(time.Now().Unix()) {
			continue
		}
		list = append(list, c)
	}
	if len(list) == 0 {
		return nil
	}
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return store.SaveCookies(data)
}

func (j *cookieJar) snapshot() []Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	list := make([]Cookie, 0, len(j.cookies))
	for _, c := range j.cookies {
		list = append(list, *c)
	}
	return list
}
