// quota.go 读取 WorkBuddy 账号的真实额度，并把它拆成与 magpie 一致的三窗口：
//
//	Credits        套餐基础积分（plan_base）
//	Extra credits  套餐赠送 / 充值 / 平台奖励
//	Total credits  两者之和 —— 真正会让账号停下的窗口
//
// 数据源：
//
//	POST /billing/meter/get-user-resource-summary        额度汇总
//	POST /billing/meter/get-user-resource-paid-packages 付费包（含到期时间）
//	POST /billing/meter/get-user-resource-free-packages 免费包（含每日切片）
//
// runsOutAt/runsOut 告诉路由「这个账号的积分最早什么时候作废、有多少」，
// 智能路由据此优先花掉快过期的账号。
package workbuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"web2api/internal/engine"
)

// Window 是账号的一个额度窗口。
type Window struct {
	Name      string  `json:"name"`
	Used      float64 `json:"used"`   // 已用百分比 0-100
	Amount    float64 `json:"amount"` // 已用额度
	Limit     float64 `json:"limit"`  // 总额度
	Display   string  `json:"display"`
	Unit      string  `json:"unit"`
	ResetsAt  string  `json:"resetsAt,omitempty"`
	RunsOutAt string  `json:"runsOutAt,omitempty"`
	RunsOut   float64 `json:"runsOut,omitempty"`
	Aside     bool    `json:"aside,omitempty"` // 展示但不参与路由停号
}

// Quota 是账号额度快照。
type Quota struct {
	Plan    string    `json:"plan,omitempty"`
	Windows []Window  `json:"windows,omitempty"`
	ReadAt  time.Time `json:"read_at"`
	Err     string    `json:"error,omitempty"`
}

// Exhausted 报告「真正会让账号停下的窗口」是否已用尽。
// 只有 Total credits（或只有单一 Credits 窗口）能停号；Extra 用尽不影响。
func (q *Quota) Exhausted() bool {
	if q == nil {
		return false
	}
	for _, w := range q.Windows {
		if w.Aside {
			continue
		}
		if w.Limit > 0 && w.Amount >= w.Limit {
			return true
		}
	}
	return false
}

// RunsOutAt 返回这个账号最早作废的一批积分的到期时间（无则零值）。
func (q *Quota) RunsOutAt() time.Time {
	if q == nil {
		return time.Time{}
	}
	var at time.Time
	for _, w := range q.Windows {
		if w.RunsOutAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, w.RunsOutAt)
		if err != nil {
			continue
		}
		if at.IsZero() || t.Before(at) {
			at = t
		}
	}
	return at
}

// Snapshot 保留结构化额度用于展示；不改变原有路由使用的 Quota。
func (q *Quota) Snapshot(now time.Time) *engine.QuotaSnapshot {
	if q == nil {
		return nil
	}
	s := &engine.QuotaSnapshot{Error: q.Err, Stale: q.ReadAt.IsZero() || now.Sub(q.ReadAt) > 15*time.Minute}
	if !q.ReadAt.IsZero() {
		s.ReadAt = q.ReadAt.Unix()
	}
	for _, w := range q.Windows {
		// A component expiry also changes Total credits. Retain all values as
		// history until refreshed instead of presenting a guessed new balance.
		for _, boundary := range []string{w.ResetsAt, w.RunsOutAt} {
			if boundary != "" {
				at, err := time.Parse(time.RFC3339, boundary)
				if err != nil || !at.After(now) {
					s.Stale = true
				}
			}
		}
		remaining := w.Limit - w.Amount
		if remaining < 0 {
			remaining = 0
		}
		s.Windows = append(s.Windows, engine.QuotaWindow{
			ID: w.Name, Name: w.Name, Unit: w.Unit, Limit: w.Limit, Remaining: remaining,
			ResetsAt: w.ResetsAt, ExpiresAt: w.RunsOutAt, Expiring: w.RunsOut, Aside: w.Aside,
		})
	}
	return s
}

