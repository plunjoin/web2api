package flow

import (
	"net/http"
	"time"

	"web2api/internal/engine/browsertransport"
)

// newFlowHTTPClient 构造 Flow 的 HTTP 客户端。
//
// Flow 会校验 TLS 指纹与请求头顺序，所以用浏览器指纹传输层而不是标准库
// 客户端。响应头超时给 60s：批量执行 RPC 的往返本来就比普通 API 慢，
// 但也不能无限等——卡死的连接会占住账号的并发额度。
func newFlowHTTPClient(proxyURL string) (*http.Client, error) {
	roundTripper, err := browsertransport.New(proxyURL)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: roundTripper}, nil
}

// flowRequestTimeout 是单次 Flow 请求的整体超时。
const flowRequestTimeout = 90 * time.Second
