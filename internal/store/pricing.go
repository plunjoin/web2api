// pricing.go 模型单价与金额计费。
//
// 计费不再按「Token 积分」：每个模型配置人民币单价（元 / 百万 Token），
// 每次请求按输入、缓存命中、输出分别计价后乘以生效倍率，得到金额。
//
// 金额在库内统一用「微元」整数（1 微元 = 0.000001 元）存储，避免浮点误差：
// 单价单位是「元/百万 Token」，数值上恰好等于「微元/Token」，因此
// 金额(微元) = Σ Token × 单价 后按倍率放大。users.balance、api_keys.cost_used、
// usage_records.cost_micros、ledger.amount 全部是微元。
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
)

// MicrosPerYuan 一元包含的微元数。
const MicrosPerYuan = 1_000_000

// DefaultInputPrice / DefaultOutputPrice 是未配置单价的模型使用的兜底价
// （人民币元 / 百万 Token）。保留 ¥1 与旧「1 计费 Token = 1 微元」等价，
// 升级后历史金额无需换算。
const (
	DefaultInputPrice  = 1.0
	DefaultOutputPrice = 1.0
	// MaxTokenPrice 单个维度单价上限（元/百万 Token），防止误填。
	MaxTokenPrice = 1_000_000.0
)

// ModelPrice 模型单价（元 / 百万 Token）。CacheRead 为 -1 时表示未单独定价，
// 缓存命中的输入按 Input 价计费；0 表示缓存命中免费。
type ModelPrice struct {
	Model     string  `json:"model"`
	Input     float64 `json:"input"`
	Output    float64 `json:"output"`
	CacheRead float64 `json:"cache_read"`
	UpdatedAt int64   `json:"updated_at"`
}

// EffectiveCacheRead 缓存命中的实际单价。
func (p ModelPrice) EffectiveCacheRead() float64 {
	if p.CacheRead < 0 {
		return p.Input
	}
	return p.CacheRead
}

// DefaultModelPrice 未配置任何单价时的兜底价格。
func DefaultModelPrice() ModelPrice {
	return ModelPrice{Model: DefaultModelKey, Input: DefaultInputPrice, Output: DefaultOutputPrice, CacheRead: -1}
}

// Validate 校验单价：有限、>=0、<=MaxTokenPrice；CacheRead 允许 -1。
func (p ModelPrice) Validate() error {
	for _, v := range []struct {
		name  string
		value float64
	}{
		{"input", p.Input}, {"output", p.Output}, {"cache_read", p.CacheRead},
	} {
		if math.IsNaN(v.value) || math.IsInf(v.value, 0) || v.value < -1 || v.value > MaxTokenPrice {
			return fmt.Errorf("%s 单价必须在 0 到 %g 之间（元/百万 Token）", v.name, MaxTokenPrice)
		}
	}
	if p.CacheRead < 0 && p.CacheRead != -1 {
		return errors.New("cache_read 为 -1 表示按输入价计费，或填 0 及正数")
	}
	return nil
}

