// errors.go 路由层语义化错误的兼容出口：实现已下沉到 engine 基础包
// （见 internal/engine/errors.go），网关与管理台仍从 provider 引用。
package provider

import "web2api/internal/engine"

// IsModelNotFound 判定错误是否为「无人声明该模型」。
func IsModelNotFound(err error) bool { return engine.IsModelNotFound(err) }

// IsUnsupportedEndpoint 判定错误是否为「端点与模型用途不匹配」。
func IsUnsupportedEndpoint(err error) bool { return engine.IsUnsupportedEndpoint(err) }
