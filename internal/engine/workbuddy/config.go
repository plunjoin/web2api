// config.go 读取 WorkBuddy 的产品配置 GET /v3/config，拿到账号真实可用
// 的模型目录（含每个模型的积分价格、免费标识、上下文/输出上限与推理档位）。
//
// 这是 WorkBuddy 桌面 CLI 自己读的接口：User-Agent 必须是完整 CLI 代理
// （WorkBuddy/<app> <appName>/<app> CLI/<cli>），裸 WorkBuddy/<v> 会被当成
// CodeBuddy IDE 的配置（没有 cli agent 的模型列表）。
//
// 免费标识来自每个模型的 credits（"x0.00"/"0"/0 即免费）与 modelPromotions
// 里的整日限免折扣；只限夜间几小时的价格（如 Hy4 preview 的 23:00–8:00）
// 永远不算免费，否则夜里读到的列表会一整天都标成免费。
package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ConfigModel 是 /v3/config 里 cli agent 的一个模型。
type ConfigModel struct {
	ID            string
	Name          string
	Context       int64 // maxInputTokens
	Output        int64 // maxOutputTokens
	Images        bool  // supportsImages（已扣除 WorkBuddy AI 的 GLM 特例）
	Free          bool
	Rate          float64 // 积分倍率（0 = 免费）
	RateWas       float64 // 整日折扣前的原价（无折扣为 0）
	Efforts       []string
	DefaultEffort string
}

// wbConfig 是一次 /v3/config 的解析结果（cli agent 视角）。
type wbConfig struct {
	Models []ConfigModel
	ReadAt time.Time
}

