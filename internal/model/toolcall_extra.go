package model

import (
	"encoding/json"
	"strings"
	"sync"
)

// 工具调用扩展信息缓存（call_id → extra_content）。
//
// Responses 客户端（Codex）回传历史 function_call 时只带 call_id/name/arguments，
// 上游给的 thought_signature 会丢；网关在返回工具调用时记下，下一轮按 call_id
// 补回去。容量有限、先进先出，进程重启后丢失（上游侧有占位签名兜底）。
const toolCallExtraCap = 8192

var toolCallExtras = struct {
	sync.Mutex
	m     map[string]json.RawMessage
	order []string
}{m: map[string]json.RawMessage{}}

// RememberToolCallExtra 记录 call_id 对应的 extra_content。
func RememberToolCallExtra(callID string, extra json.RawMessage) {
	callID = strings.TrimSpace(callID)
	if callID == "" || len(extra) == 0 {
		return
	}
	toolCallExtras.Lock()
	defer toolCallExtras.Unlock()
	if _, ok := toolCallExtras.m[callID]; !ok {
		toolCallExtras.order = append(toolCallExtras.order, callID)
	}
	toolCallExtras.m[callID] = append(json.RawMessage(nil), extra...)
	for len(toolCallExtras.order) > toolCallExtraCap {
		delete(toolCallExtras.m, toolCallExtras.order[0])
		toolCallExtras.order = toolCallExtras.order[1:]
	}
}

// LookupToolCallExtra 取回 call_id 对应的 extra_content。
func LookupToolCallExtra(callID string) json.RawMessage {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil
	}
	toolCallExtras.Lock()
	defer toolCallExtras.Unlock()
	return toolCallExtras.m[callID]
}
