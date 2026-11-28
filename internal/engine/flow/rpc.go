// rpc.go Google Flow 前端 RPC 客户端。
//
// Flow 前端是 Angular 应用，它的所有动作都走同一个 batchexecute 端点：
//
//	POST https://flow.google.com/_/AiSandboxAngularFrontend/data/batchexecute
//	     ?rpcids=<rpc>&source-path=/project/<pid>&bl=<bootloader>
//	     &f.sid=<session>&hl=en&_reqid=<n>&rt=c
//
// 表单体只有两个字段：f.req（RPC 调用的三明治 JSON）与 at（XSRF token）。
// 每次调用前都要先 GET 一次项目页拿到 bootloader / session / XSRF。
package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// flowBaseURL 是 Flow 前端根地址。
	flowBaseURL = "https://flow.google.com"
	// batchexecutePath 是前端 RPC 端点。
	batchexecutePath = "/_/AiSandboxAngularFrontend/data/batchexecute"
	// flowUserAgent 是默认 UA。
	//
	// 提交请求的 UA 必须与打码时浏览器用的 UA 一致，否则 reCAPTCHA V3 会判
	// UNUSUAL_ACTIVITY。captcha 包会把浏览器指纹带回来覆盖这个默认值。
	flowUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
	// flowAcceptLanguage 是默认 Accept-Language。
	flowAcceptLanguage = "en-US,en;q=0.9"
)

// RPC ID（Flow 前端 batchexecute）。
const (
	rpcCreateProject = "jHPbke"
	rpcDeleteProject = "QI2zvc"
	rpcCredits       = "nzlxg"
	rpcGetMedia      = "as29s"
	rpcUploadImage   = "maseQ"
	rpcImageUpsample = "SPrCad"
	rpcImageGenerate = "ogiZ0b"
	rpcVideoText     = "YhhmEf"
	rpcVideoRefs     = "MZZa6b"
	rpcVideoStartEnd = "nprQif"
	rpcVideoStart    = "eb1hJf"
	rpcVideoExtend   = "fZytfe"
	rpcVideoUpsample = "p0UkFb"
	rpcVideoStatus   = "jwpduf"
)

// 前端页面里解析出来的引导信息。
var (
	bootloaderPattern = regexp.MustCompile(`boq_labs-ai-sandbox-frontend_[A-Za-z0-9_.-]+`)
	sessionPattern    = regexp.MustCompile(`"FdrFJe":"(-?[0-9]+)"`)
	xsrfPattern       = regexp.MustCompile(`"SNlM0e"\s*:\s*"((?:\\.|[^"\\])+)"`)
	xsrfChallenge     = regexp.MustCompile(`\["xsrf"\s*,\s*"((?:\\.|[^"\\])*)"`)
	uuidPattern       = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

// Bootstrap 是前端引导信息。
type Bootstrap struct {
	// BootloaderID 是前端构建号。
	BootloaderID string
	// SessionID 是前端会话 ID（对应 f.sid）。
	SessionID string
	// XSRFToken 是 CSRF token（对应表单的 at 字段）。
	XSRFToken string
	// ProjectID 是引导页里的项目 ID（可能为空）。
	ProjectID string
	// Email 是页面里带的账号邮箱。
	Email string
	// Cookies 是合并过本次响应 Set-Cookie 后的登录态。
	Cookies []Cookie
}

// RPCError 是 Flow 前端返回的业务错误。
type RPCError struct {
	// RPCID 触发错误的 RPC。
	RPCID string
	// Code 错误码（如 PUBLIC_ERROR_UNSAFE_GENERATION）；可能是数字串。
	Code string
	// Detail 原始错误描述。
	Detail string
}

func (e *RPCError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("Flow 拒绝请求: rpc=%s, code=%s", e.RPCID, e.Code)
	}
	if e.Detail != "" {
		return fmt.Sprintf("Flow 拒绝请求: rpc=%s, %s", e.RPCID, e.Detail)
	}
	return fmt.Sprintf("Flow 拒绝请求: rpc=%s", e.RPCID)
}

// ErrorCode 返回错误码。
func (e *RPCError) ErrorCode() string { return e.Code }

// AuthenticationError 表示登录态失效。
type AuthenticationError struct {
	Detail string
}

func (e *AuthenticationError) Error() string {
	return "Flow 登录认证失败：Google Cookie 已失效、不完整，或来自不同账号（" + e.Detail + "）"
}

// RPCClient 是单个账号的 Flow 前端客户端。
type RPCClient struct {
	httpClient *http.Client
	rpcClient  *http.Client
	userAgent  string
	acceptLang string
}

