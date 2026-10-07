//go:build web2api_unit

package provider

import (
	"testing"

	"web2api/internal/model"
)

func TestGeminiCredentialsFromStorageState(t *testing.T) {
	raw := `{"cookies":[{"name":"SID","value":"sid"},{"name":"__Secure-1PSID","value":"psid"},{"name":"__Secure-1PSIDTS","value":"psidts"}],"origins":[]}`
	creds, err := geminiCredentialsFromStorageState(raw)
	if err != nil {
		t.Fatalf("提取引擎2 Cookie 失败: %v", err)
	}
	if creds.PSID != "psid" || creds.PSIDTS != "psidts" {
		t.Fatalf("Cookie 提取结果错误: %+v", creds)
	}
}

func TestGeminiCredentialsFromStorageStateRequiresCookies(t *testing.T) {
	if _, err := geminiCredentialsFromStorageState(`{"cookies":[]}`); err == nil {
		t.Fatal("缺少 Gemini Cookie 时应返回错误")
	}
}

func TestMessagesToPrompt(t *testing.T) {
	messages := []model.ChatMessage{
		{Role: "system", Content: "你是一个助手"},
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "你好！"},
		{Role: "user", Content: "介绍一下自己"},
	}
	prompt := messagesToPrompt(messages)
	want := "[System Instructions]\n你是一个助手\n\n[Conversation]\nUser: 你好\n\nAssistant: 你好！\n\nUser: 介绍一下自己"
	if prompt != want {
		t.Fatalf("prompt 转换错误:\n got %q\nwant %q", prompt, want)
	}
}

func TestMessagesToPromptNoSystem(t *testing.T) {
	messages := []model.ChatMessage{
		{Role: "user", Content: "hi"},
	}
	prompt := messagesToPrompt(messages)
	if prompt != "User: hi" {
		t.Fatalf("无 system 时 prompt 错误: %q", prompt)
	}
}

func TestMatchAny(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		want     bool
	}{
		{"gemini-flash", []string{"gemini-flash", "gemini-pro"}, true},
		{"gemini-3.7-flash", []string{"gemini-3"}, true},
		{"veo-3.1", []string{"gemini-*"}, false},
		{"gemini-flash-lite", []string{"gemini-flash*"}, true},
		{"custom-model", []string{"gemini"}, false},
	}
	for _, c := range cases {
		if got := matchAny(c.name, c.patterns); got != c.want {
			t.Errorf("matchAny(%q, %v) = %v, want %v", c.name, c.patterns, got, c.want)
		}
	}
}

func TestIsMultimodalModel(t *testing.T) {
	cases := map[string]bool{
		"gemini-3.1-flash-image": true,
		"veo-3.1-fast":           true,
		"lyria-3-pro":            true,
		"gemini-3.8-flash-tts":   true,
		"gemini-3.7-flash":       false,
		"gemini-pro":             false,
	}
	for name, want := range cases {
		if got := isMultimodalModel(name); got != want {
			t.Errorf("isMultimodalModel(%q) = %v, want %v", name, got, want)
		}
	}
}
