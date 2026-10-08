//go:build web2api_unit

package provider

import (
	"encoding/json"
	"testing"

	aistudio "web2api/internal/aistudio2api/aistudio"
	"web2api/internal/model"
)

func TestVideo4kEncoded(t *testing.T) {
	t.Run("videos size", func(t *testing.T) {
		body, err := aistudio.EncodeGenerateVideoRequest(aistudio.VideoRequest{
			Model:  "veo-3.1-fast-generate-preview",
			Prompt: "a red circle",
			Size:   "4k",
		})
		if err != nil {
			t.Fatal(err)
		}
		var wire []any
		if err := json.Unmarshal(body, &wire); err != nil {
			t.Fatal(err)
		}
		config, ok := wire[2].([]any)
		if !ok || len(config) < 4 {
			t.Fatalf("video config = %#v", wire[2])
		}
		if config[3] != "4k" {
			t.Fatalf("encoded resolution = %#v, want 4k", config[3])
		}
	})
	t.Run("omni chat 4k", func(t *testing.T) {
		// KWa field 6, JSON index 5. Request enum: 1080p=3, 4k=4.
		if got := encodedOmniResolution(t, "4k"); got != float64(4) {
			t.Fatalf("interaction[53][0][0][3][5] = %#v, want 4 for 4k", got)
		}
	})
	t.Run("omni chat 1080p is not 4k", func(t *testing.T) {
		if got := encodedOmniResolution(t, "1080p"); got != float64(3) {
			t.Fatalf("interaction[53][0][0][3][5] = %#v, want 3 for 1080p", got)
		}
	})
	t.Run("omni omitted keeps historical 1", func(t *testing.T) {
		if got := encodedOmniResolution(t, ""); got != float64(1) {
			t.Fatalf("omitted resolution = %#v, want historical 1", got)
		}
	})
}

func encodedOmniResolution(t *testing.T, resolution string) any {
	t.Helper()
	gen, err := (&NativeAIStudioEngine{}).buildGenerateRequest(model.ChatRequest{
		Model:      "gemini-omni-1.1-flash",
		Messages:   []model.ChatMessage{{Role: "user", Content: "a red circle moving"}},
		Resolution: resolution,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := aistudio.EncodeCreateInteractionStreamRequest(gen, aistudio.GenerationDefaults{
		MaxOutputTokens:      8192,
		DefaultThinkingLevel: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	var root []any
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatal(err)
	}
	interaction, ok := root[3].([]any)
	if !ok || len(interaction) < 54 {
		t.Fatalf("interaction = %#v", root[3])
	}
	outer, ok := interaction[53].([]any)
	if !ok || len(outer) == 0 {
		t.Fatalf("video config = %#v", interaction[53])
	}
	mid, ok := outer[0].([]any)
	if !ok || len(mid) == 0 {
		t.Fatalf("video config mid = %#v", outer[0])
	}
	inner, ok := mid[0].([]any)
	if !ok || len(inner) < 4 {
		t.Fatalf("video config inner = %#v", mid[0])
	}
	slot, ok := inner[3].([]any)
	if !ok || len(slot) < 6 {
		t.Fatalf("resolution slot = %#v", inner[3])
	}
	return slot[5]
}
