//go:build !dev

// 生产构建：不引入 webfront/dev（esbuild / fsnotify），前端直接用内嵌的 internal/webui/dist。
package gateway

import "net/http"

func newDevUI() http.Handler { return nil }
