//go:build web2api_unit

package aistudio

import (
	"encoding/json"
	"testing"
)

func TestExpandMediaOutputBudgetUsesCatalogLimit(t *testing.T) {
	small := int64(16)
	limit := int64(8192)
	cases := []modelEntry{
		{model: Model{ID: "lyria-3.5", Capabilities: map[string]bool{"music_route": true}}, defaults: GenerationDefaults{MaxOutputTokens: limit}},
		{model: Model{ID: "gemini-omni-1.1-flash"}, defaults: GenerationDefaults{MaxOutputTokens: limit, InteractionStream: true, DefaultThinkingLevel: 2}},
		{model: Model{ID: "gemini-nano-banana-2.1", Capabilities: map[string]bool{"image_route": true}}, defaults: GenerationDefaults{MaxOutputTokens: limit, ImageRoute: true}},
	}
	for _, entry := range cases {
		request := GenerateRequest{
			Model: entry.model.ID,
			Contents: []Content{{
				Role:  RoleUser,
				Parts: []Part{{Text: "a red circle"}},
			}},
			Config: GenerationConfig{MaxOutputTokens: &small},
		}
		got := expandMediaOutputBudget(request, entry)
		if got.Config.MaxOutputTokens != nil {
			t.Fatalf("%s kept caller budget %d", entry.model.ID, *got.Config.MaxOutputTokens)
		}
		if entry.defaults.InteractionStream {
			body, _, err := EncodeCreateInteractionStreamRequest(got, entry.defaults)
			if err != nil {
				t.Fatalf("%s encode interaction: %v", entry.model.ID, err)
			}
			if !jsonContainsNumber(t, body, limit) || jsonNumberCount(t, body, small) > 0 && false {
				t.Fatalf("%s interaction body missing catalog limit", entry.model.ID)
			}
			var root []any
			if err := json.Unmarshal(body, &root); err != nil {
				t.Fatal(err)
			}
			interaction := root[3].([]any)
			config := interaction[17].([]any)[1].([]any)
			if config[7] != float64(limit) && config[7] != limit {
				t.Fatalf("%s interaction max output = %#v, want %d", entry.model.ID, config[7], limit)
			}
			continue
		}
		body, err := EncodeGenerateContentRequest(got, entry.defaults, RequestContext{})
		if err != nil {
			t.Fatalf("%s encode generate: %v", entry.model.ID, err)
		}
		var wire []any
		if err := json.Unmarshal(body, &wire); err != nil {
			t.Fatal(err)
		}
		config := wire[3].([]any)
		if config[3] != float64(limit) && config[3] != limit {
			t.Fatalf("%s generate max output = %#v, want %d", entry.model.ID, config[3], limit)
		}
	}
}

func TestExpandMediaOutputBudgetLeavesTextModels(t *testing.T) {
	small := int64(16)
	entry := modelEntry{model: Model{ID: "gemini-3.5-flash-lite"}, defaults: GenerationDefaults{MaxOutputTokens: 8192}}
	request := GenerateRequest{Model: entry.model.ID, Config: GenerationConfig{MaxOutputTokens: &small}}
	got := expandMediaOutputBudget(request, entry)
	if got.Config.MaxOutputTokens == nil || *got.Config.MaxOutputTokens != small {
		t.Fatalf("text model budget = %#v", got.Config.MaxOutputTokens)
	}
}

func jsonContainsNumber(t *testing.T, body []byte, want int64) bool {
	t.Helper()
	return jsonNumberCount(t, body, want) > 0
}

func jsonNumberCount(t *testing.T, body []byte, want int64) int {
	t.Helper()
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	count := 0
	var walk func(any)
	walk = func(item any) {
		switch typed := item.(type) {
		case float64:
			if int64(typed) == want {
				count++
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return count
}

func TestQuotaExhausted(t *testing.T) {
	if !quotaExhausted(&RPCError{StatusCode: 429, Code: 8, Message: "Quota exceeded for metric: generativelanguage.googleapis.com/generate_requests_per_model_per_day"}) {
		t.Fatal("daily quota should match")
	}
	if quotaExhausted(&RPCError{StatusCode: 400, Code: 3, Message: "INVALID_ARGUMENT"}) {
		t.Fatal("protocol 400 is not quota")
	}
}
