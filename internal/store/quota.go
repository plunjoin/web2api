package store

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ==================== Token 额度与倍率 ====================
//
// 计费规则（所有字段都在管理 API 中可见）：
//
//	有效倍率 multiplier = 模型倍率 × Key 倍率 × 用户倍率（无归属用户的 Key 用户倍率为 1）
//	计费 Token charged  = ceil(total_tokens × multiplier)（浮点误差 1e-6 内按整数处理）
//	Key 已用 tokens_used += charged（仅成功请求）
//	Key 归属用户时，同一事务内从用户余额扣除 charged 并写一条 usage 账单流水
//
// 模型倍率取 model_multipliers 中该模型的值；未配置时取 model='*' 的默认值；
// 都没有则为 1。Key 倍率默认 1，可作为分组/客户级默认倍率。
//
// 额度在请求进入时检查：tokens_used >= token_limit（token_limit>0）即拒绝。
// 请求完成后才扣减，因此跨过额度线的那一次请求会完整返回，并发请求也可能
// 共同超出少量额度；之后的请求一律拒绝。

// DefaultModelKey 是默认模型倍率的保留键。
const DefaultModelKey = "*"

// MaxMultiplier 倍率上限，防止误填导致额度瞬间耗尽。
const MaxMultiplier = 1000

const keyColumns = `id, key, name, enabled, created_at, token_limit, tokens_used, multiplier, expires_at, allowed_models, rpm_limit, user_id`

type rowScanner interface{ Scan(dest ...any) error }

func scanKey(row rowScanner) (APIKey, error) {
	var k APIKey
	var enabled int
	var allowed string
	if err := row.Scan(&k.ID, &k.Key, &k.Name, &enabled, &k.CreatedAt, &k.TokenLimit, &k.TokensUsed, &k.Multiplier,
		&k.ExpiresAt, &allowed, &k.RPMLimit, &k.UserID); err != nil {
		return APIKey{}, err
	}
	k.Enabled = enabled != 0
	k.AllowedModels = splitModels(allowed)
	k.fillQuota()
	return k, nil
}

// ValidateMultiplier 校验倍率：有限、>=0、<=MaxMultiplier。0 表示该模型/Key 不计费。
func ValidateMultiplier(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > MaxMultiplier {
		return fmt.Errorf("倍率必须是 0 到 %d 之间的数字", MaxMultiplier)
	}
	return nil
}

// EffectiveMultiplier 有效倍率 = 模型倍率 × Key 倍率。
func EffectiveMultiplier(modelMultiplier, keyMultiplier float64) float64 {
	return modelMultiplier * keyMultiplier
}

// ChargedTokens 计费 Token = ceil(total × multiplier)。
// 乘积与最近整数相差不超过 1e-6 时视为整数，避免 100×1.1=110.00000000000001 被进位成 111。
func ChargedTokens(totalTokens int64, multiplier float64) int64 {
	if totalTokens <= 0 || multiplier <= 0 {
		return 0
	}
	value := float64(totalTokens) * multiplier
	if rounded := math.Round(value); math.Abs(value-rounded) < 1e-6 {
		return int64(rounded)
	}
	return int64(math.Ceil(value))
}

// QuotaExceeded 是否已无剩余额度（limit<=0 表示不限）。
func QuotaExceeded(limit, used int64) bool {
	return limit > 0 && used >= limit
}

// KeyAuth 每请求鉴权所需的 Key 信息。
type KeyAuth struct {
	ID            int64
	Enabled       bool
	TokenLimit    int64
	TokensUsed    int64
	Multiplier    float64
	ExpiresAt     int64
	AllowedModels []string
	RPMLimit      int64
	// UserID 归属用户；0 表示无归属（config Key、管理员创建的 Key），不扣用户余额。
	UserID int64
	// UserEnabled 归属用户是否启用（无归属时为 true）。
	UserEnabled bool
	// Balance 归属用户的余额（计费 Token）。
	Balance int64
}

