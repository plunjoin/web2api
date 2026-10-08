package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ==================== 账号 ====================

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("记录不存在")

// CreateAccount 新增账号。
func (s *Store) CreateAccount(engine, label, credentials string, enabled bool) (Account, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine != "a" && engine != "b" {
		return Account{}, fmt.Errorf("无效引擎: %q", engine)
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return Account{}, errors.New("账号标签不能为空")
	}
	ts := now()
	status := "initializing"
	if !enabled {
		status = "disabled"
	}
	res, err := s.db.Exec(
		`INSERT INTO accounts (engine, label, credentials, enabled, status, detail, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', ?, ?)`,
		engine, label, credentials, boolToInt(enabled), status, ts, ts)
	if err != nil {
		return Account{}, fmt.Errorf("新增账号失败: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetAccount(id)
}

// GetAccount 按 ID 查询。
func (s *Store) GetAccount(id int64) (Account, error) {
	row := s.db.QueryRow(`SELECT id, engine, label, credentials, enabled, status, detail, created_at, updated_at
		FROM accounts WHERE id = ?`, id)
	return scanAccount(row)
}

// GetAccountByLabel 按引擎与标签查询。
func (s *Store) GetAccountByLabel(engine, label string) (Account, error) {
	row := s.db.QueryRow(`SELECT id, engine, label, credentials, enabled, status, detail, created_at, updated_at
		FROM accounts WHERE engine = ? AND label = ?`, engine, label)
	return scanAccount(row)
}

// ListAccounts 全部账号。
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT id, engine, label, credentials, enabled, status, detail, created_at, updated_at
		FROM accounts ORDER BY engine, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		acc, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, acc)
	}
	return out, rows.Err()
}

// UpdateAccountStatus 更新账号运行状态。
func (s *Store) UpdateAccountStatus(id int64, status, detail string) error {
	_, err := s.db.Exec(`UPDATE accounts SET status = ?, detail = ?, updated_at = ? WHERE id = ?`,
		status, detail, now(), id)
	return err
}

// UpdateAccountCredentials 更新凭据。
func (s *Store) UpdateAccountCredentials(id int64, credentials string) error {
	_, err := s.db.Exec(`UPDATE accounts SET credentials = ?, status = 'initializing', detail = '', updated_at = ? WHERE id = ?`,
		credentials, now(), id)
	return err
}

// SetAccountEnabled 启停账号。
func (s *Store) SetAccountEnabled(id int64, enabled bool) error {
	status := "ok"
	if !enabled {
		status = "disabled"
	}
	_, err := s.db.Exec(`UPDATE accounts SET enabled = ?, status = ?, updated_at = ? WHERE id = ?`,
		boolToInt(enabled), status, now(), id)
	return err
}

