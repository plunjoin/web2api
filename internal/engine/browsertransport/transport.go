// Package browsertransport 提供「长得像浏览器」的 HTTP 传输层。
//
// 上游（Google 的 Flow / AI Studio）会检查 TLS 指纹与请求头顺序：
// Go 标准库的 JA3 与 Chrome 差别明显，容易被判成机器人。这里用
// bogdanfinn/tls-client 伪装成 Chrome 指纹，并固定请求头顺序。
//
// AI Studio 引擎原本自带一份同样的实现；Flow 引擎也要用，所以抽到这里
// 共用——两份实现各自漂移迟早会出现「A 引擎能过、B 引擎过不去」的怪事。
package browsertransport

import (
	"bufio"
	"context"
	cryptotls "crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"golang.org/x/net/proxy"
)

// proxyConnectTimeout 是连代理的超时。
const proxyConnectTimeout = 30 * time.Second

// headerOrder 是浏览器发送请求时的头顺序。
//
// 顺序本身参与指纹：同一组头换个顺序，JA3/HA2 就变了。
var headerOrder = []string{
	"user-agent",
	"accept",
	"accept-language",
	"accept-encoding",
	"referer",
	"content-type",
	"authorization",
	"cookie",
	"origin",
	"sec-fetch-dest",
	"sec-fetch-mode",
	"sec-fetch-site",
	"priority",
	"te",
}

// RoundTripper 是伪装成浏览器的传输层。
type RoundTripper struct {
	client tlsclient.HttpClient
}

// New 构造浏览器指纹传输层。proxyURL 为空表示直连。
func New(proxyURL string) (http.RoundTripper, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	var proxyDialer proxy.ContextDialer
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed.Hostname() == "" {
			return nil, fmt.Errorf("代理 URL 无效")
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https", "socks5":
		default:
			return nil, fmt.Errorf("代理协议必须是 http、https 或 socks5")
		}
		proxyDialer, err = newProxyDialer(parsed)
		if err != nil {
			return nil, err
		}
	}
	options := []tlsclient.HttpClientOption{
		tlsclient.WithClientProfile(chromeProfile()),
		tlsclient.WithTimeoutMilliseconds(0),
		tlsclient.WithNotFollowRedirects(),
	}
	if proxyDialer != nil {
		options = append(options, tlsclient.WithDialContext(proxyDialer.DialContext))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("创建浏览器网络传输: %w", err)
	}
	return &RoundTripper{client: client}, nil
}

// RoundTrip 执行一次请求。
func (t *RoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	var payload io.Reader
	if request.Body != nil {
		payload = request.Body
	}
	upstream, err := fhttp.NewRequestWithContext(request.Context(), request.Method, request.URL.String(), payload)
	if err != nil {
		if request.Body != nil {
			_ = request.Body.Close()
		}
		return nil, err
	}
	upstream.Host = request.Host
	upstream.ContentLength = request.ContentLength
	upstream.Header = make(fhttp.Header, len(request.Header)+1)
	for name, values := range request.Header {
		upstream.Header[name] = append([]string(nil), values...)
	}
	upstream.Header[fhttp.HeaderOrderKey] = orderedHeaderNames(upstream.Header)
	response, err := t.client.Do(upstream)
	if err != nil {
		return nil, err
	}
	trailer := standardHeader(response.Trailer)
	responseContent := io.ReadCloser(http.NoBody)
	if response.Body != nil {
		responseContent = &responseBody{body: response.Body, source: response, trailer: trailer}
	}
	responseRequest := new(http.Request)
	*responseRequest = *request
	responseRequest.Body = nil
	return &http.Response{
		Status:           response.Status,
		StatusCode:       response.StatusCode,
		Proto:            response.Proto,
		ProtoMajor:       response.ProtoMajor,
		ProtoMinor:       response.ProtoMinor,
		Header:           standardHeader(response.Header),
		Body:             responseContent,
		ContentLength:    response.ContentLength,
		TransferEncoding: append([]string(nil), response.TransferEncoding...),
		Close:            response.Close,
		Uncompressed:     response.Uncompressed,
		Trailer:          trailer,
		Request:          responseRequest,
	}, nil
}

// CloseIdleConnections 关闭空闲连接。
func (t *RoundTripper) CloseIdleConnections() {
	t.client.CloseIdleConnections()
}

// responseBody 包装上游响应体，保持 Trailer 同步。
type responseBody struct {
	body    io.ReadCloser
	source  *fhttp.Response
	trailer http.Header
}

