// Package cdp 是一个极简的 Chrome DevTools Protocol 客户端。
//
// 只实现 Flow 引擎真正需要的那几件事：
//   - 启动本机的 Chrome / Edge（headless 或有头）
//   - 建一个隔离的浏览器上下文，注入 Google Cookie
//   - 在页面加载前注入脚本（Page.addScriptToEvaluateOnNewDocument）
//   - 导航到目标页并等待 load
//   - 在页面里执行 JS（Runtime.evaluate，awaitPromise）
//
// 不引第三方浏览器自动化框架：web2api 的部署环境大多没有 Node/Python，
// 而 CDP 本身只要一个 websocket 就能驱动。
package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ErrClosed 表示连接已关闭。
var ErrClosed = errors.New("cdp: 连接已关闭")

// RPCError 是 CDP 返回的协议级错误。
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return "cdp: 未知错误"
	}
	if e.Data != "" {
		return fmt.Sprintf("cdp: %s (%d): %s", e.Message, e.Code, e.Data)
	}
	return fmt.Sprintf("cdp: %s (%d)", e.Message, e.Code)
}

// Message 是一条 CDP 消息（请求响应或事件共用同一结构）。
type Message struct {
	ID        int             `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *RPCError       `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// Conn 是到浏览器 websocket 的一条连接。
type Conn struct {
	conn *websocket.Conn

	mu       sync.Mutex
	nextID   int
	pending  map[int]chan Message
	closed   bool
	closeErr error

	events chan Message
	done   chan struct{}
	once   sync.Once
}

// Dial 连接到浏览器调试 websocket。
func Dial(ctx context.Context, wsURL string) (*Conn, error) {
	dialer := &websocket.Dialer{
		HandshakeTimeout: 20 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("连接浏览器调试端口失败: %w", err)
	}
	c := &Conn{
		conn:    conn,
		pending: map[int]chan Message{},
		events:  make(chan Message, 256),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// readLoop 读取消息：响应按 id 分发，事件进 events 队列。
func (c *Conn) readLoop() {
	defer close(c.done)
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			c.closed = true
			c.closeErr = err
			pending := c.pending
			c.pending = map[int]chan Message{}
			c.mu.Unlock()
			for _, ch := range pending {
				close(ch)
			}
			return
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.ID != 0 {
			c.mu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.mu.Unlock()
			if ok {
				ch <- msg
				close(ch)
			}
			continue
		}
		if msg.Method != "" {
			select {
			case c.events <- msg:
			default:
				// 事件队列满了就丢掉：Flow 引擎只关心少数几个事件，
				// 丢弃比阻塞读取更安全（否则会卡住整条读循环）。
			}
		}
	}
}

// Call 发起一次 CDP 命令。
func (c *Conn) Call(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	c.nextID++
	id := c.nextID
	ch := make(chan Message, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	payload := map[string]any{"id": id, "method": method}
	if params != nil {
		payload["params"] = params
	}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("编码 CDP 命令失败: %w", err)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	err = c.conn.WriteMessage(websocket.TextMessage, encoded)
	c.mu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("发送 CDP 命令失败: %w", err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case msg, ok := <-ch:
		if !ok {
			return nil, ErrClosed
		}
		if msg.Error != nil {
			return nil, msg.Error
		}
		return msg.Result, nil
	}
}

// Events 返回事件队列。
func (c *Conn) Events() <-chan Message { return c.events }

// WaitEvent 等待指定方法的事件。
func (c *Conn) WaitEvent(ctx context.Context, method string, timeout time.Duration) (Message, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return Message{}, ctx.Err()
		case <-deadline.C:
			return Message{}, fmt.Errorf("等待事件 %s 超时", method)
		case msg, ok := <-c.events:
			if !ok {
				return Message{}, ErrClosed
			}
			if msg.Method == method {
				return msg, nil
			}
		}
	}
}

// Close 关闭连接。
func (c *Conn) Close() error {
	var err error
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		err = c.conn.Close()
	})
	return err
}

// ==================== 浏览器进程 ====================

// LaunchOptions 描述如何启动本地浏览器。
type LaunchOptions struct {
	// Binary 浏览器可执行文件路径；为空时按常见安装位置自动探测。
	Binary string
	// UserDataDir 用户数据目录；为空时用临时目录（浏览器退出后删除）。
	UserDataDir string
	// Proxy 代理地址（http:// / socks5://）；为空表示直连。
	Proxy string
	// Headless 是否以无头模式启动。
	Headless bool
	// ExtraArgs 追加的额外命令行参数。
	ExtraArgs []string
	// Timeout 等待调试端口就绪的超时。
	Timeout time.Duration
}

// Browser 是一个已启动的浏览器实例。
type Browser struct {
	cmd         *exec.Cmd
	conn        *Conn
	userDataDir string
	ownedDir    bool
	binPath     string
}