// ListModelPrices 全部模型单价（含 '*' 默认项）。
func (s *Store) ListModelPrices() ([]ModelPrice, error) {
	rows, err := s.db.Query(`SELECT model, input_price, output_price, cache_read_price, updated_at
		FROM model_prices ORDER BY model = '*' DESC, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModelPrice{}
	for rows.Next() {
		var p ModelPrice
		if err := rows.Scan(&p.Model, &p.Input, &p.Output, &p.CacheRead, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetModelPrice 新增或修改模型单价；model='*' 为默认单价。
func (s *Store) SetModelPrice(p ModelPrice) (ModelPrice, error) {
	p.Model = strings.TrimSpace(p.Model)
	if p.Model == "" {
		return ModelPrice{}, errors.New("model 不能为空（默认单价使用 \"*\"）")
	}
	if err := p.Validate(); err != nil {
		return ModelPrice{}, err
	}
	p.UpdatedAt = now()
	_, err := s.db.Exec(`INSERT INTO model_prices (model, input_price, output_price, cache_read_price, updated_at)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(model) DO UPDATE SET
		input_price = excluded.input_price, output_price = excluded.output_price,
		cache_read_price = excluded.cache_read_price, updated_at = excluded.updated_at`,
		p.Model, p.Input, p.Output, p.CacheRead, p.UpdatedAt)
	if err != nil {
		return ModelPrice{}, err
	}
	return p, nil
}

// DeleteModelPrice 删除模型单价（回落到 '*' 或兜底价）。
func (s *Store) DeleteModelPrice(model string) error {
	res, err := s.db.Exec(`DELETE FROM model_prices WHERE model = ?`, strings.TrimSpace(model))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PriceFor 取生效单价：精确匹配 → '*' 默认 → 兜底价。
func (s *Store) PriceFor(model string) (ModelPrice, error) {
	return priceFor(s.db, model)
}

func priceFor(q queryRower, model string) (ModelPrice, error) {
	p, found, err := explicitPriceFor(q, model)
	if err != nil || found {
		return p, err
	}
	return DefaultModelPrice(), nil
}

// explicitPriceFor 精确匹配 → '*'，found=false 表示两者都没有配置。
func explicitPriceFor(q queryRower, model string) (ModelPrice, bool, error) {
	var p ModelPrice
	err := q.QueryRow(`SELECT model, input_price, output_price, cache_read_price, updated_at
		FROM model_prices WHERE model IN (?, '*') ORDER BY model = '*' LIMIT 1`, strings.TrimSpace(model)).
		Scan(&p.Model, &p.Input, &p.Output, &p.CacheRead, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelPrice{}, false, nil
	}
	if err != nil {
		return ModelPrice{}, false, err
	}
	return p, true, nil
}

// priceForBilling 计费取价：先看上游实际模型，再看请求模型，最后默认价。
func priceForBilling(q queryRower, actual, requested string) (ModelPrice, error) {
	if actual = strings.TrimSpace(actual); actual != "" {
		if p, ok, err := explicitPriceFor(q, actual); err != nil || ok {
			return p, err
		}
	}
	if requested = strings.TrimSpace(requested); requested != "" && requested != actual {
		if p, ok, err := explicitPriceFor(q, requested); err != nil || ok {
			return p, err
		}
	}
	return DefaultModelPrice(), nil
}

// CostMicros 按单价计算一次请求的金额（微元），再乘以生效倍率。
// cached 含在 prompt 内，按 CacheRead 单价计费；prompt 之外的部分按 Input 计费。
func CostMicros(promptTokens, completionTokens, cachedTokens int64, price ModelPrice, multiplier float64) int64 {
	if multiplier <= 0 || (promptTokens <= 0 && completionTokens <= 0) {
		return 0
	}
	cached := cachedTokens
	if cached < 0 {
		cached = 0
	}
	if cached > promptTokens {
		cached = promptTokens
	}
	uncached := promptTokens - cached
	value := float64(uncached)*price.Input + float64(cached)*price.EffectiveCacheRead() + float64(completionTokens)*price.Output
	value *= multiplier
	return roundUpMicros(value)
}

// ScaledCostMicros 固定单价场景（视频按秒）乘倍率后的金额。
func ScaledCostMicros(micros int64, multiplier float64) int64 {
	if micros <= 0 || multiplier <= 0 {
		return 0
	}
	return roundUpMicros(float64(micros) * multiplier)
}

// FormatCNY 把微元格式化为人民币，如 ¥0.00276、¥12.50。
func FormatCNY(micros int64) string {
	sign := ""
	if micros < 0 {
		sign, micros = "-", -micros
	}
	frac := fmt.Sprintf("%06d", micros%MicrosPerYuan)
	frac = strings.TrimRight(frac, "0")
	if len(frac) < 2 {
		frac += strings.Repeat("0", 2-len(frac))
	}
	return fmt.Sprintf("%s¥%d.%s", sign, micros/MicrosPerYuan, frac)
}

// roundUpMicros 去掉浮点噪声后向上取整，保证有消耗时不会因为舍入变成 0。
func roundUpMicros(value float64) int64 {
	if value <= 0 {
		return 0
	}
	if rounded := math.Round(value); math.Abs(value-rounded) < 1e-6 {
		return int64(rounded)
	}
	return int64(math.Ceil(value))
}
