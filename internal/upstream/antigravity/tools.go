package antigravity

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Tool 是发往 v1internal 的函数工具定义（Gemini functionDeclarations）。
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ToolCall 模型发起的一次函数调用。
type ToolCall struct {
	ID         string
	Name       string
	Arguments  string // JSON 对象字符串
	ThoughtSig string
}

// ToolCallDelta 流式工具调用增量（Antigravity 上游通常整包下发，增量一次给齐）。
type ToolCallDelta struct {
	Index      int
	ID         string
	Name       string
	Arguments  string
	ThoughtSig string
}

// Message 扩展字段：assistant 的 ToolCalls、tool 角色的回填。
// 为保持兼容，这些字段加在 Message 上（见本文件的辅助构造，不改原 Role/Text 语义）。

// withTools 把 OpenAI 风格 tools 写进 geminiBody。
func applyTools(body *geminiBody, tools []Tool, toolChoice json.RawMessage) {
	if body == nil {
		return
	}
	decls := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		var params any
		raw := tool.Parameters
		if len(raw) == 0 || string(raw) == "null" {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		} else if err := json.Unmarshal(raw, &params); err != nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		decl := map[string]any{
			"name":        name,
			"description": tool.Description,
			"parameters":  params,
		}
		decls = append(decls, decl)
	}
	if len(decls) > 0 {
		body.Tools = []map[string]any{{
			"functionDeclarations": decls,
		}}
	}
	mode := "AUTO"
	if len(decls) == 0 {
		mode = "VALIDATED"
	}
	// tool_choice：字符串 auto/none/required；对象指定函数时用 ANY + allowedFunctionNames
	trimmed := strings.TrimSpace(string(toolChoice))
	if trimmed != "" && trimmed != "null" {
		if strings.HasPrefix(trimmed, "\"") {
			var s string
			if json.Unmarshal(toolChoice, &s) == nil {
				switch strings.ToLower(s) {
				case "none":
					mode = "NONE"
				case "required", "any":
					mode = "ANY"
				default:
					mode = "AUTO"
				}
			}
		} else {
			var choice struct {
				Type     string `json:"type"`
				Name     string `json:"name"`
				Function *struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if json.Unmarshal(toolChoice, &choice) == nil {
				name := choice.Name
				if name == "" && choice.Function != nil {
					name = choice.Function.Name
				}
				if name != "" {
					mode = "ANY"
					body.ToolConfig = map[string]any{
						"functionCallingConfig": map[string]any{
							"mode":                 mode,
							"allowedFunctionNames": []string{name},
						},
					}
					return
				}
			}
		}
	}
	body.ToolConfig = map[string]any{
		"functionCallingConfig": map[string]any{"mode": mode},
	}
}

// appendFunctionCallPart 把一次工具调用写成 model 角色的 functionCall part。
func appendFunctionCallPart(parts *[]geminiPart, call ToolCall) {
	args := json.RawMessage(call.Arguments)
	if !json.Valid(args) || len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var argsObj any
	if err := json.Unmarshal(args, &argsObj); err != nil {
		argsObj = map[string]any{}
	}
	part := geminiPart{
		FunctionCall: &geminiFunctionCall{
			Name: call.Name,
			Args: argsObj,
		},
	}
	sig := strings.TrimSpace(call.ThoughtSig)
	if sig == "" {
		sig = DummyThoughtSig
	}
	part.ThoughtSignature = sig
	*parts = append(*parts, part)
}

// appendFunctionResponsePart 把工具执行结果写成 user 角色的 functionResponse part。
func appendFunctionResponsePart(parts *[]geminiPart, callID, name, output string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "tool"
	}
	resp := map[string]any{"result": output}
	if strings.TrimSpace(callID) != "" {
		resp["id"] = callID
	}
	*parts = append(*parts, geminiPart{
		FunctionResponse: &geminiFunctionResponse{
			Name:     name,
			Response: resp,
		},
	})
}

func argsToJSONString(args any) string {
	if args == nil {
		return "{}"
	}
	switch v := args.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return "{}"
		}
		if json.Valid([]byte(v)) {
			return v
		}
		b, err := json.Marshal(v)
		if err != nil {
			return "{}"
		}
		return string(b)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "{}"
		}
		return string(b)
	}
}

func newToolCallID(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		n = "tool"
	}
	return fmt.Sprintf("call_%s_%s", n, hex16())
}