// NewRPCClient 构造 RPC 客户端。
func NewRPCClient(httpClient *http.Client, userAgent string) *RPCClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = flowUserAgent
	}
	// 项目页引导需要自己走跳转链并逐跳合并 Set-Cookie，RPC 也不应把
	// 登录重定向当成功响应，所以控制面使用不跟随跳转的副本。媒体下载
	// 仍走原客户端，保留正常的重定向行为。
	rpcClient := *httpClient
	rpcClient.Jar = nil
	rpcClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &RPCClient{
		httpClient: httpClient,
		rpcClient:  &rpcClient,
		userAgent:  userAgent,
		acceptLang: flowAcceptLanguage,
	}
}

// SetUserAgent 更新 UA（打码后按浏览器指纹对齐）。
func (c *RPCClient) SetUserAgent(userAgent string) {
	if strings.TrimSpace(userAgent) != "" {
		c.userAgent = userAgent
	}
}

// SetAcceptLanguage 更新 Accept-Language。
func (c *RPCClient) SetAcceptLanguage(acceptLanguage string) {
	if strings.TrimSpace(acceptLanguage) != "" {
		c.acceptLang = acceptLanguage
	}
}

// HTTPClient 返回底层 HTTP 客户端（用于下载签名媒体地址）。
func (c *RPCClient) HTTPClient() *http.Client { return c.httpClient }

// Bootstrap 拉取前端引导信息，并顺手收下响应轮换的 Cookie。
//
// 传输层不跟随重定向，所以这里自己走跳转链：每一跳的 Set-Cookie 都合并进 cookies。
// 跳到 flow.google.com 以外（通常是登录页）说明登录态已经死了，不再继续，也不保存。
func (c *RPCClient) Bootstrap(ctx context.Context, projectID string, cookies []Cookie) (Bootstrap, error) {
	sourcePath := "/projects"
	if projectID != "" {
		sourcePath = "/project/" + projectID
	}
	return c.bootstrap(ctx, flowBaseURL+sourcePath, projectID, cookies, "flow.google.com")
}

// bootstrap 自己走跳转链并解析最终页面。
// allowedHost 是唯一允许保存登录态的主机。
func (c *RPCClient) bootstrap(ctx context.Context, pageURL, projectID string, cookies []Cookie, allowedHost string) (Bootstrap, error) {
	start, err := url.Parse(pageURL)
	if err != nil {
		return Bootstrap{}, err
	}
	current := append([]Cookie(nil), cookies...)
	var resp *http.Response
	var body []byte
	var requestedHost string
	for hop := 0; hop < 5; hop++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return Bootstrap{}, err
		}
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", c.acceptLang)
		req.Header.Set("Referer", flowBaseURL+"/")
		req.Header.Set("User-Agent", c.userAgent)
		if header := CookieHeader(current, req.URL.Hostname()); header != "" {
			req.Header.Set("Cookie", header)
		}
		requestedHost = req.URL.Hostname()
		resp, err = c.rpcClient.Do(req)
		if err != nil {
			return Bootstrap{}, fmt.Errorf("加载 Flow 项目页失败: %w", err)
		}
		host := ""
		if req.URL != nil {
			host = req.URL.Hostname()
		}
		current = ApplySetCookies(current, resp.Header.Values("Set-Cookie"), host)
		if !isRedirect(resp.StatusCode) {
			body, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			resp.Body.Close()
			if err != nil {
				return Bootstrap{}, fmt.Errorf("读取 Flow 项目页失败: %w", err)
			}
			break
		}
		resp.Body.Close()
		next, err := redirectTarget(req, resp)
		if err != nil {
			return Bootstrap{}, &AuthenticationError{Detail: "登录态被重定向到未知地址"}
		}
		if !googleHost(next.Hostname()) && !sameOrigin(next, start) {
			return Bootstrap{}, &AuthenticationError{Detail: "登录态被重定向到 " + next.Hostname()}
		}
		pageURL = next.String()
		resp = nil
	}
	if resp == nil {
		return Bootstrap{}, &AuthenticationError{Detail: "登录态重定向次数过多"}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Bootstrap{}, &AuthenticationError{Detail: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	if resp.StatusCode >= 400 {
		return Bootstrap{}, fmt.Errorf("加载 Flow 项目页失败: HTTP %d", resp.StatusCode)
	}
	// 传输层不跟随重定向，响应里的 Request 仍是发出去的那个。
	// 最终主机用这一跳实际请求的地址，而不是 resp.Request.URL。
	if !strings.EqualFold(requestedHost, allowedHost) {
		return Bootstrap{}, &AuthenticationError{Detail: "登录态停在 " + requestedHost}
	}
	text := string(body)
	bootloader := bootloaderPattern.FindString(text)
	session := sessionPattern.FindStringSubmatch(text)
	if bootloader == "" || len(session) < 2 {
		return Bootstrap{}, errors.New("Flow 前端引导信息缺失（bootloader / session）")
	}
	out := Bootstrap{
		BootloaderID: bootloader,
		SessionID:    session[1],
		ProjectID:    projectID,
		Cookies:      current,
	}
	if match := xsrfPattern.FindStringSubmatch(text); len(match) >= 2 {
		if decoded, err := jsonUnescapeString(match[1]); err == nil {
			out.XSRFToken = decoded
		} else {
			out.XSRFToken = match[1]
		}
	}
	if match := regexp.MustCompile(`"oPEP7c"\s*:\s*"([^"]*)"`).FindStringSubmatch(text); len(match) >= 2 {
		out.Email = match[1]
	}
	if strings.TrimSpace(out.Email) == "" {
		return Bootstrap{}, &AuthenticationError{Detail: "项目页没有登录邮箱"}
	}
	return out, nil
}

// isRedirect 判断状态码是不是需要跟着走的跳转。
func isRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

// redirectTarget 解析这一跳的 Location。相对地址按当前请求补全。
func redirectTarget(req *http.Request, resp *http.Response) (*url.URL, error) {
	location := strings.TrimSpace(resp.Header.Get("Location"))
	if location == "" {
		return nil, errors.New("重定向没有 Location")
	}
	next, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	if req.URL != nil {
		next = req.URL.ResolveReference(next)
	}
	if next.Host == "" {
		return nil, errors.New("重定向没有主机")
	}
	return next, nil
}

// googleHost 判断主机是不是 Google 域。登录跳走时不再跟随。
func googleHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	return host == "google.com" || strings.HasSuffix(host, ".google.com")
}

