//go:build web2api_unit

package model

import "testing"

func TestClassifyKind(t *testing.T) {
	cases := []struct {
		id, display string
		caps        map[string]bool
		want        string
	}{
		{id: "gemini-2.5-flash", want: KindChat},
		{id: "gemini-3.1-flash-image", want: KindImage},
		{id: "gpt-image-2.5-sunburst", want: KindImage},
		{id: "dall-e-3", want: KindImage},
		{id: "veo-3.1-fast-generate-preview", want: KindVideo},
		{id: "gemini-omni-1.1-flash", want: KindVideo},
		{id: "gemini-omni-flash-preview", want: KindVideo},
		{id: "lyria-3", display: "Lyria", want: KindMusic},
		{id: "gemini-2.5-flash-preview-tts", want: KindSpeech},
		{id: "text-embedding-004", want: KindEmbedding},
		{id: "gemini-flash", caps: map[string]bool{"chat_model": true}, want: KindChat},
		{id: "custom", caps: map[string]bool{"image_route": true, "chat_model": true}, want: KindImage},
		{id: "custom-video", caps: map[string]bool{"video_route": true, "image_route": true}, want: KindVideo},
		{id: "named-image", caps: map[string]bool{"music_route": true}, want: KindMusic},
	}
	if PreferKind(KindChat, KindImage) != KindImage || PreferKind(KindVideo, KindChat) != KindVideo {
		t.Fatal("合并用途时应保留生图或视频，而不是对话")
	}
	for _, tc := range cases {
		if got := ClassifyKind(tc.id, tc.display, tc.caps); got != tc.want {
			t.Fatalf("ClassifyKind(%q, %q) = %q, want %q", tc.id, tc.display, got, tc.want)
		}
	}
}
