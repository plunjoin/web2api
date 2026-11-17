// attempts.go 记录一次请求在路由层的尝试轨迹：请求先落到哪个引擎、
// 失败后换到了谁、每次失败的原因是什么。用量明细靠它还原换号/换供应商路径。
package provider

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

// Attempt 一次引擎尝试。Error 为空表示这次尝试成功。
type Attempt struct {
	Engine     string `json:"engine"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// 轨迹最多保留的条数；正常号池不会触发，防止异常配置下无限增长。
const maxAttempts = 12

type attemptTrace struct {
	mu    sync.Mutex
	items []Attempt
}

type attemptTraceKey struct{}

// WithAttemptTrace 在 ctx 上挂载尝试轨迹。网关在调用 Manager 前挂载，
// Manager 的每次引擎尝试都会写入；没有挂载时记录为空操作。
func WithAttemptTrace(ctx context.Context) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, attemptTraceKey{}, &attemptTrace{})
}

// RecordAttempt 记录一次引擎尝试。started 为这次尝试的开始时间。
func RecordAttempt(ctx context.Context, engineName string, started time.Time, err error) {
	trace, _ := ctx.Value(attemptTraceKey{}).(*attemptTrace)
	if trace == nil {
		return
	}
	item := Attempt{Engine: engineName}
	if !started.IsZero() {
		item.DurationMs = time.Since(started).Milliseconds()
	}
	if err != nil {
		item.Error = truncate(err.Error(), 300)
	}
	trace.mu.Lock()
	if len(trace.items) < maxAttempts {
		trace.items = append(trace.items, item)
	} else {
		// 超出上限时丢掉最早的一条，保留最近尝试。
		copy(trace.items, trace.items[1:])
		trace.items[len(trace.items)-1] = item
	}
	trace.mu.Unlock()
}

// AttemptTrace 读取 ctx 上的尝试轨迹副本。
func AttemptTrace(ctx context.Context) []Attempt {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(attemptTraceKey{}).(*attemptTrace)
	if trace == nil {
		return nil
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	if len(trace.items) == 0 {
		return nil
	}
	out := make([]Attempt, len(trace.items))
	copy(out, trace.items)
	return out
}

// ErrorKind 把路由/上游错误归成稳定的类别，写入用量明细供聚合与筛选。
func ErrorKind(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case IsModelNotFound(err):
		return "model_not_found"
	case IsModelDisabled(err):
		return "model_unavailable"
	case IsUnsupportedEndpoint(err):
		return "unsupported_endpoint"
	case IsToolsNotSupported(err):
		return "tools_not_supported"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		return "upstream_timeout"
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "429"), strings.Contains(lower, "rate limit"), strings.Contains(lower, "too many requests"):
		return "upstream_rate_limited"
	case strings.Contains(lower, "401"), strings.Contains(lower, "403"),
		strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"),
		strings.Contains(lower, "token") && (strings.Contains(lower, "expired") || strings.Contains(lower, "invalid")):
		return "upstream_auth"
	case strings.Contains(lower, "未启用"), strings.Contains(lower, "不可用"),
		strings.Contains(lower, "no available account"), strings.Contains(lower, "冷却"):
		return "engine_unavailable"
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline exceeded"):
		return "upstream_timeout"
	default:
		return "upstream_error"
	}
}

// engineUnavailable 判断错误是否来自引擎自身不可用（未就绪/无可用账号）。
func engineUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "不可用") || strings.Contains(msg, "no available account") ||
		strings.Contains(msg, "未启用") || strings.Contains(msg, "没有可用账号")
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