// sameOrigin 判断两次请求是否仍是同一站点。测试与私有部署可能不使用
// Google 域名，此时只允许留在起始站点内的跳转。
func sameOrigin(next, start *url.URL) bool {
	if next == nil || start == nil {
		return false
	}
	return strings.EqualFold(next.Scheme, start.Scheme) && strings.EqualFold(next.Host, start.Host)
}

// jsonUnescapeString 把 JSON 字符串字面量还原成原始字符串。
func jsonUnescapeString(value string) (string, error) {
	var decoded string
	if err := json.Unmarshal([]byte(`"`+value+`"`), &decoded); err != nil {
		return "", err
	}
	return decoded, nil
}

// Call 执行一次 batchexecute RPC，返回解析后的负载。
//
// argument 是 RPC 的位置参数（数组）。上游用的是位置协议——每个槽位都有
// 固定含义，没有命名键。
func (c *RPCClient) Call(ctx context.Context, boot Bootstrap, rpcID string, argument any, cookieHeader, sourcePath string) (json.RawMessage, error) {
	if sourcePath == "" {
		if boot.ProjectID != "" {
			sourcePath = "/project/" + boot.ProjectID
		} else {
			sourcePath = "/projects"
		}
	}
	body, err := c.callOnce(ctx, boot, rpcID, argument, cookieHeader, sourcePath, boot.XSRFToken)
	if err != nil {
		return nil, err
	}
	// 服务端可能回一个 xsrf 挑战：换上新 token 原样重发一次（只重试一次）。
	if challenge := xsrfChallenge.FindSubmatch(body); len(challenge) >= 2 {
		token, decodeErr := jsonUnescapeString(string(challenge[1]))
		if decodeErr != nil {
			token = string(challenge[1])
		}
		return c.callOnce(ctx, boot, rpcID, argument, cookieHeader, sourcePath, token)
	}
	return parseRPCPayload(body, rpcID)
}

// callOnce 发一次 batchexecute 请求。
func (c *RPCClient) callOnce(ctx context.Context, boot Bootstrap, rpcID string, argument any, cookieHeader, sourcePath, xsrf string) (json.RawMessage, error) {
	argumentJSON, err := json.Marshal(argument)
	if err != nil {
		return nil, fmt.Errorf("编码 RPC 参数失败: %w", err)
	}
	inner, err := json.Marshal([]any{[]any{[]any{rpcID, string(argumentJSON), nil, "generic"}}})
	if err != nil {
		return nil, fmt.Errorf("编码 RPC 请求失败: %w", err)
	}
	form := url.Values{}
	form.Set("f.req", string(inner))
	if xsrf != "" {
		form.Set("at", xsrf)
	}
	endpoint := flowBaseURL + batchexecutePath + "?" + url.Values{
		"rpcids":      []string{rpcID},
		"source-path": []string{sourcePath},
		"bl":          []string{boot.BootloaderID},
		"f.sid":       []string{boot.SessionID},
		"hl":          []string{"en"},
		"_reqid":      []string{strconv.Itoa(rand.Intn(80000) + 10000)},
		"rt":          []string{"c"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", c.acceptLang)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Origin", flowBaseURL)
	req.Header.Set("Referer", flowBaseURL+"/")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Same-Domain", "1")
	req.Header.Set("User-Agent", c.userAgent)
	if cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}
	resp, err := c.rpcClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Flow RPC %s 失败: %w", rpcID, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 Flow RPC %s 响应失败: %w", rpcID, err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &AuthenticationError{Detail: fmt.Sprintf("rpc=%s HTTP %d", rpcID, resp.StatusCode)}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Flow RPC %s 失败: HTTP %d", rpcID, resp.StatusCode)
	}
	return body, nil
}

