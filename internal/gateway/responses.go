package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"web2api/internal/ir"
	"web2api/internal/model"
	"web2api/internal/provider"
	"web2api/internal/store"
)

// 本文件实现 OpenAI Responses API 的兼容子集（POST /v1/responses）。
//
// 目的：让 Codex 这类只讲 Responses 协议的客户端直连网关，不必再经
// CC Switch 之类的本地路由做协议转换——转换层是流式截断的常见来源。
//
// 已支持：input（字符串或 item 数组）、instructions、stream、
// temperature/top_p/max_output_tokens、usage、response.* 事件流，
// 以及 function calling（tools / tool_choice / function_call 输出项）。
// 未支持：previous_response_id 等有状态字段。
// local_shell / apply_patch / web_search 等非 function 工具在入口丢弃并记日志，
// 不进入任何上游报文。

// responsesRequest 是 Responses 请求体（网关支持的子集）。
type responsesRequest struct {
	Model             string               `json:"model"`
	Input             json.RawMessage      `json:"input"`
	Instructions      string               `json:"instructions"`
	Stream            bool                 `json:"stream"`
	Temperature       *float64             `json:"temperature"`
	TopP              *float64             `json:"top_p"`
	MaxOutputTokens   *int                 `json:"max_output_tokens"`
	User              string               `json:"user"`
	StreamOptions     *model.StreamOptions `json:"stream_options"`
	Tools             []responsesTool      `json:"tools"`
	ToolChoice        json.RawMessage      `json:"tool_choice"`
	ParallelToolCalls *bool                `json:"parallel_tool_calls"`
}

// responsesTool 是 Responses 格式的工具定义：name/description/parameters
// 平铺在对象上，而 Chat 格式是嵌在 function 里，转换时需要拆包。
// namespace 类型会带嵌套 tools；local_shell / web_search 等内置工具通常只有 type。
type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Tools       []responsesTool `json:"tools"` // namespace 嵌套
}

// responsesItem 是 input 数组里的单项。
type responsesItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Output    json.RawMessage `json:"output"`
	CallID    string          `json:"call_id"`
	ID        string          `json:"id"`
}

// responsesContentPart 是 content 数组里的分段。
type responsesContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL string `json:"image_url"`
}

// handleResponses 处理 POST /v1/responses。
func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取请求体失败", "invalid_request_error", nil)
		return
	}
	var req responsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error(), "invalid_request_error", nil)
		return
	}
	chatReq, err := req.toChat(s.logger.Printf)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
		return
	}
	// 统一 IR 入口：与 /v1/chat/completions 走同一套规范化不变量。
	chatReq = ir.NormalizeChat(chatReq, s.logger.Printf)
	if !s.modelAllowed(w, r, chatReq.Model) {
		return
	}

	start := time.Now()
	if req.Stream {
		s.streamResponses(w, r, chatReq, start)
		return
	}

	res, err := s.mgr.Chat(r.Context(), chatReq)
	if err != nil {
		s.logger.Printf("[responses] 请求失败 model=%q: %v", chatReq.Model, err)
		s.recordChat(r, chatReq, res, false, err, start, "/v1/responses")
		s.writeResponsesError(w, err)
		return
	}
	rec := s.recordChat(r, chatReq, res, false, nil, start, "/v1/responses")
	usage := usageFromRecord(rec)
	text := ""
	var toolCalls []model.ToolCall
	if res != nil {
		text = res.Text
		toolCalls = res.ToolCalls
	}
	output := make([]any, 0, len(toolCalls)+1)
	if text != "" || len(toolCalls) == 0 {
		output = append(output, responsesMessageItem("msg_"+model.NewID(24), text, "completed"))
	}
	for _, call := range toolCalls {
		output = append(output, responsesFunctionCallItem("fc_"+model.NewID(24), call.ID, call.Function.Name, call.Function.Arguments, "completed"))
	}
	writeJSON(w, http.StatusOK, responsesObject(
		"resp_"+model.NewID(24), chatReq.Model, "completed",
		output, responsesUsage(usage), time.Now().Unix(),
	))
}

