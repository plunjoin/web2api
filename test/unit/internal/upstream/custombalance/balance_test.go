//go:build web2api_unit

package custombalance

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		url  string
		want providerKind
	}{
		{"https://api.deepseek.com", providerDeepSeek},
		{"https://api.deepseek.com/v1", providerDeepSeek},
		{"https://api.stepfun.com/v1", providerStepFun},
		{"https://api.siliconflow.cn/v1", providerSiliconFlowCN},
		{"https://api.siliconflow.com/v1", providerSiliconFlowEN},
		{"https://openrouter.ai/api/v1", providerOpenRouter},
		{"https://api.novita.ai/v3", providerNovita},
		{"https://api.minimaxi.com/v1", providerMiniMaxCN},
		{"https://api.minimax.cn/v1", providerMiniMaxCN},
		{"https://api.minimax.io/v1", providerMiniMaxEN},
		{"https://relay.example.com/v1", providerCompat},
	}
	for _, tc := range cases {
		if got := Detect(tc.url); got != tc.want {
			t.Fatalf("Detect(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestParseMiniMaxSummary(t *testing.T) {
	body := map[string]any{
		"model_remains": []any{
			map[string]any{
				"model_name":                          "video",
				"current_interval_remaining_percent":  50.0,
				"current_weekly_remaining_percent":    50.0,
				"current_weekly_status":               1.0,
			},
			map[string]any{
				"model_name":                         "general",
				"current_interval_remaining_percent": 98.0,
				"current_weekly_remaining_percent":   95.0,
				"current_weekly_status":              1.0,
			},
		},
	}
	summary, remain, ok := parseMiniMaxSummary(body)
	if !ok || remain != 98 || summary != "5h剩余 98% · 周剩余 95%" {
		t.Fatalf("got summary=%q remain=%v ok=%v", summary, remain, ok)
	}
}

func TestFormatMoney(t *testing.T) {
	if got := formatMoney("余额", 12, "CNY"); got != "余额 12 CNY" {
		t.Fatalf("got %q", got)
	}
	if got := formatMoney("剩余", 12.345, "USD"); got != "剩余 12.35 USD" {
		t.Fatalf("got %q", got)
	}
}

func TestAsFloat(t *testing.T) {
	if asFloat("3.14") != 3.14 {
		t.Fatal("string parse")
	}
	if asFloat(float64(2)) != 2 {
		t.Fatal("float64")
	}
}
