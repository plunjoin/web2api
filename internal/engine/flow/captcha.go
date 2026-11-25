// captcha.go reCAPTCHA token 采集。
//
// Flow 的每次图片/视频生成都需要一个 reCAPTCHA V3 token，塞在 batchexecute
// 的 clientContext[10]。拿不到 token 就直接失败——这是 Flow 引擎唯一的硬依赖。
//
// 采集方式：驱动本机 Chrome / Edge 打开 Flow 项目页，劫持
// grecaptcha.enterprise.execute 拿到 token。劫持方式有两层：
//
//  1. 页面加载前 patch Function.prototype.bind——Flow 会把 enterprise.execute
//     重新赋值成一个包装函数（里面可能把 action 改成 extension_hijack_detected），
//     直接调用会被判失败。patch bind 能在包装发生前抓到原始实现。
//  2. patch Object.assign——新 Flow 会用赋值的方式覆盖 action。patch 掉
//     那次覆盖就能保住原本请求的 action。
//
// 采集到的 token 只能配合「同一个浏览器、同一份 UA」提交，否则 reCAPTCHA
// V3 会判定环境不一致（UNUSUAL_ACTIVITY）。
package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"web2api/internal/engine/flow/cdp"
)

// reCAPTCHA site key（Flow 前端固定值）。
const recaptchaSiteKey = "6LdsFiUsAAAAAIjVDZcuLhaHiDn5nnHVXVRQGeMV"

// reCAPTCHA action。
const (
	actionImageGeneration = "IMAGE_GENERATION"
	actionVideoGeneration = "VIDEO_GENERATION"
	actionUploadImage     = "UPLOAD_IMAGE"
)

// trustedExecuteHook 在页面加载前注入：劫持 grecaptcha.enterprise.execute。
//
// 原理：Flow 前端加载 reCAPTCHA 后会把 enterprise.execute 换成自己的包装
// 函数（把 action 改成 extension_hijack_detected 以检测插件劫持）。我们在
// Function.prototype.bind 上做手脚——包装函数内部必然有一处 .bind 调用，
// 借此在替换之前抓到原始 execute。
const trustedExecuteHook = `
(() => {
    if (window.__web2apiTrustedRecaptchaHookInstalled) return;
    Object.defineProperty(window, '__web2apiTrustedRecaptchaHookInstalled', { value: true });

    const storageKey = '__web2apiTrustedRecaptchaExecute';
    const originalBind = Function.prototype.bind;
    Function.prototype.bind = function(thisArg, ...args) {
        const bound = originalBind.call(this, thisArg, ...args);
        try {
            const enterprise = window.grecaptcha && window.grecaptcha.enterprise;
            if (!window[storageKey] && enterprise && this === enterprise.execute && typeof bound === 'function') {
                window[storageKey] = bound;
            }
        } catch (error) {}
        return bound;
    };

    Object.defineProperty(window, '__web2apiRecaptchaActionGuardInstalled', { value: true });
    const originalAssign = Object.assign;
    Object.assign = function(target, ...sources) {
        let nextSources = sources;
        if (sources.length >= 2) {
            const previous = sources[sources.length - 2];
            const last = sources[sources.length - 1];
            if (previous && typeof previous.action === 'string' && previous.action &&
                last && last.action === 'extension_hijack_detected') {
                nextSources = sources.slice(0, -1);
            }
        }
        return originalAssign.call(this, target, ...nextSources);
    };
})();
`

// stealthPatch 是最基础的自动化痕迹清理。
const stealthPatch = `
Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
`

