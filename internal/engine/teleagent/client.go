// client.go 实现 TeleAgent 上游客户端：签名聊天（SSE）与积分门户查询。
package teleagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"web2api/internal/model"
)

// chatPayload 构造 OpenAI 兼容的 chat/completions 请求体。
// TeleAgent 桌面端所有请求都是 stream:true，这里保持一致（非流式也向上游发流式，
// 本地聚合），避免在请求形态上露出差异。
func chatPayload(req model.ChatRequest) map[string]any {
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
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
		if choice := bytes.TrimSpace(req.ToolChoice); len(choice) > 0 && string(choice) != "null" {
			payload["tool_choice"] = json.RawMessage(choice)
		}
		if req.ParallelToolCalls != nil {
			payload["parallel_tool_calls"] = *req.ParallelToolCalls
		}
	}
	// 思考档位：上游只认 low/high/max，且不接受「关闭思考」。宿主可能写
	// reasoning_effort=medium 等非法值，这里在发出去前削成合法档位（见 effort）。
	if v := effortOf(req); v != "" {
		payload["reasoning_effort"] = v
	}
	return payload
}

// effortOf 从请求里取 reasoning_effort 并归一化。拿不到就返回空串（不发该字段，
// 上游按默认档位思考）。返回空串也可能表示「应删掉字段」，这里统一不写。
func effortOf(req model.ChatRequest) string {
	var level string
	if len(req.Extra) > 0 {
		var extra map[string]any
		if json.Unmarshal(req.Extra, &extra) == nil {
			if s, ok := extra["reasoning_effort"].(string); ok {
				level = s
			}
		}
	}
	if level == "" {
		return ""
	}
	return normalizeEffort(level)
}

// signedRequest 发起一次带两步 HMAC 签名的 POST（body 为已序列化 JSON 文本）。
func signedRequest(ctx context.Context, client *http.Client, creds Credentials, body []byte) (*http.Response, error) {
	pathname := APIPrefix + ChatPath
	auth := buildAuthHeaders(creds, http.MethodPost, pathname, string(body))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ChatEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range auth.headers {
		req.Header.Set(k, v)
	}
	return client.Do(req)
}

