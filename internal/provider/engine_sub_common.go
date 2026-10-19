// engine_sub_common.go 订阅引擎（codex/antigravity/kiro/workbuddy）共享的
// 请求转换与账号池辅助逻辑。
package provider

import (
	"encoding/json"
	"strings"
	"time"

	"web2api/internal/model"
)

// subImageURLs 从 ChatMessage.Content（string 或分段数组）中提取图片 URL。
func subImageURLs(content any) []string {
	var out []string
	appendURL := func(raw string) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:image/") {
			out = append(out, raw)
		}
	}
	switch parts := content.(type) {
	case []model.ContentPart:
		for _, p := range parts {
			if p.Type == "image_url" && p.ImageURL != nil {
				appendURL(p.ImageURL.URL)
			}
		}
	case []any:
		for _, p := range parts {
			if pm, ok := p.(map[string]any); ok {
				if t, _ := pm["type"].(string); t != "image_url" {
					continue
				}
				if iu, ok := pm["image_url"].(map[string]any); ok {
					if u, _ := iu["url"].(string); u != "" {
						appendURL(u)
					}
				}
			}
		}
	case []map[string]any:
		for _, pm := range parts {
			if t, _ := pm["type"].(string); t != "image_url" {
				continue
			}
			if iu, ok := pm["image_url"].(map[string]any); ok {
				if u, _ := iu["url"].(string); u != "" {
					appendURL(u)
				}
			}
		}
	}
	return out
}

// subSystemText 提取全部 system 消息文本。
func subSystemText(req model.ChatRequest) string {
	var parts []string
	for _, m := range req.Messages {
		if strings.EqualFold(m.Role, "system") {
			if t := m.Text(); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// subCooldown 判断账号冷却是否仍在生效。
func subCooldownActive(until time.Time) bool {
	return time.Now().Before(until)
}

// subResultUsage 组装 TokenUsage；reasoningTokens 计入 completion（上游口径已含），
// Estimated=false 表示上游权威返回。
func subResultUsage(prompt, completion, reasoning int64) *model.TokenUsage {
	if prompt <= 0 && completion <= 0 {
		return nil // 上游无用量 → 网关按文本估算
	}
	return &model.TokenUsage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
		ReasoningTokens:  reasoning,
	}
}

// marshalCompact 序列化为紧凑 JSON（凭据落库）。
func marshalCompact(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
