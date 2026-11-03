package upstream

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"web2api/internal/model"
)

// isMusicModel 判断模型是否为音乐生成模型（Lyria 等）。
func isMusicModel(modelID string) bool {
	return model.ClassifyKind(modelID, "", nil) == model.KindMusic
}

// isGeminiOpenAIBase 判断 base_url 是否指向 Gemini 的 OpenAI 兼容层或原生地址。
func isGeminiOpenAIBase(baseURL string) bool {
	u := strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	return strings.Contains(u, "generativelanguage.googleapis.com") ||
		strings.HasSuffix(u, "/v1beta/openai") ||
		strings.HasSuffix(u, "/v1alpha/openai") ||
		strings.HasSuffix(u, "/v1beta")
}

// geminiAPIBase 从 OpenAI 兼容 base_url 推导 Gemini 原生 REST 根路径（…/v1beta）。
func geminiAPIBase(baseURL string) string {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(u)
	if strings.HasSuffix(lower, "/openai") {
		u = strings.TrimRight(u[:len(u)-len("/openai")], "/")
		lower = strings.ToLower(u)
	}
	if strings.HasSuffix(lower, "/v1beta") || strings.HasSuffix(lower, "/v1alpha") {
		return u
	}
	if strings.HasSuffix(lower, "/v1") {
		return u[:len(u)-len("/v1")] + "/v1beta"
	}
	return u + "/v1beta"
}

// chatMusic 将音乐模型的对话请求转换为上游支持的音乐生成协议（Gemini generateContent /
// Interactions 或 /v1/music/generations），并将产出的音频编码为 Markdown data URI 返回。
func (o *OpenAI) chatMusic(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	type attemptFunc func(context.Context, model.ChatRequest) (*model.ChatResult, error)
	var attempts []attemptFunc
	if isGeminiOpenAIBase(o.baseURL) {
		attempts = []attemptFunc{
			o.musicViaGeminiGenerateContent,
			o.musicViaGeminiInteractions,
			o.musicViaEndpoint,
		}
	} else {
		attempts = []attemptFunc{
			o.musicViaEndpoint,
			o.musicViaGeminiGenerateContent,
			o.musicViaGeminiInteractions,
		}
	}

	var lastErr error
	for _, fn := range attempts {
		res, err := fn(ctx, req)
		if err == nil && res != nil && strings.TrimSpace(res.Text) != "" {
			o.MarkReady()
			if sink != nil {
				if err := sink.Text(res.Text); err != nil {
					return nil, err
				}
			}
			return res, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("上游音乐生成未返回有效内容")
}

// musicViaGeminiGenerateContent 调用 Gemini /v1beta/models/{model}:generateContent 生成音频。
func (o *OpenAI) musicViaGeminiGenerateContent(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	bareModel := strings.TrimPrefix(strings.TrimSpace(req.Model), "models/")
	if bareModel == "" {
		return nil, fmt.Errorf("model 不能为空")
	}
	url := geminiAPIBase(o.baseURL) + "/models/" + bareModel + ":generateContent"
	contents, sysText := buildGeminiMusicContents(req.Messages)

	modalitiesList := [][]string{
		{"AUDIO", "TEXT"},
		{"AUDIO"},
	}
	var lastErr error
	for _, modalities := range modalitiesList {
		genCfg := map[string]any{
			"responseModalities": modalities,
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
			res, parseErr := parseGeminiGenerateContentMusic(data, o.name, req.Model)
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

// musicViaGeminiInteractions 调用 Gemini /v1beta/interactions 生成音乐（Lyria 官方 Interactions API）。
func (o *OpenAI) musicViaGeminiInteractions(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	bareModel := strings.TrimPrefix(strings.TrimSpace(req.Model), "models/")
	if bareModel == "" {
		return nil, fmt.Errorf("model 不能为空")
	}
	url := geminiAPIBase(o.baseURL) + "/interactions"
	input := buildGeminiInteractionInput(req.Messages)
	payload := map[string]any{
		"model": bareModel,
		"input": input,
	}
	data, status, _, err := o.doGeminiJSON(ctx, url, payload)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("上游返回 HTTP %d: %s", status, truncate(data))
	}
	return parseGeminiInteractionsMusic(data, o.name, req.Model)
}

// musicViaEndpoint 调用 OpenAI 风格 /v1/music/generations 端点生成音乐。
func (o *OpenAI) musicViaEndpoint(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	prompt := extractMusicPrompt(req.Messages)
	if prompt == "" {
		prompt = "music"
	}
	payload := map[string]any{
		"model":  req.Model,
		"prompt": prompt,
	}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint("/v1/music/generations"), bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("上游返回 HTTP %d: %s", resp.StatusCode, truncate(data))
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.HasPrefix(ct, "audio/") {
		mime := strings.TrimSpace(strings.Split(ct, ";")[0])
		b64 := base64.StdEncoding.EncodeToString(data)
		return &model.ChatResult{
			Text:         "\n![media](data:" + mime + ";base64," + b64 + ")\n",
			Engine:       o.name,
			Model:        req.Model,
			FinishReason: "stop",
		}, nil
	}
	return parseMusicGenerationJSON(data, o.name, req.Model)
}

func (o *OpenAI) doGeminiJSON(ctx context.Context, url string, body any) ([]byte, int, string, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("X-Goog-Api-Key", o.apiKey)
		// Google 官方原生端点如果收到非 OAuth 的 Authorization: Bearer AIza... 会 401，
		// 因此仅在非 googleapis.com 的第三方反代上同时附带 Bearer。
		if !strings.Contains(strings.ToLower(url), "googleapis.com") {
			req.Header.Set("Authorization", "Bearer "+o.apiKey)
		}
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, resp.StatusCode, "", err
	}
	return data, resp.StatusCode, resp.Header.Get("Content-Type"), nil
}

func buildGeminiMusicContents(messages []model.ChatMessage) ([]map[string]any, string) {
	var sysParts []string
	var contents []map[string]any
	for _, m := range messages {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role == "system" || role == "developer" {
			if t := strings.TrimSpace(m.Text()); t != "" {
				sysParts = append(sysParts, t)
			}
			continue
		}
		geminiRole := "user"
		if role == "assistant" || role == "model" {
			geminiRole = "model"
		}
		parts := extractGeminiParts(m)
		if len(parts) == 0 {
			continue
		}
		contents = append(contents, map[string]any{
			"role":  geminiRole,
			"parts": parts,
		})
	}
	if len(contents) == 0 {
		contents = []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "generate music"}}},
		}
	}
	return contents, strings.Join(sysParts, "\n")
}

