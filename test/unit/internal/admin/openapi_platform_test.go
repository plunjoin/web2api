//go:build web2api_unit

package admin

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// 每个注册的 /admin/api 与 /api 路由都要出现在 OpenAPI 文档里，方法一致。
func TestOpenAPICoversRoutes(t *testing.T) {
	src, err := os.ReadFile("admin.go")
	if err != nil {
		t.Fatal(err)
	}
	spec := openAPISpec()
	paths := spec["paths"].(map[string]any)
	route := regexp.MustCompile(`mux\.HandleFunc\("(GET|POST|PUT|PATCH|DELETE) (/(?:admin/)?api/[^"]*)"`)
	matches := route.FindAllStringSubmatch(string(src), -1)
	if len(matches) < 40 {
		t.Fatalf("only found %d routes", len(matches))
	}
	for _, m := range matches {
		method, path := strings.ToLower(m[1]), strings.ReplaceAll(m[2], "...}", "}")
		if strings.HasSuffix(path, "/") {
			continue // JSON 404 兜底
		}
		item, ok := paths[path].(map[string]any)
		if !ok {
			t.Errorf("%s %s missing from OpenAPI paths", m[1], path)
			continue
		}
		if _, ok := item[method]; !ok {
			t.Errorf("%s %s missing method in OpenAPI", m[1], path)
		}
	}
}
