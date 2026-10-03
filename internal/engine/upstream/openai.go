// Package upstream 实现「OpenAI 兼容上游」适配器：
// 引擎B（AIStudio2API / 任意 OpenAI 兼容服务）以及引擎A 的远程模式都基于它。
package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"web2api/internal/model"
)

// PassthroughWriter 抽象透传目标（网关 http.ResponseWriter）。
type PassthroughWriter interface {
	Header() http.Header
	WriteHeader(status int)
	Write(p []byte) (int, error)
}

// OpenAI 一个 OpenAI 兼容上游。
type OpenAI struct {
	mu sync.RWMutex

	name         string
	baseURL      string // 如 http://127.0.0.1:2048
	apiKey       string
	passthrough  bool
	http         *http.Client
	cachedModels []model.ModelInfo
	ready        bool
	readyErr     string
	readyAt      time.Time
}

// New 创建上游。
func New(name, baseURL, apiKey string, passthrough bool, timeout time.Duration) *OpenAI {
	return &OpenAI{
		name:        name,
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		passthrough: passthrough,
		http:        &http.Client{Timeout: timeout},
	}
}

// Name 上游名。
func (o *OpenAI) Name() string { return o.name }

// Passthrough 是否透传。
func (o *OpenAI) Passthrough() bool { return o.passthrough }

// Ready 上游可达性（以最近一次探测为准）。
func (o *OpenAI) Ready() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.ready
}

// ReadyErr 返回最近错误。
func (o *OpenAI) ReadyErr() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.readyErr
}

// MarkReady 标记上游可用。
func (o *OpenAI) MarkReady() {
	o.mu.Lock()
	o.ready = true
	o.readyAt = time.Now()
	o.readyErr = ""
	o.mu.Unlock()
}

// MarkDown 标记上游不可用。
func (o *OpenAI) MarkDown(err error) {
	o.mu.Lock()
	o.ready = false
	o.readyAt = time.Time{}
	if err != nil {
		o.readyErr = err.Error()
	}
	o.mu.Unlock()
}

// do 执行请求并返回响应体与状态码。
func (o *OpenAI) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(data)
	}
	url := o.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// RefreshModels 主动刷新模型缓存并标记可用性。
func (o *OpenAI) RefreshModels(ctx context.Context) []model.ModelInfo {
	data, status, err := o.do(ctx, http.MethodGet, "/v1/models", nil)
	if err != nil || status != http.StatusOK {
		o.MarkDown(fmt.Errorf("上游 /v1/models 失败: %v (HTTP %d)", err, status))
		return nil
	}
	var resp struct {
		Data []model.ModelInfo `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		o.MarkDown(err)
		return nil
	}
	o.mu.Lock()
	o.cachedModels = resp.Data
	o.ready = true
	o.readyAt = time.Now()
	o.readyErr = ""
	o.mu.Unlock()
	return resp.Data
}

// ListModels 返回缓存的模型列表。
func (o *OpenAI) ListModels() []model.ModelInfo {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.cachedModels
}

// Models 返回缓存的模型列表（同 ListModels）。
func (o *OpenAI) Models() []model.ModelInfo {
	return o.ListModels()
}

// BaseURL 上游地址。
func (o *OpenAI) BaseURL() string { return o.baseURL }

// Chat 非流式对话。
func (o *OpenAI) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	payload := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   false,
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		payload["max_tokens"] = *req.MaxTokens
	}
	if req.TopP != nil {
		payload["top_p"] = *req.TopP
	}
	data, status, err := o.do(ctx, http.MethodPost, "/v1/chat/completions", payload)
	if err != nil {
		o.MarkDown(err)
		return nil, err
	}
	if status != http.StatusOK {
		o.MarkDown(fmt.Errorf("上游 /v1/chat/completions 返回 HTTP %d", status))
		return nil, fmt.Errorf("上游返回 HTTP %d: %s", status, truncate(data))
	}
	var resp model.ChatCompletionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析上游响应失败: %w", err)
	}
	text := ""
	if len(resp.Choices) > 0 {
		text = resp.Choices[0].Message.Content
	}
	o.MarkReady()
	return &model.ChatResult{Text: text, Engine: o.name, Model: req.Model}, nil
}

// ChatStream 流式对话（SSE 解析）。
func (o *OpenAI) ChatStream(ctx context.Context, req model.ChatRequest, onDelta func(string) error) (*model.ChatResult, error) {
	payload := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   true,
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		payload["max_tokens"] = *req.MaxTokens
	}
	if req.TopP != nil {
		payload["top_p"] = *req.TopP
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := o.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.http.Do(httpReq)
	if err != nil {
		o.MarkDown(err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		o.MarkDown(fmt.Errorf("上游 /v1/chat/completions 返回 HTTP %d", resp.StatusCode))
		return nil, fmt.Errorf("上游返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk model.Chunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		full.WriteString(delta)
		if onDelta != nil {
			if err := onDelta(delta); err != nil {
				return nil, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	o.MarkReady()
	return &model.ChatResult{Text: full.String(), Engine: o.name, Model: req.Model}, nil
}

// ServePassthrough 将请求原样转发到上游同路径。
func (o *OpenAI) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w PassthroughWriter) error {
	url := o.baseURL + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		o.MarkDown(err)
		return err
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}

func truncate(data []byte) string {
	s := string(data)
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
