package store

import (
	"strings"
	"unicode/utf8"
)

// AuditEntry 一条管理操作审计记录。只记录「谁、什么时候、做了什么、结果如何」，
// 不记录密码、API Key 明文或账号凭据。
type AuditEntry struct {
	ID        int64  `json:"id"`
	TS        int64  `json:"ts"`
	Actor     string `json:"actor"`      // 操作人邮箱或 system
	ActorKind string `json:"actor_kind"` // root | admin | user | system | anonymous
	IP        string `json:"ip,omitempty"`
	Action    string `json:"action"`           // user.balance / account.delete ...
	Target    string `json:"target,omitempty"` // 被操作对象，如 user:12、account#3
	Detail    string `json:"detail,omitempty"` // 可读的操作摘要
	Result    string `json:"result"`           // ok | fail
	Error     string `json:"error,omitempty"`
}

// RecordAudit 写入一条审计记录。写入失败由调用方决定是否降级为日志。
func (s *Store) RecordAudit(e AuditEntry) error {
	if e.TS == 0 {
		e.TS = now()
	}
	if e.Result == "" {
		e.Result = "ok"
	}
	if e.ActorKind == "" {
		e.ActorKind = "system"
	}
	e.Actor = truncateAudit(e.Actor, 200)
	e.IP = truncateAudit(e.IP, 64)
	e.Action = truncateAudit(e.Action, 80)
	e.Target = truncateAudit(e.Target, 200)
	e.Detail = truncateAudit(e.Detail, 1000)
	e.Error = truncateAudit(e.Error, 1000)
	_, err := s.db.Exec(`INSERT INTO audit_logs (ts, actor, actor_kind, ip, action, target, detail, result, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.TS, e.Actor, e.ActorKind, e.IP, e.Action, e.Target, e.Detail, e.Result, e.Error)
	return err
}

// AuditFilter 审计日志查询条件。
type AuditFilter struct {
	Since  int64  // Unix 秒
	Actor  string // 模糊匹配操作人
	Action string // 精确或前缀匹配（如 user.）
	Result string // ok | fail | 空
	Query  string // 关键词：target/detail/error 模糊匹配
	Limit  int
	Offset int
}

// ListAuditLogs 审计记录（新→旧），返回总条数用于分页。
func (s *Store) ListAuditLogs(f AuditFilter) ([]AuditEntry, int64, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	clauses := []string{"ts >= ?"}
	args := []any{f.Since}
	if v := strings.TrimSpace(f.Actor); v != "" {
		clauses = append(clauses, "actor LIKE ?")
		args = append(args, "%"+v+"%")
	}
	if v := strings.TrimSpace(f.Action); v != "" {
		if strings.HasSuffix(v, ".") {
			clauses = append(clauses, "action LIKE ?")
			args = append(args, v+"%")
		} else {
			clauses = append(clauses, "action = ?")
			args = append(args, v)
		}
	}
	if v := strings.TrimSpace(f.Result); v != "" {
		clauses = append(clauses, "result = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(f.Query); v != "" {
		like := "%" + v + "%"
		clauses = append(clauses, "(target LIKE ? OR detail LIKE ? OR error LIKE ? OR actor LIKE ?)")
		args = append(args, like, like, like, like)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_logs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT id, ts, actor, actor_kind, ip, action, target, detail, result, error
		FROM audit_logs`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.TS, &e.Actor, &e.ActorKind, &e.IP, &e.Action, &e.Target, &e.Detail, &e.Result, &e.Error); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func truncateAudit(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	if n <= 0 {
		return "…"
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}
