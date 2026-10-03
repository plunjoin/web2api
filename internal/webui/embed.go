// Package webui 内嵌号池管理台单页应用（/admin）。
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var distFS embed.FS

// Assets 提供内嵌的前端依赖，不需要浏览器访问外部 CDN。
func Assets() http.Handler {
	assets, err := fs.Sub(distFS, "dist/assets")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/admin/assets/", http.FileServer(http.FS(assets)))
}

// ServeIndex 输出管理台页面。
func ServeIndex(w http.ResponseWriter, r *http.Request) {
	data, err := distFS.ReadFile("dist/index.html")
	if err != nil {
		http.Error(w, "管理台资源缺失", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