// BalanceExhausted 归属用户余额是否不足（<=0）。无归属 Key 永远为 false。
func (k KeyAuth) BalanceExhausted() bool { return k.UserID > 0 && k.Balance <= 0 }

// Exhausted 额度是否用尽。
func (k KeyAuth) Exhausted() bool { return QuotaExceeded(k.TokenLimit, k.TokensUsed) }

// Expired 是否已过期（atUnix 为当前 Unix 秒）。
func (k KeyAuth) Expired(atUnix int64) bool { return k.ExpiresAt > 0 && atUnix >= k.ExpiresAt }

// ModelAllowed 白名单为空时允许全部；条目支持末尾 * 前缀匹配，大小写不敏感。
func (k KeyAuth) ModelAllowed(model string) bool { return ModelAllowed(k.AllowedModels, model) }

// ModelAllowed 判断 model 是否命中白名单。
func ModelAllowed(allowed []string, model string) bool {
	if len(allowed) == 0 {
		return true
	}
	model = strings.ToLower(strings.TrimSpace(model))
	for _, pattern := range allowed {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "*" || pattern == model {
			return true
		}
		if strings.HasSuffix(pattern, "*") && strings.HasPrefix(model, strings.TrimSuffix(pattern, "*")) {
			return true
		}
	}
	return false
}

// NormalizeModels 清洗白名单：去空白、去重、保持顺序。
func NormalizeModels(models []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, m := range models {
		for _, part := range strings.Split(m, ",") {
			part = strings.TrimPrefix(strings.TrimSpace(part), "models/")
			if part == "" || seen[strings.ToLower(part)] {
				continue
			}
			seen[strings.ToLower(part)] = true
			out = append(out, part)
		}
	}
	return out
}

func splitModels(raw string) []string { return NormalizeModels([]string{raw}) }

// LookupKey 按 Key 字符串取鉴权信息（单条主键/唯一索引查询，保持轻量）。
func (s *Store) LookupKey(key string) (KeyAuth, bool, error) {
	var info KeyAuth
	var enabled int
	var allowed string
	var userEnabled int
	var userExists int
	err := s.db.QueryRow(`SELECT k.id, k.enabled, k.token_limit, k.tokens_used, k.multiplier, k.expires_at, k.allowed_models, k.rpm_limit,
		k.user_id, COALESCE(u.enabled, 1), COALESCE(u.balance, 0), CASE WHEN u.id IS NULL THEN 0 ELSE 1 END
		FROM api_keys k LEFT JOIN users u ON u.id = k.user_id AND k.user_id > 0 WHERE k.key = ?`, key).
		Scan(&info.ID, &enabled, &info.TokenLimit, &info.TokensUsed, &info.Multiplier, &info.ExpiresAt, &allowed, &info.RPMLimit,
			&info.UserID, &userEnabled, &info.Balance, &userExists)
	info.AllowedModels = splitModels(allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return KeyAuth{}, false, nil
	}
	if err != nil {
		return KeyAuth{}, false, err
	}
	info.Enabled = enabled != 0
	info.UserEnabled = userEnabled != 0
	if info.UserID > 0 && userExists == 0 {
		info.UserEnabled = false // 归属用户已删除：Key 视为不可用
	}
	return info, true, nil
}

