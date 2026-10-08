package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ==================== 用户、余额、账单流水与兑换码 ====================
//
// 余额以「计费 Token」为单位（与 usage_records.charged_tokens 同口径）。
// 每一次余额变动都在同一事务里写一条 ledger 记录（变动前/后余额），
// 因此任何时刻都满足：users.balance = Σ ledger.amount（按用户）。

// 账单流水类型。
const (
	LedgerAdjust      = "adjust"       // 管理员加减余额
	LedgerRedeem      = "redeem"       // 兑换码充值
	LedgerUsage       = "usage"        // 请求扣费
	LedgerRefund      = "refund"       // 请求退款
	LedgerSignupBonus = "signup_bonus" // 注册赠送
)

// 用户角色。
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// 业务错误（handler 据此返回 4xx）。
var (
	ErrEmailTaken           = errors.New("该邮箱已注册")
	ErrInsufficientBalance  = errors.New("余额不足")
	ErrCodeNotFound         = errors.New("兑换码不存在")
	ErrCodeRedeemed         = errors.New("兑换码已被使用")
	ErrCodeDisabled         = errors.New("兑换码已停用")
	ErrCodeExpired          = errors.New("兑换码已过期")
	ErrAlreadyRefunded      = errors.New("该请求已退款")
	ErrNothingToRefund      = errors.New("该请求没有可退的扣费")
	ErrKeyLimitReached      = errors.New("API Key 数量已达上限")
	ErrInvalidLedgerAmount  = errors.New("金额必须是非零整数")
	ErrBalanceWouldNegative = errors.New("扣减后余额不能为负数")
)

// MaxBalanceChange 单次加减余额 / 单个兑换码面额上限（防误填）。
const MaxBalanceChange = 1_000_000_000_000

var platformSchema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL UNIQUE,
		nickname TEXT NOT NULL DEFAULT '',
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'user' CHECK(role IN ('admin','user')),
		enabled INTEGER NOT NULL DEFAULT 1,
		balance INTEGER NOT NULL DEFAULT 0,
		multiplier REAL NOT NULL DEFAULT 1,
		note TEXT NOT NULL DEFAULT '',
		token_version INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		last_login_at INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS ledger (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		kind TEXT NOT NULL,
		amount INTEGER NOT NULL,
		balance_before INTEGER NOT NULL,
		balance_after INTEGER NOT NULL,
		ref_id INTEGER NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT '',
		operator TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ledger_user ON ledger(user_id, id)`,
	`CREATE INDEX IF NOT EXISTS idx_ledger_ts ON ledger(ts)`,
	`CREATE TABLE IF NOT EXISTS redeem_codes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT NOT NULL UNIQUE,
		amount INTEGER NOT NULL CHECK(amount > 0),
		batch TEXT NOT NULL DEFAULT '',
		note TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		expires_at INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		created_by TEXT NOT NULL DEFAULT '',
		redeemed_by INTEGER NOT NULL DEFAULT 0,
		redeemed_email TEXT NOT NULL DEFAULT '',
		redeemed_at INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS idx_redeem_batch ON redeem_codes(batch)`,
	`CREATE TABLE IF NOT EXISTS platform_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
}

// ---------- 用户 ----------

