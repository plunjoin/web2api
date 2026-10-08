package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	aistudio "web2api/internal/aistudio2api/aistudio"
	"web2api/internal/store"
)

// videoCreateRequest 是 OpenAI 风格的视频创建请求。Veo 的实际 RPC 是
// GenerateVideo 长任务，创建后需要通过 GET /v1/videos/{id} 轮询。
type videoCreateRequest struct {
	Model           string          `json:"model"`
	Prompt          string          `json:"prompt"`
	Seconds         json.RawMessage `json:"seconds"`
	DurationSeconds int             `json:"duration_seconds"`
	AspectRatio     string          `json:"aspect_ratio"`
	Resolution      string          `json:"resolution"`
	Size            string          `json:"size"`
}

type videoResponse struct {
	ID        string     `json:"id"`
	Object    string     `json:"object"`
	Status    string     `json:"status"`
	Model     string     `json:"model,omitempty"`
	CreatedAt int64      `json:"created_at"`
	Seconds   string     `json:"seconds,omitempty"`
	Size      string     `json:"size,omitempty"`
	Output    *videoFile `json:"output,omitempty"`
}

type videoFile struct {
	FileID string `json:"file_id"`
	MIME   string `json:"mime_type,omitempty"`
	URL    string `json:"url,omitempty"`
}

func (s *Server) handleCreateVideo(w http.ResponseWriter, r *http.Request) {
	var req videoCreateRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error(), "invalid_request_error", nil)
		return
	}
	if strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "model and prompt are required", "invalid_request_error", nil)
		return
	}
	if !s.modelAllowed(w, r, req.Model) {
		return
	}
	seconds := req.DurationSeconds
	if seconds == 0 {
		seconds = rawInt(req.Seconds)
	}
	if seconds == 0 {
		seconds = 4
	}
	// 用户 Key：视频没有 Token 用量，按「秒 × 每秒计费 Token」在创建成功后扣费；未配置单价时不开放。
	var videoPrice int64
	if info, ok := keyInfoFrom(r); ok && info.UserID > 0 {
		settings, err := s.st.GetPlatformSettings()
		if err != nil || settings.VideoTokensPerSecond <= 0 {
			writeError(w, http.StatusForbidden, "Video generation is not enabled for user API keys (视频接口未对用户 Key 开放).",
				"permission_error", "endpoint_not_allowed")
			return
		}
		videoPrice = settings.VideoTokensPerSecond
	}
	start := time.Now()
	operation, err := s.mgr.GenerateVideo(r.Context(), aistudio.VideoRequest{
		Model: req.Model, Prompt: req.Prompt, Count: 1,
		AspectRatio: req.AspectRatio, DurationSeconds: seconds,
		Resolution: req.Resolution, Size: req.Size,
	})
	if err != nil {
		s.logger.Printf("[video] 创建失败 model=%q: %v", req.Model, err)
		writeVideoError(w, err)
		return
	}
	if videoPrice > 0 {
		info, _ := keyInfoFrom(r)
		if _, err := s.st.RecordRequest(store.UsageRecord{
			KeyID: info.ID, APIKey: s.currentKey(r), Engine: "b", Model: req.Model, Endpoint: "/v1/videos",
			Success: true, TotalTokens: int64(seconds) * videoPrice, LatencyMs: time.Since(start).Milliseconds(),
		}); err != nil {
			s.logger.Printf("[video] 记录视频扣费失败 model=%q: %v", req.Model, err)
		}
	}
	writeJSON(w, http.StatusAccepted, videoResponse{
		ID: operation.ID, Object: "video", Status: videoStatus(operation),
		Model: req.Model, CreatedAt: time.Now().Unix(), Seconds: operation.Seconds, Size: operation.Size,
	})
}

func (s *Server) handleGetVideo(w http.ResponseWriter, r *http.Request) {
	operationID := strings.TrimSpace(r.PathValue("id"))
	if operationID == "" {
		writeError(w, http.StatusBadRequest, "video id is required", "invalid_request_error", nil)
		return
	}
	operation, err := s.mgr.GetGenerateVideoOperation(r.Context(), operationID)
	if err != nil {
		writeVideoError(w, err)
		return
	}
	resp := videoResponse{
		ID: operation.ID, Object: "video", Status: videoStatus(operation),
		Model: operation.Model, CreatedAt: operation.CreatedAt.Unix(), Seconds: operation.Seconds, Size: operation.Size,
	}
	if operation.File != nil {
		resp.Output = &videoFile{FileID: operation.File.ID, MIME: operation.File.MIME,
			URL: "/v1/videos/" + operation.ID + "/content"}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleVideoContent(w http.ResponseWriter, r *http.Request) {
	operationID := strings.TrimSpace(r.PathValue("id"))
	if operationID == "" {
		writeError(w, http.StatusBadRequest, "video id is required", "invalid_request_error", nil)
		return
	}
	operation, err := s.mgr.GetGenerateVideoOperation(r.Context(), operationID)
	if err != nil {
		writeVideoError(w, err)
		return
	}
	if !operation.Done || operation.File == nil {
		writeError(w, http.StatusConflict, "video is not completed yet", "operation_in_progress", nil)
		return
	}
	media, err := s.mgr.DownloadVideoFile(r.Context(), operation.File.ID)
	if err != nil {
		writeVideoError(w, err)
		return
	}
	defer media.Body.Close()
	mime := strings.TrimSpace(media.MIME)
	if mime == "" {
		mime = "video/mp4"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.mp4"`, operationID))
	if media.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(media.Size, 10))
	}
	_, _ = io.Copy(w, media.Body)
}

// writeVideoError 保留额度错误的公开状态，支持账户池包装或合并的错误。
func writeVideoError(w http.ResponseWriter, err error) {
	var statusError interface{ HTTPStatus() int }
	if errors.As(err, &statusError) && statusError.HTTPStatus() == http.StatusTooManyRequests {
		writeError(w, http.StatusTooManyRequests, err.Error(), "rate_limit_error", "rate_limit_exceeded")
		return
	}
	if errors.Is(err, aistudio.ErrInvalidArgument) {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
		return
	}
	writeError(w, http.StatusBadGateway, err.Error(), "api_error", nil)
}

func videoStatus(operation aistudio.VideoOperation) string {
	if operation.Done && operation.File != nil {
		return "completed"
	}
	if operation.Done {
		return "failed"
	}
	return "in_progress"
}

func rawInt(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		n, _ = strconv.Atoi(strings.TrimSpace(text))
	}
	return n
}
