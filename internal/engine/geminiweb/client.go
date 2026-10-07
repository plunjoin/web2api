package geminiweb

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 错误类型。
var (
	ErrNotReady        = errors.New("gemini 客户端未初始化")
	ErrUsageLimit      = errors.New("usage limit exceeded")
	ErrModelInvalid    = errors.New("model inconsistent or invalid")
	ErrIPBlocked       = errors.New("ip temporarily blocked")
	ErrStreamSuspended = errors.New("stream suspended by google")
	ErrAuth            = errors.New("authentication failed")
)

// AccountStatus 账号状态码（部分）。
const (
	StatusAvailable       = 1000
	StatusUnauthenticated = 1016
	StatusTempUnavailable = 1014
)

// Client 引擎A 的单账号客户端。
type Client struct {
	mu sync.Mutex

	name        string
	jar         *cookieJar
	cookieStore CookieStore
	proxy       string
	timeout     time.Duration

	http *http.Client

	// 会话参数
	accessToken string
	buildLabel  string
	sessionID   string
	language    string
	pushID      string
	sessionUUID string

	reqID int

	ready         bool
	initializing  bool
	accountStatus int
	cookieSource  string
	lastActivity  time.Time

	models []*AvailableModel

	refreshStop chan struct{}
}

// NewClient 创建客户端；Cookie 会话由注入的 SQLite 存储持久化。
func NewClient(name, psid, psidts, proxy string, timeout time.Duration) *Client {
	jar := newCookieJar()
	if psid != "" {
		jar.Set("__Secure-1PSID", psid)
	}
	if psidts != "" {
		jar.Set("__Secure-1PSIDTS", psidts)
	}
	transport := &http.Transport{
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
		Proxy:               proxyFunc(proxy),
	}
	return &Client{
		name:          name,
		jar:           jar,
		proxy:         proxy,
		timeout:       timeout,
		http:          &http.Client{Transport: transport, Timeout: 0},
		reqID:         time.Now().Nanosecond()%90000 + 10000,
		accountStatus: StatusAvailable,
		lastActivity:  time.Now(),
	}
}

func proxyFunc(proxy string) func(*http.Request) (*url.URL, error) {
	if proxy == "" {
		return nil
	}
	return func(r *http.Request) (*url.URL, error) {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		return u, nil
	}
}

// SetCookieStore attaches persistence before initialization.
func (c *Client) SetCookieStore(store CookieStore) { c.cookieStore = store }

// Name 账号名。
func (c *Client) Name() string { return c.name }

// Models 返回已发现模型（未初始化时返回内置表）。
func (c *Client) Models() []*AvailableModel {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.models) == 0 {
		return builtinModels()
	}
	return c.models
}

// Ready 是否已初始化。
func (c *Client) Ready() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ready
}

// AccountStatus 返回账号状态。
func (c *Client) AccountStatus() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accountStatus
}

// SetProxy 运行时切换代理（重新建 transport）。
func (c *Client) SetProxy(proxy string) {
	c.proxy = proxy
	c.http.Transport = &http.Transport{
		IdleConnTimeout: 90 * time.Second,
		Proxy:           proxyFunc(proxy),
	}
}