// User 平台用户（管理员之外的登录主体；role=admin 的用户也可进入管理台）。
type User struct {
	ID           int64   `json:"id"`
	Email        string  `json:"email"`
	Nickname     string  `json:"nickname"`
	PasswordHash string  `json:"-"`
	Role         string  `json:"role"`
	Enabled      bool    `json:"enabled"`
	Balance      int64   `json:"balance"`
	Multiplier   float64 `json:"multiplier"`
	Note         string  `json:"note"`
	TokenVersion int64   `json:"-"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
	LastLoginAt  int64   `json:"last_login_at"`
}

const userColumns = `id, email, nickname, password_hash, role, enabled, balance, multiplier, note, token_version, created_at, updated_at, last_login_at`

func scanUser(row rowScanner) (User, error) {
	var u User
	var enabled int
	err := row.Scan(&u.ID, &u.Email, &u.Nickname, &u.PasswordHash, &u.Role, &enabled, &u.Balance, &u.Multiplier, &u.Note,
		&u.TokenVersion, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	u.Enabled = enabled != 0
	return u, err
}

// NormalizeEmail 统一邮箱大小写与空白。
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// ValidateEmail 校验邮箱格式。
func ValidateEmail(email string) error {
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		return errors.New("请输入有效邮箱")
	}
	return nil
}

// ValidateNickname 1–64 字。
func ValidateNickname(nickname string) error {
	if n := utf8.RuneCountInString(strings.TrimSpace(nickname)); n == 0 || n > 64 {
		return errors.New("昵称需为 1–64 个字")
	}
	return nil
}

// NewUser 创建用户的参数。
type NewUser struct {
	Email        string
	Nickname     string
	PasswordHash string
	Role         string
	Multiplier   float64
	Note         string
	// InitialBalance 初始余额（>0 时写一条流水，类型为 InitialKind）。
	InitialBalance int64
	InitialKind    string
	InitialNote    string
	Operator       string
}

// CreateUser 创建用户；初始余额与流水在同一事务。
func (s *Store) CreateUser(in NewUser) (User, error) {
	in.Email = NormalizeEmail(in.Email)
	if err := ValidateEmail(in.Email); err != nil {
		return User{}, err
	}
	in.Nickname = strings.TrimSpace(in.Nickname)
	if in.Nickname == "" {
		in.Nickname = strings.SplitN(in.Email, "@", 2)[0]
	}
	if err := ValidateNickname(in.Nickname); err != nil {
		return User{}, err
	}
	if in.Role == "" {
		in.Role = RoleUser
	}
	if in.Role != RoleUser && in.Role != RoleAdmin {
		return User{}, errors.New("角色只能是 admin 或 user")
	}
	if in.Multiplier == 0 {
		in.Multiplier = 1
	}
	if err := ValidateMultiplier(in.Multiplier); err != nil {
		return User{}, err
	}
	if in.InitialBalance < 0 || in.InitialBalance > MaxBalanceChange {
		return User{}, errors.New("初始余额无效")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()
	ts := now()
	res, err := tx.Exec(`INSERT INTO users (email, nickname, password_hash, role, enabled, balance, multiplier, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, 0, ?, ?, ?, ?)`, in.Email, in.Nickname, in.PasswordHash, in.Role, in.Multiplier, strings.TrimSpace(in.Note), ts, ts)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	id, _ := res.LastInsertId()
	if in.InitialBalance > 0 {
		kind := in.InitialKind
		if kind == "" {
			kind = LedgerAdjust
		}
		if _, err := applyBalanceTx(tx, id, in.InitialBalance, kind, 0, in.InitialNote, in.Operator, false); err != nil {
			return User{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return s.GetUser(id)
}

// GetUser 按 ID 查询。
func (s *Store) GetUser(id int64) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

// GetUserByEmail 按邮箱查询（含密码哈希，供登录）。
func (s *Store) GetUserByEmail(email string) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE email = ?`, NormalizeEmail(email)))
}

// UserSummary 管理台用户列表行。
type UserSummary struct {
	User
	KeyCount      int64 `json:"key_count"`
	Charged7d     int64 `json:"charged_7d"`
	Requests7d    int64 `json:"requests_7d"`
	TotalRecharge int64 `json:"total_recharge"`
}

// UserFilter 用户列表查询条件。
type UserFilter struct {
	Query  string // 邮箱/昵称模糊匹配
	Role   string
	Status string // enabled | disabled
	Limit  int
	Offset int
}

