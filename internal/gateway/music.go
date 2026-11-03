package gateway

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"web2api/internal/model"
	"web2api/internal/provider"
)

// isMusicGenerationEndpoint 判断是否为音乐生成专用端点。
func isMusicGenerationEndpoint(method, path string) bool {
	if method != http.MethodPost {
		return false
	}
	p := strings.TrimRight(strings.TrimSpace(path), "/")
	return p == "/v1/music/generations" || p == "/v1/audio/generations"
}

type musicGenerationRequest struct {
	Model          string              `json:"model"`
	Prompt         string              `json:"prompt"`
	Input          any                 `json:"input"`
	Messages       []model.ChatMessage `json:"messages"`
	ResponseFormat any                 `json:"response_format"`
}

type musicGenerationItem struct {
	B64JSON  string `json:"b64_json,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
	URL      string `json:"url,omitempty"`
	Text     string `json:"text,omitempty"`
}

type musicGenerationResponse struct {
	Created int64                 `json:"created"`
	Model   string                `json:"model"`
	Data    []musicGenerationItem `json:"data"`
	Usage   model.TokenUsage      `json:"usage"`
}

// handleMusicGeneration 统一处理 POST /v1/music/generations 与 /v1/audio/generations：
//  1. 若目标上游原生实现了该透传端点且返回 2xx，直接透传返回；
//  2. 否则（AI Studio 原生模式、Custom 中的 Gemini API Key 等）自动转为内部 Chat 调用，
//     从响应中提取音频 data URI 与歌词文本，组装为标准音乐生成响应。
func (s *Server) handleMusicGeneration(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if r.Body == nil {
		writeError(w, http.StatusBadRequest, "读取请求体失败", "invalid_request_error", nil)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取请求体失败", "invalid_request_error", nil)
		return
	}
	var req musicGenerationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体必须是合法 JSON: "+err.Error(), "invalid_request_error", nil)
		return
	}
	rawModel := strings.TrimSpace(req.Model)
	modelID := strings.TrimPrefix(rawModel, "models/")
	if modelID == "" {
		writeError(w, http.StatusBadRequest,
			"Missing required parameter: 'model'. Please provide the model id in the request body (请在请求体的 JSON 中提供 model 字段，如 {\"model\":\"lyria-3.5\",\"prompt\":\"...\"}).",
			"invalid_request_error", nil)
		return
	}

	// 1) 尝试原生透传（第三方上游若直接实现了 /v1/music/generations 则优先原样返回）
	contentType := r.Header.Get("Content-Type")
	buf := newBufferedPassthroughWriter()
	handled, passErr := s.mgr.ServeModelPassthrough(r.Context(), modelID, r.Method, path, bytes.NewReader(body), contentType, buf)
	if !handled {
		if eng := s.mgr.PassthroughEngine(); eng != nil {
			handled = true
			passErr = eng.ServePassthrough(r.Context(), r.Method, path, bytes.NewReader(body), contentType, buf)
		}
	}
	if handled && passErr == nil && buf.status >= 200 && buf.status < 300 && buf.body.Len() > 0 {
		buf.flushTo(w)
		return
	}

	// 2) 自动转换为统一 Chat 调用（覆盖 AI Studio 原生模式、Gemini API Key 等）
	chatReq, ok := buildMusicChatRequest(req)
	if !ok {
		writeError(w, http.StatusBadRequest,
			"Missing required parameter: 'prompt' (请在请求体中提供 prompt、input 或 messages).",
			"invalid_request_error", nil)
		return
	}
	start := time.Now()
	res, chatErr := s.mgr.Chat(r.Context(), chatReq)
	if chatErr != nil {
		s.logger.Printf("[music] 请求失败 model=%q: %v", chatReq.Model, chatErr)
		s.recordChat(r, chatReq, res, false, chatErr, start, path)
		if provider.IsModelNotFound(chatErr) {
			writeError(w, http.StatusNotFound, chatErr.Error(), "invalid_request_error", "model_not_found")
			return
		}
		if provider.IsUnsupportedEndpoint(chatErr) {
			writeError(w, http.StatusBadRequest, chatErr.Error(), "invalid_request_error", "unsupported_endpoint")
			return
		}
		writeError(w, http.StatusBadGateway, chatErr.Error(), "api_error", nil)
		return
	}

	rec := s.recordChat(r, chatReq, res, false, nil, start, path)
	usage := resolveUsage(chatReq, res).Normalize()
	w.Header().Set("X-Web2api-Usage-Source", usageSource(rec.Estimated))
	w.Header().Set("X-Web2api-Multiplier", strconv.FormatFloat(rec.Multiplier, 'f', -1, 64))
	w.Header().Set("X-Web2api-Charged-Tokens", strconv.FormatInt(rec.ChargedTokens, 10))
	if rec.UserID > 0 && rec.ChargedTokens > 0 {
		w.Header().Set("X-Web2api-Balance", strconv.FormatInt(rec.BalanceAfter, 10))
	}

	items := extractMusicItemsFromText(res.Text)
	if wantsBinaryAudio(req.ResponseFormat, r.Header.Get("Accept")) {
		for _, item := range items {
			if item.B64JSON == "" {
				continue
			}
			rawAudio, decErr := base64.StdEncoding.DecodeString(item.B64JSON)
			if decErr == nil {
				mime := item.MIMEType
				if mime == "" {
					mime = "audio/mpeg"
				}
				w.Header().Set("Content-Type", mime)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(rawAudio)
				return
			}
		}
	}

	writeJSON(w, http.StatusOK, musicGenerationResponse{
		Created: time.Now().Unix(),
		Model:   rawModel,
		Data:    items,
		Usage:   usage,
	})
}

func buildMusicChatRequest(req musicGenerationRequest) (model.ChatRequest, bool) {
	if len(req.Messages) > 0 {
		return model.ChatRequest{
			Model:    req.Model,
			Messages: req.Messages,
		}, true
	}
	if p := strings.TrimSpace(req.Prompt); p != "" {
		return model.ChatRequest{
			Model:    req.Model,
			Messages: []model.ChatMessage{{Role: "user", Content: p}},
		}, true
	}
	switch in := req.Input.(type) {
	case string:
		if s := strings.TrimSpace(in); s != "" {
			return model.ChatRequest{
				Model:    req.Model,
				Messages: []model.ChatMessage{{Role: "user", Content: s}},
			}, true
		}
	case []any:
		var parts []model.ContentPart
		for _, raw := range in {
			switch v := raw.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					parts = append(parts, model.ContentPart{Type: "text", Text: v})
				}
			case map[string]any:
				typ, _ := v["type"].(string)
				switch typ {
				case "text", "input_text", "":
					if t, _ := v["text"].(string); strings.TrimSpace(t) != "" {
						parts = append(parts, model.ContentPart{Type: "text", Text: t})
					}
				case "image", "input_image":
					if data, _ := v["data"].(string); strings.TrimSpace(data) != "" {
						mime, _ := v["mime_type"].(string)
						if mime == "" {
							mime = "image/jpeg"
						}
						parts = append(parts, model.NewImagePart("data:"+mime+";base64,"+strings.TrimSpace(data)))
					} else if u, _ := v["url"].(string); strings.TrimSpace(u) != "" {
						parts = append(parts, model.NewImagePart(strings.TrimSpace(u)))
					}
				case "image_url":
					if img, ok := v["image_url"].(map[string]any); ok {
						if u, _ := img["url"].(string); strings.TrimSpace(u) != "" {
							parts = append(parts, model.NewImagePart(strings.TrimSpace(u)))
						}
					}
				}
			}
		}
		if len(parts) > 0 {
			return model.ChatRequest{
				Model:    req.Model,
				Messages: []model.ChatMessage{{Role: "user", Content: parts}},
			}, true
		}
	}
	return model.ChatRequest{}, false
}

var markdownMediaPattern = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)

func extractMusicItemsFromText(text string) []musicGenerationItem {
	matches := markdownMediaPattern.FindAllStringSubmatch(text, -1)
	cleanedText := strings.TrimSpace(markdownMediaPattern.ReplaceAllString(text, ""))

	var items []musicGenerationItem
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		target := strings.TrimSpace(m[1])
		if strings.HasPrefix(target, "data:") {
			rest := target[len("data:"):]
			if semi := strings.Index(rest, ";base64,"); semi >= 0 {
				mime := strings.TrimSpace(rest[:semi])
				b64 := strings.TrimSpace(rest[semi+len(";base64,"):])
				if mime == "" {
					mime = "audio/mpeg"
				}
				items = append(items, musicGenerationItem{
					B64JSON:  b64,
					MIMEType: mime,
					URL:      target,
				})
				continue
			}
		}
		if target != "" {
			items = append(items, musicGenerationItem{
				URL: target,
			})
		}
	}
	if len(items) == 0 {
		items = append(items, musicGenerationItem{
			Text: cleanedText,
		})
	} else if cleanedText != "" {
		items[0].Text = cleanedText
	}
	return items
}

func wantsBinaryAudio(respFormat any, acceptHeader string) bool {
	switch v := respFormat.(type) {
	case string:
		f := strings.ToLower(strings.TrimSpace(v))
		if f == "mp3" || f == "wav" || f == "audio" || f == "binary" || strings.HasPrefix(f, "audio/") {
			return true
		}
	case map[string]any:
		if t, _ := v["type"].(string); strings.EqualFold(strings.TrimSpace(t), "audio") {
			return true
		}
	}
	accept := strings.ToLower(acceptHeader)
	return strings.Contains(accept, "audio/") && !strings.Contains(accept, "application/json")
}

type bufferedPassthroughWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newBufferedPassthroughWriter() *bufferedPassthroughWriter {
	return &bufferedPassthroughWriter{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

func (b *bufferedPassthroughWriter) Header() http.Header {
	return b.header
}

func (b *bufferedPassthroughWriter) WriteHeader(status int) {
	b.status = status
}

func (b *bufferedPassthroughWriter) Write(p []byte) (int, error) {
	return b.body.Write(p)
}

func (b *bufferedPassthroughWriter) flushTo(w http.ResponseWriter) {
	for k, vals := range b.header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(b.status)
	_, _ = w.Write(b.body.Bytes())
}
