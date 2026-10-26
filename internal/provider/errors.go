// errors.go 路由层的语义化错误：区分「没人声明这个模型」与「引擎挂了」。
package provider

import (
	"errors"
	"fmt"
	"strings"
)

// ModelNotFoundError 表示没有任何引擎/账号声明过该模型。
// 网关据此返回 404 + code=model_not_found，而不是把「路由不到」包装成
// 上游 502——后者会让下游误以为是临时故障而反复重试。
type ModelNotFoundError struct {
	Model  string // 请求的模型名（已去掉 models/ 前缀）
	Detail string // 面向人的补充说明：查过哪些号池、为什么没命中
}

func (e *ModelNotFoundError) Error() string {
	detail := strings.TrimSpace(e.Detail)
	if detail == "" {
		detail = fmt.Sprintf("当前号池没有引擎或账号声明模型 %q", e.Model)
	}
	return "model_not_found: " + detail
}

// newModelNotFound 构造模型未声明错误。
func newModelNotFound(model, detail string) error {
	return &ModelNotFoundError{Model: strings.TrimPrefix(strings.TrimSpace(model), "models/"), Detail: detail}
}

// IsModelNotFound 判定错误是否为「无人声明该模型」。
func IsModelNotFound(err error) bool {
	var target *ModelNotFoundError
	return errors.As(err, &target)
}

// UnsupportedEndpointError 模型存在，但当前端点产不出这种类型的输出。
// 例如把音乐模型发到 /v1/chat/completions：chat 只能返回文本，上游必然拒绝。
// 网关据此返回 400，并告诉调用方该换哪个端点。
type UnsupportedEndpointError struct {
	Model  string
	Kind   string
	Detail string
}

func (e *UnsupportedEndpointError) Error() string {
	detail := strings.TrimSpace(e.Detail)
	if detail == "" {
		detail = fmt.Sprintf("模型 %q 是%s模型，当前端点无法返回该类型的输出", e.Model, kindLabel(e.Kind))
	}
	return "unsupported_endpoint: " + detail
}

// IsUnsupportedEndpoint 判定错误是否为「端点与模型用途不匹配」。
func IsUnsupportedEndpoint(err error) bool {
	var target *UnsupportedEndpointError
	return errors.As(err, &target)
}

// newUnsupportedEndpoint 构造「该换端点」的错误，文案里直接给出正确入口。
func newUnsupportedEndpoint(model, kind string) error {
	name := strings.TrimPrefix(strings.TrimSpace(model), "models/")
	return &UnsupportedEndpointError{
		Model: name,
		Kind:  kind,
		Detail: fmt.Sprintf("模型 %q 是%s模型，/v1/chat/completions 只能返回文本，上游会直接拒绝（如 Unhandled generated data mime type: audio/mpeg）。请改用 %s；或启用引擎B 原生模式（它可以在 chat 响应里直接带回媒体）",
			name, kindLabel(kind), endpointForKind(kind)),
	}
}

// endpointForKind 该用途应使用的端点。
func endpointForKind(kind string) string {
	switch kind {
	case "music":
		return "POST /v1/music/generations"
	case "video":
		return "POST /v1/videos"
	case "embedding":
		return "POST /v1/embeddings"
	case "image":
		return "POST /v1/images/generations"
	case "speech":
		return "POST /v1/audio/speech"
	default:
		return "POST /v1/chat/completions"
	}
}