// ListUsers 用户列表（新→旧）。
func (s *Store) ListUsers(f UserFilter) ([]UserSummary, int64, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	clauses, args := []string{"1=1"}, []any{}
	if q := strings.TrimSpace(f.Query); q != "" {
		clauses, args = append(clauses, "(u.email LIKE ? OR u.nickname LIKE ? OR CAST(u.id AS TEXT) = ?)"), append(args, "%"+q+"%", "%"+q+"%", q)
	}
	if f.Role != "" {
		clauses, args = append(clauses, "u.role = ?"), append(args, f.Role)
	}
	switch f.Status {
	case "enabled":
		clauses = append(clauses, "u.enabled = 1")
	case "disabled":
		clauses = append(clauses, "u.enabled = 0")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users u`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	since := now() - 7*86400
	rows, err := s.db.Query(`SELECT u.id, u.email, u.nickname, u.password_hash, u.role, u.enabled, u.balance, u.multiplier, u.note,
		u.token_version, u.created_at, u.updated_at, u.last_login_at,
		(SELECT COUNT(*) FROM api_keys k WHERE k.user_id = u.id),
		(SELECT COALESCE(SUM(r.charged_tokens), 0) FROM usage_records r WHERE r.user_id = u.id AND r.ts >= ?),
		(SELECT COUNT(*) FROM usage_records r WHERE r.user_id = u.id AND r.ts >= ?),
		(SELECT COALESCE(SUM(l.amount), 0) FROM ledger l WHERE l.user_id = u.id AND l.kind IN ('adjust','redeem','signup_bonus') AND l.amount > 0)
		FROM users u`+where+` ORDER BY u.id DESC LIMIT ? OFFSET ?`, append([]any{since, since}, append(args, f.Limit, f.Offset)...)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []UserSummary{}
	for rows.Next() {
		var u UserSummary
		var enabled int
		if err := rows.Scan(&u.ID, &u.Email, &u.Nickname, &u.PasswordHash, &u.Role, &enabled, &u.Balance, &u.Multiplier, &u.Note,
			&u.TokenVersion, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt, &u.KeyCount, &u.Charged7d, &u.Requests7d, &u.TotalRecharge); err != nil {
			return nil, 0, err
		}
		u.Enabled = enabled != 0
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// UserUpdate 部分更新；nil 字段不修改。PasswordHash 非空时同时作废已签发的登录态。
type UserUpdate struct {
	Nickname     *string
	Role         *string
	Enabled      *bool
	Multiplier   *float64
	Note         *string
	PasswordHash *string
}

// UpdateUser 部分更新用户。
func (s *Store) UpdateUser(id int64, u UserUpdate) (User, error) {
	var sets []string
	var args []any
	if u.Nickname != nil {
		if err := ValidateNickname(*u.Nickname); err != nil {
			return User{}, err
		}
		sets, args = append(sets, "nickname = ?"), append(args, strings.TrimSpace(*u.Nickname))
	}
	if u.Role != nil {
		if *u.Role != RoleUser && *u.Role != RoleAdmin {
			return User{}, errors.New("角色只能是 admin 或 user")
		}
		sets, args = append(sets, "role = ?", "token_version = token_version + 1"), append(args, *u.Role)
	}
	if u.Enabled != nil {
		sets, args = append(sets, "enabled = ?"), append(args, boolToInt(*u.Enabled))
		if !*u.Enabled {
			sets = append(sets, "token_version = token_version + 1")
		}
	}
	if u.Multiplier != nil {
		if err := ValidateMultiplier(*u.Multiplier); err != nil {
			return User{}, err
		}
		sets, args = append(sets, "multiplier = ?"), append(args, *u.Multiplier)
	}
	if u.Note != nil {
		if len(*u.Note) > 500 {
			return User{}, errors.New("备注最长 500 字节")
		}
		sets, args = append(sets, "note = ?"), append(args, strings.TrimSpace(*u.Note))
	}
	if u.PasswordHash != nil {
		sets, args = append(sets, "password_hash = ?", "token_version = token_version + 1"), append(args, *u.PasswordHash)
	}
	if len(sets) == 0 {
		return s.GetUser(id)
	}
	sets, args = append(sets, "updated_at = ?"), append(args, now(), id)
	res, err := s.db.Exec(`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return User{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return User{}, ErrNotFound
	}
	return s.GetUser(id)
}

// TouchUserLogin 记录最近登录时间。
func (s *Store) TouchUserLogin(id int64) {
	_, _ = s.db.Exec(`UPDATE users SET last_login_at = ? WHERE id = ?`, now(), id)
}

// DeleteUser 删除用户及其 API Key 与账单流水。用量明细保留（user_id 仍指向原 ID，便于对账）；
// 兑换记录保留兑换时的邮箱快照。
func (s *Store) DeleteUser(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(`DELETE FROM api_keys WHERE user_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ledger WHERE user_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// CountUsers 用户总数与启用数。
func (s *Store) CountUsers() (total, enabled int64, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(enabled), 0) FROM users`).Scan(&total, &enabled)
	return
}

// ---------- 余额与流水 ----------

// LedgerEntry 一条余额变动。
type LedgerEntry struct {
	ID            int64  `json:"id"`
	TS            int64  `json:"ts"`
	UserID        int64  `json:"user_id"`
	UserEmail     string `json:"user_email,omitempty"`
	Kind          string `json:"kind"`
	Amount        int64  `json:"amount"`
	BalanceBefore int64  `json:"balance_before"`
	BalanceAfter  int64  `json:"balance_after"`
	RefID         int64  `json:"ref_id"`
	Note          string `json:"note"`
	Operator      string `json:"operator"`
}

// applyBalanceTx 在事务内改余额并写流水。allowNegative=false 时结果不得为负。
func applyBalanceTx(tx *sql.Tx, userID, amount int64, kind string, refID int64, note, operator string, allowNegative bool) (LedgerEntry, error) {
	var before int64
	if err := tx.QueryRow(`SELECT balance FROM users WHERE id = ?`, userID).Scan(&before); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LedgerEntry{}, ErrNotFound
		}
		return LedgerEntry{}, err
	}
	after := before + amount
	if after < 0 && !allowNegative {
		return LedgerEntry{}, ErrBalanceWouldNegative
	}
	ts := now()
	if _, err := tx.Exec(`UPDATE users SET balance = ?, updated_at = ? WHERE id = ?`, after, ts, userID); err != nil {
		return LedgerEntry{}, err
	}
	if len(note) > 500 {
		note = note[:500]
	}
	entry := LedgerEntry{TS: ts, UserID: userID, Kind: kind, Amount: amount, BalanceBefore: before, BalanceAfter: after,
		RefID: refID, Note: strings.TrimSpace(note), Operator: operator}
	res, err := tx.Exec(`INSERT INTO ledger (ts, user_id, kind, amount, balance_before, balance_after, ref_id, note, operator)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, entry.TS, userID, kind, amount, before, after, refID, entry.Note, operator)
	if err != nil {
		return LedgerEntry{}, err
	}
	entry.ID, _ = res.LastInsertId()
	return entry, nil
}

// AdjustBalance 管理员加减余额（amount 可正可负），扣减后不得为负。
func (s *Store) AdjustBalance(userID, amount int64, note, operator string) (LedgerEntry, error) {
	if amount == 0 || amount > MaxBalanceChange || amount < -MaxBalanceChange {
		return LedgerEntry{}, ErrInvalidLedgerAmount
	}
	tx, err := s.db.Begin()
	if err != nil {
		return LedgerEntry{}, err
	}
	defer func() { _ = tx.Rollback() }()
	entry, err := applyBalanceTx(tx, userID, amount, LedgerAdjust, 0, note, operator, false)
	if err != nil {
		return LedgerEntry{}, err
	}
	return entry, tx.Commit()
}

// RefundUsage 把一次请求的扣费退回用户余额（每条用量只能退一次）。
func (s *Store) RefundUsage(recordID int64, note, operator string) (LedgerEntry, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return LedgerEntry{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var userID, keyID, charged, refunded int64
	err = tx.QueryRow(`SELECT user_id, key_id, charged_tokens, refunded_tokens FROM usage_records WHERE id = ?`, recordID).
		Scan(&userID, &keyID, &charged, &refunded)
	if errors.Is(err, sql.ErrNoRows) {
		return LedgerEntry{}, ErrNotFound
	}
	if err != nil {
		return LedgerEntry{}, err
	}
	if refunded > 0 {
		return LedgerEntry{}, ErrAlreadyRefunded
	}
	if userID == 0 || charged <= 0 {
		return LedgerEntry{}, ErrNothingToRefund
	}
	if strings.TrimSpace(note) == "" {
		note = fmt.Sprintf("退回请求 #%d 的扣费", recordID)
	}
	entry, err := applyBalanceTx(tx, userID, charged, LedgerRefund, recordID, note, operator, true)
	if err != nil {
		return LedgerEntry{}, err
	}
	if _, err := tx.Exec(`UPDATE usage_records SET refunded_tokens = ? WHERE id = ?`, charged, recordID); err != nil {
		return LedgerEntry{}, err
	}
	if _, err := tx.Exec(`UPDATE api_keys SET tokens_used = MAX(tokens_used - ?, 0) WHERE id = ?`, charged, keyID); err != nil {
		return LedgerEntry{}, err
	}
	return entry, tx.Commit()
}

// LedgerFilter 流水查询条件。
type LedgerFilter struct {
	UserID int64
	Kind   string
	Query  string // 用户邮箱/备注模糊匹配（管理员）
	Since  int64
	Limit  int
	Offset int
}

// ListLedger 流水（新→旧）与总条数、筛选范围内的收支合计。
func (s *Store) ListLedger(f LedgerFilter) ([]LedgerEntry, int64, map[string]int64, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	clauses, args := []string{"l.ts >= ?"}, []any{f.Since}
	if f.UserID > 0 {
		clauses, args = append(clauses, "l.user_id = ?"), append(args, f.UserID)
	}
	if f.Kind != "" {
		clauses, args = append(clauses, "l.kind = ?"), append(args, f.Kind)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		clauses, args = append(clauses, "(u.email LIKE ? OR l.note LIKE ?)"), append(args, "%"+q+"%", "%"+q+"%")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	from := ` FROM ledger l LEFT JOIN users u ON u.id = l.user_id`
	var total, credit, debit int64
	if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN l.amount > 0 THEN l.amount END), 0),
		COALESCE(SUM(CASE WHEN l.amount < 0 THEN -l.amount END), 0)`+from+where, args...).Scan(&total, &credit, &debit); err != nil {
		return nil, 0, nil, err
	}
	rows, err := s.db.Query(`SELECT l.id, l.ts, l.user_id, COALESCE(u.email, ''), l.kind, l.amount, l.balance_before, l.balance_after,
		l.ref_id, l.note, l.operator`+from+where+` ORDER BY l.id DESC LIMIT ? OFFSET ?`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, nil, err
	}
	defer rows.Close()
	out := []LedgerEntry{}
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.ID, &e.TS, &e.UserID, &e.UserEmail, &e.Kind, &e.Amount, &e.BalanceBefore, &e.BalanceAfter,
			&e.RefID, &e.Note, &e.Operator); err != nil {
			return nil, 0, nil, err
		}
		out = append(out, e)
	}
	return out, total, map[string]int64{"credit": credit, "debit": debit}, rows.Err()
}