// recaptchaExecuteScript 在页面里执行并拿 token。
//
// 三个策略依次尝试：captured（hook 抓到的原始实现）→ direct（当前
// enterprise.execute）→ render（新建一个 invisible 容器再执行）。
const recaptchaExecuteScript = `
(async (siteKey, requestedAction, timeoutMs) => {
    const deadline = Date.now() + Math.max(0, timeoutMs);
    const captured = window['__web2apiTrustedRecaptchaExecute'];
    const enterprise = window.grecaptcha && window.grecaptcha.enterprise;
    const errors = [];
    const strategies = [];
    let container = null;

    if (typeof captured === 'function') {
        strategies.push(['captured', () => captured(siteKey, {action: requestedAction})]);
    }
    if (enterprise && typeof enterprise.execute === 'function') {
        let executeSource = '';
        try { executeSource = String(enterprise.execute); } catch (error) {}
        if (executeSource.includes('extension_hijack_detected') && !window['__web2apiRecaptchaActionGuardInstalled']) {
            errors.push('trusted reCAPTCHA action guard is unavailable');
        } else {
            strategies.push(['direct', () => enterprise.execute.call(enterprise, siteKey, {action: requestedAction})]);
            if (typeof enterprise.render === 'function') {
                strategies.push(['render', () => {
                    container = document.createElement('div');
                    container.setAttribute('data-web2api-recaptcha', '1');
                    container.style.display = 'none';
                    (document.body || document.documentElement).appendChild(container);
                    const clientId = enterprise.render(container, {sitekey: siteKey, size: 'invisible', action: requestedAction});
                    return enterprise.execute.call(enterprise, clientId, {action: requestedAction});
                }]);
            }
        }
    }

    let successfulToken = null;
    let resolveSuccess;
    const successPromise = new Promise(resolve => { resolveSuccess = resolve; });
    const waitForResult = async (completion, waitMs) => {
        let timer;
        try {
            await Promise.race([
                successPromise,
                completion,
                new Promise(resolve => { timer = setTimeout(resolve, Math.max(0, waitMs)); })
            ]);
        } finally {
            clearTimeout(timer);
        }
    };

    const attempts = [];
    try {
        for (let index = 0; index < strategies.length; index += 1) {
            const [label, factory] = strategies[index];
            const remaining = deadline - Date.now();
            if (remaining <= 0) { errors.push(label + ': total execute timeout'); break; }
            const attempt = {label, settled: false};
            attempt.promise = Promise.resolve().then(factory).then(token => {
                attempt.settled = true;
                const normalized = typeof token === 'string' ? token.trim() : '';
                if (!normalized) {
                    errors.push(label + ': empty token');
                } else if (Date.now() >= deadline) {
                    errors.push(label + ': total execute timeout');
                } else if (!successfulToken) {
                    successfulToken = normalized;
                    resolveSuccess(normalized);
                }
            }, error => {
                attempt.settled = true;
                errors.push(label + ': ' + (error && error.message ? error.message : String(error)));
            });
            attempts.push(attempt);
            const attemptMs = Math.max(1, Math.floor(remaining / (strategies.length - index)));
            await waitForResult(attempt.promise, attemptMs);
            if (successfulToken) return {ok: true, token: successfulToken};
        }
        const remaining = deadline - Date.now();
        if (remaining > 0 && attempts.some(attempt => !attempt.settled)) {
            await waitForResult(Promise.all(attempts.map(attempt => attempt.promise)), remaining);
        }
        if (successfulToken) return {ok: true, token: successfulToken};
        for (const attempt of attempts) {
            if (!attempt.settled) errors.push(attempt.label + ': total execute timeout');
        }
        return {ok: false, error: errors.join(' | ') || 'reCAPTCHA execute is unavailable'};
    } finally {
        if (container) container.remove();
    }
})(siteKey, requestedAction, timeoutMs)
`

// enterpriseReadyScript 判断 reCAPTCHA enterprise 是否就绪。
const enterpriseReadyScript = `
(typeof grecaptcha !== 'undefined' && typeof grecaptcha.enterprise !== 'undefined' &&
 typeof grecaptcha.enterprise.execute === 'function')
`

