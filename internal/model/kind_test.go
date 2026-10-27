package model

import "testing"

func TestKindFromNameGeminiKeyCatalog(t *testing.T) {
	cases := map[string]string{
		// 实测 chat/completions 200：保持 chat
		"gemini-2.5-flash":                        KindChat,
		"gemini-3.5-flash":                        KindChat,
		"gemma-4-26b-a4b-it":                      KindChat,
		"gemini-3.1-pro-preview-customtools":      KindChat,
		"gemini-robotics-er-2-preview":            KindChat,
		"gemini-2.5-computer-use-preview-10-2025": KindChat,
		"gemini-flash-latest":                     KindChat,
		"gpt-5.5":                                 KindChat,
		"claude-sonnet-4-5":                       KindChat,
		"deepseek-v4-flash":                       KindChat,
		// 只支持 WebSocket bidiGenerateContent / 转写
		"gemini-3.8-live":                               KindSpeech,
		"gemini-3.8-live-extended-thinking":             KindSpeech,
		"gemini-3.1-flash-live-preview":                 KindSpeech,
		"gemini-3.5-live-translate-preview":             KindSpeech,
		"gemini-3.5-transcribe":                         KindSpeech,
		"gemini-3.5-transcribe-live":                    KindSpeech,
		"gemini-robotics-er-2-streaming-preview":        KindSpeech,
		"gemini-2.5-flash-native-audio-preview-12-2025": KindSpeech,
		"models/gemini-2.5-flash-native-audio-latest":   KindSpeech,
		"gemini-3.8-flash-tts":                          KindSpeech,
		// Interactions-only / 专用接口
		"deep-research-preview-04-2026":            KindOther,
		"deep-research-max-preview-04-2026":        KindOther,
		"antigravity-preview-latest":               KindOther,
		"antigravity-preview-09-2026":              KindOther,
		"models/deep-research-pro-preview-12-2025": KindOther,
		"models/antigravity-preview-05-2026":       KindOther,
		"models/aqa":                               KindOther,
		"models/gemini-3.8-live":                   KindSpeech,
		"models/gemini-2.5-flash":                  KindChat,
		"aqa":                                      KindOther,
		// 既有规则不变
		"veo-3.1-generate":          KindVideo,
		"gemini-omni-flash-preview": KindVideo,
		"lyria-3.5":                 KindMusic,
		"gemini-3-pro-image":        KindImage,
		"nano-banana-pro-preview":   KindImage,
		"gemini-embedding-001":      KindEmbedding,
	}
	for name, want := range cases {
		if got := kindFromName(name); got != want {
			t.Errorf("kindFromName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestClassifyKindCapabilitiesWin(t *testing.T) {
	// AI Studio 能力标记优先于名字：名字看起来像 live，标了 image_route 就是生图。
	if got := ClassifyKind("gemini-3.8-live", "", map[string]bool{"image_route": true}); got != KindImage {
		t.Fatalf("capability should win, got %q", got)
	}
	if got := ClassifyKind("some-model", "", map[string]bool{"live_route": true}); got != KindSpeech {
		t.Fatalf("live_route should map to speech, got %q", got)
	}
	if got := ClassifyKind("gemini-2.5-flash", "Gemini 2.5 Flash", nil); got != KindChat {
		t.Fatalf("plain chat model misclassified: %q", got)
	}
}
