package model

import (
	"regexp"
	"strings"
)

// 模型用途。前端按这个字段筛选，不再靠模型 ID 猜测。
const (
	KindChat      = "chat"
	KindImage     = "image"
	KindVideo     = "video"
	KindMusic     = "music"
	KindSpeech    = "speech"
	KindEmbedding = "embedding"
	// KindOther 不走 chat/completions 的特殊模型：只支持 Interactions API 的
	// 智能体（deep-research-*、antigravity-preview-*）和 aqa 这类专用接口模型。
	KindOther = "other"
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

// ChatIncompatibleKind 该用途的模型无法通过 /v1/chat/completions 产出结果。
// 音乐和视频必须走各自的专用端点：OpenAI 兼容的 chat 端点只能返回文本，
// 把音乐模型发过去，上游会直接拒（如 Google 的
// “Unhandled generated data mime type: audio/mpeg”）。
// 向量模型不在此列——历史上有调用方拿 chat 端点探活，保持原样不动。
func ChatIncompatibleKind(kind string) bool {
	switch kind {
	case KindMusic, KindVideo:
		return true
	}
	return false
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
	case KindEmbedding, KindOther:
		return 2
	default:
		return 1
	}
}

func kindFromName(name string) string {
	n := strings.ToLower(name)
	base := n[strings.LastIndex(n, "/")+1:] // 去掉 models/ 之类的前缀
	switch {
	case strings.Contains(n, "veo") || strings.Contains(n, "video") || strings.Contains(n, "omni"):
		return KindVideo
	case strings.Contains(n, "lyria") || strings.Contains(n, "music"):
		return KindMusic
	case strings.Contains(n, "imagen") || strings.Contains(n, "image") || strings.Contains(n, "nano-banana") || strings.Contains(n, "dall-e") || strings.Contains(n, "dalle"):
		return KindImage
	case strings.Contains(n, "tts") || strings.Contains(n, "speech") || strings.Contains(n, "audio"):
		return KindSpeech
	// Gemini Live / 实时转写 / 同传：只支持 WebSocket bidiGenerateContent，
	// chat/completions 会 400（gemini-3.8-live、gemini-3.1-flash-live-preview、
	// gemini-3.5-live-translate-preview、gemini-robotics-er-2-streaming-preview）。
	case liveSegment.MatchString(n) || strings.Contains(n, "transcribe") || strings.Contains(n, "-streaming-preview"):
		return KindSpeech
	case strings.Contains(n, "embedding") || strings.Contains(n, "embed"):
		return KindEmbedding
	// 只支持 Interactions API（"This model only supports Interactions API"）或
	// 专用接口（aqa 不支持 generateContent）的模型。
	case strings.HasPrefix(base, "deep-research-") || strings.HasPrefix(base, "antigravity-preview") || aqaName.MatchString(n):
		return KindOther
	default:
		return KindChat
	}
}

// liveSegment 匹配独立的 live 段（-live、-live-preview），不误伤 deliver 之类的词。
var liveSegment = regexp.MustCompile(`(^|[-_ /])live($|[-_ ])`)

// aqaName 匹配 Google 的 aqa（Attributed Question Answering）模型。
var aqaName = regexp.MustCompile(`(^|/)aqa($|[-_ ])`)
