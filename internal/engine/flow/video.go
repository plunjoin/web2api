// video.go Flow 原生视频提交与状态解析。
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
	"strings"

	"web2api/internal/engine"
)

// videoStatus 是从 jwpduf 响应里解出的任务状态。
type videoStatus struct {
	Done    bool
	MediaID string
}

// withVideoSession 完成 bootstrap、打码与项目初始化，并把 token 交给提交阶段。
func (e *Engine) withVideoSession(ctx context.Context, account *flowAccount) (Bootstrap, string, string, error) {
	boot, cookies, err := e.withSession(ctx, account, actionVideoGeneration)
	if err != nil {
		return Bootstrap{}, "", "", err
	}
	return boot, cookies, recaptchaToken(account), nil
}

// submitVideo 按模型的生成模式选择 Flow 前端 RPC。
func (e *Engine) submitVideo(ctx context.Context, account *flowAccount, boot Bootstrap, cookies, token string, item videoModel, request engine.VideoRequest) (string, error) {
	aspect := videoAspectRatio(item.AspectRatio)
	resolution := videoResolution(request.Resolution)
	sourcePath := "/project/" + boot.ProjectID
	modelKey := strings.TrimSpace(item.ModelKey)

	switch item.VideoType {
	case "t2v", "":
		payload, err := account.client.Call(ctx, boot, rpcVideoText,
			videoArgument(videoTextRequest(request.Prompt, modelKey, aspect, boot.SessionID, resolution), boot.ProjectID, token),
			cookies, sourcePath)
		if err != nil {
			return "", e.handleGenerationError(account, err)
		}
		return parseVideoOperationID(payload), nil

	case "i2v":
		mediaIDs, err := e.uploadReferenceImages(ctx, account, boot, cookies, token, request.ImageURLs)
		if err != nil {
			return "", err
		}
		if len(mediaIDs) == 2 {
			payload, err := account.client.Call(ctx, boot, rpcVideoStartEnd,
				videoArgument(videoStartEndRequest(request.Prompt, modelKey, aspect, boot.SessionID, mediaIDs[0], mediaIDs[1]), boot.ProjectID, token),
				cookies, sourcePath)
			if err != nil {
				return "", e.handleGenerationError(account, err)
			}
			return parseVideoOperationID(payload), nil
		}
		if len(mediaIDs) == 1 {
			// 上游单帧模型键带 _fl 后缀；这里只在新键缺失时回退，避免改变
			// 已经验证过的模型键映射。
			key := modelKey
			if !strings.HasSuffix(key, "_fl") {
				key += "_fl"
			}
			payload, err := account.client.Call(ctx, boot, rpcVideoStart,
				videoArgument(videoStartRequest(request.Prompt, key, aspect, boot.SessionID, mediaIDs[0], resolution), boot.ProjectID, token),
				cookies, sourcePath)
			if err != nil {
				return "", e.handleGenerationError(account, err)
			}
			return parseVideoOperationID(payload), nil
		}
		return "", fmt.Errorf("flow: %s 需要 1 或 2 张参考图，当前 %d 张", item.ID, len(mediaIDs))

	case "r2v":
		mediaIDs, err := e.uploadReferenceImages(ctx, account, boot, cookies, token, request.ImageURLs)
		if err != nil {
			return "", err
		}
		payload, err := account.client.Call(ctx, boot, rpcVideoRefs,
			videoArgument(videoReferencesRequest(request.Prompt, modelKey, aspect, boot.SessionID, mediaIDs, resolution), boot.ProjectID, token),
			cookies, sourcePath)
		if err != nil {
			return "", e.handleGenerationError(account, err)
		}
		return parseVideoOperationID(payload), nil

	case "omni":
		mediaIDs, err := e.uploadReferenceImages(ctx, account, boot, cookies, token, request.ImageURLs)
		if err != nil {
			return "", err
		}
		if len(mediaIDs) == 0 {
			payload, err := account.client.Call(ctx, boot, rpcVideoText,
				videoArgument(videoTextRequest(request.Prompt, modelKey, aspect, boot.SessionID, resolution), boot.ProjectID, token),
				cookies, sourcePath)
			if err != nil {
				return "", e.handleGenerationError(account, err)
			}
			return parseVideoOperationID(payload), nil
		}
		key := strings.TrimSpace(item.ReferenceKey)
		if key == "" {
			key = modelKey
		}
		payload, err := account.client.Call(ctx, boot, rpcVideoRefs,
			videoArgument(videoReferencesRequest(request.Prompt, key, aspect, boot.SessionID, mediaIDs, resolution), boot.ProjectID, token),
			cookies, sourcePath)
		if err != nil {
			return "", e.handleGenerationError(account, err)
		}
		return parseVideoOperationID(payload), nil

	case "extend":
		payload, err := account.client.Call(ctx, boot, rpcVideoExtend,
			videoArgument(videoTextRequest(request.Prompt, modelKey, aspect, boot.SessionID, resolution), boot.ProjectID, token),
			cookies, sourcePath)
		if err != nil {
			return "", e.handleGenerationError(account, err)
		}
		return parseVideoOperationID(payload), nil
	}
	return "", fmt.Errorf("flow: 不支持的视频模式 %q", item.VideoType)
}

