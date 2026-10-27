package model

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestToolCallExtraCache(t *testing.T) {
	RememberToolCallExtra("call_a", json.RawMessage(`{"google":{"thought_signature":"s"}}`))
	if got := string(LookupToolCallExtra("call_a")); got != `{"google":{"thought_signature":"s"}}` {
		t.Fatalf("lookup = %s", got)
	}
	RememberToolCallExtra("", json.RawMessage(`{}`))
	RememberToolCallExtra("call_b", nil)
	if LookupToolCallExtra("call_b") != nil || LookupToolCallExtra("") != nil {
		t.Fatal("empty ids / extras must not be stored")
	}
	for i := 0; i < toolCallExtraCap+10; i++ {
		RememberToolCallExtra(fmt.Sprintf("bulk_%d", i), json.RawMessage(`1`))
	}
	if LookupToolCallExtra("call_a") != nil {
		t.Fatal("oldest entry should be evicted once the cache is full")
	}
}

func TestToolCallExtraContentRoundTrip(t *testing.T) {
	var call ToolCall
	in := `{"id":"c1","type":"function","function":{"name":"f","arguments":"{}"},"extra_content":{"google":{"thought_signature":"x"}}}`
	if err := json.Unmarshal([]byte(in), &call); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(call)
	if string(out) != in {
		t.Fatalf("round trip mismatch:\n got %s\nwant %s", out, in)
	}
}
