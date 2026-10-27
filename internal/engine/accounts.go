// accounts.go 各引擎共享的账号实时状态结构（供管理台展示）。
package engine

// AccountCooldown 账号仍在生效的额度冷却。Until 为 Unix 秒。
type AccountCooldown struct {
	Scope  string `json:"scope"`
	Until  int64  `json:"until"`
	Reason string `json:"reason,omitempty"`
}

// AccountState 账号实时状态（供管理台）。
type AccountState struct {
	ID        string            `json:"id"`
	Label     string            `json:"label"`
	Enabled   bool              `json:"enabled"`
	Ready     bool              `json:"ready"`
	Status    string            `json:"status"`
	Detail    string            `json:"detail,omitempty"`
	Models    int               `json:"models"`
	Tier      string            `json:"tier,omitempty"`
	Quota     string            `json:"quota,omitempty"` // 用量/余额描述（订阅引擎）
	Cooldowns []AccountCooldown `json:"cooldowns,omitempty"`
}