// fetchConfig 拉取并解析 /v3/config。失败时返回 error，调用方退回内置目录。
func fetchConfig(ctx context.Context, proxy string, a StreamAuth) ([]ConfigModel, error) {
	endpoint := strings.TrimRight(a.Endpoint, "/")
	if endpoint == "" {
		endpoint = a.Site.endpoint()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/v3/config", nil)
	if err != nil {
		return nil, err
	}
	applySignedHeaders(req, a)
	req.Header.Set("Accept", "application/json")
	resp, err := doRequest(ctx, proxy, req, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("workbuddy config: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("workbuddy config 解析失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if env.Code != 0 {
			return nil, fmt.Errorf("workbuddy config HTTP %d (code=%d): %s", resp.StatusCode, env.Code, strings.TrimSpace(env.Msg))
		}
		return nil, fmt.Errorf("workbuddy config HTTP %d: %s", resp.StatusCode, snippet(raw))
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("workbuddy config code=%d: %s", env.Code, strings.TrimSpace(env.Msg))
	}
	var data struct {
		Models []map[string]any `json:"models"`
		Agents []struct {
			Name   string   `json:"name"`
			Models []string `json:"models"`
		} `json:"agents"`
		ModelPromotions []map[string]any `json:"modelPromotions"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, fmt.Errorf("workbuddy config data 解析失败: %w", err)
	}
	byID := map[string]map[string]any{}
	for _, m := range data.Models {
		if id := str(m["id"]); id != "" {
			byID[id] = m
		}
	}
	promotions := data.ModelPromotions
	var out []ConfigModel
	seen := map[string]bool{}
	for _, agent := range data.Agents {
		if agent.Name != "cli" {
			continue
		}
		for _, id := range agent.Models {
			if seen[id] {
				continue
			}
			seen[id] = true
			d := byID[id]
			if d == nil {
				out = append(out, ConfigModel{ID: id, Name: id})
				continue
			}
			m := ConfigModel{
				ID:      id,
				Name:    str(d["name"]),
				Context: int64(num(d["maxInputTokens"])),
				Output:  int64(num(d["maxOutputTokens"])),
			}
			if m.Name == "" {
				m.Name = id
			}
			m.Images = d["supportsImages"] == true && !textOnlyModel(a.Site, id)
			price := priceOf(d, promotions)
			m.Free, m.Rate, m.RateWas = price.Free, price.Rate, price.RateWas
			efforts, def := effortsOf(d, a.Site, id)
			m.Efforts, m.DefaultEffort = efforts, def
			out = append(out, m)
		}
	}
	return out, nil
}

// priceOf 复刻 magpie 插件的 priceOf：credits + modelPromotions → 价格。
// 夜间限时价格永远返回原价（不算免费）。
type priceInfo struct {
	Free          bool
	Rate, RateWas float64
}

func priceOf(d map[string]any, promotions []map[string]any) priceInfo {
	base, baseOK := creditsOf(d["credits"])
	mine := make([]map[string]any, 0, 2)
	for _, p := range promotions {
		if p["kind"] != "discount" {
			continue
		}
		if !listContains(modelIds(p), str(d["id"])) {
			continue
		}
		if !promoRunning(p, time.Now()) {
			continue
		}
		mine = append(mine, p)
	}
	// 按小时（daily 区间）的折扣：给原价，不算免费（夜里读到的列表不能整天标免费）。
	hasNight, nightZero := false, false
	for _, p := range mine {
		if dailyHours(p) == nil || p["discount"] == nil {
			continue
		}
		hasNight = true
		if r, ok := discountOf(p, base); ok && r == 0 {
			nightZero = true
		}
	}
	if hasNight {
		if !baseOK || (base == 0 && nightZero) {
			return priceInfo{}
		}
		return priceInfo{Rate: base}
	}
	// 整日折扣：取最低价，0 即免费，并记录原价。
	best := -1.0
	bestOK := false
	for _, p := range mine {
		if p["discount"] == nil {
			continue
		}
		if r, ok := discountOf(p, base); ok {
			if !bestOK || r < best {
				best, bestOK = r, true
			}
		}
	}
	if bestOK {
		out := priceInfo{Rate: best, Free: best == 0}
		if baseOK && base > best {
			out.RateWas = base
		}
		return out
	}
	if !baseOK {
		return priceInfo{}
	}
	return priceInfo{Free: base == 0, Rate: base}
}

// creditsOf 解析积分倍率："x0.03"、"0"、"0.00"、数字；读不出返回 ok=false。
func creditsOf(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	if s == "" {
		return 0, false
	}
	s = strings.ToLower(s)
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "credits"))
	s = strings.TrimPrefix(s, "x")
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// discountOf 读促销价：discountedCredits（"0.50x"/"0.00x"，去尾 x 后解析），
// 否则 factor×base。
func discountOf(p map[string]any, base float64) (float64, bool) {
	d, _ := p["discount"].(map[string]any)
	if d == nil {
		return 0, false
	}
	raw := strings.TrimSpace(fmt.Sprintf("%v", d["discountedCredits"]))
	raw = strings.TrimSuffix(raw, "x")
	if said, ok := creditsOf(raw); ok {
		return said, true
	}
	if f, ok := d["factor"].(float64); ok && f >= 0 {
		return base * f, true
	}
	return 0, false
}

func modelIds(p map[string]any) []string {
	var out []string
	if list, ok := p["modelIds"].([]any); ok {
		for _, v := range list {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func promoRunning(p map[string]any, now time.Time) bool {
	if p["enabled"] == false {
		return false
	}
	sched, _ := p["schedule"].(map[string]any)
	if sched == nil {
		return true // 无排期视为长期有效
	}
	if from := parseISO(sched["validFrom"]); !from.IsZero() && now.Before(from) {
		return false
	}
	if until := parseISO(sched["validUntil"]); !until.IsZero() && !now.Before(until) {
		return false
	}
	return true
}

func dailyHours(p map[string]any) []any {
	sched, _ := p["schedule"].(map[string]any)
	if sched == nil {
		return nil
	}
	if d, ok := sched["daily"].([]any); ok && len(d) > 0 {
		return d
	}
	return nil
}

func parseISO(v any) time.Time {
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	if s == "" || s == "<nil>" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

// effortsOf 复刻 magpie 的 effortsOf：模型支持的推理档位（含 none 判定）。
func effortsOf(d map[string]any, site Site, id string) ([]string, string) {
	var said []string
	if r, ok := d["reasoning"].(map[string]any); ok {
		if list, ok := r["supportedEfforts"].([]any); ok && len(list) > 0 {
			for _, v := range list {
				if s, ok := v.(string); ok {
					said = append(said, s)
				}
			}
		}
	}
	if len(said) == 0 {
		if d["supportsReasoning"] != true {
			return nil, ""
		}
		own := ""
		if r, ok := d["reasoning"].(map[string]any); ok {
			if s, ok := r["effort"].(string); ok {
				own = s
			}
		}
		set := map[string]bool{"low": true, "high": true, "max": true}
		if own != "" && containsString(effortOrder, own) {
			set[own] = true
		}
		said = nil
		for _, e := range effortOrder {
			if set[e] {
				said = append(said, e)
			}
		}
	}
	// 是否支持 none：非 onlyReasoning 模型默认能关思考（canDisableThinking 缺省视为 true），
	// 或该模型被实测为 none 生效（deepseek 的 onlyReasoning 特例）。
	onlyReasoning := d["onlyReasoning"] == true
	canOff, hasCanOff := true, false
	if r, ok := d["reasoning"].(map[string]any); ok {
		if v, ok := r["canDisableThinking"].(bool); ok {
			canOff, hasCanOff = v, true
		}
	}
	off := (!onlyReasoning && (!hasCanOff || canOff)) || offTaken(id)
	if off {
		said = append([]string{"none"}, said...)
	}
	def := ""
	if r, ok := d["reasoning"].(map[string]any); ok {
		if s, ok := r["defaultEffort"].(string); ok && containsString(said, s) {
			def = s
		} else if s, ok := r["effort"].(string); ok && containsString(said, s) {
			def = s
		}
	}
	return said, def
}

var effortOrder = []string{"low", "medium", "high", "xhigh", "max"}

func offTaken(id string) bool {
	return id == "deepseek-v4.1-flash" || id == "deepseek-v4.1-flash-sg"
}

// textOnlyModel 复刻 magpie 的 textOnly：WorkBuddy AI 的 GLM 系列上游会
// 吞掉图片，即便 config 说 supportsImages。
func textOnlyModel(site Site, id string) bool {
	return site.ID == SiteIntl.ID && strings.HasPrefix(id, "glm-")
}

func listContains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
