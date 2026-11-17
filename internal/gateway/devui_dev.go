//go:build dev

// 开发态前端服务器：`wf dev`（= go run -tags dev . dev）时接管 /src、/@deps 与 HMR。
// 单独成文件并加 build tag，保证生产二进制里没有 esbuild / fsnotify。
package gateway

import (
	"net/http"
	"os"

	"github.com/plunjoin/webfront/dev"
)

// newDevUI 只在显式传入 dev 参数时启用，避免 `go run -tags dev .` 误把服务切到开发态。
func newDevUI() http.Handler {
	if !hasArg("dev") {
		return nil
	}
	root := os.Getenv("WF_WEB_DIR")
	if root == "" {
		root = "web"
	}
	ds, err := dev.NewDevServer(root)
	if err != nil {
		panic(err)
	}
	return ds.Handler()
}

func hasArg(want string) bool {
	for _, a := range os.Args[1:] {
		if a == want {
			return true
		}
	}
	return false
}
