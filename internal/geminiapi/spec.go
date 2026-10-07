package geminiapi

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed official.openapi.json
var officialSpec []byte

// Spec returns a fresh object so callers can customize gateway authentication.
func Spec() map[string]any {
	var spec map[string]any
	if err := json.Unmarshal(officialSpec, &spec); err != nil {
		panic(err)
	}
	return spec
}

// Mounts includes every resource from the official snapshot, plus Files API
// uploads/downloads and model discovery required for media workflows.
func Mounts() []string {
	set := map[string]bool{"/v1beta/": true, "/upload/": true, "/download/": true, "/gemini-upload/": true}
	for name := range Spec()["paths"].(map[string]any) {
		if strings.HasPrefix(name, "/{api_version}/") {
			resource := strings.Split(strings.TrimPrefix(name, "/{api_version}/"), "/")[0]
			set["/v1/"+resource] = true
			set["/v1/"+resource+"/"] = true
		}
	}
	// /v1/files belongs to the existing OpenAI upstream interface. Stable Google
	// files calls are available under /gemini/v1/files to avoid that collision.
	set["/gemini/"] = true
	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// GatewaySpec uses concrete gateway paths and gateway authentication. API
// version expansion retains every operation and every component of the source.
func GatewaySpec() map[string]any {
	spec := Spec()
	spec["servers"] = []any{map[string]any{"url": "/", "description": "web2api 官方 Gemini 后端"}}
	spec["security"] = []any{map[string]any{"GeminiGatewayKey": []any{}}}
	components := spec["components"].(map[string]any)
	components["securitySchemes"] = map[string]any{"GeminiGatewayKey": map[string]any{
		"type": "apiKey", "in": "header", "name": "x-goog-api-key",
		"description": "填写网关 sk- Key；Google API Key 由服务器配置。也支持 Authorization: Bearer 或查询 key。",
	}}
	paths := map[string]any{}
	for name, value := range spec["paths"].(map[string]any) {
		for _, version := range []string{"v1beta", "v1"} {
			data, _ := json.Marshal(value)
			var item map[string]any
			_ = json.Unmarshal(data, &item)
			parameters, _ := item["parameters"].([]any)
			filtered := []any{}
			for _, parameter := range parameters {
				p, _ := parameter.(map[string]any)
				if p["$ref"] != "#/components/parameters/api_version" {
					filtered = append(filtered, parameter)
				}
			}
			item["parameters"] = filtered
			for _, method := range []string{"get", "post", "patch", "put", "delete"} {
				if op, ok := item[method].(map[string]any); ok {
					op["operationId"] = "Gemini_" + version + "_" + op["operationId"].(string)
					op["tags"] = []string{"Gemini 官方 API"}
					delete(op, "security")
				}
			}
			paths[strings.ReplaceAll(name, "{api_version}", version)] = item
		}
	}
	spec["paths"] = paths
	spec["tags"] = []any{map[string]any{"name": "Gemini 官方 API", "description": "完整转发官方参数，模型、权限和版本支持由 Google 校验。v1 使用官方稳定版的支持子集。"}}
	return spec
}

// MergeDocs adds official operations without confusing them with Chat Completions.
func MergeDocs(spec map[string]any) {
	google := GatewaySpec()
	var rewrite func(any)
	rewrite = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok && strings.HasPrefix(ref, "#/components/") {
				parts := strings.Split(ref, "/")
				parts[len(parts)-1] = "Gemini_" + parts[len(parts)-1]
				v["$ref"] = strings.Join(parts, "/")
			}
			for _, child := range v {
				rewrite(child)
			}
		case []any:
			for _, child := range v {
				rewrite(child)
			}
		}
	}
	rewrite(google)
	components := spec["components"].(map[string]any)
	for kind, values := range google["components"].(map[string]any) {
		objects, ok := values.(map[string]any)
		if !ok {
			continue
		}
		if components[kind] == nil {
			components[kind] = map[string]any{}
		}
		for name, object := range objects {
			components[kind].(map[string]any)["Gemini_"+name] = object
		}
	}
	for name, value := range google["paths"].(map[string]any) {
		item := value.(map[string]any)
		for _, method := range []string{"get", "post", "patch", "put", "delete"} {
			if op, ok := item[method].(map[string]any); ok {
				op["security"] = []any{map[string]any{"Gemini_GeminiGatewayKey": []any{}}}
			}
		}
		spec["paths"].(map[string]any)[name] = item
	}
	spec["tags"] = append(spec["tags"].([]any), google["tags"].([]any)...)
}
