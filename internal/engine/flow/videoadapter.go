// videoadapter.go 把网关使用的中立视频接口适配到 Flow 原生协议。
package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"web2api/internal/engine"
)

var errFlowVideo = errors.New("flow video")

// videoOperation 是 Flow 的 operation → 账号绑定。
type videoOperation struct {
	AccountID string
	ProjectID string
	ModelID   string
}

// SubmitVideo 实现 engine.VideoEngine。
func (e *Engine) SubmitVideo(ctx context.Context, request engine.VideoRequest) (engine.VideoOperation, error) {
	item, ok := findVideoModel(request.Model)
	if !ok {
		return engine.VideoOperation{}, fmt.Errorf("%w: 模型 %q 不是 Flow 视频模型", errFlowVideo, request.Model)
	}
	account, err := e.pickAccount(request.Model, engine.PolicyKindVideo)
	if err != nil {
		return engine.VideoOperation{}, err
	}
	if !e.acquirePolicySlot(account, engine.PolicyKindVideo) {
		return engine.VideoOperation{}, engine.UnavailableError(e)
	}
	defer e.releasePolicySlot(account, engine.PolicyKindVideo)

	// reCAPTCHA token 必须和提交流量的 User-Agent / Accept-Language 一致；
	// 这里先完成 bootstrap 并把浏览器指纹写回 RPC 客户端。
	boot, cookies, token, err := e.withVideoSession(ctx, account)
	if err != nil {
		return engine.VideoOperation{}, err
	}

	operationID, err := e.submitVideo(ctx, account, boot, cookies, token, item, request)
	if err != nil {
		return engine.VideoOperation{}, err
	}
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return engine.VideoOperation{}, errors.New("flow: 视频提交响应没有 operation ID")
	}

	e.videoMu.Lock()
	e.videoOperations[operationID] = videoOperation{
		AccountID: account.id,
		ProjectID: boot.ProjectID,
		ModelID:   item.ID,
	}
	e.videoMu.Unlock()
	defer func() {
		if err != nil {
			e.forgetVideoOperation(operationID)
		}
	}()

	return engine.VideoOperation{
		ID:        operationID,
		Done:      false,
		Model:     item.ID,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// PollVideo 实现 engine.VideoEngine。
func (e *Engine) PollVideo(ctx context.Context, operationID string) (engine.VideoOperation, error) {
	binding, ok := e.videoBinding(operationID)
	if !ok {
		return engine.VideoOperation{}, fmt.Errorf("%w: 未知的视频任务 %q", errFlowVideo, operationID)
	}
	account, err := e.accountByID(binding.AccountID)
	if err != nil {
		return engine.VideoOperation{}, err
	}

	boot, cookies, _, err := e.withVideoSession(ctx, account)
	if err != nil {
		return engine.VideoOperation{}, err
	}
	if boot.ProjectID == "" {
		boot.ProjectID = binding.ProjectID
	}
	payload, err := account.client.Call(ctx, boot, rpcVideoStatus,
		videoStatusArgument([]string{operationID}), cookies, "/project/"+boot.ProjectID)
	if err != nil {
		return engine.VideoOperation{}, e.handleGenerationError(account, err)
	}

	status, err := parseVideoStatus(payload, operationID)
	if err != nil {
		return engine.VideoOperation{}, err
	}
	// 任务完成后把 file ID 也登记成同一个账号的绑定，网关下载时会用它查账号。
	if status.Done && status.MediaID != "" && status.MediaID != operationID {
		e.videoMu.Lock()
		e.videoOperations[status.MediaID] = binding
		e.videoMu.Unlock()
	}
	result := engine.VideoOperation{
		ID:        operationID,
		Model:     binding.ModelID,
		CreatedAt: time.Now().UTC(),
	}
	if !status.Done {
		return result, nil
	}
	result.Done = true
	if status.MediaID != "" {
		result.File = &engine.VideoFile{
			ID:   status.MediaID,
			Name: status.MediaID,
			MIME: "video/mp4",
		}
	}
	return result, nil
}

// FetchVideoFile 实现 engine.VideoEngine。
func (e *Engine) FetchVideoFile(ctx context.Context, fileID string) (engine.MediaStream, error) {
	// 文件名就是媒体 ID：查一次所属任务，拿到当初提交任务的账号。
	binding, ok := e.videoBinding(fileID)
	if !ok {
		return engine.MediaStream{}, fmt.Errorf("%w: 未知的视频文件 %q", errFlowVideo, fileID)
	}
	account, err := e.accountByID(binding.AccountID)
	if err != nil {
		return engine.MediaStream{}, err
	}

	boot, cookies, _, err := e.withVideoSession(ctx, account)
	if err != nil {
		return engine.MediaStream{}, err
	}
	if boot.ProjectID == "" {
		boot.ProjectID = binding.ProjectID
	}
	payload, err := account.client.Call(ctx, boot, rpcGetMedia,
		[]any{fileID}, cookies, "/project/"+boot.ProjectID)
	if err != nil {
		return engine.MediaStream{}, e.handleGenerationError(account, err)
	}
	mediaURL := findContentURL(payload, "video")
	if mediaURL == "" {
		return engine.MediaStream{}, errors.New("flow: 视频响应没有下载地址")
	}
	body, mimeType, err := e.fetchMedia(ctx, account, mediaURL)
	if err != nil {
		return engine.MediaStream{}, err
	}
	if mimeType == "" {
		mimeType = "video/mp4"
	}
	return engine.MediaStream{Body: body, MIME: mimeType, Name: mediaIDFilename(fileID)}, nil
}

func (e *Engine) videoBinding(id string) (videoOperation, bool) {
	e.videoMu.RLock()
	binding, ok := e.videoOperations[id]
	e.videoMu.RUnlock()
	return binding, ok
}

func (e *Engine) forgetVideoOperation(id string) {
	e.videoMu.Lock()
	delete(e.videoOperations, id)
	e.videoMu.Unlock()
}

func (e *Engine) accountByID(id string) (*flowAccount, error) {
	e.mu.RLock()
	index, ok := e.ids[id]
	var account *flowAccount
	if ok {
		account = e.accounts[index]
	}
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: 账号 %q 已不在号池中", errFlowVideo, id)
	}
	return account, nil
}

func mediaIDFilename(mediaID string) string {
	base := strings.TrimRight(strings.TrimSpace(mediaID), "/")
	if index := strings.LastIndex(base, "/"); index >= 0 {
		base = base[index+1:]
	}
	if base == "" {
		return "video"
	}
	return base
}

var _ engine.VideoEngine = (*Engine)(nil)