// Init 初始化：按「缓存 → 用户提供」顺序尝试，提取 SNlM0e 等会话参数。
// 随后发起 RPC 拉取账号状态与可用模型，并持久化 Cookie 缓存。
// 网络请求在锁外执行（带 initializing 防重入标志），不阻塞状态查询。
func (c *Client) Init(ctx context.Context) error {
	c.mu.Lock()
	if c.initializing {
		c.mu.Unlock()
		return errors.New("客户端正在初始化")
	}
	c.initializing = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.initializing = false
		c.mu.Unlock()
	}()

	// 1. SQLite 会话优先。
	cached, cacheErr := loadCachedJar(c.cookieStore)
	if cacheErr != nil {
		return fmt.Errorf("读取 Cookie 会话: %w", cacheErr)
	}
	c.mu.Lock()
	if cached != nil {
		c.jar = cached
		c.cookieSource = "SQLite"
	} else {
		c.cookieSource = "Base Cookies"
	}
	c.mu.Unlock()

	// 2. GET /app 提取初始化参数（锁外网络）
	params, err := c.fetchInitParams(ctx)
	if err != nil {
		c.mu.Lock()
		c.cookieSource = ""
		c.mu.Unlock()
		return err
	}

	// 3. RPC 初始化（账号状态 + 模型发现，锁外网络）
	models, status, rpcErr := c.initRPC(ctx)

	c.mu.Lock()
	c.accessToken = params.AccessToken
	c.buildLabel = params.BuildLabel
	c.sessionID = params.SessionID
	c.language = params.Language
	c.pushID = params.PushID
	if c.sessionUUID == "" {
		c.sessionUUID = strings.ToUpper(randID())
	}
	c.ready = true
	c.lastActivity = time.Now()
	if rpcErr == nil {
		c.accountStatus = status
		c.models = models
	}
	unauth := c.accountStatus == StatusUnauthenticated
	c.mu.Unlock()

	if unauth {
		if c.cookieStore != nil {
			_ = c.cookieStore.DeleteCookies()
		}
		return ErrAuth
	}

	// 4. 持久化缓存
	return saveCookies(c.cookieStore, c.jar)
}

