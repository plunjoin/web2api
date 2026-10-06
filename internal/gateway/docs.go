package gateway

import (
	"net/http"
)

// handlePublicDocs 返回对外网关 API 的 OpenAPI 文档。管理台接口文档仍由
// /admin/api/docs 提供；两个文档分开，避免把管理员接口混入客户端协议。
func (s *Server) handlePublicDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, publicAPISpec())
}

func publicAPISpec() map[string]any {
	jsonContent := func(schema any) map[string]any {
		return map[string]any{"application/json": map[string]any{"schema": schema}}
	}
	jsonBody := func(schema string) map[string]any {
		return map[string]any{"required": true, "content": jsonContent(map[string]any{"$ref": "#/components/schemas/" + schema})}
	}
	jsonResponse := func(description, schema string) map[string]any {
		response := map[string]any{"description": description}
		if schema != "" {
			response["content"] = jsonContent(map[string]any{"$ref": "#/components/schemas/" + schema})
		}
		return response
	}
	pathID := map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}
	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "web2api 对外 API",
			"version":     "1.0.0",
			"description": mediaAPIOverview,
		},
		"servers":  []any{map[string]any{"url": "/", "description": "当前 web2api 服务"}},
		"security": []any{map[string]any{"ApiKey": []any{}}},
		"tags": []any{
			map[string]any{"name": "基础", "description": "健康检查、模型和账号状态"},
			map[string]any{"name": "聊天", "description": "OpenAI Chat Completions 兼容接口"},
			map[string]any{"name": "视频", "description": "Veo GenerateVideo 长任务接口"},
			map[string]any{"name": "图片", "description": "原生模式通过聊天接口生成图片；upstream 模式可透传图片专用接口"},
			map[string]any{"name": "音频", "description": "原生模式通过聊天接口请求音频；upstream 模式可透传语音专用接口"},
			map[string]any{"name": "透传", "description": "仅 upstream 引擎B可用的多模态透传路径"},
		},
		"paths": map[string]any{
			"/health": map[string]any{
				"get": map[string]any{"tags": []string{"基础"}, "summary": "健康检查", "security": []any{}, "responses": map[string]any{"200": jsonResponse("服务与引擎状态", "HealthResponse")}},
			},
			"/v1/docs": map[string]any{
				"get": map[string]any{"tags": []string{"基础"}, "summary": "获取本对外 API 文档", "security": []any{}, "responses": map[string]any{"200": map[string]any{"description": "OpenAPI 3.1 JSON"}}},
			},
			"/v1/models": map[string]any{
				"get": map[string]any{"tags": []string{"基础"}, "summary": "列出可用模型", "responses": map[string]any{"200": jsonResponse("模型列表", "ModelList"), "401": jsonResponse("API Key 无效", "Error")}},
			},
			"/v1/accounts": map[string]any{
				"get": map[string]any{"tags": []string{"基础"}, "summary": "查看引擎和账号池状态", "responses": map[string]any{"200": jsonResponse("账号状态", "AccountsResponse"), "401": jsonResponse("API Key 无效", "Error")}},
			},
			"/v1/chat/completions": map[string]any{
				"post": map[string]any{"tags": []string{"聊天"}, "summary": "创建聊天补全", "description": "stream=true 时返回 text/event-stream；普通请求返回 ChatCompletionResponse。", "requestBody": jsonBody("ChatCompletionRequest"), "responses": map[string]any{"200": jsonResponse("聊天结果或 SSE 流", "ChatCompletionResponse"), "400": jsonResponse("请求参数错误", "Error"), "401": jsonResponse("API Key 无效", "Error"), "502": jsonResponse("上游模型调用失败", "Error")}},
			},
			"/v1/videos": map[string]any{
				"post": map[string]any{"tags": []string{"视频"}, "summary": "创建 Veo 视频任务", "requestBody": jsonBody("VideoCreateRequest"), "responses": map[string]any{"202": jsonResponse("任务已创建", "VideoResponse"), "400": jsonResponse("请求参数错误", "Error"), "401": jsonResponse("API Key 无效", "Error"), "429": jsonResponse("上游限流、配额不足或账户冷却", "Error"), "502": jsonResponse("Veo 协议调用失败", "Error")}},
			},
			"/v1/videos/{id}": map[string]any{
				"parameters": []any{pathID},
				"get":        map[string]any{"tags": []string{"视频"}, "summary": "查询 Veo 任务状态", "responses": map[string]any{"200": jsonResponse("任务状态", "VideoResponse"), "401": jsonResponse("API Key 无效", "Error"), "429": jsonResponse("上游限流、配额不足或账户冷却", "Error"), "502": jsonResponse("Veo 协议调用失败", "Error")}},
			},
			"/v1/videos/{id}/content": map[string]any{
				"parameters": []any{pathID},
				"get":        map[string]any{"tags": []string{"视频"}, "summary": "下载完成的 Veo 视频", "responses": map[string]any{"200": map[string]any{"description": "video/mp4 二进制流", "content": map[string]any{"video/mp4": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}}, "409": jsonResponse("视频仍在生成", "Error"), "401": jsonResponse("API Key 无效", "Error"), "429": jsonResponse("上游限流、配额不足或账户冷却", "Error"), "502": jsonResponse("Veo 协议调用失败", "Error")}},
			},
			"/v1/images/{path}":     passthroughPath("图片透传（仅 upstream 模式）"),
			"/v1/audio/{path}":      passthroughPath("音频透传（仅 upstream 模式）"),
			"/v1/files/{path}":      passthroughPath("文件透传（仅 upstream 模式）"),
			"/v1/embeddings/{path}": passthroughPath("Embedding 透传（仅 upstream 模式）"),
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{"ApiKey": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "API Key", "description": "管理台创建的 sk- API Key"}},
			"schemas":         publicSchemas(),
		},
	}
	addMediaAPIDocs(spec)
	return spec
}