// injectRecaptchaScript 手动注入 reCAPTCHA 脚本（页面没加载成功时兜底）。
const injectRecaptchaScript = `
(({url, timeoutMs}) => new Promise((resolve, reject) => {
    const script = document.createElement('script');
    const nonceSource = Array.from(document.scripts).find(node => node.nonce);
    if (nonceSource) script.nonce = nonceSource.nonce;
    script.async = true;
    let settled = false;
    const finish = (error) => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        script.onload = null;
        script.onerror = null;
        if (error) { script.remove(); reject(error); }
        else { resolve(true); }
    };
    const timer = setTimeout(() => { finish(new Error('reCAPTCHA script load timeout')); }, timeoutMs);
    script.onload = () => finish();
    script.onerror = () => finish(new Error('reCAPTCHA script failed to load'));
    try {
        try { script.src = url; }
        catch (error) {
            const policyKey = '__web2apiRecaptchaScriptURLPolicy';
            if (!window[policyKey]) {
                window[policyKey] = window.trustedTypes.createPolicy(
                    'web2api-recaptcha', {createScriptURL: value => value});
            }
            script.src = window[policyKey].createScriptURL(url);
        }
        (document.head || document.documentElement).appendChild(script);
    } catch (error) { finish(error); }
}))({url, timeoutMs})
`

// CaptchaResult 是一次打码的结果。
type CaptchaResult struct {
	// Token reCAPTCHA token。
	Token string
	// UserAgent 打码时浏览器的 UA——提交请求必须沿用它。
	UserAgent string
	// AcceptLanguage 打码时浏览器的 Accept-Language。
	AcceptLanguage string
}

// CaptchaProvider 采集 reCAPTCHA token。
type CaptchaProvider interface {
	// Token 采集一个 token。action 是 reCAPTCHA 的 page_action，proxy 是该账号的出口代理。
	Token(ctx context.Context, projectID, action string, cookies []Cookie, proxy string) (CaptchaResult, error)
}

// BrowserCaptchaProvider 驱动本机浏览器采集 token。
type BrowserCaptchaProvider struct {
	// Binary 浏览器可执行文件路径；为空则自动探测。
	Binary string
	// Proxy 代理地址；为空表示直连。
	Proxy string
	// Headless 是否无头运行。
	Headless bool
	// Warmup 打开页面后的预热等待时间。
	Warmup time.Duration
	// Timeout 单次采集的总超时。
	Timeout time.Duration

	mu       sync.Mutex
	browsers map[string]*cdp.Browser
}

// NewBrowserCaptchaProvider 构造浏览器打码器。
func NewBrowserCaptchaProvider() *BrowserCaptchaProvider {
	return &BrowserCaptchaProvider{
		Headless: true,
		Warmup:   4 * time.Second,
		Timeout:  90 * time.Second,
	}
}