// DeleteAccount 删除账号记录。
func (s *Store) DeleteAccount(id int64) error {
	res, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanAccount(row interface{ Scan(...any) error }) (Account, error) {
	var acc Account
	var enabled int
	err := row.Scan(&acc.ID, &acc.Engine, &acc.Label, &acc.Credentials, &enabled, &acc.Status, &acc.Detail, &acc.CreatedAt, &acc.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	acc.Enabled = enabled != 0
	return acc, nil
}

// ==================== API Key ====================

// CreateKey 生成并保存新 Key（sk- 前缀 + 32 位随机）。
func (s *Store) CreateKey(name string) (APIKey, error) {
	key, err := GenerateKey()
	if err != nil {
		return APIKey{}, err
	}
	ts := now()
	name = strings.TrimSpace(name)
	res, err := s.db.Exec(`INSERT INTO api_keys (key, name, enabled, created_at) VALUES (?, ?, 1, ?)`,
		key, name, ts)
	if err != nil {
		return APIKey{}, fmt.Errorf("保存 Key 失败: %w", err)
	}
	id, _ := res.LastInsertId()
	created := APIKey{ID: id, Key: key, Name: name, Enabled: true, CreatedAt: ts, Multiplier: 1, AllowedModels: []string{}}
	created.fillQuota()
	return created, nil
}

// ImportKey 导入指定 Key（用于迁移 config 的静态 key）。
func (s *Store) ImportKey(key, name string) error {
	_, err := s.db.Exec(`INSERT INTO api_keys (key, name, enabled, created_at)
		VALUES (?, ?, 1, ?) ON CONFLICT(key) DO NOTHING`, key, name, now())
	return err
}

// ListKeys 全部 Key。
func (s *Store) ListKeys() ([]APIKey, error) {
	rows, err := s.db.Query(`SELECT ` + keyColumns + ` FROM api_keys ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// KeyEnabled 校验 Key 是否有效（每请求调用，保持轻量）。
func (s *Store) KeyEnabled(key string) (bool, error) {
	var enabled int
	err := s.db.QueryRow(`SELECT enabled FROM api_keys WHERE key = ?`, key).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return enabled != 0, nil
}

// KeyExists 是否存在（含禁用）。
func (s *Store) KeyExists(key string) (bool, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM api_keys WHERE key = ?`, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// SetKeyEnabled 启停 Key。
func (s *Store) SetKeyEnabled(id int64, enabled bool) error {
	res, err := s.db.Exec(`UPDATE api_keys SET enabled = ? WHERE id = ?`, boolToInt(enabled), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteKey 删除 Key。
func (s *Store) DeleteKey(id int64) error {
	res, err := s.db.Exec(`DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GenerateKey 生成 sk- 前缀的随机 Key。
func GenerateKey() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf)
	return "sk-" + hex.EncodeToString(sum[:16]), nil
}

// ==================== 用量 ====================

// RecordUsage 记录一次请求（按分钟聚合落库以控制行数）。
// 只写旧聚合表、不扣额度；网关对话请求改用 RecordRequest。
func (s *Store) RecordUsage(apiKey, engine, model string, promptTokens, completionTokens int64, success bool) error {
	ts := now() / 60 // 分钟粒度
	// 使用事务把“更新已有聚合行 / 插入新行”串起来。SQLite 连接池虽然限制为
	// 单连接，但两个独立 Exec 仍可能被并发请求交错，导致重复聚合行。
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := recordUsageTx(tx, apiKey, engine, model, promptTokens, completionTokens, success, ts); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func recordUsageTx(tx *sql.Tx, apiKey, engine, model string, promptTokens, completionTokens int64, success bool, minute int64) error {
	res, err := tx.Exec(`
		UPDATE usage_log SET
			requests = requests + 1,
			success = success + ?,
			prompt_tokens = prompt_tokens + ?,
			completion_tokens = completion_tokens + ?
		WHERE api_key = ? AND engine = ? AND model = ? AND ts = ?`,
		boolToInt(success), promptTokens, completionTokens, apiKey, engine, model, minute)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = tx.Exec(`
		INSERT INTO usage_log (api_key, engine, model, requests, success, prompt_tokens, completion_tokens, ts)
		VALUES (?, ?, ?, 1, ?, ?, ?, ?)`,
		apiKey, engine, model, boolToInt(success), promptTokens, completionTokens, minute)
	return err
}

// UsageSummary 按时间范围聚合用量（sinceUnix 为起始秒；按 key+model 分组）。
func (s *Store) UsageSummary(sinceUnix int64) ([]UsageRow, error) {
	sinceMinute := sinceUnix / 60
	rows, err := s.db.Query(`
		SELECT api_key, engine, model,
			SUM(requests), SUM(success), SUM(prompt_tokens), SUM(completion_tokens)
		FROM usage_log WHERE ts >= ?
		GROUP BY api_key, engine, model
		ORDER BY SUM(requests) DESC`, sinceMinute)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsageRow
	for rows.Next() {
		var r UsageRow
		if err := rows.Scan(&r.APIKey, &r.Engine, &r.Model, &r.Requests, &r.SuccessRequests, &r.PromptTokens, &r.CompletionTokens); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
