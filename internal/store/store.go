// Package store 提供号池管理平台的 SQLite 存储：
// accounts（双引擎账号）、api_keys（对外 Key 分发）、usage_log（用量明细）。
// 使用 modernc.org/sqlite（纯 Go，无 cgo）。
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store SQLite 存储。
type Store struct {
	db *sql.DB
}

// Account 账号记录（engine=a：Gemini 网页号；engine=b：AI Studio 号）。
type Account struct {
	ID          int64  `json:"id"`
	Engine      string `json:"engine"` // a | b
	Label       string `json:"label"`
	Credentials string `json:"-"` // JSON 字符串（凭据，不对外输出）
	Enabled     bool   `json:"enabled"`
	Status      string `json:"status"` // unknown | initializing | ok | error | disabled
	Detail      string `json:"detail"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// APIKey 对外分发的 Key。
type APIKey struct {
	ID        int64  `json:"id"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	// TokenLimit 是 Key 的 Token 额度（按倍率计费后的 Token），0 表示不限。
	TokenLimit int64 `json:"token_limit"`
	// TokensUsed 是已扣减的计费 Token（= Σ ceil(total_tokens × 模型倍率 × Key 倍率)）。
	TokensUsed int64 `json:"tokens_used"`
	// Multiplier 是 Key 倍率（分组默认倍率），与模型倍率相乘，默认 1。
	Multiplier float64 `json:"multiplier"`
	// TokensRemaining 剩余额度；不限额时为 null。
	TokensRemaining *int64 `json:"tokens_remaining"`
	// QuotaExhausted 额度是否已用尽（用尽后请求返回 429 insufficient_quota）。
	QuotaExhausted bool `json:"quota_exhausted"`
	// ExpiresAt 过期时间（Unix 秒），0 表示永不过期；过期后请求返回 401 key_expired。
	ExpiresAt int64 `json:"expires_at"`
	// Expired 是否已过期（派生字段）。
	Expired bool `json:"expired"`
	// AllowedModels 模型白名单；空表示允许全部。支持末尾 * 前缀匹配（如 gemini-3.5-*）。
	AllowedModels []string `json:"allowed_models"`
	// RPMLimit 每分钟请求数上限，0 表示只受全局限流约束。
	RPMLimit int64 `json:"rpm_limit"`
}

// fillQuota 计算派生字段 TokensRemaining / QuotaExhausted。
func (k *APIKey) fillQuota() {
	k.Expired = k.ExpiresAt > 0 && now() >= k.ExpiresAt
	if k.AllowedModels == nil {
		k.AllowedModels = []string{}
	}
	k.TokensRemaining = nil
	k.QuotaExhausted = false
	if k.TokenLimit > 0 {
		remaining := k.TokenLimit - k.TokensUsed
		if remaining < 0 {
			remaining = 0
		}
		k.TokensRemaining = &remaining
		k.QuotaExhausted = k.TokensUsed >= k.TokenLimit
	}
}

// UsageRow 聚合用量行。
type UsageRow struct {
	APIKey           string `json:"api_key"`
	Engine           string `json:"engine"`
	Model            string `json:"model"`
	Requests         int64  `json:"requests"`
	SuccessRequests  int64  `json:"success_requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}

// Open 打开/创建数据库并执行迁移。
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录失败: %w", err)
		}
	}
	// busy_timeout 避免并发写时的锁等待失败；WAL 提升读写并发
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", filepath.ToSlash(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	db.SetMaxOpenConns(1) // modernc sqlite 单写者，避免 SQLITE_BUSY
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭。
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS admin_settings (
			id INTEGER PRIMARY KEY CHECK(id=1),
			email TEXT NOT NULL,
			nickname TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			redis_url TEXT NOT NULL,
			jwt_secret TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			engine TEXT NOT NULL CHECK(engine IN ('a','b')),
			label TEXT NOT NULL,
			credentials TEXT NOT NULL DEFAULT '{}',
			enabled INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'unknown',
			detail TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_engine_label ON accounts(engine, label)`,
		`CREATE TABLE IF NOT EXISTS cookie_sessions (
			account_id INTEGER PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
			data BLOB NOT NULL, updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS gemini_upload_sessions (
			id TEXT PRIMARY KEY, url TEXT NOT NULL, expires_at INTEGER NOT NULL
		)`,
		`CREATE TRIGGER IF NOT EXISTS invalidate_cookie_session AFTER UPDATE OF credentials ON accounts
			WHEN NEW.credentials != OLD.credentials BEGIN DELETE FROM cookie_sessions WHERE account_id=NEW.id; END`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usage_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			api_key TEXT NOT NULL,
			engine TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			requests INTEGER NOT NULL DEFAULT 1,
			success INTEGER NOT NULL DEFAULT 1,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			ts INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_ts ON usage_log(ts)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_key ON usage_log(api_key, ts)`,
		// 模型倍率：model='*' 为未单独配置模型时的默认倍率。
		`CREATE TABLE IF NOT EXISTS model_multipliers (
			model TEXT PRIMARY KEY,
			multiplier REAL NOT NULL DEFAULT 1,
			updated_at INTEGER NOT NULL
		)`,
		// 逐请求用量明细（ts 为 Unix 秒）。estimated=1 表示 Token 数含本地估算。
		`CREATE TABLE IF NOT EXISTS usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts INTEGER NOT NULL,
			key_id INTEGER NOT NULL DEFAULT 0,
			api_key TEXT NOT NULL DEFAULT '',
			engine TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			endpoint TEXT NOT NULL DEFAULT '',
			stream INTEGER NOT NULL DEFAULT 0,
			success INTEGER NOT NULL DEFAULT 1,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			estimated INTEGER NOT NULL DEFAULT 0,
			model_multiplier REAL NOT NULL DEFAULT 1,
			key_multiplier REAL NOT NULL DEFAULT 1,
			multiplier REAL NOT NULL DEFAULT 1,
			charged_tokens INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			latency_ms INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_ts ON usage_records(ts)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_key ON usage_records(key_id, ts)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("迁移失败: %w", err)
		}
	}
	// 旧库补列（CREATE TABLE IF NOT EXISTS 不会给已有表加列）。
	for _, column := range []struct{ table, name, ddl string }{
		{"api_keys", "token_limit", "INTEGER NOT NULL DEFAULT 0"},
		{"api_keys", "tokens_used", "INTEGER NOT NULL DEFAULT 0"},
		{"api_keys", "multiplier", "REAL NOT NULL DEFAULT 1"},
		{"api_keys", "expires_at", "INTEGER NOT NULL DEFAULT 0"},
		{"api_keys", "allowed_models", "TEXT NOT NULL DEFAULT ''"},
		{"api_keys", "rpm_limit", "INTEGER NOT NULL DEFAULT 0"},
		{"usage_records", "latency_ms", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := s.ensureColumn(column.table, column.name, column.ddl); err != nil {
			return fmt.Errorf("迁移失败: %w", err)
		}
	}
	return nil
}

// ensureColumn 在列不存在时执行 ALTER TABLE ADD COLUMN。
func (s *Store) ensureColumn(table, column, ddl string) error {
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return err
	}
	exists := false
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		if name == column {
			exists = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + ddl)
	return err
}

func now() int64 { return time.Now().Unix() }
