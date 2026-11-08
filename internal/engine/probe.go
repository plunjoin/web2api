package engine

import "context"

// probeContextKey 标记管理台模型探测请求。探测的目的是验证上游是否可达，
// 不能被动「模型已下架」的开关拦住：否则误关之后再也无法通过检查恢复。
// 该标记放在 engine 包而不是 provider 包，图片/向量等由引擎内部选号的路径
// 才能识别它（custom.ProbeImage/ProbeEmbedding 就是这样绕过 pick 的闸门）。
type probeContextKey struct{}

// WithProbeContext 给 ctx 打上「管理台探测」标记。
func WithProbeContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, probeContextKey{}, true)
}

// IsProbeContext 判断是否是管理台探测请求。
func IsProbeContext(ctx context.Context) bool {
	return ctx != nil && ctx.Value(probeContextKey{}) != nil
}
