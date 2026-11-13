package model

import (
	"encoding/json"
	"testing"
)

// 上游缓存用量有两种写法：OpenAI 的 prompt_tokens_details.cached_tokens
// 与 Anthropic 风格的 cache_read_input_tokens，都要落到 CachedTokens。
func TestTokenUsageCachedTokensUnmarshal(t *testing.T) {
	var openai TokenUsage
	if err := json.Unmarshal([]byte(`{"prompt_tokens":1100,"completion_tokens":249,"total_tokens":1349,"prompt_tokens_details":{"cached_tokens":1089}}`), &openai); err != nil {
		t.Fatal(err)
	}
	if openai.CachedTokens != 1089 || openai.PromptTokens != 1100 || openai.CompletionTokens != 249 {
		t.Fatalf("openai usage = %+v", openai)
	}
	var anthropic TokenUsage
	if err := json.Unmarshal([]byte(`{"prompt_tokens":10,"completion_tokens":5,"cache_read_input_tokens":7}`), &anthropic); err != nil {
		t.Fatal(err)
	}
	if anthropic.CachedTokens != 7 {
		t.Fatalf("anthropic usage = %+v", anthropic)
	}
}