// chatStream 签名聊天（上游总是流式，这里解析 SSE 增量到 sink）。
func chatStream(ctx context.Context, client *http.Client, creds Credentials, req model.ChatRequest, sink model.StreamSink) (*model.ChatResult, error) {
	if sink == nil {
		sink = model.TextSink(nil)
	}
	data, err := json.Marshal(chatPayload(req))
	if err != nil {
		return nil, err
	}
	resp, err := signedRequest(ctx, client, creds, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, upstreamError(resp.StatusCode, body)
	}

	var full strings.Builder
	var usage *model.TokenUsage
	finishReason := ""
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
			if err := sink.ToolCall(tc); err != nil {
				return nil, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	result := &model.ChatResult{Text: full.String(), Model: req.Model, Usage: usage, FinishReason: finishReason}
	sort.Ints(indices)
	for _, index := range indices {
		call := *accs[index]
		if call.ID == "" {
			call.ID = "call_" + model.NewID(16)
		}
		if call.Type == "" {
			call.Type = "function"
		}
		result.ToolCalls = append(result.ToolCalls, call)
	}
	if len(result.ToolCalls) > 0 && result.FinishReason == "" {
		result.FinishReason = "tool_calls"
	}
	return result, nil
}

// upstreamError 把上游非 200 响应翻译成带错误码的可读错误。
func upstreamError(status int, body []byte) error {
	msg := parseUpstreamError(body)
	// 业务码优先于 HTTP 状态，便于账号健康判断（40104/40108/40109/402/429…）。
	if m := extractCode(body); m != "" {
		return fmt.Errorf("teleagent 上游错误 %s（HTTP %d）: %s", m, status, msg)
	}
	return fmt.Errorf("teleagent 上游返回 HTTP %d: %s", status, msg)
}

// extractCode 从上游错误体里提取业务 code / error.code / error.type。
func extractCode(body []byte) string {
	var v struct {
		Code  any `json:"code"`
		Error struct {
			Code any `json:"code"`
			Type any `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &v) != nil {
		return ""
	}
	for _, c := range []any{v.Code, v.Error.Code, v.Error.Type} {
		switch t := c.(type) {
		case string:
			if t != "" {
				return t
			}
		case float64:
			return fmt.Sprintf("%.0f", t)
		}
	}
	return ""
}

// ---- 积分门户（不签名，头鉴权，见 docs/guide/teleagent-auth.md 第 4/5 节） ----

// portalBase 是积分门户根地址（与聊天网关同一前缀，但不用 HMAC 签名）。
const portalBase = Origin + APIPrefix + "/user/portal"

// portalRequest 调积分门户接口（X-Token + 时间戳 + nonce 头鉴权，无 HMAC）。
// 返回 data 原始 JSON：不同端点的 data 形状不一（userCenter/checkDayTask 是对象、
// checkTask 签到成功是空数组），由调用方各自解析。
func portalRequest(ctx context.Context, client *http.Client, creds Credentials, path string) (json.RawMessage, error) {
	url := portalBase + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	appVersion := creds.AppVersion
	if appVersion == "" {
		appVersion = DefaultAppVersion
	}
	req.Header.Set("X-Token", creds.Token)
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("X-Nonce", newNonce())
	req.Header.Set("X-App-Version", appVersion)
	req.Header.Set("X-SuperAgent-Device-Id", creds.DeviceID)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var v struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("解析积分响应失败: %w", err)
	}
	if v.Code != 0 && v.Code != 200 {
		return nil, fmt.Errorf("积分接口 code=%d: %s", v.Code, v.Msg)
	}
	return v.Data, nil
}

// pointsBalance 查询积分余额（best-effort，失败不阻塞聊天）。
func pointsBalance(ctx context.Context, client *http.Client, creds Credentials) (balance float64, planName string, err error) {
	raw, err := portalRequest(ctx, client, creds, "/companyStore/userCenter")
	if err != nil {
		return 0, "", err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return 0, "", err
	}
	balance = firstNumber(data, "current_points_balance", "pointsBalance", "balance", "points")
	planName = firstString(data, "plan_display_name", "planName", "plan")
	return balance, planName, nil
}

// ---- 每日签到（见 docs/guide/teleagent-auth.md 第 5 节，全部 GET） ----

// dailyStatus 每日签到任务状态（v2/checkDayTask 的 data 摘要）。
type dailyStatus struct {
	Visible    bool    // 活动是否展示
	Claimed    bool    // 今日是否已领
	Claimable  float64 // 今日可领积分
	TotalDays  float64 // 本期累计领取天数
	ButtonText string  // 领取按钮文案
}

// dailyTaskStatus 查询每日签到状态。
func dailyTaskStatus(ctx context.Context, client *http.Client, creds Credentials) (*dailyStatus, error) {
	raw, err := portalRequest(ctx, client, creds, "/companyStore/v2/checkDayTask")
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	st := &dailyStatus{}
	if act, ok := data["activity"].(map[string]any); ok {
		st.Visible = asBool(act["visible"])
	}
	if dt, ok := data["daily_task"].(map[string]any); ok {
		st.Claimed = asBool(dt["claimed_today"])
		st.Claimable = firstNumber(dt, "daily_claimable_points")
		st.TotalDays = firstNumber(dt, "total_claim_days")
		st.ButtonText = firstString(dt, "button_text")
	}
	return st, nil
}

// claimDailyTask 领取每日签到积分（checkTask?type=daily_login）。
// 是否真的可领由调用方先查 dailyTaskStatus 决定；重复领取上游会按已领处理。
func claimDailyTask(ctx context.Context, client *http.Client, creds Credentials) error {
	_, err := portalRequest(ctx, client, creds, "/companyStore/checkTask?type=daily_login")
	return err
}

// asBool 兼容 bool / 数值 / 字符串形式的布尔字段。
func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case json.Number:
		n, err := t.Float64()
		return err == nil && n != 0
	case string:
		return t == "true" || t == "1"
	}
	return false
}

// firstNumber 从嵌套 map 里取第一个存在的数值字段。
func firstNumber(data map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := data[k]; ok {
			if n, ok := asNumber(v); ok {
				return n
			}
		}
	}
	return 0
}

// firstString 从嵌套 map 里取第一个存在的字符串字段。
func firstString(data map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := data[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// asNumber 兼容 float64 / json.Number / int 等数值类型。
func asNumber(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		var f float64
		if _, err := fmt.Sscanf(t, "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}
