package gateway

// 多模态文档以实际网关适配能力为准；透传示例需上游支持对应协议。
const mediaAPIOverview = `所有生成请求使用 Authorization: Bearer sk-...。先 GET /v1/models 获取实际可用的模型 ID；示例模型只有出现在当前目录并具备生成额度时才能使用。

## 生成接口怎么选

| 用途 | 引擎B native（默认） | 引擎B upstream 且 passthrough=true |
| --- | --- | --- |
| 图片生成 | POST /v1/chat/completions，选择图片模型 | POST /v1/images/generations，需上游实现该接口 |
| 音频生成 | POST /v1/chat/completions，选择 TTS/音频模型 | POST /v1/audio/speech，需上游实现该接口 |
| Veo 视频 | POST /v1/videos → GET /v1/videos/{id} → GET /v1/videos/{id}/content | 这三条标准视频路由仍要求 native 模式，不能当作上游视频透传使用 |

native 图片和音频生成返回聊天 JSON，媒体位于 choices[0].message.content 中的 Markdown 链接，通常是 data:MIME;base64,...。图片/音频专用透传接口直接保留上游的响应结构。

/v1/docs 返回可导入 Postman、Insomnia 或 Swagger UI 的 OpenAPI 3.1 JSON，无需 API Key。下面每个生成接口均提供请求字段、示例及结果读取说明。`

const nativeMediaDescription = `## 图片生成（引擎B native）

选择 GET /v1/models 中的图片模型，例如 gemini-3.1-flash-image，在 messages 中描述图片。建议先使用 stream=false。

` + "```bash\n" + `curl http://localhost:8800/v1/chat/completions \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemini-3.1-flash-image","messages":[{"role":"user","content":"生成一张被浓雾笼罩的森林图片，气氛阴森"}],"stream":false}'
` + "```\n" + `
结果在 choices[0].message.content，例如 ![media](data:image/png;base64,...)。提取括号内的 data URI，按逗号分隔 MIME 信息和 Base64 数据，解码后保存图片；也可能返回媒体 URL。

## 音频生成（引擎B native）

选择实际目录中的 TTS/音频模型，例如 gemini-2.5-flash-preview-tts。底层会根据模型目录请求 AUDIO 输出；具体模型是否接受该请求仍由上游决定。

` + "```bash\n" + `curl http://localhost:8800/v1/chat/completions \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemini-2.5-flash-preview-tts","messages":[{"role":"user","content":"请用平静的中文声音朗读：欢迎来到雾中的森林。"}],"stream":false}'
` + "```\n" + `
结果仍在 choices[0].message.content，例如 ![media](data:audio/pcm;rate=24000;base64,...)，实际 MIME 由上游返回。Base64 解码不会自动转换格式；原始 PCM 需要按返回的采样率等信息封装或转换后播放，不能仅改名为 mp3。

## 当前聊天适配参数

该接口支持 model、messages、stream、temperature、top_p、max_tokens、user。原生图片/音频输出模态由模型目录自动选择。当前 HTTP 聊天适配器未映射 image_config、speech_config、response_modalities、voice、size、n 或 response_format；传入这些额外字段不会设置生成参数。原生消息适配只提取文本，不能用 image_url 或 input_audio 完成参考图编辑、音频输入。

stream=true 时媒体链接也放在 choices[0].delta.content 中，需拼接分片后再提取完整链接。普通聊天仍使用同一接口；失败状态码见 responses。`

