package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

const (
	routingAccountOrderKey = "routing_account_order"
	routingAffinityKey     = "routing_affinity"
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
