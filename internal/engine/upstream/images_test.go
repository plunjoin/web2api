package upstream

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"web2api/internal/model"
)

const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestParseGeminiGenerateContentImage(t *testing.T) {
	body := `{
		"candidates": [{"content": {"parts": [
			{"text": "here is your cover"},
			{"inlineData": {"mimeType": "image/png", "data": "` + tinyPNG + `"}}
		]}, "finishReason": "STOP"}],
		"usageMetadata": {"promptTokenCount": 12, "candidatesTokenCount": 34, "totalTokenCount": 46}
	}`
	res, err := parseGeminiGenerateContentImage([]byte(body), "custom", "gemini-nano-banana-2.1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(res.Text, "here is your cover") {
		t.Fatalf("text part lost: %q", res.Text)
	}
	if !strings.Contains(res.Text, "![media](data:image/png;base64,"+tinyPNG+")") {
		t.Fatalf("image part not encoded as data URI: %q", res.Text)
	}
	if res.Usage == nil || res.Usage.TotalTokens != 46 {
		t.Fatalf("usage not parsed: %+v", res.Usage)
	}
}

func TestParseGeminiGenerateContentImageRequiresImage(t *testing.T) {
	body := `{"candidates": [{"content": {"parts": [{"text": "I cannot generate that."}]}}]}`
	if _, err := parseGeminiGenerateContentImage([]byte(body), "custom", "m"); err == nil {
		t.Fatal("text-only response must be treated as no image")
	}
}

func TestMessageImagesMarkdown(t *testing.T) {
	remote := model.MessageImage{}
	remote.ImageURL = &struct {
		URL string `json:"url"`
	}{URL: "https://cdn.example.com/a.png"}
	inline := model.MessageImage{}
	inline.ImageURL = &struct {
		URL string `json:"url"`
	}{URL: "data:image/jpeg;base64," + tinyPNG}
	raw := model.MessageImage{B64JSON: tinyPNG, MIMEType: "image/png"}
	html := model.MessageImage{}
	html.ImageURL = &struct {
		URL string `json:"url"`
	}{URL: "javascript:alert(1)"}

	got := messageImagesMarkdown([]model.MessageImage{remote, inline, raw, html})
	want := "\n![media](https://cdn.example.com/a.png)\n" +
		"\n![media](data:image/jpeg;base64," + tinyPNG + ")\n" +
		"\n![media](data:image/png;base64," + tinyPNG + ")\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Gemini 官方 OpenAI 兼容层产不出图片，图片模型要走原生 :generateContent；
// 请求里要带 X-Goog-Api-Key、responseModalities 与 imageConfig。
func TestChatImageUsesNativeGeminiGenerateContent(t *testing.T) {
	var gotPath, gotKey, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-Goog-Api-Key")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates": [{"content": {"parts": [
			{"inlineData": {"mimeType": "image/png", "data": "` + tinyPNG + `"}}
		]}}]}`))
	}))
	defer srv.Close()

	o := New("custom", srv.URL+"/v1beta/openai", "test-key", false, 0)
	res, err := o.Chat(context.Background(), model.ChatRequest{
		Model:     "gemini-nano-banana-2.1",
		ImageSize: "1K",
		Messages:  []model.ChatMessage{{Role: "user", Content: "a cover"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if gotPath != "/v1beta/models/gemini-nano-banana-2.1:generateContent" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotKey != "test-key" {
		t.Fatalf("api key header = %q", gotKey)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("payload: %v (%s)", err, gotBody)
	}
	cfg, _ := payload["generationConfig"].(map[string]any)
	modes, _ := cfg["responseModalities"].([]any)
	if len(modes) != 1 || modes[0] != "IMAGE" {
		t.Fatalf("responseModalities = %v", cfg["responseModalities"])
	}
	imageCfg, _ := cfg["imageConfig"].(map[string]any)
	if imageCfg["imageSize"] != "1K" {
		t.Fatalf("imageConfig = %v", imageCfg)
	}
	if !strings.Contains(res.Text, "data:image/png;base64,"+tinyPNG) {
		t.Fatalf("result text = %q", res.Text)
	}
}

// 原生协议失败时回退到 OpenAI 兼容 chat，并解析 message.images。
func TestChatImageFallsBackToOpenAICompatImages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, ":generateContent") {
			http.Error(w, `{"error":{"message":"not supported"}}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "gemini-nano-banana-2.1",
			"choices": [{"message": {"role": "assistant", "content": "done", "images": [
				{"type": "image_url", "image_url": {"url": "data:image/png;base64,` + tinyPNG + `"}}
			]}}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3}
		}`))
	}))
	defer srv.Close()

	o := New("custom", srv.URL+"/v1beta/openai", "test-key", false, 0)
	res, err := o.Chat(context.Background(), model.ChatRequest{
		Model:    "gemini-nano-banana-2.1",
		Messages: []model.ChatMessage{{Role: "user", Content: "a cover"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if !strings.Contains(res.Text, "done") || !strings.Contains(res.Text, "data:image/png;base64,"+tinyPNG) {
		t.Fatalf("result text = %q", res.Text)
	}
}