func addMediaAPIDocs(spec map[string]any) {
	paths := spec["paths"].(map[string]any)
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	chat := paths["/v1/chat/completions"].(map[string]any)["post"].(map[string]any)
	chat["tags"] = []string{"聊天", "图片", "音频"}
	chat["summary"] = "聊天补全 / 原生图片和音频生成"
	chat["description"] = nativeMediaDescription
	chat["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["examples"] = map[string]any{
		"chat":  mediaExample("文本聊天", nativeMediaRequest("gemini-2.5-flash", "什么是号池？")),
		"image": mediaExample("native 图片生成", nativeMediaRequest("gemini-3.1-flash-image", "生成一张被浓雾笼罩的森林图片，气氛阴森")),
		"audio": mediaExample("native 音频生成", nativeMediaRequest("gemini-2.5-flash-preview-tts", "请用平静的中文声音朗读：欢迎来到雾中的森林。")),
	}
	chatResponse := chat["responses"].(map[string]any)["200"].(map[string]any)
	chatResponse["description"] = "stream=false 返回聊天 JSON；媒体在 choices[0].message.content。stream=true 返回 SSE，增量在 choices[0].delta.content，最后 data: [DONE]。"
	chatResponse["content"].(map[string]any)["application/json"].(map[string]any)["examples"] = map[string]any{
		"image": mediaExample("图片输出结构（Base64 已省略）", mediaChatResponse("gemini-3.1-flash-image", "![media](data:image/png;base64,<图片Base64>)")),
		"audio": mediaExample("音频输出结构（Base64 已省略）", mediaChatResponse("gemini-2.5-flash-preview-tts", "![media](data:audio/pcm;rate=24000;base64,<音频Base64>)")),
	}
	chatResponse["content"].(map[string]any)["text/event-stream"] = map[string]any{"schema": map[string]any{"type": "string"}, "example": "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\ndata: [DONE]\n\n"}
	chatProperties := schemas["ChatCompletionRequest"].(map[string]any)["properties"].(map[string]any)
	chatProperties["model"].(map[string]any)["description"] = "GET /v1/models 中的模型 ID。原生图片选择图片模型，音频选择 TTS/音频模型；模型可见不代表有剩余额度。"
	chatProperties["messages"].(map[string]any)["description"] = "至少一条消息。原生媒体生成用 user 角色的文本描述图片、朗读文本或音乐；当前原生适配只提取文本。"
	chatProperties["stream"].(map[string]any)["description"] = "false 返回完整 JSON，适合先验证媒体输出；true 返回 SSE，需拼接 delta.content。"
	schemas["ChatMessage"].(map[string]any)["properties"].(map[string]any)["content"] = map[string]any{
		"description": "推荐使用字符串。结构化内容中，当前 native 模式只提取 type=text 的文本段。",
		"oneOf":       []any{map[string]any{"type": "string"}, map[string]any{"type": "array", "items": map[string]any{"type": "object"}}},
	}
	addVideoAPIDocs(paths, schemas)
	addMediaPassthroughDocs(paths, schemas)
}

func nativeMediaRequest(modelID, prompt string) map[string]any {
	return map[string]any{"model": modelID, "messages": []any{map[string]any{"role": "user", "content": prompt}}, "stream": false}
}

func mediaExample(summary string, value any) map[string]any {
	return map[string]any{"summary": summary, "value": value}
}

func mediaChatResponse(modelID, content string) map[string]any {
	return map[string]any{
		"id": "chatcmpl-example", "object": "chat.completion", "created": 1760000000, "model": modelID,
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
	}
}