// parseRPCPayload 从 batchexecute 响应里取出指定 RPC 的负载。
//
// 响应是换行分隔的 JSON 帧；负载帧的形状是 ["wrb.fr", "<rpc>", "<json 字符串>"]，
// 其中负载本身还是一个 JSON 字符串，要再解一层。
func parseRPCPayload(body []byte, rpcID string) (json.RawMessage, error) {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		var frame []json.RawMessage
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			continue
		}
		for _, item := range frame {
			var parts []json.RawMessage
			if err := json.Unmarshal(item, &parts); err != nil || len(parts) < 3 {
				continue
			}
			var kind string
			_ = json.Unmarshal(parts[0], &kind)
			switch kind {
			case "er":
				// 错误帧：状态码在第 5 / 6 槽。
				code := ""
				if len(parts) > 5 {
					code = string(parts[5])
				}
				if code == "" && len(parts) > 6 {
					code = string(parts[6])
				}
				code = strings.Trim(code, `"`)
				if code == "401" {
					return nil, &AuthenticationError{Detail: fmt.Sprintf("rpc=%s code=401", rpcID)}
				}
				return nil, &RPCError{RPCID: rpcID, Code: code}
			case "wrb.fr":
				var id string
				if err := json.Unmarshal(parts[1], &id); err != nil || id != rpcID {
					continue
				}
				// 应用层错误码在第 5 槽；无负载时第 5 槽可能是错误文案。
				detail := ""
				if len(parts) > 5 {
					code := extractErrorCode(parts[5])
					if code == "" && string(parts[2]) == "null" {
						var text string
						if json.Unmarshal(parts[5], &text) == nil {
							detail = strings.TrimSpace(text)
						}
					}
					if code != "" {
						return nil, &RPCError{RPCID: rpcID, Code: code, Detail: detail}
					}
				}
				if string(parts[2]) == "null" {
					if detail != "" {
						return nil, &RPCError{RPCID: rpcID, Detail: detail}
					}
					return nil, &RPCError{RPCID: rpcID, Detail: "上游没有返回负载"}
				}
				var payload string
				if err := json.Unmarshal(parts[2], &payload); err != nil {
					return nil, fmt.Errorf("解析 Flow RPC %s 负载失败: %w", rpcID, err)
				}
				return json.RawMessage(payload), nil
			}
		}
	}
	return nil, fmt.Errorf("Flow RPC %s 没有返回负载", rpcID)
}

// extractErrorCode 从错误槽里取出 PUBLIC_ERROR_/ERROR_ 开头的错误码。
func extractErrorCode(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if strings.HasPrefix(text, "PUBLIC_ERROR_") || strings.HasPrefix(text, "ERROR_") {
			return text
		}
		return ""
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return ""
	}
	for _, item := range list {
		if code := extractErrorCode(item); code != "" {
			return code
		}
	}
	return ""
}

// FindUUID 在负载里找第一个 UUID 形态的字符串。
func FindUUID(payload json.RawMessage, exclude ...string) string {
	excluded := make(map[string]bool, len(exclude))
	for _, item := range exclude {
		excluded[item] = true
	}
	for _, match := range uuidPattern.FindAllString(string(payload), -1) {
		if !excluded[match] {
			return match
		}
	}
	return ""
}

// findContentURL 在负载里找 flow-content.google 的媒体地址。
func findContentURL(payload json.RawMessage, kind string) string {
	marker := "/" + kind + "/"
	var walk func(raw json.RawMessage) string
	walk = func(raw json.RawMessage) string {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			if strings.HasPrefix(text, "https://") && strings.Contains(text, marker) {
				return text
			}
			return ""
		}
		var list []json.RawMessage
		if err := json.Unmarshal(raw, &list); err == nil {
			for _, item := range list {
				if found := walk(item); found != "" {
					return found
				}
			}
			return ""
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err == nil {
			for _, item := range object {
				if found := walk(item); found != "" {
					return found
				}
			}
		}
		return ""
	}
	return walk(payload)
}

// rpcTimeout 是控制面请求的超时。
const rpcTimeout = 30 * time.Second