func extractGeminiParts(m model.ChatMessage) []map[string]any {
	var parts []map[string]any
	switch c := m.Content.(type) {
	case string:
		if strings.TrimSpace(c) != "" {
			parts = append(parts, map[string]any{"text": c})
		}
	case []model.ContentPart:
		for _, p := range c {
			switch p.Type {
			case "text":
				if strings.TrimSpace(p.Text) != "" {
					parts = append(parts, map[string]any{"text": p.Text})
				}
			case "image_url":
				if p.ImageURL != nil {
					if mime, b64, ok := parseDataURI(p.ImageURL.URL); ok {
						parts = append(parts, map[string]any{
							"inlineData": map[string]any{"mimeType": mime, "data": b64},
						})
					}
				}
			}
		}
	case []any:
		for _, raw := range c {
			pm, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			switch pm["type"] {
			case "text":
				if s, ok := pm["text"].(string); ok && strings.TrimSpace(s) != "" {
					parts = append(parts, map[string]any{"text": s})
				}
			case "image_url":
				if img, ok := pm["image_url"].(map[string]any); ok {
					if u, ok := img["url"].(string); ok {
						if mime, b64, ok := parseDataURI(u); ok {
							parts = append(parts, map[string]any{
								"inlineData": map[string]any{"mimeType": mime, "data": b64},
							})
						}
					}
				}
			}
		}
	}
	return parts
}

func buildGeminiInteractionInput(messages []model.ChatMessage) []map[string]any {
	var items []map[string]any
	for _, m := range messages {
		switch c := m.Content.(type) {
		case string:
			if strings.TrimSpace(c) != "" {
				items = append(items, map[string]any{"type": "text", "text": c})
			}
		case []model.ContentPart:
			for _, p := range c {
				switch p.Type {
				case "text":
					if strings.TrimSpace(p.Text) != "" {
						items = append(items, map[string]any{"type": "text", "text": p.Text})
					}
				case "image_url":
					if p.ImageURL != nil {
						if mime, b64, ok := parseDataURI(p.ImageURL.URL); ok {
							items = append(items, map[string]any{"type": "image", "mime_type": mime, "data": b64})
						}
					}
				}
			}
		default:
			if t := strings.TrimSpace(m.Text()); t != "" {
				items = append(items, map[string]any{"type": "text", "text": t})
			}
		}
	}
	if len(items) == 0 {
		items = []map[string]any{{"type": "text", "text": "generate music"}}
	}
	return items
}

func extractMusicPrompt(messages []model.ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "user") {
			if t := strings.TrimSpace(messages[i].Text()); t != "" {
				return t
			}
		}
	}
	var all []string
	for _, m := range messages {
		if t := strings.TrimSpace(m.Text()); t != "" {
			all = append(all, t)
		}
	}
	return strings.Join(all, "\n")
}

func parseDataURI(uri string) (mime, b64 string, ok bool) {
	uri = strings.TrimSpace(uri)
	if !strings.HasPrefix(uri, "data:") {
		return "", "", false
	}
	rest := uri[len("data:"):]
	semi := strings.Index(rest, ";base64,")
	if semi < 0 {
		return "", "", false
	}
	mime = strings.TrimSpace(rest[:semi])
	b64 = strings.TrimSpace(rest[semi+len(";base64,"):])
	if mime == "" || b64 == "" {
		return "", "", false
	}
	return mime, b64, true
}