func (b *responseBody) Read(buffer []byte) (int, error) {
	read, err := b.body.Read(buffer)
	if err == io.EOF {
		b.syncTrailer()
	}
	return read, err
}

func (b *responseBody) Close() error {
	err := b.body.Close()
	b.syncTrailer()
	return err
}

func (b *responseBody) syncTrailer() {
	clear(b.trailer)
	for name, values := range b.source.Trailer {
		b.trailer[name] = append([]string(nil), values...)
	}
}

// proxyConnectDialer 通过 HTTP CONNECT 隧道走代理。
type proxyConnectDialer struct {
	proxyURL    *url.URL
	proxyTarget string
	direct      *net.Dialer
}

// newProxyDialer 构造代理拨号器。
func newProxyDialer(proxyURL *url.URL) (proxy.ContextDialer, error) {
	direct := &net.Dialer{Timeout: proxyConnectTimeout, KeepAlive: 30 * time.Second}
	proxyTarget := proxyURL.Host
	if proxyURL.Port() == "" {
		switch strings.ToLower(proxyURL.Scheme) {
		case "http":
			proxyTarget = net.JoinHostPort(proxyURL.Hostname(), "80")
		case "https":
			proxyTarget = net.JoinHostPort(proxyURL.Hostname(), "443")
		case "socks5":
			return nil, fmt.Errorf("SOCKS5 代理 URL 缺少端口")
		}
	}
	if strings.EqualFold(proxyURL.Scheme, "socks5") {
		var auth *proxy.Auth
		if proxyURL.User != nil {
			password, _ := proxyURL.User.Password()
			auth = &proxy.Auth{User: proxyURL.User.Username(), Password: password}
		}
		dialer, err := proxy.SOCKS5("tcp", proxyTarget, auth, direct)
		if err != nil {
			return nil, fmt.Errorf("创建 SOCKS5 代理连接: %w", err)
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("SOCKS5 代理不支持上下文取消")
		}
		return contextDialer, nil
	}
	return &proxyConnectDialer{proxyURL: proxyURL, proxyTarget: proxyTarget, direct: direct}, nil
}

// Dial 拨号（不带上下文）。
func (d *proxyConnectDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

// DialContext 通过代理建隧道。
func (d *proxyConnectDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var connection net.Conn
	var err error
	if strings.EqualFold(d.proxyURL.Scheme, "https") {
		tlsDialer := cryptotls.Dialer{
			NetDialer: d.direct,
			Config: &cryptotls.Config{
				ServerName: d.proxyURL.Hostname(),
				NextProtos: []string{"http/1.1"},
			},
		}
		connection, err = tlsDialer.DialContext(ctx, network, d.proxyTarget)
	} else {
		connection, err = d.direct.DialContext(ctx, network, d.proxyTarget)
	}
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = connection.Close()
		}
	}()
	deadline := time.Now().Add(proxyConnectTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return nil, err
	}
	stopCancel := context.AfterFunc(ctx, func() {
		_ = connection.Close()
	})
	defer stopCancel()
	request := (&http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: address},
		Host:   address,
		Header: make(http.Header),
	}).WithContext(ctx)
	if d.proxyURL.User != nil && d.proxyURL.User.Username() != "" {
		password, _ := d.proxyURL.User.Password()
		credentials := d.proxyURL.User.Username() + ":" + password
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(credentials)))
	}
	if err := request.Write(connection); err != nil {
		return nil, err
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		if response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, fmt.Errorf("代理 CONNECT 返回 %s", response.Status)
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	success = true
	return connection, nil
}

// standardHeader 把 fhttp.Header 转成标准库 Header。
func standardHeader(source fhttp.Header) http.Header {
	result := make(http.Header, len(source))
	for name, values := range source {
		result[name] = append([]string(nil), values...)
	}
	return result
}

// orderedHeaderNames 按浏览器顺序排列请求头。
func orderedHeaderNames(headers fhttp.Header) []string {
	result := make([]string, 0, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for _, name := range headerOrder {
		if _, ok := headers[http.CanonicalHeaderKey(name)]; ok {
			result = append(result, name)
			seen[name] = struct{}{}
		}
	}
	remaining := make([]string, 0, len(headers))
	for name := range headers {
		name = strings.ToLower(name)
		if name == strings.ToLower(fhttp.HeaderOrderKey) {
			continue
		}
		if _, ok := seen[name]; !ok {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	return append(result, remaining...)
}

// chromeProfile 构造 Chrome TLS 指纹。
func chromeProfile() profiles.ClientProfile {
	return profiles.Chrome_146
}

var _ http.RoundTripper = (*RoundTripper)(nil)
