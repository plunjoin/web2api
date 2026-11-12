package store

import (
	"database/sql"
	"errors"
	"sort"
	"strings"

	"web2api/internal/model"
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

// modelKey 下架记录的键。带 models/ 前缀、大小写不同的写法都归到同一个裸名，
// 否则管理台按下架的是上游目录原样（models/gpt-4o），调用方写 gpt-4o 就查不到。
func modelKey(name string) string { return model.CanonicalModelID(name) }

// ModelEnabled 模型是否对下游开放。无记录或 enabled=1 视为开放。
// 带 models/ 前缀或大小写不同的写法与裸名是同一条记录。
func (s *Store) ModelEnabled(model string) (bool, error) {
	key := modelKey(model)
	if key == "" {
		return false, errors.New("model 不能为空")
	}
	enabled, found, err := s.modelEnabledExact(key)
	if err != nil || found {
		return enabled, err
	}
	// 旧库可能存着带前缀或原始大小写的键，归一化查询对不上时按裸名再对一次。
	return s.modelEnabledLoose(key)
}

func (s *Store) modelEnabledExact(key string) (enabled bool, found bool, err error) {
	var v int
	err = s.db.QueryRow(`SELECT enabled FROM model_settings WHERE model = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return true, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return v != 0, true, nil
}

// modelEnabledLoose 在精确键没有记录时，按归一化后的裸名扫一遍已有记录。
// 记录不多（一个模型一行），全表扫描只发生在缓存未命中的冷路径上。
func (s *Store) modelEnabledLoose(key string) (bool, error) {
	item, found, err := s.modelSettingLoose(key)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	return item.Enabled, nil
}

// preferSetting 判断 candidate 是否比 current 更适合作为某个归一化键的生效记录：
// 精确的归一化键优先，其次取最近更新的一行。旧库可能同时存着 models/x 与 x 两行，
// 只有所有读取路径都用同一优先级，管理台开关、公开目录与请求拦截才会得出同一个结论。
func preferSetting(key string, current, candidate ModelSetting) bool {
	currentExact := current.Model == key
	candidateExact := candidate.Model == key
	if candidateExact != currentExact {
		return candidateExact
	}
	return candidate.UpdatedAt > current.UpdatedAt
}

// mergeSetting 把同一归一化键的两条记录合成一条生效记录：开关等基础字段取
// preferSetting 选中的那一行，检查结果取两条中较晚的一次。旧库升级后可能同时存在
// 裸名行与 models/x 旧行，检查结果往往只写在其中一行上；只挑一行会把真实检查状态
// 丢掉（与 upsertModelEnabled 写入时的合并规则保持一致）。
func mergeSetting(key string, a, b ModelSetting) ModelSetting {
	keep, other := a, b
	if preferSetting(key, a, b) {
		keep, other = b, a
	}
	if other.LastCheckAt > keep.LastCheckAt {
		keep.LastCheckAt = other.LastCheckAt
		keep.LastCheckOK = other.LastCheckOK
		keep.LastCheckDetail = other.LastCheckDetail
	}
	return keep
}

// DisabledModels 返回所有被标记为不可用的模型，键是归一化后的裸名。
// 路由与公开目录用它判断「这个模型还能不能接单」，所以必须和请求里的写法无关，
// 也必须和 ModelEnabled 使用同一套生效记录。
func (s *Store) DisabledModels() (map[string]struct{}, error) {
	list, err := s.ListModelSettings()
	if err != nil {
		return nil, err
	}
	out := map[string]struct{}{}
	for _, item := range list {
		if key := modelKey(item.Model); key != "" && !item.Enabled {
			out[key] = struct{}{}
		}
	}
	return out, nil
}

// ListModelSettings 全部已持久化的模型设置（含启用与不可用）。
// 同一归一化裸名只返回一条生效记录（精确键优先，其次最近更新），
// 避免旧库里 models/x 与 x 并存时管理台展示或读取到互相矛盾的两行。
func (s *Store) ListModelSettings() ([]ModelSetting, error) {
	rows, err := s.db.Query(`SELECT model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail
		FROM model_settings ORDER BY model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	best := map[string]ModelSetting{}
	for rows.Next() {
		item, err := scanModelSetting(rows)
		if err != nil {
			return nil, err
		}
		key := modelKey(item.Model)
		if key == "" {
			continue
		}
		if current, ok := best[key]; !ok {
			best[key] = item
		} else {
			best[key] = mergeSetting(key, current, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]ModelSetting, 0, len(best))
	for _, item := range best {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out, nil
}

// ModelSettingFor 读取单个模型设置；无记录时返回默认启用视图。
// 带 models/ 前缀或大小写不同的写法与裸名是同一条记录，返回的 Model 是实际存储的键。
func (s *Store) ModelSettingFor(model string) (ModelSetting, error) {
	key := modelKey(model)
	if key == "" {
		return ModelSetting{}, errors.New("model 不能为空")
	}
	row := s.db.QueryRow(`SELECT model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail
		FROM model_settings WHERE model = ?`, key)
	item, err := scanModelSetting(row)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ModelSetting{}, err
	}
	if item, ok, err := s.modelSettingLoose(key); err != nil || ok {
		return item, err
	}
	return ModelSetting{Model: key, Enabled: true}, nil
}

// modelSettingLoose 按归一化裸名找旧库里大小写或前缀不同的那条生效记录。
func (s *Store) modelSettingLoose(key string) (ModelSetting, bool, error) {
	// ORDER BY 让 updated_at 相同的旧行也有确定顺序：mergeSetting 只在严格更新时
	// 换检查结果，没有排序时平手取哪一行取决于 SQLite 的扫描顺序，重启后可能翻转。
	rows, err := s.db.Query(`SELECT model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail
		FROM model_settings ORDER BY model`)
	if err != nil {
		return ModelSetting{}, false, err
	}
	defer rows.Close()
	var best ModelSetting
	found := false
	for rows.Next() {
		item, err := scanModelSetting(rows)
		if err != nil {
			return ModelSetting{}, false, err
		}
		if modelKey(item.Model) != key {
			continue
		}
		if !found {
			best = item
			found = true
		} else {
			best = mergeSetting(key, best, item)
		}
	}
	return best, found, rows.Err()
}

// SetModelEnabled 设置模型是否对下游开放。写入一律用归一化裸名，
// 旧库里同一模型的带前缀记录会被合并过来，避免出现两行互相矛盾的开关。
func (s *Store) SetModelEnabled(model string, enabled bool) (ModelSetting, error) {
	key := modelKey(model)
	if key == "" {
		return ModelSetting{}, errors.New("model 不能为空")
	}
	ts := now()
	tx, err := s.db.Begin()
	if err != nil {
		return ModelSetting{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertModelEnabled(tx, key, enabled, ts); err != nil {
		return ModelSetting{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModelSetting{}, err
	}
	return s.ModelSettingFor(key)
}

// upsertModelEnabled 以归一化裸名为键写入开关，并把同一裸名的旧记录合并掉。
// 合并时取「更晚更新」的检查结果，开关本身以这次写入为准。
func upsertModelEnabled(tx *sql.Tx, key string, enabled bool, ts int64) error {
	rows, err := tx.Query(`SELECT model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail
		FROM model_settings`)
	if err != nil {
		return err
	}
	type stored struct {
		setting ModelSetting
		ok      int
	}
	var same []stored
	for rows.Next() {
		item, err := scanModelSetting(rows)
		if err != nil {
			rows.Close()
			return err
		}
		if modelKey(item.Model) != key {
			continue
		}
		ok := 0
		if item.LastCheckOK {
			ok = 1
		}
		same = append(same, stored{setting: item, ok: ok})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var keep ModelSetting
	for _, item := range same {
		if item.setting.Model == key {
			keep = item.setting
		}
	}
	for _, item := range same {
		if item.setting.LastCheckAt > keep.LastCheckAt {
			keep.LastCheckAt = item.setting.LastCheckAt
			keep.LastCheckOK = item.setting.LastCheckOK
			keep.LastCheckDetail = item.setting.LastCheckDetail
		}
		if item.setting.Model != key {
			if _, err := tx.Exec(`DELETE FROM model_settings WHERE model = ?`, item.setting.Model); err != nil {
				return err
			}
		}
	}
	ok := 0
	if keep.LastCheckOK {
		ok = 1
	}
	_, err = tx.Exec(`INSERT INTO model_settings (model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(model) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at,
			last_check_at = excluded.last_check_at, last_check_ok = excluded.last_check_ok,
			last_check_detail = excluded.last_check_detail`,
		key, boolToInt(enabled), ts, keep.LastCheckAt, ok, keep.LastCheckDetail)
	return err
}

// SetModelsEnabled 批量设置模型是否对下游开放，返回生效条数（去重、去空白、按裸名合并后）。
// 单事务写入；任一失败整体回滚。不改变已记录的检查结果。
func (s *Store) SetModelsEnabled(models []string, enabled bool) (int, error) {
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(models))
	for _, raw := range models {
		key := modelKey(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ids = append(ids, key)
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
	for _, key := range ids {
		if err := upsertModelEnabled(tx, key, enabled, ts); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// RecordModelCheck 写入最近一次模型检查结果（不改变 enabled，除非尚无记录则默认启用）。
// 写入前先把同一裸名的旧记录合并到归一化键上，避免检查结果写到另一行。
func (s *Store) RecordModelCheck(model string, ok bool, detail string) (ModelSetting, error) {
	model = modelKey(model)
	if model == "" {
		return ModelSetting{}, errors.New("model 不能为空")
	}
	detail = strings.TrimSpace(detail)
	if len(detail) > 500 {
		detail = detail[:500]
	}
	ts := now()
	// 先按当前开关合并旧记录（带前缀的键并到裸名上），再只更新检查字段。
	current, err := s.ModelEnabled(model)
	if err != nil {
		return ModelSetting{}, err
	}
	if _, err = s.SetModelEnabled(model, current); err != nil {
		return ModelSetting{}, err
	}
	_, err = s.db.Exec(`INSERT INTO model_settings (model, enabled, updated_at, last_check_at, last_check_ok, last_check_detail)
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
	Free        bool   `json:"free,omitempty"`
	FetchedAt   int64  `json:"fetched_at"`
}

// CatalogModels 全部已保存的模型快照（按引擎、模型名排序）。
func (s *Store) CatalogModels() ([]CatalogModel, error) {
	rows, err := s.db.Query(`SELECT engine, model, display_name, kind, free, fetched_at
		FROM model_catalog ORDER BY engine, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CatalogModel{}
	for rows.Next() {
		var item CatalogModel
		var free int
		if err := rows.Scan(&item.Engine, &item.Model, &item.DisplayName, &item.Kind, &free, &item.FetchedAt); err != nil {
			return nil, err
		}
		item.Free = free != 0
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
	// 「已知」必须同时包含目录快照和已有的上下架记录。只看 model_catalog 时，
	// 某个模型若只在 model_settings 里有记录（旧供应商键、旧库的 models/x 写法、
	// 目录被清空过），刷新会把它当成新模型再写一条 enabled=0 的归一化记录；
	// 精确键优先的读取规则会让这条新记录压过原来已开放的那行，模型被静默下架。
	for _, query := range []string{
		`SELECT DISTINCT model FROM model_catalog`,
		`SELECT model FROM model_settings`,
	} {
		rows, err := s.db.Query(query)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return 0, err
			}
			// 按裸名去重：models/gpt-4o 和 gpt-4o 是同一个模型，不该被当成「新出现」再下架一次。
			if key := modelKey(id); key != "" {
				known[key] = struct{}{}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return 0, err
		}
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
	stmt, err := tx.Prepare(`INSERT INTO model_catalog (engine, model, display_name, kind, free, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?)`)
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
		free := 0
		if m.Free {
			free = 1
		}
		if _, err := stmt.Exec(engine, id, strings.TrimSpace(m.DisplayName), strings.TrimSpace(m.Kind), free, ts); err != nil {
			return 0, err
		}
		// 任何供应商都没出现过的模型才算新的，默认下架；已开放或已下架的保持原状。
		if _, exists := known[modelKey(id)]; exists {
			continue
		}
		if _, err := disable.Exec(modelKey(id), ts); err != nil {
			return 0, err
		}
		// 记归一化键：后面另一个供应商用不同写法（models/xxx / xxx）列出同一模型时，
		// 不能再算一次「新出现」。
		known[modelKey(id)] = struct{}{}
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