// Display 把额度压成一行给管理台看。
func (q *Quota) Display() string {
	if q == nil {
		return ""
	}
	if q.Err != "" {
		return "额度未知: " + q.Err
	}
	parts := make([]string, 0, len(q.Windows))
	for _, w := range q.Windows {
		if w.Display != "" {
			parts = append(parts, w.Name+" "+w.Display)
		}
	}
	s := strings.Join(parts, " · ")
	if q.Exhausted() {
		if s != "" {
			s += "（已用尽）"
		} else {
			s = "已用尽"
		}
	}
	return s
}

// shortErr 把上游错误压成一行给管理台：HTML 错误页只保留状态码（否则整页
// 401 的 body 会把账号卡片撑出横向滚动条），普通错误截断到可读长度。
func shortErr(err error) string {
	if err == nil {
		return ""
	}
	var ue *UpstreamError
	if errors.As(err, &ue) {
		body := strings.TrimSpace(ue.Body)
		if strings.HasPrefix(body, "<") {
			return fmt.Sprintf("上游 HTTP %d", ue.Status)
		}
		if body == "" {
			return fmt.Sprintf("上游 HTTP %d", ue.Status)
		}
		return fmt.Sprintf("上游 HTTP %d: %s", ue.Status, clipRunes(body, 120))
	}
	return clipRunes(err.Error(), 160)
}

// clipRunes 按字符（不是字节）截断，避免把中文切坏。
func clipRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// fetchQuota 读取一个账号的额度快照。任何一步失败都返回带 Err 的快照，
// 而不是让调用方拿到 nil——额度读不到时路由按「未知」处理即可。
func fetchQuota(ctx context.Context, proxy string, a StreamAuth, plan string) *Quota {
	q := &Quota{Plan: plan, ReadAt: time.Now()}
	sum, err := meterData(ctx, proxy, a, "/billing/meter/get-user-resource-summary", map[string]any{})
	if err != nil {
		q.Err = shortErr(err)
		return q
	}
	q.Windows = buildWindows(sum)

	// 包列表用来算 runsOutAt；读不到不影响主窗口。
	if pkgs, perr := packageList(ctx, proxy, a); perr == nil {
		applyRunsOut(q, sum, pkgs)
		if cycle := cycleOf(sum, pkgs); cycle != "" {
			for i := range q.Windows {
				if q.Windows[i].Name == "Credits" || q.Windows[i].Name == "Total credits" {
					if q.Windows[i].ResetsAt == "" {
						q.Windows[i].ResetsAt = cycle
					}
				}
			}
		}
	}
	if q.Plan == "" {
		if isPaid(sum) {
			q.Plan = "Pro"
		} else {
			q.Plan = "Free plan"
		}
	}
	return q
}

// meterData 调 billing 接口并返回 data 对象（{code,msg,data} 里的 data）。
func meterData(ctx context.Context, proxy string, a StreamAuth, path string, body map[string]any) (map[string]any, error) {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Endpoint+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	applySignedHeaders(req, a)
	req.Header.Set("Content-Type", "application/json")
	resp, err := doRequest(ctx, proxy, req, 20*time.Second)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{Status: resp.StatusCode, Body: string(raw)}
	}
	var env struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("额度响应解析失败: %w", err)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("额度接口 code=%d: %s", env.Code, strings.TrimSpace(env.Msg))
	}
	if env.Data == nil {
		return nil, fmt.Errorf("额度响应缺少 data")
	}
	return env.Data, nil
}

