// Package upstream 实现「OpenAI 兼容上游」适配器：
// 引擎B upstream 模式（任意 OpenAI 兼容服务）以及引擎A 的远程模式都基于它。
package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
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
	return NewWithClient(name, baseURL, apiKey, passthrough, newHTTPClient(timeout))
}

// newHTTPClient 构造只约束「响应头到达时间」的客户端。
//
// 不能用 http.Client.Timeout：它覆盖整个响应体读取，长回答的 SSE 会在
// 到达该时长时被硬切断（表现为流式截断）。响应体读取交给请求 ctx 控制。
func newHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if timeout > 0 {
		transport.ResponseHeaderTimeout = timeout
	}
	return &http.Client{Transport: transport}
}

// NewWithClient 使用自定义 HTTP 客户端创建上游（可带代理）。
func NewWithClient(name, baseURL, apiKey string, passthrough bool, client *http.Client) *OpenAI {
	if client == nil {
		client = newHTTPClient(300 * time.Second)
	}
	return &OpenAI{
		name:        name,
		baseURL:     NormalizeBaseURL(baseURL),
		apiKey:      apiKey,
		passthrough: passthrough,
		http:        client,
	}
}

// NormalizeBaseURL 去掉尾部斜杠和可选的 /v1。
func NormalizeBaseURL(baseURL string) string {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(u), "/v1") {
		u = strings.TrimRight(u[:len(u)-3], "/")
	}
	return u
}

// endpoint 拼接上游地址。base_url 自带版本段时（Gemini 的
// https://generativelanguage.googleapis.com/v1beta/openai、智谱 /api/paas/v4 等）
// 不能再补 /v1：/v1beta/openai/v1/models 会 404。此时把路径里的 /v1 前缀去掉。
func (o *OpenAI) endpoint(path string) string {
	if HasVersionedBase(o.baseURL) {
		if rest, ok := strings.CutPrefix(path, "/v1/"); ok {
			path = "/" + rest
		}
	}
	return o.baseURL + path
}

// HasVersionedBase 判断 base_url 的路径是否已经包含 API 版本段
// （末段形如 v1beta / v4，或 Gemini 的 .../openai）。
func HasVersionedBase(baseURL string) bool {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || u.Path == "" {
		return false
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	last := strings.ToLower(segs[len(segs)-1])
	if last == "openai" && len(segs) >= 2 && versionSegment.MatchString(strings.ToLower(segs[len(segs)-2])) {
		return true
	}
	return versionSegment.MatchString(last)
}

var versionSegment = regexp.MustCompile(`^v\d+[a-z0-9]*$`)

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
	url := o.endpoint(path)
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

// dummyThoughtSignature 是 Gemini 文档给出的占位签名：历史里的工具调用拿不到
// 原始 thought_signature 时（客户端没回传、网关重启丢了缓存）用它跳过校验。
// https://ai.google.dev/gemini-api/docs/thought-signatures
var dummyThoughtSignature = json.RawMessage(`{"google":{"thought_signature":"skip_thought_signature_validator"}}`)

// withThoughtSignatures 给发往 Gemini 模型的历史工具调用补齐 thought_signature：
// 先用网关缓存的原始签名，缺失时用占位签名。非 Gemini 模型原样返回。
func withThoughtSignatures(modelName string, messages []model.ChatMessage) []model.ChatMessage {
	if !strings.Contains(strings.ToLower(modelName), "gemini") {
		return messages
	}
	var out []model.ChatMessage
	for i, msg := range messages {
		if len(msg.ToolCalls) == 0 {
			continue
		}
		if out == nil {
			out = append([]model.ChatMessage(nil), messages...)
		}
		calls := append([]model.ToolCall(nil), msg.ToolCalls...)
		for j := range calls {
			if len(calls[j].ExtraContent) > 0 {
				continue
			}
			if extra := model.LookupToolCallExtra(calls[j].ID); len(extra) > 0 {
				calls[j].ExtraContent = extra
			} else if j == 0 {
				// Gemini 只要求每个回合第一个 functionCall 带签名。
				calls[j].ExtraContent = dummyThoughtSignature
			}
		}
		out[i].ToolCalls = calls
	}
	if out == nil {
		return messages
	}
	return out
}

// BaseURL 上游地址。
func (o *OpenAI) BaseURL() string { return o.baseURL }

// chatPayload 组装 /v1/chat/completions 请求体。工具定义只在非空时下发：
// 部分第三方上游看到空的 tools 数组会直接 400。
func (o *OpenAI) chatPayload(req model.ChatRequest, stream bool) map[string]any {
	payload := map[string]any{
		"model":    req.Model,
		"messages": withThoughtSignatures(req.Model, req.Messages),
		"stream":   stream,
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
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
		if choice := bytes.TrimSpace(req.ToolChoice); len(choice) > 0 && string(choice) != "null" {
			// 必须转回 json.RawMessage：[]byte 会被 encoding/json 编成 base64 字符串
			// （"auto" → "ImF1dG8i"），上游直接 400。
			payload["tool_choice"] = json.RawMessage(choice)
		}
		if req.ParallelToolCalls != nil {
			payload["parallel_tool_calls"] = *req.ParallelToolCalls
		}
	}
	// include_usage 只按客户端要求透传：少数第三方上游不认识该字段，
	// 无条件下发会让整条流式请求失败。网关自己的用量回退到估算。
	if stream && req.StreamOptions != nil && req.StreamOptions.IncludeUsage {
		payload["stream_options"] = req.StreamOptions
	}
	return payload
}

// Chat 非流式对话。
func (o *OpenAI) Chat(ctx context.Context, req model.ChatRequest) (*model.ChatResult, error) {
	payload := o.chatPayload(req, false)
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
	result := &model.ChatResult{Engine: o.name, Model: req.Model}
	if len(resp.Choices) > 0 {
		result.Text = resp.Choices[0].Message.Content
		result.ToolCalls = resp.Choices[0].Message.ToolCalls
		rememberToolCallExtras(result.ToolCalls)
		result.FinishReason = resp.Choices[0].FinishReason
	}
	o.MarkReady()
	if resp.Usage.TotalTokens > 0 || resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		// 上游 OpenAI 兼容接口返回的权威 usage。
		usage := model.TokenUsage{
			PromptTokens:     int64(resp.Usage.PromptTokens),
			CompletionTokens: int64(resp.Usage.CompletionTokens),
			TotalTokens:      int64(resp.Usage.TotalTokens),
		}.Normalize()
		result.Usage = &usage
	}
	return result, nil
}