// streamResponses 以 response.* 事件流返回结果。
//
// 输出项按「首次出现」的顺序编号：先出文本就占 output_index 0，先出工具
// 调用就占 0。这样模型只回工具调用时不会凭空多出一个空的 message 项。
func (s *Server) streamResponses(w http.ResponseWriter, r *http.Request, req model.ChatRequest, start time.Time) {
	sse := newSSEStream(w)
	if err := sse.Start(); err != nil {
		s.logger.Printf("[responses] 初始化流失败: %v", err)
		return
	}
	defer sse.Close()

	respID := "resp_" + model.NewID(24)
	createdAt := time.Now().Unix()
	seq := 0
	emit := func(event string, payload map[string]any) {
		payload["type"] = event
		payload["sequence_number"] = seq
		seq++
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		_ = sse.Write(event, data)
	}

	emit("response.created", map[string]any{
		"response": responsesObject(respID, req.Model, "in_progress", []any{}, nil, createdAt),
	})
	emit("response.in_progress", map[string]any{
		"response": responsesObject(respID, req.Model, "in_progress", []any{}, nil, createdAt),
	})

	var (
		items     []*responsesOutputItem
		textItem  *responsesOutputItem
		toolItems = map[int]*responsesOutputItem{}
	)
	openMessage := func() *responsesOutputItem {
		if textItem != nil {
			return textItem
		}
		textItem = &responsesOutputItem{
			index: len(items),
			id:    "msg_" + model.NewID(24),
			kind:  "message",
		}
		items = append(items, textItem)
		emit("response.output_item.added", map[string]any{
			"output_index": textItem.index,
			"item": map[string]any{
				"id": textItem.id, "type": "message", "status": "in_progress",
				"role": "assistant", "content": []any{},
			},
		})
		emit("response.content_part.added", map[string]any{
			"item_id": textItem.id, "output_index": textItem.index, "content_index": 0,
			"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
		})
		return textItem
	}
	createToolItem := func(index int, callID, name string) *responsesOutputItem {
		item := &responsesOutputItem{
			index:  len(items),
			id:     "fc_" + model.NewID(24),
			kind:   "function_call",
			callID: callID,
			name:   name,
		}
		if item.callID == "" {
			item.callID = "call_" + model.NewID(24)
		}
		toolItems[index] = item
		items = append(items, item)
		emit("response.output_item.added", map[string]any{
			"output_index": item.index,
			"item": map[string]any{
				"id": item.id, "type": "function_call", "status": "in_progress",
				"call_id": item.callID, "name": item.name, "arguments": "",
			},
		})
		return item
	}

	sink := &chatSink{
		text: func(delta string) error {
			item := openMessage()
			item.text.WriteString(delta)
			emit("response.output_text.delta", map[string]any{
				"item_id": item.id, "output_index": item.index, "content_index": 0,
				"delta": delta, "logprobs": []any{},
			})
			// 写失败说明客户端已断开，返回错误让上游停止生成。
			return sse.Err()
		},
		tool: func(delta model.ToolCallDelta) error {
			item := toolItems[delta.Index]
			if item == nil {
				// 首块就把函数名带上，output_item.added 里才有 name——
				// 客户端可能只读这个事件来确定要调哪个函数。
				item = createToolItem(delta.Index, delta.ID, delta.Function.Name)
			} else if delta.Function.Name != "" {
				item.name += delta.Function.Name
			}
			if delta.Function.Arguments != "" {
				item.args.WriteString(delta.Function.Arguments)
				emit("response.function_call_arguments.delta", map[string]any{
					"item_id": item.id, "output_index": item.index,
					"delta": delta.Function.Arguments,
				})
			}
			return sse.Err()
		},
	}

	res, runErr := s.mgr.ChatStream(r.Context(), req, sink)
	if runErr != nil {
		s.logger.Printf("[responses] 流式请求失败 model=%q: %v", req.Model, runErr)
		s.recordChat(r, req, res, true, runErr, start, "/v1/responses")
		failed := responsesObject(respID, req.Model, "failed", []any{}, nil, createdAt)
		failed["error"] = map[string]any{"code": "server_error", "message": runErr.Error()}
		emit("response.failed", map[string]any{"response": failed})
		return
	}

	rec := s.recordChat(r, req, res, true, nil, start, "/v1/responses")
	usage := usageFromRecord(rec)
	// 一个字都没吐（例如上游只给了用量）：补一个空消息项，
	// 保证 response.output 不为空，客户端不会当成异常响应。
	if len(items) == 0 {
		openMessage()
	}
	// 引擎在流结束后汇总出的工具调用优先于增量拼接结果：
	// 部分上游不发增量，只在结束分片上一次性给出完整 tool_calls。
	if res != nil && len(res.ToolCalls) > 0 {
		for index, call := range res.ToolCalls {
			item := toolItems[index]
			if item == nil {
				item = createToolItem(index, call.ID, call.Function.Name)
			}
			if call.ID != "" {
				item.callID = call.ID
			}
			if call.Function.Name != "" {
				item.name = call.Function.Name
			}
			if call.Function.Arguments != "" && item.args.Len() == 0 {
				item.args.WriteString(call.Function.Arguments)
			}
		}
	}

	output := make([]any, 0, len(items))
	for _, item := range items {
		switch item.kind {
		case "message":
			text := item.text.String()
			emit("response.output_text.done", map[string]any{
				"item_id": item.id, "output_index": item.index, "content_index": 0, "text": text,
			})
			emit("response.content_part.done", map[string]any{
				"item_id": item.id, "output_index": item.index, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": text, "annotations": []any{}},
			})
			msg := responsesMessageItem(item.id, text, "completed")
			emit("response.output_item.done", map[string]any{"output_index": item.index, "item": msg})
			output = append(output, msg)
		case "function_call":
			args := item.args.String()
			if args == "" {
				args = "{}"
			}
			emit("response.function_call_arguments.done", map[string]any{
				"item_id": item.id, "output_index": item.index, "arguments": args,
			})
			call := responsesFunctionCallItem(item.id, item.callID, item.name, args, "completed")
			emit("response.output_item.done", map[string]any{"output_index": item.index, "item": call})
			output = append(output, call)
		}
	}
	// response.completed 是 Codex 判定「这一轮正常结束」的依据，
	// 缺了它客户端会把输出当成被截断。
	emit("response.completed", map[string]any{
		"response": responsesObject(respID, req.Model, "completed", output, responsesUsage(usage), createdAt),
	})
}

