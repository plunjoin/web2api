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

// SetModelsEnabled 批量设置模型是否对下游开放，返回生效条数（去重、去空白后）。
// 单事务写入；任一失败整体回滚。不改变已记录的检查结果。
func (s *Store) SetModelsEnabled(models []string, enabled bool) (int, error) {
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		ids = append(ids, model)
	}
	if len(ids) == 0 {
		return 0, errors.New("models 不能为空")
	}
	ts := now()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`INSERT INTO model_settings (model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail)
		VALUES (?, ?, ?, 0, 0, '')
		ON CONFLICT(model) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, model := range ids {
		if _, err := stmt.Exec(model, boolToInt(enabled), ts); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(ids), nil
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

// CatalogModel 某供应商快照里的一个模型。
type CatalogModel struct {
	Engine      string `json:"engine"`
	Model       string `json:"model"`
	DisplayName string `json:"display_name,omitempty"`
	Kind        string `json:"kind,omitempty"`
	FetchedAt   int64  `json:"fetched_at"`
}

// CatalogModels 全部已保存的模型快照（按引擎、模型名排序）。
func (s *Store) CatalogModels() ([]CatalogModel, error) {
	rows, err := s.db.Query(`SELECT engine, model, display_name, kind, fetched_at
		FROM model_catalog ORDER BY engine, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CatalogModel{}
	for rows.Next() {
		var item CatalogModel
		if err := rows.Scan(&item.Engine, &item.Model, &item.DisplayName, &item.Kind, &item.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ReplaceEngineCatalog 用新拉到的列表整表替换某供应商的模型快照。
// 空列表也是一次有效替换（清掉该供应商之前存下的模型）。
// 第一次出现的模型默认下架：拉取只是把供应商能提供的模型列出来，
// 是否对下游开放由管理员勾选决定。返回这次新出现（已下架）的模型数。
func (s *Store) ReplaceEngineCatalog(engine string, models []CatalogModel) (int, error) {
	engine = strings.TrimSpace(engine)
	if engine == "" {
		return 0, errors.New("engine 不能为空")
	}
	known := map[string]struct{}{}
	if rows, err := s.db.Query(`SELECT DISTINCT model FROM model_catalog`); err == nil {
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return 0, err
			}
			known[id] = struct{}{}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return 0, err
		}
	} else {
		return 0, err
	}
	ts := now()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM model_catalog WHERE engine = ?`, engine); err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`INSERT INTO model_catalog (engine, model, display_name, kind, fetched_at)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	disable, err := tx.Prepare(`INSERT INTO model_settings (model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail)
		VALUES (?, 0, ?, 0, 0, '')
		ON CONFLICT(model) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer disable.Close()
	seen := map[string]struct{}{}
	added := 0
	for _, m := range models {
		id := strings.TrimSpace(m.Model)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if _, err := stmt.Exec(engine, id, strings.TrimSpace(m.DisplayName), strings.TrimSpace(m.Kind), ts); err != nil {
			return 0, err
		}
		// 任何供应商都没出现过的模型才算新的，默认下架；已开放或已下架的保持原状。
		if _, exists := known[id]; exists {
			continue
		}
		if _, err := disable.Exec(id, ts); err != nil {
			return 0, err
		}
		known[id] = struct{}{}
		added++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return added, nil
}

// DeleteEngineCatalog 删除某个供应商键下的全部模型快照。
// 第三方 Key 拆成按上游地址存储后，用来清掉旧的整包 "custom" 快照。
func (s *Store) DeleteEngineCatalog(engine string) error {
	engine = strings.TrimSpace(engine)
	if engine == "" {
		return errors.New("engine 不能为空")
	}
	_, err := s.db.Exec(`DELETE FROM model_catalog WHERE engine = ?`, engine)
	return err
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
