package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"web2api/internal/model"
)

// isImageModel 判断模型是否为图片生成模型（Gemini 图像模型、gpt-image 等）。
func isImageModel(modelID string) bool {
	return model.ClassifyKind(modelID, "", nil) == model.KindImage
}

// chatImage 把图片模型的对话请求转成上游的图片生成协议，并把产出的图片
// 编码成 Markdown data URI 放回文本，保持下游只依赖 chat/completions。
//
// Gemini 原生协议优先：Google 的 OpenAI 兼容层对新的图片模型只回
// "Unhandled generated data mime type: image/jpeg"，拿不到图片。原生
// :generateContent 能直接返回 inlineData。其他供应商（OpenRouter 等）
// 则在 message.images 里返回图片，由 chatOpenAI 统一转换。
func (o *OpenAI) chatImage(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	var nativeErr error
	if isGeminiOpenAIBase(o.baseURL) {
		res, err := o.imageViaGeminiGenerateContent(ctx, req)
		if err == nil && res != nil && strings.TrimSpace(res.Text) != "" {
			o.MarkReady()
			if sink != nil {
				if err := sink.Text(res.Text); err != nil {
					return nil, err
				}
			}
			return res, nil
		}
		nativeErr = err
	}

	// 原生协议不适用或失败时回退到 OpenAI 兼容 chat。
	res, err := o.chatOpenAI(ctx, req)
	if err != nil {
		if nativeErr != nil {
			return nil, fmt.Errorf("原生图片生成失败: %v; OpenAI 兼容接口失败: %w", nativeErr, err)
		}
		return nil, err
	}
	if sink != nil && res != nil && res.Text != "" {
		o.MarkReady()
		if err := sink.Text(res.Text); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// imageViaGeminiGenerateContent 调用 Gemini /v1beta/models/{model}:generateContent
// 生成图片。responseModalities 先只要求 IMAGE，部分模型（如 gemini-2.5-flash-image）
// 要求同时要 TEXT，收到 400 时再换一组重试。
func (o *OpenAI) imageViaGeminiGenerateContent(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	bareModel := strings.TrimPrefix(strings.TrimSpace(req.Model), "models/")
	if bareModel == "" {
		return nil, fmt.Errorf("model 不能为空")
	}
	url := geminiAPIBase(o.baseURL) + "/models/" + bareModel + ":generateContent"
	contents, sysText := buildGeminiContents(req.Messages, "generate an image")

	modalitiesList := [][]string{
		{"IMAGE"},
		{"TEXT", "IMAGE"},
	}
	var lastErr error
	for _, modalities := range modalitiesList {
		genCfg := map[string]any{
			"responseModalities": modalities,
		}
		if size := strings.TrimSpace(req.ImageSize); size != "" {
			genCfg["imageConfig"] = map[string]any{"imageSize": size}
		}
		if req.Temperature != nil {
			genCfg["temperature"] = *req.Temperature
		}
		if req.TopP != nil {
			genCfg["topP"] = *req.TopP
		}
		payload := map[string]any{
			"contents":         contents,
			"generationConfig": genCfg,
		}
		if sysText != "" {
			payload["systemInstruction"] = map[string]any{
				"parts": []map[string]any{{"text": sysText}},
			}
		}
		data, status, _, err := o.doGeminiJSON(ctx, url, payload)
		if err != nil {
			return nil, err
		}
		if status == http.StatusOK {
			res, parseErr := parseGeminiGenerateContentImage(data, o.name, req.Model)
			if parseErr == nil {
				return res, nil
			}
			lastErr = parseErr
			break
		}
		lastErr = fmt.Errorf("上游返回 HTTP %d: %s", status, truncate(data))
		msg := strings.ToLower(string(data))
		if status == http.StatusBadRequest && (strings.Contains(msg, "modality") || strings.Contains(msg, "modalities")) {
			continue
		}
		break
	}
	return nil, lastErr
}

// parseGeminiGenerateContentImage 解析原生 generateContent 响应里的图片分段。
func parseGeminiGenerateContentImage(data []byte, engineName, modelID string) (*model.ChatResult, error) {
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text       string `json:"text"`
					InlineData *struct {
						MIMEType string `json:"mimeType"`
						AltMIME  string `json:"mime_type"`
						Data     string `json:"data"`
					} `json:"inlineData"`
					AltInlineData *struct {
						MIMEType string `json:"mimeType"`
						AltMIME  string `json:"mime_type"`
						Data     string `json:"data"`
					} `json:"inline_data"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount     int64 `json:"promptTokenCount"`
			CandidatesTokenCount int64 `json:"candidatesTokenCount"`
			TotalTokenCount      int64 `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析 Gemini 响应失败: %w", err)
	}
	var full strings.Builder
	images := 0
	for _, cand := range resp.Candidates {
		for _, part := range cand.Content.Parts {
			if t := strings.TrimSpace(part.Text); t != "" {
				if full.Len() > 0 {
					full.WriteString("\n")
				}
				full.WriteString(t)
			}
			inline := part.InlineData
			if inline == nil {
				inline = part.AltInlineData
			}
			if inline == nil || strings.TrimSpace(inline.Data) == "" {
				continue
			}
			mime := strings.TrimSpace(inline.MIMEType)
			if mime == "" {
				mime = strings.TrimSpace(inline.AltMIME)
			}
			if !strings.HasPrefix(strings.ToLower(mime), "image/") {
				mime = sniffImageMIME(inline.Data)
			}
			full.WriteString("\n![media](data:" + mime + ";base64," + strings.TrimSpace(inline.Data) + ")\n")
			images++
		}
	}
	if images == 0 {
		return nil, fmt.Errorf("Gemini 响应未包含图片")
	}
	res := &model.ChatResult{
		Text:         full.String(),
		Engine:       engineName,
		Model:        modelID,
		FinishReason: "stop",
	}
	if resp.UsageMetadata != nil && resp.UsageMetadata.TotalTokenCount > 0 {
		u := model.TokenUsage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}.Normalize()
		res.Usage = &u
	}
	return res, nil
}

