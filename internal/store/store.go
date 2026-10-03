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
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("迁移失败: %w", err)
		}
	}
	return nil
}

func now() int64 { return time.Now().Unix() }
