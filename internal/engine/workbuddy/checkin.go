// checkin.go 复刻 magpie 插件的 checkinOf：国服 WorkBuddy 的每日签到，领取当天
// 免费积分。国际服（workbuddy-ai）没有签到活动（magpie #47），调用前先判站点。
//
// 两个端点（POST，签名头与其它 billing 接口一致，路径带 /v2）：
//
//	/v2/billing/meter/checkin-activity-status  当天活动状态（active / 是否已签 / 今日积分）
//	/v2/billing/meter/daily-checkin            领取当天积分（credit / streak_days）
//
// 结果分类与 magpie 一致：done（今日已签 / 1001）、claimed（领到 credit + 连续天数）、
// inactive（活动未开放 / 1003）、ineligible（API Key 或非官方 App 登录 / 1002 或 401）。
// 分类出来的结果走返回值（不当作 error），只有拿不到明确答案的失败才返回 error。
package workbuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// checkin 执行一次签到（幂等），返回结果文案。a 必须是国服站点。
func checkin(ctx context.Context, proxy string, a StreamAuth) (string, error) {
	code, msg, data, status, err := meterEnv(ctx, proxy, a, "/v2/billing/meter/checkin-activity-status")
	if err != nil {
		return "", err
	}
	if status != http.StatusOK || code != 0 {
		if code == 1001 {
			return "今日已签到", nil
		}
		return "", fmt.Errorf("签到状态查询失败 (HTTP %d, code=%d): %s", status, code, strings.TrimSpace(msg))
	}
	if !boolVal(data["active"]) {
		return "今日签到活动未开放", nil
	}
	if boolVal(data["today_checked_in"]) || boolVal(data["todayCheckedIn"]) {
		credit := num(data["today_credit"])
		if credit <= 0 {
			credit = num(data["daily_credit"])
		}
		if credit > 0 {
			return fmt.Sprintf("今日已签到（+%s 积分）", compact(credit)), nil
		}
		return "今日已签到", nil
	}

	for attempt := 0; ; attempt++ {
		code, msg, data, status, err := meterEnv(ctx, proxy, a, "/v2/billing/meter/daily-checkin")
		if err != nil {
			return "", err
		}
		if status == http.StatusOK && code == 0 {
			credit := num(data["credit"])
			streak := int(num(data["streak_days"]))
			if credit > 0 {
				s := fmt.Sprintf("签到成功（+%s 积分", compact(credit))
				if streak > 0 {
					s += fmt.Sprintf("，连续 %d 天", streak)
				}
				return s + "）", nil
			}
			return "签到成功", nil
		}
		switch code {
		case 1001:
			return "今日已签到", nil
		case 1002:
			return "账号不符合签到条件", nil
		case 1003:
			return "今日签到活动未开放", nil
		}
		if status == http.StatusUnauthorized {
			return "签到需要官方 App 登录态，API Key 账号不能签到", nil
		}
		if (status >= 500 || code == 10000) && attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		return "", fmt.Errorf("签到失败 (HTTP %d, code=%d): %s", status, code, strings.TrimSpace(msg))
	}
}

// meterEnv 调 billing 接口并原样返回 {code,msg,data} 与 HTTP 状态（不折叠非零 code，
// 签到要靠 code 1001/1002/1003 分类）。
func meterEnv(ctx context.Context, proxy string, a StreamAuth, path string) (code int, msg string, data map[string]any, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Endpoint+path, bytes.NewReader([]byte("{}")))
	if err != nil {
		return 0, "", nil, 0, err
	}
	applySignedHeaders(req, a)
	req.Header.Set("Content-Type", "application/json")
	resp, err := doRequest(ctx, proxy, req, 20*time.Second)
	if err != nil {
		return 0, "", nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var env struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return 0, "", nil, resp.StatusCode, fmt.Errorf("签到响应解析失败: %w", err)
	}
	return env.Code, env.Msg, env.Data, resp.StatusCode, nil
}

func boolVal(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		s := strings.TrimSpace(b)
		return s == "1" || strings.EqualFold(s, "true")
	case float64:
		return b != 0
	case int:
		return b != 0
	case int64:
		return b != 0
	}
	return false
}