// responsesOutputItem 是流式输出中正在累积的一项。
type responsesOutputItem struct {
	index  int
	id     string
	kind   string // message / function_call
	text   strings.Builder
	callID string
	name   string
	args   strings.Builder
}

// writeResponsesError 复用 chat 的错误语义：模型无人声明 404、
// 端点与模型用途不匹配 400，其余 502。
func (s *Server) writeResponsesError(w http.ResponseWriter, err error) {
	if provider.IsModelNotFound(err) {
		writeError(w, http.StatusNotFound, err.Error(), "invalid_request_error", "model_not_found")
		return
	}
	if provider.IsUnsupportedEndpoint(err) {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "unsupported_endpoint")
		return
	}
	writeError(w, http.StatusBadGateway, err.Error(), "api_error", nil)
}

// toChat 把 Responses 请求转换成内部 Chat 请求（近 IR；不明工具类型的
// 丢弃与日志在此处理，其余不变量由 ir.NormalizeChat 统一兜底）。
func (req responsesRequest) toChat(logf ir.Logf) (model.ChatRequest, error) {
	out := model.ChatRequest{
		Model:             strings.TrimSpace(req.Model),
		Stream:            req.Stream,
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		MaxTokens:         req.MaxOutputTokens,
		User:              req.User,
		StreamOptions:     req.StreamOptions,
		Tools:             req.toChatTools(logf),
		ToolChoice:        toChatToolChoice(req.ToolChoice),
		ParallelToolCalls: req.ParallelToolCalls,
	}
	if out.Model == "" {
		out.Model = "gemini-flash"
	}
	if s := strings.TrimSpace(req.Instructions); s != "" {
		out.Messages = append(out.Messages, model.ChatMessage{Role: "system", Content: s})
	}
	messages, err := parseResponsesInput(req.Input)
	if err != nil {
		return out, err
	}
	out.Messages = append(out.Messages, messages...)
	if len(out.Messages) == 0 {
		return out, fmt.Errorf("input is required")
	}
	return out, nil
}