// 常见浏览器的 Windows 安装位置。
var browserCandidates = []string{
	`C:\Program Files\Google\Chrome\Application\chrome.exe`,
	`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	"/usr/bin/google-chrome",
	"/usr/bin/google-chrome-stable",
	"/usr/bin/chromium",
	"/usr/bin/chromium-browser",
	"/usr/bin/microsoft-edge",
}

// FindBrowser 探测本机可用的浏览器。
func FindBrowser() string {
	if env := strings.TrimSpace(os.Getenv("WEB2API_FLOW_BROWSER")); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	for _, candidate := range browserCandidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// Launch 启动浏览器并连上它的调试端口。
func Launch(ctx context.Context, opts LaunchOptions) (*Browser, error) {
	bin := strings.TrimSpace(opts.Binary)
	if bin == "" {
		bin = FindBrowser()
	}
	if bin == "" {
		return nil, errors.New("未找到 Chrome / Edge 可执行文件（可用 WEB2API_FLOW_BROWSER 指定路径）")
	}
	userDataDir := strings.TrimSpace(opts.UserDataDir)
	owned := false
	if userDataDir == "" {
		dir, err := os.MkdirTemp("", "web2api-flow-browser-*")
		if err != nil {
			return nil, fmt.Errorf("创建浏览器用户目录失败: %w", err)
		}
		userDataDir = dir
		owned = true
	}

	// --remote-debugging-port=0 让浏览器自己挑一个空闲端口，
	// 实际端口写在用户目录的 DevToolsActivePort 文件里。
	args := []string{
		"--remote-debugging-port=0",
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-features=Translate,MediaRouter,OptimizationHints",
		"--disable-popup-blocking",
		"--password-store=basic",
		"--use-mock-keychain",
		"--no-service-autorun",
		"--disable-dev-shm-usage",
	}
	if opts.Headless {
		args = append(args, "--headless=new")
	}
	if proxy := strings.TrimSpace(opts.Proxy); proxy != "" {
		args = append(args, "--proxy-server="+proxy)
	}
	args = append(args, opts.ExtraArgs...)
	args = append(args, "about:blank")

	cmd := exec.Command(bin, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		if owned {
			_ = os.RemoveAll(userDataDir)
		}
		return nil, fmt.Errorf("启动浏览器失败: %w", err)
	}

	b := &Browser{cmd: cmd, userDataDir: userDataDir, ownedDir: owned, binPath: bin}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	port, err := waitDevToolsPort(ctx, userDataDir, timeout)
	if err != nil {
		_ = b.Close()
		return nil, err
	}
	wsURL, err := devToolsWebSocketURL(ctx, port)
	if err != nil {
		_ = b.Close()
		return nil, err
	}
	conn, err := Dial(ctx, wsURL)
	if err != nil {
		_ = b.Close()
		return nil, err
	}
	b.conn = conn
	return b, nil
}

// waitDevToolsPort 轮询 DevToolsActivePort 文件拿调试端口。
func waitDevToolsPort(ctx context.Context, userDataDir string, timeout time.Duration) (int, error) {
	path := filepath.Join(userDataDir, "DevToolsActivePort")
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) > 0 {
				if port, convErr := strconv.Atoi(strings.TrimSpace(lines[0])); convErr == nil && port > 0 {
					return port, nil
				}
			}
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("等待浏览器调试端口超时（%s）", path)
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// devToolsWebSocketURL 从 /json/version 拿浏览器级 websocket 地址。
func devToolsWebSocketURL(ctx context.Context, port int) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("读取浏览器版本信息失败: %w", err)
	}
	defer resp.Body.Close()
	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("解析浏览器版本信息失败: %w", err)
	}
	if payload.WebSocketDebuggerURL == "" {
		return "", errors.New("浏览器未返回调试 websocket 地址")
	}
	return payload.WebSocketDebuggerURL, nil
}

// Conn 返回浏览器的 CDP 连接。
func (b *Browser) Conn() *Conn { return b.conn }

// Binary 返回浏览器可执行文件路径。
func (b *Browser) Binary() string { return b.binPath }

// UserDataDir 返回浏览器用户数据目录。
func (b *Browser) UserDataDir() string { return b.userDataDir }

// Close 关闭浏览器并清理临时用户目录。
func (b *Browser) Close() error {
	if b.conn != nil {
		_ = b.conn.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		_, _ = b.cmd.Process.Wait()
	}
	if b.ownedDir && b.userDataDir != "" {
		_ = os.RemoveAll(b.userDataDir)
	}
	return nil
}

// ==================== 页面 ====================

// Page 是一个已附着的页面（target）。
type Page struct {
	conn      *Conn
	targetID  string
	sessionID string
}

// NewContext 创建一个隔离的浏览器上下文（相当于无痕窗口）。
// 每个 Flow 账号用它自己的上下文，Cookie 互不干扰。
func (b *Browser) NewContext(ctx context.Context) (string, error) {
	result, err := b.conn.Call(ctx, "Target.createBrowserContext", map[string]any{}, "")
	if err != nil {
		return "", err
	}
	var payload struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		return "", fmt.Errorf("解析浏览器上下文失败: %w", err)
	}
	return payload.BrowserContextID, nil
}

// DisposeContext 销毁浏览器上下文。
func (b *Browser) DisposeContext(ctx context.Context, browserContextID string) error {
	if browserContextID == "" {
		return nil
	}
	_, err := b.conn.Call(ctx, "Target.disposeBrowserContext", map[string]any{
		"browserContextId": browserContextID,
	}, "")
	return err
}

// NewPage 在指定上下文里开一个新页面并附着。
func (b *Browser) NewPage(ctx context.Context, browserContextID string, url string) (*Page, error) {
	params := map[string]any{"url": url}
	if browserContextID != "" {
		params["browserContextId"] = browserContextID
	}
	result, err := b.conn.Call(ctx, "Target.createTarget", params, "")
	if err != nil {
		return nil, err
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(result, &created); err != nil {
		return nil, fmt.Errorf("解析 targetId 失败: %w", err)
	}
	attachResult, err := b.conn.Call(ctx, "Target.attachToTarget", map[string]any{
		"targetId": created.TargetID,
		"flatten":  true,
	}, "")
	if err != nil {
		return nil, err
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attachResult, &attached); err != nil {
		return nil, fmt.Errorf("解析 sessionId 失败: %w", err)
	}
	page := &Page{conn: b.conn, targetID: created.TargetID, sessionID: attached.SessionID}
	if _, err := page.call(ctx, "Page.enable", nil); err != nil {
		return nil, err
	}
	if _, err := page.call(ctx, "Network.enable", nil); err != nil {
		return nil, err
	}
	return page, nil
}

// call 发起页面级命令。
func (p *Page) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return p.conn.Call(ctx, method, params, p.sessionID)
}

// TargetID 返回页面 target id。
func (p *Page) TargetID() string { return p.targetID }

// SetCookies 在当前浏览器上下文里写入 Cookie。
func (p *Page) SetCookies(ctx context.Context, browserContextID string, cookies []Cookie) error {
	if len(cookies) == 0 {
		return nil
	}
	params := map[string]any{"cookies": cookies}
	if browserContextID != "" {
		params["browserContextId"] = browserContextID
	}
	_, err := p.call(ctx, "Storage.setCookies", params)
	return err
}

// Cookie 是一条浏览器 Cookie。
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain,omitempty"`
	Path     string  `json:"path,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	HTTPOnly bool    `json:"httpOnly,omitempty"`
	SameSite string  `json:"sameSite,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
	URL      string  `json:"url,omitempty"`
}