// ensureBrowser 按代理惰性启动浏览器（同一代理进程级复用）。
func (p *BrowserCaptchaProvider) ensureBrowser(ctx context.Context, proxy string) (*cdp.Browser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.browsers == nil {
		p.browsers = make(map[string]*cdp.Browser)
	}
	if browser := p.browsers[proxy]; browser != nil {
		return browser, nil
	}
	browser, err := cdp.Launch(ctx, cdp.LaunchOptions{
		Binary:   p.Binary,
		Proxy:    proxy,
		Headless: p.Headless,
		Timeout:  40 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	p.browsers[proxy] = browser
	return browser, nil
}

// Close 关闭所有按代理复用的浏览器。
func (p *BrowserCaptchaProvider) Close() error {
	p.mu.Lock()
	browsers := p.browsers
	p.browsers = nil
	p.mu.Unlock()

	var errs []error
	for _, browser := range browsers {
		if browser == nil {
			continue
		}
		if err := browser.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Token 采集一个 reCAPTCHA token。
func (p *BrowserCaptchaProvider) Token(ctx context.Context, projectID, action string, cookies []Cookie, proxy string) (CaptchaResult, error) {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		proxy = strings.TrimSpace(p.Proxy)
	}
	browser, err := p.ensureBrowser(ctx, proxy)
	if err != nil {
		return CaptchaResult{}, err
	}
	browserContextID, err := browser.NewContext(ctx)
	if err != nil {
		return CaptchaResult{}, fmt.Errorf("创建浏览器上下文失败: %w", err)
	}
	defer browser.DisposeContext(context.WithoutCancel(ctx), browserContextID)

	page, err := browser.NewPage(ctx, browserContextID, "about:blank")
	if err != nil {
		return CaptchaResult{}, fmt.Errorf("打开打码页面失败: %w", err)
	}
	defer page.Close(context.WithoutCancel(ctx))

	// 注入 Cookie：浏览器要带着账号登录态访问 Flow 项目页。
	if err := page.SetCookies(ctx, browserContextID, browserCookies(cookies)); err != nil {
		return CaptchaResult{}, fmt.Errorf("注入 Google Cookie 失败: %w", err)
	}
	// 注入 hook 与 stealth 补丁——必须在页面加载前。
	if err := page.AddScriptOnNewDocument(ctx, trustedExecuteHook); err != nil {
		return CaptchaResult{}, fmt.Errorf("注入 reCAPTCHA hook 失败: %w", err)
	}
	if err := page.AddScriptOnNewDocument(ctx, stealthPatch); err != nil {
		return CaptchaResult{}, fmt.Errorf("注入 stealth 补丁失败: %w", err)
	}

	// 打开项目页：reCAPTCHA 的 site key 是按页面绑定的，必须在 Flow 页面里求值。
	targets := []string{
		flowBaseURL + "/project/" + projectID,
		flowBaseURL + "/about",
	}
	var loaded bool
	for _, target := range targets {
		if err := page.Navigate(ctx, target, 45*time.Second); err != nil {
			continue
		}
		loaded = true
		break
	}
	if !loaded {
		return CaptchaResult{}, errors.New("打码页面打不开（Flow 项目页加载失败）")
	}

	// 预热：让页面里的 reCAPTCHA 完成初始化。
	if p.Warmup > 0 {
		select {
		case <-ctx.Done():
			return CaptchaResult{}, ctx.Err()
		case <-time.After(p.Warmup):
		}
	}

	// 等 reCAPTCHA enterprise 就绪；不就绪就手动注入脚本再等一轮。
	if !p.waitEnterpriseReady(ctx, page, 15*time.Second) {
		p.injectRecaptcha(ctx, page, 15*time.Second)
		if !p.waitEnterpriseReady(ctx, page, 10*time.Second) {
			return CaptchaResult{}, errors.New("reCAPTCHA enterprise 未就绪")
		}
	}

	// 采集浏览器指纹：提交请求要用同一个 UA。
	fingerprint, err := p.captureFingerprint(ctx, page)
	if err != nil {
		return CaptchaResult{}, fmt.Errorf("读取浏览器指纹失败: %w", err)
	}

	// 执行打码脚本。
	script := buildExecuteScript(recaptchaSiteKey, action, 25000)
	result, err := page.Evaluate(ctx, script, true)
	if err != nil {
		return CaptchaResult{}, fmt.Errorf("执行 reCAPTCHA 失败: %w", err)
	}
	var parsed struct {
		OK    bool   `json:"ok"`
		Token string `json:"token"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		return CaptchaResult{}, fmt.Errorf("解析 reCAPTCHA 结果失败: %w", err)
	}
	if !parsed.OK || strings.TrimSpace(parsed.Token) == "" {
		reason := parsed.Error
		if reason == "" {
			reason = "没有拿到 token"
		}
		return CaptchaResult{}, fmt.Errorf("reCAPTCHA 采集失败: %s", reason)
	}
	return CaptchaResult{
		Token:          strings.TrimSpace(parsed.Token),
		UserAgent:      fingerprint.UserAgent,
		AcceptLanguage: fingerprint.AcceptLanguage,
	}, nil
}

// waitEnterpriseReady 轮询等待 reCAPTCHA enterprise 就绪。
func (p *BrowserCaptchaProvider) waitEnterpriseReady(ctx context.Context, page *cdp.Page, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(ctx, enterpriseReadyScript, false)
		if err == nil && strings.TrimSpace(value) == "true" {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(250 * time.Millisecond):
		}
	}
	return false
}

// injectRecaptcha 手动注入 reCAPTCHA 脚本。
func (p *BrowserCaptchaProvider) injectRecaptcha(ctx context.Context, page *cdp.Page, timeout time.Duration) {
	hosts := []string{"https://www.google.com", "https://www.recaptcha.net"}
	for _, host := range hosts {
		url := host + "/recaptcha/enterprise.js?render=" + recaptchaSiteKey + "&trustedtypes=true"
		script := fmt.Sprintf("%s(%s,%d)", strings.TrimSpace(injectRecaptchaScript), jsonString(url), timeout.Milliseconds())
		if _, err := page.Evaluate(ctx, script, true); err != nil {
			continue
		}
		return
	}
}

// Fingerprint 是浏览器指纹。
type Fingerprint struct {
	UserAgent       string
	AcceptLanguage  string
	SecChUA         string
	SecChUAMobile   string
	SecChUAPlatform string
}

// captureFingerprint 读取页面里的浏览器指纹。
func (p *BrowserCaptchaProvider) captureFingerprint(ctx context.Context, page *cdp.Page) (Fingerprint, error) {
	const script = `(() => {
        const ua = navigator.userAgent || '';
        const lang = navigator.language || '';
        const uaData = navigator.userAgentData || null;
        let secChUa = ''; let secChUaMobile = ''; let secChUaPlatform = '';
        if (uaData) {
            if (Array.isArray(uaData.brands) && uaData.brands.length > 0) {
                secChUa = uaData.brands.map((item) => '"' + item.brand + '";v="' + item.version + '"').join(', ');
            }
            secChUaMobile = uaData.mobile ? '?1' : '?0';
            if (uaData.platform) { secChUaPlatform = '"' + uaData.platform + '"'; }
        }
        return JSON.stringify({
            user_agent: ua, accept_language: lang, sec_ch_ua: secChUa,
            sec_ch_ua_mobile: secChUaMobile, sec_ch_ua_platform: secChUaPlatform
        });
    })()`
	raw, err := page.Evaluate(ctx, script, false)
	if err != nil {
		return Fingerprint{}, err
	}
	var fingerprint Fingerprint
	if err := json.Unmarshal([]byte(raw), &fingerprint); err != nil {
		return Fingerprint{}, err
	}
	return fingerprint, nil
}

// buildExecuteScript 填充 site key / action / 超时。
func buildExecuteScript(siteKey, action string, timeoutMS int) string {
	script := recaptchaExecuteScript
	script = strings.ReplaceAll(script, "siteKey", jsonString(siteKey))
	script = strings.ReplaceAll(script, "requestedAction", jsonString(action))
	script = strings.ReplaceAll(script, "timeoutMs", fmt.Sprintf("%d", timeoutMS))
	return script
}

// jsonString 把字符串编码成 JSON 字面量。
func jsonString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

// browserCookies 把凭据里的 Cookie 转成浏览器可用的形态。
//
// CDP 的 Storage.setCookies 不接受 url + path 同时存在；带域的 Cookie 用
// domain + path，只带 URL 的用 url。
func browserCookies(cookies []Cookie) []cdp.Cookie {
	out := make([]cdp.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		item := cdp.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Domain:   cookie.Domain,
			Path:     cookie.Path,
			Secure:   cookie.Secure,
			HTTPOnly: cookie.HTTPOnly,
			SameSite: cookie.SameSite,
			Expires:  cookie.Expires,
		}
		if cookie.Path == "" {
			item.Path = "/"
		}
		if cookie.SameSite == "" {
			item.SameSite = "Lax"
		}
		if item.Domain == "" {
			item.URL = "https://flow.google.com/"
		}
		out = append(out, item)
	}
	return out
}
