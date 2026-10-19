package store

import (
	"database/sql"
	"errors"
	"strings"
)

// ModelSetting 管理台对模型上下架与最近一次检查结果的持久化。
// 无记录时视为启用（Enabled=true）。
type ModelSetting struct {
	Model           string `json:"model"`
	Enabled         bool   `json:"enabled"`
	UpdatedAt       int64  `json:"updated_at"`
	LastCheckAt     int64  `json:"last_check_at,omitempty"`
	LastCheckOK     bool   `json:"last_check_ok"`
	LastCheckDetail string `json:"last_check_detail,omitempty"`
}

// ModelEnabled 模型是否对下游开放。无记录或 enabled=1 视为开放。
func (s *Store) ModelEnabled(model string) (bool, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return false, errors.New("model 不能为空")
	}
	var enabled int
	err := s.db.QueryRow(`SELECT enabled FROM model_settings WHERE model = ?`, model).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return enabled != 0, nil
}

// DisabledModels 返回所有被标记为不可用的模型 ID 集合。
func (s *Store) DisabledModels() (map[string]struct{}, error) {
	rows, err := s.db.Query(`SELECT model FROM model_settings WHERE enabled = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return nil, err
		}
		out[model] = struct{}{}
	}
	return out, rows.Err()
}

// ListModelSettings 全部已持久化的模型设置（含启用与不可用）。
func (s *Store) ListModelSettings() ([]ModelSetting, error) {
	rows, err := s.db.Query(`SELECT model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail
		FROM model_settings ORDER BY model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModelSetting{}
	for rows.Next() {
		item, err := scanModelSetting(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ModelSettingFor 读取单个模型设置；无记录时返回默认启用视图。
func (s *Store) ModelSettingFor(model string) (ModelSetting, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelSetting{}, errors.New("model 不能为空")
	}
	row := s.db.QueryRow(`SELECT model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail
		FROM model_settings WHERE model = ?`, model)
	item, err := scanModelSetting(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelSetting{Model: model, Enabled: true}, nil
	}
	return item, err
}

// SetModelEnabled 设置模型是否对下游开放。
func (s *Store) SetModelEnabled(model string, enabled bool) (ModelSetting, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelSetting{}, errors.New("model 不能为空")
	}
	ts := now()
	_, err := s.db.Exec(`INSERT INTO model_settings (model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail)
		VALUES (?, ?, ?, 0, 0, '')
		ON CONFLICT(model) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`,
		model, boolToInt(enabled), ts)
	if err != nil {
		return ModelSetting{}, err
	}
	return s.ModelSettingFor(model)
}

// RecordModelCheck 写入最近一次模型检查结果（不改变 enabled，除非尚无记录则默认启用）。
func (s *Store) RecordModelCheck(model string, ok bool, detail string) (ModelSetting, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelSetting{}, errors.New("model 不能为空")
	}
	detail = strings.TrimSpace(detail)
	if len(detail) > 500 {
		detail = detail[:500]
	}
	ts := now()
	_, err := s.db.Exec(`INSERT INTO model_settings (model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail)
		VALUES (?, 1, ?, ?, ?, ?)
		ON CONFLICT(model) DO UPDATE SET
			last_check_at = excluded.last_check_at,
			last_check_ok = excluded.last_check_ok,
			last_check_detail = excluded.last_check_detail,
			updated_at = excluded.updated_at`,
		model, ts, ts, boolToInt(ok), detail)
	if err != nil {
		return ModelSetting{}, err
	}
	return s.ModelSettingFor(model)
}

type modelSettingScanner interface {
	Scan(dest ...any) error
}

func scanModelSetting(row modelSettingScanner) (ModelSetting, error) {
	var item ModelSetting
	var enabled, checkOK int
	if err := row.Scan(&item.Model, &enabled, &item.UpdatedAt, &item.LastCheckAt, &checkOK, &item.LastCheckDetail); err != nil {
		return ModelSetting{}, err
	}
	item.Enabled = enabled != 0
	item.LastCheckOK = checkOK != 0
	return item, nil
}
