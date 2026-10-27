// errors.go 路由层与引擎共用的语义化错误：区分「没人声明这个模型」与
// 「引擎挂了」。放 engine 基础包，让引擎适配器在构造错误时不依赖 provider。
package engine

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
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

// NewModelNotFound 构造模型未声明错误。
func NewModelNotFound(model, detail string) error {
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
		detail = fmt.Sprintf("模型 %q 是%s模型，当前端点无法返回该类型的输出", e.Model, KindLabel(e.Kind))
	}
	return "unsupported_endpoint: " + detail
}

// IsUnsupportedEndpoint 判定错误是否为「端点与模型用途不匹配」。
func IsUnsupportedEndpoint(err error) bool {
	var target *UnsupportedEndpointError
	return errors.As(err, &target)
}

// NewUnsupportedEndpoint 构造「该换端点」的错误，文案里直接给出正确入口。
func NewUnsupportedEndpoint(model, kind string) error {
	name := strings.TrimPrefix(strings.TrimSpace(model), "models/")
	return &UnsupportedEndpointError{
		Model: name,
		Kind:  kind,
		Detail: fmt.Sprintf("模型 %q 是%s模型，/v1/chat/completions 只能返回文本，上游会直接拒绝（如 Unhandled generated data mime type: audio/mpeg）。请改用 %s；或启用引擎B 原生模式（它可以在 chat 响应里直接带回媒体）",
			name, KindLabel(kind), EndpointForKind(kind)),
	}
}

// EndpointForKind 该用途应使用的端点。
func EndpointForKind(kind string) string {
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

// KindLabel 模型用途的中文标签（错误文案用）。
func KindLabel(kind string) string {
	switch kind {
	case "video":
		return "视频"
	case "music":
		return "音乐"
	case "speech":
		return "语音"
	case "image":
		return "生图"
	case "embedding":
		return "向量"
	default:
		return "对话"
	}
}

// UnavailableError 保留引擎最近一次初始化/健康检查错误，避免线上只看到
// 「引擎不可用」而无法判断是账号、代理、网络还是上游协议问题。
func UnavailableError(eng Engine) error {
	msg := fmt.Sprintf("引擎 %s 不可用", eng.Name())
	if status := eng.Status(); status != nil {
		switch {
		case statusString(status["error"]) != "":
			msg += ": " + statusString(status["error"])
		case statusString(status["worker_error"]) != "":
			// 账号 ready 但 WAA Worker 起不来：这层信息最容易被漏掉，单独透出。
			msg += ": WAA Worker 未就绪（worker_ready=" + statusInt(status["worker_ready"]) + "）: " + statusString(status["worker_error"])
		case status["worker_ready"] != nil && statusInt(status["worker_ready"]) == "0":
			msg += ": WAA Worker 未就绪（worker_ready=0），账号在池但没有任何可用的工人"
		case emptyStatusList(status["accounts"]):
			msg += ": 没有可用账号（请在管理台添加账号，或检查 auth_states 配置）"
		case firstAccountError(status["accounts"]) != "":
			// 账号级错误（凭据失效 / 冷却中等）
			msg += ": " + firstAccountError(status["accounts"])
		}
	}
	return errors.New(msg)
}

func statusString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// statusInt 安全读取状态里的整数字段（数字或字符串形式）。
func statusInt(v any) string {
	switch n := v.(type) {
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case string:
		return strings.TrimSpace(n)
	default:
		return "0"
	}
}

// firstAccountError 从账号状态列表中提取第一条错误描述。列表既可能是
// []map[string]any，也可能是引擎自定义的账号状态切片（字段 Status/Detail/Label），
// 统一用反射读，避免 engine 包反向依赖具体引擎类型。
func firstAccountError(v any) string {
	if list, ok := v.([]any); ok {
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				status, _ := m["status"].(string)
				detail, _ := m["detail"].(string)
				label, _ := m["label"].(string)
				if status == "error" && strings.TrimSpace(detail) != "" {
					if label != "" {
						return label + ": " + strings.TrimSpace(detail)
					}
					return strings.TrimSpace(detail)
				}
			}
		}
		return ""
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return ""
	}
	for i := 0; i < rv.Len(); i++ {
		ev := rv.Index(i)
		if ev.Kind() == reflect.Ptr {
			ev = ev.Elem()
		}
		if ev.Kind() != reflect.Struct {
			continue
		}
		status, detail, label := ev.FieldByName("Status"), ev.FieldByName("Detail"), ev.FieldByName("Label")
		if !status.IsValid() || !detail.IsValid() {
			continue
		}
		if status.Kind() == reflect.String && status.String() == "error" && strings.TrimSpace(detail.String()) != "" {
			if label.IsValid() && label.Kind() == reflect.String && strings.TrimSpace(label.String()) != "" {
				return strings.TrimSpace(label.String()) + ": " + strings.TrimSpace(detail.String())
			}
			return strings.TrimSpace(detail.String())
		}
	}
	return ""
}

func emptyStatusList(v any) bool {
	switch list := v.(type) {
	case []map[string]any:
		return len(list) == 0
	case []any:
		return len(list) == 0
	default:
		// 订阅引擎的 []AccountState 等具体切片类型
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice {
			return rv.Len() == 0
		}
		return false
	}
}