// GetKey 按 ID 取 Key。
func (s *Store) GetKey(id int64) (APIKey, error) {
	k, err := scanKey(s.db.QueryRow(`SELECT `+keyColumns+` FROM api_keys WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, ErrNotFound
	}
	return k, err
}

// KeyUpdate 部分更新；nil 字段不修改。
type KeyUpdate struct {
	Name       *string
	Enabled    *bool
	TokenLimit *int64
	Multiplier *float64
	ResetUsage bool // tokens_used 归零（不删除用量明细）
	// ExpiresAt 过期时间 Unix 秒；0 表示永不过期。
	ExpiresAt *int64
	// AllowedModels 模型白名单；空切片表示允许全部。
	AllowedModels *[]string
	// RPMLimit 每分钟请求数上限；0 表示不单独限制。
	RPMLimit *int64
}

// Empty 是否没有任何修改。
func (u KeyUpdate) Empty() bool {
	return u.Name == nil && u.Enabled == nil && u.TokenLimit == nil && u.Multiplier == nil && !u.ResetUsage &&
		u.ExpiresAt == nil && u.AllowedModels == nil && u.RPMLimit == nil
}

// UpdateKey 部分更新 Key。
func (s *Store) UpdateKey(id int64, update KeyUpdate) (APIKey, error) {
	var sets []string
	var args []any
	if update.Name != nil {
		sets, args = append(sets, "name = ?"), append(args, strings.TrimSpace(*update.Name))
	}
	if update.Enabled != nil {
		sets, args = append(sets, "enabled = ?"), append(args, boolToInt(*update.Enabled))
	}
	if update.TokenLimit != nil {
		if *update.TokenLimit < 0 {
			return APIKey{}, errors.New("token_limit 不能为负数（0 表示不限）")
		}
		sets, args = append(sets, "token_limit = ?"), append(args, *update.TokenLimit)
	}
	if update.Multiplier != nil {
		if err := ValidateMultiplier(*update.Multiplier); err != nil {
			return APIKey{}, err
		}
		sets, args = append(sets, "multiplier = ?"), append(args, *update.Multiplier)
	}
	if update.ResetUsage {
		sets = append(sets, "tokens_used = 0")
	}
	if update.ExpiresAt != nil {
		if *update.ExpiresAt < 0 {
			return APIKey{}, errors.New("expires_at 不能为负数（0 表示永不过期）")
		}
		sets, args = append(sets, "expires_at = ?"), append(args, *update.ExpiresAt)
	}
	if update.AllowedModels != nil {
		models := NormalizeModels(*update.AllowedModels)
		if len(models) > 200 {
			return APIKey{}, errors.New("allowed_models 最多 200 项")
		}
		sets, args = append(sets, "allowed_models = ?"), append(args, strings.Join(models, ","))
	}
	if update.RPMLimit != nil {
		if *update.RPMLimit < 0 || *update.RPMLimit > 1_000_000 {
			return APIKey{}, errors.New("rpm_limit 必须在 0 到 1000000 之间（0 表示不单独限制）")
		}
		sets, args = append(sets, "rpm_limit = ?"), append(args, *update.RPMLimit)
	}
	if len(sets) == 0 {
		return s.GetKey(id)
	}
	args = append(args, id)
	res, err := s.db.Exec(`UPDATE api_keys SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return APIKey{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return APIKey{}, ErrNotFound
	}
	return s.GetKey(id)
}

// RegenerateKey 为 Key 生成新的密钥字符串，保留额度、倍率、白名单等设置；旧密钥立即失效。
func (s *Store) RegenerateKey(id int64) (APIKey, error) {
	key, err := GenerateKey()
	if err != nil {
		return APIKey{}, err
	}
	res, err := s.db.Exec(`UPDATE api_keys SET key = ? WHERE id = ?`, key, id)
	if err != nil {
		return APIKey{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return APIKey{}, ErrNotFound
	}
	return s.GetKey(id)
}

// ==================== 模型倍率 ====================

// ModelMultiplier 模型倍率配置。
type ModelMultiplier struct {
	Model      string  `json:"model"`
	Multiplier float64 `json:"multiplier"`
	UpdatedAt  int64   `json:"updated_at"`
}

// ListModelMultipliers 全部模型倍率（含 '*' 默认项）。
func (s *Store) ListModelMultipliers() ([]ModelMultiplier, error) {
	rows, err := s.db.Query(`SELECT model, multiplier, updated_at FROM model_multipliers ORDER BY model = '*' DESC, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModelMultiplier{}
	for rows.Next() {
		var m ModelMultiplier
		if err := rows.Scan(&m.Model, &m.Multiplier, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetModelMultiplier 新增或修改模型倍率；model='*' 为默认倍率。
func (s *Store) SetModelMultiplier(model string, multiplier float64) (ModelMultiplier, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelMultiplier{}, errors.New("model 不能为空（默认倍率使用 \"*\"）")
	}
	if err := ValidateMultiplier(multiplier); err != nil {
		return ModelMultiplier{}, err
	}
	ts := now()
	_, err := s.db.Exec(`INSERT INTO model_multipliers (model, multiplier, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(model) DO UPDATE SET multiplier = excluded.multiplier, updated_at = excluded.updated_at`, model, multiplier, ts)
	if err != nil {
		return ModelMultiplier{}, err
	}
	return ModelMultiplier{Model: model, Multiplier: multiplier, UpdatedAt: ts}, nil
}

// DeleteModelMultiplier 删除模型倍率（恢复默认）。
func (s *Store) DeleteModelMultiplier(model string) error {
	res, err := s.db.Exec(`DELETE FROM model_multipliers WHERE model = ?`, strings.TrimSpace(model))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

// ModelMultiplierFor 模型倍率：精确匹配 → '*' 默认 → 1。
func (s *Store) ModelMultiplierFor(model string) (float64, error) {
	return modelMultiplierFor(s.db, model)
}

func modelMultiplierFor(q queryRower, model string) (float64, error) {
	var value float64
	err := q.QueryRow(`SELECT multiplier FROM model_multipliers WHERE model IN (?, '*')
		ORDER BY model = '*' LIMIT 1`, model).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 1, nil
	}
	return value, err
}

// ==================== 逐请求用量 ====================

// UsageRecord 一次请求的用量明细。
type UsageRecord struct {
	ID               int64   `json:"id"`
	TS               int64   `json:"ts"`
	KeyID            int64   `json:"key_id"`
	APIKey           string  `json:"-"`
	KeyName          string  `json:"key_name"`
	KeyMasked        string  `json:"key_masked"`
	Engine           string  `json:"engine"`
	Model            string  `json:"model"`
	Endpoint         string  `json:"endpoint"`
	Stream           bool    `json:"stream"`
	Success          bool    `json:"success"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Estimated        bool    `json:"estimated"`
	ModelMultiplier  float64 `json:"model_multiplier"`
	KeyMultiplier    float64 `json:"key_multiplier"`
	Multiplier       float64 `json:"multiplier"`
	ChargedTokens    int64   `json:"charged_tokens"`
	Error            string  `json:"error,omitempty"`
	LatencyMs        int64   `json:"latency_ms"`
	// UserID 归属用户（0 = 无归属 Key）；UserMultiplier 为计费时的用户倍率。
	UserID         int64   `json:"user_id"`
	UserEmail      string  `json:"user_email,omitempty"`
	UserMultiplier float64 `json:"user_multiplier"`
	// BalanceAfter 本次扣费后的用户余额（仅归属用户且产生扣费时有意义）。
	BalanceAfter int64 `json:"balance_after,omitempty"`
	// Refunded 已退款的计费 Token。
	Refunded int64 `json:"refunded_tokens"`
}

// RecordRequest 写入一条用量明细并按倍率扣减 Key 额度（同一事务），
// 同时保持旧的按分钟聚合表 usage_log 兼容。返回补全倍率/计费后的记录。
func (s *Store) RecordRequest(rec UsageRecord) (UsageRecord, error) {
	if rec.TS == 0 {
		rec.TS = now()
	}
	if rec.TotalTokens < rec.PromptTokens+rec.CompletionTokens {
		rec.TotalTokens = rec.PromptTokens + rec.CompletionTokens
	}
	if len(rec.Error) > 500 {
		rec.Error = rec.Error[:500]
	}
	tx, err := s.db.Begin()
	if err != nil {
		return rec, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	rec.KeyMultiplier, rec.UserMultiplier, rec.UserID = 1, 1, 0
	var keyID, userID int64
	var keyMultiplier, userMultiplier float64
	// 优先按鉴权时的 Key ID 查找（请求期间 Key 被重新生成也能正确扣额度）。
	const keyLookup = `SELECT k.id, k.multiplier, COALESCE(u.id, 0), COALESCE(u.multiplier, 1)
		FROM api_keys k LEFT JOIN users u ON u.id = k.user_id AND k.user_id > 0 WHERE `
	lookup := tx.QueryRow(keyLookup+`k.key = ?`, rec.APIKey)
	if rec.KeyID > 0 {
		lookup = tx.QueryRow(keyLookup+`k.id = ?`, rec.KeyID)
	}
	switch scanErr := lookup.Scan(&keyID, &keyMultiplier, &userID, &userMultiplier); {
	case scanErr == nil:
		rec.KeyID, rec.KeyMultiplier, rec.UserID, rec.UserMultiplier = keyID, keyMultiplier, userID, userMultiplier
	case errors.Is(scanErr, sql.ErrNoRows):
		rec.KeyID = 0 // 无鉴权模式（anonymous）或 Key 已被删除：只记录不扣额度
	default:
		err = scanErr
		return rec, err
	}
	if rec.ModelMultiplier, err = modelMultiplierFor(tx, rec.Model); err != nil {
		return rec, err
	}
	rec.Multiplier = EffectiveMultiplier(rec.ModelMultiplier, rec.KeyMultiplier) * rec.UserMultiplier
	rec.ChargedTokens = 0
	if rec.Success {
		rec.ChargedTokens = ChargedTokens(rec.TotalTokens, rec.Multiplier)
	}
	res, err := tx.Exec(`INSERT INTO usage_records (ts, key_id, api_key, engine, model, endpoint, stream, success,
		prompt_tokens, completion_tokens, total_tokens, estimated, model_multiplier, key_multiplier, multiplier, charged_tokens, error, latency_ms,
		user_id, user_multiplier)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.TS, rec.KeyID, rec.APIKey, rec.Engine, rec.Model, rec.Endpoint, boolToInt(rec.Stream), boolToInt(rec.Success),
		rec.PromptTokens, rec.CompletionTokens, rec.TotalTokens, boolToInt(rec.Estimated),
		rec.ModelMultiplier, rec.KeyMultiplier, rec.Multiplier, rec.ChargedTokens, rec.Error, rec.LatencyMs,
		rec.UserID, rec.UserMultiplier)
	if err != nil {
		return rec, err
	}
	rec.ID, _ = res.LastInsertId()
	if rec.KeyID > 0 && rec.ChargedTokens > 0 {
		if _, err = tx.Exec(`UPDATE api_keys SET tokens_used = tokens_used + ? WHERE id = ?`, rec.ChargedTokens, rec.KeyID); err != nil {
			return rec, err
		}
	}
	if rec.UserID > 0 && rec.ChargedTokens > 0 {
		// 余额允许被跨线的这一次请求扣成负数（请求已完整返回）；之后的请求在鉴权时被 402 拒绝。
		var entry LedgerEntry
		entry, err = applyBalanceTx(tx, rec.UserID, -rec.ChargedTokens, LedgerUsage, rec.ID,
			fmt.Sprintf("%s · %d Token × %s", rec.Model, rec.TotalTokens, strconv.FormatFloat(rec.Multiplier, 'f', -1, 64)), "system", true)
		if err != nil {
			return rec, err
		}
		rec.BalanceAfter = entry.BalanceAfter
	}
	if err = recordUsageTx(tx, rec.APIKey, rec.Engine, rec.Model, rec.PromptTokens, rec.CompletionTokens, rec.Success, rec.TS/60); err != nil {
		return rec, err
	}
	err = tx.Commit()
	return rec, err
}

// UsageFilter 用量明细查询条件。
type UsageFilter struct {
	Since  int64 // Unix 秒
	KeyID  int64 // 0 = 全部
	UserID int64 // 0 = 全部；>0 只看该用户的 Key
	Model  string
	Limit  int
	Offset int
}

func (f UsageFilter) where() (string, []any) {
	clauses := []string{"r.ts >= ?"}
	args := []any{f.Since}
	if f.KeyID > 0 {
		clauses, args = append(clauses, "r.key_id = ?"), append(args, f.KeyID)
	}
	if f.Model != "" {
		clauses, args = append(clauses, "r.model = ?"), append(args, f.Model)
	}
	if f.UserID > 0 {
		clauses, args = append(clauses, "r.user_id = ?"), append(args, f.UserID)
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// ListUsageRecords 逐请求明细（新→旧），返回总条数用于分页。
func (s *Store) ListUsageRecords(filter UsageFilter) ([]UsageRecord, int64, error) {
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	where, args := filter.where()
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM usage_records r`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT r.id, r.ts, r.key_id, r.api_key, COALESCE(k.name, ''), r.engine, r.model, r.endpoint, r.stream, r.success,
		r.prompt_tokens, r.completion_tokens, r.total_tokens, r.estimated, r.model_multiplier, r.key_multiplier, r.multiplier,
		r.charged_tokens, r.error, r.latency_ms, r.user_id, COALESCE(u.email, ''), r.user_multiplier, r.refunded_tokens
		FROM usage_records r LEFT JOIN api_keys k ON k.id = r.key_id LEFT JOIN users u ON u.id = r.user_id AND r.user_id > 0`+where+` ORDER BY r.id DESC LIMIT ? OFFSET ?`,
		append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []UsageRecord{}
	for rows.Next() {
		var r UsageRecord
		var stream, success, estimated int
		if err := rows.Scan(&r.ID, &r.TS, &r.KeyID, &r.APIKey, &r.KeyName, &r.Engine, &r.Model, &r.Endpoint, &stream, &success,
			&r.PromptTokens, &r.CompletionTokens, &r.TotalTokens, &estimated, &r.ModelMultiplier, &r.KeyMultiplier, &r.Multiplier,
			&r.ChargedTokens, &r.Error, &r.LatencyMs, &r.UserID, &r.UserEmail, &r.UserMultiplier, &r.Refunded); err != nil {
			return nil, 0, err
		}
		r.Stream, r.Success, r.Estimated = stream != 0, success != 0, estimated != 0
		r.KeyMasked = MaskKey(r.APIKey)
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// UsageBreakdownRow 按 Key × 模型聚合的明细统计。
type UsageBreakdownRow struct {
	KeyID             int64  `json:"key_id"`
	KeyName           string `json:"key_name"`
	KeyMasked         string `json:"key_masked"`
	Model             string `json:"model"`
	Requests          int64  `json:"requests"`
	SuccessRequests   int64  `json:"success_requests"`
	PromptTokens      int64  `json:"prompt_tokens"`
	CompletionTokens  int64  `json:"completion_tokens"`
	TotalTokens       int64  `json:"total_tokens"`
	ChargedTokens     int64  `json:"charged_tokens"`
	EstimatedRequests int64  `json:"estimated_requests"`
	AvgLatencyMs      int64  `json:"avg_latency_ms"`
}

// UsageBreakdown 按 Key × 模型聚合 usage_records。
func (s *Store) UsageBreakdown(filter UsageFilter) ([]UsageBreakdownRow, error) {
	where, args := filter.where()
	rows, err := s.db.Query(`SELECT r.key_id, MAX(r.api_key), COALESCE(MAX(k.name), ''), r.model, COUNT(*), SUM(r.success),
		SUM(r.prompt_tokens), SUM(r.completion_tokens), SUM(r.total_tokens), SUM(r.charged_tokens), SUM(r.estimated),
		CAST(COALESCE(AVG(CASE WHEN r.success = 1 THEN r.latency_ms END), 0) AS INTEGER)
		FROM usage_records r LEFT JOIN api_keys k ON k.id = r.key_id`+where+`
		GROUP BY r.key_id, r.model ORDER BY SUM(r.charged_tokens) DESC, COUNT(*) DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageBreakdownRow{}
	for rows.Next() {
		var r UsageBreakdownRow
		var key string
		if err := rows.Scan(&r.KeyID, &key, &r.KeyName, &r.Model, &r.Requests, &r.SuccessRequests,
			&r.PromptTokens, &r.CompletionTokens, &r.TotalTokens, &r.ChargedTokens, &r.EstimatedRequests, &r.AvgLatencyMs); err != nil {
			return nil, err
		}
		r.KeyMasked = MaskKey(key)
		out = append(out, r)
	}
	return out, rows.Err()
}

// MaskKey 脱敏显示 Key：sk-abcd…wxyz。
func MaskKey(key string) string {
	if key == "" || key == "anonymous" {
		return key
	}
	if len(key) <= 12 {
		if len(key) <= 4 {
			return "****"
		}
		return key[:3] + "…" + key[len(key)-2:]
	}
	return key[:7] + "…" + key[len(key)-4:]
}

// UsagePoint 时间序列中的一个桶。
type UsagePoint struct {
	TS              int64 `json:"ts"` // 桶起点（Unix 秒）
	Requests        int64 `json:"requests"`
	SuccessRequests int64 `json:"success_requests"`
	TotalTokens     int64 `json:"total_tokens"`
	ChargedTokens   int64 `json:"charged_tokens"`
	AvgLatencyMs    int64 `json:"avg_latency_ms"`
}

// UsageTimeseries 按固定桶宽聚合 usage_records。offsetSeconds 为本地时区相对 UTC 的偏移，
// 使日桶按本地零点切分。返回值补齐空桶，便于直接画图。
func (s *Store) UsageTimeseries(filter UsageFilter, bucketSeconds, offsetSeconds, untilUnix int64) ([]UsagePoint, error) {
	if bucketSeconds <= 0 {
		bucketSeconds = 3600
	}
	where, args := filter.where()
	query := `SELECT ((r.ts + ?) / ?) * ? - ? AS bucket, COUNT(*), SUM(r.success), SUM(r.total_tokens), SUM(r.charged_tokens),
		CAST(COALESCE(AVG(CASE WHEN r.success = 1 THEN r.latency_ms END), 0) AS INTEGER)
		FROM usage_records r` + where + ` GROUP BY bucket ORDER BY bucket`
	rows, err := s.db.Query(query, append([]any{offsetSeconds, bucketSeconds, bucketSeconds, offsetSeconds}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byBucket := map[int64]UsagePoint{}
	for rows.Next() {
		var p UsagePoint
		if err := rows.Scan(&p.TS, &p.Requests, &p.SuccessRequests, &p.TotalTokens, &p.ChargedTokens, &p.AvgLatencyMs); err != nil {
			return nil, err
		}
		byBucket[p.TS] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	align := func(ts int64) int64 { return ((ts+offsetSeconds)/bucketSeconds)*bucketSeconds - offsetSeconds }
	start := filter.Since
	if start <= 0 { // 不限时间：从最早有数据的桶开始补齐
		start = untilUnix
		for ts := range byBucket {
			if ts < start {
				start = ts
			}
		}
	}
	out := []UsagePoint{}
	for ts := align(start); ts <= align(untilUnix); ts += bucketSeconds {
		p, ok := byBucket[ts]
		if !ok {
			p = UsagePoint{TS: ts}
		}
		out = append(out, p)
		if len(out) > 5000 {
			break
		}
	}
	return out, nil
}
