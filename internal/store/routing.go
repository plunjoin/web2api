package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

const (
	routingAccountOrderKey = "routing_account_order"
	routingAffinityKey     = "routing_affinity"
	routingGroupModesKey   = "routing_group_modes"
)

// RoutingAccountOrder 返回管理台保存的账号路由顺序（accounts.id 列表）。
// 未配置过时返回 nil；配置损坏时同样返回 nil，由调用方按默认顺序处理，
// 不因为一条设置读不出来就让全部引擎拒绝接单。
func (s *Store) RoutingAccountOrder() ([]string, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM platform_settings WHERE key = ?`, routingAccountOrderKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, nil
	}
	return ids, nil
}

// SetRoutingAccountOrder 保存账号路由顺序。传空列表表示清除配置、恢复默认顺序。
func (s *Store) SetRoutingAccountOrder(ids []string) error {
	if ids == nil {
		ids = []string{}
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO platform_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, routingAccountOrderKey, string(raw))
	return err
}

// RoutingAffinity 返回会话粘性模式（off / session）。未配置过时返回空，
// 由调用方按默认值处理；读取失败也不影响网关继续接单。
func (s *Store) RoutingAffinity() (string, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM platform_settings WHERE key = ?`, routingAffinityKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return raw, nil
}

// SetRoutingAffinity 保存会话粘性模式。
func (s *Store) SetRoutingAffinity(mode string) error {
	_, err := s.db.Exec(`INSERT INTO platform_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, routingAffinityKey, mode)
	return err
}

// RoutingGroupModes 返回「同名模型路由组」的模式（组键 = 归一化模型名）。
// 没配置过的组不在 map 里，由调用方按默认（smart）处理。
func (s *Store) RoutingGroupModes() (map[string]string, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM platform_settings WHERE key = ?`, routingGroupModesKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		// 单条设置损坏不能拖垮全部模型路由，按默认模式继续。
		return map[string]string{}, nil
	}
	return out, nil
}

// SetRoutingGroupMode 保存一个模型路由组的模式；mode 为空表示删除该组设置。
func (s *Store) SetRoutingGroupMode(group, mode string) error {
	modes, err := s.RoutingGroupModes()
	if err != nil {
		return err
	}
	if modes == nil {
		modes = map[string]string{}
	}
	if mode == "" {
		delete(modes, group)
	} else {
		modes[group] = mode
	}
	raw, err := json.Marshal(modes)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO platform_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, routingGroupModesKey, string(raw))
	return err
}