func passthroughPath(summary string) map[string]any {
	return map[string]any{"post": map[string]any{
		"tags": []string{"透传"}, "summary": summary,
		"requestBody": map[string]any{"required": false, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}, "application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}},
		"responses":   map[string]any{"200": map[string]any{"description": "上游响应"}, "401": map[string]any{"description": "API Key 无效"}, "502": map[string]any{"description": "透传失败"}},
	}}
}

func publicSchemas() map[string]any {
	return map[string]any{
		"Error":                  map[string]any{"type": "object", "properties": map[string]any{"error": map[string]any{"type": "object", "properties": map[string]any{"message": map[string]any{"type": "string"}, "type": map[string]any{"type": "string"}, "code": map[string]any{}}}}},
		"HealthResponse":         map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string"}, "uptime": map[string]any{"type": "string"}, "engine_a": map[string]any{"type": "boolean"}, "engine_b": map[string]any{"type": "boolean"}}},
		"ModelList":              map[string]any{"type": "object", "properties": map[string]any{"object": map[string]any{"type": "string", "example": "list"}, "data": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Model"}}}},
		"Model":                  map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "object": map[string]any{"type": "string"}, "display_name": map[string]any{"type": "string"}, "available": map[string]any{"type": "boolean"}, "engine": map[string]any{"type": "string", "enum": []string{"a", "b"}}}},
		"AccountsResponse":       map[string]any{"type": "object", "properties": map[string]any{"object": map[string]any{"type": "string"}, "accounts": map[string]any{"type": "object", "additionalProperties": true}}},
		"ChatCompletionRequest":  map[string]any{"type": "object", "required": []string{"model", "messages"}, "properties": map[string]any{"model": map[string]any{"type": "string"}, "messages": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ChatMessage"}}, "stream": map[string]any{"type": "boolean", "default": false}, "temperature": map[string]any{"type": "number"}, "top_p": map[string]any{"type": "number"}, "max_tokens": map[string]any{"type": "integer"}, "user": map[string]any{"type": "string"}}},
		"ChatMessage":            map[string]any{"type": "object", "required": []string{"role", "content"}, "properties": map[string]any{"role": map[string]any{"type": "string", "enum": []string{"system", "user", "assistant", "tool"}}, "content": map[string]any{}}},
		"ChatCompletionResponse": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "object": map[string]any{"type": "string"}, "created": map[string]any{"type": "integer"}, "model": map[string]any{"type": "string"}, "choices": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}, "usage": map[string]any{"type": "object"}}},
		"VideoCreateRequest":     map[string]any{"type": "object", "required": []string{"model", "prompt"}, "properties": map[string]any{"model": map[string]any{"type": "string", "example": "veo-3.1-fast-generate-preview"}, "prompt": map[string]any{"type": "string"}, "seconds": map[string]any{"oneOf": []any{map[string]any{"type": "integer"}, map[string]any{"type": "string"}}, "default": 4}, "duration_seconds": map[string]any{"type": "integer"}, "aspect_ratio": map[string]any{"type": "string", "default": "16:9"}, "resolution": map[string]any{"type": "string", "default": "720p"}, "size": map[string]any{"type": "string"}}},
		"VideoResponse":          map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "object": map[string]any{"type": "string", "example": "video"}, "status": map[string]any{"type": "string", "enum": []string{"in_progress", "completed", "failed"}}, "model": map[string]any{"type": "string"}, "created_at": map[string]any{"type": "integer"}, "seconds": map[string]any{"type": "string"}, "size": map[string]any{"type": "string"}, "output": map[string]any{"type": "object", "properties": map[string]any{"file_id": map[string]any{"type": "string"}, "mime_type": map[string]any{"type": "string"}, "url": map[string]any{"type": "string"}}}}},
	}
}
