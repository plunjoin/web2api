package geminiweb

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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
	if len(j.cookies) == 0 {
		return
	}
	req.Header.Set("Cookie", j.Header())
}

// UpdateFromResponse 从响应 Set-Cookie 中更新 Cookie。
func (j *cookieJar) UpdateFromResponse(resp *http.Response) {
	for _, c := range resp.Cookies() {
		cc := &Cookie{
			Name:   c.Name,
			Value:  c.Value,
			Domain: c.Domain,
			Path:   c.Path,
		}
		if !c.Expires.IsZero() {
			cc.Expires = float64(c.Expires.Unix())
		}
		j.mu.Lock()
		j.cookies[c.Name] = cc
		j.mu.Unlock()
	}
}

// cookiesCacheDir 返回缓存目录（GEMINI_COOKIE_PATH 或程序内 cookies/）。
func cookiesCacheDir(override string) string {
	if override != "" {
		return override
	}
	if env := os.Getenv("GEMINI_COOKIE_PATH"); env != "" {
		return env
	}
	return "cookies"
}

// cachePathFor 按 PSID 生成缓存文件路径。
func cachePathFor(dir, psid string) string {
	return filepath.Join(dir, ".cached_cookies_"+psid+".json")
}

// loadCachedJar 从缓存文件加载 Cookie；文件不存在或无效返回 nil。
func loadCachedJar(dir, psid string) *cookieJar {
	path := cachePathFor(dir, psid)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var list []Cookie
	if err := json.Unmarshal(data, &list); err != nil {
		return nil
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
		return nil
	}
	return jar
}

// saveCookies 持久化到缓存文件（仅 google 域 + 未过期 + 认证 Cookie）。
func saveCookies(dir string, jar *cookieJar) error {
	psid := jar.Get("__Secure-1PSID")
	if psid == "" {
		return nil
	}
	var list []Cookie
	for _, c := range jar.cookies {
		domain := strings.TrimPrefix(c.Domain, ".")
		if domain != "google.com" && !strings.HasSuffix(domain, ".google.com") {
			continue
		}
		isAuth := c.Name == "__Secure-1PSID" || c.Name == "__Secure-1PSIDTS"
		if !isAuth && c.Expires > 0 && c.Expires < float64(time.Now().Unix()) {
			continue
		}
		list = append(list, *c)
	}
	if len(list) == 0 {
		return nil
	}
	path := cachePathFor(dir, psid)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// clearCache 删除某 PSID 的缓存文件（会话未认证时清理）。
func clearCache(dir, psid string) {
	if psid == "" {
		return
	}
	_ = os.Remove(cachePathFor(dir, psid))
}
