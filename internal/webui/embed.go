// Package webui 内嵌 React 单页应用（/admin 管理台、/console 用户控制台、/login、/register）。
//
// 前端源码在 web/，由 `npm run build` 输出到 dist/（已提交到仓库，go build 不需要 Node）。
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
	// Vite 产物文件名自带 8 位内容哈希（如 index-DTrA9p-0.js），内容变化文件名就变化。
	hashedNameExpr = regexp.MustCompile(`^[A-Za-z0-9._-]+-[A-Za-z0-9_-]{8}\.(js|css|woff2?|svg|png|webp)$`)
)

// isContentHashed 文件名本身已含内容哈希，可以永久缓存，也不能再追加 ?v=
// （分包之间用 ./index-xxx.js 相对引用，多一个查询参数就会被当成另一个模块再执行一次）。
func isContentHashed(name string) bool { return hashedNameExpr.MatchString(name) }

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
			if !ok || isContentHashed(string(parts[2])) {
				return match
			}
			return []byte(string(parts[1]) + assetPrefix + string(parts[2]) + "?v=" + hash + string(parts[3]))
		})
	})
}

// AssetURL 返回可安全缓存的资源地址：文件名已含哈希时原样返回，否则追加 ?v=<内容哈希>；
// 资源不存在时返回不带版本的地址。
func AssetURL(name string) string {
	loadAssets()
	if hash, ok := assetHashes[name]; ok && !isContentHashed(name) {
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
			if isContentHashed(name) || r.URL.Query().Get("v") == hash {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
		}
		files.ServeHTTP(w, r)
	}))
}

// ServeIndex 输出单页应用入口（前端路由在浏览器里决定显示哪个页面）。
func ServeIndex(w http.ResponseWriter, r *http.Request) {
	loadAssets()
	if indexErr != nil || indexHTML == nil {
		http.Error(w, "前端资源缺失，请先在 web/ 目录执行 npm run build", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(indexHTML)
}