// LedgerIntegrity 校验每个用户 balance 是否等于其流水合计，并且流水前后余额首尾相接。
// 返回不一致的用户 ID（空表示一致）。
func (s *Store) LedgerIntegrity() ([]int64, error) {
	rows, err := s.db.Query(`SELECT u.id FROM users u
		WHERE u.balance != COALESCE((SELECT SUM(amount) FROM ledger l WHERE l.user_id = u.id), 0)
		OR EXISTS (SELECT 1 FROM ledger l WHERE l.user_id = u.id AND l.balance_after != l.balance_before + l.amount)
		OR EXISTS (SELECT 1 FROM ledger l WHERE l.user_id = u.id AND l.balance_before != COALESCE(
			(SELECT p.balance_after FROM ledger p WHERE p.user_id = l.user_id AND p.id < l.id ORDER BY p.id DESC LIMIT 1), 0))`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bad := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		bad = append(bad, id)
	}
	return bad, rows.Err()
}

// ---------- 兑换码 ----------

// RedeemCode 兑换码。
type RedeemCode struct {
	ID            int64  `json:"id"`
	Code          string `json:"code"`
	Amount        int64  `json:"amount"`
	Batch         string `json:"batch"`
	Note          string `json:"note"`
	Enabled       bool   `json:"enabled"`
	ExpiresAt     int64  `json:"expires_at"`
	CreatedAt     int64  `json:"created_at"`
	CreatedBy     string `json:"created_by"`
	RedeemedBy    int64  `json:"redeemed_by"`
	RedeemedEmail string `json:"redeemed_email"`
	RedeemedAt    int64  `json:"redeemed_at"`
	// Status 派生：unused | redeemed | disabled | expired。
	Status string `json:"status"`
}

