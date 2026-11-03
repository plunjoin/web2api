// Package responses 实现 OpenAI Responses API（/v1/responses）协议的入站解码与
// 出站编码。
//
// 让 Codex 这类只讲 Responses 协议的客户端直连网关，不必再经 CC Switch 之类的
// 本地路由做协议转换——转换层是流式截断的常见来源。解码后的 ChatRequest 由
// 调用方统一走 ir.NormalizeChat 规范化；编码层把内部结果翻译成 response.* 事件流。
//
// 已支持：input（字符串或 item 数组）、instructions、stream、temperature/top_p/
// max_output_tokens、usage、response.* 事件流，以及 function calling（tools /
// tool_choice / function_call 输出项）。未支持：previous_response_id 等有状态字段。
// local_shell / apply_patch / web_search 等非 function 工具在入口丢弃并记日志，
// 不进入任何上游报文。
package responses

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"web2api/internal/ir"
	"web2api/internal/model"
)

// StreamWriter 是 SSE 输出目标（gateway 的 *sseStream 满足）。
type StreamWriter interface {
	Write(event string, data []byte) error
	Err() error
}

// ---- 请求类型 ----

// Request 是 Responses 请求体（网关支持的子集）。
type Request struct {
	Model             string          `json:"model"`
	Input             json.RawMessage `json:"input"`
	Instructions      string          `json:"instructions"`
	Stream            bool            `json:"stream"`
	Temperature       *float64        `json:"temperature"`
	TopP              *float64        `json:"top_p"`
	MaxOutputTokens   *int            `json:"max_output_tokens"`
	User              string          `json:"user"`
	StreamOptions     *model.StreamOptions `json:"stream_options"`
	Tools             []Tool          `json:"tools"`
	ToolChoice        json.RawMessage `json:"tool_choice"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls"`
}

// Tool 是 Responses 格式的工具定义：name/description/parameters 平铺在对象上，
// 而 Chat 格式是嵌在 function 里，转换时需要拆包。namespace 类型会带嵌套 tools；
// local_shell / web_search 等内置工具通常只有 type。
type Tool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Tools       []Tool          `json:"tools"` // namespace 嵌套
}

// Item 是 input 数组里的单项。
type Item struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Output    json.RawMessage `json:"output"`
	CallID    string          `json:"call_id"`
	ID        string          `json:"id"`
}

// ContentPart 是 content 数组里的分段。
type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL string `json:"image_url"`
}

// Decode 把 Responses 请求体解析为内部 ChatRequest（尚未 IR 规范化）。
// 协议级默认值（model 缺省为 gemini-flash）在此处理；input 解析失败返回错误。
// 不明工具类型（local_shell / web_search 等）在此丢弃并记日志，不进入上游报文；
// 其余不变量由调用方走 ir.NormalizeChat 兜底。
func Decode(body []byte, logf ir.Logf) (model.ChatRequest, error) {
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		return model.ChatRequest{}, fmt.Errorf("Invalid JSON body: %w", err)
	}
	out := model.ChatRequest{
		Model:             strings.TrimSpace(req.Model),
		Stream:            req.Stream,
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		MaxTokens:         req.MaxOutputTokens,
		User:              req.User,
		StreamOptions:     req.StreamOptions,
		Tools:             toChatTools(req.Tools, logf),
		ToolChoice:        toChatToolChoice(req.ToolChoice),
		ParallelToolCalls: req.ParallelToolCalls,
	}
	if out.Model == "" {
		out.Model = "gemini-flash"
	}
	if s := strings.TrimSpace(req.Instructions); s != "" {
		out.Messages = append(out.Messages, model.ChatMessage{Role: "system", Content: s})
	}
	messages, err := parseInput(req.Input)
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
// apply_patch / web_search 等内置工具在入口丢弃并记日志：它们依赖 OpenAI 服务端
// 或客户端专用执行环境，任何上游报文里都不该出现；降级成同名 function 只会把
// 「模型发起了调用但客户端执行不了」的困惑从网关挪到上游。Codex 0.16x 起的
// shell 工具本就是 type=function（exec_command 等），不受影响。
func toChatTools(tools []Tool, logf ir.Logf) []model.Tool {
	out := make([]model.Tool, 0, len(tools))
	var dropped []string
	drop := func(typ string) {
		if typ != "" {
			dropped = append(dropped, typ)
		}
	}
	var appendTool func(tool Tool)
	appendTool = func(tool Tool) {
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
			drop(tool.Type)
		}
	}
	for _, tool := range tools {
		appendTool(tool)
	}
	if len(dropped) > 0 && logf != nil {
		logf("[responses] 丢弃非 function 工具（不进入上游报文）: %s", strings.Join(dropped, ", "))
	}
	return out
}

// toChatToolChoice 把 Responses 的 tool_choice 转成 Chat 形式。两者字符串形式
// 一致；指定函数时结构不同：Responses 是 {type:function,name}，Chat 是
// {type:function,function:{name}}。
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