// toChatTools 把 Responses 格式的工具定义转成 Chat（IR）格式。
//
// 只保留 type=function（namespace 嵌套拍平为独立 function）。local_shell /
// apply_patch / web_search 等内置工具在入口丢弃并记日志：它们依赖 OpenAI
// 服务端或客户端专用执行环境，任何上游报文里都不该出现（计划 §5.2）；
// 降级成同名 function 只会把「模型发起了调用但客户端执行不了」的困惑
// 从网关挪到上游。Codex 0.16x 起的 shell 工具本就是 type=function
// （exec_command 等），不受影响。
func (req responsesRequest) toChatTools(logf ir.Logf) []model.Tool {
	out := make([]model.Tool, 0, len(req.Tools))
	var dropped []string
	drop := func(typ string) {
		if typ != "" {
			dropped = append(dropped, typ)
		}
	}
	var appendTool func(tool responsesTool)
	appendTool = func(tool responsesTool) {
		typ := strings.ToLower(strings.TrimSpace(tool.Type))
		switch typ {
		case "", "function":
			name := strings.TrimSpace(tool.Name)
			if name == "" {
				drop("function(name缺失)")
				return
			}
			params := tool.Parameters
			if len(params) == 0 || string(params) == "null" {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			out = append(out, model.Tool{
				Type: "function",
				Function: &model.ToolFunc{
					Name:        name,
					Description: tool.Description,
					Parameters:  params,
				},
			})
		case "namespace":
			for _, nested := range tool.Tools {
				appendTool(nested)
			}
		default:
			// local_shell / web_search 等内置工具：入口丢弃。
			drop(tool.Type)
		}
	}
	for _, tool := range req.Tools {
		appendTool(tool)
	}
	if len(dropped) > 0 && logf != nil {
		logf("[responses] 丢弃非 function 工具（不进入上游报文）: %s", strings.Join(dropped, ", "))
	}
	return out
}

// toChatToolChoice 把 Responses 的 tool_choice 转成 Chat 形式。
// 两者字符串形式一致；指定函数时结构不同：
// Responses 是 {type:function,name}，Chat 是 {type:function,function:{name}}。
func toChatToolChoice(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		return raw
	}
	var choice struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Function *struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &choice); err != nil {
		return raw
	}
	if choice.Type != "" && choice.Type != "function" {
		return raw
	}
	name := choice.Name
	if name == "" && choice.Function != nil {
		name = choice.Function.Name
	}
	if name == "" {
		return raw
	}
	converted, err := json.Marshal(map[string]any{
		"type":     "function",
		"function": map[string]string{"name": name},
	})
	if err != nil {
		return raw
	}
	return converted
}

// parseResponsesInput 解析 input：字符串，或 message / function_call 等 item 数组。
func parseResponsesInput(raw json.RawMessage) ([]model.ChatMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("input 解析失败: %w", err)
		}
		return []model.ChatMessage{{Role: "user", Content: text}}, nil
	}

	var items []responsesItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("input 解析失败: %w", err)
	}
	out := make([]model.ChatMessage, 0, len(items))
	// callID → 函数名：Responses 的 function_call_output 通常只有 call_id，
	// 不含 name；MiniMax / Gemini functionResponse / AI Studio 都需要 name。
	callNames := map[string]string{}
	for _, item := range items {
		switch item.Type {
		case "", "message":
			role := item.Role
			if role == "" {
				role = "user"
			}
			// Responses 用 developer 表示系统级指令，Chat 只有 system。
			if role == "developer" {
				role = "system"
			}
			text, images := parseResponsesContent(item.Content)
			if text == "" && len(images) == 0 {
				continue
			}
			if len(images) > 0 {
				parts := make([]model.ContentPart, 0, len(images)+1)
				if text != "" {
					parts = append(parts, model.ContentPart{Type: "text", Text: text})
				}
				parts = append(parts, images...)
				out = append(out, model.ChatMessage{Role: role, Content: parts})
				continue
			}
			out = append(out, model.ChatMessage{Role: role, Content: text})
		case "function_call":
			// 历史工具调用要还原成 assistant.tool_calls：降级成纯文本会让
			// 模型看不到调用与结果的对应关系，多轮工具链直接断掉。
			callID := item.CallID
			if callID == "" {
				callID = item.ID
			}
			if callID == "" {
				callID = "call_" + model.NewID(16)
			}
			name := strings.TrimSpace(item.Name)
			if name != "" {
				callNames[callID] = name
			}
			call := model.ToolCall{
				ID:   callID,
				Type: "function",
				Function: model.ToolFunction{
					Name:      name,
					Arguments: rawJSONString(item.Arguments),
				},
			}
			// 连续的 function_call 属于同一个 assistant 回合，归并成一条消息。
			// Content 必须是 "" 而不是 nil：encoding/json 会把 nil any 编成
			// null，MiniMax 等第三方上游对 assistant.content=null 直接 400。
			if n := len(out); n > 0 && out[n-1].Role == "assistant" && out[n-1].ToolCallID == "" {
				out[n-1].ToolCalls = append(out[n-1].ToolCalls, call)
				continue
			}
			out = append(out, model.ChatMessage{Role: "assistant", Content: "", ToolCalls: []model.ToolCall{call}})
		case "function_call_output":
			callID := strings.TrimSpace(item.CallID)
			name := strings.TrimSpace(item.Name)
			if name == "" && callID != "" {
				name = callNames[callID]
			}
			out = append(out, model.ChatMessage{
				Role:       "tool",
				Name:       name,
				ToolCallID: callID,
				Content:    rawText(item.Output),
			})
		case "reasoning":
			// 推理项不进入上下文。
		default:
			if text := rawText(item.Content); text != "" {
				out = append(out, model.ChatMessage{Role: "user", Content: text})
			}
		}
	}
	return out, nil
}

