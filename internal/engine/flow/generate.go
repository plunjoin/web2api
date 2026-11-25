package flow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"web2api/internal/engine"
	"web2api/internal/model"
)

// generationImage 是一次图片生成的中间结果。
type generationImage struct {
	ID  string
	URL string
}

// generateImage 完成一次 Flow 文生图（含可选的 2K/4K 放大）。
func (e *Engine) generateImage(ctx context.Context, account *flowAccount, item imageModel, req model.ChatRequest) (*model.ChatResult, error) {
	prompt := chatPrompt(req)
	if prompt == "" {
		return nil, errors.New("flow: 缺少用户提示词")
	}
	aspect, err := imageAspectFromModel(item)
	if err != nil {
		return nil, err
	}
	if err := applyImageRequest(&req, item, &aspect); err != nil {
		return nil, err
	}

	boot, cookies, err := e.withSession(ctx, account, actionImageGeneration)
	if err != nil {
		return nil, err
	}
	payload, err := account.client.Call(ctx, boot, rpcImageGenerate,
		imageGenerationArgument(boot.ProjectID, prompt, item.ModelName, aspect, recaptchaToken(account), boot.SessionID, nil),
		cookies, "/project/"+boot.ProjectID)
	if err != nil {
		return nil, e.handleGenerationError(account, err)
	}
	images := parseImagePayload(payload)
	if len(images) == 0 {
		return nil, errors.New("flow: 图片生成响应没有可下载的图片")
	}

	if item.Upsample != "" {
		token := recaptchaToken(account)
		upsampled, upErr := account.client.Call(ctx, boot, rpcImageUpsample,
			[]any{strings.TrimRight(images[0].ID, "/"), upsampleResolution(item.Upsample), projectContext(boot.ProjectID, token)},
			cookies, "/project/"+boot.ProjectID)
		if upErr != nil {
			return nil, e.handleGenerationError(account, upErr)
		}
		if next := parseImagePayload(upsampled); len(next) > 0 {
			images = next
		} else if mediaURL := findContentURL(upsampled, "image"); mediaURL != "" {
			images[0] = generationImage{URL: mediaURL}
		} else if encoded := findBase64Payload(upsampled); encoded != "" {
			data, decodeErr := base64.StdEncoding.DecodeString(encoded)
			if decodeErr != nil {
				return nil, fmt.Errorf("flow: 图片放大结果解码失败: %w", decodeErr)
			}
			return &model.ChatResult{Text: "data:image/png;base64," + base64.StdEncoding.EncodeToString(data), Engine: e.name, Model: req.Model, ActualModel: item.ID}, nil
		}
	}

	image := images[0]
	if image.URL == "" && image.ID != "" {
		mediaPayload, mediaErr := account.client.Call(ctx, boot, rpcGetMedia,
			[]any{image.ID}, cookies, "/project/"+boot.ProjectID+"/edit/"+image.ID)
		if mediaErr != nil {
			return nil, e.handleGenerationError(account, mediaErr)
		}
		image.URL = findContentURL(mediaPayload, "image")
	}
	if image.URL == "" {
		return nil, errors.New("flow: 图片响应没有下载地址")
	}
	data, mimeType, err := e.fetchMediaBytes(ctx, account, image.URL)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	if mimeType == "" {
		mimeType = "image/png"
	}
	return &model.ChatResult{
		Text:        "data:" + mimeType + ";base64," + encoded,
		Engine:      e.name,
		Model:       req.Model,
		ActualModel: item.ID,
	}, nil
}

// recaptchaToken 读取当前账号会话里的 token；导出给视频路径共用。
func recaptchaToken(account *flowAccount) string {
	account.mu.Lock()
	defer account.mu.Unlock()
	return account.recaptcha
}

// handleGenerationError 把上游风控错误转成账号冷却。
func (e *Engine) handleGenerationError(account *flowAccount, err error) error {
	code := ""
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		code = rpcErr.ErrorCode()
	}
	switch {
	case strings.Contains(code, "UNUSUAL_ACTIVITY"), strings.Contains(err.Error(), "UNUSUAL_ACTIVITY"):
		e.markCooldown(account, "Flow 风控：reCAPTCHA 环境不一致", 5*time.Minute)
	case strings.Contains(code, "MODEL_ACCESS_DENIED"):
		e.markCooldown(account, "Flow 账号无该模型权限", time.Hour)
	case strings.Contains(code, "RATE"), strings.Contains(code, "QUOTA"):
		e.markCooldown(account, "Flow 额度或限流", 15*time.Minute)
	}
	e.setErr(err)
	return err
}

