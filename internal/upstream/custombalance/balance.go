// Package custombalance 按 Base URL 识别常见第三方供应商并查询余额。
// OpenAI 兼容中转优先试 Sub2API 的 /v1/usage（用 sk- Key 即可），再回退通用 /user/balance 与 NewAPI。
package custombalance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"web2api/internal/engine/upstream"
)

// Result 余额查询结果。
type Result struct {
	Provider  string // DeepSeek / Sub2API / OpenRouter …
	Summary   string // 管理台展示文案，如「剩余 12.34 USD」
	Remaining float64
	Unit      string
}

type providerKind int

const (
	providerUnknown providerKind = iota
	providerDeepSeek
	providerStepFun
	providerSiliconFlowCN
	providerSiliconFlowEN
	providerOpenRouter
	providerNovita
	providerMiniMaxCN // api.minimaxi.com / api.minimax.cn
	providerMiniMaxEN // api.minimax.io
	providerCompat    // OpenAI 兼容中转：级联探测
)

// Detect 根据 base_url 识别余额接口类型。
func Detect(baseURL string) providerKind {
	u := strings.ToLower(baseURL)
	switch {
	case strings.Contains(u, "api.deepseek.com"):
		return providerDeepSeek
	case strings.Contains(u, "api.stepfun.ai"), strings.Contains(u, "api.stepfun.com"):
		return providerStepFun
	case strings.Contains(u, "api.siliconflow.cn"):
		return providerSiliconFlowCN
	case strings.Contains(u, "api.siliconflow.com"):
		return providerSiliconFlowEN
	case strings.Contains(u, "openrouter.ai"):
		return providerOpenRouter
	case strings.Contains(u, "api.novita.ai"):
		return providerNovita
	case hostContains(u, "api.minimax.io"):
		return providerMiniMaxEN
	case hostContains(u, "api.minimaxi.com"), hostContains(u, "api.minimax.cn"), hostContains(u, "www.minimaxi.com"):
		return providerMiniMaxCN
	default:
		return providerCompat
	}
}

// hostContains 避免 api.minimax.io.example.com 误匹配。
func hostContains(baseURL, host string) bool {
	u := strings.ToLower(baseURL)
	if !strings.Contains(u, "://") {
		u = "https://" + u
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return strings.Contains(baseURL, host)
	}
	h := strings.ToLower(parsed.Hostname())
	return h == host || strings.HasSuffix(h, "."+host)
}

// Query 查询余额。
func Query(ctx context.Context, client *http.Client, baseURL, apiKey string) (*Result, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("api_key 为空")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	kind := Detect(baseURL)
	switch kind {
	case providerDeepSeek:
		return queryDeepSeek(ctx, client, apiKey)
	case providerStepFun:
		return queryStepFun(ctx, client, apiKey)
	case providerSiliconFlowCN:
		return querySiliconFlow(ctx, client, apiKey, true)
	case providerSiliconFlowEN:
		return querySiliconFlow(ctx, client, apiKey, false)
	case providerOpenRouter:
		return queryOpenRouter(ctx, client, apiKey)
	case providerNovita:
		return queryNovita(ctx, client, apiKey)
	case providerMiniMaxCN:
		return queryMiniMax(ctx, client, apiKey, true)
	case providerMiniMaxEN:
		return queryMiniMax(ctx, client, apiKey, false)
	default:
		return queryCompatCascade(ctx, client, baseURL, apiKey)
	}
}

// queryCompatCascade 对齐 OpenAI 兼容 Key 的常用模板顺序：
// 1) Sub2API GET /v1/usage（sk- Key）
// 2) 通用 GET /user/balance
// 3) NewAPI GET /api/user/self（部分站也接受 sk-）
func queryCompatCascade(ctx context.Context, client *http.Client, baseURL, apiKey string) (*Result, error) {
	var errs []string
	if r, err := querySub2API(ctx, client, baseURL, apiKey); err == nil {
		return r, nil
	} else if err != nil {
		errs = append(errs, "usage: "+err.Error())
	}
	if r, err := queryGenericBalance(ctx, client, baseURL, apiKey); err == nil {
		return r, nil
	} else if err != nil {
		errs = append(errs, "balance: "+err.Error())
	}
	if r, err := queryNewAPI(ctx, client, baseURL, apiKey); err == nil {
		return r, nil
	} else if err != nil {
		errs = append(errs, "newapi: "+err.Error())
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("余额接口均不可用")
	}
	return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
}

func queryDeepSeek(ctx context.Context, client *http.Client, apiKey string) (*Result, error) {
	body, err := getJSON(ctx, client, "https://api.deepseek.com/user/balance", apiKey)
	if err != nil {
		return nil, err
	}
	infos, _ := body["balance_infos"].([]any)
	var total float64
	unit := "CNY"
	for _, raw := range infos {
		info, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if c, ok := info["currency"].(string); ok && c != "" {
			unit = c
		}
		total += asFloat(info["total_balance"])
	}
	return &Result{
		Provider:  "DeepSeek",
		Remaining: total,
		Unit:      unit,
		Summary:   formatMoney("余额", total, unit),
	}, nil
}