// fetchInitParams 发送 GET /app 并解析初始化参数。
func (c *Client) fetchInitParams(ctx context.Context) (*initParams, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, EndpointInit, nil)
	if err != nil {
		return nil, err
	}
	c.jar.ApplyToRequest(req)
	req.Header.Set("Content-Type", ContentTypeForm)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("访问 %s 失败: %w", EndpointInit, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("IP 被临时限流 (429)：请检查网络/代理")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("初始化请求失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	c.jar.UpdateFromResponse(resp)
	params, err := parseInitParams(string(body))
	if err != nil {
		return nil, err
	}
	if c.cookieSource == "" {
		c.cookieSource = "Base Cookies"
	}
	return params, nil
}

// initRPC 执行 RPC：GetUserStatus 拉取账号状态与模型列表。
func (c *Client) initRPC(ctx context.Context) ([]*AvailableModel, int, error) {
	frames, err := c.batchExecute(ctx, []rpcPayload{{RPCID: "otAQ7b", Data: "[]", Identifier: "generic"}})
	if err != nil {
		return nil, StatusAvailable, err
	}
	status := StatusAvailable
	var models []*AvailableModel
	for _, frame := range frames {
		part, ok := frame.([]any)
		if !ok || len(part) < 3 {
			continue
		}
		bodyStr, _ := part[2].(string)
		if bodyStr == "" {
			continue
		}
		var body any
		if err := jsonUnmarshal([]byte(bodyStr), &body); err != nil {
			continue
		}
		if list, ok := body.([]any); ok {
			if n, ok := toInt(getNested(list, nil, 14)); ok && n != 0 {
				status = n
			}
			models = parseModelsFromRPC([]any{part})
		}
	}
	if len(models) == 0 {
		models = builtinModels()
	}
	return models, status, nil
}

// batchExecute 发送 batchexecute RPC，返回解析后的帧。
func (c *Client) batchExecute(ctx context.Context, payloads []rpcPayload) ([]any, error) {
	params := url.Values{}
	params.Set("rpcids", stringsJoinID(payloads))
	params.Set("hl", c.language)
	params.Set("_reqid", strconv.Itoa(c.nextReqID()))
	params.Set("rt", "c")
	params.Set("source-path", "/app")
	if c.buildLabel != "" {
		params.Set("bl", c.buildLabel)
	}
	if c.sessionID != "" {
		params.Set("f.sid", c.sessionID)
	}

	form := buildBatchForm(c.accessToken, payloads)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, EndpointBatchExec+"?"+params.Encode(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	c.jar.ApplyToRequest(req)
	req.Header.Set("Content-Type", ContentTypeForm)
	req.Header.Set("Origin", "https://gemini.google.com")
	req.Header.Set("Referer", "https://gemini.google.com/")
	req.Header.Set("X-Same-Domain", "1")
	req.Header.Set(HeaderModelKey, `[1,null,null,null,null,null,null,null,[4,5,6,8],null,null,null,null,null,null,null]`)
	req.Header.Set(HeaderExtra1, "[0]")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batchexecute 失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	return parseResponseFrames(string(body)), nil
}

// nextReqID 生成递增的 _reqid。
func (c *Client) nextReqID() int {
	c.reqID += 100000
	if c.reqID > 999999999 {
		c.reqID = 10000
	}
	return c.reqID
}

// generateURL 拼接 StreamGenerate 的 query。
func (c *Client) generateURL(reqID int) string {
	params := url.Values{}
	params.Set("hl", c.language)
	params.Set("_reqid", strconv.Itoa(reqID))
	params.Set("rt", "c")
	if c.buildLabel != "" {
		params.Set("bl", c.buildLabel)
	}
	if c.sessionID != "" {
		params.Set("f.sid", c.sessionID)
	}
	return EndpointGenerate + "?" + params.Encode()
}

// 选择模型。
func (c *Client) resolveModel(name string) *AvailableModel {
	if name == "" {
		return nil
	}
	target := strings.ToLower(strings.TrimSpace(name))
	for _, m := range c.models {
		if m.Name == target || strings.EqualFold(m.Name, target) {
			return m
		}
		for _, a := range m.Aliases {
			if strings.EqualFold(a, target) {
				return m
			}
		}
	}
	// 版本无关匹配：去掉 gemini- 前缀
	normTarget := strings.ReplaceAll(strings.TrimPrefix(target, "gemini-"), "_", "-")
	for _, m := range c.models {
		norm := strings.ReplaceAll(strings.TrimPrefix(m.Name, "gemini-"), "_", "-")
		if norm == normTarget || strings.Contains(norm, normTarget) {
			return m
		}
	}
	return nil
}

// Generate 非流式对话，返回完整文本。
func (c *Client) Generate(ctx context.Context, prompt string, modelName string) (string, error) {
	var sb strings.Builder
	_, err := c.GenerateStream(ctx, prompt, modelName, func(delta string) error {
		sb.WriteString(delta)
		return nil
	})
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}

// GenerateStream 流式对话，逐段回调增量文本。
func (c *Client) GenerateStream(ctx context.Context, prompt string, modelName string, onDelta func(string) error) (string, error) {
	c.mu.Lock()
	if !c.ready {
		c.mu.Unlock()
		return "", ErrNotReady
	}
	model := c.resolveModel(modelName)
	reqID := c.nextReqID()
	language := c.language
	accessToken := c.accessToken
	jar := c.jar
	modelNumber := 0
	modelHeaders := map[string]string{}
	if model != nil {
		modelNumber = model.ModelNumber
		modelHeaders = model.Header()
	}
	uuidVal := strings.ToUpper(randID())
	c.lastActivity = time.Now()
	c.mu.Unlock()

	form := buildGenerateForm(prompt, language, accessToken, modelNumber, false, true)
	u := c.generateURL(reqID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	jar.ApplyToRequest(req)
	req.Header.Set("Content-Type", ContentTypeForm)
	req.Header.Set("Origin", "https://gemini.google.com")
	req.Header.Set("Referer", "https://gemini.google.com/")
	req.Header.Set("X-Same-Domain", "1")
	req.Header.Set(HeaderModelStreamKey, fmt.Sprintf(`["%s",1]`, uuidVal))
	for k, v := range modelHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("对话请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("对话请求失败: HTTP %d (%s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// 流式解析与候选提取
	parser := NewStreamingFrameParser()
	lastTexts := map[string]string{}
	var full strings.Builder
	reader := bufio.NewReaderSize(resp.Body, 64<<10)
	buf := make([]byte, 32<<10)
	for {
		n, rerr := reader.Read(buf)
		if n > 0 {
			frames := parser.Feed(string(buf[:n]))
			if err := c.processFrames(frames, lastTexts, &full, onDelta); err != nil {
				return "", err
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				return "", fmt.Errorf("读取流失败: %w", rerr)
			}
			break
		}
	}
	if full.Len() == 0 {
		return "", ErrStreamSuspended
	}
	c.touchActivity()
	return full.String(), nil
}

// processFrames 处理一帧。
func (c *Client) processFrames(frames []any, lastTexts map[string]string, full *strings.Builder, onDelta func(string) error) error {
	for _, frame := range frames {
		part, ok := frame.([]any)
		if !ok || len(part) < 3 {
			continue
		}
		// 检查错误码 [5][2][0][1][0]
		if errCode, ok := toInt(getNested(part, nil, 5, 2, 0, 1, 0)); ok && errCode != 0 {
			switch errCode {
			case 1037:
				return ErrUsageLimit
			case 1050, 1052:
				return ErrModelInvalid
			case 1060:
				return ErrIPBlocked
			default:
				return fmt.Errorf("gemini 服务返回错误码 %d", errCode)
			}
		}
		innerStr, _ := part[2].(string)
		if innerStr == "" {
			continue
		}
		var inner any
		if err := jsonUnmarshal([]byte(innerStr), &inner); err != nil {
			continue
		}
		candidates, ok := getNested(inner, nil, 4).([]any)
		if !ok {
			continue
		}
		for _, cd := range candidates {
			cand, ok := cd.([]any)
			if !ok {
				continue
			}
			rcid, _ := getNested(cand, "", 0).(string)
			text := parseCandidateText(cand)
			if text == "" {
				continue
			}
			prev := lastTexts[rcid]
			if len(text) > len(prev) && strings.HasPrefix(text, prev) {
				delta := text[len(prev):]
				if delta != "" {
					full.WriteString(delta)
					if onDelta != nil {
						if err := onDelta(delta); err != nil {
							return err
						}
					}
				}
				lastTexts[rcid] = text
			} else if prev == "" {
				// 第一帧（新增候选）
				lastTexts[rcid] = text
			}
		}
		// final 标记：inner[25] 存在时代表本轮结束
		if _, ok := getNested(inner, nil, 25).(string); ok {
			// 结束帧，继续读以收集剩余 delta
		}
	}
	return nil
}

// touchActivity 更新活跃时间。
func (c *Client) touchActivity() {
	c.mu.Lock()
	c.lastActivity = time.Now()
	c.mu.Unlock()
}

// Rotate 轮换 __Secure-1PSIDTS（后台任务调用）。
func (c *Client) Rotate(ctx context.Context) error {
	c.mu.Lock()
	jar := c.jar
	httpClient := c.http
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, EndpointRotateCookies, strings.NewReader(`[000,"-0000000000000000000"]`))
	if err != nil {
		return err
	}
	jar.ApplyToRequest(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://accounts.google.com")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		c.setStatus(StatusUnauthenticated)
		return ErrAuth
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("轮换 Cookie 失败: HTTP %d", resp.StatusCode)
	}
	c.mu.Lock()
	jar.UpdateFromResponse(resp)
	c.mu.Unlock()
	return saveCookies(c.cookieStore, jar)
}

// StartAutoRefresh 后台协程按间隔自动轮换 Cookie。
func (c *Client) StartAutoRefresh(ctx context.Context, interval time.Duration) {
	if interval < 60*time.Second {
		interval = 60 * time.Second
	}
	stop := make(chan struct{})
	c.mu.Lock()
	c.refreshStop = stop
	c.mu.Unlock()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-time.After(interval):
				if !c.Ready() {
					continue
				}
				_ = c.Rotate(ctx)
			}
		}
	}()
}

// StopAutoRefresh 停止自动轮换。
func (c *Client) StopAutoRefresh() {
	c.mu.Lock()
	if c.refreshStop != nil {
		close(c.refreshStop)
		c.refreshStop = nil
	}
	c.mu.Unlock()
}

func (c *Client) setStatus(status int) {
	c.mu.Lock()
	c.accountStatus = status
	c.mu.Unlock()
}

func stringsJoinID(payloads []rpcPayload) string {
	ids := make([]string, 0, len(payloads))
	for _, p := range payloads {
		ids = append(ids, p.RPCID)
	}
	return strings.Join(ids, ",")
}