// parseInput 解析 input：字符串，或 message / function_call 等 item 数组。
func parseInput(raw json.RawMessage) ([]model.ChatMessage, error) {
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

	var items []Item
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
			if role == "developer" {
				role = "system"
			}
			text, images := parseContent(item.Content)
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
			// Content 必须是 "" 而不是 nil：encoding/json 会把 nil any 编成 null，
			// MiniMax 等第三方上游对 assistant.content=null 直接 400。
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

// parseContent 解析 content：字符串，或 [{type:"input_text",text:...}] 数组。
func parseContent(raw json.RawMessage) (string, []model.ContentPart) {
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
	var parts []ContentPart
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

// rawJSONString 取出「JSON 字符串字面量」的内容（function_call 的 arguments 在
// Responses 协议里是字符串，里面再套一层 JSON）。如果上游直接给了对象，就原样返回。
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

// ---- 出站编码 ----

// Usage 输出 Responses 协议的 usage 结构。
func Usage(usage model.TokenUsage) map[string]any {
	return map[string]any{
		"input_tokens":          usage.PromptTokens,
		"output_tokens":         usage.CompletionTokens,
		"total_tokens":          usage.TotalTokens,
		"input_tokens_details":  map[string]any{"cached_tokens": 0},
		"output_tokens_details": map[string]any{"reasoning_tokens": usage.ReasoningTokens},
	}
}

// messageItem 构造 assistant 文本消息项。
func messageItem(id, text, status string) map[string]any {
	return map[string]any{
		"id": id, "type": "message", "status": status, "role": "assistant",
		"content": []any{map[string]any{
			"type": "output_text", "text": text, "annotations": []any{},
		}},
	}
}

// functionCallItem 构造 function_call 输出项。call_id 是客户端回填
// function_call_output 时使用的关联键，必须与模型请求的调用 ID 一致。
func functionCallItem(id, callID, name, arguments, status string) map[string]any {
	if arguments == "" {
		arguments = "{}"
	}
	return map[string]any{
		"id": id, "type": "function_call", "status": status,
		"call_id": callID, "name": name, "arguments": arguments,
	}
}

// Object 构造 Responses 响应对象。createdAt 由调用方固定，保证同一次请求的各条
// 事件里 created_at 一致。
func Object(id, modelName, status string, output []any, usage map[string]any, createdAt int64) map[string]any {
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

// NonStreamResponse 构造非流式 Responses 响应对象（含 output 项）。
// output 项按「首次出现」的顺序：先出文本就占 0，先出工具调用就占 0——
// 模型只回工具调用时不会凭空多出一个空的 message 项。
func NonStreamResponse(req model.ChatRequest, res *model.ChatResult, usage model.TokenUsage) map[string]any {
	if res == nil {
		res = &model.ChatResult{}
	}
	text := res.Text
	toolCalls := res.ToolCalls
	output := make([]any, 0, len(toolCalls)+1)
	if text != "" || len(toolCalls) == 0 {
		output = append(output, messageItem("msg_"+model.NewID(24), text, "completed"))
	}
	for _, call := range toolCalls {
		output = append(output, functionCallItem("fc_"+model.NewID(24), call.ID, call.Function.Name, call.Function.Arguments, "completed"))
	}
	return Object("resp_"+model.NewID(24), req.Model, "completed", output, Usage(usage), time.Now().Unix())
}

// ---- 流式编码 ----

// outputItem 是流式输出中正在累积的一项。
type outputItem struct {
	index  int
	id     string
	kind   string // message / function_call
	text   strings.Builder
	callID string
	name   string
	args   strings.Builder
}

// StreamEncoder 以 response.* 事件流编码流式结果。它是 model.StreamSink 的实现，
// 文本与工具调用增量分别写进对应的 output_item，结束时发射 done 事件与
// response.completed（Codex 判定「这一轮正常结束」的依据）。
type StreamEncoder struct {
	w        StreamWriter
	model    string
	respID   string
	created  int64
	seq      int
	items    []*outputItem
	textItem *outputItem
	toolItem map[int]*outputItem
}

// NewStreamEncoder 创建流式编码器。
func NewStreamEncoder(w StreamWriter, modelName string) *StreamEncoder {
	return &StreamEncoder{
		w:        w,
		model:    modelName,
		respID:   "resp_" + model.NewID(24),
		created:  time.Now().Unix(),
		toolItem: map[int]*outputItem{},
	}
}

// Open 发射 response.created / response.in_progress。
func (e *StreamEncoder) Open() {
	e.emit("response.created", map[string]any{
		"response": Object(e.respID, e.model, "in_progress", []any{}, nil, e.created),
	})
	e.emit("response.in_progress", map[string]any{
		"response": Object(e.respID, e.model, "in_progress", []any{}, nil, e.created),
	})
}

// Sink 返回承接文本与工具调用增量的 model.StreamSink。
func (e *StreamEncoder) Sink() model.StreamSink {
	return &responsesSink{
		text: func(delta string) error {
			item := e.openMessage()
			item.text.WriteString(delta)
			e.emit("response.output_text.delta", map[string]any{
				"item_id": item.id, "output_index": item.index, "content_index": 0,
				"delta": delta, "logprobs": []any{},
			})
			return nil
		},
		tool: func(delta model.ToolCallDelta) error {
			item := e.toolItem[delta.Index]
			if item == nil {
				item = e.createToolItem(delta.Index, delta.ID, delta.Function.Name)
			} else if delta.Function.Name != "" {
				item.name += delta.Function.Name
			}
			if delta.Function.Arguments != "" {
				item.args.WriteString(delta.Function.Arguments)
				e.emit("response.function_call_arguments.delta", map[string]any{
					"item_id": item.id, "output_index": item.index,
					"delta": delta.Function.Arguments,
				})
			}
			return nil
		},
	}
}

// EmitFailed 发射 response.failed（运行错误时由调用方在 ChatStream 返回错误后调用）。
func (e *StreamEncoder) EmitFailed(runErr error) {
	failed := Object(e.respID, e.model, "failed", []any{}, nil, e.created)
	failed["error"] = map[string]any{"code": "server_error", "message": runErr.Error()}
	e.emit("response.failed", map[string]any{"response": failed})
}

// Finish 发射各 output_item 的 done 事件与 response.completed。
// 引擎在流结束后汇总出的工具调用优先于增量拼接结果：部分上游不发增量，
// 只在结束分片上一次性给出完整 tool_calls。
func (e *StreamEncoder) Finish(res *model.ChatResult, usage model.TokenUsage) {
	if res == nil {
		res = &model.ChatResult{}
	}
	if len(e.items) == 0 {
		e.openMessage()
	}
	if len(res.ToolCalls) > 0 {
		for index, call := range res.ToolCalls {
			item := e.toolItem[index]
			if item == nil {
				item = e.createToolItem(index, call.ID, call.Function.Name)
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

	output := make([]any, 0, len(e.items))
	for _, item := range e.items {
		switch item.kind {
		case "message":
			text := item.text.String()
			e.emit("response.output_text.done", map[string]any{
				"item_id": item.id, "output_index": item.index, "content_index": 0, "text": text,
			})
			e.emit("response.content_part.done", map[string]any{
				"item_id": item.id, "output_index": item.index, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": text, "annotations": []any{}},
			})
			msg := messageItem(item.id, text, "completed")
			e.emit("response.output_item.done", map[string]any{"output_index": item.index, "item": msg})
			output = append(output, msg)
		case "function_call":
			args := item.args.String()
			if args == "" {
				args = "{}"
			}
			e.emit("response.function_call_arguments.done", map[string]any{
				"item_id": item.id, "output_index": item.index, "arguments": args,
			})
			call := functionCallItem(item.id, item.callID, item.name, args, "completed")
			e.emit("response.output_item.done", map[string]any{"output_index": item.index, "item": call})
			output = append(output, call)
		}
	}
	e.emit("response.completed", map[string]any{
		"response": Object(e.respID, e.model, "completed", output, Usage(usage), e.created),
	})
}

func (e *StreamEncoder) openMessage() *outputItem {
	if e.textItem != nil {
		return e.textItem
	}
	item := &outputItem{
		index: len(e.items),
		id:    "msg_" + model.NewID(24),
		kind:  "message",
	}
	e.textItem = item
	e.items = append(e.items, item)
	e.emit("response.output_item.added", map[string]any{
		"output_index": item.index,
		"item": map[string]any{
			"id": item.id, "type": "message", "status": "in_progress",
			"role": "assistant", "content": []any{},
		},
	})
	e.emit("response.content_part.added", map[string]any{
		"item_id": item.id, "output_index": item.index, "content_index": 0,
		"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
	})
	return item
}

func (e *StreamEncoder) createToolItem(index int, callID, name string) *outputItem {
	item := &outputItem{
		index:  len(e.items),
		id:     "fc_" + model.NewID(24),
		kind:   "function_call",
		callID: callID,
		name:   name,
	}
	if item.callID == "" {
		item.callID = "call_" + model.NewID(24)
	}
	e.toolItem[index] = item
	e.items = append(e.items, item)
	e.emit("response.output_item.added", map[string]any{
		"output_index": item.index,
		"item": map[string]any{
			"id": item.id, "type": "function_call", "status": "in_progress",
			"call_id": item.callID, "name": item.name, "arguments": "",
		},
	})
	return item
}

func (e *StreamEncoder) emit(event string, payload map[string]any) {
	if e.w.Err() != nil {
		return
	}
	payload["type"] = event
	payload["sequence_number"] = e.seq
	e.seq++
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = e.w.Write(event, data)
}

// responsesSink 把增量分发到 Responses 事件编码器。
type responsesSink struct {
	text func(string) error
	tool func(model.ToolCallDelta) error
}

func (s *responsesSink) Text(delta string) error {
	if s.text == nil {
		return nil
	}
	return s.text(delta)
}

func (s *responsesSink) ToolCall(delta model.ToolCallDelta) error {
	if s.tool == nil {
		return nil
	}
	return s.tool(delta)
}
