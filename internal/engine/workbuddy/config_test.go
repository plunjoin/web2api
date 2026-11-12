package workbuddy

import (
	"testing"
	"time"
)

func TestCreditsOf(t *testing.T) {
	cases := []struct {
		in   any
		want float64
		ok   bool
	}{
		{"x0.03", 0.03, true},
		{"0", 0, true},
		{"x0.00", 0, true},
		{0, 0, true},
		{0.29, 0.29, true},
		{"", 0, false},
		{nil, 0, false},
		{"x", 0, false},
	}
	for _, c := range cases {
		got, ok := creditsOf(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("creditsOf(%v) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestPriceOf(t *testing.T) {
	now := time.Now()
	dayAgo := now.Add(-24 * time.Hour).Format(time.RFC3339)
	dayLater := now.Add(24 * time.Hour).Format(time.RFC3339)
	wholeDayPromo := []map[string]any{{
		"kind": "discount", "enabled": true,
		"modelIds": []any{"m1"},
		"discount": map[string]any{"discountedCredits": "0.00x"},
		"schedule": map[string]any{"validFrom": dayAgo, "validUntil": dayLater},
	}}
	nightPromo := []map[string]any{{
		"kind": "discount", "enabled": true,
		"modelIds": []any{"m1"},
		"discount": map[string]any{"discountedCredits": "0.00x"},
		"schedule": map[string]any{"daily": []any{map[string]any{"start": "23:00", "end": "08:00"}}},
	}}

	cases := []struct {
		name string
		d    map[string]any
		prom []map[string]any
		free bool
		rate float64
		was  float64
	}{
		{"免费模型", map[string]any{"id": "m1", "credits": "0"}, nil, true, 0, 0},
		{"付费模型", map[string]any{"id": "m1", "credits": "x0.29"}, nil, false, 0.29, 0},
		{"整日限免", map[string]any{"id": "m1", "credits": "x0.29"}, wholeDayPromo, true, 0, 0.29},
		{"夜间限免不算免费", map[string]any{"id": "m1", "credits": "x0.29"}, nightPromo, false, 0.29, 0},
		{"免费模型的夜间价不算免费", map[string]any{"id": "m1", "credits": "0"}, nightPromo, false, 0, 0},
		{"无价格", map[string]any{"id": "m1"}, nil, false, 0, 0},
	}
	for _, c := range cases {
		got := priceOf(c.d, c.prom)
		if got.Free != c.free || got.Rate != c.rate || got.RateWas != c.was {
			t.Errorf("%s: priceOf = %+v, want free=%v rate=%v was=%v", c.name, got, c.free, c.rate, c.was)
		}
	}
}

func TestEffortsOfNone(t *testing.T) {
	// onlyReasoning 且不能关思考的模型（如 hy3）没有 none 档。
	d := map[string]any{
		"onlyReasoning": true,
		"reasoning": map[string]any{
			"supportedEfforts":   []any{"low", "high"},
			"canDisableThinking": false,
			"defaultEffort":      "high",
		},
	}
	efforts, def := effortsOf(d, SiteCN, "hy3")
	if containsString(efforts, "none") {
		t.Errorf("onlyReasoning 模型不应有 none 档：%v", efforts)
	}
	if def != "high" {
		t.Errorf("default effort = %q, want high", def)
	}
	// deepseek 的 onlyReasoning 特例：none 实测生效。
	d2 := map[string]any{
		"onlyReasoning": true,
		"reasoning":     map[string]any{"supportedEfforts": []any{"low", "high", "max"}, "canDisableThinking": false},
	}
	efforts2, _ := effortsOf(d2, SiteCN, "deepseek-v4.1-flash")
	if !containsString(efforts2, "none") {
		t.Errorf("deepseek-v4.1-flash 应有 none 档：%v", efforts2)
	}
}