// packageList 拉取付费 + 免费包（各最多 10 页 × 200）。
func packageList(ctx context.Context, proxy string, a StreamAuth) ([]map[string]any, error) {
	out := []map[string]any{}
	day := beijingDay(time.Now())
	lists := []struct {
		path string
		body map[string]any
	}{
		{"/billing/meter/get-user-resource-paid-packages", map[string]any{"NeedRenewInfo": true}},
		{"/billing/meter/get-user-resource-free-packages", map[string]any{
			"SlicePeriodStartTime": day + " 00:00:00",
			"SlicePeriodEndTime":   day + " 23:59:59",
		}},
	}
	for _, l := range lists {
		for page := 1; page <= 10; page++ {
			body := map[string]any{"PageNumber": page, "PageSize": 200, "Status": []int{0, 3}}
			for k, v := range l.body {
				body[k] = v
			}
			data, err := meterData(ctx, proxy, a, l.path, body)
			if err != nil {
				return out, err
			}
			got := mapList(data["Accounts"])
			out = append(out, got...)
			total := int(num(data["TotalCount"]))
			if len(got) < 200 || (total > 0 && len(out) >= total) {
				break
			}
		}
	}
	return out, nil
}

// buildWindows 把 summary 拆成三窗口，规则与 magpie usageOf/kindsOf 一致。
func buildWindows(sum map[string]any) []Window {
	planTotal, planUsed, extraTotal, extraUsed := 0.0, 0.0, 0.0, 0.0
	groups := mapList(sum["CreditGroups"])
	grouped := false
	for _, g := range groups {
		if !hasCapacity(g["CycleTotalCapacity"]) {
			continue
		}
		grouped = true
		if str(g["Kind"]) == "plan_base" {
			planTotal += num(g["CycleTotalCapacity"])
			planUsed += num(g["CycleUsedCapacity"])
		} else {
			extraTotal += num(g["CycleTotalCapacity"])
			extraUsed += num(g["CycleUsedCapacity"])
		}
	}
	if !grouped {
		subCode := str(sum["SubscriptionPackageCode"])
		for _, p := range mapList(sum["Packages"]) {
			if !hasCapacity(p["CycleTotalCapacity"]) {
				continue
			}
			if isPlanPackage(p, subCode) {
				planTotal += num(p["CycleTotalCapacity"])
				planUsed += num(p["CycleUsedCapacity"])
			} else {
				extraTotal += num(p["CycleTotalCapacity"])
				extraUsed += num(p["CycleUsedCapacity"])
			}
		}
	}

	var out []Window
	if planTotal > 0 && extraTotal > 0 {
		credits := newWindow("Credits", planUsed, planTotal)
		extra := newWindow("Extra credits", extraUsed, extraTotal)
		credits.Aside, extra.Aside = true, true
		total := newWindow("Total credits", planUsed+extraUsed, planTotal+extraTotal)
		out = append(out, credits, extra, total)
	} else if t := planTotal + extraTotal; t > 0 {
		out = append(out, newWindow("Credits", planUsed+extraUsed, t))
	}
	return out
}

func newWindow(name string, used, total float64) Window {
	w := Window{Name: name, Amount: used, Limit: total, Unit: "credits"}
	if total > 0 {
		w.Used = 100 * used / total
	}
	w.Display = fmt.Sprintf("%s / %s", compact(used), compact(total))
	return w
}

// applyRunsOut 用包列表算出最早作废的积分与到期时间，挂到不停号的那个窗口上
// （Total，或单一 Credits）。
func applyRunsOut(q *Quota, sum map[string]any, pkgs []map[string]any) {
	counted := map[string]bool{}
	for _, p := range mapList(sum["Packages"]) {
		counted[str(p["PackageCode"])] = true
	}
	now := time.Now()
	var at time.Time
	amount := 0.0
	for _, p := range pkgs {
		if !counted[str(p["PackageCode"])] {
			continue
		}
		if isSlice(p) { // 每日切片：每个账号同一时刻刷新，区分不了账号
			continue
		}
		left := num(p["CycleCapacityRemainPrecise"])
		if left <= 0 {
			left = num(p["CycleCapacityRemain"])
		}
		end := packageEnd(p)
		if left <= 0 || end.IsZero() || !end.After(now) {
			continue
		}
		if at.IsZero() || end.Before(at) {
			at, amount = end, left
		} else if end.Equal(at) {
			amount += left
		}
	}
	if at.IsZero() {
		return
	}
	for i := range q.Windows {
		if q.Windows[i].Aside {
			continue
		}
		q.Windows[i].RunsOutAt = at.Format(time.RFC3339)
		q.Windows[i].RunsOut = amount
		return
	}
}

