package model

import "strings"

// 模型用途。前端按这个字段筛选，不再靠模型 ID 猜测。
const (
	KindChat      = "chat"
	KindImage     = "image"
	KindVideo     = "video"
	KindMusic     = "music"
	KindSpeech    = "speech"
	KindEmbedding = "embedding"
)

// ClassifyKind 根据上游能力标记和模型名判断用途。
// 能力标记优先：AI Studio 目录里的 video_route、image_route 等。
// 没有标记时再看模型 ID 和显示名里的关键词。
func ClassifyKind(id, display string, capabilities map[string]bool) string {
	switch {
	case capabilities["video_route"]:
		return KindVideo
	case capabilities["image_route"]:
		return KindImage
	case capabilities["music_route"]:
		return KindMusic
	case capabilities["speech_route"] || capabilities["transcription_output"] || capabilities["live_route"]:
		return KindSpeech
	}
	return kindFromName(id + " " + display)
}

// PreferKind 在同一模型来自多个引擎时保留更具体的用途。
func PreferKind(current, next string) string {
	if kindRank(next) > kindRank(current) {
		return next
	}
	return current
}

func kindRank(kind string) int {
	switch kind {
	case KindVideo:
		return 5
	case KindImage, KindMusic:
		return 4
	case KindSpeech:
		return 3
	case KindEmbedding:
		return 2
	default:
		return 1
	}
}

func kindFromName(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "veo") || strings.Contains(n, "video"):
		return KindVideo
	case strings.Contains(n, "lyria") || strings.Contains(n, "music"):
		return KindMusic
	case strings.Contains(n, "imagen") || strings.Contains(n, "image") || strings.Contains(n, "nano-banana"):
		return KindImage
	case strings.Contains(n, "tts") || strings.Contains(n, "speech") || strings.Contains(n, "audio"):
		return KindSpeech
	case strings.Contains(n, "embedding") || strings.Contains(n, "embed"):
		return KindEmbedding
	default:
		return KindChat
	}
}