func queryStepFun(ctx context.Context, client *http.Client, apiKey string) (*Result, error) {
	body, err := getJSON(ctx, client, "https://api.stepfun.com/v1/accounts", apiKey)
	if err != nil {
		return nil, err
	}
	bal := asFloat(body["balance"])
	return &Result{
		Provider:  "StepFun",
		Remaining: bal,
		Unit:      "CNY",
		Summary:   formatMoney("余额", bal, "CNY"),
	}, nil
}

func querySiliconFlow(ctx context.Context, client *http.Client, apiKey string, isCN bool) (*Result, error) {
	domain := "api.siliconflow.com"
	unit := "USD"
	if isCN {
		domain = "api.siliconflow.cn"
		unit = "CNY"
	}
	body, err := getJSON(ctx, client, "https://"+domain+"/v1/user/info", apiKey)
	if err != nil {
		return nil, err
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return nil, fmt.Errorf("响应缺少 data")
	}
	bal := asFloat(data["totalBalance"])
	return &Result{
		Provider:  "SiliconFlow",
		Remaining: bal,
		Unit:      unit,
		Summary:   formatMoney("余额", bal, unit),
	}, nil
}

func queryOpenRouter(ctx context.Context, client *http.Client, apiKey string) (*Result, error) {
	body, err := getJSON(ctx, client, "https://openrouter.ai/api/v1/credits", apiKey)
	if err != nil {
		return nil, err
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		data = body
	}
	credits := asFloat(data["total_credits"])
	used := asFloat(data["total_usage"])
	remaining := credits - used
	return &Result{
		Provider:  "OpenRouter",
		Remaining: remaining,
		Unit:      "USD",
		Summary:   formatMoney("剩余", remaining, "USD"),
	}, nil
}

func queryNovita(ctx context.Context, client *http.Client, apiKey string) (*Result, error) {
	body, err := getJSON(ctx, client, "https://api.novita.ai/v3/user/balance", apiKey)
	if err != nil {
		return nil, err
	}
	available := asFloat(body["availableBalance"]) / 10000
	return &Result{
		Provider:  "Novita",
		Remaining: available,
		Unit:      "USD",
		Summary:   formatMoney("余额", available, "USD"),
	}, nil
}

// queryMiniMax 对齐 Token Plan：GET /v1/api/openplatform/coding_plan/remains
// 国内额度域名固定 api.minimaxi.com（api.minimax.cn 仅推理）；海外 api.minimax.io。
func queryMiniMax(ctx context.Context, client *http.Client, apiKey string, isCN bool) (*Result, error) {
	domain := "api.minimax.io"
	if isCN {
		domain = "api.minimaxi.com"
	}
	body, err := getJSON(ctx, client, "https://"+domain+"/v1/api/openplatform/coding_plan/remains", apiKey)
	if err != nil {
		return nil, err
	}
	if base, ok := body["base_resp"].(map[string]any); ok {
		code := int64(asFloat(base["status_code"]))
		if code != 0 {
			msg, _ := base["status_msg"].(string)
			if msg == "" {
				msg = "未知错误"
			}
			return nil, fmt.Errorf("MiniMax API error %d: %s", code, msg)
		}
	}
	summary, remain5h, ok := parseMiniMaxSummary(body)
	if !ok {
		return nil, fmt.Errorf("MiniMax 响应无 general 编程套餐额度")
	}
	return &Result{
		Provider:  "MiniMax",
		Remaining: remain5h,
		Unit:      "%",
		Summary:   summary,
	}, nil
}

// parseMiniMaxSummary 取 model_name=general；展示「5h剩余 x% · 周剩余 y%」。
func parseMiniMaxSummary(body map[string]any) (summary string, remain5h float64, ok bool) {
	remains, _ := body["model_remains"].([]any)
	var general map[string]any
	for _, raw := range remains {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := item["model_name"].(string)
		if name == "general" {
			general = item
			break
		}
	}
	if general == nil {
		return "", 0, false
	}
	parts := make([]string, 0, 2)
	if _, has := general["current_interval_remaining_percent"]; has {
		remain5h = asFloat(general["current_interval_remaining_percent"])
		parts = append(parts, fmt.Sprintf("5h剩余 %.0f%%", remain5h))
	}
	// current_weekly_status==1 才有周限额；3 表示无周限
	if int64(asFloat(general["current_weekly_status"])) == 1 {
		if _, has := general["current_weekly_remaining_percent"]; has {
			week := asFloat(general["current_weekly_remaining_percent"])
			parts = append(parts, fmt.Sprintf("周剩余 %.0f%%", week))
		}
	} else if int64(asFloat(general["current_weekly_status"])) == 3 {
		parts = append(parts, "周无限")
	}
	if len(parts) == 0 {
		return "", 0, false
	}
	return strings.Join(parts, " · "), remain5h, true
}