// uploadReferenceImages 把网关侧的图片 URL / data URI 上传成 Flow 媒体 ID。
func (e *Engine) uploadReferenceImages(ctx context.Context, account *flowAccount, boot Bootstrap, cookies, token string, refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(refs))
	for i, ref := range refs {
		data, mimeType, err := decodeImageReference(ctx, account, ref)
		if err != nil {
			return nil, fmt.Errorf("flow: 第 %d 张参考图无效: %w", i+1, err)
		}
		extension := "jpg"
		if strings.Contains(mimeType, "png") {
			extension = "png"
		}
		filename := fmt.Sprintf("web2api_%d.%s", i+1, extension)
		payload, err := account.client.Call(ctx, boot, rpcUploadImage,
			uploadImageArgument(boot.ProjectID, base64.StdEncoding.EncodeToString(data), mimeType, filename, token),
			cookies, "/project/"+boot.ProjectID)
		if err != nil {
			return nil, e.handleGenerationError(account, err)
		}
		mediaID := FindUUID(payload, boot.ProjectID)
		if mediaID == "" {
			if mediaURL := findContentURL(payload, "image"); mediaURL != "" {
				if match := uuidPattern.FindString(mediaURL); match != "" {
					mediaID = match
				}
			}
		}
		if mediaID == "" {
			return nil, errors.New("flow: 参考图上传响应没有媒体 ID")
		}
		ids = append(ids, mediaID)
	}
	return ids, nil
}

// decodeImageReference 接受 data URI 或 http(s) 图片地址。
func decodeImageReference(ctx context.Context, account *flowAccount, ref string) ([]byte, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, "", errors.New("空图片引用")
	}
	if strings.HasPrefix(ref, "data:") {
		comma := strings.IndexByte(ref, ',')
		if comma < 0 {
			return nil, "", errors.New("data URI 格式错误")
		}
		meta := strings.ToLower(ref[5:comma])
		mimeType := "image/png"
		base64Data := false
		for _, part := range strings.Split(meta, ";") {
			switch {
			case strings.HasPrefix(part, "image/"):
				mimeType = part
			case part == "base64":
				base64Data = true
			}
		}
		encoded := ref[comma+1:]
		if base64Data {
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return nil, "", err
			}
			return data, mimeType, nil
		}
		return []byte(encoded), mimeType, nil
	}
	parsed, err := url.Parse(ref)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, "", errors.New("仅支持 data URI 与 http(s) 图片地址")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := account.client.HTTPClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, "", err
	}
	mimeType := resp.Header.Get("Content-Type")
	if parsedMIME, _, err := mime.ParseMediaType(mimeType); err == nil {
		mimeType = parsedMIME
	}
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/png"
	}
	return data, mimeType, nil
}

// parseVideoOperationID 从提交响应里取 operation ID。
func parseVideoOperationID(payload json.RawMessage) string {
	var root []json.RawMessage
	if json.Unmarshal(payload, &root) != nil {
		return ""
	}
	for _, raw := range root {
		var record []json.RawMessage
		if json.Unmarshal(raw, &record) != nil || len(record) == 0 {
			continue
		}
		var id string
		if json.Unmarshal(record[0], &id) == nil && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

// parseVideoStatus 解析 jwpduf 返回的任务记录。
func parseVideoStatus(payload json.RawMessage, operationID string) (videoStatus, error) {
	var root any
	if err := json.Unmarshal(payload, &root); err != nil {
		return videoStatus{}, fmt.Errorf("flow: 解析视频状态失败: %w", err)
	}
	for _, record := range findVideoRecords(root, "") {
		status, found := parseVideoStatusRecord(record, operationID)
		if found {
			return status, nil
		}
	}
	return videoStatus{}, errors.New("flow: 视频状态响应里没有这个 operation")
}

func findVideoRecords(value any, projectID string) [][]any {
	var records [][]any
	seen := map[string]bool{}
	var walk func(any)
	walk = func(value any) {
		var list []any
		list, ok := value.([]any)
		if !ok {
			return
		}
		if len(list) >= 3 && (projectID == "" || flowString(list[1]) == projectID) {
			id := flowString(list[0])
			if !seen[id] {
				seen[id] = true
				records = append(records, list)
			}
			return
		}
		for _, item := range list {
			walk(item)
		}
	}
	walk(value)
	return records
}

func parseVideoStatusRecord(record []any, operationID string) (videoStatus, bool) {
	id := flowString(record[0])
	if id != operationID {
		return videoStatus{}, false
	}
	state := strings.ToUpper(strings.TrimSpace(flowString(record[2])))
	mediaID := ""
	for index := len(record) - 1; index >= 3; index-- {
		if flowString(record[index]) != "" {
			mediaID = strings.TrimRight(flowString(record[index]), "/")
			break
		}
	}
	if mediaID == "" {
		if url := findVideoContentURL(record); url != "" {
			if match := uuidPattern.FindString(url); match != "" {
				mediaID = match
			} else if index := strings.Index(url, "/video/"); index >= 0 {
				mediaID = strings.TrimSpace(strings.SplitN(url[index+len("/video/"):], "/", 2)[0])
			}
		}
	}
	switch {
	case strings.Contains(state, "SUCCESSFUL"), state == "DONE", state == "COMPLETED", state == "SUCCEEDED":
		if mediaID == "" {
			mediaID = id
		}
		return videoStatus{Done: true, MediaID: mediaID}, true
	case strings.Contains(state, "FAILED"), strings.Contains(state, "ERROR"), strings.Contains(state, "CANCELLED"):
		return videoStatus{Done: true}, true
	}
	return videoStatus{}, true
}

func findVideoContentURL(value any) string {
	switch item := value.(type) {
	case string:
		if strings.HasPrefix(item, "https://") && strings.Contains(item, "/video/") {
			return item
		}
	case []any:
		for _, child := range item {
			if url := findVideoContentURL(child); url != "" {
				return url
			}
		}
	case map[string]any:
		for _, child := range item {
			if url := findVideoContentURL(child); url != "" {
				return url
			}
		}
	}
	return ""
}

func flowString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func isFlowUUID(value any) bool {
	return uuidPattern.MatchString(flowString(value))
}
