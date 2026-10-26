package aistudio

import (
	"io"
	"net/http"
	"time"
)

// Options 定义单个 AI Studio 账户的纯 Go Worker 配置
type Options struct {
	ExecutablePath   string
	StorageStatePath string
	Model            string
	BootstrapPrompt  string
	Locale           string
	Timezone         string
	Proxy            string
	ProxyBypass      string
	Headless         bool
	TemporaryChat    bool
	CacheDirectory   string
	ReadyTimeout     time.Duration
	Log              io.Writer
}

// State 返回 Worker 当前页面与 bootstrap 结果
type State struct {
	PID         int
	PageURL     string
	UserAgent   string
	Platform    string
	Timezone    string
	SnapshotKey string
	Headers     http.Header
}

// ProtectedResponse 表示上游流式返回的 MakerSuite 响应
type ProtectedResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}