func addVideoAPIDocs(paths, schemas map[string]any) {
	create := paths["/v1/videos"].(map[string]any)["post"].(map[string]any)
	create["description"] = `仅引擎B native 模式可用。model 和 prompt 必填，创建成功返回 202 JSON 和任务 id，此时还没有视频文件。使用返回的真实 id 轮询 GET /v1/videos/{id}，status=completed 后请求 GET /v1/videos/{id}/content 下载 MP4；status=failed 则终止轮询。

` + "```bash\n" + `curl http://localhost:8800/v1/videos \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{"model":"veo-3.1-fast-generate-preview","prompt":"被浓雾笼罩的森林，体现阴森的感觉","aspect_ratio":"16:9","seconds":8,"resolution":"1080p"}'
` + "```\n" + `
时长、比例和分辨率由上游实时模型目录校验；示例参数需该模型支持。HTTP 429 / 协议码 8 表示上游限流或配额问题，需检查对应账户资格、计费和用量，降低分辨率不能保证恢复。

当前创建接口只接受文本提示词，每次一个结果；参考图、首尾帧、负向提示词等扩展参数尚未映射。`
	create["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["examples"] = map[string]any{
		"forest":   mediaExample("8 秒 / 1080p / 横屏", map[string]any{"model": "veo-3.1-fast-generate-preview", "prompt": "被浓雾笼罩的森林，体现阴森的感觉", "aspect_ratio": "16:9", "seconds": 8, "resolution": "1080p"}),
		"defaults": mediaExample("使用默认参数（需模型支持）", map[string]any{"model": "veo-3.1-fast-generate-preview", "prompt": "一只橘猫在雨后的街头奔跑"}),
	}
	created := map[string]any{"id": "video_abc123", "object": "video", "status": "in_progress", "model": "veo-3.1-fast-generate-preview", "created_at": 1760000000, "seconds": "8", "size": "1920x1080"}
	create["responses"].(map[string]any)["202"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["example"] = created
	create["responses"].(map[string]any)["429"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["example"] = map[string]any{"error": map[string]any{"message": "AI Studio GenerateVideo 返回 HTTP 429、协议错误码 8: You exceeded your current quota", "type": "rate_limit_error", "code": "rate_limit_exceeded"}}
	poll := paths["/v1/videos/{id}"].(map[string]any)["get"].(map[string]any)
	poll["description"] = "用创建响应中的 id 查询（示例 video_abc123 是占位符）。in_progress：等待数秒后继续查询；completed：读取 output.url 或下载 /v1/videos/{id}/content；failed：生成结束但没有输出文件，停止轮询。当前 failed 响应不附带上游详细失败原因。\n\n```bash\ncurl http://localhost:8800/v1/videos/video_abc123 -H 'Authorization: Bearer sk-xxxxxxxx'\n```"
	completed := map[string]any{"id": "video_abc123", "object": "video", "status": "completed", "model": "veo-3.1-fast-generate-preview", "created_at": 1760000000, "seconds": "8", "size": "1920x1080", "output": map[string]any{"file_id": "file_example", "mime_type": "video/mp4", "url": "/v1/videos/video_abc123/content"}}
	poll["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["examples"] = map[string]any{"inProgress": mediaExample("生成中", created), "completed": mediaExample("可下载", completed), "failed": mediaExample("生成结束但无文件", map[string]any{"id": "video_abc123", "object": "video", "status": "failed"})}
	download := paths["/v1/videos/{id}/content"].(map[string]any)["get"].(map[string]any)
	download["description"] = "仅 status=completed 且存在输出文件时返回视频二进制。将响应保存到文件；未完成或没有输出文件时返回 409 JSON。\n\n```bash\ncurl http://localhost:8800/v1/videos/video_abc123/content -H 'Authorization: Bearer sk-xxxxxxxx' -o output.mp4\n```"
	props := schemas["VideoCreateRequest"].(map[string]any)["properties"].(map[string]any)
	descriptions := map[string]string{
		"model":            "实时目录中支持视频长任务的 Veo 模型 ID；需要账户具备生成资格和可用额度。",
		"prompt":           "视频内容描述，必填且不能全为空白；当前只支持文本提示词。",
		"seconds":          "时长（秒），可用整数或整数文本，如 8 或 \"8\"。默认 4；常见 4/6/8，实际以模型目录为准。",
		"duration_seconds": "seconds 的整数别名；非零时优先于 seconds，不要同时指定两者。",
		"aspect_ratio":     "视频比例，默认 16:9；常见 16:9、9:16，实际以模型目录为准。",
		"resolution":       "实际生成分辨率，默认 720p；常见 720p、1080p、4k，支持范围和时长组合由上游决定。",
		"size":             "仅设置返回任务的尺寸标签，不控制生成分辨率。通常省略，服务会按 resolution 和 aspect_ratio 推导，如 1920x1080。",
	}
	for name, description := range descriptions {
		props[name].(map[string]any)["description"] = description
	}
	props["prompt"].(map[string]any)["example"] = "被浓雾笼罩的森林，体现阴森的感觉"
	videoProps := schemas["VideoResponse"].(map[string]any)["properties"].(map[string]any)
	videoProps["id"].(map[string]any)["description"] = "上游任务 ID，查询和下载必须使用创建时返回的原值。"
	videoProps["seconds"].(map[string]any)["description"] = "时长以字符串返回，例如 \"8\"。"
	videoProps["status"].(map[string]any)["description"] = "in_progress 继续轮询；completed 下载视频；failed 停止轮询。"
	videoProps["output"].(map[string]any)["description"] = "有输出文件时出现；url 为本服务的下载路径，下载仍需 API Key。"
}

func addMediaPassthroughDocs(paths, schemas map[string]any) {
	schemas["ImageGenerationRequest"] = map[string]any{"type": "object", "required": []string{"model", "prompt"}, "additionalProperties": true, "properties": map[string]any{
		"model":           map[string]any{"type": "string", "description": "配置的上游所支持的图片模型 ID；替换示例占位值。"},
		"prompt":          map[string]any{"type": "string", "description": "图片内容描述。"},
		"n":               map[string]any{"type": "integer", "example": 1, "description": "生成数量；支持范围和默认值由上游定义。"},
		"size":            map[string]any{"type": "string", "example": "1024x1024", "description": "图片尺寸；可用值由上游定义。"},
		"response_format": map[string]any{"type": "string", "example": "b64_json", "description": "常见 url 或 b64_json；仅在上游支持时有效。"},
	}}
	schemas["ImageGenerationResponse"] = map[string]any{"type": "object", "additionalProperties": true, "properties": map[string]any{
		"created": map[string]any{"type": "integer"},
		"data":    map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": true, "properties": map[string]any{"url": map[string]any{"type": "string", "description": "上游返回的图片 URL。"}, "b64_json": map[string]any{"type": "string", "description": "Base64 图片数据，解码后保存；格式以实际上游响应为准。"}}}},
	}}
	schemas["AudioSpeechRequest"] = map[string]any{"type": "object", "required": []string{"model", "input", "voice"}, "additionalProperties": true, "properties": map[string]any{
		"model":           map[string]any{"type": "string", "description": "配置的上游所支持的语音模型 ID；替换示例占位值。"},
		"input":           map[string]any{"type": "string", "description": "需要朗读的文本。"},
		"voice":           map[string]any{"type": "string", "description": "上游支持的声音名称；不同上游值不同，替换示例占位值。"},
		"response_format": map[string]any{"type": "string", "example": "mp3", "description": "输出格式，如 mp3/wav/pcm；支持范围和默认值由上游定义。"},
		"speed":           map[string]any{"type": "number", "example": 1, "description": "语速；是否支持及范围由上游定义。"},
	}}
	image := mediaPassthroughOperation("图片", "图片生成（upstream 专用接口）", "ImageGenerationRequest")
	image["description"] = `前提：engine_b.enabled=true、mode=upstream、passthrough=true，且 base_url 对应的上游实现 /v1/images/generations。native 模式调用此路径返回 502；原生图片生成请使用 /v1/chat/completions。

以下为常见 OpenAI 兼容协议示例，替换模型占位值，确认上游支持 size、n 和 response_format 后使用。网关原样转发请求体，不验证或转换这些字段，也不补全默认值。

` + "```bash\n" + `curl http://localhost:8800/v1/images/generations \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{"model":"<上游图片模型ID>","prompt":"被浓雾笼罩的森林","n":1,"size":"1024x1024","response_format":"b64_json"}'
` + "```\n" + `
响应沿用上游结构；常见为 data[].url 或 data[].b64_json，后者需要 Base64 解码后保存图片。上游可能返回不同格式或错误。`
	image["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["example"] = map[string]any{"model": "<上游图片模型ID>", "prompt": "被浓雾笼罩的森林", "n": 1, "size": "1024x1024", "response_format": "b64_json"}
	image["responses"].(map[string]any)["200"] = map[string]any{"description": "上游图片结果（下方为常见 OpenAI 兼容结构）", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/ImageGenerationResponse"}, "examples": map[string]any{"base64": mediaExample("Base64 图片（内容已省略）", map[string]any{"created": 1760000000, "data": []any{map[string]any{"b64_json": "<图片Base64>"}}}), "url": mediaExample("图片 URL", map[string]any{"created": 1760000000, "data": []any{map[string]any{"url": "https://example.com/image.png"}}})}}}}
	paths["/v1/images/generations"] = map[string]any{"post": image}
	audio := mediaPassthroughOperation("音频", "语音生成（upstream 专用接口）", "AudioSpeechRequest")
	audio["description"] = `前提：engine_b.enabled=true、mode=upstream、passthrough=true，且 base_url 对应的上游实现 /v1/audio/speech。native 模式调用此路径返回 502；原生音频生成请使用 /v1/chat/completions。

这是文本转语音示例。替换模型和声音占位值，确认上游支持输出格式后使用；音乐生成或实时语音需上游另有对应协议。网关原样转发请求体，参数范围和默认值由上游定义。

` + "```bash\n" + `curl http://localhost:8800/v1/audio/speech \
  -H 'Authorization: Bearer sk-xxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{"model":"<上游语音模型ID>","input":"欢迎来到雾中的森林。","voice":"<上游声音名称>","response_format":"mp3"}' \
  -o speech.mp3
` + "```\n" + `
成功时通常返回音频二进制，直接保存即可；失败时可能返回 JSON 错误，保存后需检查 HTTP 状态和 Content-Type。与原生聊天接口中的 Markdown data URI 输出不同。`
	audio["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["example"] = map[string]any{"model": "<上游语音模型ID>", "input": "欢迎来到雾中的森林。", "voice": "<上游声音名称>", "response_format": "mp3"}
	audio["responses"].(map[string]any)["200"] = map[string]any{"description": "上游音频二进制，格式由上游和 response_format 决定", "content": map[string]any{"audio/mpeg": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}, "audio/wav": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}, "application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}}
	paths["/v1/audio/speech"] = map[string]any{"post": audio}
	// OpenAPI 模板路径参数需显式声明；泛型路径仍供其他上游端点参考。
	for _, path := range []string{"/v1/images/{path}", "/v1/audio/{path}", "/v1/files/{path}", "/v1/embeddings/{path}"} {
		item := paths[path].(map[string]any)
		item["parameters"] = []any{map[string]any{"name": "path", "in": "path", "required": true, "description": "上游端点后缀，字段、方法和响应以对应上游文档为准。", "schema": map[string]any{"type": "string"}}}
		item["post"].(map[string]any)["description"] = "仅 engine_b.mode=upstream 且 passthrough=true 可用，需上游实现对应端点。图片生成见 /v1/images/generations，语音生成见 /v1/audio/speech；native 模式媒体生成见 /v1/chat/completions。"
	}
}

func mediaPassthroughOperation(tag, summary, schema string) map[string]any {
	return map[string]any{
		"tags": []string{tag}, "summary": summary,
		"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + schema}}}},
		"responses": map[string]any{
			"401":     map[string]any{"description": "客户端 API Key 无效或上游认证失败"},
			"429":     map[string]any{"description": "网关限流或上游配额限制；上游错误正文原样返回"},
			"502":     map[string]any{"description": "未启用 upstream 透传；其他上游状态码和响应沿用上游协议"},
			"default": map[string]any{"description": "其他上游响应，状态码、Content-Type 和正文原样返回"},
		},
	}
}