// parseResponsesContent 解析 content：字符串，或 [{type:"input_text",text:...}] 数组。
func parseResponsesContent(raw json.RawMessage) (string, []model.ContentPart) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", nil
		}
		return text, nil
	}
	var parts []responsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", nil
	}
	var sb strings.Builder
	var images []model.ContentPart
	for _, part := range parts {
		switch part.Type {
		case "input_image", "image_url":
			if part.ImageURL != "" {
				images = append(images, model.NewImagePart(part.ImageURL))
			}
		default:
			sb.WriteString(part.Text)
		}
	}
	return sb.String(), images
}

// rawText 尽量从任意 JSON 里取出文本（字符串直接返回，对象取 text/output）。
func rawText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if strings.HasPrefix(trimmed, "\"") {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return text
		}
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range []string{"text", "output", "content"} {
			if v, ok := obj[key].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// rawJSONString 取出「JSON 字符串字面量」的内容（function_call 的
// arguments 在 Responses 协议里是字符串，里面再套一层 JSON）。
// 如果上游直接给了对象，就原样返回其 JSON 文本。
func rawJSONString(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if strings.HasPrefix(trimmed, "\"") {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return text
		}
		return ""
	}
	return trimmed
}

// usageFromRecord 把落库的用量记录转成协议用量。
func usageFromRecord(rec store.UsageRecord) model.TokenUsage {
	return model.TokenUsage{
		PromptTokens:     rec.PromptTokens,
		CompletionTokens: rec.CompletionTokens,
		TotalTokens:      rec.TotalTokens,
		Estimated:        rec.Estimated,
	}.Normalize()
}

// responsesUsage 输出 Responses 协议的 usage 结构。
func responsesUsage(usage model.TokenUsage) map[string]any {
	return map[string]any{
		"input_tokens":          usage.PromptTokens,
		"output_tokens":         usage.CompletionTokens,
		"total_tokens":          usage.TotalTokens,
		"input_tokens_details":  map[string]any{"cached_tokens": 0},
		"output_tokens_details": map[string]any{"reasoning_tokens": usage.ReasoningTokens},
	}
}

// responsesMessageItem 构造 assistant 文本消息项。
func responsesMessageItem(id, text, status string) map[string]any {
	return map[string]any{
		"id": id, "type": "message", "status": status, "role": "assistant",
		"content": []any{map[string]any{
			"type": "output_text", "text": text, "annotations": []any{},
		}},
	}
}

// responsesFunctionCallItem 构造 function_call 输出项。
// call_id 是客户端回填 function_call_output 时使用的关联键，必须与
// 模型请求的调用 ID 一致。
func responsesFunctionCallItem(id, callID, name, arguments, status string) map[string]any {
	if arguments == "" {
		arguments = "{}"
	}
	return map[string]any{
		"id": id, "type": "function_call", "status": status,
		"call_id": callID, "name": name, "arguments": arguments,
	}
}

// responsesObject 构造 Responses 响应对象。createdAt 由调用方固定，
// 保证同一次请求的各条事件里 created_at 一致。
func responsesObject(id, modelName, status string, output []any, usage map[string]any, createdAt int64) map[string]any {
	obj := map[string]any{
		"id":                  id,
		"object":              "response",
		"created_at":          createdAt,
		"status":              status,
		"model":               modelName,
		"output":              output,
		"parallel_tool_calls": true,
		"tool_choice":         "auto",
		"tools":               []any{},
		"error":               nil,
		"incomplete_details":  nil,
	}
	if usage != nil {
		obj["usage"] = usage
	}
	return obj
}
