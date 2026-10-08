//go:build web2api_unit

package provider

import (
	"encoding/json"
	"testing"

	aistudio "web2api/internal/aistudio2api/aistudio"
	"web2api/internal/model"
)

func TestChatImageSizeEncoded(t *testing.T) {
	t.Run("4K", func(t *testing.T) {
		size := encodedImageSize(t, "4K")
		if size != "4K" {
			t.Fatalf("image size = %q, want 4K", size)
		}
	})
	for _, allowed := range []string{"512", "1K", "2K"} {
		allowed := allowed
		t.Run(allowed, func(t *testing.T) {
			if got := encodedImageSize(t, allowed); got != allowed {
				t.Fatalf("image size = %q, want %s", got, allowed)
			}
		})
	}
	t.Run("default 1K", func(t *testing.T) {
		size := encodedImageSize(t, "")
		if size != "1K" {
			t.Fatalf("omitted image_size encoded %q, want default 1K", size)
		}
	})
	t.Run("reject lowercase", func(t *testing.T) {
		_, err := (&NativeAIStudioEngine{}).buildGenerateRequest(model.ChatRequest{ImageSize: "4k"})
		if err == nil {
			t.Fatal("lowercase 4k should be rejected")
		}
	})
}

func encodedImageSize(t *testing.T, imageSize string) string {
	t.Helper()
	gen, err := (&NativeAIStudioEngine{}).buildGenerateRequest(model.ChatRequest{
		Model:     "gemini-nano-banana-2.1",
		Messages:  []model.ChatMessage{{Role: "user", Content: "a red circle"}},
		ImageSize: imageSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := aistudio.EncodeGenerateContentRequest(gen, aistudio.GenerationDefaults{
		OutputResolution: true,
		MaxOutputTokens:  1024,
		ImageRoute:       true,
	}, aistudio.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	var wire []any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire) < 4 {
		t.Fatalf("wire length %d: %s", len(wire), body)
	}
	generation, ok := wire[3].([]any)
	if !ok || len(generation) < 27 {
		t.Fatalf("generation config = %#v", wire[3])
	}
	config, ok := generation[26].([]any)
	if !ok || len(config) < 2 {
		t.Fatalf("image config = %#v", generation[26])
	}
	size, _ := config[1].(string)
	return size
}
