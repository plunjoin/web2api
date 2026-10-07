package gateway

// Document the public HTTP adapter's actual limits, independently of the
// capabilities available inside the AI Studio private protocol implementation.
const geminiCompatibilityDescription = `## Gemini 官方参数与当前网关的差异

对照日期：2026-10-07。新增官方 Gemini 后端，启用 gemini_api 后可使用 /v1/interactions 或 /v1beta/interactions 完整转发官方参数、响应及 SSE；本聊天接口仍使用网页登录态或 OpenAI 兼容上游。

聊天只接收 model、messages、stream、temperature、top_p、max_tokens、user。messages 必填且至少一条；model 省略或为空时使用 gemini-flash。user 虽可解析，但没有进入生成请求。其他顶层参数会被忽略。

即使配置 upstream 且 passthrough=true，聊天仍重新组装 model、messages、stream、temperature、top_p、max_tokens，不会原样转发 tools、tool_choice、response_format、generation_config、seed、stop、previous_interaction_id、background 等字段。消息级 tool_calls、tool_call_id 和 name 也未保留。只有图片、音频、文件等专用透传路径保留原始请求体。

native 模式将 system 消息拼为系统提示、assistant 作为历史回复，其余角色按用户文本处理；结构化 content 只提取 type=text。内部虽有图片配置、语音配置、转写、工具、媒体输入和思考等结构，HTTP 聊天没有接入这些官方配置对象。

Gemini 新官方协议使用 steps 与带 type 的 response_format；旧 outputs、response_mime_type、response_modalities、generation_config.image_config 不能作为新官方请求示例。内部 AI Studio 私有 RPC 的字段不等同于公开 API，不能仅根据公开字段改名判断已支持。

交互存储、查询、取消、删除、后台执行、webhook、continuation_token 或 last_event_id 恢复均使用官方 Interactions 路由，由 Google 实际执行。Veo /v1/videos 是独立长任务协议。其他官方资源（agent、环境、声音、检索配置等）也通过官方后端转发，字段与账号权限由 Google 校验。

HTTP 聊天读取上限为 8 MiB。非流式 usage 按文本字符长度估算，不是 Google 权威计费；不返回官方思考、缓存、工具、分模态统计。媒体以 Markdown 链接放入 content。SSE 使用 choices[].delta.content 和 [DONE]，不提供官方 step 事件、event_id 或 thought signature；流开始后失败可能输出 [Error] 文本。`

func addGeminiCompatibilityDocs(spec map[string]any) {
	info := spec["info"].(map[string]any)
	info["description"] = info["description"].(string) + "\n\n" + geminiCompatibilityDescription
	paths := spec["paths"].(map[string]any)
	chat := paths["/v1/chat/completions"].(map[string]any)["post"].(map[string]any)
	chat["description"] = chat["description"].(string) + "\n\n" + geminiCompatibilityDescription
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	request := schemas["ChatCompletionRequest"].(map[string]any)
	request["required"] = []string{"messages"}
	request["description"] = "OpenAI 兼容聊天请求，不是 Gemini Interactions 请求。未知顶层字段被忽略，包括 upstream 聊天模式。"
	properties := request["properties"].(map[string]any)
	properties["model"].(map[string]any)["default"] = "gemini-flash"
	properties["model"].(map[string]any)["description"] = "建议显式指定 GET /v1/models 中的实际模型 ID；省略或空字符串时使用 gemini-flash。模型可见不保证生成额度。"
	properties["messages"].(map[string]any)["minItems"] = 1
	properties["messages"].(map[string]any)["description"] = "必填且至少一条。native 仅提取文本；system 为系统提示、assistant 为历史回复，其余角色按用户文本处理。消息级工具元数据未映射。"
	properties["temperature"].(map[string]any)["description"] = "传入 native 内部配置或兼容上游；部分私有 Interaction RPC 不编码它。官方新 Interactions schema 标记同名配置为废弃。"
	properties["top_p"].(map[string]any)["description"] = "传入 native 内部配置或兼容上游；部分私有 Interaction RPC 不编码它。官方新 Interactions schema 标记同名配置为废弃。"
	properties["max_tokens"].(map[string]any)["description"] = "native 转为 max_output_tokens 并按模型范围校验；兼容上游发送同名字段。省略时使用所选协议和模型默认配置。"
	properties["user"].(map[string]any)["description"] = "可解析，但当前未用于 native 生成或发送到兼容上游；不等价于官方 labels 或会话隔离。"
	schemas["ChatCompletionResponse"].(map[string]any)["description"] = "正文为字符串，媒体包装成 Markdown 链接。usage 为字符长度估算；finish_reason 当前固定为 stop，不是官方交互状态映射。"
}