// ChatStream 流式对话（SSE 解析）。
func (o *OpenAI) ChatStream(ctx context.Context, req model.ChatRequest, onDelta func(string) error) (*model.ChatResult, error) {
	return o.ChatStreamSink(ctx, req, model.TextSink(onDelta))
}

// ChatStreamSink 流式对话，同时回传文本与工具调用增量。
func (o *OpenAI) ChatStreamSink(ctx context.Context, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	if sink == nil {
		sink = model.TextSink(nil)
	}
	data, err := json.Marshal(o.chatPayload(req, true))
	if err != nil {
		return nil, err
	}
	url := o.endpoint("/v1/chat/completions")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
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
	var usage *model.TokenUsage
	finishReason := ""
	// 工具调用分块按 index 归并：上游会把 id/name 放在首块，
	// 参数按 JSON 片段逐块下发，必须拼完才能得到完整 arguments。
	accs := map[int]*model.ToolCall{}
	var indices []int

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
		// 上游用量可能挂在结束分片上，也可能单独一个 choices 为空的分片
		// （stream_options.include_usage）。两种都要收，否则网关只能估算。
		if chunk.Usage != nil {
			u := chunk.Usage.Normalize()
			usage = &u
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			finishReason = *choice.FinishReason
		}
		if delta := choice.Delta.Content; delta != "" {
			full.WriteString(delta)
			if err := sink.Text(delta); err != nil {
				return nil, err
			}
		}
		for _, tc := range choice.Delta.ToolCalls {
			acc := accs[tc.Index]
			if acc == nil {
				acc = &model.ToolCall{Type: "function"}
				accs[tc.Index] = acc
				indices = append(indices, tc.Index)
			}
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Type != "" {
				acc.Type = tc.Type
			}
			acc.Function.Name += tc.Function.Name
			acc.Function.Arguments += tc.Function.Arguments
			if len(tc.ExtraContent) > 0 {
				acc.ExtraContent = tc.ExtraContent
			}
			if err := sink.ToolCall(tc); err != nil {
				return nil, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	o.MarkReady()
	result := &model.ChatResult{Text: full.String(), Engine: o.name, Model: req.Model, Usage: usage, FinishReason: finishReason}
	sort.Ints(indices)
	for _, index := range indices {
		call := *accs[index]
		if call.ID == "" {
			// 少数上游不给 id，补一个稳定的本地 id，否则客户端无法回填结果。
			call.ID = "call_" + model.NewID(16)
		}
		if call.Type == "" {
			call.Type = "function"
		}
		result.ToolCalls = append(result.ToolCalls, call)
	}
	rememberToolCallExtras(result.ToolCalls)
	if len(result.ToolCalls) > 0 && result.FinishReason == "" {
		result.FinishReason = "tool_calls"
	}
	return result, nil
}

// rememberToolCallExtras 记下上游给的 extra_content（thought_signature），
// 供只回传 call_id 的客户端（Codex / Responses）下一轮补回。
func rememberToolCallExtras(calls []model.ToolCall) {
	for _, call := range calls {
		model.RememberToolCallExtra(call.ID, call.ExtraContent)
	}
}

// ProbeImage 探测图片模型：调用 /v1/images/generations（不走 Chat Completions）。
func (o *OpenAI) ProbeImage(ctx context.Context, modelName string) (string, error) {
	attempts := []map[string]any{
		{"model": modelName, "prompt": "a tiny red dot", "n": 1, "size": "1024x1024"},
		{"model": modelName, "prompt": "a tiny red dot", "n": 1},
		{"model": modelName, "prompt": "a tiny red dot"},
	}
	var last error
	for _, payload := range attempts {
		data, status, err := o.do(ctx, http.MethodPost, "/v1/images/generations", payload)
		if err != nil {
			return "", err
		}
		if status == http.StatusOK {
			o.MarkReady()
			return "图片接口响应正常", nil
		}
		last = fmt.Errorf("上游返回 HTTP %d: %s", status, truncate(data))
		if status != http.StatusBadRequest {
			break
		}
	}
	return "", last
}

// ProbeEmbedding 探测向量模型：调用 /v1/embeddings。
func (o *OpenAI) ProbeEmbedding(ctx context.Context, modelName string) (string, error) {
	payload := map[string]any{
		"model": modelName,
		"input": "ping",
	}
	data, status, err := o.do(ctx, http.MethodPost, "/v1/embeddings", payload)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("上游返回 HTTP %d: %s", status, truncate(data))
	}
	o.MarkReady()
	return "向量接口响应正常", nil
}

// ServePassthrough 将请求原样转发到上游同路径。
func (o *OpenAI) ServePassthrough(ctx context.Context, method, path string, body io.Reader, contentType string, w PassthroughWriter) error {
	url := o.endpoint("/" + strings.TrimLeft(path, "/"))
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