// querySub2API 对齐 Sub2API 模板：GET {base}/v1/usage，Authorization: Bearer sk-…
func querySub2API(ctx context.Context, client *http.Client, baseURL, apiKey string) (*Result, error) {
	root := upstream.NormalizeBaseURL(baseURL)
	if root == "" {
		return nil, fmt.Errorf("base_url 为空")
	}
	body, err := getJSON(ctx, client, root+"/v1/usage", apiKey)
	if err != nil {
		return nil, err
	}
	unit := "USD"
	if u, ok := body["unit"].(string); ok && u != "" {
		unit = u
	}
	quota, _ := body["quota"].(map[string]any)
	if quota != nil {
		if u, ok := quota["unit"].(string); ok && u != "" {
			unit = u
		}
	}

	var remaining float64
	var hasRemaining bool
	if v, ok := body["remaining"]; ok && v != nil {
		remaining = asFloat(v)
		hasRemaining = true
	}
	if !hasRemaining && quota != nil {
		limit := asFloat(quota["limit"])
		used := asFloat(quota["used"])
		if limit > 0 {
			remaining = limit - used
			hasRemaining = true
		}
	}
	// 部分站把余额放在 balance / credit / total_available
	if !hasRemaining {
		for _, key := range []string{"balance", "credit", "credits", "total_available", "available"} {
			if v, ok := body[key]; ok && v != nil {
				remaining = asFloat(v)
				hasRemaining = true
				break
			}
		}
	}
	if !hasRemaining {
		return nil, fmt.Errorf("/v1/usage 无 remaining/quota 字段")
	}

	plan := ""
	if p, ok := body["planName"].(string); ok {
		plan = p
	}
	summary := formatMoney("剩余", remaining, unit)
	if plan != "" {
		summary = plan + " · " + summary
	}
	return &Result{
		Provider:  "Sub2API",
		Remaining: remaining,
		Unit:      unit,
		Summary:   summary,
	}, nil
}

// queryGenericBalance 对齐通用模板：GET {base}/user/balance
func queryGenericBalance(ctx context.Context, client *http.Client, baseURL, apiKey string) (*Result, error) {
	root := upstream.NormalizeBaseURL(baseURL)
	endpoints := []string{root + "/user/balance"}
	if origin, err := originOf(baseURL); err == nil && origin+"/" != root+"/" {
		endpoints = append(endpoints, origin+"/user/balance")
	}
	var lastErr error
	for _, ep := range endpoints {
		body, err := getJSON(ctx, client, ep, apiKey)
		if err != nil {
			lastErr = err
			continue
		}
		bal := asFloat(body["balance"])
		if bal == 0 {
			bal = asFloat(body["remaining"])
		}
		if bal == 0 {
			if data, ok := body["data"].(map[string]any); ok {
				bal = asFloat(data["balance"])
				if bal == 0 {
					bal = asFloat(data["remaining"])
				}
			}
		}
		unit := "USD"
		if u, ok := body["unit"].(string); ok && u != "" {
			unit = u
		}
		return &Result{
			Provider:  "通用",
			Remaining: bal,
			Unit:      unit,
			Summary:   formatMoney("余额", bal, unit),
		}, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("/user/balance 不可用")
}

// queryNewAPI：GET {origin}/api/user/self
// NewAPI 的 quota 字段本身就是剩余额度（单位 500000 = $1），不是总额。
func queryNewAPI(ctx context.Context, client *http.Client, baseURL, apiKey string) (*Result, error) {
	origin, err := originOf(baseURL)
	if err != nil {
		return nil, err
	}
	body, err := getJSON(ctx, client, origin+"/api/user/self", apiKey)
	if err != nil {
		return nil, err
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		data = body
	}
	quota := asFloat(data["quota"])
	used := asFloat(data["used_quota"])
	if quota == 0 && used == 0 {
		if bal := asFloat(data["balance"]); bal != 0 {
			return &Result{
				Provider:  "NewAPI",
				Remaining: bal,
				Unit:      "USD",
				Summary:   formatMoney("余额", bal, "USD"),
			}, nil
		}
		return nil, fmt.Errorf("NewAPI 响应无额度字段")
	}
	// remaining = quota / 500000（quota 即剩余）
	remainingUSD := quota / 500000
	usedUSD := used / 500000
	summary := formatMoney("剩余", remainingUSD, "USD")
	if usedUSD > 0 {
		summary = fmt.Sprintf("%s（已用 %.2f）", summary, usedUSD)
	}
	if g, ok := data["group"].(string); ok && g != "" {
		summary = g + " · " + summary
	}
	return &Result{
		Provider:  "NewAPI",
		Remaining: remainingUSD,
		Unit:      "USD",
		Summary:   summary,
	}, nil
}

func originOf(baseURL string) (string, error) {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return "", fmt.Errorf("base_url 为空")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("无法解析 base_url")
	}
	return u.Scheme + "://" + u.Host, nil
}

func getJSON(ctx context.Context, client *http.Client, endpoint, apiKey string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "web2api/custom-balance")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("鉴权失败 HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 160 {
			msg = msg[:160]
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return body, nil
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	default:
		return 0
	}
}

func formatMoney(prefix string, amount float64, unit string) string {
	if amount == float64(int64(amount)) && amount < 1e9 {
		return fmt.Sprintf("%s %.0f %s", prefix, amount, unit)
	}
	return fmt.Sprintf("%s %.2f %s", prefix, amount, unit)
}