// messageImagesMarkdown 把上游响应 message.images 里的图片转成 Markdown。
// 兼容 image_url.url（data URI 或远程地址）与 b64_json 两种写法。
func messageImagesMarkdown(images []model.MessageImage) string {
	var b strings.Builder
	for _, img := range images {
		link := ""
		if img.ImageURL != nil {
			link = strings.TrimSpace(img.ImageURL.URL)
		}
		if link == "" && strings.TrimSpace(img.B64JSON) != "" {
			mime := strings.TrimSpace(img.MIMEType)
			if !strings.HasPrefix(strings.ToLower(mime), "image/") {
				mime = sniffImageMIME(img.B64JSON)
			}
			link = "data:" + mime + ";base64," + strings.TrimSpace(img.B64JSON)
		}
		if !isImageLink(link) {
			continue
		}
		b.WriteString("\n![media](" + link + ")\n")
	}
	return b.String()
}

// isImageLink 只放行图片 data URI 与 http(s) 地址，避免把任意文本拼进 Markdown。
func isImageLink(link string) bool {
	lower := strings.ToLower(link)
	if strings.HasPrefix(lower, "data:image/") {
		return strings.Contains(lower, ";base64,")
	}
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// sniffImageMIME 从 base64 前缀猜图片类型；猜不出时按 PNG 处理。
func sniffImageMIME(b64 string) string {
	b64 = strings.TrimSpace(b64)
	switch {
	case strings.HasPrefix(b64, "iVBORw0KGgo"):
		return "image/png"
	case strings.HasPrefix(b64, "/9j/"):
		return "image/jpeg"
	case strings.HasPrefix(b64, "R0lGOD"):
		return "image/gif"
	case strings.HasPrefix(b64, "UklGR"):
		return "image/webp"
	default:
		return "image/png"
	}
}
