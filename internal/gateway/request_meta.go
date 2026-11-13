package gateway

import (
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"web2api/internal/model"
	"web2api/internal/provider"
)

// clientName 从 User-Agent 识别调用方客户端，供用量明细的「AGENT」列展示。
// 识别不出来时退回 UA 的首个产品名，而不是丢掉这条信息。
func clientName(r *http.Request) string {
	ua := strings.TrimSpace(r.UserAgent())
	lower := strings.ToLower(ua)
	switch {
	case lower == "":
		return ""
	case strings.Contains(lower, "claude-code") || strings.Contains(lower, "claudecode"):
		return "Claude Code"
	case strings.Contains(lower, "claude-desktop") || strings.Contains(lower, "claudedesktop"):
		return "Claude Desktop"
	case strings.Contains(lower, "anthropic-sdk") || strings.HasPrefix(lower, "anthropic/"):
		return "Anthropic SDK"
	case strings.Contains(lower, "claude"):
		return "Claude Desktop"
	case strings.Contains(lower, "cc-switch") || strings.Contains(lower, "ccswitch"):
		return "CC Switch"
	case strings.Contains(lower, "cursor"):
		return "Cursor"
	case strings.Contains(lower, "windsurf"):
		return "Windsurf"
	case strings.Contains(lower, "vscode") || strings.Contains(lower, "visual studio code"):
		return "VS Code"
	case strings.Contains(lower, "codex"):
		return "Codex"
	case strings.Contains(lower, "cherry"):
		return "Cherry Studio"
	case strings.Contains(lower, "lobechat"):
		return "LobeChat"
	case strings.Contains(lower, "chatbox"):
		return "Chatbox"
	case strings.Contains(lower, "dify"):
		return "Dify"
	case strings.Contains(lower, "nextchat"):
		return "NextChat"
	case strings.Contains(lower, "openai-python") || strings.Contains(lower, "openai/node") || strings.HasPrefix(lower, "openai/"):
		return "OpenAI SDK"
	case strings.Contains(lower, "go-http-client"):
		return "Go HTTP"
	case strings.Contains(lower, "python-requests") || strings.Contains(lower, "python-urllib"):
		return "Python SDK"
	case strings.Contains(lower, "axios"):
		return "axios"
	case strings.Contains(lower, "postman"):
		return "Postman"
	case strings.Contains(lower, "curl"):
		return "curl"
	}
	// 通用兜底：取 UA 第一段（"name/version ..." → "name"）。
	name := ua
	if i := strings.IndexAny(name, "/ \t"); i > 0 {
		name = name[:i]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, name)
	if len(name) > 40 {
		name = name[:40]
	}
	return strings.TrimSpace(name)
}

// upstreamErrorStatus 把引擎错误映射成交给客户端的 HTTP 状态码，
// 与各入口写错误响应时使用的映射保持一致。
func upstreamErrorStatus(err error) int {
	switch {
	case provider.IsModelDisabled(err), provider.IsModelNotFound(err):
		return http.StatusNotFound
	case provider.IsUnsupportedEndpoint(err), provider.IsToolsNotSupported(err):
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}

// timedSink 包住流式输出目标，记录第一个增量到达的时刻（首字耗时）。
// 引擎都在返回前同步写完增量，但个别实现可能由内部协程写 sink，
// 因此计时用锁保护，读到的是稳定的首字时刻。
type timedSink struct {
	inner   model.StreamSink
	started time.Time
	mu      sync.Mutex
	first   time.Duration
}

func newTimedSink(inner model.StreamSink, started time.Time) *timedSink {
	return &timedSink{inner: inner, started: started}
}

func (t *timedSink) mark() {
	t.mu.Lock()
	if t.first == 0 {
		t.first = time.Since(t.started)
	}
	t.mu.Unlock()
}

func (t *timedSink) Text(delta string) error {
	if delta != "" {
		t.mark()
	}
	return t.inner.Text(delta)
}

func (t *timedSink) ToolCall(delta model.ToolCallDelta) error {
	t.mark()
	return t.inner.ToolCall(delta)
}

// TTFT 首字耗时；没有收到任何增量时返回 0。
func (t *timedSink) TTFT() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.first
}