// AddScriptOnNewDocument 在页面后续每次加载前注入脚本。
func (p *Page) AddScriptOnNewDocument(ctx context.Context, source string) error {
	_, err := p.call(ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]any{
		"source": source,
	})
	return err
}

// Navigate 导航到目标 URL 并等待 load 事件。
func (p *Page) Navigate(ctx context.Context, url string, timeout time.Duration) error {
	if _, err := p.call(ctx, "Page.navigate", map[string]any{"url": url}); err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("等待页面加载超时: %s", url)
		}
		wait := remaining
		if wait > 2*time.Second {
			wait = 2 * time.Second
		}
		if _, err := p.conn.WaitEvent(ctx, "Page.loadEventFired", wait); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// 事件可能被别的页面占用或已错过；轮询文档状态兜底。
		state, err := p.documentReadyState(ctx)
		if err == nil && state == "complete" {
			return nil
		}
	}
}

// documentReadyState 读取 document.readyState。
func (p *Page) documentReadyState(ctx context.Context) (string, error) {
	result, err := p.Evaluate(ctx, "document.readyState", false)
	if err != nil {
		return "", err
	}
	return result, nil
}

// Evaluate 在页面里执行 JS。awaitPromise 为真时等待 Promise 完成。
func (p *Page) Evaluate(ctx context.Context, expression string, awaitPromise bool) (string, error) {
	params := map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  awaitPromise,
	}
	result, err := p.call(ctx, "Runtime.evaluate", params)
	if err != nil {
		return "", err
	}
	var payload struct {
		Result struct {
			Type        string          `json:"type"`
			Subtype     string          `json:"subtype"`
			Value       json.RawMessage `json:"value"`
			Description string          `json:"description"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception *struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		return "", fmt.Errorf("解析 CDP 执行结果失败: %w", err)
	}
	if payload.ExceptionDetails != nil {
		detail := payload.ExceptionDetails.Text
		if payload.ExceptionDetails.Exception != nil && payload.ExceptionDetails.Exception.Description != "" {
			detail = payload.ExceptionDetails.Exception.Description
		}
		return "", fmt.Errorf("页面脚本执行失败: %s", detail)
	}
	if len(payload.Result.Value) == 0 {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(payload.Result.Value, &text); err == nil {
		return text, nil
	}
	return string(payload.Result.Value), nil
}

// Close 关闭页面。
func (p *Page) Close(ctx context.Context) error {
	_, err := p.conn.Call(ctx, "Target.closeTarget", map[string]any{"targetId": p.targetID}, "")
	return err
}
