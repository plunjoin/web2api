// Package webui 内嵌号池管理台单页应用（/admin）。
package webui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
)

//go:embed dist
var distFS embed.FS

const assetPrefix = "/admin/assets/"

var (
	assetsOnce   sync.Once
	assetHashes  map[string]string // 文件名 -> 内容哈希（前 12 位十六进制）
	indexHTML    []byte            // 已为资源地址追加 ?v=<哈希> 的页面
	indexErr     error
	assetRefExpr = regexp.MustCompile(`(["'])/admin/assets/([A-Za-z0-9._-]+)(["'])`)
)

// loadAssets 计算内嵌资源的内容哈希，并改写页面里的资源地址。
//
// 资源地址带上内容哈希后，每次发版 CSS/JS 都换新 URL，浏览器或 CDN 里缓存的旧版本
// 不会再和新页面混用（v0.2.5 升级后旧 admin.css 被缓存，页面因此失去全部样式）。
func loadAssets() {
	assetsOnce.Do(func() {
		assetHashes = map[string]string{}
		entries, err := fs.ReadDir(distFS, "dist/assets")
		if err != nil {
			indexErr = err
			return
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := distFS.ReadFile("dist/assets/" + entry.Name())
			if err != nil {
				indexErr = err
				return
			}
			sum := sha256.Sum256(data)
			assetHashes[entry.Name()] = hex.EncodeToString(sum[:])[:12]
		}
		data, err := distFS.ReadFile("dist/index.html")
		if err != nil {
			indexErr = err
			return
		}
		indexHTML = assetRefExpr.ReplaceAllFunc(data, func(match []byte) []byte {
			parts := assetRefExpr.FindSubmatch(match)
			hash, ok := assetHashes[string(parts[2])]
			if !ok {
				return match
			}
			return []byte(string(parts[1]) + assetPrefix + string(parts[2]) + "?v=" + hash + string(parts[3]))
		})
	})
}

// AssetURL 返回带内容哈希的资源地址；资源不存在时返回不带版本的地址。
func AssetURL(name string) string {
	loadAssets()
	if hash, ok := assetHashes[name]; ok {
		return assetPrefix + name + "?v=" + hash
	}
	return assetPrefix + name
}

// Assets 提供内嵌的前端依赖，不需要浏览器访问外部 CDN。
//
// 每个资源都带 ETag；地址里的 ?v= 与当前内容哈希一致时允许长期缓存，
// 否则要求每次向服务端校验，避免旧缓存与新页面混用。
func Assets() http.Handler {
	loadAssets()
	assets, err := fs.Sub(distFS, "dist/assets")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	return http.StripPrefix(strings.TrimSuffix(assetPrefix, "/"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Base(r.URL.Path)
		if hash, ok := assetHashes[name]; ok {
			w.Header().Set("ETag", `"`+hash+`"`)
			if r.URL.Query().Get("v") == hash {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
		}
		files.ServeHTTP(w, r)
	}))
}

// ServeIndex 输出管理台页面。
func ServeIndex(w http.ResponseWriter, r *http.Request) {
	loadAssets()
	if indexErr != nil || indexHTML == nil {
		http.Error(w, "管理台资源缺失", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(indexHTML)
}