func parseGeminiGenerateContentMusic(data []byte, engineName, modelID string) (*model.ChatResult, error) {
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
	for _, cand := range resp.Candidates {
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				full.WriteString(part.Text)
			}
			inline := part.InlineData
			if inline == nil {
				inline = part.AltInlineData
			}
			if inline != nil && strings.TrimSpace(inline.Data) != "" {
				mime := strings.TrimSpace(inline.MIMEType)
				if mime == "" {
					mime = strings.TrimSpace(inline.AltMIME)
				}
				if mime == "" {
					mime = "audio/mpeg"
				}
				full.WriteString("\n![media](data:" + mime + ";base64," + strings.TrimSpace(inline.Data) + ")\n")
			}
		}
	}
	if strings.TrimSpace(full.String()) == "" {
		return nil, fmt.Errorf("Gemini 响应未包含音频或文本内容")
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

func parseGeminiInteractionsMusic(data []byte, engineName, modelID string) (*model.ChatResult, error) {
	var resp struct {
		Steps []struct {
			Type    string `json:"type"`
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				MIMEType string `json:"mime_type"`
				Data     string `json:"data"`
			} `json:"content"`
		} `json:"steps"`
		OutputText  string `json:"output_text"`
		OutputAudio *struct {
			MIMEType string `json:"mime_type"`
			Data     string `json:"data"`
		} `json:"output_audio"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析 Gemini Interactions 响应失败: %w", err)
	}
	var full strings.Builder
	hasAudio := false
	for _, step := range resp.Steps {
		if step.Type != "" && step.Type != "model_output" {
			continue
		}
		for _, c := range step.Content {
			switch c.Type {
			case "text":
				if c.Text != "" {
					full.WriteString(c.Text)
				}
			case "audio":
				if strings.TrimSpace(c.Data) != "" {
					mime := strings.TrimSpace(c.MIMEType)
					if mime == "" {
						mime = "audio/mpeg"
					}
					full.WriteString("\n![media](data:" + mime + ";base64," + strings.TrimSpace(c.Data) + ")\n")
					hasAudio = true
				}
			}
		}
	}
	if full.Len() == 0 && resp.OutputText != "" {
		full.WriteString(resp.OutputText)
	}
	if !hasAudio && resp.OutputAudio != nil && strings.TrimSpace(resp.OutputAudio.Data) != "" {
		mime := strings.TrimSpace(resp.OutputAudio.MIMEType)
		if mime == "" {
			mime = "audio/mpeg"
		}
		full.WriteString("\n![media](data:" + mime + ";base64," + strings.TrimSpace(resp.OutputAudio.Data) + ")\n")
	}
	if strings.TrimSpace(full.String()) == "" {
		return nil, fmt.Errorf("Gemini Interactions 响应未包含音频内容")
	}
	return &model.ChatResult{
		Text:         full.String(),
		Engine:       engineName,
		Model:        modelID,
		FinishReason: "stop",
	}, nil
}

func parseMusicGenerationJSON(data []byte, engineName, modelID string) (*model.ChatResult, error) {
	var resp struct {
		Text   string `json:"text"`
		Lyrics string `json:"lyrics"`
		Audio  string `json:"audio"`
		B64    string `json:"b64_json"`
		URL    string `json:"url"`
		MIME   string `json:"mime_type"`
		Data   []struct {
			B64    string `json:"b64_json"`
			Audio  string `json:"audio"`
			Data   string `json:"data"`
			URL    string `json:"url"`
			MIME   string `json:"mime_type"`
			Text   string `json:"text"`
			Lyrics string `json:"lyrics"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析音乐生成响应失败: %w", err)
	}
	var full strings.Builder
	appendText := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		if full.Len() > 0 {
			full.WriteString("\n")
		}
		full.WriteString(t)
	}
	appendMedia := func(b64, urlStr, mime string) {
		b64 = strings.TrimSpace(b64)
		urlStr = strings.TrimSpace(urlStr)
		mime = strings.TrimSpace(mime)
		if mime == "" {
			mime = "audio/mpeg"
		}
		if b64 != "" {
			full.WriteString("\n![media](data:" + mime + ";base64," + b64 + ")\n")
		} else if urlStr != "" {
			full.WriteString("\n![media](" + urlStr + ")\n")
		}
	}

	appendText(resp.Text)
	appendText(resp.Lyrics)
	for _, item := range resp.Data {
		appendText(item.Text)
		appendText(item.Lyrics)
		b64 := item.B64
		if b64 == "" {
			b64 = item.Audio
		}
		if b64 == "" {
			b64 = item.Data
		}
		appendMedia(b64, item.URL, item.MIME)
	}
	topB64 := resp.B64
	if topB64 == "" {
		topB64 = resp.Audio
	}
	if len(resp.Data) == 0 {
		appendMedia(topB64, resp.URL, resp.MIME)
	}
	if strings.TrimSpace(full.String()) == "" {
		return nil, fmt.Errorf("音乐生成响应未包含音频数据")
	}
	return &model.ChatResult{
		Text:         full.String(),
		Engine:       engineName,
		Model:        modelID,
		FinishReason: "stop",
	}, nil
}