func (c *RedeemCode) fillStatus(at int64) {
	switch {
	case c.RedeemedBy > 0 || c.RedeemedAt > 0:
		c.Status = "redeemed"
	case !c.Enabled:
		c.Status = "disabled"
	case c.ExpiresAt > 0 && at >= c.ExpiresAt:
		c.Status = "expired"
	default:
		c.Status = "unused"
	}
}

const codeColumns = `id, code, amount, batch, note, enabled, expires_at, created_at, created_by, redeemed_by, redeemed_email, redeemed_at`

func scanCode(row rowScanner) (RedeemCode, error) {
	var c RedeemCode
	var enabled int
	err := row.Scan(&c.ID, &c.Code, &c.Amount, &c.Batch, &c.Note, &enabled, &c.ExpiresAt, &c.CreatedAt, &c.CreatedBy,
		&c.RedeemedBy, &c.RedeemedEmail, &c.RedeemedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RedeemCode{}, ErrNotFound
	}
	c.Enabled = enabled != 0
	c.fillStatus(now())
	return c, err
}

// 去掉易混淆字符（0/O、1/I/L）。
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// generateCode 生成 W2A-XXXX-XXXX-XXXX-XXXX 形式的兑换码（80 bit 随机）。
func generateCode() (string, error) {
	var b strings.Builder
	b.WriteString("W2A")
	max := big.NewInt(int64(len(codeAlphabet)))
	for i := 0; i < 16; i++ {
		if i%4 == 0 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(codeAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// NormalizeCode 用户输入的兑换码：去空白、转大写。
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.Join(strings.Fields(code), ""))
}

// NewCodes 批量生成兑换码参数。
type NewCodes struct {
	Amount    int64
	Count     int
	ExpiresAt int64
	Note      string
	Batch     string
	Operator  string
}

// CreateRedeemCodes 批量生成兑换码（同一事务）。
func (s *Store) CreateRedeemCodes(in NewCodes) ([]RedeemCode, error) {
	if in.Amount <= 0 || in.Amount > MaxBalanceChange {
		return nil, errors.New("面额必须是正整数")
	}
	if in.Count <= 0 || in.Count > 1000 {
		return nil, errors.New("数量需在 1–1000 之间")
	}
	if in.ExpiresAt < 0 || (in.ExpiresAt > 0 && in.ExpiresAt <= now()) {
		return nil, errors.New("过期时间必须晚于现在（0 表示永不过期）")
	}
	if len(in.Note) > 200 {
		return nil, errors.New("备注最长 200 字节")
	}
	in.Batch = strings.TrimSpace(in.Batch)
	if in.Batch == "" {
		in.Batch = "B" + strconv.FormatInt(now(), 36)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	ts := now()
	ids := make([]int64, 0, in.Count)
	for len(ids) < in.Count {
		code, err := generateCode()
		if err != nil {
			return nil, err
		}
		res, err := tx.Exec(`INSERT INTO redeem_codes (code, amount, batch, note, enabled, expires_at, created_at, created_by)
			VALUES (?, ?, ?, ?, 1, ?, ?, ?) ON CONFLICT(code) DO NOTHING`, code, in.Amount, in.Batch, strings.TrimSpace(in.Note), in.ExpiresAt, ts, in.Operator)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			id, _ := res.LastInsertId()
			ids = append(ids, id)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	out := make([]RedeemCode, 0, len(ids))
	for _, id := range ids {
		c, err := s.GetRedeemCode(id)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// GetRedeemCode 按 ID 查询。
func (s *Store) GetRedeemCode(id int64) (RedeemCode, error) {
	return scanCode(s.db.QueryRow(`SELECT `+codeColumns+` FROM redeem_codes WHERE id = ?`, id))
}

// CodeFilter 兑换码查询条件。
type CodeFilter struct {
	Status string // unused | redeemed | disabled | expired
	Batch  string
	Query  string // 兑换码 / 备注 / 兑换人邮箱
	UserID int64  // 只看某个用户兑换的
	Limit  int
	Offset int
}

// ListRedeemCodes 兑换码列表（新→旧）。
func (s *Store) ListRedeemCodes(f CodeFilter) ([]RedeemCode, int64, error) {
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}
	ts := now()
	clauses, args := []string{"1=1"}, []any{}
	switch f.Status {
	case "redeemed":
		clauses = append(clauses, "redeemed_at > 0")
	case "disabled":
		clauses = append(clauses, "redeemed_at = 0 AND enabled = 0")
	case "expired":
		clauses, args = append(clauses, "redeemed_at = 0 AND enabled = 1 AND expires_at > 0 AND expires_at <= ?"), append(args, ts)
	case "unused":
		clauses, args = append(clauses, "redeemed_at = 0 AND enabled = 1 AND (expires_at = 0 OR expires_at > ?)"), append(args, ts)
	}
	if f.Batch != "" {
		clauses, args = append(clauses, "batch = ?"), append(args, f.Batch)
	}
	if f.UserID > 0 {
		clauses, args = append(clauses, "redeemed_by = ?"), append(args, f.UserID)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		clauses, args = append(clauses, "(code LIKE ? OR note LIKE ? OR redeemed_email LIKE ? OR batch = ?)"),
			append(args, "%"+NormalizeCode(q)+"%", "%"+q+"%", "%"+q+"%", q)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM redeem_codes`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := ` ORDER BY id DESC`
	if f.UserID > 0 {
		order = ` ORDER BY redeemed_at DESC`
	}
	rows, err := s.db.Query(`SELECT `+codeColumns+` FROM redeem_codes`+where+order+` LIMIT ? OFFSET ?`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []RedeemCode{}
	for rows.Next() {
		c, err := scanCode(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// CodeStats 兑换码汇总。
type CodeStats struct {
	Total          int64 `json:"total"`
	Unused         int64 `json:"unused"`
	Redeemed       int64 `json:"redeemed"`
	UnusedAmount   int64 `json:"unused_amount"`
	RedeemedAmount int64 `json:"redeemed_amount"`
}

// RedeemCodeStats 兑换码汇总（未使用 = 启用且未过期且未兑换）。
func (s *Store) RedeemCodeStats() (CodeStats, error) {
	var st CodeStats
	ts := now()
	err := s.db.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN redeemed_at = 0 AND enabled = 1 AND (expires_at = 0 OR expires_at > ?) THEN 1 END), 0),
		COALESCE(SUM(CASE WHEN redeemed_at > 0 THEN 1 END), 0),
		COALESCE(SUM(CASE WHEN redeemed_at = 0 AND enabled = 1 AND (expires_at = 0 OR expires_at > ?) THEN amount END), 0),
		COALESCE(SUM(CASE WHEN redeemed_at > 0 THEN amount END), 0) FROM redeem_codes`, ts, ts).
		Scan(&st.Total, &st.Unused, &st.Redeemed, &st.UnusedAmount, &st.RedeemedAmount)
	return st, err
}

// SetRedeemCodeEnabled 启停兑换码（已兑换的不能再修改）。
func (s *Store) SetRedeemCodeEnabled(id int64, enabled bool) (RedeemCode, error) {
	res, err := s.db.Exec(`UPDATE redeem_codes SET enabled = ? WHERE id = ? AND redeemed_at = 0`, boolToInt(enabled), id)
	if err != nil {
		return RedeemCode{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c, err := s.GetRedeemCode(id)
		if err != nil {
			return RedeemCode{}, err
		}
		if c.Status == "redeemed" {
			return RedeemCode{}, ErrCodeRedeemed
		}
	}
	return s.GetRedeemCode(id)
}

// DisableRedeemBatch 停用某批次全部未兑换的码，返回停用数量。
func (s *Store) DisableRedeemBatch(batch string) (int64, error) {
	res, err := s.db.Exec(`UPDATE redeem_codes SET enabled = 0 WHERE batch = ? AND redeemed_at = 0 AND enabled = 1`, batch)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteRedeemCode 删除兑换码。已兑换的码删除后，对应流水仍保留（ref_id 指向原码 ID）。
func (s *Store) DeleteRedeemCode(id int64) error {
	res, err := s.db.Exec(`DELETE FROM redeem_codes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RedeemResult 兑换结果。
type RedeemResult struct {
	Code   RedeemCode  `json:"code"`
	Entry  LedgerEntry `json:"entry"`
	Amount int64       `json:"amount"`
}

// Redeem 用户兑换。条件更新 + 同事务入账保证每个码只能成功兑换一次（并发安全）。
func (s *Store) Redeem(userID int64, rawCode string) (RedeemResult, error) {
	code := NormalizeCode(rawCode)
	if code == "" || len(code) > 64 {
		return RedeemResult{}, ErrCodeNotFound
	}
	user, err := s.GetUser(userID)
	if err != nil {
		return RedeemResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return RedeemResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	c, err := scanCode(tx.QueryRow(`SELECT `+codeColumns+` FROM redeem_codes WHERE code = ?`, code))
	if errors.Is(err, ErrNotFound) {
		return RedeemResult{}, ErrCodeNotFound
	}
	if err != nil {
		return RedeemResult{}, err
	}
	switch c.Status {
	case "redeemed":
		return RedeemResult{}, ErrCodeRedeemed
	case "disabled":
		return RedeemResult{}, ErrCodeDisabled
	case "expired":
		return RedeemResult{}, ErrCodeExpired
	}
	ts := now()
	res, err := tx.Exec(`UPDATE redeem_codes SET redeemed_by = ?, redeemed_email = ?, redeemed_at = ?
		WHERE id = ? AND redeemed_at = 0 AND redeemed_by = 0 AND enabled = 1 AND (expires_at = 0 OR expires_at > ?)`,
		userID, user.Email, ts, c.ID, ts)
	if err != nil {
		return RedeemResult{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return RedeemResult{}, ErrCodeRedeemed
	}
	note := "兑换码 " + MaskCode(c.Code)
	if c.Note != "" {
		note += " · " + c.Note
	}
	entry, err := applyBalanceTx(tx, userID, c.Amount, LedgerRedeem, c.ID, note, user.Email, false)
	if err != nil {
		return RedeemResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RedeemResult{}, err
	}
	c.RedeemedBy, c.RedeemedEmail, c.RedeemedAt = userID, user.Email, ts
	c.fillStatus(ts)
	return RedeemResult{Code: c, Entry: entry, Amount: c.Amount}, nil
}

// MaskCode 兑换码脱敏：W2A-ABCD-****-****-WXYZ。
func MaskCode(code string) string {
	parts := strings.Split(code, "-")
	if len(parts) != 5 {
		if len(code) <= 6 {
			return "****"
		}
		return code[:3] + "…" + code[len(code)-3:]
	}
	return parts[0] + "-" + parts[1] + "-****-****-" + parts[4]
}

// ---------- 用户 Key ----------

// CreateUserKey 为用户创建 Key（受每人 Key 数上限约束；与计数在同一事务内）。
func (s *Store) CreateUserKey(userID int64, name string, tokenLimit int64, maxKeys int) (APIKey, error) {
	if tokenLimit < 0 {
		return APIKey{}, errors.New("token_limit 不能为负数（0 表示不限）")
	}
	if len(name) > 200 {
		return APIKey{}, errors.New("名称最长 200 字节")
	}
	key, err := GenerateKey()
	if err != nil {
		return APIKey{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return APIKey{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if maxKeys > 0 {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE user_id = ?`, userID).Scan(&count); err != nil {
			return APIKey{}, err
		}
		if count >= maxKeys {
			return APIKey{}, fmt.Errorf("%w（每个用户最多 %d 个）", ErrKeyLimitReached, maxKeys)
		}
	}
	res, err := tx.Exec(`INSERT INTO api_keys (key, name, enabled, created_at, token_limit, user_id) VALUES (?, ?, 1, ?, ?, ?)`,
		key, strings.TrimSpace(name), now(), tokenLimit, userID)
	if err != nil {
		return APIKey{}, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return APIKey{}, err
	}
	return s.GetKey(id)
}

// ListUserKeys 用户自己的 Key。
func (s *Store) ListUserKeys(userID int64) ([]APIKey, error) {
	rows, err := s.db.Query(`SELECT `+keyColumns+` FROM api_keys WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// UserEmails 批量取用户邮箱（管理台展示 Key 归属）。
func (s *Store) UserEmails() (map[int64]string, error) {
	rows, err := s.db.Query(`SELECT id, email FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return nil, err
		}
		out[id] = email
	}
	return out, rows.Err()
}

// ---------- 平台设置 ----------

// PlatformSettings 管理员可调整的平台设置。
type PlatformSettings struct {
	// RegistrationOpen 是否开放自助注册（默认关闭，升级后的已有部署不会突然对外开放）。
	RegistrationOpen bool `json:"registration_open"`
	// SignupBonus 注册赠送余额（计费 Token）。
	SignupBonus int64 `json:"signup_bonus"`
	// DefaultUserMultiplier 新用户默认倍率。
	DefaultUserMultiplier float64 `json:"default_user_multiplier"`
	// MaxKeysPerUser 每个用户最多可创建的 Key 数。
	MaxKeysPerUser int `json:"max_keys_per_user"`
	// SiteName 显示在登录页和侧边栏的站点名。
	SiteName string `json:"site_name"`
	// Announcement 用户控制台顶部公告（可留空）。
	Announcement string `json:"announcement"`
	// VideoTokensPerSecond 用户 Key 创建视频时按秒扣的计费 Token（再乘模型/Key/用户倍率）。
	// 0 表示不向用户 Key 开放 /v1/videos（视频没有 Token 用量，避免免费使用）。
	VideoTokensPerSecond int64 `json:"video_tokens_per_second"`
	// UserUnmeteredRoutes 是否允许用户 Key 调用不计 Token 的接口（Gemini 原生、多模态透传）。默认关闭。
	UserUnmeteredRoutes bool `json:"user_unmetered_routes"`
}

// DefaultPlatformSettings 默认值。
func DefaultPlatformSettings() PlatformSettings {
	return PlatformSettings{RegistrationOpen: false, SignupBonus: 0, DefaultUserMultiplier: 1, MaxKeysPerUser: 20, SiteName: "web2api"}
}

// GetPlatformSettings 读取设置（缺省项取默认值）。
func (s *Store) GetPlatformSettings() (PlatformSettings, error) {
	out := DefaultPlatformSettings()
	rows, err := s.db.Query(`SELECT key, value FROM platform_settings`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return out, err
		}
		switch key {
		case "registration_open":
			out.RegistrationOpen = value == "true"
		case "signup_bonus":
			out.SignupBonus, _ = strconv.ParseInt(value, 10, 64)
		case "default_user_multiplier":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				out.DefaultUserMultiplier = v
			}
		case "max_keys_per_user":
			if v, err := strconv.Atoi(value); err == nil {
				out.MaxKeysPerUser = v
			}
		case "site_name":
			out.SiteName = value
		case "announcement":
			out.Announcement = value
		case "video_tokens_per_second":
			out.VideoTokensPerSecond, _ = strconv.ParseInt(value, 10, 64)
		case "user_unmetered_routes":
			out.UserUnmeteredRoutes = value == "true"
		}
	}
	return out, rows.Err()
}

// SavePlatformSettings 校验并保存设置。
func (s *Store) SavePlatformSettings(v PlatformSettings) (PlatformSettings, error) {
	if v.SignupBonus < 0 || v.SignupBonus > MaxBalanceChange {
		return v, errors.New("注册赠送必须是非负整数")
	}
	if err := ValidateMultiplier(v.DefaultUserMultiplier); err != nil {
		return v, err
	}
	if v.MaxKeysPerUser < 1 || v.MaxKeysPerUser > 1000 {
		return v, errors.New("每人 Key 上限需在 1–1000 之间")
	}
	v.SiteName = strings.TrimSpace(v.SiteName)
	if v.SiteName == "" {
		v.SiteName = "web2api"
	}
	if utf8.RuneCountInString(v.SiteName) > 40 {
		return v, errors.New("站点名最长 40 字")
	}
	if utf8.RuneCountInString(v.Announcement) > 500 {
		return v, errors.New("公告最长 500 字")
	}
	if v.VideoTokensPerSecond < 0 || v.VideoTokensPerSecond > 100_000_000 {
		return v, errors.New("视频每秒计费 Token 需在 0–100000000 之间")
	}
	values := map[string]string{
		"registration_open":       strconv.FormatBool(v.RegistrationOpen),
		"signup_bonus":            strconv.FormatInt(v.SignupBonus, 10),
		"default_user_multiplier": strconv.FormatFloat(v.DefaultUserMultiplier, 'f', -1, 64),
		"max_keys_per_user":       strconv.Itoa(v.MaxKeysPerUser),
		"site_name":               v.SiteName,
		"announcement":            strings.TrimSpace(v.Announcement),
		"video_tokens_per_second": strconv.FormatInt(v.VideoTokensPerSecond, 10),
		"user_unmetered_routes":   strconv.FormatBool(v.UserUnmeteredRoutes),
	}
	tx, err := s.db.Begin()
	if err != nil {
		return v, err
	}
	defer func() { _ = tx.Rollback() }()
	for key, value := range values {
		if _, err := tx.Exec(`INSERT INTO platform_settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return v, err
		}
	}
	if err := tx.Commit(); err != nil {
		return v, err
	}
	return s.GetPlatformSettings()
}

// ---------- 统计 ----------

// PlatformTotals 平台级汇总（管理台总览）。
type PlatformTotals struct {
	Users           int64 `json:"users"`
	UsersEnabled    int64 `json:"users_enabled"`
	BalanceTotal    int64 `json:"balance_total"`
	Recharged30d    int64 `json:"recharged_30d"`
	Consumed30d     int64 `json:"consumed_30d"`
	NewUsers7d      int64 `json:"new_users_7d"`
	ActiveUsers7d   int64 `json:"active_users_7d"`
	UnusedCodes     int64 `json:"unused_codes"`
	UnusedCodeValue int64 `json:"unused_code_value"`
}

// GetPlatformTotals 平台汇总。
func (s *Store) GetPlatformTotals() (PlatformTotals, error) {
	var t PlatformTotals
	ts := now()
	err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(enabled), 0), COALESCE(SUM(balance), 0),
		COALESCE(SUM(CASE WHEN created_at >= ? THEN 1 END), 0) FROM users`, ts-7*86400).
		Scan(&t.Users, &t.UsersEnabled, &t.BalanceTotal, &t.NewUsers7d)
	if err != nil {
		return t, err
	}
	err = s.db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN kind IN ('adjust','redeem','signup_bonus') AND amount > 0 THEN amount END), 0),
		COALESCE(SUM(CASE WHEN kind = 'usage' THEN -amount WHEN kind = 'refund' THEN -amount END), 0)
		FROM ledger WHERE ts >= ?`, ts-30*86400).Scan(&t.Recharged30d, &t.Consumed30d)
	if err != nil {
		return t, err
	}
	if err = s.db.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM usage_records WHERE user_id > 0 AND ts >= ?`, ts-7*86400).Scan(&t.ActiveUsers7d); err != nil {
		return t, err
	}
	stats, err := s.RedeemCodeStats()
	t.UnusedCodes, t.UnusedCodeValue = stats.Unused, stats.UnusedAmount
	return t, err
}