// cycleOf 返回订阅周期结束时间（RFC3339）。免费账号没有 SubscriptionPackageCode，
// 退回每日切片包的 CycleEndTime（月周期的结束）。
func cycleOf(sum map[string]any, pkgs []map[string]any) string {
	if code := str(sum["SubscriptionPackageCode"]); code != "" {
		for _, p := range pkgs {
			if str(p["PackageCode"]) == code {
				if end := packageEnd(p); !end.IsZero() && end.After(time.Now()) {
					return end.Format(time.RFC3339)
				}
			}
		}
		return ""
	}
	for _, p := range pkgs {
		if isSlice(p) {
			if end := packageEnd(p); !end.IsZero() && end.After(time.Now()) {
				return end.Format(time.RFC3339)
			}
		}
	}
	return ""
}

func isPaid(sum map[string]any) bool {
	if v, ok := sum["IsPaidUser"].(bool); ok {
		return v
	}
	return false
}

// planPackageCodes 是「套餐基础积分」的常见包码（取自 WorkBuddy 客户端的
// resolveCreditGroup：Pro 月/年、青春、进阶、旗舰、试用、免费月等）。
// 只在 summary 没有 CreditGroups 时作为兜底分类。
var planPackageCodes = map[string]bool{
	"TCACA_code_002_AkiJS3ZHF5": true, "TCACA_code_005_maRGyrHhw1": true,
	"TCACA_code_003_FAnt7lcmRT": true, "TCACA_code_023_4xbGhMrE6q": true,
	"TCACA_code_026_BaESVICNoi": true, "TCACA_code_027_0FCGVA6vSa": true,
	"TCACA_code_043_6oIkxT7Z0S": true, "TCACA_code_035_ArVxJcGDsm": true,
	"TCACA_code_039_KRcQj7wUat": true, "TCACA_code_040_mi9rCYg46x": true,
	"TCACA_code_008_cfWoLwvjU4": true,
}

// isPlanPackage 无 CreditGroups 时按套餐码判定：订阅套餐码、常见套餐码，
// 或每日切片（CapacityType=4，套餐基础积分的一部分）都算 plan。
func isPlanPackage(p map[string]any, subCode string) bool {
	code := str(p["PackageCode"])
	if code == "" {
		return false
	}
	if subCode != "" && code == subCode {
		return true
	}
	return planPackageCodes[code] || isSlice(p)
}

func isSlice(p map[string]any) bool { return str(p["CapacityType"]) == "4" }

func hasCapacity(v any) bool {
	if v == nil {
		return false
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) != ""
	}
	return true
}

// packageEnd 是包内积分作废的时间：DeductionEndTime（毫秒）优先，
// 其次是 ExpiredTime / CycleEndTime（UTC+8 字符串）。
func packageEnd(p map[string]any) time.Time {
	if ms := num(p["DeductionEndTime"]); ms > 0 {
		return time.UnixMilli(int64(ms))
	}
	for _, key := range []string{"ExpiredTime", "CycleEndTime"} {
		if t := parseBeijing(str(p[key])); !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

func parseBeijing(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.In(time.FixedZone("CST", 8*3600))
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

// beijingDay 返回 UTC+8 时区的日期 "2006-01-02"。
func beijingDay(t time.Time) string {
	return t.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
}

// num 兼容 JSON 数字、数字字符串与 nil。
func num(v any) float64 {
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
		s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(n), "x"))
		if s == "" {
			return 0
		}
		var f float64
		fmt.Sscanf(s, "%f", &f)
		return f
	}
	return 0
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// mapList 把 []any 里能转成 map 的元素取出来。
func mapList(v any) []map[string]any {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// compact 把额度数字压成短字符串（整数不带小数点）。
func compact(n float64) string {
	if n == float64(int64(n)) {
		return fmt.Sprintf("%d", int64(n))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", n), "0"), ".")
}
