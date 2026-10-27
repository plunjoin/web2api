package gateway

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseResponsesInputFillsToolNameAndEmptyAssistantContent(t *testing.T) {
	raw := json.RawMessage(`[
		{"type":"message","role":"user","content":"hi"},
		{"type":"function_call","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"ls\"}"},
		{"type":"function_call_output","call_id":"call_1","output":"ok"}
	]`)
	msgs, err := parseResponsesInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("msgs=%d %#v", len(msgs), msgs)
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "" {
		t.Fatalf("assistant content must be empty string, got %#v", msgs[1].Content)
	}
	data, _ := json.Marshal(msgs[1])
	if strings.Contains(string(data), `"content":null`) {
		t.Fatalf("content must not marshal as null: %s", data)
	}
	if msgs[2].Role != "tool" || msgs[2].Name != "exec_command" || msgs[2].ToolCallID != "call_1" {
		t.Fatalf("tool msg=%#v", msgs[2])
	}
}


func TestToChatToolsKeepsFunctionAndFlattensNamespace(t *testing.T) {
	req := responsesRequest{Tools: []responsesTool{
		{Type: "function", Name: "exec_command", Description: "run", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Type: "namespace", Name: "multi_agent_v1", Tools: []responsesTool{
			{Type: "function", Name: "spawn_agent", Parameters: json.RawMessage(`{"type":"object"}`)},
		}},
		{Type: "web_search"},
		{Type: "local_shell"},
	}}
	tools := req.toChatTools()
	names := map[string]bool{}
	for _, tool := range tools {
		if tool.Function == nil {
			t.Fatalf("nil function: %+v", tool)
		}
		names[tool.Function.Name] = true
	}
	for _, want := range []string{"exec_command", "spawn_agent", "local_shell"} {
		if !names[want] {
			t.Fatalf("missing %s in %#v", want, names)
		}
	}
	if names["web_search"] || names["multi_agent_v1"] {
		t.Fatalf("should drop web_search/namespace wrapper, got %#v", names)
	}
}

func TestToChatToolsLocalShellGetsCommandSchema(t *testing.T) {
	req := responsesRequest{Tools: []responsesTool{{Type: "local_shell"}}}
	tools := req.toChatTools()
	if len(tools) != 1 || tools[0].Function == nil {
		t.Fatalf("tools=%+v", tools)
	}
	if tools[0].Function.Name != "local_shell" {
		t.Fatalf("name=%q", tools[0].Function.Name)
	}
	var schema map[string]any
	if err := json.Unmarshal(tools[0].Function.Parameters, &schema); err != nil {
		t.Fatal(err)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["command"]; !ok {
		t.Fatalf("expected command property, got %#v", schema)
	}
}