// imageAspectFromModel 从目录条目取宽高比（模型 ID 与上游枚举解耦）。
func imageAspectFromModel(item imageModel) (int, error) {
	return imageAspectRatio(item.AspectRatio)
}

// applyImageRequest 允许请求参数覆盖模型的默认宽高比。
func applyImageRequest(req *model.ChatRequest, item imageModel, aspect *int) error {
	raw := strings.TrimSpace(req.ImageSize)
	if raw == "" && len(req.Extra) > 0 {
		var extra struct {
			ImageSize   string `json:"image_size"`
			AspectRatio string `json:"aspect_ratio"`
			Size        string `json:"size"`
		}
		if json.Unmarshal(req.Extra, &extra) == nil {
			raw = extra.ImageSize
			if raw == "" {
				raw = extra.AspectRatio
			}
			if raw == "" {
				raw = extra.Size
			}
		}
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "", "auto":
		return nil
	case "1k", "2k", "4k":
		if !strings.HasSuffix(item.ID, "-"+raw) {
			return fmt.Errorf("flow: 模型 %s 不支持分辨率 %s", item.ID, raw)
		}
		return nil
	default:
		mapped, err := imageAspectRatio(raw)
		if err != nil {
			return err
		}
		*aspect = mapped
		return nil
	}
}

// upsampleResolution 放大目标转上游整数。
func upsampleResolution(target string) int {
	if strings.Contains(strings.ToUpper(target), "4K") {
		return 2
	}
	return 1
}

// parseImagePayload 解析 ogiZ0b / SPrCad 的图片负载。
func parseImagePayload(payload json.RawMessage) []generationImage {
	var outer []json.RawMessage
	if json.Unmarshal(payload, &outer) != nil || len(outer) == 0 {
		return nil
	}
	var items []json.RawMessage
	if json.Unmarshal(outer[0], &items) != nil {
		return nil
	}
	out := make([]generationImage, 0, len(items))
	for _, raw := range items {
		var fields []json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || len(fields) == 0 {
			continue
		}
		var id string
		_ = json.Unmarshal(fields[0], &id)
		url := findContentURL(raw, "image")
		out = append(out, generationImage{ID: strings.TrimSpace(id), URL: url})
	}
	return out
}

// findBase64Payload 在负载里找长 base64 字符串（SPrCad 可能直接返回图片字节）。
func findBase64Payload(payload json.RawMessage) string {
	var walk func(json.RawMessage) string
	walk = func(raw json.RawMessage) string {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			if len(text) >= 512 && !strings.HasPrefix(text, "http") {
				return text
			}
			return ""
		}
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) == nil {
			for _, item := range list {
				if found := walk(item); found != "" {
					return found
				}
			}
			return ""
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) == nil {
			for _, item := range object {
				if found := walk(item); found != "" {
					return found
				}
			}
		}
		return ""
	}
	return walk(payload)
}

// fetchMediaBytes 下载 flow-content.google 的签名媒体地址并缓存到内存。
func (e *Engine) fetchMediaBytes(ctx context.Context, account *flowAccount, mediaURL string) ([]byte, string, error) {
	body, mimeType, err := e.fetchMedia(ctx, account, mediaURL)
	if err != nil {
		return nil, "", err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, 128<<20))
	if err != nil {
		return nil, "", err
	}
	return data, mimeType, nil
}

// fetchMedia 下载 flow-content.google 的签名媒体地址，返回需要调用方关闭的流。
func (e *Engine) fetchMedia(ctx context.Context, account *flowAccount, mediaURL string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := account.client.HTTPClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("下载 Flow 媒体失败: %w", err)
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, "", fmt.Errorf("下载 Flow 媒体失败: HTTP %d", resp.StatusCode)
	}
	mimeType := resp.Header.Get("Content-Type")
	if parsed, _, err := mime.ParseMediaType(mimeType); err == nil {
		mimeType = parsed
	}
	if mimeType == "" {
		if ext := strings.ToLower(path.Ext(safeURLPath(mediaURL))); ext != "" {
			mimeType = mime.TypeByExtension(ext)
		}
	}
	return resp.Body, mimeType, nil
}

// safeURLPath 取 URL 路径，失败时返回空串。
func safeURLPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Path
}

// mediaName 生成下载文件名。
func mediaName(mediaURL, fallback, mimeType string) string {
	name := path.Base(safeURLPath(mediaURL))
	if name != "" && name != "." && name != "/" {
		return name
	}
	ext := ".bin"
	if extensions, _ := mime.ExtensionsByType(mimeType); len(extensions) > 0 {
		ext = extensions[0]
	}
	return fallback + ext
}

// Unused guard: keep engine import used if future helpers move around.
var _ = engine.PolicyKindImage
